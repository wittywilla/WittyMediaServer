// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "testing"

func TestTVLevelTable(t *testing.T) {
	tab := TVLevels()
	if len(tab.Levels) != 3 || tab.Default != TVFull || tab.Note == "" {
		t.Fatalf("levels %v, default %q, note %q", tab.Levels, tab.Default, tab.Note)
	}
	ids := map[string]bool{}
	for _, s := range tab.Screens {
		if s.ID == "" || s.Title == "" || s.Path == "" || s.Path[0] != '/' {
			t.Errorf("screen %+v needs an id, a title and an address", s)
		}
		if !ValidTVLevel(s.Level) || s.Level == "" {
			t.Errorf("screen %s: level %q", s.ID, s.Level)
		}
		if ids[s.ID] {
			t.Errorf("screen %s is listed twice", s.ID)
		}
		ids[s.ID] = true
	}
	for _, p := range tab.Parts {
		if p.ID == "" || p.Title == "" || !ValidTVLevel(p.Level) || p.Level == "" {
			t.Errorf("part %+v", p)
		}
		if ids[p.ID] {
			t.Errorf("part %s has a screen's id", p.ID)
		}
		ids[p.ID] = true
	}
}

func TestTVScreenFor(t *testing.T) {
	cases := map[string]string{
		"/":                          "",
		"/settings/libraries":        TVDesktopOnly,
		"/settings/libraries/":       TVDesktopOnly,
		"/settings/pages":            TVDesktopOnly,
		"/settings/pages/navigation": TVDesktopOnly,
		"/settings/playback":         TVView,
		"/settings/appearance":       "",
		"/settings/games":            "",
		"/settings/gaming":           "",
		"/controllers":               "",
		"/tv":                        "",
		"/tv/channels":               TVView,
		"/tv/channels/new":           TVDesktopOnly,
		"/tv/channels/12?x=1":        TVDesktopOnly,
		"/tv/sets":                   "",
		"/manage/library/4":          TVDesktopOnly,
		"/markers/9":                 TVDesktopOnly,
		"/markers":                   "",
		"/item/9":                    "",
	}
	for path, want := range cases {
		got := ""
		if s := TVScreenFor(path); s != nil {
			got = s.Level
		}
		if got != want {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}
	if TVPartLevel("appearance.colours") != TVDesktopOnly || TVPartLevel("no.such.part") != TVFull {
		t.Error("part levels")
	}
}

func TestCatalogueCarriesTVLevels(t *testing.T) {
	if len(Catalog().TVLevels) == 0 {
		t.Fatal("the catalogue has no tvLevels")
	}
}
