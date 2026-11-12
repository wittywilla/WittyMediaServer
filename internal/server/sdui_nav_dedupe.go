// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
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
