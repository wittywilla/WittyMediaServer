package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
)

// Edit layout (WittyMediaClient's docs/WITTYPLAY-EXPERIENCE.md, Edit layout): where everything
// sits while playing, kept per account, per console and per shape of device, on the controls
// route as kind "layout" keyed by the shape. The server checks a layout itself, so it can be kept
// whatever version of WittyPlay is installed.

const kindLayout = "layout"

// layoutShapes are the shapes of device a layout is kept for.
var layoutShapes = map[string]bool{
	"phone-portrait": true, "phone-landscape": true,
	"tablet-portrait": true, "tablet-landscape": true,
	"desktop": true, "tv": true,
}

type layoutSpot struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// layoutScreen is the picture, or one screen of a two-screen console: its centre, and its height
// as a fraction of the play area's.
type layoutScreen struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Size float64 `json:"size"`
}

type savedLayout struct {
	Controls        map[string]wittyplay.Placement `json:"controls,omitempty"`
	Menu            *layoutSpot                    `json:"menu,omitempty"`
	Touchbar        *layoutSpot                    `json:"touchbar,omitempty"`
	Screens         []layoutScreen                 `json:"screens,omitempty"`
	FaceTogether    *bool                          `json:"faceTogether,omitempty"`
	ScreensTogether *bool                          `json:"screensTogether,omitempty"`
}

// cleanLayout checks a layout someone wants to keep, given the console's controls by key, and
// returns it in the one form stored. It comes from a client, so nothing in it is taken on trust.
func cleanLayout(controls map[string]bool, data json.RawMessage) (json.RawMessage, error) {
	if len(data) > wittyplay.MaxControlSize {
		return nil, errors.New("this layout is too large")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var l savedLayout
	if err := dec.Decode(&l); err != nil {
		return nil, errors.New("this layout can't be read")
	}
	in := func(v, lo, hi float64) bool { return v >= lo && v <= hi }
	for k, p := range l.Controls {
		if !controls[k] {
			return nil, fmt.Errorf("this console has no control %q", k)
		}
		if !in(p.X, 0, 1) || !in(p.Y, 0, 1) || !in(p.Size, 0.05, 0.6) || !in(p.Opacity, 0.1, 1) {
			return nil, fmt.Errorf("%s is out of range", k)
		}
	}
	for name, s := range map[string]*layoutSpot{"the menu button": l.Menu, "the shortcuts bar": l.Touchbar} {
		if s != nil && (!in(s.X, 0, 1) || !in(s.Y, 0, 1)) {
			return nil, fmt.Errorf("%s is off the screen", name)
		}
	}
	if len(l.Screens) > 2 {
		return nil, errors.New("a layout places one picture, or two screens")
	}
	for _, s := range l.Screens {
		if !in(s.X, 0, 1) || !in(s.Y, 0, 1) || !in(s.Size, 0.1, 1) {
			return nil, errors.New("a screen is out of range")
		}
	}
	return json.Marshal(l)
}

// controlKeys is a console's on-screen controls, by key.
func controlKeys(console string) map[string]bool {
	known := map[string]bool{}
	for _, c := range wittyplay.Controls(console) {
		known[c.Key] = true
	}
	return known
}
