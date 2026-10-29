package wittydvd

import (
	"os"
	"testing"
)

func cmd(b ...byte) Command {
	var c Command
	copy(c[:], b)
	return c
}

func TestDecode(t *testing.T) {
	cases := []struct {
		in   Command
		want string
	}{
		{cmd(0x30, 0x02, 0, 0, 0, 1, 0, 0), "JumpTT 1"},
		{cmd(0x20, 0x01, 0, 0, 0, 0, 0, 0x0d), "LinkTailPGC"},
		{cmd(0x91, 0x0a, 0, 1, 0, 0, 0, 0), "g10 = 1"},
		{cmd(0x20, 0xa4, 0, 0x0a, 0, 2, 0, 0x30), "if (g10 == 2) LinkPGCN 48"},
		{cmd(0x30, 0x06, 0, 0, 0, 0x43, 0, 0), "JumpSS VMGM menu root"},
		{cmd(0x30, 0x08, 0, 0, 0x01, 0x83, 0, 0), "CallSS VTSM menu root (resume cell 1)"},
		{cmd(0x00, 0x01, 0, 0, 0, 0, 0, 12), "Goto 12"},
		{cmd(0x30, 0x05, 0, 0x03, 0, 1, 0, 0), "JumpVTS_PTT 1 chapter 3"},
		{cmd(0x71, 0x01, 0, 0x0a, 0, 1, 0, 0x0d), "g10 = 1; LinkTailPGC"},
	}
	for _, c := range cases {
		if got := Decode(c.in).String(); got != c.want {
			t.Errorf("%x: got %q want %q", c.in, got, c.want)
		}
	}
}

func TestRun(t *testing.T) {
	r := NewRegisters(RegisterOptions{})
	// g10 = 2 ; if (g10 == 2) LinkPGCN 48 ; JumpTT 1 (never reached)
	res := r.Run([]Command{
		cmd(0x91, 0x0a, 0, 2, 0, 0, 0, 0),
		cmd(0x20, 0xa4, 0, 0x0a, 0, 2, 0, 0x30),
		cmd(0x30, 0x02, 0, 0, 0, 1, 0, 0),
	}, RunOptions{})
	if res.Kind != ResultLink || res.Link == nil || res.Link.Op != LinkPGCN || res.Link.N != 48 {
		t.Fatalf("unexpected result %+v", res)
	}
	if r.GPRM[10] != 2 {
		t.Fatalf("g10 = %d", r.GPRM[10])
	}
	// Goto loops are bounded.
	res = r.Run([]Command{cmd(0x00, 0x01, 0, 0, 0, 0, 0, 1)}, RunOptions{})
	if res.Kind != ResultStop {
		t.Fatalf("runaway loop not stopped: %+v", res)
	}
	// Button links set SPRM8.
	res = r.Run([]Command{cmd(0x20, 0x01, 0, 0, 0, 0, 3<<2, 0x0d)}, RunOptions{})
	if res.Kind != ResultLink || r.SPRM[SprmHLBTNN]>>10 != 3 {
		t.Fatalf("button highlight not applied: %+v sprm8=%d", res, r.SPRM[SprmHLBTNN])
	}
}

func TestSPUDecode(t *testing.T) {
	unit := []byte{
		0x00, 30, // size
		0x00, 6, // DCSQT offset
		0x90,                   // top field: run 2 colour 1
		0x90,                   // bottom field
		0x00, 0x00, 0x00, 0x06, // delay, next (self)
		0x03, 0x00, 0x00, // SET_COLOR
		0x04, 0xff, 0xff, // SET_CONTR all opaque
		0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x01, // area 0..1 x 0..1
		0x06, 0x00, 0x04, 0x00, 0x05, // pixel data offsets
		0x01, // STA_DSP
		0xff,
	}
	s, err := DecodeSPU(unit)
	if err != nil {
		t.Fatal(err)
	}
	if s.Width != 2 || s.Height != 2 {
		t.Fatalf("size %dx%d", s.Width, s.Height)
	}
	for i, p := range s.Pixels {
		if p != 1 {
			t.Fatalf("pixel %d = %d", i, p)
		}
	}
	var pal [16]uint32
	pal[0] = 0x808080 // grey
	img := s.Render(pal, nil)
	if !img.Opaque() || img.Pix[3] != 255 {
		t.Fatalf("expected opaque render, alpha=%d", img.Pix[3])
	}
}

