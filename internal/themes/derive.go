// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package themes

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// A theme sets a handful of colours and the rest are worked out from them: the hairlines
// from the text colour, the selected-row tint from the accent, the accent as text from the
// accent and the surfaces it must be read on (web/src/styles.css, scripts/theme-contrast.py).
// Derive does the same for a set of overrides, so a module or a profile that sets only
// "accent" gets a hover, a tint, a focus ring and a name-in-the-corner colour that go with
// it instead of the theme's old ones.

// tint is a colour derived as another one, see-through: the same channels, with an alpha.
type tint struct {
	from  string
	alpha int // 0..255, used when the base theme doesn't show its own
}

// tints are the see-through shades, as the web's stylesheet mixes them.
var tints = map[string]tint{
	"border":       {"fg", 0x1A}, // color-mix(fg 10%, transparent)
	"borderStrong": {"fg", 0x33}, // 20%
	"hover":        {"fg", 0x12}, // 7%
	"pressed":      {"fg", 0x1F}, // 12%
	"accentSoft":   {"accent", 0x24},
	"accentLine":   {"accent", 0x73},
	"okSoft":       {"ok", 0x21},
	"warnSoft":     {"warn", 0x21},
	"dangerSoft":   {"danger", 0x1C},
}

// DerivedFrom lists, for each token worked out from others, the tokens it comes from.
var DerivedFrom = map[string][]string{
	"border": {"fg"}, "borderStrong": {"fg"}, "hover": {"fg"}, "pressed": {"fg"},
	"accentSoft": {"accent"}, "accentLine": {"accent"}, "accentHover": {"accent"}, "accentFg": {"accent"},
	"accentText": {"accent"}, "focus": {"accentText"}, "brandAccent": {"accentText"},
	"okSoft": {"ok"}, "warnSoft": {"warn"}, "dangerSoft": {"danger"},
	"controlLine": {"bg", "bg2", "bg3"},
}

// ink is the dark label colour light accents get on dark themes (scripts/theme-contrast.py).
const ink = "#0E1013"

// Derive returns over with the tokens worked out from it added: every token in DerivedFrom
// whose sources over changes (sets to something other than base has) and that over
// doesn't set itself. base is the palette over
// is laid on (the theme with the layers under this one); it gives the surfaces the accent
// as text must be readable on, and how see-through the theme makes each shade. The result
// is a new map; over is not changed.
func Derive(base, over map[string]string) map[string]string {
	out := make(map[string]string, len(over)+8)
	for k, v := range over {
		out[k] = v
	}
	if len(over) == 0 {
		return out
	}
	pal := make(map[string]string, len(base)+len(out))
	for k, v := range base {
		pal[k] = v
	}
	// A token set to what it already is changes nothing, and derives nothing.
	changed := func(k string) bool {
		v, ok := out[k]
		if !ok {
			return false
		}
		a, okA := NormalizeColor(v)
		b, okB := NormalizeColor(base[k])
		return !okA || !okB || a != b
	}
	set := func(k, v string) {
		if _, own := over[k]; own || v == "" {
			return
		}
		out[k] = v
		pal[k] = v
	}
	for k, v := range out {
		pal[k] = v
	}
	// Order matters: the accent as text before what follows it.
	if changed("accent") {
		accent := pal["accent"]
		set("accentHover", mixRGB(accent, "#000000", 0.12))
		set("accentFg", labelOn(accent))
		set("accentText", readableOn(accent, surfaces(pal), ContrastAA, isDark(pal["bg"])))
	}
	if changed("accentText") {
		set("focus", pal["accentText"])
		set("brandAccent", pal["accentText"])
	}
	for name, t := range tints {
		if changed(t.from) {
			set(name, withAlpha(pal[t.from], alphaOf(base, name, t)))
		}
	}
	// Outlines of fields: 3:1 on every surface, and no stronger than that needs, as
	// theme-contrast.py makes them: the raised surface, moved until it reads.
	if changed("bg") || changed("bg2") || changed("bg3") {
		set("controlLine", readableOn(pal["bg3"], surfaces(pal), 3, isDark(pal["bg"])))
	}
	return out
}

// alphaOf is how see-through the base theme makes a shade of its source colour, when it
// is one (the high contrast theme's hairlines are stronger); else the stylesheet's.
func alphaOf(base map[string]string, name string, t tint) int {
	b, ok1 := NormalizeColor(base[name])
	src, ok2 := NormalizeColor(base[t.from])
	if ok1 && ok2 && len(b) == 9 && strings.EqualFold(b[:7], src[:7]) {
		if a, err := strconv.ParseUint(b[7:], 16, 8); err == nil {
			return int(a)
		}
	}
	return t.alpha
}

