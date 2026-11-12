// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/gameid/fixture"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// onlineAccounts: an administrator, two adults, a Kids profile its administrator gave one adult
// as a friend, a Kids profile with none, and a guest.
type onlineAccounts struct {
	ts                               *testServer
	owner, alex, sam, kit, kira, gus *auth.User
	ownerT, alexT, samT, kitT, gusT  string
}

func newOnlineAccounts(t *testing.T) *onlineAccounts {
	t.Helper()
	a := &onlineAccounts{ts: newTestServer(t)}
	a.owner, a.ownerT = a.ts.user(t, "owner", auth.KindAdult, true, "")
	a.alex, a.alexT = a.ts.user(t, "alex", auth.KindAdult, false, "")
	a.sam, a.samT = a.ts.user(t, "sam", auth.KindAdult, false, "")
	a.kit, a.kitT = a.ts.user(t, "kit", auth.KindKids, false, "")
	a.kira, _ = a.ts.user(t, "kira", auth.KindKids, false, "")
	a.gus, a.gusT = a.ts.user(t, "gus", auth.KindGuest, false, "")
	if err := a.ts.Store.SetKidsFriends(context.Background(), a.kit.ID, []int64{a.alex.ID, a.kit.ID}); err != nil {
		t.Fatal(err)
	}
	return a
}

