// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/channels"
	"github.com/wittywilla/wittymediaserver/internal/livetv"
)

// Channel sets: which channels the guide shows, and in what order. Some come from what the
// server has (every channel, favourites, each source, each group of a playlist); the rest are
// a person's own, made from any channels they like, which an administrator can share with the
// household. Which set a device shows is the device's own choice, kept by the app.

func (s *Server) tvSetRoutes(m *http.ServeMux) {
	a := s.requireUser
	m.Handle("GET /api/tv-sets", a(s.handleTVSets))
	m.Handle("POST /api/tv-sets", a(s.handleCreateTVSet))
	m.Handle("PUT /api/tv-sets/order", a(s.handleOrderTVSets))
	m.Handle("GET /api/tv-sets/{id}", a(s.handleTVSet))
	m.Handle("PUT /api/tv-sets/{id}", a(s.handleUpdateTVSet))
	m.Handle("POST /api/tv-sets/{id}/channels", a(s.handleUpdateTVSet))
	m.Handle("DELETE /api/tv-sets/{id}", a(s.handleDeleteTVSet))
}

// guideIdx is every channel a profile may see, numbered in order, with what's needed to
// narrow it down: a set, a search, a page.
type guideIdx struct {
	rows      []guideChannel
	tunerList []*livetv.Channel
}

func (s *Server) guideIndex(ctx context.Context, r *http.Request) *guideIdx {
	rows := s.libraryRows(ctx, r)
	trows, list := s.tunerIndex(ctx, r)
	rows = append(rows, trows...)
	favs, err := s.Channels.Favourites(ctx, userFrom(r).ID)
	if err != nil {
		s.Log.WarnContext(ctx, "could not read favourite channels", "cat", "tv", "err", err)
	}
	for i := range rows {
		rows[i].Favourite = favs[rows[i].Key]
	}
	sort.SliceStable(rows, func(i, j int) bool { return channels.NumberLess(rows[i].Number, rows[j].Number) })
	return &guideIdx{rows: rows, tunerList: list}
}

// groupNames: every group and category, the two everyone knows first, then the rest by name.
func (ix *guideIdx) groupNames() []string {
	seen := map[string]bool{}
	for _, c := range ix.rows {
		if c.Group != "" {
			seen[c.Group] = true
		}
		for _, g := range c.Categories {
			seen[g] = true
		}
	}
	names := make([]string, 0, len(seen))
	for g := range seen {
		names = append(names, g)
	}
	rank := func(g string) int {
		return map[string]int{"Movies": 0, "Kids": 1}[g] + b2i(g != "Movies" && g != "Kids")*2
	}
	sort.Slice(names, func(i, j int) bool {
		if a, b := rank(names[i]), rank(names[j]); a != b {
			return a < b
		}
		return names[i] < names[j]
	})
	return names
}

func inGroup(c *guideChannel, g string) bool { return c.Group == g || slices.Contains(c.Categories, g) }

func tunerOf(c *guideChannel) int64 {
	if c.tc == nil {
		return 0
	}
	return c.tc.TunerID
}

var errNoSuchSet = errors.New("set not found")

