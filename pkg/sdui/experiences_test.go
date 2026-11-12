// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"encoding/json"
	"strings"
	"testing"
)

// The experiences are data files: each must be whole, use known menu styles and name a
// WittyTV look that exists.
func TestExperienceFilesAreWhole(t *testing.T) {
	if len(Experiences) != 14 || Experiences[0].ID != ExpLumen {
		t.Fatalf("%d experiences, first %q", len(Experiences), Experiences[0].ID)
	}
	var tv []struct{ ID string }
	if err := json.Unmarshal(Catalog().TVThemes, &tv); err != nil || len(tv) != 8 {
		t.Fatalf("WittyTV looks: %d %v", len(tv), err)
	}
	looks := map[string]bool{}
	for _, l := range tv {
		looks[l.ID] = true
	}
	seen := map[string]bool{}
	for _, e := range Experiences {
		if seen[e.ID] || e.Name == "" || e.Theme == "" || e.Description == "" || e.Preview.BG == "" || !contains(Icons, e.Icon) {
			t.Errorf("%s: %+v", e.ID, e)
		}
		seen[e.ID] = true
		if !looks[e.TVTheme] {
			t.Errorf("%s suggests a WittyTV look, %q, that isn't one", e.ID, e.TVTheme)
		}
		for _, ff := range FormFactors {
			nav := e.Nav[ff]
			if nav == "" || !contains(NavStyles, nav) {
				t.Errorf("%s on %s: menu %q", e.ID, ff, nav)
			}
			if navSizes[nav][ff] == 0 && navSizes[nav][FormWeb] == 0 {
				t.Errorf("%s on %s: no size for %s", e.ID, ff, nav)
			}
		}
		for _, v := range []string{e.Effects.Glass, e.Effects.Gloss, e.Effects.Glow, e.Effects.Texture, e.Effects.Bevel} {
			if v != FxNone && v != FxSubtle && v != FxFull {
				t.Errorf("%s: effect level %q", e.ID, v)
			}
		}
		if !contains([]string{"system", "humanist", "rounded", "mono", "serif"}, e.Type) || !contains(Densities, e.Density) {
			t.Errorf("%s: type %q density %q", e.ID, e.Type, e.Density)
		}
		if sh := ShellFor(e.ID, FormTV); sh.Experience != e.ID || sh.Size == 0 {
			t.Errorf("%s: shell %+v", e.ID, sh)
		}
	}
	if sh := ShellFor("rail", FormDesktop); sh.Nav != NavRailDrawer || sh.Open != 240 || sh.Drawer != 360 {
		t.Errorf("the rail's states: %+v", sh)
	}
	if sh := ShellFor("rail", FormTV); sh.Nav != NavTVSideMenu || sh.Open != 400 || sh.Drawer != 480 {
		t.Errorf("the rail on a television: %+v", sh)
	}
}

// Every category of Crossbar's Home has a name of its own: a row repeated over libraries or pinned
// views has "{each}" in its title, which would make a blank tab, so the tab is named for what the
// copies are. A page whose Tab has no label is refused by the validator.
func TestCrossbarTabsAreAllLabelled(t *testing.T) {
	home, _ := StockPage("home")
	e := ExperienceByID("crossbar")
	if e == nil {
		t.Fatal("no crossbar experience")
	}
	p := styled(home, e)
	var bar *Section
	for i := range p.Sections {
		if p.Sections[i].Component == "CrossBar" {
			bar = &p.Sections[i]
		}
	}
	if bar == nil || len(bar.Children) < 3 {
		t.Fatalf("Home in Crossbar has no bar of categories: %+v", p.Sections)
	}
	seen := map[string]bool{}
	for _, tab := range bar.Children {
		label := propStr(tab.Props, "label", "")
		if label == "" || strings.Contains(label, "{") || seen[label] {
			t.Errorf("tab %s has the label %q", tab.ID, label)
		}
		seen[label] = true
	}
	if !seen["Saved views"] || !seen["Recently added"] {
		t.Errorf("the repeated rows should be named for what they are: %v", seen)
	}
	if issues := ValidatePage(&p); HasErrors(issues) {
		t.Errorf("the restructured Home doesn't validate: %v", issues)
	}
}

