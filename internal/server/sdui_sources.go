// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"fmt"
	"hash/fnv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// The data sources a page needs to show a books library's shelf, what a profile liked, and
// statistics, on any device (docs/SDUI.md, "Data sources"), and the module rows on Top
// Shelf. Like the rest (sdui_data.go), every one reads with the account's limits.

// books answers the books source: a books library's books, narrowed and in an order, as
// WittyInk's bookshelf has them.
func (rv *sduiResolver) books(src sdui.Source, req sdui.SourceRequest, lib *library.Library) (sdui.SourceResult, error) {
	if lib == nil || lib.Type != "books" {
		return sdui.SourceResult{}, nil
	}
	q := library.ItemQuery{LibraryID: lib.ID, Kind: "book", Access: rv.acc, Brief: true, Limit: req.Limit + 1, Offset: req.Offset, UserID: rv.u.ID}
	// With filterable on, the page's FilterBar (format, reading state, order) narrows it.
	if src.Bool("filterable") {
		for _, k := range []string{"format", "reading", "sort", "desc"} {
			if v := req.Filter[k]; v != "" {
				src[k] = v
			}
		}
		if req.Filter["desc"] == "1" {
			src["desc"] = true
		}
	}
	if r := src.Str("reading"); r == "reading" || r == "finished" || r == "unread" {
		q.Reading = r
	}
	group := func(name string) *string {
		v := strings.TrimSpace(src.Str(name))
		if v == "" {
			return nil
		}
		if v == sdui.BooksNone {
			v = ""
		}
		return &v
	}
	q.Album, q.AlbumArtist = group("series"), group("author")
	if f := src.Str("format"); f != "" && f != "all" {
		q.Containers = mediatypes.ReaderFormats(f)
		if q.Containers == nil {
			q.Containers = []string{} // a reader there isn't opens nothing
		}
	}
	order := src.Str("sort")
	if order == "" {
		order = "title"
		if q.Album != nil {
			order = "series"
		}
	}
	// The newest first and the latest year first are the natural way round; desc turns
	// any order round.
	q.Sort, q.Desc = order, (order == "added" || order == "year") != src.Bool("desc")
	items, total, err := rv.s.Store.Items(rv.ctx, q)
	if err != nil {
		return sdui.SourceResult{}, err
	}
	_ = rv.s.Store.AttachProgress(rv.ctx, rv.u.ID, items)
	res := sdui.SourceResult{Items: rv.items(items), Total: total}
	if len(res.Items) > req.Limit {
		res.Items, res.More = res.Items[:req.Limit], true
	}
	return res, nil
}

// bookGroups are the series or authors of the page's books library, for a repeat over them.
// Each copy's ID is a hash of the name: names are free text, which two could share once cut
// down to what an ID may hold.
func (rv *sduiResolver) bookGroups(by string) ([]sdui.Each, error) {
	lib, ok := rv.library(rv.vars["library"])
	if !ok || lib.Type != "books" {
		return nil, nil
	}
	groups, err := rv.s.Store.BookGroups(rv.ctx, lib.ID, by, rv.acc)
	if err != nil {
		return nil, err
	}
	out := make([]sdui.Each, 0, len(groups))
	for _, g := range groups {
		key, label := g.Name, g.Name
		if key == "" {
			key, label = sdui.BooksNone, "No series"
			if by == "author" {
				label = "No author"
			}
		}
		h := fnv.New64a()
		h.Write([]byte(key))
		out = append(out, sdui.Each{Key: key, Label: label, ID: fmt.Sprintf("%016x", h.Sum64())})
	}
	return out, nil
}

// mediaKinds are the item kinds a "media" parameter keeps; nil keeps every kind.
func mediaKinds(media string) []string {
	switch media {
	case "watch":
		return []string{"video", "dvd-iso", "dvd-folder", "bluray-iso", "bluray-folder"}
	case "read":
		return []string{"book"}
	case "listen":
		return []string{"audio"}
	}
	return nil
}

// liked answers the liked source: what the profile liked, most recently first.
func (rv *sduiResolver) liked(src sdui.Source, req sdui.SourceRequest, lib *library.Library) (sdui.SourceResult, error) {
	var libID int64
	if lib != nil {
		libID = lib.ID
	}
	items, err := rv.s.Store.LikedItems(rv.ctx, rv.u.ID, libID, mediaKinds(src.Str("media")), req.Offset, req.Limit+1, rv.acc)
	if err != nil {
		return sdui.SourceResult{}, err
	}
	_ = rv.s.Store.AttachProgress(rv.ctx, rv.u.ID, items)
	res := sdui.SourceResult{Items: rv.items(items), Total: -1}
	if len(res.Items) > req.Limit {
		res.Items, res.More = res.Items[:req.Limit], true
	}
	return res, nil
}

// statsPeriods are how many days each period of the statistics source covers.
var statsPeriods = map[string]int{"week": 7, "month": 30, "year": 365}

