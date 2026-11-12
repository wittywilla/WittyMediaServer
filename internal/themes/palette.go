// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package themes

import (
	"encoding/json"
	"io/fs"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	builtin "github.com/wittywilla/wittymediaserver/themes"
)

// colorTokens is every colour name the built-in themes give in theme.json
// "colors" (bg, bg2, fg, accent, accentFg, …): the names a colour override may use.
var colorTokens = sync.OnceValue(func() map[string]bool {
	out := map[string]bool{}
	dirs, _ := fs.ReadDir(builtin.Builtin, ".")
	for _, d := range dirs {
		raw, err := fs.ReadFile(builtin.Builtin, d.Name()+"/theme.json")
		if err != nil {
			continue
		}
		var m Meta
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		for k := range m.Colors {
			out[k] = true
		}
	}
	return out
})

// ColorTokens lists the colour token names themes know, sorted.
func ColorTokens() []string {
	out := make([]string, 0, len(colorTokens()))
	for k := range colorTokens() {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// IsColorToken reports whether name is one of ColorTokens.
func IsColorToken(name string) bool { return colorTokens()[name] }

// Colors is a theme's colours by token name (the theme.json "colors" names),
// normalised to #RRGGBB or #RRGGBBAA. Themes without them in theme.json (older
// uploads) are read from their CSS. A built-in theme not yet unpacked into the
// themes folder is read from the copy built into the server.
func (m *Manager) Colors(id string) (map[string]string, error) {
	id = strings.ToLower(id)
	if m != nil {
		for _, meta := range m.List() {
			if meta.ID == id && len(meta.Colors) > 0 {
				return normalized(meta.Colors), nil
			}
		}
		if t, err := m.Tokens(id); err == nil {
			out := make(map[string]string, len(t.Colors))
			for k, v := range t.Colors {
				out[camel(k)] = v
			}
			return out, nil
		}
	}
	if !idRe.MatchString(id) {
		return nil, ErrNotFound
	}
	raw, err := fs.ReadFile(builtin.Builtin, id+"/theme.json")
	if err != nil {
		return nil, ErrNotFound
	}
	var meta Meta
	if err := json.Unmarshal(raw, &meta); err != nil || len(meta.Colors) == 0 {
		return nil, ErrNotFound
	}
	return normalized(meta.Colors), nil
}

func normalized(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		if c, ok := NormalizeColor(v); ok {
			out[k] = c
		}
	}
	return out
}

// camel turns a CSS variable name into its theme.json name: bg-2 → bg2, accent-fg → accentFg.
func camel(s string) string {
	parts := strings.Split(s, "-")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// ---- WCAG 2.2 contrast, as scripts/theme-contrast.py checks it ----

// ContrastAA is the WCAG AA ratio for normal text.
const ContrastAA = 4.5

// opaqueRGB reads #RRGGBB, or #RRGGBBAA when fully opaque, as 0..1 channels.
func opaqueRGB(c string) ([3]float64, bool) {
	n, ok := NormalizeColor(c)
	if !ok || (len(n) == 9 && !strings.EqualFold(n[7:], "FF")) {
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

func luminance(c [3]float64) float64 {
	f := func(v float64) float64 {
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c[0]) + 0.7152*f(c[1]) + 0.0722*f(c[2])
}

// ContrastRatio is the WCAG contrast ratio of two opaque colours (1 to 21).
// ok is false when either isn't an opaque colour.
func ContrastRatio(a, b string) (ratio float64, ok bool) {
	ca, okA := opaqueRGB(a)
	cb, okB := opaqueRGB(b)
	if !okA || !okB {
		return 0, false
	}
	la, lb := luminance(ca), luminance(cb)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05), true
}

// ContrastIssue is text that falls short of AA on a surface.
type ContrastIssue struct {
	Text    string  `json:"text"`    // token used as text
	Surface string  `json:"surface"` // token it sits on
	Ratio   float64 `json:"ratio"`   // rounded to two places
	Need    float64 `json:"need"`
}

// contrastPairs are the pairs scripts/theme-contrast.py holds the built-in themes to, and
// the name in the corner (brandAccent), which they all take from the accent as text.
var contrastPairs = func() [][2]string {
	var out [][2]string
	surfaces := []string{"bg", "bg2", "bg3"}
	for _, t := range []string{"fg", "fg2", "fg3", "accentText", "brandAccent", "danger", "ok", "warn"} {
		for _, s := range surfaces {
			out = append(out, [2]string{t, s})
		}
	}
	return append(out, [2]string{"accentFg", "accent"})
}()

// CheckContrast lists the text/surface pairs in colors below AA. Pairs with a
// missing or translucent colour are skipped. When only is non-nil, a pair is
// reported only if one of its tokens is in it (the tokens an override changed).
func CheckContrast(colors map[string]string, only map[string]string) []ContrastIssue {
	var out []ContrastIssue
	for _, p := range contrastPairs {
		if only != nil {
			_, a := only[p[0]]
			_, b := only[p[1]]
			if !a && !b {
				continue
			}
		}
		r, ok := ContrastRatio(colors[p[0]], colors[p[1]])
		if ok && r < ContrastAA {
			out = append(out, ContrastIssue{Text: p[0], Surface: p[1], Ratio: math.Round(r*100) / 100, Need: ContrastAA})
		}
	}
	return out
}