func TestPCIButtonLayout(t *testing.T) {
	b := make([]byte, pciMin)
	b[pciHLISS+1] = 1 // new highlight
	b[pciBTNNS] = 1
	b[pciBTNMD] = 0x10 // one group
	o := pciBTNIT
	// x0=100 x1=200 y0=50 y1=80, colour scheme 1, links all to 1, cmd JumpTT 1
	b[o] = 1<<6 | byte(100>>4)
	b[o+1] = byte(100&0x0f)<<4 | byte(200>>8)
	b[o+2] = byte(200 & 0xff)
	b[o+3] = byte(50 >> 4)
	b[o+4] = byte(50&0x0f)<<4 | byte(80>>8)
	b[o+5] = byte(80 & 0xff)
	b[o+6], b[o+7], b[o+8], b[o+9] = 1, 1, 1, 1
	copy(b[o+10:], []byte{0x30, 0x02, 0, 0, 0, 1, 0, 0})
	p := ParsePCI(b)
	if p == nil || len(p.Buttons) != 1 {
		t.Fatalf("buttons: %+v", p)
	}
	bt := p.Buttons[0]
	if bt.X0 != 100 || bt.X1 != 200 || bt.Y0 != 50 || bt.Y1 != 80 || bt.ColorScheme != 1 {
		t.Fatalf("geometry %+v", bt)
	}
	if Decode(bt.Cmd).String() != "JumpTT 1" {
		t.Fatalf("cmd %s", Decode(bt.Cmd))
	}
}

// TestRealDisc exercises the whole stack against a decrypted DVD image when
// WMS_TEST_ISO points at one (skipped otherwise).
func TestRealDisc(t *testing.T) {
	p := os.Getenv("WMS_TEST_ISO")
	if p == "" {
		t.Skip("set WMS_TEST_ISO to a decrypted DVD image")
	}
	o, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if len(o.Disc.Titles()) == 0 {
		t.Fatal("no titles")
	}
	nav := NewNav(o.Disc, NavOptions{})
	st := nav.Start()
	found := false
	for i := 0; i < 200 && st.Kind != StepStop; i++ {
		if st.Kind == StepMenu {
			rd, err := o.Reader(SpaceFor(st.Domain, st.VTS))
			if err != nil {
				t.Fatal(err)
			}
			pic, err := ScanMenu(rd, st.FirstSector, st.LastSector, MenuScanOptions{Palette: st.Palette, Video: o.Disc.VMG.MenuVideo})
			if err == nil && len(pic.Buttons) > 0 {
				found = true
				t.Logf("menu %s pgc %d: %d buttons, highlights=%v", st.DomainName, st.PGCN, len(pic.Buttons), pic.HasHighlights)
				break
			}
		}
		if st.StillSec > 1e9 {
			break
		}
		st = nav.CellEnded()
	}
	if !found {
		t.Log("no menu with buttons reached from first play (disc may go straight to the feature)")
	}
}

func BenchmarkRun(b *testing.B) {
	prog := []Command{
		cmd(0x71, 0x0a, 0, 2, 0, 0, 0, 0),       // g10 = 2
		cmd(0x00, 0xa1, 0, 0x0a, 0, 3, 0, 4),    // if (g10 == 3) Goto 4
		cmd(0x71, 0x0b, 0, 7, 0, 0, 0, 0),       // g11 = 7
		cmd(0x20, 0xa4, 0, 0x0a, 0, 2, 0, 0x30), // if (g10 == 2) LinkPGCN 48
	}
	r := NewRegisters(RegisterOptions{})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.Run(prog, RunOptions{})
	}
}
