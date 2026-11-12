// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package themes

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"

	builtin "github.com/wittywilla/wittymediaserver/themes"
)

// Design 2 (docs/plans/DESIGN-2.md section 2): the parts of the design tokens a client
// gets with the theme. Units and steps are the same for every theme (themes/tokens.json);
// the colours in TV and Elevation are the theme's, with the profile's overrides applied.

// TVColors are the ten-foot colours of a theme (theme.json "tv", from --tv-ring-color,
// --hero-scrim and --tv-text): the focus ring on a screen across the room, how strong the
// scrim behind hero text is, and body text a little stronger for distance.
type TVColors struct {
	RingColor string  `json:"ringColor"`
	HeroScrim float64 `json:"heroScrim"` // 0..1
	TVText    string  `json:"tvText"`
}

// TV is what the response's "tv" holds: the theme's ten-foot colours and the units every
// ten-foot screen uses (1 unit = 1 pt on Apple TV at 1920x1080, 0.5 dp on Android TV).
type TV struct {
	TVColors
	Units json.RawMessage `json:"units"`
}

// Shadow is one layer of a drop shadow, black, as tokens.json has it.
type Shadow struct {
	Y       float64 `json:"y"`
	Blur    float64 `json:"blur"`
	Opacity float64 `json:"opacity"`
}

// ElevationStep is one step of the elevation table: which surface it fills with, which
// line it takes (empty for none), and the shadow (empty for none; Apple TV ignores it).
type ElevationStep struct {
	FillToken string   `json:"fillToken"`
	Fill      string   `json:"fill"`
	LineToken string   `json:"lineToken,omitempty"`
	Line      string   `json:"line,omitempty"`
	Shadow    []Shadow `json:"shadow"`
}

// Density is the spacing table, and which step a device starts with.
type Density struct {
	Default   string                        `json:"default"`
	TVDefault string                        `json:"tvDefault"`
	Steps     map[string]map[string]float64 `json:"steps"`
}

type designData struct {
	TV        json.RawMessage `json:"tv"`
	Density   Density         `json:"density"`
	Elevation map[string]struct {
		Fill   string   `json:"fill"`
		Line   *string  `json:"line"`
		Shadow []Shadow `json:"shadow"`
	} `json:"elevation"`
}

var (
	designOnce sync.Once
	designTok  designData
)

func design() *designData {
	designOnce.Do(func() { _ = json.Unmarshal(builtin.TokensJSON, &designTok) })
	return &designTok
}

// DensityTable is the density steps and defaults.
func DensityTable() Density {
	d := design().Density
	if d.Default == "" {
		d.Default, d.TVDefault = "comfortable", "comfortable"
	}
	steps := make(map[string]map[string]float64, len(d.Steps))
	for k, v := range d.Steps {
		c := make(map[string]float64, len(v))
		for a, b := range v {
			c[a] = b
		}
		steps[k] = c
	}
	d.Steps = steps
	return d
}

// DefaultHeroScrim is the stylesheet's --hero-scrim.
const DefaultHeroScrim = 0.72

// TVFor works a theme's ten-foot colours out for the palette in use. own is the theme's
// own (its theme.json tv block, which may be nil or partial); base is the theme's own
// palette and tokens the palette with overrides applied. Where the palette still has the
// theme's text colour, the theme's own ten-foot colours stand; where an override changed
// it, they are derived again so a profile's colours still get a legible ring and text.
func TVFor(own *TVColors, base, tokens map[string]string) TV {
	var c TVColors
	if own != nil {
		c = *own
	}
	fg := tokens["fg"]
	sameFg := sameColor(fg, base["fg"])
	if c.RingColor == "" || !sameFg {
		c.RingColor = fg
	}
	if c.TVText == "" || !sameFg {
		c.TVText = forDistance(fg, tokens["bg"])
	}
	if c.HeroScrim <= 0 || c.HeroScrim > 1 {
		c.HeroScrim = DefaultHeroScrim
	}
	return TV{TVColors: c, Units: design().TV}
}

