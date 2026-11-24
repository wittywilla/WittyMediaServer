// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Library health: what in the libraries needs a look, in one place, each with the fix that
// exists for it. Everything but the NFO files is worked out from what's stored, so the page
// costs a few queries and reads no media. NFO files are read by a check someone starts, as
// that reads beside every file (see CheckSidecars).
//
// It ties the tools that were apart together: Fix matches for titles with no match, a scan
// for files that have gone, the item editor for names, Repair for what's wrong with the
// server itself. Nothing here renames, moves or deletes a media file.

// The sections, in the order they're shown.
const (
	HealthMissing    = "missing"
	HealthUnmatched  = "unmatched"
	HealthDuplicates = "duplicates"
	HealthNames      = "names"
	HealthNFO        = "nfo"
	HealthArtwork    = "artwork"
	HealthUnreadable = "unreadable"
)

// HealthSections lists them in order.
var HealthSections = []string{HealthMissing, HealthUnmatched, HealthDuplicates, HealthNames, HealthNFO, HealthArtwork, HealthUnreadable}

// The fixes a row can offer. The page draws a button for each.
const (
	FixForget   = "forget"   // the file is gone for good: forget the title (and its history)
	FixScan     = "scan"     // scan the library again
	FixMatch    = "match"    // choose the match, in Fix matches
	FixLeave    = "leave"    // leave it with no match
	FixVersions = "versions" // keep them as versions of one film
	FixOpen     = "open"     // open the title, to edit it
	FixRules    = "rules"    // naming rules, in Fix matches
	FixRefresh  = "refresh"  // fetch its details and artwork again
	FixProbe    = "probe"    // read the file's details again
	FixSetAside = "setaside" // it's fine as it is: stop showing it
)

// HealthMember is one of the titles in a group of duplicates.
type HealthMember struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Path   string `json:"path"`
	Height int    `json:"height,omitempty"`
	Size   int64  `json:"size,omitempty"`
}

// HealthRow is one thing to look at.
type HealthRow struct {
	Key       string         `json:"key"` // stable, to set it aside
	Target    string         `json:"target"`
	ID        int64          `json:"id"`
	LibraryID int64          `json:"libraryId"`
	Title     string         `json:"title"`
	Year      *int           `json:"year,omitempty"`
	Path      string         `json:"path,omitempty"`
	Detail    string         `json:"detail,omitempty"`
	Members   []HealthMember `json:"members,omitempty"`
	Fixes     []string       `json:"fixes"`
}

// HealthSection is one kind of problem: how many, and the first few.
type HealthSection struct {
	ID    string      `json:"id"`
	Count int         `json:"count"`
	Rows  []HealthRow `json:"rows"`
	// Checked: when the files were last read, for a section that reads them (NFO files).
	Checked int64 `json:"checked,omitempty"`
}

// HealthReport is the page.
type HealthReport struct {
	Sections []HealthSection `json:"sections"`
	SetAside int             `json:"setAside"`
	Scanning []int64         `json:"scanning"` // libraries being scanned, whose numbers are moving
}

// HealthOptions says what to look at.
type HealthOptions struct {
	Library  int64           // 0: every library
	Limit    int             // rows a section, at most
	Only     string          // one section, or every one
	SetAside map[string]bool // keys someone said are fine
	// Sidecars is the last NFO check's rows, kept by the server.
	Sidecars *SidecarCheck
}

// SidecarCheck is what the last NFO check found.
type SidecarCheck struct {
	At      time.Time
	Rows    []HealthRow
	Stopped bool // it ran out of time or files before the end
}

const healthMaxRows = 500

func (o HealthOptions) limit() int {
	if o.Limit <= 0 {
		return 25
	}
	if o.Limit > healthMaxRows {
		return healthMaxRows
	}
	return o.Limit
}

