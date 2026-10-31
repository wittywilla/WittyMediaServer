package wittybluray

import (
	"bytes"
	"encoding/gob"
	"image/color"
	"testing"
)

func testMenu() *Menu {
	var p Palette
	p[1] = color.RGBA{10, 20, 30, 255}
	p[2] = color.RGBA{40, 50, 60, 128}
	pix := func(w, h int, c byte) []byte { return bytes.Repeat([]byte{c}, w*h) }
	return &Menu{Width: 64, Height: 48, Popup: true, Timeout: 1.5, SelectionTimeout: 2.5, UserTimeout: 3.5,
		palettes: map[int]Palette{0: p},
		objects: map[int]*Object{
			7: {ID: 7, Width: 8, Height: 4, Pixels: pix(8, 4, 1)},
			8: {ID: 8, Width: 8, Height: 4, Pixels: pix(8, 4, 2)},
		},
		Pages: []Page{{ID: 3, DefaultSelected: 1, DefaultActivated: NoButton, AnimationRate: 2,
			In: EffectSequence{
				Windows: []Window{{ID: 1, X: 2, Y: 3, W: 40, H: 30}},
				Effects: []Effect{{DurationSec: 0.5, Palette: 0, Objects: []compObject{{object: 7, window: 1, forced: true, x: 4, y: 5, cx: 1, cy: 1, cw: 4, ch: 2}}}},
			},
			BOGs: []BOG{{DefaultValid: 1, Buttons: []Button{{ID: 1, NumericSelect: NoButton, X: 10, Y: 20, Up: NoButton, Down: NoButton, Left: NoButton, Right: NoButton,
				Normal: StateImages{Start: 7, End: 7}, Selected: StateImages{Start: 8, End: 8}, Activated: StateImages{Start: NoButton, End: NoButton},
				SelectedSound: 1, ActivatedSound: NoSound, Commands: []Command{{Op: 1, Dst: 2, Src: 3}}}}}},
		}},
	}
}

func viaGob[T any](t *testing.T, in *T) (*T, error) {
	t.Helper()
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(in); err != nil {
		t.Fatal(err)
	}
	var out T
	err := gob.NewDecoder(&buf).Decode(&out)
	return &out, err
}

// A menu drawn from what arrived is the menu drawn from what was read.
func TestMenuTravels(t *testing.T) {
	m := testMenu()
	got, err := viaGob(t, m)
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != 64 || !got.Popup || got.UserTimeout != 3.5 || len(got.Pages) != 1 || got.Objects() != 2 || got.Palettes() != 1 {
		t.Fatalf("%+v", got)
	}
	page, gotPage := &m.Pages[0], &got.Pages[0]
	buttons, gotButtons := page.Visible(nil, nil), gotPage.Visible(nil, nil)
	if len(buttons) != 1 || len(gotButtons) != 1 {
		t.Fatalf("%d and %d buttons", len(buttons), len(gotButtons))
	}
	if a, b := m.Render(page, buttons, 1), got.Render(gotPage, gotButtons, 1); !bytes.Equal(a.Pix, b.Pix) {
		t.Error("the page draws differently")
	}
	if a, b := m.DrawEffect(&page.In, 0), got.DrawEffect(&gotPage.In, 0); !bytes.Equal(a.Pix, b.Pix) {
		t.Error("the effect draws differently")
	}
	if m.Rect(buttons[0]) != got.Rect(gotButtons[0]) {
		t.Error("the button is somewhere else")
	}
	if b := gotPage.Button(1); b == nil || len(b.Commands) != 1 || b.Commands[0] != (Command{1, 2, 3}) {
		t.Errorf("the button's commands: %+v", b)
	}
}

// What drawing relies on is checked as it arrives, since a fork sends what it likes.
func TestBadMenusAreRefused(t *testing.T) {
	for name, spoil := range map[string]func(m *Menu){
		"too few pixels":  func(m *Menu) { m.objects[7].Pixels = m.objects[7].Pixels[:3] },
		"too many pixels": func(m *Menu) { m.objects[7].Pixels = append(m.objects[7].Pixels, 0) },
		"negative size":   func(m *Menu) { m.objects[7].Width, m.objects[7].Height = -8, -4 },
		"huge picture":    func(m *Menu) { m.objects[7].Width, m.objects[7].Height = 1<<16, 1 },
		"huge frame":      func(m *Menu) { m.Width, m.Height = 1<<20, 1<<20 },
		"negative frame":  func(m *Menu) { m.Width = -1 },
		"negative crop":   func(m *Menu) { m.Pages[0].In.Effects[0].Objects[0].cw = -1 },
		"endless crop":    func(m *Menu) { m.Pages[0].In.Effects[0].Objects[0].ch = 1 << 40 },
		"negative time":   func(m *Menu) { m.Timeout = -1 },
		"negative effect": func(m *Menu) { m.Pages[0].In.Effects[0].DurationSec = -1 },
	} {
		m := testMenu()
		spoil(m)
		if _, err := viaGob(t, m); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSoundsTravel(t *testing.T) {
	s := &Sounds{Clips: []SoundClip{{Channels: 2, Rate: 48000, offset: 2, length: 4}}, data: []byte{0, 0, 1, 2, 3, 4, 0}}
	got, err := viaGob(t, s)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.WAV(0)
	b, ok := got.WAV(0)
	if !ok || !bytes.Equal(a, b) {
		t.Errorf("%x, want %x", b, a)
	}
	for _, c := range []SoundClip{{offset: -1, length: 1}, {offset: 1, length: -1}, {offset: 6, length: 2}, {offset: 1 << 40, length: 1}, {offset: 1, length: 1<<63 - 1}} {
		if _, err := viaGob(t, &Sounds{Clips: []SoundClip{c}, data: s.data}); err == nil {
			t.Errorf("a clip at %d of %d bytes was accepted", c.offset, c.length)
		}
	}
}
