// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/search"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// searchFound is one search's result for a page, worked out once however many of
// the page's sections show a part of it.
type searchFound struct {
	key   string
	res   search.Result
	chans map[string]sdui.Item // the channels searched, by key
}

// searchText is the page's search, with the year its filter bar holds.
func searchText(query, year string) string {
	query = strings.TrimSpace(query)
	if len(query) > 100 {
		query = query[:100]
	}
	if year = strings.TrimSpace(year); year != "" && !strings.ContainsAny(year, " \t") {
		query += " year:" + year
	}
	return query
}

// search runs the page's search on the index, or says it can't yet (the index is
// still being built when the server has just started).
func (rv *sduiResolver) search(text string) (*searchFound, bool) {
	idx := rv.s.searchIndex()
	if idx == nil {
		return nil, false
	}
	if rv.found != nil && rv.found.key == text {
		return rv.found, true
	}
	allowed := map[int64]bool{0: true}
	for _, l := range rv.libraries() {
		allowed[l.ID] = true
	}
	f := &searchFound{key: text, chans: map[string]sdui.Item{}}
	q := search.Query{Text: text, PerGroup: 40, Library: func(lib int64) bool { return allowed[lib] }}
	for _, c := range rv.channels(false, "", 5000, false) {
		f.chans[c.ID] = c
		q.Extra = append(q.Extra, search.Doc{Group: "channels", Key: c.ID, Title: c.Title})
	}
	f.res = idx.Search(q)
	rv.found = f
	if !rv.noted && rv.u != nil && strings.TrimSpace(search.Parse(&search.Query{Text: text})) != "" {
		rv.noted = true
		rv.s.noteSearch(rv.ctx, rv.u.ID, rv.vars["query"])
	}
	return f, true
}

// searchGroups are the groups the page's search found something in, best first:
// what the search page's shelves are repeated over. kinds keeps only those groups.
func (rv *sduiResolver) searchGroups(kinds []string) []sdui.Each {
	f, ok := rv.search(searchText(rv.vars["query"], rv.vars["year"]))
	if !ok {
		return nil
	}
	var out []sdui.Each
	for _, g := range f.res.Groups {
		if len(kinds) > 0 && !slices.Contains(kinds, g.Key) {
			continue
		}
		if len(rv.searchItems(f, g.Key, 1)) == 0 { // everything it found is hidden from this profile
			continue
		}
		out = append(out, sdui.Each{Key: g.Key, Label: g.Label, Kind: g.Key})
	}
	return out
}

// searchGroup answers a search source naming a group.
func (rv *sduiResolver) searchGroup(text, group string, req sdui.SourceRequest) (sdui.SourceResult, bool) {
	f, ok := rv.search(text)
	if !ok {
		return sdui.SourceResult{}, false
	}
	items := rv.searchItems(f, group, req.Offset+req.Limit+1)
	res := page(items, req)
	if g := f.res.Group(group); g != nil && g.More {
		res.Total = -1
	}
	return res, true
}

