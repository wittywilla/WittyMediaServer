package wittydvd

import (
	"encoding/binary"
	"math"
	"testing"
)

// spuUnit assembles a sub-picture unit from RLE pixel data and display
// control sequences, each given as a 90 kHz/1024 delay and its commands.
func spuUnit(pix []byte, seqs ...[]byte) []byte {
	u := append([]byte{0, 0, 0, 0}, pix...)
	table := len(u)
	at := table
	for i, sq := range seqs {
		next := at + 2 + len(sq)
		if i == len(seqs)-1 {
			next = at
		}
		u = append(u, sq[0], sq[1], byte(next>>8), byte(next))
		u = append(u, sq[2:]...)
		at = next
	}
	binary.BigEndian.PutUint16(u, uint16(len(u)))
	binary.BigEndian.PutUint16(u[2:], uint16(table))
	return u
}

// A 4x2 picture of code 1 at frame (10,20): one RLE byte per row (run 4).
var spuPix = []byte{0x11, 0x11}

func spuSetup(extra ...byte) []byte {
	return append([]byte{
		0x03, 0x00, 0x10, // SET_COLOR: code 1 -> palette 1
		0x04, 0x00, 0xf0, // SET_CONTR: code 1 opaque
		0x05, 0x00, 0xa0, 0x0d, 0x01, 0x40, 0x15, // SET_DAREA x 10..13, y 20..21
		0x06, 0x00, 0x04, 0x00, 0x05, // SET_DSPXA
	}, extra...)
}

func TestSPUFrames(t *testing.T) {
	// Shown at 0, recoloured at 100 (1.14 s), stopped at 200: two pictures.
	u := spuUnit(spuPix,
		append([]byte{0, 0}, spuSetup(0x01, 0xff)...),
		[]byte{0, 100, 0x03, 0x00, 0x20, 0xff},
		[]byte{0, 200, 0x02, 0xff},
	)
	fs, err := DecodeSPUFrames(u)
	if err != nil {
		t.Fatal(err)
	}
	sec := func(d int) float64 { return float64(d) * 1024 / 90000 }
	if len(fs) != 2 || fs[0].StartSec != 0 || fs[0].StopSec != sec(100) || fs[1].StartSec != sec(100) || fs[1].StopSec != sec(200) {
		t.Fatalf("frames: %+v", fs)
	}
	if fs[0].Color[1] != 1 || fs[1].Color[1] != 2 || fs[0].X != 10 || fs[0].Width != 4 || fs[1].Pixels[0] != 1 {
		t.Fatalf("frame states: %+v / %+v", fs[0], fs[1])
	}
	// DecodeSPU still gives the first picture with its stop time.
	one, err := DecodeSPU(u)
	if err != nil || one.StopSec != sec(100) || one.Color[1] != 1 {
		t.Fatalf("DecodeSPU: %+v %v", one, err)
	}

	// Blinking: on, off, on again (until the next unit).
	u = spuUnit(spuPix,
		append([]byte{0, 0}, spuSetup(0x00, 0xff)...),
		[]byte{0, 50, 0x02, 0xff},
		[]byte{0, 90, 0x01, 0xff},
	)
	fs, _ = DecodeSPUFrames(u)
	if len(fs) != 2 || !fs[0].Forced || fs[1].Forced || fs[1].StartSec != sec(90) || fs[1].StopSec >= 0 {
		t.Fatalf("blink: %+v", fs)
	}

	// Menu artwork that never starts still decodes (for highlights).
	if s, err := DecodeSPU(spuUnit(spuPix, append([]byte{0, 0}, spuSetup(0xff)...))); err != nil || s.StartSec >= 0 || s.Width != 4 {
		t.Fatalf("no start: %+v %v", s, err)
	}
}

func TestSPUColCon(t *testing.T) {
	// Lines 20..21: from column 12 on, code 1 uses palette 3, half contrast.
	colcon := []byte{
		0x07, 0x00, 2 + 4 + 6 + 4,
		0x00, 0x14, 0x10, 0x15, // lines 20..21, one pixel control
		0x00, 0x0c, 0x00, 0x30, 0x00, 0x80, // column 12: colour 3, contrast 8
		0x0f, 0xff, 0xff, 0xff,
	}
	u := spuUnit(spuPix, append([]byte{0, 0}, spuSetup(append(colcon, 0x01, 0xff)...)...))
	s, err := DecodeSPU(u)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.ColCon) != 1 || s.ColCon[0].FirstLine != 20 || s.ColCon[0].LastLine != 21 || s.ColCon[0].Columns[0].Column != 12 {
		t.Fatalf("CHG_COLCON: %+v", s.ColCon)
	}
	var pal [16]uint32
	pal[1], pal[3] = 0x10_80_80, 0xeb_80_80 // black, white
	img := s.Render(pal, nil)
	px := func(x int) []byte { return img.Pix[x*4 : x*4+4] }
	if px(0)[0] != 0 || px(0)[3] != 255 {
		t.Fatalf("left of the column keeps its own look: %v", px(0))
	}
	if px(2)[0] != 255 || px(2)[3] != 8*17 {
		t.Fatalf("right of the column is recoloured: %v", px(2))
	}
	// A highlight look replaces CHG_COLCON inside the button.
	look := Look{Color: [4]int{0, 1, 0, 0}, Contrast: [4]int{0, 15, 0, 0}}
	if hl := s.RenderRect(pal, &look, 10, 20, 13, 21); hl.Pix[2*4] != 0 || hl.Pix[2*4+3] != 255 {
		t.Fatalf("highlight: %v", hl.Pix[8:12])
	}
}

func TestSPUBadSequences(t *testing.T) {
	// A start before the display area is known shows from the sequence that
	// sets it; a delay that goes back in time is clamped.
	u := spuUnit(spuPix,
		[]byte{0, 40, 0x01, 0xff},
		append([]byte{0, 10}, spuSetup(0xff)...),
	)
	fs, err := DecodeSPUFrames(u)
	if err != nil || len(fs) != 1 || math.Abs(fs[0].StartSec-float64(40)*1024/90000) > 1e-9 {
		t.Fatalf("late area: %+v %v", fs, err)
	}
	// A truncated CHG_COLCON is an error, not a crash.
	if _, err := DecodeSPU(spuUnit(spuPix, []byte{0, 0, 0x07, 0x00, 0x40, 0xff})); err == nil {
		t.Fatal("expected CHG_COLCON size error")
	}
}