func TestValidatorRefusesATabWithoutALabel(t *testing.T) {
	for _, label := range []string{"", "   ", "{each}"} {
		p := Page{ID: "x", Kind: "custom", Title: "X", Sections: []Section{{ID: "bar", Component: "CrossBar", Children: []Section{
			{ID: "a", Component: "Tab", Props: map[string]any{"label": label}, Children: []Section{{ID: "t", Component: "Text", Props: map[string]any{"text": "hi"}}}}}}}}
		if !HasErrors(ValidatePage(&p)) {
			t.Errorf("a tab labelled %q passed", label)
		}
	}
}

// The layout a picker draws (Wire) is what the design canvas's wire thumbnails show for each look
// on a desktop window: where the menu is, the shape of Home, the cards, the surface.
func TestExperienceWireMatchesTheCanvas(t *testing.T) {
	want := map[string]Wire{
		"lumen":            {Nav: "top", Style: "pill", Home: "hero", Cards: "poster", Brand: "left", Finish: "flat"},
		"rail":             {Nav: "rail", Style: "pill", Home: "banner", Cards: "plate", Brand: "top", Finish: "flat"},
		"stage":            {Nav: "top", Style: "pill", Home: "hero", Cards: "landscape", Brand: "left", Finish: "flat"},
		"aero":             {Nav: "top", Style: "gloss", Home: "hero", Cards: "poster", Brand: "left", Finish: "gloss"},
		"classic":          {Nav: "side", Open: true, Style: "pill", Home: "hero", Cards: "poster", Brand: "top", Finish: "flat"},
		"classicCable":     {Nav: "top", Style: "cable", Home: "list", Cards: "landscape", Brand: "left", Finish: "flat"},
		"classicSatellite": {Nav: "top", Style: "bevel", Home: "hero", Cards: "poster", Brand: "left", Finish: "bevel"},
		"paper":            {Nav: "top", Style: "under", Home: "banner", Cards: "poster", Brand: "left", Finish: "flat"},
		"marquee":          {Nav: "top", Style: "pill", Home: "carousel", Cards: "poster", Brand: "left", Finish: "flat"},
		"crossbar":         {Nav: "none", Style: "crossbar", Home: "cross", Cards: "landscape", Brand: "centre", Finish: "flat"},
		"bigpicture":       {Nav: "top", Style: "pill", Home: "tiles", Cards: "landscape", Brand: "left", Finish: "flat"},
		"retro":            {Nav: "top", Style: "tele", Home: "list", Cards: "block", Brand: "left", Finish: "crt"},
		// Compact and Dashboard: a rail on a desktop window, the side menu on a television.
		"compact":   {Nav: "rail", Style: "pill", Home: "list", Cards: "poster", Brand: "top", Finish: "flat"},
		"dashboard": {Nav: "rail", Style: "pill", Home: "widgets", Cards: "landscape", Brand: "top", Finish: "flat"},
	}
	for _, e := range Experiences {
		w, ok := want[e.ID]
		if !ok {
			t.Errorf("%s: no wire expected", e.ID)
			continue
		}
		if got := e.Wire[FormDesktop]; got != w {
			t.Errorf("%s on a desktop: %+v, want %+v", e.ID, got, w)
		}
		if len(e.Wire) != len(FormFactors) {
			t.Errorf("%s: wire for %d form factors", e.ID, len(e.Wire))
		}
	}
	if w := ExperienceByID("compact").Wire[FormTV]; w.Nav != "side" || !w.Dock {
		t.Errorf("compact on a television: %+v", w)
	}
}
