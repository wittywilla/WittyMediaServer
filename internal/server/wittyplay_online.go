// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/play/stream"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// WittyPlay Online (docs/plans/DESIGN-2-SDUI-GAPS.md, section 6; docs/GAMING-API.md, Playing
// together): the data behind its home row, classics, friends, rooms, save states, memories and
// controllers. They're WittyPlay's sources (declared in its sdui.json), answered here because
// what they show is this server's: who is playing what, a profile's saves and history, the
// other profiles. Each goes through the profile's limits like any source, and a module's items
// are cleaned the same way (cleanModuleItem).
//
// Friends are the other profiles of this server. A profile chooses whether they see what it plays,
// and whether it appears online at all. A Kids profile sees, and is seen by, only the profiles an administrator
// chose for it, and gets no open rooms. A guest has no friends. A room is a game being played on
// this server with seats for others (game_seats.go): its host may open it to friends, who then
// join with no code.

const (
	srcPlayRecent      = "wittyplay.recent"
	srcPlayCollections = "wittyplay.collections"
	srcPlayFriends     = "wittyplay.friends"
	srcPlayRooms       = "wittyplay.rooms"
	srcPlayRoom        = "wittyplay.room"
	srcPlaySaveStates  = "wittyplay.saveStates"
	srcPlayMemories    = "wittyplay.memories"
	srcPlayControllers = "wittyplay.controllers"
	// The profile's own two choices as switches, and an administrator's choice of a Kids
	// profile's friends (item: the profile's ID), each a row with a control.
	srcPlayPrivacy     = "wittyplay.privacy"
	srcPlayKidsFriends = "wittyplay.kidsFriends"
)

// wittyPlayOnlineSources are the sources the server answers for WittyPlay.
var wittyPlayOnlineSources = []string{srcPlayRecent, srcPlayCollections, srcPlayFriends, srcPlayRooms, srcPlayRoom, srcPlaySaveStates, srcPlayMemories, srcPlayControllers, srcPlayPrivacy, srcPlayKidsFriends}

// Presence: how recently a profile was seen, from its sign-ins.
const (
	presenceOnline = 5 * time.Minute
	presenceAway   = time.Hour
	// shotsKept is how many screenshots a profile keeps of one game; the oldest go first.
	shotsKept = 200
)

// Eras as WittyPlay Online shows them, in the order its Classics tab lists them.
var eraOrder = []string{eraFirst, era8Bit, era16Bit, eraEarly3D, eraDisc, eraPocket}

var eraLabels = map[string]struct{ badge, title string }{
	eraFirst:   {"First era", "First-era classics"},
	era8Bit:    {"8-bit", "8-bit classics"},
	era16Bit:   {"16-bit", "16-bit classics"},
	eraEarly3D: {"3D era", "3D-era classics"},
	eraDisc:    {"Disc era", "Disc-era classics"},
	eraPocket:  {"Handheld", "Handheld classics"},
}

// consoleEra is a console's era, "" when it has none (a custom console, a test one).
func consoleEra(console string) string { return consoleTraitsTable[console].Era }

// playersWords says how many may play a console's games at once.
func playersWords(ports int) string {
	switch {
	case ports <= 1:
		return "1 player"
	default:
		return fmt.Sprintf("1 to %d players", ports)
	}
}

// playedWords is time played as a person says it: "Played 3 hr 12 min".
func playedWords(sec float64) string {
	if sec < 60 {
		return ""
	}
	d := time.Duration(sec) * time.Second
	h, m := int(d.Hours()), int(d.Minutes())%60
	switch {
	case h == 0:
		return fmt.Sprintf("Played %d min", m)
	case m == 0:
		return fmt.Sprintf("Played %d hr", h)
	}
	return fmt.Sprintf("Played %d hr %d min", h, m)
}

// ---------------------------------------------------------------- friends and presence

// presence is what a friend is doing.
type presence struct {
	State string // playing, online, away, offline
	Game  string // with playing: the game's title
	Item  int64
}

var presenceWords = map[string]string{"playing": "Playing", "online": "Online", "away": "Away", "offline": "Offline"}

// playFriends is the profiles this one may see as friends, by name. A guest has none; a Kids
// profile has the ones an administrator chose; anyone else every profile but guests and the Kids
// profiles that weren't given them.
func (s *Server) playFriends(ctx context.Context, viewer *auth.User) ([]auth.User, error) {
	if viewer == nil || viewer.Kind == auth.KindGuest {
		return nil, nil
	}
	all, err := s.Auth.Users(ctx)
	if err != nil {
		return nil, err
	}
	var mine []int64
	if viewer.Kind == auth.KindKids {
		if mine, err = s.Store.KidsFriends(ctx, viewer.ID); err != nil {
			return nil, err
		}
	}
	out := []auth.User{}
	for _, u := range all {
		if u.ID == viewer.ID || u.Kind == auth.KindGuest {
			continue
		}
		if viewer.Kind == auth.KindKids && !slices.Contains(mine, u.ID) {
			continue
		}
		if u.Kind == auth.KindKids {
			theirs, err := s.Store.KidsFriends(ctx, u.ID)
			if err != nil || !slices.Contains(theirs, viewer.ID) {
				continue
			}
		}
		out = append(out, u)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(profileName(&out[i])) < strings.ToLower(profileName(&out[j]))
	})
	return out, nil
}

