// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandname

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// Per-part colours (docs/plans/DESIGN-2.md 7.6): every part of the name carries the colour
// it is drawn in. The quick modes (auto, one, pick) give fg and accent; the editor's spans
// give any of the five kinds.
const (
	KindFG       = "fg"       // the theme's text colour
	KindAccent   = "accent"   // the theme's accent as text (brandAccent): changes with the theme
	KindToken    = "token"    // any theme colour token (ok, warn, accentText, ...): changes with the theme
	KindStatic   = "static"   // a fixed #rrggbb that does not change with themes
	KindGradient = "gradient" // two to four fixed stops across the part, in a direction
)

// Color is how one part is coloured.
type Color struct {
	Kind  string   `json:"kind"`
	Token string   `json:"token,omitempty"`
	Hex   string   `json:"hex,omitempty"`
	Stops []string `json:"stops,omitempty"`
	// Angle is a gradient's direction in degrees (90: left to right).
	Angle int `json:"angle,omitempty"`
}

// Span colours a range of the name: [From, To) code points.
type Span struct {
	From  int   `json:"from"`
	To    int   `json:"to"`
	Color Color `json:"color"`
	Bold  bool  `json:"bold,omitempty"`
}

var (
	hexRe   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	tokenRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,24}$`)
)

// MaxSpans bounds the spans of one name.
const MaxSpans = 40

// Validate checks one colour.
func (c Color) Validate() error {
	switch c.Kind {
	case KindFG, KindAccent:
		return nil
	case KindToken:
		if !tokenRe.MatchString(c.Token) {
			return errors.New("a token colour names a theme colour token")
		}
	case KindStatic:
		if !hexRe.MatchString(c.Hex) {
			return errors.New("a fixed colour is #rrggbb")
		}
	case KindGradient:
		if len(c.Stops) < 2 || len(c.Stops) > 4 {
			return errors.New("a gradient has two to four stops")
		}
		for _, s := range c.Stops {
			if !hexRe.MatchString(s) {
				return errors.New("a gradient's stops are #rrggbb")
			}
		}
		if c.Angle < 0 || c.Angle > 360 {
			return errors.New("a gradient's angle is 0 to 360 degrees")
		}
	default:
		return errors.New("a colour kind is fg, accent, token, static or gradient")
	}
	return nil
}

// ParseSpans reads the editor's spans (JSON) and checks them: in order, not overlapping,
// inside the name, every colour valid. "" is no spans.
func ParseSpans(raw string, nameRunes int) ([]Span, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var spans []Span
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spans); err != nil {
		return nil, errors.New(`the spans are [{"from": 0, "to": 5, "color": {"kind": "accent"}}, ...]`)
	}
	if len(spans) > MaxSpans {
		return nil, errors.New("too many spans")
	}
	end := 0
	for _, s := range spans {
		if s.From < end || s.To <= s.From || s.To > nameRunes {
			return nil, errors.New("spans are in order, don't overlap and stay inside the name")
		}
		if err := s.Color.Validate(); err != nil {
			return nil, err
		}
		end = s.To
	}
	return spans, nil
}

// PartsFromSpans splits name by spans; letters between spans are fg. Neighbouring parts that
// look the same are joined.
func PartsFromSpans(name string, spans []Span) []Part {
	r := []rune(name)
	type look struct {
		C    Color
		Bold bool
	}
	looks := make([]look, len(r))
	for i := range looks {
		looks[i] = look{C: Color{Kind: KindFG}}
	}
	for _, s := range spans {
		for i := s.From; i < s.To && i < len(r); i++ {
			looks[i] = look{C: s.Color, Bold: s.Bold}
		}
	}
	same := func(a, b look) bool {
		x, _ := json.Marshal(a)
		y, _ := json.Marshal(b)
		return string(x) == string(y)
	}
	var out []Part
	start := 0
	for i := 1; i <= len(r); i++ {
		if i == len(r) || !same(looks[i], looks[start]) {
			c := looks[start].C
			out = append(out, Part{Text: string(r[start:i]), Accent: c.Kind == KindAccent, Color: &c, Bold: looks[start].Bold})
			start = i
		}
	}
	return out
}

// Mark is what the rail's square badge holds: the first letter, initials, one or two
// characters of the owner's own, or an uploaded square logo.
type Mark struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
	// ImageURL is set by the server when Kind is image: where the logo is.
	ImageURL string `json:"imageUrl,omitempty"`
}

// Mark kinds.
const (
	MarkLetter   = "letter"
	MarkInitials = "initials"
	MarkText     = "text"
	MarkImage    = "image"
)

// ParseMark reads the mark setting. "" is the first letter.
func ParseMark(raw string) (Mark, error) {
	if strings.TrimSpace(raw) == "" {
		return Mark{Kind: MarkLetter}, nil
	}
	var m Mark
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Mark{}, errors.New(`the mark is {"kind": "letter"|"initials"|"text"|"image", "text": "..."}`)
	}
	switch m.Kind {
	case MarkLetter, MarkInitials, MarkImage:
		m.Text = ""
	case MarkText:
		if n := len([]rune(strings.TrimSpace(m.Text))); n < 1 || n > 2 {
			return Mark{}, errors.New("your own mark is one or two characters")
		}
		m.Text = strings.TrimSpace(m.Text)
	default:
		return Mark{}, errors.New("a mark is a letter, initials, text or an image")
	}
	m.ImageURL = ""
	return m, nil
}

// Shown is what the badge shows for a mark that is text: the first letter of the name,
// its initials (the first letter of each word, at most three) or the owner's own characters.
func (m Mark) Shown(name string) string {
	switch m.Kind {
	case MarkText:
		return m.Text
	case MarkInitials:
		var out []rune
		for _, w := range Words(name) {
			if len(out) < 3 {
				out = append(out, []rune(name)[w[0]])
			}
		}
		return string(out)
	}
	r := []rune(strings.TrimSpace(name))
	if len(r) == 0 {
		return ""
	}
	return strings.ToUpper(string(r[0]))
}
