// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "testing"

// The button an empty section offers is checked: a label, and exactly one place to go or command to run.
func TestEmptyActionIsValidated(t *testing.T) {
	good := map[string]any{"label": "Scan now", "op": "library.scan", "item": "$library", "when": map[string]any{"admin": true}}
	p := Page{ID: "x", Kind: "library", Title: "X", Sections: []Section{{ID: "none", Component: "Grid", Props: map[string]any{"empty": "message", "emptyHint": "Add files.", "emptyAction": good}, Source: Source{"type": "recentlyAdded", "library": "$library"}}}}
	if issues := ValidatePage(&p); HasErrors(issues) {
		t.Fatalf("a good button: %v", issues)
	}
	for name, bad := range map[string]map[string]any{
		"no label": {"native": "settings"}, "outside address": {"label": "Go", "to": "https://x.example"},
		"two places": {"label": "Go", "to": "/a", "native": "settings"}, "unknown command": {"label": "Go", "op": "nothing.here"}, "none": {"label": "Go"},
	} {
		p.Sections[0].Props["emptyAction"] = bad
		if !HasErrors(ValidatePage(&p)) {
			t.Errorf("%s passed", name)
		}
	}
}