func friendNames(t *testing.T, s *Server, u *auth.User) []string {
	t.Helper()
	list, err := s.playFriends(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range list {
		out = append(out, f.Name)
	}
	return out
}

// Friends are the server's other profiles: a Kids profile sees, and is seen by, only those an
// administrator chose; a guest has none and is nobody's.
func TestPlayFriendsFollowTheRules(t *testing.T) {
	a := newOnlineAccounts(t)
	for _, c := range []struct {
		who  *auth.User
		want []string
	}{
		{a.owner, []string{"alex", "sam"}},
		{a.alex, []string{"kit", "owner", "sam"}},
		{a.kit, []string{"alex"}}, // never itself, though it was listed
		{a.kira, nil},
		{a.gus, nil},
	} {
		if got := friendNames(t, a.ts.Server, c.who); !slices.Equal(got, c.want) {
			t.Errorf("%s sees %v, want %v", c.who.Name, got, c.want)
		}
	}
}

// What a friend is doing shows only as far as it shares: someone who doesn't share is offline to
// everyone.
func TestPresenceSharesOnlyWhatAProfileAllows(t *testing.T) {
	a := newOnlineAccounts(t)
	ctx := context.Background()
	p := a.ts.playPresence(ctx, []auth.User{*a.alex, *a.sam})
	if p[a.alex.ID].State != "online" || p[a.sam.ID].State != "online" {
		t.Fatalf("signed in just now: %+v", p)
	}
	if err := a.ts.Store.SetPresence(ctx, a.sam.ID, library.Presence{Playing: false, Online: false}); err != nil {
		t.Fatal(err)
	}
	g := a.ts.games()
	g.seats.started("s1", a.alex.ID, "alex", "TV", seatGame{Item: 7, Title: "Star Tide", Console: "snes"})
	g.seats.started("s2", a.sam.ID, "sam", "TV", seatGame{Item: 8, Title: "Moth Racer", Console: "snes"})
	p = a.ts.playPresence(ctx, []auth.User{*a.alex, *a.sam})
	if p[a.alex.ID].State != "playing" || p[a.alex.ID].Game != "Star Tide" {
		t.Errorf("alex: %+v", p[a.alex.ID])
	}
	if p[a.sam.ID].State != "offline" || p[a.sam.ID].Game != "" {
		t.Errorf("sam doesn't appear online, and showed %+v", p[a.sam.ID])
	}
	// Online, but not saying what it plays: online, with no game.
	if err := a.ts.Store.SetPresence(ctx, a.sam.ID, library.Presence{Playing: false, Online: true}); err != nil {
		t.Fatal(err)
	}
	if p := a.ts.playPresence(ctx, []auth.User{*a.sam}); p[a.sam.ID].State != "online" || p[a.sam.ID].Game != "" {
		t.Errorf("sam appears online without its game, and showed %+v", p[a.sam.ID])
	}
}

// A room opened by its host takes a friend with no code; a seat someone holds is swapped when the
// host moves another into it; the snapshot shows the game being played.
func TestOpenRoomsAndMovingSeats(t *testing.T) {
	var b seatBook
	b.started("s1", 1, "Willow", "TV", seatGame{Item: 7, Title: "Star Tide", Console: "snes", Players: 2})
	if _, err := b.joinOpen("s1", 2, "Pip"); err != errRoomClosed {
		t.Fatalf("joined a closed room: %v", err)
	}
	if b.setOpen("s1", 2, true) {
		t.Fatal("someone else opened the host's room")
	}
	if !b.setOpen("s1", 1, true) {
		t.Fatal("the host couldn't open its room")
	}
	j, err := b.joinOpen("s1", 2, "Pip")
	if err != nil || j.Seat != 1 {
		t.Fatalf("Pip: %+v %v", j, err)
	}
	j, _ = b.joinOpen("s1", 3, "Sam")
	if j.Seat != 2 {
		t.Fatalf("Sam: %+v", j)
	}
	if err := b.assign("s1", 2, 3, 1); err != errSeatNotHost {
		t.Errorf("a player moved another: %v", err)
	}
	if err := b.assign("s1", 1, 3, 1); err != nil {
		t.Fatal(err)
	}
	tables := b.tablesNow()
	if len(tables) != 1 || !tables[0].Open || tables[0].Game.Title != "Star Tide" {
		t.Fatalf("%+v", tables)
	}
	seats := map[int64]int{}
	for _, h := range tables[0].Holders {
		seats[h.UserID] = h.Seat
	}
	if seats[3] != 1 || seats[2] != 2 {
		t.Errorf("not swapped: %v", seats)
	}
	if err := b.assign("s1", 1, 9, 1); err != errSeatNobody {
		t.Errorf("moved someone who isn't there: %v", err)
	}
	if err := b.assign("s1", 1, 3, 0); err != errSeatNumber {
		t.Errorf("moved someone into the host's seat: %v", err)
	}
	b.setOpen("s1", 1, false)
	if _, err := b.joinOpen("s1", 4, "Lee"); err != errRoomClosed {
		t.Errorf("joined after it closed: %v", err)
	}
}

// resolverAs is a resolver for a page asked for by this profile.
func resolverAs(s *Server, u *auth.User) *sduiResolver {
	r := httptest.NewRequest("GET", "/api/sdui/pages/games", nil)
	r = r.WithContext(context.WithValue(r.Context(), userKey, u))
	return s.newSDUIResolver(r, nil)
}

func onlineItems(t *testing.T, s *Server, u *auth.User, src string, params ...any) []sdui.Item {
	t.Helper()
	source := sdui.Source{"type": src}
	for i := 0; i+1 < len(params); i += 2 {
		source[params[i].(string)] = params[i+1]
	}
	res, ok, err := resolverAs(s, u).wittyPlayOnline(sdui.SourceRequest{Source: source, Limit: 50})
	if !ok || err != nil {
		t.Fatalf("%s: %v %v", src, ok, err)
	}
	return res.Items
}

// The sources, as a profile is given them: friends playing first, with the game only when the
// friend shares it; no rooms for a Kids profile; a room with its seats, its code for its host only;
// a player for each seat.
func TestPlayOnlineSources(t *testing.T) {
	a := newOnlineAccounts(t)
	g := a.ts.games()
	g.seats.started("s1", a.sam.ID, "sam", "Living room", seatGame{Item: 7, Title: "Star Tide", Console: "snes", Players: 2})

	friends := onlineItems(t, a.ts.Server, a.alex, srcPlayFriends)
	if len(friends) != 3 || friends[0].Title != "sam" || friends[0].Badges[0] != "Playing" {
		t.Fatalf("alex's friends: %+v", friends)
	}
	// Alex may not open item 7 (there's no such game here), so the game isn't named.
	if friends[0].Subtitle != "Playing" {
		t.Errorf("a game alex can't open was named: %q", friends[0].Subtitle)
	}
	if got := onlineItems(t, a.ts.Server, a.kit, srcPlayFriends); len(got) != 1 || got[0].Title != "alex" {
		t.Errorf("kit's friends: %+v", got)
	}

	// Not open yet: no rooms. Open, but of a game alex can't open: still none.
	if got := onlineItems(t, a.ts.Server, a.alex, srcPlayRooms); len(got) != 0 {
		t.Errorf("a closed room offered: %+v", got)
	}
	g.seats.setOpen("s1", a.sam.ID, true)
	if got := onlineItems(t, a.ts.Server, a.alex, srcPlayRooms); len(got) != 0 {
		t.Errorf("a room of a game alex may not play: %+v", got)
	}
	if got := onlineItems(t, a.ts.Server, a.kit, srcPlayRooms); len(got) != 0 {
		t.Errorf("a Kids profile was offered a room: %+v", got)
	}

	// The host's room: the room, then a card for each player.
	code, _, _ := g.seats.invite("s1", a.sam.ID)
	room := onlineItems(t, a.ts.Server, a.sam, srcPlayRoom)
	if len(room) != 3 || room[0].Title != "Star Tide" || room[0].Tagline != code || room[1].Title != "sam" || room[2].Title != "Waiting…" {
		t.Fatalf("sam's room: %+v", room)
	}
	if _, err := g.seats.joinOpen("s1", a.alex.ID, "alex"); err != nil {
		t.Fatal(err)
	}
	room = onlineItems(t, a.ts.Server, a.alex, srcPlayRoom)
	if len(room) != 3 || room[0].Tagline != "" || room[2].Title != "alex" {
		t.Errorf("alex's view of the room, with no code: %+v", room)
	}
	pads := onlineItems(t, a.ts.Server, a.alex, srcPlayControllers)
	if len(pads) != 2 || pads[0].Subtitle != "sam · Living room" || pads[1].Subtitle != "alex" {
		t.Errorf("controllers: %+v", pads)
	}
	if got := onlineItems(t, a.ts.Server, a.kira, srcPlayRoom); len(got) != 0 {
		t.Errorf("a room kira isn't in: %+v", got)
	}
}

// Memories are a profile's own saves and kept screenshots, newest first, of games it may see.
func TestMemoriesListOnlyGamesTheProfileMaySee(t *testing.T) {
	a := newOnlineAccounts(t)
	dir := a.ts.games().shotsDir(a.alex.ID, 99)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "1794460000000000000.png"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Item 99 isn't a game alex may open, so it gives nothing.
	if got := onlineItems(t, a.ts.Server, a.alex, srcPlayMemories); len(got) != 0 {
		t.Errorf("%+v", got)
	}
	if shots := listShots(dir); len(shots) != 1 || shots[0].Taken != 1794460000 {
		t.Errorf("%+v", shots)
	}
}

// The routes: a profile's choice to share, and an administrator's choice of a Kids profile's
// friends.
func TestPlayOnlineRoutes(t *testing.T) {
	a := newOnlineAccounts(t)
	ts := a.ts
	var me struct {
		SharePlaying bool `json:"sharePlaying"`
		AppearOnline bool `json:"appearOnline"`
	}
	w := ts.do(t, "GET", "/api/games/online/me", a.alexT, nil)
	if json.Unmarshal(w.Body.Bytes(), &me) != nil || !me.SharePlaying || !me.AppearOnline {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	w = ts.do(t, "PUT", "/api/games/online/me", a.alexT, map[string]any{"sharePlaying": false})
	if json.Unmarshal(w.Body.Bytes(), &me) != nil || me.SharePlaying || !me.AppearOnline {
		t.Fatalf("one choice changes only that one: %d %s", w.Code, w.Body.String())
	}
	w = ts.do(t, "PUT", "/api/games/online/me", a.alexT, map[string]any{"appearOnline": false})
	if json.Unmarshal(w.Body.Bytes(), &me) != nil || me.SharePlaying || me.AppearOnline {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if w := ts.do(t, "PUT", "/api/games/online/me", a.gusT, map[string]any{"sharePlaying": true}); w.Code != 403 {
		t.Errorf("a guest shared: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/games/online/me", a.alexT, map[string]any{}); w.Code != 400 {
		t.Errorf("no choice: %d", w.Code)
	}

	kids := "/api/games/online/kids/" + itoa64(a.kit.ID)
	if w := ts.do(t, "GET", kids, a.alexT, nil); w.Code != 403 {
		t.Errorf("not an administrator: %d", w.Code)
	}
	w = ts.do(t, "PUT", kids, a.ownerT, map[string]any{"friends": []int64{a.sam.ID, a.alex.ID}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), itoa64(a.sam.ID)) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if w := ts.do(t, "PUT", kids, a.ownerT, map[string]any{"friends": []int64{a.gus.ID}}); w.Code != 400 {
		t.Errorf("a guest as a friend: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/games/online/kids/"+itoa64(a.alex.ID), a.ownerT, nil); w.Code != 400 {
		t.Errorf("an adult's chosen friends: %d", w.Code)
	}
	var page struct {
		Candidates []kidsCandidate `json:"candidates"`
	}
	if err := json.Unmarshal(ts.do(t, "GET", kids, a.ownerT, nil).Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	approved := map[string]bool{}
	for _, c := range page.Candidates {
		approved[c.Name] = c.Approved
	}
	// Every profile but kit itself and the guest, approved or not.
	if len(page.Candidates) != 4 || !approved["alex"] || !approved["sam"] || approved["owner"] || approved["kira"] {
		t.Errorf("candidates: %+v", page.Candidates)
	}
	if got := friendNames(t, ts.Server, a.kit); !slices.Equal(got, []string{"alex", "sam"}) {
		t.Errorf("kit's friends now: %v", got)
	}
}

// With a game the profiles may play: it's in the recent row with its era, players, time played
// and latest save; in its era's collection; its saves are listed; and an open room of it is offered
// to a friend, who can join it with no code, though a Kids profile can't.
func TestPlayOnlineWithAGame(t *testing.T) {
	g := newGameServer(t)
	s := g.s
	s.Modules = modtest.Need(t, "wittyplay") // as on a real server: its sdui.json says what each source gives
	ctx := context.Background()
	game := addGame(t, g, "snes", gameFile(t, "Game.sfc", fixture.SNES("STAR TIDE")))
	users, err := s.Auth.Users(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*auth.User{}
	for i := range users {
		byName[users[i].Name] = &users[i]
	}
	admin, other := byName["admin"], byName["other"]
	if err := s.Store.NoteGamePlayed(ctx, admin.ID, game.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.SetProgress(ctx, admin.ID, game.ID, library.Progress{PositionSec: 3*3600 + 12*60}); err != nil {
		t.Fatal(err)
	}
	dir := s.games().saveDir(admin.ID, game.ID)
	name := slotFileNames()[3]
	if name == "" {
		t.Fatal("no name for slot 3")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".state"), []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}

	recent := onlineItems(t, s, admin, srcPlayRecent)
	if len(recent) != 1 || recent[0].Badges[0] != "16-bit" || recent[0].RuntimeSec != 3*3600+12*60 {
		t.Fatalf("recent: %+v", recent)
	}
	if want := "16-bit classics · 1 to 2 players · Played 3 hr 12 min · Save state 3 ready"; recent[0].Subtitle != want {
		t.Errorf("recent's line: %q, want %q", recent[0].Subtitle, want)
	}
	eras := onlineItems(t, s, admin, srcPlayCollections)
	if len(eras) != 1 || eras[0].ID != "era-16-bit" || eras[0].Count != 1 || eras[0].Kind != "collection" {
		t.Errorf("collections: %+v", eras)
	}
	if got := onlineItems(t, s, admin, srcPlayCollections, "era", "16-bit"); len(got) != 1 || got[0].Kind != "game" {
		t.Errorf("the 16-bit games: %+v", got)
	}
	if got := onlineItems(t, s, admin, srcPlayCollections, "era", "pocket"); len(got) != 0 {
		t.Errorf("handheld games: %+v", got)
	}
	saves := onlineItems(t, s, admin, srcPlaySaveStates, "item", itoa64(game.ID))
	if len(saves) != 1 || saves[0].ID != "slot-3" || saves[0].Title != "Save state 3" {
		t.Errorf("saves: %+v", saves)
	}
	if got := onlineItems(t, s, admin, srcPlayMemories); len(got) != 1 || got[0].Badges[0] != "Save" {
		t.Errorf("memories: %+v", got)
	}

	// Through the path every module source takes: what WittyPlay's sdui.json says each yields, and
	// what any module item is held to.
	for _, src := range wittyPlayOnlineSources {
		if _, err := resolverAs(s, admin).moduleItems(wittyPlayID, sdui.SourceRequest{Source: sdui.Source{"type": src, "item": itoa64(game.ID)}, Limit: 20}); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
	cleaned, err := resolverAs(s, admin).moduleItems(wittyPlayID, sdui.SourceRequest{Source: sdui.Source{"type": srcPlayRecent}, Limit: 20})
	if err != nil || len(cleaned.Items) != 1 || cleaned.Items[0].Scope != wittyPlayID || cleaned.Items[0].Subtitle != recent[0].Subtitle {
		t.Errorf("recent, cleaned: %+v %v", cleaned, err)
	}
	cleaned, _ = resolverAs(s, admin).moduleItems(wittyPlayID, sdui.SourceRequest{Source: sdui.Source{"type": srcPlayCollections}, Limit: 20})
	if len(cleaned.Items) != 1 || cleaned.Items[0].Count != 1 {
		t.Errorf("collections, cleaned: %+v", cleaned)
	}

	// An open room of it, for the other adult to join with no code.
	seats := &s.games().seats
	seats.started("room1", admin.ID, "admin", "TV", seatGame{Item: game.ID, Title: game.Title, Console: "snes"})
	seats.setOpen("room1", admin.ID, true)
	rooms := onlineItems(t, s, other, srcPlayRooms)
	if len(rooms) != 1 || rooms[0].ID != "room1" || rooms[0].Title != game.Title+" · 1 of 2" {
		t.Fatalf("rooms: %+v", rooms)
	}
	if got := onlineItems(t, s, admin, srcPlayRooms); len(got) != 0 {
		t.Errorf("the host was offered its own room: %+v", got)
	}
	// Joining needs the session to be running; there isn't one, so it's refused as gone, and a
	// Kids profile is refused before that.
	if w := g.do("POST", "/api/play/join", g.kid, map[string]any{"room": "room1"}); w.Code != 404 {
		t.Errorf("joined by someone who may not: %d %s", w.Code, w.Body.String())
	}
}
