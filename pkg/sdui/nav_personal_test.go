// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "testing"

// The menu says what a profile may arrange and what it has arranged, so an app
// can offer "Arrange the menu" from the answer alone: every entry in the
// profile's order, the hidden ones included and marked.
func TestNavigationPersonalListsHiddenEntries(t *testing.T) {
	pages := func(id string) (Page, bool) {
		for _, p := range StockPages() {
			if p.ID == id {
				return p, true
			}
		}
		return Page{}, false
	}
	libs := []NavLibrary{{ID: 1, Name: "Films", Kind: "movies"}, {ID: 2, Name: "Shows", Kind: "shows"}}
	ctx := func(p *Personal, can bool) *RenderContext {
		return &RenderContext{ProfileKind: "adult", Flags: map[string]bool{}, Vars: map[string]string{}, CanPersonalise: can, Personal: p}
	}

	plain := RenderNavigation(StockNavigation(), pages, libs, ctx(nil, true))
	if plain.Personal == nil || !plain.Personal.Editable || plain.Personal.URL != "/api/v1/sdui/navigation/personal" {
		t.Fatalf("personal = %+v, want an editable arrangement with its address", plain.Personal)
	}
	if len(plain.Personal.Sections) != len(plain.Items) {
		t.Fatalf("%d entries listed, %d in the menu", len(plain.Personal.Sections), len(plain.Items))
	}
	for i, s := range plain.Personal.Sections {
		if s.ID != plain.Items[i].ID || s.Title != plain.Items[i].Label || s.Hidden {
			t.Errorf("entry %d = %+v, want %s unhidden", i, s, plain.Items[i].ID)
		}
	}

	mine := RenderNavigation(StockNavigation(), pages, libs, ctx(&Personal{Order: []string{"lib-2", "home"}, Hidden: []string{"lib-1", "settings"}}, true))
	if got := mine.Personal.Sections; got[0].ID != "lib-2" || got[1].ID != "home" {
		t.Errorf("order starts %s, %s; want lib-2, home", got[0].ID, got[1].ID)
	}
	var sawHidden, sawSettings bool
	for _, s := range mine.Personal.Sections {
		if s.ID == "lib-1" {
			sawHidden = s.Hidden
		}
		if s.ID == "settings" {
			sawSettings = true
			if s.Hidden {
				t.Error("Settings is marked hidden; it can't be")
			}
		}
	}
	if !sawHidden || !sawSettings {
		t.Errorf("hidden library listed and marked: %v; Settings listed: %v", sawHidden, sawSettings)
	}
	for _, e := range mine.Items {
		if e.ID == "lib-1" {
			t.Error("a hidden entry is still in the menu")
		}
	}
	if len(mine.Personal.Sections) != len(mine.Items)+1 {
		t.Errorf("%d listed, %d shown: want one more listed than shown", len(mine.Personal.Sections), len(mine.Items))
	}

	// A profile that may not rearrange is told nothing to arrange.
	if no := RenderNavigation(StockNavigation(), pages, libs, ctx(&Personal{Hidden: []string{"lib-1"}}, false)); no.Personal != nil {
		t.Errorf("personal = %+v for a profile that may not rearrange", no.Personal)
	}
}