// guidePick is a set's channels, in the set's order: a person's own order for a custom set,
// by number for the rest.
func (s *Server) guidePick(ctx context.Context, r *http.Request, ix *guideIdx, id string) ([]guideChannel, error) {
	keep := func(ok func(*guideChannel) bool) []guideChannel {
		out := make([]guideChannel, 0, len(ix.rows))
		for i := range ix.rows {
			if ok(&ix.rows[i]) {
				out = append(out, ix.rows[i])
			}
		}
		return out
	}
	kind, arg, _ := strings.Cut(id, ":")
	switch {
	case id == "" || id == "all":
		return ix.rows, nil
	case id == "fav":
		return keep(func(c *guideChannel) bool { return c.Favourite }), nil
	case kind == "grp" && arg != "":
		return keep(func(c *guideChannel) bool { return inGroup(c, arg) }), nil
	case id == "src:lib":
		return keep(func(c *guideChannel) bool { return c.Kind == "library" }), nil
	case kind == "src" && strings.HasPrefix(arg, "tun-"):
		tid, err := strconv.ParseInt(strings.TrimPrefix(arg, "tun-"), 10, 64)
		if err != nil {
			return nil, errNoSuchSet
		}
		return keep(func(c *guideChannel) bool { return tunerOf(c) == tid }), nil
	case kind == "set":
		sid, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return nil, errNoSuchSet
		}
		set, err := s.Channels.GetSet(ctx, userFrom(r).ID, sid)
		if err != nil {
			return nil, errNoSuchSet
		}
		byKey := make(map[string]int, len(ix.rows))
		for i, c := range ix.rows {
			byKey[c.Key] = i
		}
		out := make([]guideChannel, 0, len(set.Channels))
		for _, k := range set.Channels {
			// What this profile may not see stays in the set, and isn't shown.
			if i, ok := byKey[k]; ok {
				out = append(out, ix.rows[i])
			}
		}
		return out, nil
	}
	return nil, errNoSuchSet
}

// channelMatches: find is lower case. A number matches from its start, a name anywhere.
func channelMatches(c *guideChannel, find string) bool {
	return strings.HasPrefix(c.Number, find) || strings.Contains(strings.ToLower(c.Name), find)
}

// locateChannel finds a channel by its key or its number: the place of the first with that
// number, which a dialled 7 finds as 7, 07 or 7.0.
func locateChannel(rows []guideChannel, want string) int {
	for i, c := range rows {
		if c.Key == want || c.Number == want {
			return i
		}
	}
	if n, err := strconv.ParseFloat(want, 64); err == nil {
		for i, c := range rows {
			if m, err := strconv.ParseFloat(c.Number, 64); err == nil && m == n {
				return i
			}
		}
	}
	return -1
}

// liteChannel is a guide row with only what a channel picker or dialling needs: a list
// of ten thousand of these is a few hundred kilobytes rather than megabytes.
type liteChannel struct {
	Key       string        `json:"key"`
	Kind      string        `json:"kind"`
	ID        int64         `json:"id"`
	Number    string        `json:"number"`
	Name      string        `json:"name"`
	Group     string        `json:"group,omitempty"`
	Logo      string        `json:"logo,omitempty"`
	Brand     *channelBrand `json:"brand,omitempty"`
	Favourite bool          `json:"favourite,omitempty"`
	Encrypted bool          `json:"encrypted,omitempty"`
}

// setInfo is one entry of the set picker.
type setInfo struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"` // all | favourites | custom | source | group
	Name     string `json:"name"`
	Count    int    `json:"count"`
	Shared   bool   `json:"shared,omitempty"`
	Editable bool   `json:"editable,omitempty"`
	Mine     bool   `json:"mine,omitempty"`
	Driver   string `json:"driver,omitempty"`
}

