package wittybluray

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"image/color"
	"math"
)

// Sending what a disc holds to another process: when the engine runs as a helper,
// the server draws its menus and plays its sounds from what the helper read. Those
// types keep their pictures and samples in unexported fields, which gob can't see,
// so they say how to send themselves here.
//
// The side that decodes may not trust the side that encoded (a helper can be
// someone's fork), so decoding checks everything drawing or playing would rely on.
// Anything these checks let through is safe to draw and play.

// Limits on what is drawn. The largest frame a Blu-ray has is 3840x2160.
const (
	maxPictureSide = 4096
	maxCoordinate  = 65535 // the format stores positions and sizes in 16 bits
)

// compWire is a compObject that gob can see.
type compWire struct {
	Object, Window int
	Forced         bool
	X, Y           int
	CX, CY, CW, CH int
}

type effectWire struct {
	DurationSec float64
	Palette     int
	Objects     []compWire
}

// GobEncode sends an effect with where its pictures go.
func (e Effect) GobEncode() ([]byte, error) {
	w := effectWire{DurationSec: e.DurationSec, Palette: e.Palette, Objects: make([]compWire, len(e.Objects))}
	for i, o := range e.Objects {
		w.Objects[i] = compWire{o.object, o.window, o.forced, o.x, o.y, o.cx, o.cy, o.cw, o.ch}
	}
	return gobBytes(&w)
}

// GobDecode reads an effect and checks it.
func (e *Effect) GobDecode(b []byte) error {
	var w effectWire
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&w); err != nil {
		return err
	}
	if !usableSeconds(w.DurationSec) {
		return errors.New("a menu effect has an impossible length")
	}
	*e = Effect{DurationSec: w.DurationSec, Palette: w.Palette, Objects: make([]compObject, len(w.Objects))}
	for i, o := range w.Objects {
		for _, v := range []int{o.X, o.Y, o.CX, o.CY, o.CW, o.CH} {
			if v < 0 || v > maxCoordinate {
				return errors.New("a menu effect has an impossible position")
			}
		}
		e.Objects[i] = compObject{o.Object, o.Window, o.Forced, o.X, o.Y, o.CX, o.CY, o.CW, o.CH}
	}
	return nil
}

// menuFields is a Menu without its methods, so a field added to Menu travels too.
type menuFields Menu

type menuWire struct {
	Menu     menuFields
	Palettes map[int][]byte // 256 colours of 4 bytes each
	Objects  []Object
}

const paletteBytes = len(Palette{}) * 4

// GobEncode sends a menu with its palettes and pictures.
func (m *Menu) GobEncode() ([]byte, error) {
	w := menuWire{Menu: menuFields(*m), Palettes: make(map[int][]byte, len(m.palettes)), Objects: make([]Object, 0, len(m.objects))}
	for id, p := range m.palettes {
		b := make([]byte, paletteBytes)
		for i, c := range p {
			b[i*4], b[i*4+1], b[i*4+2], b[i*4+3] = c.R, c.G, c.B, c.A
		}
		w.Palettes[id] = b
	}
	for _, o := range m.objects {
		w.Objects = append(w.Objects, *o)
	}
	return gobBytes(&w)
}

// GobDecode reads a menu and checks it.
func (m *Menu) GobDecode(b []byte) error {
	var w menuWire
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&w); err != nil {
		return err
	}
	f := &w.Menu
	if f.Width < 0 || f.Height < 0 || f.Width > maxPictureSide || f.Height > maxPictureSide {
		return fmt.Errorf("a menu of %dx%d can't be drawn", f.Width, f.Height)
	}
	if !usableSeconds(f.Timeout) || !usableSeconds(f.SelectionTimeout) || !usableSeconds(f.UserTimeout) {
		return errors.New("a menu has an impossible time")
	}
	out := Menu(w.Menu)
	out.palettes = make(map[int]Palette, len(w.Palettes))
	for id, raw := range w.Palettes {
		if len(raw) != paletteBytes {
			return fmt.Errorf("menu palette %d has the wrong size", id)
		}
		var p Palette
		for i := range p {
			p[i] = color.RGBA{raw[i*4], raw[i*4+1], raw[i*4+2], raw[i*4+3]}
		}
		out.palettes[id] = p
	}
	out.objects = make(map[int]*Object, len(w.Objects))
	for i := range w.Objects {
		o := &w.Objects[i]
		if o.Width <= 0 || o.Height <= 0 || o.Width > maxPictureSide || o.Height > maxPictureSide || len(o.Pixels) != o.Width*o.Height {
			return fmt.Errorf("menu picture %d has the wrong size", o.ID)
		}
		out.objects[o.ID] = o
	}
	*m = out
	return nil
}

type soundClipWire struct{ Channels, Rate, Offset, Length int }

type soundsWire struct {
	Clips []soundClipWire
	Data  []byte
}

// GobEncode sends a disc's button sounds.
func (s *Sounds) GobEncode() ([]byte, error) {
	w := soundsWire{Clips: make([]soundClipWire, len(s.Clips)), Data: s.data}
	for i, c := range s.Clips {
		w.Clips[i] = soundClipWire{c.Channels, c.Rate, c.offset, c.length}
	}
	return gobBytes(&w)
}

// GobDecode reads a disc's button sounds and checks that each lies in their data.
func (s *Sounds) GobDecode(b []byte) error {
	var w soundsWire
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&w); err != nil {
		return err
	}
	out := Sounds{Clips: make([]SoundClip, len(w.Clips)), data: w.Data}
	for i, c := range w.Clips {
		if c.Offset < 0 || c.Length < 0 || c.Offset > len(w.Data) || c.Length > len(w.Data)-c.Offset {
			return fmt.Errorf("button sound %d lies outside the sound data", i)
		}
		out.Clips[i] = SoundClip{Channels: c.Channels, Rate: c.Rate, offset: c.Offset, length: c.Length}
	}
	*s = out
	return nil
}

func gobBytes(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func usableSeconds(v float64) bool { return v >= 0 && !math.IsInf(v, 0) } // NaN fails the first test
