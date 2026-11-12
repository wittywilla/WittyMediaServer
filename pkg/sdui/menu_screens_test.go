// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"strings"
	"testing"
)

// The catalogue says which native screens a menu entry may open, and the validator accepts
// exactly those: an editor that offers the catalogue's choices never gets a refusal.
func TestMenuScreensAgreeWithTheValidator(t *testing.T) {
	cases := []struct {
		screen string
		menu   bool
	}{
		{"settings", true}, {"guide", true}, {"profiles", true}, {"downloads", true}, {"remote", true}, {"nowPlaying", true}, {"pair", true}, {"collections", true},
		{"player", false}, {"livePlayer", false}, {"reader", false}, {"game", false}, {"download", false}, {"share", false}, {"playOn", false}, {"watchParty", false},
	}
	cat := Catalog()
	if len(cases) != len(cat.NativeScreens) || len(cat.NativeScreens) != len(NativeScreens) {
		t.Fatalf("%d cases, %d screens in the catalogue, %d in NativeScreens: add the new screen here", len(cases), len(cat.NativeScreens), len(NativeScreens))
	}
	for _, tc := range cases {
		t.Run(tc.screen, func(t *testing.T) {
			spec := nativeScreen(tc.screen)
			if spec == nil {
				t.Fatal("not in the catalogue")
			}
			if spec.Menu != tc.menu {
				t.Errorf("menu %v, want %v", spec.Menu, tc.menu)
			}
			if spec.Menu && (spec.Route == "" || strings.Contains(spec.Route, "{")) {
				t.Errorf("a menu screen needs a route of its own with nothing to fill in: %q", spec.Route)
			}
			nav := Navigation{Items: []NavItem{{ID: "x", Type: "native", Screen: tc.screen}}}
			if got := !HasErrors(ValidateNavigation(&nav, nil)); got != tc.menu {
				t.Errorf("the validator accepts it as a menu entry: %v, want %v", got, tc.menu)
			}
		})
	}
}