// GET /api/tv-sets: every set this profile can choose, with how many channels it would show.
func (s *Server) handleTVSets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(r)
	ix := s.guideIndex(ctx, r)
	visible := make(map[string]bool, len(ix.rows))
	favs, lib := 0, 0
	byTuner := map[int64]int{}
	byGroup := map[string]int{}
	for i := range ix.rows {
		c := &ix.rows[i]
		visible[c.Key] = true
		if c.Favourite {
			favs++
		}
		if c.Kind == "library" {
			lib++
		} else {
			byTuner[tunerOf(c)]++
		}
		counted := map[string]bool{}
		for _, g := range append([]string{c.Group}, c.Categories...) {
			if g != "" && !counted[g] {
				counted[g] = true
				byGroup[g]++
			}
		}
	}
	out := []setInfo{{ID: "all", Kind: "all", Name: "All channels", Count: len(ix.rows)}}
	if favs > 0 {
		out = append(out, setInfo{ID: "fav", Kind: "favourites", Name: "Favourites", Count: favs})
	}
	own, err := s.Channels.Sets(ctx, u.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, x := range own {
		n := 0
		for _, k := range x.Keys {
			if visible[k] {
				n++
			}
		}
		mine := x.UserID == u.ID
		out = append(out, setInfo{ID: "set:" + strconv.FormatInt(x.ID, 10), Kind: "custom", Name: x.Name, Count: n, Shared: x.Shared, Mine: mine,
			Editable: channels.MayEditSet(&channels.Set{UserID: x.UserID, Shared: x.Shared}, u.ID, u.IsAdmin)})
	}
	// A source of its own is worth choosing only when there's more than one.
	var sources []setInfo
	if lib > 0 {
		sources = append(sources, setInfo{ID: "src:lib", Kind: "source", Name: "My library", Count: lib, Driver: "library"})
	}
	if len(byTuner) > 0 && s.TV != nil {
		tuners, err := s.TV.Tuners(ctx)
		if err != nil {
			s.Log.WarnContext(ctx, "could not list tuners for the set picker", "cat", "tv", "err", err)
		}
		for _, t := range tuners {
			if n := byTuner[t.ID]; n > 0 {
				sources = append(sources, setInfo{ID: "src:tun-" + strconv.FormatInt(t.ID, 10), Kind: "source", Name: t.Name, Count: n, Driver: t.Driver})
			}
		}
	}
	if len(sources) > 1 {
		out = append(out, sources...)
	}
	for _, g := range ix.groupNames() {
		out = append(out, setInfo{ID: "grp:" + g, Kind: "group", Name: g, Count: byGroup[g]})
	}
	writeJSON(w, 200, map[string]any{"sets": out, "canShare": u.IsAdmin})
}

// setID reads {id}: the number, or "set:7" as the picker names it.
func setID(r *http.Request) (int64, bool) {
	v := strings.TrimPrefix(r.PathValue("id"), "set:")
	id, err := strconv.ParseInt(v, 10, 64)
	return id, err == nil && id > 0
}

type setAnswer struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Shared   bool     `json:"shared"`
	Editable bool     `json:"editable"`
	Mine     bool     `json:"mine"`
	Channels []string `json:"channels"`
}

func answerSet(x *channels.Set, u int64, admin bool) setAnswer {
	return setAnswer{ID: "set:" + strconv.FormatInt(x.ID, 10), Name: x.Name, Shared: x.Shared, Editable: channels.MayEditSet(x, u, admin), Mine: x.UserID == u, Channels: x.Channels}
}

// GET /api/tv-sets/{id}: one custom set, with its channels in order.
func (s *Server) handleTVSet(w http.ResponseWriter, r *http.Request) {
	id, ok := setID(r)
	if !ok {
		writeErr(w, 404, "set not found")
		return
	}
	u := userFrom(r)
	x, err := s.Channels.GetSet(r.Context(), u.ID, id)
	if err != nil {
		s.setError(w, err)
		return
	}
	writeJSON(w, 200, answerSet(x, u.ID, u.IsAdmin))
}

func (s *Server) setError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, channels.ErrSetNotFound):
		writeErr(w, 404, err.Error())
	case errors.Is(err, channels.ErrSetNotYours):
		writeErr(w, 403, err.Error())
	default:
		writeErr(w, 400, err.Error())
	}
}

// keysMayBeSeen: a set can only be given channels the person may watch. That they could
// have known a hidden channel's key from elsewhere isn't a reason to keep it.
func (s *Server) keysMayBeSeen(r *http.Request, keys []string) bool {
	for _, k := range keys {
		// What isn't a channel key at all is dropped when the set is saved.
		if channels.ValidKey(k) && !s.canSeeChannelKey(r, k) {
			return false
		}
	}
	return true
}

