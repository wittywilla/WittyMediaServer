// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// The sources of the pages every app draws from the server's own data (docs/SDUI.md, "Data sources"):
// a title's versions, the lists a title is in, this profile's signed-in devices, its year, what the
// server is doing, and notifications. Each answers only what the profile may see: a title through
// visibleItem, devices and notifications the profile's own (an administrator's activity is the
// server's), and each row that does something carries the command for it.

// pageSource answers one of them; ok is false for any other source.
func (rv *sduiResolver) pageSource(req sdui.SourceRequest) (sdui.SourceResult, bool, error) {
	typ := req.Source.Type()
	var all []sdui.Item
	var err error
	switch typ {
	case "versions":
		all, err = rv.versionsOf(req.Source.Str("item"))
	case "listsFor":
		all, err = rv.listsFor(req.Source.Str("item"))
	case "devices":
		all, err = rv.devices()
	case "year":
		all, err = rv.yearRows(req.Source.Str("part"), req.Source.Int("year"))
	case "activity":
		all = rv.activityRows(req.Source.Str("part"))
	case "notices":
		all, err = rv.noticeRows()
	case "formats":
		all = rv.formatRows(req.Source.Str("item"))
	case "newList":
		all = []sdui.Item{{ID: "name", Kind: "view", Title: "Name", Icon: "plus", Control: &sdui.Control{Type: sdui.ControlField, Name: "name", Placeholder: "A name for the list", MaxLength: 60, Input: "text"}}}
	default:
		return sdui.SourceResult{}, false, nil
	}
	if err != nil {
		return sdui.SourceResult{}, true, err
	}
	if lim := req.Source.Int("limit"); lim > 0 && lim < req.Limit {
		req.Limit = lim
	}
	return page(all, req), true, nil
}

// subjectItem is the title an "item" the page names is, when this profile may see it (a film, an
// episode or a show-<n>): nil otherwise, which a page shows as nothing.
func (rv *sduiResolver) subjectItem(id string) (*library.Item, *library.Show) {
	if strings.HasPrefix(id, "show-") {
		n, err := strconv.ParseInt(strings.TrimPrefix(id, "show-"), 10, 64)
		if err != nil {
			return nil, nil
		}
		sh, err := rv.s.visibleShow(rv.r, n)
		if err != nil {
			return nil, nil
		}
		return nil, sh
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, nil
	}
	it, err := rv.s.visibleItem(rv.r, n)
	if err != nil {
		return nil, nil
	}
	return it, nil
}

func sizeWords(b int64) string {
	switch {
	case b <= 0:
		return ""
	case b >= 1<<30:
		return fmt.Sprintf("%.0f GB", float64(b)/(1<<30))
	default:
		return fmt.Sprintf("%.0f MB", float64(b)/(1<<20))
	}
}

// versionsOf: the files a title plays from (the title's own, then the versions attached to it), each a row
// that plays it: the version's label and quality, its length and size. One row for a title with one file.
func (rv *sduiResolver) versionsOf(id string) ([]sdui.Item, error) {
	it, _ := rv.subjectItem(id)
	if it == nil {
		return nil, nil
	}
	local := rv.s.localFor(rv.r, it)
	var out []sdui.Item
	for i, v := range local.Versions {
		li, err := rv.s.Store.Item(rv.ctx, v.ItemID)
		if err != nil {
			continue
		}
		title := strings.TrimSpace(strings.Join(nonEmpty(v.Label, v.Quality), " · "))
		if title == "" {
			title = it.Title
		}
		line := strings.Join(nonEmpty(runtimeWords(li.RuntimeSec), sizeWords(v.SizeBytes), strings.ToUpper(v.Container)), " · ")
		row := sdui.Item{ID: itoa(v.ItemID), Kind: "video", Title: title, Subtitle: line, Icon: "play", Scope: ""}
		if i == 0 {
			row.Badges = []string{"Default"}
		}
		if !rv.noPlay {
			row.Action = &sdui.Action{Type: sdui.ActPlay, Item: itoa(v.ItemID)}
		}
		out = append(out, row)
	}
	return out, nil
}