// Health works out the report. The scanner is for the match queue, which is the same list
// Fix matches shows.
func (s *Scanner) Health(ctx context.Context, o HealthOptions) (*HealthReport, error) {
	libs, err := s.Store.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	types := map[int64]string{}
	rep := &HealthReport{Scanning: []int64{}}
	for _, l := range libs {
		if o.Library == 0 || o.Library == l.ID {
			types[l.ID] = l.Type
		}
		if l.Scanning || s.Running(l.ID) {
			rep.Scanning = append(rep.Scanning, l.ID)
		}
	}
	if o.Library != 0 && types[o.Library] == "" {
		return nil, ErrNotFound
	}
	h := &healthRun{s: s, ctx: ctx, o: o, types: types, scanning: map[int64]bool{}}
	for _, id := range rep.Scanning {
		h.scanning[id] = true
	}
	for _, id := range HealthSections {
		if o.Only != "" && o.Only != id {
			continue
		}
		var rows []HealthRow
		var err error
		sec := HealthSection{ID: id}
		switch id {
		case HealthMissing:
			rows, err = h.missing()
		case HealthUnmatched:
			rows, err = h.unmatched()
		case HealthDuplicates:
			rows, err = h.duplicates()
		case HealthNames:
			rows, err = h.names()
		case HealthNFO:
			if o.Sidecars != nil {
				sec.Checked = o.Sidecars.At.Unix()
				for _, r := range o.Sidecars.Rows {
					if _, ok := types[r.LibraryID]; ok {
						rows = append(rows, r)
					}
				}
			}
		case HealthArtwork:
			rows, err = h.artwork()
		case HealthUnreadable:
			rows, err = h.unreadable()
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		for _, r := range rows {
			if o.SetAside[r.Key] {
				rep.SetAside++
				continue
			}
			sec.Count++
			if len(sec.Rows) < o.limit() {
				sec.Rows = append(sec.Rows, r)
			}
		}
		sec.Count += h.uncounted
		h.uncounted = 0
		if sec.Rows == nil {
			sec.Rows = []HealthRow{}
		}
		rep.Sections = append(rep.Sections, sec)
	}
	return rep, nil
}

type healthRun struct {
	s        *Scanner
	ctx      context.Context
	o        HealthOptions
	types    map[int64]string
	scanning map[int64]bool
	// uncounted: more rows than the section listed, counted only.
	uncounted int
}

// in is the SQL for "one of the libraries looked at", with its arguments.
func (h *healthRun) in(col string) (string, []any) {
	if h.o.Library != 0 {
		return col + " = ?", []any{h.o.Library}
	}
	return "1", nil
}

func rowKey(section, target string, id int64) string {
	return section + ":" + target + ":" + strconv.FormatInt(id, 10)
}

// cap is how many rows a query reads: enough to count every one on a big library, and no
// more, so a library that lost a drive's worth of files doesn't fill memory.
const healthReadCap = 20000

// missing: files that have gone, which scans keep (with everything known about them) in
// case they come back. Titles someone removed on purpose aren't here: those are in Fix
// matches, Removed.
func (h *healthRun) missing() ([]HealthRow, error) {
	where, args := h.in("i.library_id")
	rows, err := h.s.Store.DB.QueryContext(h.ctx, `SELECT i.id, i.library_id, i.title, i.path, i.year, IFNULL(sh.title, ''), i.season, i.episode FROM items i
		LEFT JOIN shows sh ON sh.id = i.show_id
		WHERE i.missing = 1 AND `+where+`
		  AND NOT EXISTS (SELECT 1 FROM library_moves m WHERE m.path = i.path AND m.to_library = 0)
		  AND (i.show_id IS NULL OR sh.missing = 0)
		ORDER BY i.library_id, i.sort_title COLLATE NOCASE LIMIT `+strconv.Itoa(healthReadCap), args...)
	if err != nil {
		return nil, err
	}
	var out []HealthRow
	for rows.Next() {
		var r HealthRow
		var show string
		var season, episode *int
		if err := rows.Scan(&r.ID, &r.LibraryID, &r.Title, &r.Path, &r.Year, &show, &season, &episode); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := h.types[r.LibraryID]; !ok {
			continue
		}
		r.Target, r.Key = TargetItem, rowKey(HealthMissing, TargetItem, r.ID)
		if show != "" {
			r.Detail = show + episodeWords(season, episode)
		}
		r.Fixes = []string{FixScan, FixForget, FixSetAside}
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	where, args = h.in("library_id")
	srows, err := h.s.Store.DB.QueryContext(h.ctx, `SELECT id, library_id, title, path, year FROM shows s WHERE missing = 1 AND `+where+`
		AND NOT EXISTS (SELECT 1 FROM library_moves m WHERE m.path = s.path AND m.to_library = 0) LIMIT `+strconv.Itoa(healthReadCap), args...)
	if err != nil {
		return nil, err
	}
	defer srows.Close()
	for srows.Next() {
		var r HealthRow
		if err := srows.Scan(&r.ID, &r.LibraryID, &r.Title, &r.Path, &r.Year); err != nil {
			return nil, err
		}
		if _, ok := h.types[r.LibraryID]; !ok {
			continue
		}
		r.Target, r.Key, r.Detail = TargetShow, rowKey(HealthMissing, TargetShow, r.ID), "A whole show's folder"
		r.Fixes = []string{FixScan, FixForget, FixSetAside}
		out = append(out, r)
	}
	return out, srows.Err()
}

func episodeWords(season, episode *int) string {
	switch {
	case season != nil && episode != nil:
		return fmt.Sprintf(", season %d episode %d", *season, *episode)
	case season != nil:
		return fmt.Sprintf(", season %d", *season)
	}
	return ""
}

// unmatched: the same list as Fix matches, No match.
func (h *healthRun) unmatched() ([]HealthRow, error) {
	q, err := h.s.MatchQueue(h.ctx, h.o.Library, "unmatched", "", healthMaxRows, 0)
	if err != nil {
		return nil, err
	}
	out := make([]HealthRow, 0, len(q.Rows))
	for _, r := range q.Rows {
		if h.scanning[r.LibraryID] {
			continue // the scan is still matching them
		}
		row := HealthRow{Key: rowKey(HealthUnmatched, r.Target, r.ID), Target: r.Target, ID: r.ID, LibraryID: r.LibraryID, Title: r.Title, Year: r.Year, Path: r.File,
			Fixes: []string{FixMatch, FixLeave}}
		if len(r.Top) > 0 {
			row.Detail = fmt.Sprintf("%d suggestions to choose from", len(r.Top))
			if len(r.Top) == 1 {
				row.Detail = "1 suggestion to choose from"
			}
		}
		out = append(out, row)
	}
	// MatchQueue pages: the ones past the page are counted, not listed.
	if n := q.Counts["unmatched"]; len(q.Rows) == healthMaxRows && n > len(q.Rows) {
		h.uncounted = n - len(q.Rows)
	}
	return out, nil
}

// film is a row read for the checks that look at films.
type film struct {
	id, lib      int64
	title, path  string
	year         *int
	provider, pi string
	height       int
	size         int64
	poster       string
}

// films reads the films (titles not in a show, and not attached to another) of the
// libraries looked at whose type is one of these.
func (h *healthRun) films(types ...string) ([]film, error) {
	where, args := h.in("i.library_id")
	rows, err := h.s.Store.DB.QueryContext(h.ctx, `SELECT i.id, i.library_id, i.title, i.path, i.year, IFNULL(i.provider, ''), IFNULL(i.provider_id, ''), IFNULL(i.height, 0), IFNULL(i.size_bytes, 0), IFNULL(i.poster, '')
		FROM items i WHERE i.missing = 0 AND i.show_id IS NULL AND `+where+`
		  AND NOT EXISTS (SELECT 1 FROM item_links l WHERE l.item_id = i.id)
		ORDER BY i.id LIMIT `+strconv.Itoa(healthReadCap*2), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	want := map[string]bool{}
	for _, t := range types {
		want[t] = true
	}
	var out []film
	for rows.Next() {
		var f film
		if err := rows.Scan(&f.id, &f.lib, &f.title, &f.path, &f.year, &f.provider, &f.pi, &f.height, &f.size, &f.poster); err != nil {
			return nil, err
		}
		if want[h.types[f.lib]] && !h.scanning[f.lib] {
			out = append(out, f)
		}
	}
	return out, rows.Err()
}

// duplicates: the same film twice in one library, not attached as versions; the same
// episode in two files; one show in two folders.
func (h *healthRun) duplicates() ([]HealthRow, error) {
	films, err := h.films("movies", "discs")
	if err != nil {
		return nil, err
	}
	// Copies are the same match, or the same title and year where one of them has no match
	// (a copy the NFO matched beside one that found nothing). Two films matched to different
	// titles are never the same, whatever they're called. Only with a year: remakes share names.
	parent := make([]int, len(films))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	match := make([]string, len(films)) // a group's match, kept on its root
	join := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra == rb || (match[ra] != "" && match[rb] != "" && match[ra] != match[rb]) {
			return
		}
		parent[ra] = rb
		if match[rb] == "" {
			match[rb] = match[ra]
		}
	}
	byMatch, byName := map[string]int{}, map[string][]int{}
	for i, f := range films {
		lib := strconv.FormatInt(f.lib, 10) + "\x00"
		if f.provider != "" && f.pi != "" {
			k := lib + f.provider + "\x00" + f.pi
			match[i] = k
			if j, ok := byMatch[k]; ok {
				join(i, j)
			} else {
				byMatch[k] = i
			}
		}
		if t := strings.ToLower(strings.TrimSpace(f.title)); t != "" && f.year != nil {
			k := fmt.Sprintf("%s%s\x00%d", lib, t, *f.year)
			for _, j := range byName[k] {
				join(i, j)
			}
			byName[k] = append(byName[k], i)
		}
	}
	groups := map[int][]film{}
	var order []int
	for i, f := range films {
		r := find(i)
		if groups[r] == nil {
			order = append(order, r)
		}
		groups[r] = append(groups[r], f)
	}
	var out []HealthRow
	for _, k := range order {
		g := groups[k]
		if len(g) < 2 {
			continue
		}
		sort.SliceStable(g, func(i, j int) bool { return better(g[i], g[j]) })
		r := HealthRow{Target: TargetItem, ID: g[0].id, LibraryID: g[0].lib, Title: g[0].title, Year: g[0].year,
			Detail: fmt.Sprintf("%d copies in one library", len(g)), Fixes: []string{FixVersions, FixOpen, FixSetAside}}
		ids := make([]string, len(g))
		for i, f := range g {
			ids[i] = strconv.FormatInt(f.id, 10)
			r.Members = append(r.Members, HealthMember{ID: f.id, Title: f.title, Path: f.path, Height: f.height, Size: f.size})
		}
		sort.Strings(ids)
		r.Key = "duplicates:films:" + strings.Join(ids, ",")
		out = append(out, r)
	}
	// Episodes: one show, one number, two files. A version is for films only, so the fix is
	// a person's: keep the better file.
	where, args := h.in("i.library_id")
	rows, err := h.s.Store.DB.QueryContext(h.ctx, `SELECT i.show_id, sh.title, i.library_id, i.season, i.episode, group_concat(i.id, ','), COUNT(*) FROM items i JOIN shows sh ON sh.id = i.show_id
		WHERE i.missing = 0 AND sh.missing = 0 AND i.season IS NOT NULL AND i.episode IS NOT NULL AND `+where+`
		  AND NOT EXISTS (SELECT 1 FROM item_links l WHERE l.item_id = i.id)
		GROUP BY i.show_id, i.season, i.episode HAVING COUNT(*) > 1 LIMIT 2000`, args...)
	if err != nil {
		return nil, err
	}
	type epGroup struct {
		show, lib       int64
		title           string
		season, episode int
		ids             string
	}
	var eps []epGroup
	for rows.Next() {
		var e epGroup
		var n int
		if err := rows.Scan(&e.show, &e.title, &e.lib, &e.season, &e.episode, &e.ids, &n); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := h.types[e.lib]; ok && !h.scanning[e.lib] {
			eps = append(eps, e)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, e := range eps {
		r := HealthRow{Key: fmt.Sprintf("duplicates:episode:%d:%d:%d", e.show, e.season, e.episode), Target: TargetShow, ID: e.show, LibraryID: e.lib, Title: e.title,
			Detail: fmt.Sprintf("Season %d episode %d is in more than one file", e.season, e.episode), Fixes: []string{FixOpen, FixSetAside}}
		for _, idText := range strings.Split(e.ids, ",") {
			id, _ := strconv.ParseInt(idText, 10, 64)
			if it, err := h.s.Store.Item(h.ctx, id); err == nil {
				r.Members = append(r.Members, HealthMember{ID: it.ID, Title: it.Title, Path: it.Path, Height: it.Height, Size: it.SizeBytes})
			}
		}
		out = append(out, r)
	}
	// One show in two folders: parts of one show, or two shows with a name in common.
	where, args = h.in("library_id")
	srows, err := h.s.Store.DB.QueryContext(h.ctx, `SELECT library_id, MIN(title), year, group_concat(id, ','), group_concat(path, char(10)) FROM shows WHERE missing = 0 AND `+where+`
		GROUP BY library_id, lower(title), IFNULL(year, 0) HAVING COUNT(*) > 1 LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer srows.Close()
	for srows.Next() {
		var r HealthRow
		var ids, paths string
		if err := srows.Scan(&r.LibraryID, &r.Title, &r.Year, &ids, &paths); err != nil {
			return nil, err
		}
		if _, ok := h.types[r.LibraryID]; !ok || h.scanning[r.LibraryID] {
			continue
		}
		idList, pathList := strings.Split(ids, ","), strings.Split(paths, "\n")
		r.ID, _ = strconv.ParseInt(idList[0], 10, 64)
		r.Target, r.Key = TargetShow, "duplicates:shows:"+ids
		r.Detail = fmt.Sprintf("%d folders with the same name", len(idList))
		for i, idText := range idList {
			id, _ := strconv.ParseInt(idText, 10, 64)
			m := HealthMember{ID: id, Title: r.Title}
			if i < len(pathList) {
				m.Path = pathList[i]
			}
			r.Members = append(r.Members, m)
		}
		r.Fixes = []string{FixOpen, FixSetAside}
		out = append(out, r)
	}
	return out, srows.Err()
}

// better: the copy to keep as the main version. The larger picture, then the larger file.
func better(a, b film) bool {
	if a.height != b.height {
		return a.height > b.height
	}
	if a.size != b.size {
		return a.size > b.size
	}
	return a.id < b.id
}

// names: files whose names the scan couldn't read well. An episode with no number; a file
// in a shows library that isn't in a show; a film whose title is a release name.
func (h *healthRun) names() ([]HealthRow, error) {
	var out []HealthRow
	where, args := h.in("i.library_id")
	rows, err := h.s.Store.DB.QueryContext(h.ctx, `SELECT i.id, i.library_id, i.title, i.path, IFNULL(sh.title, ''), i.show_id FROM items i
		LEFT JOIN shows sh ON sh.id = i.show_id
		WHERE i.missing = 0 AND `+where+`
		  AND NOT EXISTS (SELECT 1 FROM item_links l WHERE l.item_id = i.id)
		  AND ((i.show_id IS NOT NULL AND i.episode IS NULL AND IFNULL(i.aired, '') = '')
		    OR (i.show_id IS NULL AND i.library_id IN (SELECT id FROM libraries WHERE type IN ('shows', 'anime'))))
		ORDER BY i.library_id, i.path LIMIT `+strconv.Itoa(healthReadCap), args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r HealthRow
		var show string
		var showID *int64
		if err := rows.Scan(&r.ID, &r.LibraryID, &r.Title, &r.Path, &show, &showID); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := h.types[r.LibraryID]; !ok || h.scanning[r.LibraryID] {
			continue
		}
		r.Target, r.Key = TargetItem, rowKey(HealthNames, TargetItem, r.ID)
		if showID != nil {
			r.Detail = "In " + show + ", but the name has no episode number, such as S01E02"
		} else {
			r.Detail = "In a shows library, but not in a show's folder, or the name doesn't say which show"
		}
		r.Fixes = []string{FixRules, FixOpen, FixSetAside}
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	films, err := h.films("movies", "discs")
	if err != nil {
		return nil, err
	}
	for _, f := range films {
		if !releaseName(f.title) {
			continue
		}
		out = append(out, HealthRow{Key: rowKey(HealthNames, TargetItem, f.id), Target: TargetItem, ID: f.id, LibraryID: f.lib, Title: f.title, Year: f.year, Path: f.path,
			Detail: "The title is the file's release name", Fixes: []string{FixRules, FixMatch, FixOpen, FixSetAside}})
	}
	return out, nil
}

// artwork: films and shows with no poster, in the libraries that look titles up.
func (h *healthRun) artwork() ([]HealthRow, error) {
	films, err := h.films("movies", "discs", "shows", "anime")
	if err != nil {
		return nil, err
	}
	var out []HealthRow
	for _, f := range films {
		if f.poster != "" || h.types[f.lib] == "shows" || h.types[f.lib] == "anime" {
			continue // a file loose in a shows library is under Odd names
		}
		r := HealthRow{Key: rowKey(HealthArtwork, TargetItem, f.id), Target: TargetItem, ID: f.id, LibraryID: f.lib, Title: f.title, Year: f.year, Path: f.path}
		r.Fixes = artFixes(f.provider)
		out = append(out, r)
	}
	where, args := h.in("library_id")
	rows, err := h.s.Store.DB.QueryContext(h.ctx, `SELECT id, library_id, title, path, year, IFNULL(provider, '') FROM shows WHERE missing = 0 AND IFNULL(poster, '') = '' AND `+where+`
		ORDER BY sort_title COLLATE NOCASE LIMIT `+strconv.Itoa(healthReadCap), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r HealthRow
		var provider string
		if err := rows.Scan(&r.ID, &r.LibraryID, &r.Title, &r.Path, &r.Year, &provider); err != nil {
			return nil, err
		}
		if _, ok := h.types[r.LibraryID]; !ok || h.scanning[r.LibraryID] {
			continue
		}
		r.Target, r.Key = TargetShow, rowKey(HealthArtwork, TargetShow, r.ID)
		r.Fixes = artFixes(provider)
		out = append(out, r)
	}
	return out, rows.Err()
}

func artFixes(provider string) []string {
	if provider == "" {
		return []string{FixMatch, FixOpen, FixSetAside} // nothing to fetch it from yet
	}
	return []string{FixRefresh, FixOpen, FixSetAside}
}

// unreadable: video files whose details couldn't be read, so they may not play. A scan
// leaves the container empty when the probe fails.
func (h *healthRun) unreadable() ([]HealthRow, error) {
	where, args := h.in("i.library_id")
	rows, err := h.s.Store.DB.QueryContext(h.ctx, `SELECT i.id, i.library_id, i.title, i.path, IFNULL(sh.title, ''), i.season, i.episode FROM items i
		LEFT JOIN shows sh ON sh.id = i.show_id
		WHERE i.missing = 0 AND i.kind = 'video' AND IFNULL(i.container, '') = '' AND i.path NOT LIKE '%://%' AND `+where+`
		ORDER BY i.library_id, i.sort_title COLLATE NOCASE LIMIT `+strconv.Itoa(healthReadCap), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HealthRow
	for rows.Next() {
		var r HealthRow
		var show string
		var season, episode *int
		if err := rows.Scan(&r.ID, &r.LibraryID, &r.Title, &r.Path, &show, &season, &episode); err != nil {
			return nil, err
		}
		if _, ok := h.types[r.LibraryID]; !ok || h.scanning[r.LibraryID] {
			continue
		}
		r.Target, r.Key = TargetItem, rowKey(HealthUnreadable, TargetItem, r.ID)
		if show != "" {
			r.Detail = show + episodeWords(season, episode)
		}
		r.Fixes = []string{FixProbe, FixOpen, FixSetAside}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- fixes

// MakeVersions attaches every other film to the first as a version of it, as the item
// editor would one at a time. They're checked to still be what the page showed: the same
// library, films, not attached already.
func (s *Store) MakeVersions(ctx context.Context, ids []int64) error {
	if len(ids) < 2 || len(ids) > 20 {
		return errors.New("choose two or more copies")
	}
	main, err := s.Item(ctx, ids[0])
	if err != nil {
		return err
	}
	for _, id := range ids[1:] {
		it, err := s.Item(ctx, id)
		if err != nil {
			return err
		}
		if it.LibraryID != main.LibraryID {
			return errors.New("those copies are in different libraries")
		}
	}
	for _, id := range ids[1:] {
		if err := s.AttachItem(ctx, id, LinkVersion, "", "", &main.ID, nil); err != nil {
			return err
		}
	}
	return nil
}

// ErrStillThere: forgetting a title whose file is back, or whose drive is away.
var ErrStillThere = errors.New("its file is there, or its drive isn't connected: scan the library instead")

// ForgetGone removes a title whose file has gone, with everything known about it and
// everyone's progress on it. It's only done when the file is still gone and the library's
// folder it was in is there and not empty: a drive that isn't plugged in mustn't lose a
// library's history.
func (s *Store) ForgetGone(ctx context.Context, target string, id int64) error {
	table := "items"
	if target == TargetShow {
		table = "shows"
	}
	var lib int64
	var path string
	var missing int
	if err := s.DB.QueryRowContext(ctx, `SELECT library_id, path, missing FROM `+table+` WHERE id = ?`, id).Scan(&lib, &path, &missing); err != nil {
		return ErrNotFound
	}
	if missing != 1 {
		return ErrStillThere
	}
	file := path
	if i := strings.Index(file, "#track="); i >= 0 {
		file = file[:i]
	}
	if !filepath.IsAbs(file) {
		return ErrStillThere
	}
	if _, err := os.Lstat(file); !errors.Is(err, fs.ErrNotExist) {
		return ErrStillThere
	}
	roots, err := s.LibraryRoots(ctx)
	if err != nil {
		return err
	}
	reachable := false
	for _, r := range roots {
		if !inRoots(file, []string{r}) {
			continue
		}
		if entries, err := os.ReadDir(r); err == nil && len(entries) > 0 {
			reachable = true
		}
	}
	if !reachable {
		return ErrStillThere
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if target == TargetShow {
		if _, err := tx.ExecContext(ctx, `DELETE FROM items WHERE show_id = ? AND missing = 1`, id); err != nil {
			return err
		}
		var left int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM items WHERE show_id = ?`, id).Scan(&left); err != nil {
			return err
		}
		if left > 0 {
			return ErrStillThere // some of its episodes are back
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE id = ? AND missing = 1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM title_meta WHERE target = ? AND target_id = ?`, target, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ReadAgain reads a video file's details again, as a scan does, and keeps them if it can be
// read now. It says why not otherwise.
func (s *Scanner) ReadAgain(ctx context.Context, id int64) (*Item, error) {
	it, err := s.Store.Item(ctx, id)
	if err != nil {
		return nil, err
	}
	if it.Kind != "video" || it.Missing {
		return nil, errors.New("only a video file that's there can be read again")
	}
	if s.Tools == nil {
		return nil, errors.New("FFprobe isn't set up, so no file can be read. See Settings, Repair")
	}
	pctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	_, err = s.Tools.ProbeFile(pctx, it.Path)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("it still can't be read: %s", firstLine(err.Error()))
	}
	s.probeVideo(ctx, it)
	if it.Container == "" {
		return nil, errors.New("it still can't be read")
	}
	return it, s.Store.UpdateItem(ctx, it)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// ---------------------------------------------------------------- NFO files

// SidecarLimits keep the NFO check from running for ever on a huge library.
type SidecarLimits struct {
	Files    int
	Deadline time.Time
}

// CheckSidecars reads the NFO and metadata files beside each title's file, the ones a scan
// would read, and returns a row for each it can't use. problem says what's wrong with one
// (the same reader as Repair's check). Folders that aren't there are skipped.
func (s *Store) CheckSidecars(ctx context.Context, lim SidecarLimits, problem func(path string, b []byte) string, progress func(done int)) (*SidecarCheck, error) {
	libs, err := s.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	here := map[int64][]string{}
	for _, l := range libs {
		for _, p := range l.Paths {
			if entries, err := os.ReadDir(p); err == nil && len(entries) > 0 {
				here[l.ID] = append(here[l.ID], p)
			}
		}
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id, i.library_id, i.kind, i.path, i.title, i.show_id, IFNULL(sh.title, '') FROM items i LEFT JOIN shows sh ON sh.id = i.show_id
		WHERE i.missing = 0 ORDER BY i.library_id, i.id`)
	if err != nil {
		return nil, err
	}
	type it struct {
		id, lib           int64
		kind, path, title string
		show              *int64
		showTitle         string
	}
	var items []it
	for rows.Next() {
		var x it
		if err := rows.Scan(&x.id, &x.lib, &x.kind, &x.path, &x.title, &x.show, &x.showTitle); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := &SidecarCheck{At: time.Now(), Rows: []HealthRow{}}
	seen := map[string]bool{}
	for n, x := range items {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if len(seen) > lim.Files || (!lim.Deadline.IsZero() && time.Now().After(lim.Deadline)) {
			out.Stopped = true
			break
		}
		if progress != nil && n%200 == 0 {
			progress(n)
		}
		if x.kind == "radio" || strings.Contains(x.path, "://") {
			continue
		}
		file := x.path
		if i := strings.Index(file, "#track="); i >= 0 {
			file = file[:i]
		}
		if !filepath.IsAbs(file) || !inRoots(file, here[x.lib]) {
			continue
		}
		dir := filepath.Dir(file)
		stem := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		cands := []string{filepath.Join(dir, stem+".nfo"), filepath.Join(dir, stem+".metadata.json"), file + ".metadata.json", filepath.Join(dir, "movie.nfo")}
		if x.show != nil {
			cands = append(cands, filepath.Join(dir, "tvshow.nfo"), filepath.Join(filepath.Dir(dir), "tvshow.nfo"))
		}
		for _, p := range cands {
			if seen[p] {
				continue
			}
			seen[p] = true
			why := sidecarProblem(p, problem)
			if why == "" {
				continue
			}
			r := HealthRow{Key: "nfo:" + p, Target: TargetItem, ID: x.id, LibraryID: x.lib, Title: x.title, Path: p, Detail: why, Fixes: []string{FixOpen, FixSetAside}}
			if strings.EqualFold(filepath.Base(p), "tvshow.nfo") && x.show != nil {
				r.Target, r.ID, r.Title = TargetShow, *x.show, x.showTitle
			}
			out.Rows = append(out.Rows, r)
		}
	}
	return out, nil
}

// sidecarProblem reads one file, if it's there, the way a scan would.
func sidecarProblem(p string, problem func(string, []byte) string) string {
	st, err := os.Lstat(p)
	if err != nil || !st.Mode().IsRegular() {
		return ""
	}
	if st.Size() > maxNFOFile {
		return "it's too big to be a metadata file, so it's ignored"
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "it can't be read: " + firstLine(err.Error())
	}
	return problem(p, b)
}
