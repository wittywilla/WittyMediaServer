package wittycd

import (
	"os"
	"path/filepath"
	"testing"
)

// Each target opens whatever the fuzzer writes and reads a little of what comes
// back, next to a small file for it to find. All that is asked is that nothing
// panics, hangs or asks for more memory than a sheet can justify.

// fuzzSectors is the sibling file: short enough to be quick, and not all one value.
// It ends part way through a sector, as a damaged file may.
func fuzzSectors() []byte {
	b := make([]byte, 40*SectorBytes+123)
	for i := range b {
		b[i] = byte(i*7 + i>>8)
	}
	return b
}

func fuzzOpen(t *testing.T, path string) {
	t.Helper()
	d, err := Open(path)
	if err != nil {
		return
	}
	defer d.Close()
	d.TOC()
	d.DurationSec()
	s, err := d.NewStream(nil, 1, false)
	if err != nil {
		return
	}
	buf := make([]byte, 3*SectorBytes)
	s.ReadAt(buf, 0)
	s.ReadAt(buf, s.Size()/2+1)
	s.ReadAt(buf, max(0, s.Size()-int64(len(buf))))
	d.ReadGraphics(0, 4)
	if audio := d.AudioTracks(); len(audio) > 0 {
		if one, err := d.NewStream([]int{audio[0].Number}, 1, true); err == nil {
			one.ReadAt(buf[:SectorBytes], 0)
		}
	}
}

func FuzzTocFile(f *testing.F) {
	f.Add([]byte(tocSheet))
	f.Add([]byte("TRACK AUDIO\nFILE \"x.bin\" 0\n"))
	f.Add([]byte("CD_ROM\nTRACK MODE1 RW\nDATAFILE \"x.bin\" #16 00:00:10\nZERO MODE1 00:02:00\nTRACK AUDIO SWAP\nPREGAP 0:2:0\nFILE \"x.bin\" SWAP #4 588 1176\nINDEX 00:00:01\n"))
	f.Add([]byte("TRACK AUDIO\nSTART 99:59:74\nSILENCE 26460000\nFIFO \"x.bin\" 1\n"))
	f.Add([]byte("CD_TEXT{LANGUAGE 0{TITLE\"\\101\\\"\"GENRE{1,2}}}"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<16 {
			return
		}
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "x.bin"), fuzzSectors(), 0o644); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "x.toc")
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		fuzzOpen(t, p)
	})
}

func FuzzCCD(f *testing.F) {
	f.Add([]byte(ccdEntryText(0, 1, 1, 0, 0) + ccdEntryText(1, 1, 2, 4, 20) + ccdEntryText(2, 1, 0xa2, 0, 40)))
	f.Add([]byte("[Disc]\nCatalog=123\nDataTracksScrambled=1\n" + ccdEntryText(0, 1, 1, 0, 0) + ccdEntryText(1, 1, 0xa2, 0, 20) +
		ccdEntryText(2, 2, 2, 0, 20+sessionGap) + ccdEntryText(3, 2, 0xa2, 0, 40+sessionGap) + "[TRACK 2]\nMODE=2\nINDEX 0=11410\nINDEX 1=11420\nINDEX 2=11425\n"))
	f.Add([]byte("[Entry 0]\nPoint=0x01\nPMin=0\nPSec=2\nPFrame=0\n[CDText]\nEntries=1\nEntry 0=80 00 00 00 41 41 41 41 41 41 41 41 41 41 41 41\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<16 {
			return
		}
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "x.img"), fuzzSectors(), 0o644); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "x.ccd")
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		fuzzOpen(t, p)
	})
}

func FuzzNRG(f *testing.F) {
	pcm := fuzzSectors()[:4*SectorBytes]
	lim := int64(len(pcm))
	d := &nrgBuilder{sectors: pcm}
	d.chunk("CUEX", cuex(nrgCueEntry{0x01, 0, 0, -150}, nrgCueEntry{0x01, 1, 0, -150}, nrgCueEntry{0x01, 1, 1, 0}, nrgCueEntry{0x11, 2, 0, 1}, nrgCueEntry{0x11, 2, 1, 2}, nrgCueEntry{0x01, 0xaa, 1, 4}))
	d.chunk("DAOX", dao(true, "1234567890123", 1, 2, nrgDaoTrack{"USTST2600001", 2352, 7, 0, 0, SectorBytes}, nrgDaoTrack{"", 2352, 7, SectorBytes, 2 * SectorBytes, lim}))
	d.chunk("CDTX", makePacks(0x80, "T", "A", "B")).chunk("SINF", []byte{0, 0, 0, 2})
	f.Add(d.bytes())
	v1 := &nrgBuilder{sectors: pcm, v1: true}
	v1.chunk("CUES", cues(nrgCueEntry{0x01, 1, 1, 0}, nrgCueEntry{0x01, 0xaa, 1, 4}))
	v1.chunk("DAOI", dao(false, "", 1, 1, nrgDaoTrack{"", 2352, 7, 0, 0, lim}))
	f.Add(v1.bytes())
	etn := &nrgBuilder{sectors: pcm}
	etn.chunk("ETN2", make([]byte, 32)).chunk("ETNF", make([]byte, 20)).chunk("TINF", make([]byte, 12))
	f.Add(etn.bytes())
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<18 {
			return
		}
		p := filepath.Join(t.TempDir(), "x.nrg")
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		fuzzOpen(t, p)
	})
}
