// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// What the server puts in Data for each stock screen, so their bindings have
// something to bind to. Everything here is read the way the ordinary routes
// read it: with the account's limits in the query (library.Access), or through
// visibleItem / visibleShow / Channels.Visible. A title an account may not see
// never reaches a screen sent to it. docs/SDUI.md, "What the server puts in
// Data", lists the paths.
//
// Only as much is read as the device will be sent: the profile's MaxItems for a
// row, MaxGridItems for a grid, MaxSections rows.

// errNoScreenData: what the request names isn't there, or isn't this account's to see.
var errNoScreenData = errors.New("not found")

// sduiItem is one item as a screen is given it.
func sduiItem(it *library.Item) map[string]any {
	id := itoa(it.ID)
	m := map[string]any{"id": id, "title": it.Title, "kind": it.Kind, "route": "/item/" + id, "playRoute": "/watch/" + id}
	sub := ""
	switch {
	case it.ShowID != nil && it.Season != nil && it.Episode != nil:
		sub = fmt.Sprintf("S%d E%d", *it.Season, *it.Episode)
		if it.ShowTitle != "" {
			sub = it.ShowTitle + " · " + sub
		}
	case it.Year != nil && *it.Year > 0:
		sub = fmt.Sprint(*it.Year)
	}
	if sub != "" {
		m["subtitle"] = sub
	}
	if it.Poster != "" {
		m["imageUrl"] = "/api/items/" + id + "/poster"
	}
	if p := it.Progress; p != nil && !p.Watched && p.DurationSec > 0 && p.PositionSec > 0 {
		m["progress"] = min(1, p.PositionSec/p.DurationSec)
	}
	if it.Kind == "audio" {
		m["shape"] = "square"
	}
	return m
}

func sduiShow(sh *library.Show) map[string]any {
	id := itoa(sh.ID)
	m := map[string]any{"id": "show-" + id, "title": sh.Title, "kind": "show", "route": "/show/" + id}
	if sh.Year != nil && *sh.Year > 0 {
		m["subtitle"] = fmt.Sprint(*sh.Year)
	}
	if sh.Poster != "" {
		m["imageUrl"] = "/api/shows/" + id + "/art/poster"
	}
	return m
}

func sduiItems(items []*library.Item, limit int) []any {
	out := make([]any, 0, min(len(items), limit))
	for _, it := range items {
		if len(out) >= limit {
			break
		}
		out = append(out, sduiItem(it))
	}
	return out
}

// sduiLibraryRows reads up to limit of a library's newest titles: shows for a TV library,
// albums for music, items for the rest.
func (s *Server) sduiLibraryRows(r *http.Request, l library.Library, sort string, desc bool, limit, offset int, search string) []any {
	ctx, u, acc := r.Context(), userFrom(r), s.access(r)
	switch l.Type {
	case "shows", "anime":
		sq := library.ShowQuery{LibraryID: l.ID, Sort: sort, Desc: desc, Limit: limit, UserID: u.ID, Access: acc, Search: search}
		s.animeQuery(ctx, &sq)
		shows, err := s.Store.Shows(ctx, sq)
		if err != nil {
			return nil
		}
		out := make([]any, 0, len(shows))
		for _, sh := range shows {
			if len(out) >= limit {
				break
			}
			out = append(out, sduiShow(sh))
		}
		return out
	case "music":
		albumSort := "added"
		if sort == "title" {
			albumSort = "title"
		}
		albums, err := s.Store.Albums(ctx, l.ID, "", search, albumSort, acc)
		if err != nil {
			return nil
		}
		out := []any{}
		for i, a := range albums {
			if i < offset {
				continue
			}
			if len(out) >= limit {
				break
			}
			m := map[string]any{"id": fmt.Sprintf("album-%d-%d", l.ID, a.CoverItem), "title": a.Title, "subtitle": a.Artist, "kind": "album", "shape": "square",
				"route": "/album/" + itoa(l.ID) + "?artist=" + url.QueryEscape(a.Artist) + "&album=" + url.QueryEscape(a.Title)}
			if a.CoverItem != 0 {
				m["imageUrl"] = "/api/items/" + itoa(a.CoverItem) + "/poster"
			}
			out = append(out, m)
		}
		return out
	}
	items, _, err := s.Store.Items(ctx, library.ItemQuery{LibraryID: l.ID, Sort: sort, Desc: desc, Limit: limit, Offset: offset, Access: acc, NoTotal: true, Brief: true, Search: search})
	if err != nil {
		return nil
	}
	_ = s.Store.AttachProgress(ctx, u.ID, items)
	return sduiItems(items, limit)
}

