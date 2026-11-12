// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package themes

import (
	"encoding/json"
	"io/fs"
	"testing"

	builtin "github.com/wittywilla/wittymediaserver/themes"
)

// Every built-in theme carries the TV pass, and its ten-foot text reads across a room.
func TestBuiltinThemesHaveTheTVPass(t *testing.T) {
	dirs, _ := fs.ReadDir(builtin.Builtin, ".")
	n := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		n++
		raw, _ := fs.ReadFile(builtin.Builtin, d.Name()+"/theme.json")
		var meta Meta
		if err := json.Unmarshal(raw, &meta); err != nil || meta.TV == nil {
			t.Fatalf("%s: no tv block (%v)", d.Name(), err)
		}
		tv := meta.TV
		if tv.RingColor == "" || tv.TVText == "" || tv.HeroScrim < 0.5 || tv.HeroScrim > 1 {
			t.Errorf("%s: %+v", d.Name(), tv)
		}
		for _, s := range []string{"bg", "bg2", "bg3"} {
			if r, _ := ContrastRatio(tv.TVText, meta.Colors[s]); r < 7 {
				t.Errorf("%s: tvText on %s is %.2f:1", d.Name(), s, r)
			}
			if r, _ := ContrastRatio(tv.RingColor, meta.Colors[s]); r < 4.5 {
				t.Errorf("%s: ringColor on %s is %.2f:1", d.Name(), s, r)
			}
		}
	}
	if n != 8 {
		t.Errorf("built-in themes: %d", n)
	}
}

func TestTVForDerivesWhereTheTextColourChanged(t *testing.T) {
	base := map[string]string{"fg": "#E8EAF0", "bg": "#0F1115"}
	own := &TVColors{RingColor: "#E8EAF0", TVText: "#EAECF2", HeroScrim: 0.8}
	same := TVFor(own, base, base)
	if same.TVText != "#EAECF2" || same.HeroScrim != 0.8 || len(same.Units) == 0 {
		t.Errorf("unchanged palette keeps the theme's own: %+v", same)
	}
	over := TVFor(own, base, map[string]string{"fg": "#FFCC00", "bg": "#0F1115"})
	if over.RingColor != "#FFCC00" || over.TVText == "#EAECF2" || over.TVText == "" {
		t.Errorf("overridden text colour: %+v", over)
	}
	light := TVFor(nil, map[string]string{"fg": "#222222", "bg": "#F4F5F8"}, map[string]string{"fg": "#222222", "bg": "#F4F5F8"})
	if light.TVText >= "#222222" || light.HeroScrim != DefaultHeroScrim {
		t.Errorf("a light theme's ten-foot text is darker, and a theme with none gets the default scrim: %+v", light)
	}
}

func TestElevationAndDensityTables(t *testing.T) {
	pal := palette(t, "default")
	e := ElevationFor(pal)
	if len(e) != 4 || e["0"].Fill != pal["bg"] || e["1"].Fill != pal["bg2"] || e["1"].Line != pal["border"] || e["0"].LineToken != "" {
		t.Errorf("elevation: %+v", e)
	}
	if len(e["2"].Shadow) == 0 || len(e["1"].Shadow) != 0 {
		t.Errorf("only lifted steps have a shadow: %+v", e)
	}
	d := DensityTable()
	if d.Default != "comfortable" || d.Steps["compact"]["space"] >= 1 || d.Steps["spacious"]["control"] <= 1 {
		t.Errorf("density: %+v", d)
	}
	var tv map[string]any
	if err := json.Unmarshal(TVFor(nil, pal, pal).Units, &tv); err != nil || tv["safe"] == nil || tv["dock"] == nil {
		t.Errorf("units: %v %v", tv, err)
	}
}