// profileName is the name a profile goes by.
func profileName(u *auth.User) string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Name
}

// playPresence is what each of these profiles is doing, as far as each lets others know.
func (s *Server) playPresence(ctx context.Context, users []auth.User) map[int64]presence {
	out := map[int64]presence{}
	if len(users) == 0 {
		return out
	}
	seen := map[int64]int64{}
	if rows, err := s.DB.QueryContext(ctx, `SELECT user_id, MAX(IFNULL(last_seen, created_at)) FROM sessions WHERE expires_at > ? GROUP BY user_id`, time.Now().Unix()); err == nil {
		for rows.Next() {
			var id, at int64
			if rows.Scan(&id, &at) == nil {
				seen[id] = at
			}
		}
		_ = rows.Close()
	}
	playing := map[int64]seatTableView{}
	if s.gameSvc != nil {
		for _, t := range s.gameSvc.seats.tablesNow() {
			playing[t.Host] = t
			for _, h := range t.Holders {
				playing[h.UserID] = t
			}
		}
	}
	now := time.Now()
	for _, u := range users {
		choice := s.Store.PresenceOf(ctx, u.ID, u.Kind)
		if !choice.Online {
			out[u.ID] = presence{State: "offline"}
			continue
		}
		if t, ok := playing[u.ID]; ok && t.Game.Title != "" {
			if choice.Playing {
				out[u.ID] = presence{State: "playing", Game: t.Game.Title, Item: t.Game.Item}
			} else {
				out[u.ID] = presence{State: "online"} // here, and not saying what it plays
			}
			continue
		}
		ago := now.Sub(time.Unix(seen[u.ID], 0))
		switch {
		case seen[u.ID] > 0 && ago <= presenceOnline:
			out[u.ID] = presence{State: "online"}
		case seen[u.ID] > 0 && ago <= presenceAway:
			out[u.ID] = presence{State: "away"}
		default:
			out[u.ID] = presence{State: "offline"}
		}
	}
	return out
}

// mayJoinRoom says whether a profile may join an open room: not a Kids profile or a guest, and
// only one whose host it may see as a friend.
func (s *Server) mayJoinRoom(ctx context.Context, viewer *auth.User, host int64) bool {
	if viewer == nil || viewer.Kind == auth.KindKids || viewer.Kind == auth.KindGuest {
		return false
	}
	friends, err := s.playFriends(ctx, viewer)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(friends, func(u auth.User) bool { return u.ID == host })
}

// ---------------------------------------------------------------- saves and screenshots

var (
	slotNamesOnce sync.Once
	slotNames     map[int]string // slot → file name, as WittyPlay names them
)

// slotFileNames is every save slot's file name. WittyPlay names them, and they never change, so
// it's asked once.
func slotFileNames() map[int]string {
	slotNamesOnce.Do(func() {
		names := map[int]string{}
		for slot := 0; slot <= stream.SlotPrevAuto; slot++ {
			name, err := stream.SlotName(slot)
			if err != nil {
				return // asked again next time
			}
			names[slot] = name
		}
		slotNames = names
	})
	if slotNames == nil {
		slotNamesOnce = sync.Once{}
	}
	return slotNames
}

// listSaves is a game's saves in a profile's folder.
func listSaves(dir string) []saveSlot {
	found := map[string]os.FileInfo{}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if info, err := e.Info(); err == nil && !e.IsDir() {
				found[e.Name()] = info
			}
		}
	}
	out := []saveSlot{}
	names := slotFileNames()
	for slot := 0; slot <= stream.SlotPrevAuto; slot++ {
		name, ok := names[slot]
		if !ok {
			continue
		}
		st, ok := found[name+".state"]
		if !ok {
			continue
		}
		_, thumb := found[name+".png"]
		out = append(out, saveSlot{Slot: slot, UpdatedAt: st.ModTime().Unix(), Size: st.Size(), HasThumb: thumb, Core: slotCore(dir, name)})
	}
	return out
}

// latestOwnSave is the slot of a person's own save made last (1 to MaxSlot), or 0 for none.
func latestOwnSave(saves []saveSlot) int {
	best, at := 0, int64(0)
	for _, sv := range saves {
		if sv.Slot >= 1 && sv.Slot <= stream.MaxSlot && sv.UpdatedAt > at {
			best, at = sv.Slot, sv.UpdatedAt
		}
	}
	return best
}

