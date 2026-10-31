package dvdremote

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittydvd"
)

// A small synthetic DVD, kept as a folder: no first-play, one title of two one-minute
// cells of ten sectors each. The first cell carries a button that links to the second;
// the second is plain video.
type ifoBuf []byte

func (b ifoBuf) u16(off, v int) { binary.BigEndian.PutUint16(b[off:], uint16(v)) }
func (b ifoBuf) u32(off, v int) { binary.BigEndian.PutUint32(b[off:], uint32(v)) }

func testVMG() []byte {
	b := ifoBuf(make([]byte, 2*wittydvd.Sector))
	copy(b, "DVDVIDEO-VMG")
	b.u32(0x0c, 1)
	b.u32(0x1c, 1)
	b.u16(0x3e, 1)
	b.u32(0xc4, 1) // TT_SRPT
	tt := wittydvd.Sector
	b.u16(tt, 1)
	b.u32(tt+4, 8+12-1)
	b[tt+8], b[tt+9] = 0x3c, 1 // playback type, angles
	b.u16(tt+10, 1)            // chapters
	b[tt+14], b[tt+15] = 1, 1  // VTS 1, title 1
	return b
}

func testVTS() []byte {
	b := ifoBuf(make([]byte, 3*wittydvd.Sector))
	copy(b, "DVDVIDEO-VTS")
	b.u32(0x0c, 3+20-1)
	b.u32(0x1c, 2)
	b.u32(0xc4, 3) // title VOB
	b.u32(0xc8, 1) // PTT_SRPT
	b.u32(0xcc, 2) // PGCIT
	pt := wittydvd.Sector
	b.u16(pt, 1)
	b.u32(pt+4, 12+4-1)
	b.u32(pt+8, 12)
	b.u16(pt+12, 1)
	b.u16(pt+14, 1)
	it := 2 * wittydvd.Sector
	b.u16(it, 1)
	b.u32(it+8, 0x81<<24) // the entry chain of title 1
	const at = 16
	b.u32(it+12, at)
	p := it + at
	const cells = 2
	b[p+2], b[p+3] = cells, cells
	b.u32(p+0x1c, 0x80<<24) // sub-picture stream 0; the other 31 are absent
	b.u16(p+0xe6, 0xec)
	b.u16(p+0xe8, 0xec+cells)
	for c := range cells {
		b[p+0xec+c] = byte(c + 1)
		cp := p + 0xec + cells + c*24
		b[cp+5] = 0x01 // one minute, in BCD
		b.u32(cp+8, c*10)
		b.u32(cp+20, c*10+9)
	}
	return b
}

func navSector(button bool) []byte {
	const pciMin, pciHLISS, pciBTNMD, pciBTNNS, pciBTNIT = 0x8e + 36*18, 0x60, 0x6e, 0x71, 0x8e
	pci := make([]byte, pciMin)
	if button {
		pci[pciHLISS+1] = 1
		pci[pciBTNNS] = 1
		pci[pciBTNMD] = 0x10
		o := pciBTNIT
		pci[o] = 1<<6 | byte(100>>4)
		pci[o+1] = byte(100&0x0f)<<4 | byte(200>>8)
		pci[o+2] = byte(200 & 0xff)
		pci[o+3] = byte(50 >> 4)
		pci[o+4] = byte(50&0x0f)<<4 | byte(80>>8)
		pci[o+5] = byte(80 & 0xff)
		pci[o+6], pci[o+7], pci[o+8], pci[o+9] = 1, 1, 1, 1
		copy(pci[o+10:], []byte{0x20, 0x01, 0, 0, 0, 0, 0, 0x02}) // LinkNextCell
	}
	dsi := make([]byte, 0x3fa)
	binary.BigEndian.PutUint32(dsi[8:], 9) // the VOBU ends with the cell
	b := make([]byte, wittydvd.Sector)
	copy(b, []byte{0, 0, 1, 0xba, 0x44})
	at := 14
	for sub, data := range [][]byte{pci, dsi} {
		copy(b[at:], []byte{0, 0, 1, 0xbf})
		binary.BigEndian.PutUint16(b[at+4:], uint16(len(data)+1))
		b[at+6] = byte(sub)
		copy(b[at+7:], data)
		at += 7 + len(data)
	}
	return b
}

// scrambledPack is a video pack with its scrambling flag set.
func scrambledPack() []byte {
	b := make([]byte, wittydvd.Sector)
	copy(b, []byte{0, 0, 1, 0xba, 0x44})
	copy(b[14:], []byte{0, 0, 1, 0xe0, 0x07, 0xec, 0x30})
	return b
}

// makeDisc writes the disc and returns its folder. scrambled packs are put in the cell
// starting at those sectors.
func makeDisc(t *testing.T, scrambledAt ...int) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "MOVIE")
	dir := filepath.Join(root, "VIDEO_TS")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	vob := make([]byte, 20*wittydvd.Sector)
	copy(vob[0:], navSector(true))
	copy(vob[10*wittydvd.Sector:], navSector(false))
	for _, s := range scrambledAt {
		copy(vob[(s+1)*wittydvd.Sector:], scrambledPack())
	}
	for name, b := range map[string][]byte{"VIDEO_TS.IFO": testVMG(), "VTS_01_0.IFO": testVTS(), "VTS_01_1.VOB": vob} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestTheTestDiscPlays(t *testing.T) {
	o, err := wittydvd.Open(makeDisc(t))
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	n := wittydvd.NewNav(o.Disc, wittydvd.NavOptions{})
	st := n.Start()
	if st.Kind != wittydvd.StepTitle || st.CellN != 1 || st.CellDuration != 60 {
		t.Fatalf("%+v", st)
	}
	pic, err := o.Menu(wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: 1}, st.FirstSector, st.LastSector, wittydvd.MenuScanOptions{})
	if err != nil || len(pic.Buttons) != 1 || pic.Buttons[0].Inert {
		t.Fatalf("%+v %v", pic, err)
	}
	c := pic.Buttons[0].Cmd
	if st = n.PressButton(1, &c); st.Kind != wittydvd.StepTitle || st.CellN != 2 {
		t.Fatalf("%+v", st)
	}
}
