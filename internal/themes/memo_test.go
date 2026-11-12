// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package themes

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// What is kept of a theme follows its file: a theme made again is read again, a caller
// can't change what the next one gets, and a theme removed is gone.
func TestThemesAreKeptOnlyWhileTheirFileIsTheSame(t *testing.T) {
	m := &Manager{Dir: t.TempDir()}
	write := func(accent string) {
		t.Helper()
		colors := map[string]string{}
		for _, k := range customVars {
			colors[k] = "#202020"
		}
		colors["fg"], colors["accent"] = "#f0f0f0", accent
		if _, err := m.AddCustom(Custom{Name: "Mine", Scheme: "dark", Colors: colors, Radius: 8}); err != nil {
			t.Fatal(err)
		}
	}
	write("#3366cc")
	first, err := m.Tokens("custom-mine")
	if err != nil {
		t.Fatal(err)
	}
	if first.Colors["accent"] != "#3366cc" && first.Colors["accent"] != "#3366CC" {
		t.Fatalf("accent %q", first.Colors["accent"])
	}
	first.Colors["accent"] = "#000000" // a caller's change stays its own
	again, _ := m.Tokens("custom-mine")
	if again.Colors["accent"] == "#000000" {
		t.Fatal("a caller changed what is kept")
	}
	// The same theme made again, with another accent: a new file, read again.
	path := filepath.Join(m.Dir, "custom-mine.zip")
	before, _ := os.Stat(path)
	write("#cc3366")
	_ = os.Chtimes(path, time.Now(), before.ModTime().Add(2*time.Second))
	changed, _ := m.Tokens("custom-mine")
	if changed.Colors["accent"] != "#cc3366" && changed.Colors["accent"] != "#CC3366" {
		t.Fatalf("after the theme changed: %q", changed.Colors["accent"])
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Tokens("custom-mine"); err == nil {
		t.Fatal("a removed theme is still there")
	}
}