func nonEmpty(list ...string) []string {
	var out []string
	for _, s := range list {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func runtimeWords(sec float64) string {
	if sec <= 0 {
		return ""
	}
	return sdui.FormatRuntime(sec)
}

// listsFor: the lists this profile owns that a title can be in (its watch later, its own manual lists), each
// a switch row: on when the title is in it, and flipping it adds or takes the title out.
func (rv *sduiResolver) listsFor(id string) ([]sdui.Item, error) {
	it, sh := rv.subjectItem(id)
	if it == nil && sh == nil {
		return nil, nil
	}
	var itemID, showID int64
	if it != nil {
		itemID = it.ID
	} else {
		showID = sh.ID
	}
	holding, err := rv.s.Store.CollectionsHolding(rv.ctx, rv.u.ID, itemID, showID)
	if err != nil {
		return nil, err
	}
	all, err := rv.s.Store.Collections(rv.ctx, rv.u.ID, rv.acc)
	if err != nil {
		return nil, err
	}
	var out []sdui.Item
	for _, c := range all {
		if c.OwnerID != rv.u.ID || c.Kind == "smart" {
			continue
		}
		on := false
		for _, h := range holding {
			on = on || h == c.ID
		}
		op := "collection.add"
		if on {
			op = "collection.remove"
		}
		name := c.Name
		if c.System == "watchlater" {
			name = "Watch later"
		}
		out = append(out, sdui.Item{ID: itoa(c.ID), Kind: "collection", Title: name, Icon: map[bool]string{true: "clock", false: "collection"}[c.System == "watchlater"], Count: c.Count,
			Action:  &sdui.Action{Type: sdui.ActCommand, Op: op, Item: id, Params: map[string]any{"collection": itoa(c.ID)}},
			Control: &sdui.Control{Type: sdui.ControlSwitch, Value: on}})
	}
	return out, nil
}

// formatRows: the formats one book is in: the file that opens, then the others beside it with the same name (a
// book downloaded as CBZ, EPUB and PDF is one book; the best is used). Only the format and the size are
// said, never a path. Nothing for a title that isn't a book the profile may see.
func (rv *sduiResolver) formatRows(id string) []sdui.Item {
	it, _ := rv.subjectItem(id)
	if it == nil || it.Kind != "book" {
		return nil
	}
	stem := strings.TrimSuffix(filepath.Base(it.Path), filepath.Ext(it.Path))
	out := []sdui.Item{{ID: "own", Kind: "view", Title: strings.ToUpper(it.Container), Subtitle: strings.Join(nonEmpty("Opens in the reader", sizeWords(it.SizeBytes)), " \u00b7 "), Icon: "book", Badges: []string{"Opens"}}}
	entries, err := os.ReadDir(filepath.Dir(it.Path))
	if err != nil {
		return out
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == filepath.Base(it.Path) || strings.TrimSuffix(name, filepath.Ext(name)) != stem || mediatypes.Find(name, mediatypes.Comic, mediatypes.Book) == nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(name), "."))
		out = append(out, sdui.Item{ID: "f-" + strings.ToLower(ext), Kind: "view", Title: ext, Subtitle: sizeWords(info.Size()), Icon: "book"})
	}
	return out
}

// devices: where this profile is signed in, the one in hand first, the others with when each was last
// used and a Sign out that asks first.
func (rv *sduiResolver) devices() ([]sdui.Item, error) {
	list, err := rv.s.Auth.Sessions(rv.ctx, rv.u.ID, currentToken(rv.r))
	if err != nil {
		return nil, err
	}
	out := make([]sdui.Item, 0, len(list))
	for _, se := range list {
		name := se.Device
		if strings.TrimSpace(name) == "" {
			name = "A device"
		}
		it := sdui.Item{ID: se.ID, Kind: "view", Title: name, Icon: "devices"}
		seen := agoWords(rv.now, se.LastSeen)
		if se.Current {
			it.Subtitle, it.Trailing, it.Badges = "This device · "+seen, "Now", []string{"This device"}
		} else {
			it.Subtitle = seen
			it.Action = &sdui.Action{Type: sdui.ActCommand, Op: "session.end", Item: se.ID, Confirm: "Sign " + name + " out?", Params: map[string]any{"session": se.ID}}
			it.Trailing = "Sign out"
		}
		if se.Streaming {
			it.Badges = append(it.Badges, "Playing")
		}
		out = append(out, it)
	}
	// This device first, then the most recently used.
	sortDevices(out)
	return out, nil
}

func sortDevices(list []sdui.Item) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j].Action == nil && list[j-1].Action != nil; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