func slotTitle(slot int) string {
	switch slot {
	case 0:
		return "Automatic save"
	case stream.SlotUndo:
		return "Before the last load"
	case stream.SlotPrevAuto:
		return "Automatic save before starting over"
	}
	return fmt.Sprintf("Save state %d", slot)
}

// shotsDir is where a profile keeps a game's screenshots.
func (g *gameService) shotsDir(userID, itemID int64) string {
	return filepath.Join(g.saveDir(userID, itemID), "shots")
}

// shotName is a kept screenshot's file name: the moment it was taken, and its extension.
var shotName = regexp.MustCompile(`^[0-9]{10,19}\.png$`)

type keptShot struct {
	Name  string
	Taken int64
}

func listShots(dir string) []keptShot {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []keptShot
	for _, e := range entries {
		if e.IsDir() || !shotName.MatchString(e.Name()) {
			continue
		}
		n, _ := strconv.ParseInt(strings.TrimSuffix(e.Name(), ".png"), 10, 64)
		out = append(out, keptShot{Name: e.Name(), Taken: n / int64(time.Second)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out
}

// ---------------------------------------------------------------- sources

// wittyPlayOnline answers one of WittyPlay Online's sources. ok is false for any other source,
// which WittyPlay's helper answers.
func (rv *sduiResolver) wittyPlayOnline(req sdui.SourceRequest) (sdui.SourceResult, bool, error) {
	typ := req.Source.Type()
	if !slices.Contains(wittyPlayOnlineSources, typ) {
		return sdui.SourceResult{}, false, nil
	}
	if !rv.acc.Can(library.FeatureGames) {
		return sdui.SourceResult{}, true, nil
	}
	// Rooms and players are games running now: none before the first game since the server started.
	if rv.s.gameSvc == nil && (typ == srcPlayRooms || typ == srcPlayRoom || typ == srcPlayControllers) {
		return sdui.SourceResult{}, true, nil
	}
	var all []sdui.Item
	var err error
	switch typ {
	case srcPlayRecent:
		all, err = rv.playRecent(req)
	case srcPlayCollections:
		all, err = rv.playCollections(req)
	case srcPlayFriends:
		all, err = rv.playFriends()
	case srcPlayRooms:
		all, err = rv.playRooms()
	case srcPlayRoom:
		all = rv.playRoom()
	case srcPlaySaveStates:
		all = rv.playSaveStates(req)
	case srcPlayMemories:
		all = rv.playMemories()
	case srcPlayControllers:
		all = rv.playControllers()
	case srcPlayPrivacy:
		all = rv.playPrivacy()
	case srcPlayKidsFriends:
		all, err = rv.playKidsFriends(req)
	}
	if err != nil {
		return sdui.SourceResult{}, true, err
	}
	if lim := req.Source.Int("limit"); lim > 0 && lim < req.Limit {
		req.Limit = lim
	}
	return page(all, req), true, nil
}

// playRecent: the games this profile played, newest first, with their era, how many may play,
// how long it has played and its latest save.
func (rv *sduiResolver) playRecent(req sdui.SourceRequest) ([]sdui.Item, error) {
	rows, err := rv.s.Store.GameList(rv.ctx, rv.u.ID, false, min(2*(req.Offset+req.Limit), 400))
	if err != nil {
		return nil, err
	}
	var out []sdui.Item
	for _, e := range rows {
		li, err := rv.s.visibleItem(rv.r, e.ItemID)
		if err != nil || li.Kind != "game" {
			continue
		}
		it := rv.item(li)
		era := consoleEra(li.Console)
		line := []string{}
		if l, ok := eraLabels[era]; ok {
			it.Badges = []string{l.badge}
			line = append(line, l.title)
		}
		if ports := consoleTraitsTable[li.Console].Caps.Input.Ports; ports > 0 {
			line = append(line, playersWords(ports))
		}
		played := rv.s.Store.GamePlayedSeconds(rv.ctx, rv.u.ID, li.ID)
		if w := playedWords(played); w != "" {
			line = append(line, w)
			it.RuntimeSec = played
		}
		if slot := latestOwnSave(listSaves(rv.s.games().saveDir(rv.u.ID, li.ID))); slot > 0 {
			line = append(line, fmt.Sprintf("Save state %d ready", slot))
		}
		it.Subtitle = strings.Join(line, " · ")
		out = append(out, it)
	}
	return out, nil
}

// visibleGames is every game this profile may see in its games libraries.
func (rv *sduiResolver) visibleGames() ([]*library.Item, error) {
	var out []*library.Item
	for _, l := range rv.libraries() {
		if l.Type != "games" {
			continue
		}
		items, _, err := rv.s.Store.Items(rv.ctx, library.ItemQuery{LibraryID: l.ID, Kind: "game", Access: rv.acc, Brief: true, Limit: 20000})
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

// playCollections: the games this profile may see, by era, with how many each has. With era, the
// games of that era, by title.
func (rv *sduiResolver) playCollections(req sdui.SourceRequest) ([]sdui.Item, error) {
	games, err := rv.visibleGames()
	if err != nil {
		return nil, err
	}
	if era := req.Source.Str("era"); era != "" {
		var out []sdui.Item
		for _, g := range games {
			if consoleEra(g.Console) == era {
				it := rv.item(g)
				it.Badges = []string{eraLabels[era].badge}
				out = append(out, it)
			}
		}
		sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title) })
		return out, nil
	}
	count := map[string]int{}
	for _, g := range games {
		count[consoleEra(g.Console)]++
	}
	var out []sdui.Item
	for _, era := range eraOrder {
		if count[era] == 0 {
			continue
		}
		l := eraLabels[era]
		out = append(out, sdui.Item{ID: "era-" + era, Kind: "collection", Title: l.title, Count: count[era], Badges: []string{l.badge}, Icon: "game",
			Subtitle: fmt.Sprintf("%d %s", count[era], plural(count[era], "game", "games")), Route: "/games?era=" + era})
	}
	return out, nil
}

// playFriends: the profiles this one may see, each with what it's doing, playing first.
func (rv *sduiResolver) playFriends() ([]sdui.Item, error) {
	friends, err := rv.s.playFriends(rv.ctx, rv.u)
	if err != nil {
		return nil, err
	}
	pres := rv.s.playPresence(rv.ctx, friends)
	rank := map[string]int{"playing": 0, "online": 1, "away": 2, "offline": 3}
	sort.SliceStable(friends, func(i, j int) bool { return rank[pres[friends[i].ID].State] < rank[pres[friends[j].ID].State] })
	out := make([]sdui.Item, 0, len(friends))
	for _, f := range friends {
		p := pres[f.ID]
		sub := presenceWords[p.State]
		if p.State == "playing" {
			// A game this profile couldn't open itself is "Playing", with no title.
			if _, err := rv.s.visibleItem(rv.r, p.Item); err == nil {
				sub = "Playing " + p.Game
			}
		}
		// A person card that opens nothing: there's no profile page to open.
		out = append(out, sdui.Item{ID: "profile-" + strconv.FormatInt(f.ID, 10), Kind: "person", Title: profileName(&f), Subtitle: sub,
			Badges: []string{presenceWords[p.State]}, Icon: "user"})
	}
	return out, nil
}

// roomWords is a room's line: "Shared · Sam".
func roomWords(host string) string { return "Shared · " + host }

// seatsTaken is how many players a room has: its host and everyone in a seat.
func seatsTaken(t seatTableView) int {
	n := 1 // the host
	for _, h := range t.Holders {
		if h.Seat > 0 {
			n++
		}
	}
	return n
}

// playRooms: the open rooms this profile may join: friends' games with a seat free, of games it
// may play. None for a Kids profile or a guest.
func (rv *sduiResolver) playRooms() ([]sdui.Item, error) {
	if rv.u.Kind == auth.KindKids || rv.u.Kind == auth.KindGuest {
		return nil, nil
	}
	friends, err := rv.s.playFriends(rv.ctx, rv.u)
	if err != nil {
		return nil, err
	}
	var out []sdui.Item
	for _, t := range rv.s.gameSvc.seats.tablesNow() {
		if !t.Open || t.Host == rv.u.ID || !slices.ContainsFunc(friends, func(u auth.User) bool { return u.ID == t.Host }) {
			continue
		}
		if slices.ContainsFunc(t.Holders, func(h seatHolder) bool { return h.UserID == rv.u.ID }) {
			continue // already in it: it's this profile's room
		}
		if _, err := rv.s.visibleItem(rv.r, t.Game.Item); err != nil {
			continue
		}
		taken, total := seatsTaken(t), max(t.Game.Players, 1)
		if taken >= total {
			continue
		}
		out = append(out, sdui.Item{ID: t.Session, Kind: "view", Title: fmt.Sprintf("%s · %d of %d", t.Game.Title, taken, total), Subtitle: roomWords(t.HostName),
			Count: taken, Icon: "users", Badges: []string{"Shared"}})
	}
	return out, nil
}

// ownTable is the game this profile is playing now, as its host or in a seat.
func (rv *sduiResolver) ownTable() (seatTableView, bool) {
	for _, t := range rv.s.gameSvc.seats.tablesNow() {
		if t.Host == rv.u.ID || slices.ContainsFunc(t.Holders, func(h seatHolder) bool { return h.UserID == rv.u.ID }) {
			return t, true
		}
	}
	return seatTableView{}, false
}

// playRoom: this profile's room (a view), then each player as a person: the host, "Player n", or
// "Waiting…" for a free seat. The room's code, which lets anyone join, is the host's alone to see.
func (rv *sduiResolver) playRoom() []sdui.Item {
	t, ok := rv.ownTable()
	if !ok {
		return nil
	}
	room := sdui.Item{ID: t.Session, Kind: "view", Title: t.Game.Title, Subtitle: roomWords(t.HostName), Icon: "users", Badges: []string{"Shared"}, Count: seatsTaken(t)}
	if t.Host == rv.u.ID {
		if v, ok := rv.s.gameSvc.seats.view(t.Session, true); ok && v.Code != "" {
			room.Tagline = v.Code
			room.Route = "/games?join=" + v.Code
		}
		if t.Open {
			room.Badges = append(room.Badges, "Open")
		}
	}
	out := []sdui.Item{room}
	players := max(t.Game.Players, 1)
	for n := 0; n < players; n++ {
		it := sdui.Item{ID: fmt.Sprintf("%s-p%d", t.Session, n+1), Kind: "person", Subtitle: fmt.Sprintf("Player %d", n+1), Badges: []string{fmt.Sprintf("P%d", n+1)}, Icon: "user"}
		switch {
		case n == 0:
			it.Title, it.Subtitle = t.HostName, "Host"
		default:
			at := slices.IndexFunc(t.Holders, func(h seatHolder) bool { return h.Seat == n })
			if at >= 0 {
				it.Title = t.Holders[at].Name
			} else {
				it.Title, it.Subtitle, it.Badges = "Waiting…", "Free", []string{"Free"}
			}
		}
		out = append(out, it)
	}
	return out
}

// playSaveStates: one game's saves, a person's own first, newest first.
func (rv *sduiResolver) playSaveStates(req sdui.SourceRequest) []sdui.Item {
	id, err := strconv.ParseInt(req.Source.Str("item"), 10, 64)
	if err != nil {
		return nil
	}
	li, err := rv.s.visibleItem(rv.r, id)
	if err != nil || li.Kind != "game" {
		return nil
	}
	saves := listSaves(rv.s.games().saveDir(rv.u.ID, li.ID))
	sort.SliceStable(saves, func(i, j int) bool {
		own := func(s saveSlot) bool { return s.Slot >= 1 && s.Slot <= stream.MaxSlot }
		if own(saves[i]) != own(saves[j]) {
			return own(saves[i])
		}
		return saves[i].UpdatedAt > saves[j].UpdatedAt
	})
	out := make([]sdui.Item, 0, len(saves))
	for _, sv := range saves {
		it := sdui.Item{ID: fmt.Sprintf("slot-%d", sv.Slot), Kind: "view", Title: slotTitle(sv.Slot), Subtitle: savedWords(sv.UpdatedAt), Icon: "history",
			Route: fmt.Sprintf("/game/%d", li.ID), Badges: []string{"Save"}}
		if sv.HasThumb {
			it.Images = map[string]string{"backdrop": fmt.Sprintf("/api/games/%d/saves/%d/thumb", li.ID, sv.Slot)}
		}
		out = append(out, it)
	}
	return out
}

// savedWords is when something was saved: "Today 19:42", "Yesterday", "3 November".
func savedWords(unix int64) string {
	t := time.Unix(unix, 0)
	now := time.Now()
	y1, m1, d1 := now.Date()
	y2, m2, d2 := t.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return "Today " + t.Format("15:04")
	case now.Sub(t) < 48*time.Hour && now.AddDate(0, 0, -1).Day() == d2:
		return "Yesterday " + t.Format("15:04")
	case y1 == y2:
		return t.Format("2 January")
	}
	return t.Format("2 January 2006")
}

// playMemories: this profile's own saves and kept screenshots, in every game it may still see,
// newest first.
func (rv *sduiResolver) playMemories() []sdui.Item {
	root := filepath.Dir(rv.s.games().saveDir(rv.u.ID, 0))
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	type memory struct {
		it sdui.Item
		at int64
	}
	var all []memory
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "i") {
			continue
		}
		id, err := strconv.ParseInt(strings.TrimPrefix(e.Name(), "i"), 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		li, err := rv.s.visibleItem(rv.r, id)
		if err != nil || li.Kind != "game" {
			continue
		}
		dir := rv.s.games().saveDir(rv.u.ID, id)
		for _, sv := range listSaves(dir) {
			if sv.Slot < 1 || sv.Slot > stream.MaxSlot {
				continue
			}
			it := sdui.Item{ID: fmt.Sprintf("save-%d-%d", id, sv.Slot), Kind: "view", Title: li.Title, Subtitle: slotTitle(sv.Slot) + " · " + savedWords(sv.UpdatedAt),
				Badges: []string{"Save"}, Icon: "history", Route: fmt.Sprintf("/game/%d", id)}
			if sv.HasThumb {
				it.Images = map[string]string{"backdrop": fmt.Sprintf("/api/games/%d/saves/%d/thumb", id, sv.Slot)}
			}
			all = append(all, memory{it, sv.UpdatedAt})
		}
		for _, sh := range listShots(rv.s.games().shotsDir(rv.u.ID, id)) {
			all = append(all, memory{sdui.Item{ID: fmt.Sprintf("shot-%d-%s", id, strings.TrimSuffix(sh.Name, ".png")), Kind: "photo", Title: li.Title,
				Subtitle: "Screenshot · " + savedWords(sh.Taken), Badges: []string{"Shot"},
				Action: &sdui.Action{Type: sdui.ActNavigate, To: fmt.Sprintf("/page/online-memory?item=%d&name=%s", id, url.QueryEscape(sh.Name)), As: sdui.PresentLayer},
				Images: map[string]string{"backdrop": fmt.Sprintf("/api/games/%d/shots/%s", id, sh.Name)}}, sh.Taken})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].at > all[j].at })
	out := make([]sdui.Item, len(all))
	for i, m := range all {
		out[i] = m.it
	}
	return out
}