// statistics answers the statistics source from the play log. A profile is shown its own
// record; everyone's only to an administrator, and nothing to anyone else. What it names
// and counts keeps to what the profile may open.
func (rv *sduiResolver) statistics(src sdui.Source, req sdui.SourceRequest, lib *library.Library) (sdui.SourceResult, error) {
	s := rv.s
	q := library.WatchQuery{UserID: rv.u.ID, By: "title", Limit: req.Offset + req.Limit + 1}
	everyone := src.Str("who") == "everyone"
	if everyone {
		if !rv.u.IsAdmin {
			return sdui.SourceResult{}, nil
		}
		q.UserID = 0
	}
	period := src.Str("period")
	if period == "" {
		period = "month"
	}
	if days := statsPeriods[period]; days > 0 {
		q.FromDay = library.DayNumber(rv.now.AddDate(0, 0, 1-days))
	}
	if lib != nil {
		q.LibraryID = lib.ID
	}
	switch src.Str("stat") {
	case "shows":
		q.By = "show"
	case "libraries":
		q.By = "library"
	}
	totals, err := s.Store.MostWatched(rv.ctx, q, rv.acc)
	if err != nil {
		return sdui.SourceResult{}, err
	}
	note := func(t library.WatchTotal) string {
		out := timeWatched(t.Seconds)
		if everyone && t.People > 1 {
			out += " · " + strconv.Itoa(t.People) + " people"
		}
		return out
	}
	var all []sdui.Item
	switch q.By {
	case "library":
		names := map[int64]library.Library{}
		for _, l := range rv.libraries() {
			names[l.ID] = l
		}
		for _, t := range totals {
			l, ok := names[t.ID]
			if !ok {
				continue // a library the menus don't show, or one gone since
			}
			all = append(all, sdui.Item{ID: "lib" + itoa(l.ID), Kind: "library", Title: l.Name, Subtitle: note(t), Count: t.Titles, RuntimeSec: t.Seconds,
				Icon: sdui.LibraryIcon(l.Type), Route: "/library/" + itoa(l.ID)})
		}
	case "show":
		ids := make([]int64, len(totals))
		for i, t := range totals {
			ids[i] = t.ID
		}
		shows, err := s.Store.Shows(rv.ctx, library.ShowQuery{IDs: ids, UserID: rv.u.ID, Access: rv.acc, Limit: len(ids)})
		if err != nil {
			return sdui.SourceResult{}, err
		}
		byID := map[int64]*library.Show{}
		for _, sh := range shows {
			byID[sh.ID] = sh
		}
		for _, t := range totals {
			if sh := byID[t.ID]; sh != nil {
				it := rv.show(sh)
				it.Subtitle = note(t)
				all = append(all, it)
			}
		}
	default:
		ids := make([]int64, len(totals))
		for i, t := range totals {
			ids[i] = t.ID
		}
		items, err := s.Store.ItemsInOrder(rv.ctx, ids, rv.acc)
		if err != nil {
			return sdui.SourceResult{}, err
		}
		_ = s.Store.AttachProgress(rv.ctx, rv.u.ID, items)
		secs := map[int64]library.WatchTotal{}
		for _, t := range totals {
			secs[t.ID] = t
		}
		for _, it := range items {
			c := rv.item(it)
			c.Subtitle = note(secs[it.ID])
			if c.Kind == "episode" && c.ShowTitle != "" {
				c.Subtitle = c.ShowTitle + " · " + c.Subtitle
			}
			all = append(all, c)
		}
	}
	return page(all, req), nil
}

// timeWatched says how long, the way a card has room for: 45 min, 3 h 20 min.
func timeWatched(sec float64) string {
	d := time.Duration(sec) * time.Second
	h, m := int(d/time.Hour), int(d%time.Hour/time.Minute)
	switch {
	case h == 0 && m == 0:
		return "Under a minute"
	case h == 0:
		return strconv.Itoa(m) + " min"
	case m == 0:
		return strconv.Itoa(h) + " h"
	}
	return strconv.Itoa(h) + " h " + strconv.Itoa(m) + " min"
}

// maxTopShelfModuleRows is how many of Top Shelf's rows modules may have, so what the
// profile was watching and what's new keep their place.
const maxTopShelfModuleRows = 2

// topShelfModuleRows are the rows of the sources the profile's modules offer for Top Shelf.
func (s *Server) topShelfModuleRows(r *http.Request, rv *sduiResolver) []sdui.TopShelfRow {
	return topShelfRowsFrom(s.modulesFor(r).TopShelfSources(), func(src sdui.Source) (sdui.SourceResult, error) {
		return rv.Items(sdui.SourceRequest{Source: src, Limit: sdui.MaxTopShelfItems})
	})
}

// topShelfRowsFrom asks each offered source for a row, in order, until there are
// maxTopShelfModuleRows: a source that fails or has nothing gives none. A row is titled with
// the source's title, or its label when the title needs a page to fill it in.
func topShelfRowsFrom(sources []sdui.SourceSpec, ask func(sdui.Source) (sdui.SourceResult, error)) []sdui.TopShelfRow {
	var rows []sdui.TopShelfRow
	for _, src := range sources {
		if len(rows) >= maxTopShelfModuleRows {
			break
		}
		res, err := ask(sdui.Source{"type": src.Type})
		if err != nil || len(res.Items) == 0 {
			continue
		}
		title := src.Title
		if title == "" || strings.Contains(title, "{") {
			title = src.Label
		}
		rows = append(rows, sdui.TopShelfRow{Title: title, Items: res.Items})
	}
	return rows
}