// POST /api/tv-sets: a new set of the caller's own. Sharing it needs an administrator.
func (s *Server) handleCreateTVSet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string   `json:"name"`
		Channels []string `json:"channels"`
		Shared   bool     `json:"shared"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	u := userFrom(r)
	if body.Shared && !u.IsAdmin {
		writeErr(w, 403, "only an administrator can share a set with everyone")
		return
	}
	if len(body.Channels) > channels.MaxSetChannels {
		writeErr(w, 400, "that's more channels than a set can hold")
		return
	}
	if !s.keysMayBeSeen(r, body.Channels) {
		writeErr(w, 400, "one of those channels isn't one this profile can watch")
		return
	}
	x, err := s.Channels.CreateSet(r.Context(), u.ID, body.Name, body.Channels, body.Shared)
	if err != nil {
		s.setError(w, err)
		return
	}
	writeJSON(w, 201, answerSet(x, u.ID, u.IsAdmin))
}

// PUT /api/tv-sets/{id} renames, shares or replaces the channels of a set;
// POST /api/tv-sets/{id}/channels adds and removes some.
func (s *Server) handleUpdateTVSet(w http.ResponseWriter, r *http.Request) {
	id, ok := setID(r)
	if !ok {
		writeErr(w, 404, "set not found")
		return
	}
	var c channels.SetChange
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if r.Method == http.MethodPost {
		c = channels.SetChange{Add: c.Add, Remove: c.Remove}
	} else {
		c.Add, c.Remove = nil, nil
	}
	u := userFrom(r)
	x, err := s.Channels.GetSet(r.Context(), u.ID, id)
	if err != nil {
		s.setError(w, err)
		return
	}
	if !channels.MayEditSet(x, u.ID, u.IsAdmin) {
		s.setError(w, channels.ErrSetNotYours)
		return
	}
	if c.Shared != nil && *c.Shared != x.Shared && !u.IsAdmin {
		writeErr(w, 403, "only an administrator can share a set with everyone")
		return
	}
	added := c.Add
	if c.Channels != nil {
		// Only what's new needs checking: a channel already in the set stays, even one this
		// profile can no longer see.
		have := map[string]bool{}
		for _, k := range x.Channels {
			have[k] = true
		}
		for _, k := range *c.Channels {
			if !have[k] {
				added = append(added, k)
			}
		}
	}
	if len(added) > channels.MaxSetChannels {
		writeErr(w, 400, "that's more channels than a set can hold")
		return
	}
	if !s.keysMayBeSeen(r, added) {
		writeErr(w, 400, "one of those channels isn't one this profile can watch")
		return
	}
	x, err = s.Channels.UpdateSet(r.Context(), u.ID, id, c)
	if err != nil {
		s.setError(w, err)
		return
	}
	writeJSON(w, 200, answerSet(x, u.ID, u.IsAdmin))
}

// DELETE /api/tv-sets/{id}
func (s *Server) handleDeleteTVSet(w http.ResponseWriter, r *http.Request) {
	id, ok := setID(r)
	if !ok {
		writeErr(w, 404, "set not found")
		return
	}
	u := userFrom(r)
	x, err := s.Channels.GetSet(r.Context(), u.ID, id)
	if err != nil {
		s.setError(w, err)
		return
	}
	if !channels.MayEditSet(x, u.ID, u.IsAdmin) {
		s.setError(w, channels.ErrSetNotYours)
		return
	}
	if err := s.Channels.DeleteSet(r.Context(), id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// PUT /api/tv-sets/order: the caller's own order of the custom sets it sees.
func (s *Server) handleOrderTVSets(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(body.IDs) > 500 {
		writeErr(w, 400, "too many sets")
		return
	}
	ids := make([]int64, 0, len(body.IDs))
	for _, v := range body.IDs {
		if id, err := strconv.ParseInt(strings.TrimPrefix(v, "set:"), 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	if err := s.Channels.OrderSets(r.Context(), userFrom(r).ID, ids); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