// agoWords says how long ago a Unix time was: "Just now", "2 hours ago", "Yesterday", "3 days ago".
func agoWords(now time.Time, unix int64) string {
	if unix <= 0 {
		return "Never used"
	}
	d := now.Sub(time.Unix(unix, 0))
	switch {
	case d < 2*time.Minute:
		return "Just now"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 2*time.Hour:
		return "An hour ago"
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "Yesterday"
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}

func hoursWords(sec float64) string {
	h := sec / 3600
	if h >= 10 {
		return fmt.Sprintf("%.0f hr", h)
	}
	return sdui.FormatRuntime(sec)
}

// yearRows: the profile's year from its own play log (docs/STATS.md): part totals are the headline
// numbers, titles, shows, artists and genres the top of each.
func (rv *sduiResolver) yearRows(part string, year int) ([]sdui.Item, error) {
	if year == 0 {
		year = rv.now.Year()
	}
	rev, err := rv.s.Store.ReviewOfYear(rv.ctx, rv.u.ID, year, rv.acc)
	if err != nil {
		return nil, err
	}
	var out []sdui.Item
	top := func(list []library.ReviewEntry, icon string) {
		for i, e := range list {
			sub := e.Sub
			if e.Plays > 1 {
				sub = strings.Join(nonEmpty(sub, fmt.Sprintf("%d times", e.Plays)), " · ")
			}
			out = append(out, sdui.Item{ID: fmt.Sprintf("%s-%d", icon, i+1), Kind: "view", Title: e.Name, Subtitle: sub, Trailing: hoursWords(e.Seconds), Icon: icon})
		}
	}
	switch part {
	case "titles":
		top(rev.TopTitles, "movie")
	case "shows":
		top(rev.TopShows, "show")
	case "artists":
		top(rev.TopArtists, "artist")
	case "genres":
		top(rev.TopGenres, "filter")
	case "months":
		for m, s := range rev.ByMonth {
			if s > 0 {
				out = append(out, sdui.Item{ID: fmt.Sprintf("month-%d", m+1), Kind: "view", Title: time.Month(m + 1).String(), Trailing: hoursWords(s), Icon: "calendar"})
			}
		}
	default:
		if rev.Items == 0 {
			return nil, nil
		}
		out = append(out, sdui.Item{ID: "watched", Kind: "view", Title: "Time spent", Trailing: hoursWords(rev.Seconds), Subtitle: fmt.Sprintf("%d titles", rev.Items), Icon: "clock"},
			sdui.Item{ID: "days", Kind: "view", Title: "Days you came by", Trailing: strconv.Itoa(rev.Days), Icon: "calendar"},
			sdui.Item{ID: "streak", Kind: "view", Title: "Longest streak", Trailing: fmt.Sprintf("%d days", rev.LongestRun), Icon: "fire"})
		if rev.BusiestSecs > 0 {
			out = append(out, sdui.Item{ID: "busiest", Kind: "view", Title: "Biggest day", Trailing: hoursWords(rev.BusiestSecs), Icon: "star"})
		}
	}
	return out, nil
}

// activityRows: what the server is doing for this profile to see (part "now": what runs, with how far
// it has got) or has just done (part "history"); the profile's own limits decide whether any of it is
// shown (an account that may not see activity gets none).
func (rv *sduiResolver) activityRows(part string) []sdui.Item {
	if rv.s.Tasks == nil {
		return nil
	}
	list := rv.s.taskList(rv.s.taskViewer(rv.r), rv.s.activityAccessFor(rv.ctx, rv.u))
	src := list.Tasks
	if part == "history" {
		src = list.Recent
	}
	out := make([]sdui.Item, 0, len(src))
	for _, t := range src {
		it := sdui.Item{ID: t.ID, Kind: "view", Title: t.Title, Icon: "plug", Subtitle: strings.Join(nonEmpty(t.Phase, t.LibraryName), " · ")}
		switch {
		case t.State == "failed":
			it.Badges, it.Subtitle = []string{"Problem"}, strings.Join(nonEmpty(t.Result, it.Subtitle), " · ")
		case part == "history":
			it.Subtitle = strings.Join(nonEmpty(t.Result, agoWords(rv.now, t.FinishedAt)), " · ")
		case t.Percent >= 0:
			it.Trailing, it.Progress = fmt.Sprintf("%.0f%%", t.Percent), t.Percent/100
		}
		out = append(out, it)
	}
	return out
}

// noticeRows: what this profile may want to know: a reminder that a programme starts soon, and for an
// administrator a new version of the server. Each says what to do about it in its row's action.
func (rv *sduiResolver) noticeRows() ([]sdui.Item, error) {
	var out []sdui.Item
	if rv.u.IsAdmin && rv.s.Update != nil {
		if st := rv.s.Update.Status(); st.Latest != nil {
			out = append(out, sdui.Item{ID: "update", Kind: "view", Title: "Update " + st.Latest.Version, Subtitle: "A new version of the server is ready.", Icon: "download", Badges: []string{"Update"},
				Route: "/settings/updates"})
		}
	}
	if rv.s.Channels != nil {
		list, err := rv.s.Channels.Reminders(rv.ctx, rv.u.ID, rv.now.Unix())
		if err != nil {
			return nil, err
		}
		for _, e := range list {
			if !rv.s.canSeeChannelKey(rv.r, e.Channel) {
				continue
			}
			out = append(out, sdui.Item{ID: "reminder-" + itoa(e.ID), Kind: "view", Title: e.Title, Subtitle: "On " + time.Unix(e.Start, 0).Format("Mon 3:04 PM"), Icon: "clock",
				Trailing: "Watch", Route: "/tv/watch/" + e.Channel})
		}
	}
	return out, nil
}
