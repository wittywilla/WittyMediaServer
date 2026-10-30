package wittydvd

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

type ifoBuf []byte

func (b ifoBuf) u16(off, v int)     { binary.BigEndian.PutUint16(b[off:], uint16(v)) }
func (b ifoBuf) u32(off int, v int) { binary.BigEndian.PutUint32(b[off:], uint32(v)) }

// testVMG builds a VIDEO_TS.IFO with one title (VTS 1, title 1), a menu VOB
// of 40 sectors and a parental table for "US" that maps level 8 to mask
// 0x0002 and every other level to 0x0001 for title set 1.
func testVMG() []byte {
	b := ifoBuf(make([]byte, 3*Sector))
	copy(b, "DVDVIDEO-VMG")
	b.u32(0x0c, 3+40+3-1) // last sector of the VMG: IFO, menu VOB, BUP
	b.u32(0x1c, 2)        // last sector of the IFO
	b.u16(0x3e, 1)
	b.u32(0xc0, 3) // menu VOB
	b.u32(0xc4, 1) // TT_SRPT
	b.u32(0xcc, 2) // PTL_MAIT
	tt := Sector
	b.u16(tt, 1)
	b.u32(tt+4, 8+12-1)
	b[tt+8], b[tt+9] = 0x3c, 1 // playback type, angles
	b.u16(tt+10, 1)            // chapters
	b[tt+14], b[tt+15] = 1, 1  // VTS 1, title 1
	pt := 2 * Sector
	b.u16(pt, 1)   // countries
	b.u16(pt+2, 1) // title sets
	copy(b[pt+8:], "us")
	b.u16(pt+12, 16) // offset of the country's masks
	for level := 8; level >= 1; level-- {
		blk := pt + 16 + 2*(8-level)*2
		b.u16(blk, 0xffff)
		if level == 8 {
			b.u16(blk+2, 0x0002)
		} else {
			b.u16(blk+2, 0x0001)
		}
	}
	return b
}

// testVTS builds a VTS_01_0.IFO with one title made of PGCs given as
// (cells, parental mask, block mode<<2 | block type); the title VOB holds
// 100 sectors.
func testVTS(pgcs ...[3]int) []byte {
	b := ifoBuf(make([]byte, 4*Sector))
	copy(b, "DVDVIDEO-VTS")
	b.u32(0x0c, 4+10+100+4-1) // IFO, menu VOB, title VOB, BUP
	b.u32(0x1c, 3)
	b.u32(0xc0, 4)  // menu VOB
	b.u32(0xc4, 14) // title VOB
	b.u32(0xc8, 1)  // PTT_SRPT
	b.u32(0xcc, 2)  // PGCIT
	pt := Sector
	b.u16(pt, 1)
	b.u32(pt+4, 12+4-1)
	b.u32(pt+8, 12)
	b.u16(pt+12, 1)
	b.u16(pt+14, 1)
	it := 2 * Sector
	b.u16(it, len(pgcs))
	at := 8 + 8*len(pgcs)
	for i, spec := range pgcs {
		cells, mask, block := spec[0], spec[1], spec[2]
		cat := mask | block<<20
		if i == 0 {
			cat |= 0x81 << 24 // entry PGC of title 1
		}
		b.u32(it+8+i*8, cat)
		b.u32(it+12+i*8, at)
		p := it + at
		b[p+2], b[p+3] = byte(cells), byte(cells)
		b.u16(p+0xe6, 0xec)
		b.u16(p+0xe8, 0xec+cells)
		for c := 0; c < cells; c++ {
			b[p+0xec+c] = byte(c + 1)
			cp := p + 0xec + cells + c*24
			b.u32(cp+8, c*10)
			b.u32(cp+20, c*10+9)
		}
		at += 0xec + cells + cells*24
	}
	return b
}

