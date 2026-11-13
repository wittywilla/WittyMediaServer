// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// The administrators' pages (manage-libraries, fix-matches, match): their rows, from what the admin routes
// read. Each source is empty for anyone but an administrator, and each row's command goes to an admin route,
// which checks again.

// libraryAdminRows: every library, its kind, size and last scan; a row scans it again.
func (rv *sduiResolver) libraryAdminRows() ([]sdui.Item, error) {
	if !rv.u.IsAdmin {
		return nil, nil
	}
	libs, err := rv.s.Store.Libraries(rv.ctx)
	if err != nil {
		return nil, err
	}
	out := make([]sdui.Item, 0, len(libs))
	for _, l := range libs {
		facts := []string{l.Type, strconv.Itoa(l.ItemCount) + " titles"}
		if l.LastScanAt != nil && *l.LastScanAt > 0 {
			facts = append(facts, "scanned "+time.Unix(*l.LastScanAt, 0).Format("2 Jan 2006 15:04"))
		}
		if len(l.Paths) > 0 {
			facts = append(facts, l.Paths[0])
		}
		row := sdui.Item{ID: "lib-" + itoa(l.ID), Kind: "view", Title: l.Name, Subtitle: strings.Join(facts, " · "), Icon: "library", Trailing: "Scan"}
		if l.Scanning {
			row.Trailing = "Scanning"
		} else {
			row.Action = &sdui.Action{Type: sdui.ActCommand, Op: "library.scan", Item: itoa(l.ID)}
		}
		out = append(out, row)
	}
	return out, nil
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
			label = "Music videos and concerts"
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
			Action: &sdui.Action{Type: sdui.ActCommand, Op: "meta.match", Item: ref, Confirm: "Match it to " + name + "?",
				Params: map[string]any{"target": target, "title": idText, "source": c.Source, "ref": c.ID}}})
	}
	for _, w := range res.Warnings {
		out = append(out, sdui.Item{ID: "w-" + strconv.Itoa(len(out)), Kind: "view", Title: w, Icon: "warning"})
	}
	out = append(out, sdui.Item{ID: "unmatch", Kind: "view", Title: "None of these", Subtitle: "Leave it unmatched: scans won't look it up again.", Icon: "warning",
		Action: &sdui.Action{Type: sdui.ActCommand, Op: "meta.unmatch", Item: ref, Confirm: "Leave it unmatched?", Params: map[string]any{"target": target, "title": idText}}})
	return out, nil
}