// sduiChannels are the channels this account may watch.
func (s *Server) sduiChannels(r *http.Request, limit int, search string) (all []any, favourites []any) {
	if s.Channels == nil {
		return nil, nil
	}
	ctx, u, acc := r.Context(), userFrom(r), s.access(r)
	list, err := s.Channels.List(ctx)
	if err != nil {
		return nil, nil
	}
	fav, _ := s.Channels.Favourites(ctx, u.ID)
	search = strings.ToLower(search)
	for _, c := range list {
		if !c.Enabled || !s.Channels.Visible(ctx, c, u, acc) || (search != "" && !strings.Contains(strings.ToLower(c.Name), search)) {
			continue
		}
		key := "lib-" + itoa(c.ID)
		m := map[string]any{"id": key, "title": c.Name, "kind": "channel", "shape": "wide", "route": "/tv/watch/" + key, "playRoute": "/tv/watch/" + key, "module": "wittytv"}
		if c.Number != "" {
			m["subtitle"] = c.Number
		}
		if c.Logo != "" {
			m["imageUrl"] = "/api/channels/" + itoa(c.ID) + "/logo"
		}
		if fav[key] && len(favourites) < limit {
			favourites = append(favourites, m)
		}
		if len(all) < limit {
			all = append(all, m)
		}
	}
	return all, favourites
}