func TestParseParentalAndSizes(t *testing.T) {
	vmg, err := ParseVMG(testVMG())
	if err != nil {
		t.Fatal(err)
	}
	if vmg.MenuVOBSectors != 40 {
		t.Fatalf("menu VOB size %d", vmg.MenuVOBSectors)
	}
	if len(vmg.Parental) != 1 || vmg.Parental[0].Country != "US" {
		t.Fatalf("parental table: %+v", vmg.Parental)
	}
	d := &Disc{VMG: vmg}
	us := code2("US")
	if m := d.ParentalMask(us, 15, 1); m != 0x0002 {
		t.Fatalf("no parental control acts at level 8: %#x", m)
	}
	if m := d.ParentalMask(us, 3, 1); m != 0x0001 {
		t.Fatalf("level 3: %#x", m)
	}
	if m := d.ParentalMask(code2("DE"), 3, 1); m != 0x0001 {
		t.Fatalf("unlisted country uses the first entry: %#x", m)
	}

	vts, err := ParseVTS(testVTS([3]int{2, 0x0001, 1<<2 | 1}, [3]int{1, 0x0002, 3<<2 | 1}), 1)
	if err != nil {
		t.Fatal(err)
	}
	if vts.MenuVOBSectors != 10 || vts.TitleVOBSectors != 100 {
		t.Fatalf("VOB sizes %d %d", vts.MenuVOBSectors, vts.TitleVOBSectors)
	}
	e := vts.PGCs[1]
	if e.ParentalMask != 0x0002 || e.BlockType != 1 || e.BlockMode != 3 {
		t.Fatalf("PGC category: %+v", e)
	}
}

func TestOpenFallsBackToBUP(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "VIDEO_TS")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, b []byte) {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	vts := testVTS([3]int{2, 0, 0})
	write("VIDEO_TS.IFO", testVMG())
	// A damaged IFO: readable, but its header is gone (zeroed sectors).
	write("VTS_01_0.IFO", make([]byte, len(vts)))
	write("VTS_01_0.BUP", vts)
	o, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Disc.VTS[1] == nil || len(o.Disc.VTS[1].PGCs) != 1 {
		t.Fatalf("title set from the backup: %+v", o.Disc.VTS[1])
	}
	// A broken VMG with an intact backup opens too.
	write("VIDEO_TS.IFO", []byte("DVDVIDEO-VMG"))
	write("VIDEO_TS.BUP", testVMG())
	o2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	o2.Close()
}

func TestMainTitleSkipsDecoys(t *testing.T) {
	// Title 1 is the feature: cells 0..3 in disc order. Title 2 is a decoy
	// that plays the same cells backwards and repeats them; title 3 points
	// past the end of the title VOB.
	cells := func(order ...int) *PGC {
		p := &PGC{}
		for _, c := range order {
			p.Cells = append(p.Cells, Cell{FirstSector: uint32(c * 1000), LastSector: uint32(c*1000 + 999), Duration: 600})
		}
		return p
	}
	v := &VTS{Number: 1, TitleVOBSectors: 4000, PGCs: []PGCEntry{
		{Entry: true, TitleNumber: 1, PGC: cells(0, 1, 2, 3)},
		{Entry: true, TitleNumber: 2, PGC: cells(3, 2, 1, 0, 3, 2)},
		{Entry: true, TitleNumber: 3, PGC: cells(7, 8, 9, 10, 11, 12)},
	}, PTT: [][]PTT{{{1, 1}}, {{2, 1}}, {{3, 1}}}}
	d := &Disc{VMG: &VMG{Titles: []TitleSearch{{Number: 1, VTS: 1, VTSTitle: 1}, {Number: 2, VTS: 1, VTSTitle: 2}, {Number: 3, VTS: 1, VTSTitle: 3}}}, VTS: map[int]*VTS{1: v}}
	if m := d.MainTitle(); m == nil || m.Number != 1 {
		t.Fatalf("main title: %+v", m)
	}
}