// playControllers: each player of the game this profile is in, and who or what is playing it. A
// controller's battery and kind are the device's to say: the server doesn't see them.
func (rv *sduiResolver) playControllers() []sdui.Item {
	t, ok := rv.ownTable()
	if !ok {
		return nil
	}
	device := ""
	if v, ok := rv.s.gameSvc.seats.view(t.Session, false); ok {
		device = v.Host.Device
	}
	players := max(t.Game.Players, 1)
	out := make([]sdui.Item, 0, players)
	for n := 0; n < players; n++ {
		it := sdui.Item{ID: fmt.Sprintf("player-%d", n+1), Kind: "view", Title: fmt.Sprintf("Player %d", n+1), Icon: "devices", Badges: []string{fmt.Sprintf("P%d", n+1)}}
		switch at := slices.IndexFunc(t.Holders, func(h seatHolder) bool { return h.Seat == n }); {
		case n == 0:
			it.Subtitle = t.HostName
			if device != "" {
				it.Subtitle += " · " + device
			}
		case at >= 0:
			it.Subtitle = t.Holders[at].Name
		default:
			it.Subtitle, it.Badges = "No controller", append(it.Badges, "Free")
		}
		out = append(out, it)
	}
	return out
}

// playPrivacy: the two choices a profile makes about what its friends know, as switches in a Form
// (wittyplay.online.set saves both). A guest has no friends, so both are off and say why.
func (rv *sduiResolver) playPrivacy() []sdui.Item {
	p := rv.s.Store.PresenceOf(rv.ctx, rv.u.ID, rv.u.Kind)
	guest := rv.u.Kind == auth.KindGuest
	why := ""
	if guest {
		why = "A guest has no friends to share with."
	}
	return []sdui.Item{
		{ID: "sharePlaying", Kind: "view", Title: "Show friends what I’m playing", Subtitle: "Friends see “Playing Star Tide”. Off shows you as Online only.", Icon: "game",
			Control: &sdui.Control{Type: sdui.ControlSwitch, Name: "sharePlaying", Value: p.Playing && !guest, Disabled: guest, Why: why}},
		{ID: "appearOnline", Kind: "view", Title: "Appear online", Subtitle: "Off shows you as offline to everyone, playing or not.", Icon: "users",
			Control: &sdui.Control{Type: sdui.ControlSwitch, Name: "appearOnline", Value: p.Online && !guest, Disabled: guest, Why: why}},
	}
}