// sduiData reads what a stock screen shows, for this account and device. flags are set
// for the screen's conditions (libraryEmpty, hasProgress, ...). A screen an administrator
// made under another name gets the account's and the server's names only.
func (s *Server) sduiData(r *http.Request, screen string, l *sdui.Layout, data map[string]any, flags map[string]bool) error {
	ctx, u, acc := r.Context(), userFrom(r), s.access(r)
	q := r.URL.Query()
	libs := func() []library.Library {
		all, _ := s.visibleLibraries(ctx, acc)
		out := all[:0]
		for _, lib := range all {
			if !lib.Hidden() {
				out = append(out, lib)
			}
		}
		return out
	}
	switch screen {
	case "home":
		home := map[string]any{}
		cont, _ := s.Store.ContinueWatching(ctx, u.ID, l.MaxItems)
		cont = s.filterItems(ctx, acc, cont)
		_ = s.Store.AttachProgress(ctx, u.ID, cont)
		home["continueWatching"] = sduiItems(cont, l.MaxItems)
		visible := libs()
		home["nextUp"] = sduiItems(s.nextUp(r, visible), l.MaxItems)
		if feat, _, err := s.Store.Items(ctx, library.ItemQuery{Sort: "added", Desc: true, Limit: 5, Access: acc, NoTotal: true, Brief: true, Watchable: true, NoEpisodes: true, NoGallery: true}); err == nil {
			list := []any{}
			for _, it := range feat {
				m := sduiItem(it)
				if it.Backdrop != "" || it.Poster != "" {
					m["imageUrl"] = "/api/items/" + itoa(it.ID) + "/backdrop" // the poster, when there's no backdrop
				}
				m["shape"] = "hero"
				list = append(list, m)
			}
			home["featured"] = list
		}
		sections, music := []any{}, []any{}
		for _, lib := range visible {
			if len(sections) >= l.MaxSections {
				break
			}
			rows := s.sduiLibraryRows(r, lib, "added", true, l.MaxItems, 0, "")
			if len(rows) == 0 {
				continue
			}
			if lib.Type == "music" {
				if len(music) < l.MaxItems {
					music = append(music, rows[:min(len(rows), l.MaxItems-len(music))]...)
				}
				continue
			}
			sections = append(sections, map[string]any{"id": "lib" + itoa(lib.ID), "title": "Recently added in " + lib.Name, "seeAll": "/library/" + itoa(lib.ID), "items": rows})
		}
		home["sections"], home["music"] = sections, music
		home["channels"], _ = s.sduiChannels(r, l.MaxItems, "")
		data["home"] = home
	case "libraries":
		items := []any{}
		for _, lib := range libs() {
			items = append(items, map[string]any{"id": "lib" + itoa(lib.ID), "title": lib.Name, "subtitle": lib.Type, "kind": "library", "shape": "wide", "route": "/library/" + itoa(lib.ID)})
		}
		flags["librariesEmpty"] = len(items) == 0
		data["libraries"] = map[string]any{"items": items}
	case "library":
		id := int64(qInt(r, "library", 0))
		lib, err := s.Store.Library(ctx, id)
		if id <= 0 || err != nil || !acc.AllowsLibrary(id) {
			return errNoScreenData
		}
		sort, desc := "title", false
		switch q.Get("sort") {
		case "added", "newest":
			sort, desc = "added", true
		case "year":
			sort, desc = "year", true
		}
		rows := s.sduiLibraryRows(r, *lib, sort, desc, l.MaxGridItems, max(0, qInt(r, "offset", 0)), "")
		flags["libraryEmpty"] = len(rows) == 0
		data["library"] = map[string]any{"id": itoa(lib.ID), "title": lib.Name, "kind": lib.Type, "items": rows, "sort": sort}
	case "detail":
		item := map[string]any{}
		switch {
		case q.Get("show") != "":
			sh, err := s.visibleShow(r, int64(qInt(r, "show", 0)))
			if err != nil {
				return errNoScreenData
			}
			item = sduiShow(sh)
			item["overview"] = sh.Overview
			item["rating"] = sh.ContentRating
			if sh.Year != nil {
				item["year"] = fmt.Sprint(*sh.Year)
			}
			if sh.Backdrop != "" {
				item["backdrop"] = "/api/shows/" + itoa(sh.ID) + "/art/backdrop"
			}
			eps, _, _ := s.Store.Items(ctx, library.ItemQuery{ShowID: sh.ID, Access: acc, Limit: l.MaxGridItems, NoTotal: true, Brief: true})
			_ = s.Store.AttachProgress(ctx, u.ID, eps)
			item["children"] = sduiItems(eps, l.MaxGridItems)
			flags["notPlayable"] = true // a show is opened; its episodes are played
		default:
			it, err := s.visibleItem(r, int64(qInt(r, "item", 0)))
			if err != nil {
				return errNoScreenData
			}
			_ = s.Store.AttachProgress(ctx, u.ID, []*library.Item{it})
			item = sduiItem(it)
			item["playId"] = itoa(it.ID)
			item["overview"] = it.Overview
			item["rating"] = it.ContentRating
			item["duration"] = it.RuntimeSec
			if it.Year != nil {
				item["year"] = fmt.Sprint(*it.Year)
			}
			if it.Backdrop != "" {
				item["backdrop"] = "/api/items/" + itoa(it.ID) + "/backdrop"
			}
			_, flags["hasProgress"] = item["progress"]
		}
		data["item"] = item
	case "search":
		term := strings.TrimSpace(q.Get("q"))
		if len(term) > 100 {
			term = term[:100]
		}
		sections := []any{}
		if len([]rune(term)) >= 2 {
			for _, lib := range libs() {
				if len(sections) >= l.MaxSections {
					break
				}
				if rows := s.sduiLibraryRows(r, lib, "title", false, l.MaxItems, 0, term); len(rows) > 0 {
					sections = append(sections, map[string]any{"id": "lib" + itoa(lib.ID), "title": lib.Name, "items": rows})
				}
			}
			if ch, _ := s.sduiChannels(r, l.MaxItems, term); len(ch) > 0 {
				sections = append(sections, map[string]any{"id": "channels", "title": "Channels", "cardShape": "wide", "items": ch, "module": "wittytv"})
			}
			flags["searchEmpty"] = len(sections) == 0
		} else {
			flags["searchIdle"] = true
		}
		data["search"] = map[string]any{"query": term, "sections": sections}
	case "livetv":
		all, fav := s.sduiChannels(r, l.MaxGridItems, "")
		flags["livetvEmpty"] = len(all) == 0
		data["livetv"] = map[string]any{"channels": all, "favourites": fav}
	case "music":
		sections := []any{}
		for _, lib := range libs() {
			if lib.Type != "music" || len(sections) >= l.MaxSections {
				continue
			}
			if rows := s.sduiLibraryRows(r, lib, "added", true, l.MaxItems, 0, ""); len(rows) > 0 {
				sections = append(sections, map[string]any{"id": "lib" + itoa(lib.ID), "title": "Recently added in " + lib.Name, "cardShape": "square", "seeAll": "/library/" + itoa(lib.ID), "items": rows, "module": "wittymusic"})
			}
		}
		cont, _ := s.Store.ContinueWatching(ctx, u.ID, 4*l.MaxItems)
		var listening []*library.Item
		for _, it := range s.filterItems(ctx, acc, cont) {
			if it.Kind == "audio" {
				listening = append(listening, it)
			}
		}
		_ = s.Store.AttachProgress(ctx, u.ID, listening)
		flags["musicEmpty"] = len(sections) == 0
		data["music"] = map[string]any{"sections": sections, "continueListening": sduiItems(listening, l.MaxItems)}
	}
	return nil
}

// sduiTopShelfRows are the rows a tvOS Top Shelf shows: what this account was in the
// middle of, then what's new in the first libraries it may open.
func (s *Server) sduiTopShelfRows(r *http.Request) []map[string]any {
	ctx, u, acc := r.Context(), userFrom(r), s.access(r)
	var rows []map[string]any
	cont, _ := s.Store.ContinueWatching(ctx, u.ID, sdui.MaxTopShelfItems)
	cont = s.filterItems(ctx, acc, cont)
	_ = s.Store.AttachProgress(ctx, u.ID, cont)
	if len(cont) > 0 {
		rows = append(rows, map[string]any{"title": "Continue watching", "items": sduiItems(cont, sdui.MaxTopShelfItems)})
	}
	libs, _ := s.visibleLibraries(ctx, acc)
	for _, lib := range libs {
		if len(rows) >= sdui.MaxTopShelfSections {
			break
		}
		if lib.Hidden() {
			continue
		}
		if items := s.sduiLibraryRows(r, lib, "added", true, sdui.MaxTopShelfItems, 0, ""); len(items) > 0 {
			rows = append(rows, map[string]any{"title": "New in " + lib.Name, "items": items})
		}
	}
	return rows
}