// searchItems are a group's hits as cards, keeping only what the profile may see:
// the index knows the libraries the profile can open, and the store checks the rest
// (age limits, blocked titles) as it reads the cards.
func (rv *sduiResolver) searchItems(f *searchFound, group string, limit int) []sdui.Item {
	g := f.res.Group(group)
	if g == nil || limit <= 0 {
		return nil
	}
	if rv.searchCards == nil {
		rv.searchCards = map[string][]sdui.Item{}
	}
	if got, ok := rv.searchCards[f.key+"\n"+group]; ok {
		return got
	}
	hits := g.Hits
	ids := make([]int64, 0, len(hits))
	for _, h := range hits {
		ids = append(ids, h.ID)
	}
	var out []sdui.Item
	switch group {
	case "channels":
		for _, h := range hits {
			if c, ok := f.chans[h.Key]; ok {
				out = append(out, c)
			}
		}
	case "people":
		found, err := rv.s.Store.VisiblePeople(rv.ctx, ids, rv.acc)
		if err != nil {
			return nil
		}
		for _, p := range found {
			out = append(out, rv.personCard(p))
		}
	case "shows":
		shows, err := rv.s.Store.Shows(rv.ctx, library.ShowQuery{IDs: ids, Access: rv.acc, UserID: rv.u.ID, Limit: len(ids)})
		if err != nil {
			return nil
		}
		byID := map[int64]*library.Show{}
		for _, sh := range shows {
			byID[sh.ID] = sh
		}
		for _, h := range hits {
			if sh := byID[h.ID]; sh != nil {
				out = append(out, rv.show(sh))
			}
		}
	default:
		items, _, err := rv.s.Store.Items(rv.ctx, library.ItemQuery{IDs: ids, Access: rv.acc, Limit: len(ids), NoTotal: true, Brief: true})
		if err != nil {
			return nil
		}
		byID := map[int64]*library.Item{}
		for _, it := range items {
			byID[it.ID] = it
		}
		switch group {
		case "albums", "audiobooks", "podcasts", "artists":
			// The cover's track stands for the album or the artist: one the profile may not
			// hear hides them.
			for i, h := range hits {
				if byID[h.ID] == nil {
					continue
				}
				if group == "artists" {
					out = append(out, rv.artist(h.Lib, i, library.Artist{Name: h.Title, Albums: h.Count, CoverItem: h.ID}))
					continue
				}
				a := library.Album{Artist: h.Key, Title: h.Title, Tracks: h.Count, CoverItem: h.ID, FirstItem: h.ID}
				if h.Year > 0 {
					y := h.Year
					a.Year = &y
				}
				out = append(out, rv.album(h.Lib, a))
			}
		default:
			kept := make([]*library.Item, 0, len(items))
			for _, h := range hits {
				if it := byID[h.ID]; it != nil {
					kept = append(kept, it)
				}
			}
			_ = rv.s.Store.AttachProgress(rv.ctx, rv.u.ID, kept)
			out = rv.items(kept)
		}
	}
	rv.searchCards[f.key+"\n"+group] = out
	if len(out) > limit {
		return out[:limit]
	}
	return out
}

// personCard is a person a search found.
func (rv *sduiResolver) personCard(p library.FoundPerson) sdui.Item {
	id := itoa(p.ID)
	it := sdui.Item{ID: "person-" + id, Kind: "person", Title: p.Name, Route: "/person/" + id, Images: map[string]string{}}
	noun := " titles"
	if p.Titles == 1 {
		noun = " title"
	}
	it.Subtitle = strings.TrimPrefix(knownForLabel(p.KnownFor)+" · "+strconv.Itoa(p.Titles)+noun, " · ")
	if p.HasPhoto {
		it.Images["square"] = "/api/people/" + id + "/photo"
	}
	return it
}

// recentSearchItems are the profile's recent searches, each opening its search again.
func (rv *sduiResolver) recentSearchItems() []sdui.Item {
	var out []sdui.Item
	for i, q := range rv.s.recentSearches(rv.ctx, rv.u.ID) {
		out = append(out, sdui.Item{ID: "recent-" + strconv.Itoa(i), Kind: "view", Title: q, Icon: "history", Route: "/search?q=" + url.QueryEscape(q)})
	}
	return out
}

// GET /api/search?q=...&groups=films,shows&limit=12: the one search, grouped, as the
// search page's cards. remember=1 keeps it among the profile's recent searches (a
// client sends it when someone picks a result or presses Enter, not as they type).
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	text := strings.TrimSpace(q.Get("q"))
	rv := s.newSDUIResolver(r, map[string]string{"query": text})
	rv.noted = q.Get("remember") != "1" // noted already: only a search asked to be remembered is kept
	limit := min(max(qInt(r, "limit", 12), 1), 40)
	var groups []string
	if g := q.Get("groups"); g != "" {
		groups = strings.Split(g, ",")
	}
	type group struct {
		Key   string      `json:"key"`
		Label string      `json:"label"`
		More  bool        `json:"more"`
		Items []sdui.Item `json:"items"`
	}
	out := struct {
		Query  string  `json:"query"`
		TookMs float64 `json:"tookMs"`
		Groups []group `json:"groups"`
	}{Query: text, Groups: []group{}}
	f, ok := rv.search(searchText(text, q.Get("year")))
	if !ok {
		writeErr(w, http.StatusServiceUnavailable, "The search is getting ready. Try again in a moment.")
		return
	}
	out.TookMs = float64(f.res.Took.Microseconds()) / 1000
	for _, g := range f.res.Groups {
		if len(groups) > 0 && !slices.Contains(groups, g.Key) {
			continue
		}
		items := rv.searchItems(f, g.Key, limit+1)
		if len(items) == 0 {
			continue
		}
		more := g.More || len(items) > limit
		if len(items) > limit {
			items = items[:limit]
		}
		out.Groups = append(out.Groups, group{Key: g.Key, Label: g.Label, More: more, Items: items})
	}
	writeJSON(w, http.StatusOK, out)
}