// forDistance is a text colour a step stronger against its surface: toward white on dark
// surfaces, toward black on light ones.
func forDistance(fg, bg string) string {
	target := "#FFFFFF"
	if !isDark(bg) {
		target = "#000000"
	}
	if m := mixRGB(fg, target, 0.1); m != "" {
		return m
	}
	return fg
}

func sameColor(a, b string) bool {
	x, ok1 := NormalizeColor(a)
	y, ok2 := NormalizeColor(b)
	return ok1 && ok2 && x == y
}

// ElevationFor resolves the elevation table against a palette.
func ElevationFor(tokens map[string]string) map[string]ElevationStep {
	out := map[string]ElevationStep{}
	d := design()
	keys := make([]string, 0, len(d.Elevation))
	for k := range d.Elevation {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		e := d.Elevation[k]
		step := ElevationStep{FillToken: e.Fill, Fill: tokens[e.Fill], Shadow: append([]Shadow{}, e.Shadow...)}
		if e.Line != nil {
			step.LineToken, step.Line = *e.Line, tokens[*e.Line]
		}
		out[k] = step
	}
	return out
}

// TVColorsOf reads a theme's own ten-foot colours (nil when it has none).
func (m *Manager) TVColorsOf(id string) *TVColors {
	if meta := m.meta(id); meta != nil {
		return meta.TV
	}
	return nil
}

// Finish is how a theme finishes surfaces and controls: the glass recipe (a top and a bottom
// fill, an edge and a shadow colour) and the gloss recipe (five stops for a glossy button:
// highlight, upper body, lower body, glow, edge). Renderers draw them at the level the
// experience's effects say.
type Finish struct {
	Glass *Glass   `json:"glass,omitempty"`
	Gloss []string `json:"gloss,omitempty"`
}

// Glass is one glass recipe.
type Glass struct {
	Hi     string `json:"hi"`
	Lo     string `json:"lo"`
	Edge   string `json:"edge"`
	Shadow string `json:"shadow"`
}

// Scene is the background behind the page and under the menu: ordered layers drawn bottom
// to top, positions in percent of the window. Kinds: gradient (angle in degrees, stops with
// at in percent), radial (a soft light at x, y, w by h), shape (a circle or a hill at x, y
// of size) and image (a file of the theme). On modest hardware and with reduced
// transparency or motion a renderer draws only the first layer.
type Scene struct {
	Layers []map[string]any `json:"layers"`
}

// LookOf is a theme's finish and scene, worked out from its colours when it has none of its
// own.
func (m *Manager) LookOf(id string) (Finish, Scene) {
	meta := m.meta(strings.ToLower(id))
	var f Finish
	var sc Scene
	if meta != nil {
		if meta.Finish != nil {
			f = *meta.Finish
		}
		if meta.Scene != nil {
			sc = *meta.Scene
		}
		c := meta.Colors
		dark := meta.Scheme != "light"
		if f.Glass == nil {
			g := Glass{Hi: "rgba(255,255,255,.9)", Lo: "rgba(255,255,255,.5)", Edge: "#ffffff", Shadow: "rgba(20,120,200,.25)"}
			if dark {
				g = Glass{Hi: "rgba(255,255,255,.22)", Lo: "rgba(255,255,255,.08)", Edge: "rgba(255,255,255,.38)", Shadow: "rgba(0,0,0,.4)"}
			}
			f.Glass = &g
		}
		if len(f.Gloss) == 0 && c["accent"] != "" {
			a := c["accent"]
			f.Gloss = []string{mixRGB(a, "#FFFFFF", .5), a, mixRGB(a, "#000000", .15), mixRGB(a, "#FFFFFF", .2), mixRGB(a, "#000000", .3)}
		}
		if len(sc.Layers) == 0 && c["bg"] != "" {
			sc.Layers = []map[string]any{{"type": "gradient", "angle": 180, "stops": []map[string]any{{"at": 0, "color": c["bg"]}, {"at": 100, "color": c["bg2"]}}}}
		}
	}
	return f, sc
}