// playKidsFriends: for an administrator, every profile that could be a Kids profile's friend, each a
// switch whose action saves the whole list with that one changed (wittyplay.kids.friends). Nothing for
// anyone else, or when the profile isn't a Kids profile.
func (rv *sduiResolver) playKidsFriends(req sdui.SourceRequest) ([]sdui.Item, error) {
	if !rv.u.IsAdmin {
		return nil, nil
	}
	id, err := strconv.ParseInt(req.Source.Str("user"), 10, 64)
	if err != nil {
		return nil, nil
	}
	kid, err := rv.s.Auth.User(rv.ctx, id)
	if err != nil || kid == nil || kid.Kind != auth.KindKids {
		return nil, nil
	}
	approved, err := rv.s.Store.KidsFriends(rv.ctx, kid.ID)
	if err != nil {
		return nil, err
	}
	all, err := rv.s.Auth.Users(rv.ctx)
	if err != nil {
		return nil, err
	}
	var out []sdui.Item
	for _, o := range all {
		if o.ID == kid.ID || o.Kind == auth.KindGuest {
			continue
		}
		on := slices.Contains(approved, o.ID)
		next := []int64{}
		for _, a := range approved {
			if a != o.ID {
				next = append(next, a)
			}
		}
		if !on {
			next = append(next, o.ID)
		}
		out = append(out, sdui.Item{ID: "profile-" + strconv.FormatInt(o.ID, 10), Kind: "view", Title: profileName(&o), Icon: "user", Trailing: map[bool]string{true: "Approved", false: "Not approved"}[on],
			Action:  &sdui.Action{Type: sdui.ActCommand, Op: "wittyplay.kids.friends", Item: strconv.FormatInt(kid.ID, 10), Params: map[string]any{"friends": next}},
			Control: &sdui.Control{Type: sdui.ControlSwitch, Value: on}})
	}
	return out, nil
}

