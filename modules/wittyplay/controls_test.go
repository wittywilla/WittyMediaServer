package wittyplay

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEveryConsoleHasACompleteLayout(t *testing.T) {
	for _, c := range Consoles {
		for _, o := range []string{Landscape, Portrait} {
			l := DefaultTouch(c.ID, o)
			for _, ctl := range Controls(c.ID) {
				p, ok := l[ctl.Key]
				if !ok {
					t.Errorf("%s %s: no place for %s", c.ID, o, ctl.Label)
					continue
				}
				if p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1 {
					t.Errorf("%s %s: %s is off the screen: %+v", c.ID, o, ctl.Label, p)
				}
			}
			// What we produce must pass our own check.
			b, _ := json.Marshal(l)
			if _, err := CleanControl(c.ID, KindTouch, b); err != nil {
				t.Errorf("%s %s: %v", c.ID, o, err)
			}
		}
		seen := map[int]bool{}
		p := Pad(c.ID)
		for _, list := range [][]PadButton{p.Face, p.Shoulder, p.Meta} {
			for _, b := range list {
				if b.ID < 0 || b.ID >= retroButtons || b.ID == RetroUp || b.ID == RetroDown || b.ID == RetroLeft || b.ID == RetroRight {
					t.Errorf("%s: %s has a bad button number %d", c.ID, b.Label, b.ID)
				}
				if seen[b.ID] {
					t.Errorf("%s: button %d is used twice", c.ID, b.ID)
				}
				seen[b.ID] = true
			}
		}
	}
}

func TestCleanControl(t *testing.T) {
	good := []struct{ kind, data string }{
		{KindGamepad, `{"8":{"b":5},"4":{"a":1,"d":-1}}`},
		{KindKeyboard, `{"8":"KeyK"}`},
		{KindTouch, `{"dpad":{"x":0.5,"y":0.5,"size":0.3,"opacity":0.5},"b8":{"x":1,"y":0,"size":0.1,"opacity":1,"hidden":true}}`},
	}
	for _, c := range good {
		if _, err := CleanControl("snes", c.kind, json.RawMessage(c.data)); err != nil {
			t.Errorf("%s %s: %v", c.kind, c.data, err)
		}
	}
	bad := []struct{ name, kind, data string }{
		{"unknown kind", "wheel", `{}`},
		{"not json", KindGamepad, `{`},
		{"button out of range", KindGamepad, `{"99":{"b":1}}`},
		{"binding with both", KindGamepad, `{"8":{"b":1,"a":1,"d":1}}`},
		{"binding with neither", KindGamepad, `{"8":{}}`},
		{"axis with no direction", KindGamepad, `{"8":{"a":1}}`},
		{"huge button number", KindGamepad, `{"8":{"b":100000}}`},
		{"key with control characters", KindKeyboard, "{\"8\":\"a\\u0007\"}"},
		{"key too long", KindKeyboard, `{"8":"` + strings.Repeat("x", 33) + `"}`},
		{"a control this console lacks", KindTouch, `{"b99":{"x":0,"y":0,"size":0.1,"opacity":1}}`},
		{"off the screen", KindTouch, `{"dpad":{"x":2,"y":0,"size":0.1,"opacity":1}}`},
		{"invisible", KindTouch, `{"dpad":{"x":0,"y":0,"size":0.1,"opacity":0}}`},
		{"too large", KindKeyboard, `{"8":"` + strings.Repeat("x", MaxControlSize) + `"}`},
	}
	for _, c := range bad {
		if _, err := CleanControl("snes", c.kind, json.RawMessage(c.data)); err == nil {
			t.Errorf("%s should be refused", c.name)
		}
	}
	// Fields nobody asked for don't get stored.
	out, err := CleanControl("snes", KindTouch, json.RawMessage(`{"dpad":{"x":0.5,"y":0.5,"size":0.3,"opacity":0.5,"evil":"<script>"}}`))
	if err != nil || strings.Contains(string(out), "evil") {
		t.Fatalf("%s %v", out, err)
	}
}
