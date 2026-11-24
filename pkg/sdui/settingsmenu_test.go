// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"encoding/json"
	"testing"
)

func TestSettingsMenuIsWhole(t *testing.T) {
	m := SettingsMenuData()
	for _, p := range m.Problems() {
		t.Error(p)
	}
	var groups []string
	for _, g := range m.Groups {
		groups = append(groups, g.ID)
	}
	// The Android phone app's order (WittyMediaClient YouRules.kt), which every client keeps.
	want := []string{"account", "household", "yours", "modules", "device", "server", "maintenance", "help"}
	if len(groups) != len(want) {
		t.Fatalf("groups %v, want %v", groups, want)
	}
	for i := range want {
		if groups[i] != want[i] {
			t.Fatalf("groups %v, want %v", groups, want)
		}
	}
}

func TestSettingsMenuTVLevels(t *testing.T) {
	levels := map[string]string{}
	for _, g := range SettingsMenuData().Groups {
		for _, r := range g.Rows {
			levels[r.ID] = SettingsRowLevel(r)
		}
	}
	for id, want := range map[string]string{
		"libraries": TVDesktopOnly, "remote": TVDesktopOnly, "network": TVDesktopOnly, "dataFolder": TVDesktopOnly,
		"nameAndLogo": TVDesktopOnly, "editLayout": TVDesktopOnly, "logs": TVDesktopOnly, "reset": TVDesktopOnly,
		"playback": TVView, "names": TVView, "shareLinks": TVView,
		"quality": TVFull, "theme": TVFull, "help": TVFull, "servers": TVFull,
	} {
		if levels[id] != want {
			t.Errorf("%s: level %q, want %q", id, levels[id], want)
		}
	}
}

func TestCatalogueCarriesTheSettingsMenu(t *testing.T) {
	var m SettingsMenu
	if err := json.Unmarshal(Catalog().SettingsMenu, &m); err != nil || len(m.Groups) == 0 {
		t.Fatalf("settingsMenu in the catalogue: %v (%d groups)", err, len(m.Groups))
	}
}
