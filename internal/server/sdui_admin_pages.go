// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"fmt"
	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// The administrators' pages (manage-libraries, fix-matches, match): their rows, from what the admin routes
// read. Each source is empty for anyone but an administrator, and each row's command goes to an admin route,
// which checks again.

// libraryAdminRows: every library, its kind, size and last scan (or how far its scan has got); a row opens its
// scan choices (manage-library, as a layer). busy: a library is being scanned, so the page is to be drawn
// again (Meta.RefreshSec) until none is.
func (rv *sduiResolver) libraryAdminRows() (out []sdui.Item, busy bool, err error) {
	if !rv.u.IsAdmin {
		return nil, false, nil
	}
	libs, err := rv.s.Store.Libraries(rv.ctx)
	if err != nil {
		return nil, false, err
	}
	out = make([]sdui.Item, 0, len(libs))
	for _, l := range libs {
		facts := []string{l.Type, strconv.Itoa(l.ItemCount) + " titles"}
		if l.LastScanAt != nil && *l.LastScanAt > 0 {
			facts = append(facts, "scanned "+time.Unix(*l.LastScanAt, 0).Format("2 Jan 2006 15:04"))
		}
		if len(l.Paths) > 0 {
			facts = append(facts, l.Paths[0])
		}
		row := sdui.Item{ID: "lib-" + itoa(l.ID), Kind: "view", Title: l.Name, Subtitle: strings.Join(facts, " · "), Icon: "library", Trailing: "Scan",
			Action: &sdui.Action{Type: sdui.ActNavigate, To: "/manage/library/" + itoa(l.ID), As: sdui.PresentLayer}}
		if p := rv.s.Scanner.Snapshot(l.ID); p != nil {
			row.Trailing, busy = scanCount(p), true
		} else if l.Scanning {
			row.Trailing, busy = "Scanning", true
		}
		out = append(out, row)
	}
	return out, busy, nil
}

// scanCount says how far a scan has got, as the activity panel does: "12,400 of 16,780 files".
func scanCount(p *library.ScanProgress) string {
	if p.Found > 0 {
		return groupDigits(p.Done) + " of " + groupDigits(p.Found) + " files"
	}
	return "Scanning"
}

