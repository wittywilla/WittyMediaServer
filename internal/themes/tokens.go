package themes

import (
	"archive/zip"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Tokens is a theme as data, for clients that aren't browsers: the values of
// its CSS variables, read and normalised. theme.css stays the one source, so
// a theme author writes nothing extra for the TV apps to get the same colours.
// Anything a theme does beyond its variables (its own selectors, images) only
// exists in the web interface; WebOnly says there is such a thing.
type Tokens struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Scheme string `json:"scheme"` // dark | light
	// Colors are #RRGGBB or #RRGGBBAA, by variable name without the dashes.
	Colors map[string]string `json:"colors"`
	// Lengths are in pixels.
	Lengths map[string]float64 `json:"lengths"`
	// Fonts lists the theme's font families, best first.
	Fonts []string `json:"fonts"`
	// Other holds every remaining value as written (shadows, gradients).
	Other   map[string]string `json:"other"`
	WebOnly bool              `json:"webOnly"`
}

// Tokens reads a theme's variables.
func (m *Manager) Tokens(id string) (*Tokens, error) {
	p, err := m.zipPath(id)
	if err != nil {
		return nil, err
	}
	meta, err := m.readMeta(p)
	if err != nil {
		return nil, err
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	css, err := readZipFile(&zr.Reader, "theme.css")
	if err != nil {
		return nil, err
	}
	t := parseTokens(string(css))
	t.ID, t.Name, t.Scheme = strings.ToLower(id), meta.Name, meta.Scheme
	if t.Name == "" {
		t.Name = t.ID
	}
	if t.Scheme != "light" {
		t.Scheme = "dark"
	}
	return t, nil
}

var (
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	rootBlock  = regexp.MustCompile(`(?s):root\s*\{(.*?)\}`)
	cssVar     = regexp.MustCompile(`(?s)--([a-zA-Z0-9-]+)\s*:\s*(.*?)\s*(?:;|$)`)
	varRef     = regexp.MustCompile(`^var\(\s*--([a-zA-Z0-9-]+)\s*(?:,\s*(.*))?\)$`)
	hexColor   = regexp.MustCompile(`^#([0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	rgbColor   = regexp.MustCompile(`^rgba?\(\s*([0-9.]+)[\s,]+([0-9.]+)[\s,]+([0-9.]+)(?:[\s,/]+([0-9.]+%?))?\s*\)$`)
	lengthVal  = regexp.MustCompile(`^(-?[0-9.]+)(px|rem|em)?$`)
)

func parseTokens(css string) *Tokens {
	css = cssComment.ReplaceAllString(css, "")
	vars := map[string]string{}
	for _, block := range rootBlock.FindAllStringSubmatch(css, -1) {
		for _, m := range cssVar.FindAllStringSubmatch(block[1], -1) {
			vars[m[1]] = m[2]
		}
	}
	t := &Tokens{Colors: map[string]string{}, Lengths: map[string]float64{}, Fonts: []string{}, Other: map[string]string{}}
	t.WebOnly = strings.TrimSpace(rootBlock.ReplaceAllString(css, "")) != ""
	for name := range vars {
		v := mixedColor(vars, resolveVar(vars, name, 0))
		if strings.HasSuffix(name, "-rgb") {
			continue // the same colour again, for the browser's translucent shades
		}
		switch {
		case name == "font" || strings.HasPrefix(name, "font-"):
			if name == "font" {
				t.Fonts = fontFamilies(v)
			} else {
				t.Other[name] = v
			}
		default:
			if c, ok := NormalizeColor(v); ok {
				t.Colors[name] = c
			} else if n, ok := lengthPx(v); ok {
				t.Lengths[name] = n
			} else if v != "" {
				t.Other[name] = v
			}
		}
	}
	return t
}

var colorMix = regexp.MustCompile(`^color-mix\(in srgb,\s*(var\(--[\w-]+\)|#[0-9a-fA-F]{3,8}|[a-zA-Z]+)\s+([0-9.]+)%,\s*transparent\)$`)

// mixedColor turns a colour mixed with transparent, the way themes write their
// hairlines, into the same colour with an alpha value native apps can read.
func mixedColor(vars map[string]string, v string) string {
	m := colorMix.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return v
	}
	base := m[1]
	if strings.HasPrefix(base, "var(--") {
		base = resolveVar(vars, strings.TrimSuffix(strings.TrimPrefix(base, "var(--"), ")"), 0)
	}
	c, ok := NormalizeColor(base)
	pct, err := strconv.ParseFloat(m[2], 64)
	if !ok || err != nil || pct > 100 || len(c) != 7 {
		return v
	}
	return fmt.Sprintf("%s%02X", c, int(pct*255/100+0.5))
}

// resolveVar follows var(--x) references, a few steps deep at most.
func resolveVar(vars map[string]string, name string, depth int) string {
	v := vars[name]
	if depth > 8 {
		return ""
	}
	if m := varRef.FindStringSubmatch(v); m != nil {
		if _, ok := vars[m[1]]; ok {
			return resolveVar(vars, m[1], depth+1)
		}
		return m[2]
	}
	return v
}

func fontFamilies(v string) []string {
	out := []string{}
	for _, f := range strings.Split(v, ",") {
		if f = strings.Trim(strings.TrimSpace(f), `"'`); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func lengthPx(v string) (float64, bool) {
	m := lengthVal.FindStringSubmatch(v)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	if m[2] == "rem" || m[2] == "em" {
		n *= 16
	}
	return n, true
}

// NormalizeColor turns the colour forms themes use into #RRGGBB or #RRGGBBAA.
func NormalizeColor(v string) (string, bool) {
	v = strings.TrimSpace(v)
	switch strings.ToLower(v) {
	case "transparent":
		return "#00000000", true
	case "black":
		return "#000000", true
	case "white":
		return "#FFFFFF", true
	}
	if hexColor.MatchString(v) {
		h := strings.ToUpper(v[1:])
		if len(h) <= 4 { // #abc -> #AABBCC
			var b strings.Builder
			for _, c := range h {
				b.WriteRune(c)
				b.WriteRune(c)
			}
			h = b.String()
		}
		return "#" + h, true
	}
	m := rgbColor.FindStringSubmatch(v)
	if m == nil {
		return "", false
	}
	var ch [3]int
	for i := range ch {
		f, err := strconv.ParseFloat(m[i+1], 64)
		if err != nil || f > 255 {
			return "", false
		}
		ch[i] = int(math.Round(f))
	}
	out := fmt.Sprintf("#%02X%02X%02X", ch[0], ch[1], ch[2])
	if m[4] != "" {
		a, err := strconv.ParseFloat(strings.TrimSuffix(m[4], "%"), 64)
		if err != nil {
			return "", false
		}
		if strings.HasSuffix(m[4], "%") {
			a /= 100
		}
		out += fmt.Sprintf("%02X", int(math.Round(min(max(a, 0), 1)*255)))
	}
	return out, true
}