// ---------------------------------------------------------------- routes

func (s *Server) playOnlineRoutes(m routes) {
	a := s.requireUser
	m.Handle("POST /api/play/{sid}/room", a(s.handlePlayRoom))
	m.Handle("PUT /api/play/{sid}/seats/{user}", a(s.handlePlaySeatMove))
	m.Handle("POST /api/play/{sid}/screenshot", a(s.handlePlayScreenshotKeep))
	m.Handle("GET /api/games/{id}/shots/{name}", a(s.handleGameShot))
	m.Handle("DELETE /api/games/{id}/shots/{name}", a(s.handleGameShotDelete))
	m.Handle("GET /api/games/online/me", a(s.handlePlayOnlineMe))
	m.Handle("PUT /api/games/online/me", a(s.handlePlayOnlineMePut))
	m.Handle("GET /api/games/online/kids/{user}", s.requireAdmin(s.handlePlayKidsFriends))
	m.Handle("PUT /api/games/online/kids/{user}", s.requireAdmin(s.handlePlayKidsFriendsPut))
}

// POST /api/play/{sid}/room: the host opens its game to the friends who may see it, or closes it.
// It answers the code that lets anyone it's given to join, which a phone's QR code carries.
func (s *Server) handlePlayRoom(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Open *bool `json:"open"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	sess := s.session(r)
	if sess == nil {
		writeErr(w, 404, "not found")
		return
	}
	u := userFrom(r)
	open := b.Open == nil || *b.Open
	if open && (u.Kind == auth.KindKids || u.Kind == auth.KindGuest) {
		writeErr(w, 403, "This profile’s games can’t be opened to others. Give a code to the people you want instead.")
		return
	}
	seats := &s.games().seats
	if !seats.setOpen(sess.ID(), u.ID, open) {
		writeErr(w, 404, "not found")
		return
	}
	out := map[string]any{"sid": sess.ID(), "open": open}
	if v, ok := seats.view(sess.ID(), true); ok && v.Code != "" {
		out["code"], out["expiresAt"] = v.Code, v.ExpiresAt
	} else if code, expires, ok := seats.invite(sess.ID(), u.ID); ok {
		out["code"], out["expiresAt"] = code, expires.Unix()
	}
	s.Log.InfoContext(r.Context(), "game room opened or closed", "cat", "games", "session", sess.ID(), "open", open)
	writeJSON(w, 200, out)
}

// PUT /api/play/{sid}/seats/{user}: the host moves someone who joined to another player, or to
// watching (seat -1). A seat someone else holds is swapped.
func (s *Server) handlePlaySeatMove(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Seat int `json:"seat"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	sess := s.session(r)
	user, ok := pathInt(r, "user")
	if sess == nil || !ok {
		writeErr(w, 404, "not found")
		return
	}
	if err := s.games().seats.assign(sess.ID(), userFrom(r).ID, user, b.Seat); err != nil {
		code := 400
		if err == errSeatNobody || err == errSeatNotHost {
			code = 404
		}
		writeErr(w, code, err.Error())
		return
	}
	v, _ := s.games().seats.view(sess.ID(), true)
	writeJSON(w, 200, v)
}