// groupDigits writes a count with commas: 16,780.
func groupDigits(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// libraryScanRows: one library's scan (manage-library): how far a scan has got, or when it was last scanned,
// then Scan for changes and Full rescan (which asks first). busy: it is being scanned, so the page is to be
// drawn again (Meta.RefreshSec) until it settles on "Last scanned".
func (rv *sduiResolver) libraryScanRows(ref string) (out []sdui.Item, busy bool, err error) {
	if !rv.u.IsAdmin {
		return nil, false, nil
	}
	id, err := strconv.ParseInt(ref, 10, 64)
	if err != nil {
		return nil, false, nil
	}
	l, err := rv.s.Store.Library(rv.ctx, id)
	if err != nil {
		return nil, false, nil //nolint:nilerr // a library that's gone has nothing to show
	}
	if p := rv.s.Scanner.Snapshot(id); p != nil {
		busy = true
		doing := "Scanning"
		if p.Mode == library.ScanFull {
			doing = "Full rescan"
		}
		if p.Resumed {
			doing += ", carrying on after a restart"
		}
		state := sdui.Item{ID: "state", Kind: "view", Title: doing, Subtitle: l.Name, Icon: "clock", Trailing: scanCount(p)}
		if p.Found > 0 {
			state.Title = doing + ": " + scanCount(p)
		}
		out = append(out, state)
	} else if l.Scanning || l.ScanMode != "" {
		busy = true
		out = append(out, sdui.Item{ID: "state", Kind: "view", Title: "Scanning", Subtitle: l.Name, Icon: "clock"})
	} else if l.LastScanAt != nil && *l.LastScanAt > 0 {
		out = append(out, sdui.Item{ID: "state", Kind: "view", Title: "Last scanned " + time.Unix(*l.LastScanAt, 0).Format("2 Jan 2006 15:04"), Subtitle: l.Name, Icon: "history"})
	}
	changes := sdui.Item{ID: "changes", Kind: "view", Title: "Scan for changes", Subtitle: "Only new, changed or removed files.", Icon: "folder"}
	full := sdui.Item{ID: "full", Kind: "view", Title: "Full rescan", Subtitle: "Reads every file and its details again, from scratch.", Icon: "library"}
	if busy {
		changes.Trailing, full.Trailing = "Scanning", "Scanning"
	} else {
		changes.Action = &sdui.Action{Type: sdui.ActCommand, Op: "library.scan", Item: ref}
		full.Action = &sdui.Action{Type: sdui.ActCommand, Op: "library.rescan", Item: ref,
			Confirm: i18n.F("Read every file of {name} and its details again? On a big library this can take a long time. The library stays usable meanwhile.", "name", l.Name)}
	}
	return append(out, changes, full), busy, nil
}

// newLibraryRows: a new library's name, kind and folder, for the library.create Form.
func (rv *sduiResolver) newLibraryRows() []sdui.Item {
	if !rv.u.IsAdmin {
		return nil
	}
	kinds := []sdui.EnumOption{}
	for _, k := range sdui.LibraryKinds {
		label := strings.ToUpper(k[:1]) + k[1:]
		if k == "musicvideos" {
			label = i18n.T("Music videos and concerts")
		}
		kinds = append(kinds, sdui.EnumOption{Value: k, Label: label})
	}
	return []sdui.Item{
		{ID: "name", Kind: "view", Title: "Name", Control: &sdui.Control{Type: sdui.ControlField, Name: "name", Placeholder: "Movies", MaxLength: 100, Input: "text"}},
		{ID: "type", Kind: "view", Title: "Kind", Control: &sdui.Control{Type: sdui.ControlSelect, Name: "type", Options: kinds, Value: kinds[0].Value}},
		{ID: "path", Kind: "view", Title: "Folder", Subtitle: "The whole path of the folder on the server.", Control: &sdui.Control{Type: sdui.ControlField, Name: "path", Placeholder: "/media/movies", MaxLength: 1000, Input: "text"}},
	}
}

// matchQueueRows: titles whose match needs a look, each opening /manage/match/{item}.
func (rv *sduiResolver) matchQueueRows(state string, lib int64, limit, _ int) ([]sdui.Item, int, error) {
	if !rv.u.IsAdmin || rv.s.Scanner == nil {
		return nil, 0, nil
	}
	ask := []string{state}
	switch state {
	case "", "attention":
		ask = []string{"unmatched", "low"}
	case "all":
		ask = []string{""}
	}
	var rows []library.QueueRow
	total := 0
	for _, st := range ask {
		q, err := rv.s.Scanner.MatchQueue(rv.ctx, lib, st, "", limit, 0)
		if err != nil {
			return nil, 0, err
		}
		rows, total = append(rows, q.Rows...), total+q.Total
	}
	out := make([]sdui.Item, 0, len(rows))
	for _, r := range rows {
		ref := itoa(r.ID)
		if r.Target == library.TargetShow {
			ref = "show-" + ref
		}
		title := r.Title
		if r.Year != nil {
			title += " (" + strconv.Itoa(*r.Year) + ")"
		}
		sub := r.File
		if r.Provider != "" {
			sub = strings.TrimPrefix(fmt.Sprintf("%s · %d%% sure", r.Label, r.Confidence), " · ")
		}
		if len(r.Reasons) > 0 {
			sub += " · " + r.Reasons[0]
		}
		out = append(out, sdui.Item{ID: "m-" + ref, Kind: "view", Title: title, Subtitle: sub, Icon: "search", Trailing: matchStateWord(r.State),
			Action: &sdui.Action{Type: sdui.ActNavigate, To: "/manage/match/" + ref}})
	}
	return out, total, nil
}

func matchStateWord(state string) string {
	return map[string]string{"unmatched": "Unmatched", "low": "Doubtful", "ok": "Matched", "manual": "By hand", "none": "Left unmatched"}[state]
}

// matchCandidateRows: what the sources suggest for a title, best first, each matching it after asking, then a
// row that takes the match off.
func (rv *sduiResolver) matchCandidateRows(ref string) ([]sdui.Item, error) {
	if !rv.u.IsAdmin || rv.s.Scanner == nil {
		return nil, nil
	}
	target, idText := library.TargetItem, ref
	if rest, ok := strings.CutPrefix(ref, "show-"); ok {
		target, idText = library.TargetShow, rest
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return nil, nil
	}
	t, err := rv.s.Store.LoadTitle(rv.ctx, target, id)
	if err != nil {
		return nil, nil
	}
	res, err := rv.s.Scanner.SearchMatches(rv.ctx, t, "", nil)
	if err != nil {
		return nil, err
	}
	out := []sdui.Item{}
	for _, c := range res.Results {
		name := c.Title
		if c.Year != nil {
			name += " (" + strconv.Itoa(*c.Year) + ")"
		}
		sub := []string{c.Source, strconv.Itoa(c.Score) + "% sure"}
		if len(c.Reasons) > 0 {
			sub = append(sub, strings.Join(c.Reasons, ", "))
		}
		out = append(out, sdui.Item{ID: "c-" + c.Source + "-" + c.ID, Kind: "view", Title: name, Subtitle: strings.Join(sub, " · "), Icon: "check", Trailing: "Match",
			Action: &sdui.Action{Type: sdui.ActCommand, Op: "meta.match", Item: ref, Confirm: i18n.F("Match it to {name}?", "name", name),
				Params: map[string]any{"target": target, "title": idText, "source": c.Source, "ref": c.ID}}})
	}
	for _, w := range res.Warnings {
		out = append(out, sdui.Item{ID: "w-" + strconv.Itoa(len(out)), Kind: "view", Title: w, Icon: "warning"})
	}
	out = append(out, sdui.Item{ID: "unmatch", Kind: "view", Title: "None of these", Subtitle: "Leave it unmatched: scans won't look it up again.", Icon: "warning",
		Action: &sdui.Action{Type: sdui.ActCommand, Op: "meta.unmatch", Item: ref, Confirm: i18n.T("Leave it unmatched?"), Params: map[string]any{"target": target, "title": idText}}})
	return out, nil
}