func surfaces(pal map[string]string) []string {
	var out []string
	for _, k := range []string{"bg", "bg2", "bg3"} {
		if _, ok := opaqueRGB(pal[k]); ok {
			out = append(out, pal[k])
		}
	}
	return out
}

func isDark(bg string) bool {
	c, ok := opaqueRGB(bg)
	return !ok || luminance(c) < 0.2
}

// withAlpha is c's channels with alpha a; "" when c isn't a colour.
func withAlpha(c string, a int) string {
	n, ok := NormalizeColor(c)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s%02X", n[:7], a)
}

// mixRGB is a mixed with b, t of the way to b, opaque; "" unless both are colours.
func mixRGB(a, b string, t float64) string {
	ca, ok1 := rgbOf(a)
	cb, ok2 := rgbOf(b)
	if !ok1 || !ok2 {
		return ""
	}
	var out [3]float64
	for i := range out {
		out[i] = ca[i]*(1-t) + cb[i]*t
	}
	return hexOf(out)
}

// labelOn is the text colour for a label on a filled colour: white when it reads, the
// dark ink when that does (gold, green), else whichever reads better.
func labelOn(fill string) string {
	if _, ok := opaqueRGB(fill); !ok {
		return ""
	}
	w, _ := ContrastRatio("#FFFFFF", fill)
	if w >= ContrastAA {
		return "#FFFFFF"
	}
	if k, _ := ContrastRatio(ink, fill); k >= ContrastAA || k > w {
		return ink
	}
	return "#FFFFFF"
}

// readableOn is c moved in lightness, keeping its hue, until it reaches need on every
// surface: lighter on a dark theme, darker on a light one. The way theme-contrast.py
// makes the built-in themes' accent as text.
func readableOn(c string, on []string, need float64, lighter bool) string {
	rgb, ok := rgbOf(c)
	if !ok {
		return ""
	}
	h, l, s := toHLS(rgb)
	cur := hexOf(rgb)
	for range 200 {
		cur = hexOf(fromHLS(h, l, s))
		good := true
		for _, x := range on {
			if r, ok := ContrastRatio(cur, x); ok && r < need {
				good = false
				break
			}
		}
		if good {
			return cur
		}
		if lighter {
			l = math.Min(1, l+0.005)
		} else {
			l = math.Max(0, l-0.005)
		}
	}
	return cur
}

// rgbOf reads a colour's channels as 0..1, ignoring any alpha.
func rgbOf(c string) ([3]float64, bool) {
	n, ok := NormalizeColor(c)
	if !ok {
		return [3]float64{}, false
	}
	var out [3]float64
	for i := range out {
		v, err := strconv.ParseUint(n[1+2*i:3+2*i], 16, 8)
		if err != nil {
			return out, false
		}
		out[i] = float64(v) / 255
	}
	return out, true
}

func hexOf(c [3]float64) string {
	var b strings.Builder
	b.WriteByte('#')
	for _, v := range c {
		fmt.Fprintf(&b, "%02X", int(math.Round(math.Max(0, math.Min(1, v))*255)))
	}
	return b.String()
}

// toHLS and fromHLS are Python's colorsys conversions, so a colour is moved exactly as
// scripts/theme-contrast.py moves it.
func toHLS(c [3]float64) (h, l, s float64) {
	r, g, b := c[0], c[1], c[2]
	maxc, minc := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (minc + maxc) / 2
	if maxc == minc {
		return 0, l, 0
	}
	if l <= 0.5 {
		s = (maxc - minc) / (maxc + minc)
	} else {
		s = (maxc - minc) / (2 - maxc - minc)
	}
	rc, gc, bc := (maxc-r)/(maxc-minc), (maxc-g)/(maxc-minc), (maxc-b)/(maxc-minc)
	switch {
	case r == maxc:
		h = bc - gc
	case g == maxc:
		h = 2 + rc - bc
	default:
		h = 4 + gc - rc
	}
	h = math.Mod(h/6, 1)
	if h < 0 {
		h++
	}
	return h, l, s
}

func fromHLS(h, l, s float64) [3]float64 {
	if s == 0 {
		return [3]float64{l, l, l}
	}
	var m2 float64
	if l <= 0.5 {
		m2 = l * (1 + s)
	} else {
		m2 = l + s - l*s
	}
	m1 := 2*l - m2
	v := func(hue float64) float64 {
		hue = math.Mod(hue, 1)
		if hue < 0 {
			hue++
		}
		switch {
		case hue < 1.0/6:
			return m1 + (m2-m1)*hue*6
		case hue < 0.5:
			return m2
		case hue < 2.0/3:
			return m1 + (m2-m1)*(2.0/3-hue)*6
		}
		return m1
	}
	return [3]float64{v(h + 1.0/3), v(h), v(h - 1.0/3)}
}
