// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// dropModuleEntriesLibrariesCover keeps the menu from saying a word twice: a module's entry ("Books" from
// WittyInk, "Photos" from WittyPhotos) is left out when another entry of the menu, a library's or the
// administrator's own, already has the same label. The library entry wins: it opens the library, and what
// the module adds (its page, its views) is reached from there. The rule is the label, compared without case
// or spaces at the ends, so it works for any module and any language; a module entry with a label of its
// own stays. Personalisation lists follow what is left.
func dropModuleEntriesLibrariesCover(items []sdui.NavEntry, personal *sdui.PersonalInfo, set *sdui.ModuleSet) ([]sdui.NavEntry, *sdui.PersonalInfo) {
	fromModule := map[string]bool{}
	for _, e := range set.Navigation() {
		fromModule[e.ID] = true
	}
	return dedupeModuleEntries(items, personal, fromModule)
}

func dedupeModuleEntries(items []sdui.NavEntry, personal *sdui.PersonalInfo, fromModule map[string]bool) ([]sdui.NavEntry, *sdui.PersonalInfo) {
	if len(fromModule) == 0 {
		return items, personal
	}
	label := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	taken := map[string]bool{}
	for _, it := range items {
		if !fromModule[it.ID] {
			taken[label(it.Label)] = true
		}
	}
	dropped := map[string]bool{}
	kept := make([]sdui.NavEntry, 0, len(items))
	for _, it := range items {
		if fromModule[it.ID] && taken[label(it.Label)] {
			dropped[it.ID] = true
			continue
		}
		kept = append(kept, it)
	}
	if len(dropped) == 0 {
		return items, personal
	}
	if personal != nil {
		p := *personal
		p.Sections = nil
		for _, s := range personal.Sections {
			if !dropped[s.ID] {
				p.Sections = append(p.Sections, s)
			}
		}
		personal = &p
	}
	return kept, personal
}

// wittyPlayNavID is WittyPlay's menu entry: its console home, which is the server's Games.
const wittyPlayNavID = "wittyplay-online"

// gamesBecomeWittyPlay: with WittyPlay in the menu, it replaces the games libraries' entries rather than
// being dropped as a repeat of their label. It takes the first one's place, and its placement for every
// kind of device WittyPlay's own entry doesn't place (its own wins: Games on a phone's bottom bar); the
// libraries are still reached from inside WittyPlay. Every app gets this from here.
func gamesBecomeWittyPlay(items []sdui.NavEntry, personal *sdui.PersonalInfo, libs []sdui.NavLibrary) ([]sdui.NavEntry, *sdui.PersonalInfo) {
	at := -1
	for i, it := range items {
		if it.ID == wittyPlayNavID {
			at = i
		}
	}
	if at < 0 {
		return items, personal
	}
	games := map[string]bool{}
	for _, l := range libs {
		if l.Kind == "games" {
			games["lib-"+strconv.FormatInt(l.ID, 10)] = true
		}
	}
	play := items[at]
	dropped := map[string]bool{}
	out := make([]sdui.NavEntry, 0, len(items))
	placed := false
	for i, it := range items {
		if i == at {
			continue
		}
		if games[it.ID] {
			dropped[it.ID] = true
			if !placed {
				// The more prominent of the two: Games stays on the bar where the games library was, and
				// goes on it where WittyPlay puts itself there (a phone's bottom bar). A games entry an
				// administrator hid stays hidden.
				play.Placement = morePlaced(play.Placement, it.Placement)
				out = append(out, play)
				placed = true
			}
			continue
		}
		out = append(out, it)
	}
	if !placed {
		return items, personal
	}
	if personal != nil {
		p := *personal
		p.Sections = nil
		for _, s := range personal.Sections {
			if !dropped[s.ID] {
				p.Sections = append(p.Sections, s)
			}
		}
		personal = &p
	}
	return out, personal
}

// morePlaced is the more prominent of a module's placement and the entry it replaces: the bar, then
// pinned to its end, then More. A replaced entry that was hidden keeps it hidden.
func morePlaced(module, replaced string) string {
	if replaced == "hidden" {
		return replaced
	}
	rank := map[string]int{"primary": 3, "": 3, "end": 2, "more": 1, "hidden": 0}
	if rank[module] >= rank[replaced] {
		return module
	}
	return replaced
}
