// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"testing"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// The menu never says a word twice: a module's Books or Photos entry gives way to the library's.
func TestMenuDoesNotListAWordTwice(t *testing.T) {
	items := []sdui.NavEntry{{ID: "home", Label: "Home"}, {ID: "library-3", Label: "Books"}, {ID: "library-4", Label: "Photos"},
		{ID: "wittyink", Label: " books "}, {ID: "wittyphotos", Label: "Photos"}, {ID: "wittystats", Label: "Stats"}, {ID: "settings", Label: "Settings"}}
	personal := &sdui.PersonalInfo{Sections: []sdui.PersonalSection{{ID: "library-3"}, {ID: "wittyink"}, {ID: "wittystats"}}}
	got, p := dedupeModuleEntries(items, personal, map[string]bool{"wittyink": true, "wittyphotos": true, "wittystats": true})
	var ids []string
	for _, e := range got {
		ids = append(ids, e.ID)
	}
	want := []string{"home", "library-3", "library-4", "wittystats", "settings"}
	if len(ids) != len(want) {
		t.Fatalf("the menu: %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("the menu: %v, want %v", ids, want)
		}
	}
	if len(p.Sections) != 2 || p.Sections[1].ID != "wittystats" {
		t.Errorf("the personal list keeps what was dropped: %+v", p.Sections)
	}
	// Two module entries with the same label and no library: both stay (nothing covers them).
	same, _ := dedupeModuleEntries([]sdui.NavEntry{{ID: "a", Label: "X"}, {ID: "b", Label: "X"}}, nil, map[string]bool{"a": true, "b": true})
	if len(same) != 2 {
		t.Errorf("modules' own entries: %v", same)
	}
}
