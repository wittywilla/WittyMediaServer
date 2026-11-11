// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"strings"
	"testing"
)

// A layout from Edit layout: checked by the server itself, whatever WittyPlay is installed.
func TestLayoutsAreChecked(t *testing.T) {
	known := map[string]bool{"dpad": true, "b8": true}
	good := `{"controls":{"dpad":{"x":0.13,"y":0.66,"size":0.32,"opacity":0.55}},"menu":{"x":0.96,"y":0.05},` +
		`"screens":[{"x":0.5,"y":0.27,"size":0.5},{"x":0.5,"y":0.76,"size":0.45}],"faceTogether":true,"screensTogether":false}`
	clean, err := cleanLayout(known, json.RawMessage(good))
	if err != nil {
		t.Fatalf("a good layout: %v", err)
	}
	var back savedLayout
	if err := json.Unmarshal(clean, &back); err != nil || len(back.Screens) != 2 || back.Controls["dpad"].Size != 0.32 || back.Menu == nil || *back.ScreensTogether {
		t.Fatalf("kept as given: %s (%v)", clean, err)
	}
	for name, body := range map[string]string{
		"an unknown control":   `{"controls":{"wheel":{"x":0.5,"y":0.5,"size":0.2,"opacity":0.5}}}`,
		"a control off screen": `{"controls":{"dpad":{"x":1.4,"y":0.5,"size":0.2,"opacity":0.5}}}`,
		"a control too big":    `{"controls":{"dpad":{"x":0.5,"y":0.5,"size":0.9,"opacity":0.5}}}`,
		"an invisible control": `{"controls":{"dpad":{"x":0.5,"y":0.5,"size":0.2,"opacity":0}}}`,
		"three screens":        `{"screens":[{"x":0.5,"y":0.5,"size":0.3},{"x":0.5,"y":0.5,"size":0.3},{"x":0.5,"y":0.5,"size":0.3}]}`,
		"a screen too small":   `{"screens":[{"x":0.5,"y":0.5,"size":0.01}]}`,
		"the menu off screen":  `{"menu":{"x":-0.2,"y":0.5}}`,
		"something unknown":    `{"wallpaper":"x"}`,
		"not a layout":         `[1,2,3]`,
		"too large":            `{"controls":{` + strings.Repeat(" ", 20<<10) + `}}`,
	} {
		if _, err := cleanLayout(known, json.RawMessage(body)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// Every shape of device a client can name has a layout of its own; nothing else does.
func TestLayoutShapes(t *testing.T) {
	for _, s := range []string{"phone-portrait", "phone-landscape", "tablet-portrait", "tablet-landscape", "desktop", "tv"} {
		if !layoutShapes[s] {
			t.Errorf("%s isn't a shape", s)
		}
	}
	for _, s := range []string{"", "phone", "../tv", "watch"} {
		if layoutShapes[s] {
			t.Errorf("%q is a shape", s)
		}
	}
}

// A two-screen console says where each screen is in its frame, for a client to place them apart.
func TestScreensHaveFrames(t *testing.T) {
	for _, id := range []string{"nds", "3ds"} {
		s := consoleTraitsTable[id].Caps.Screens
		if s.Count != 2 || len(s.Frame) != 2 {
			t.Fatalf("%s: %+v", id, s)
		}
		for _, f := range s.Frame {
			if f.X < 0 || f.Y < 0 || f.X+f.W > 1.0001 || f.Y+f.H > 1.0001 || f.W <= 0 || f.H <= 0 {
				t.Errorf("%s: a screen outside the frame: %+v", id, f)
			}
		}
	}
}
