// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package themes

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	builtin "github.com/wittywilla/wittymediaserver/themes"
)

func palette(t *testing.T, id string) map[string]string {
	t.Helper()
	c, err := (*Manager)(nil).Colors(id)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func merged(base, over map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

func TestDeriveFollowsTheOverriddenAccent(t *testing.T) {
	for _, tc := range []struct {
		theme, accent string
	}{
		{"default", "#3AD29F"}, // a green on a dark theme: dark labels
		{"default", "#D02060"}, // a deep pink: white labels, a lighter accent as text
		{"light", "#D02060"},
		{"light", "#FFCC00"}, // a light yellow on a light theme: a darker accent as text
		{"midnight", "#FF8800"},
		{"contrast", "#00AAFF"},
	} {
		t.Run(tc.theme+tc.accent, func(t *testing.T) {
			base := palette(t, tc.theme)
			d := Derive(base, map[string]string{"accent": tc.accent})
			pal := merged(base, d)
			if d["accentSoft"] != tc.accent+"24" || d["accentLine"] != tc.accent+"73" {
				t.Errorf("tints: %s %s", d["accentSoft"], d["accentLine"])
			}
			if d["focus"] != d["accentText"] || d["brandAccent"] != d["accentText"] || d["accentText"] == "" {
				t.Errorf("focus %s and brandAccent %s should be the accent as text %s", d["focus"], d["brandAccent"], d["accentText"])
			}
			if d["accentHover"] == "" || d["accentHover"] == tc.accent {
				t.Errorf("accentHover: %q", d["accentHover"])
			}
			if issues := CheckContrast(pal, d); len(issues) > 0 {
				t.Errorf("an accent alone should never make text unreadable: %+v", issues)
			}
			if _, ok := d["bg"]; ok {
				t.Error("nothing that doesn't follow the accent is derived")
			}
		})
	}
}

func TestDeriveKeepsWhatIsSetAndWhatIsUnchanged(t *testing.T) {
	base := palette(t, "default")
	// Explicitly set tokens win over derived ones.
	d := Derive(base, map[string]string{"accent": "#3AD29F", "focus": "#FFFFFF", "accentText": "#EEEEEE"})
	if d["focus"] != "#FFFFFF" || d["accentText"] != "#EEEEEE" || d["brandAccent"] != "#EEEEEE" {
		t.Errorf("explicit: %+v", d)
	}
	// Setting a token to what the theme has derives nothing.
	if d := Derive(base, map[string]string{"accent": strings.ToLower(base["accent"])}); len(d) != 1 {
		t.Errorf("unchanged accent derived %+v", d)
	}
	if d := Derive(base, nil); len(d) != 0 {
		t.Errorf("nothing: %+v", d)
	}
	// The high contrast theme's strong hairlines stay strong when the text colour changes.
	c := Derive(palette(t, "contrast"), map[string]string{"fg": "#FFFF00"})
	if c["border"] != "#FFFF0066" || c["hover"] != "#FFFF0012" {
		t.Errorf("contrast hairlines: %s %s", c["border"], c["hover"])
	}
	// New surfaces get outlines that still show on them.
	s := Derive(base, map[string]string{"bg": "#203040", "bg2": "#253545", "bg3": "#2A3A4A"})
	for _, surf := range []string{"#203040", "#253545", "#2A3A4A"} {
		if r, _ := ContrastRatio(s["controlLine"], surf); r < 3 {
			t.Errorf("controlLine %s on %s: %.2f", s["controlLine"], surf, r)
		}
	}
	// The base isn't touched.
	if base["accentSoft"] != palette(t, "default")["accentSoft"] {
		t.Error("base changed")
	}
}

// The derivation reproduces the built-in themes' own rules where they follow them.
func TestDeriveMatchesThePacks(t *testing.T) {
	for _, id := range []string{"default", "light", "paper", "contrast"} {
		base := palette(t, id)
		// Pretend the accent and text came in fresh: blank them in the base.
		blank := merged(base, map[string]string{"accent": "#000001", "fg": "#000001"})
		d := Derive(blank, map[string]string{"accent": base["accent"], "fg": base["fg"]})
		for _, k := range []string{"accentSoft", "accentLine", "border", "borderStrong", "hover", "pressed", "accentFg"} {
			// The high contrast theme has its own strong hairlines and black labels (above).
			if id == "contrast" && (k == "accentFg" || k == "border") {
				continue
			}
			if !strings.EqualFold(d[k], base[k]) {
				t.Errorf("%s %s: derived %s, theme has %s", id, k, d[k], base[k])
			}
		}
		if id != "contrast" && !strings.EqualFold(d["accentText"], base["accentText"]) {
			t.Errorf("%s accentText: derived %s, theme has %s", id, d["accentText"], base["accentText"])
		}
	}
}

func TestEveryPackHasShapeAndType(t *testing.T) {
	dirs, _ := fs.ReadDir(builtin.Builtin, ".")
	n := 0
	for _, d := range dirs {
		raw, err := fs.ReadFile(builtin.Builtin, d.Name()+"/theme.json")
		if err != nil {
			continue
		}
		n++
		var m Meta
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if m.Shape == nil || m.Type == nil {
			t.Errorf("%s: no shape or type in theme.json: run python3 scripts/design-tokens.py", d.Name())
			continue
		}
		// Today every pack has the stylesheet's corners and type: what the web draws.
		if want := ShapeFor(m.Radius); *m.Shape != want {
			t.Errorf("%s shape %+v, want %+v", d.Name(), *m.Shape, want)
		}
		if !reflect.DeepEqual(*m.Type, DefaultType()) {
			t.Errorf("%s type %+v", d.Name(), *m.Type)
		}
		s, ty := (*Manager)(nil).Look(d.Name())
		if s != *m.Shape || !reflect.DeepEqual(ty, *m.Type) {
			t.Errorf("%s: Look doesn't give theme.json's", d.Name())
		}
	}
	if n < 8 {
		t.Fatalf("only %d packs", n)
	}
	// Gone or automatic: the default's.
	if s, _ := (*Manager)(nil).Look("auto"); s != ShapeFor(10) {
		t.Errorf("auto: %+v", s)
	}
}

// themes/tokens.json and ShapeRoles are one table.
func TestShapeRolesMatchTokensJSON(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "themes", "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tk struct {
		Shape struct {
			RadiusMultiple map[string]float64 `json:"radiusMultiple"`
			Pill           float64            `json:"pill"`
			Roles          map[string]string  `json:"roles"`
		} `json:"shape"`
		Focus struct {
			Width, Offset, Scale float64
		} `json:"focus"`
	}
	if err := json.Unmarshal(raw, &tk); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tk.Shape.Roles, ShapeRoles) {
		t.Errorf("tokens.json roles %v, shape.go %v", tk.Shape.Roles, ShapeRoles)
	}
	if !reflect.DeepEqual(tk.Shape.RadiusMultiple, radiusSteps) || tk.Shape.Pill != PillRadius {
		t.Errorf("radius steps %v / %v", tk.Shape.RadiusMultiple, tk.Shape.Pill)
	}
	if d := ShapeFor(10); d.FocusWidth != tk.Focus.Width || d.FocusOffset != tk.Focus.Offset || d.FocusScale != tk.Focus.Scale {
		t.Errorf("focus %+v vs %+v", d, tk.Focus)
	}
}

func TestLookFromCSS(t *testing.T) {
	s, ty := lookFromTokens(parseTokens(`:root { --radius: 4px; --focus-width: 3px; --font: "Nunito", sans-serif; --weight-heading: 700; }`))
	if s.Radius != 4 || s.Card != 4 || s.Badge != 1.6 || s.Dialog != 8 || s.Chip != PillRadius || s.FocusWidth != 3 {
		t.Errorf("shape %+v", s)
	}
	if ty.Body[0] != "Nunito" || ty.Display[0] != "Nunito" || ty.Weights["heading"] != 700 || ty.Mono[0] != "ui-monospace" {
		t.Errorf("type %+v", ty)
	}
}