// POST /api/play/{sid}/screenshot: keeps a screenshot of the host's game in its memories.
func (s *Server) handlePlayScreenshotKeep(w http.ResponseWriter, r *http.Request) {
	sess := s.session(r)
	if sess == nil {
		writeErr(w, 404, "not found")
		return
	}
	b, err := sess.Screenshot()
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	if _, err := png.DecodeConfig(bytes.NewReader(b)); err != nil {
		writeErr(w, 409, "The game gave no picture to keep.")
		return
	}
	item := sess.Info().ItemID
	dir := s.games().shotsDir(userFrom(r).ID, item)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeErr(w, 500, "The screenshot couldn’t be kept.")
		return
	}
	name := strconv.FormatInt(time.Now().UnixNano(), 10) + ".png"
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
		writeErr(w, 500, "The screenshot couldn’t be kept.")
		return
	}
	shots := listShots(dir)
	for _, old := range shots[min(len(shots), shotsKept):] {
		_ = os.Remove(filepath.Join(dir, old.Name))
	}
	writeJSON(w, 201, map[string]any{"name": name, "itemId": item, "url": fmt.Sprintf("/api/games/%d/shots/%s", item, name)})
}

// shotPath is one of the profile's kept screenshots of a game it may see, or "".
func (s *Server) shotPath(r *http.Request) string {
	it, ok := s.gameItem(r)
	name := r.PathValue("name")
	if !ok || !shotName.MatchString(name) {
		return ""
	}
	p := filepath.Join(s.games().shotsDir(userFrom(r).ID, it.ID), name)
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

func (s *Server) handleGameShot(w http.ResponseWriter, r *http.Request) {
	p := s.shotPath(r)
	if p == "" {
		writeErr(w, 404, "not found")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, p)
}

func (s *Server) handleGameShotDelete(w http.ResponseWriter, r *http.Request) {
	p := s.shotPath(r)
	if p == "" {
		writeErr(w, 404, "not found")
		return
	}
	if err := os.Remove(p); err != nil {
		writeErr(w, 500, "The screenshot couldn’t be deleted.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/games/online/me: what this profile lets its friends know: what it plays
// (sharePlaying; off, it's shown online with no game) and whether it appears online at all
// (appearOnline; off, it's offline to everyone).
func (s *Server) handlePlayOnlineMe(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	p := s.Store.PresenceOf(r.Context(), u.ID, u.Kind)
	writeJSON(w, 200, map[string]any{"sharePlaying": p.Playing, "appearOnline": p.Online, "canShare": u.Kind != auth.KindGuest})
}

// PUT /api/games/online/me: either choice, or both.
func (s *Server) handlePlayOnlineMePut(w http.ResponseWriter, r *http.Request) {
	var b struct {
		SharePlaying *bool `json:"sharePlaying"`
		AppearOnline *bool `json:"appearOnline"`
	}
	if err := readJSON(r, &b); err != nil || (b.SharePlaying == nil && b.AppearOnline == nil) {
		writeErr(w, 400, "Say sharePlaying or appearOnline: true or false.")
		return
	}
	u := userFrom(r)
	if u.Kind == auth.KindGuest {
		writeErr(w, 403, "A guest has no friends to share with.")
		return
	}
	p := s.Store.PresenceOf(r.Context(), u.ID, u.Kind)
	if b.SharePlaying != nil {
		p.Playing = *b.SharePlaying
	}
	if b.AppearOnline != nil {
		p.Online = *b.AppearOnline
	}
	if err := s.Store.SetPresence(r.Context(), u.ID, p); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.handlePlayOnlineMe(w, r)
}

// kidsProfile is the Kids profile a route names, for an administrator.
func (s *Server) kidsProfile(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	id, ok := pathInt(r, "user")
	if !ok {
		writeErr(w, 404, "not found")
		return nil, false
	}
	u, err := s.Auth.User(r.Context(), id)
	if err != nil || u == nil {
		writeErr(w, 404, "not found")
		return nil, false
	}
	if u.Kind != auth.KindKids {
		writeErr(w, 400, "Only a Kids profile has friends chosen for it.")
		return nil, false
	}
	return u, true
}

// kidsCandidate is a profile that could be a Kids profile's friend, and whether it is.
type kidsCandidate struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Approved bool   `json:"approved"`
}

// GET /api/games/online/kids/{user}: the friends a Kids profile may see and be seen by, and every
// profile that could be one (any but itself and guests), for the page that chooses them.
func (s *Server) handlePlayKidsFriends(w http.ResponseWriter, r *http.Request) {
	u, ok := s.kidsProfile(w, r)
	if !ok {
		return
	}
	ids, err := s.Store.KidsFriends(r.Context(), u.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	all, err := s.Auth.Users(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	candidates := []kidsCandidate{}
	for _, o := range all {
		if o.ID == u.ID || o.Kind == auth.KindGuest {
			continue
		}
		candidates = append(candidates, kidsCandidate{ID: o.ID, Name: profileName(&o), Kind: o.Kind, Approved: slices.Contains(ids, o.ID)})
	}
	writeJSON(w, 200, map[string]any{"user": u.ID, "name": profileName(u), "friends": ids, "candidates": candidates})
}

func (s *Server) handlePlayKidsFriendsPut(w http.ResponseWriter, r *http.Request) {
	u, ok := s.kidsProfile(w, r)
	if !ok {
		return
	}
	var b struct {
		Friends []int64 `json:"friends"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(b.Friends) > 200 {
		writeErr(w, 400, "That’s more friends than this server has profiles.")
		return
	}
	all, err := s.Auth.Users(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, id := range b.Friends {
		if !slices.ContainsFunc(all, func(x auth.User) bool { return x.ID == id && x.Kind != auth.KindGuest }) {
			writeErr(w, 400, fmt.Sprintf("Profile %d isn’t one that can be a friend.", id))
			return
		}
	}
	if err := s.Store.SetKidsFriends(r.Context(), u.ID, b.Friends); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	logging.Audit(r.Context(), s.Log, "a Kids profile's game friends changed", "cat", "profiles", "profile", u.Name, "friends", len(b.Friends), "by", userFrom(r).Name)
	s.handlePlayKidsFriends(w, r)
}
