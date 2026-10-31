package wittycd

import (
	"os"
	"path/filepath"
	"testing"
)

// Sheets and CD-TEXT are whatever is in someone's folder, so no input may crash
// them. `go test -fuzz=FuzzCue` explores further; the seeds run with every test.

func FuzzCue(f *testing.F) {
	f.Add([]byte("FILE \"a.bin\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"))
	f.Add([]byte("REM SESSION 02\nFILE \"a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\nTRACK 02 AUDIO\nPREGAP 00:02:00\nINDEX 00 00:01:00\nINDEX 01 00:02:00\nINDEX 02 00:03:00\nPOSTGAP 99:00:00\n"))
	f.Add([]byte("FILE \"a.bin\" MOTOROLA\nTRACK 99 CDG\nFLAGS PRE DCP 4CH\nISRC x\nINDEX 01 99:59:74\nFILE \"../../etc/passwd\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"))
	f.Add([]byte("\xff\xfeT\x00R\x00A\x00C\x00K\x00"))
	f.Add([]byte("TRACK 01 AUDIO\nINDEX 01 00:00:00\nFILE \"a.wav\" WAVE\nINDEX 00 00:00:10\n"))
	f.Add(append([]byte("FILE \"a.bin\" BINARY\n"), make([]byte, 64)...))
	f.Fuzz(func(t *testing.T, b []byte) {
		dir := t.TempDir()
		pcm := make([]byte, 200*SectorBytes)
		for i := range pcm {
			pcm[i] = byte(i * 7)
		}
		if err := os.WriteFile(filepath.Join(dir, "a.bin"), pcm, 0o644); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "x.cue")
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		_, _ = SheetFiles(p)
		d, err := Open(p)
		if err != nil {
			return
		}
		defer d.Close()
		exercise(t, d)
	})
}

// exercise reads what an opened disc says it has, the way a player would.
func exercise(t *testing.T, d *Disc) {
	t.Helper()
	_ = d.TOC().MusicBrainzID()
	_ = d.TOC().CDDBID()
	_ = d.Facts()
	s, err := d.NewStream(nil, 1, true)
	if err != nil {
		return
	}
	buf := make([]byte, 8192)
	for _, at := range []int64{0, 3, s.Size() / 2, s.Size() - 1, s.Size()} {
		if at >= 0 {
			_, _ = s.ReadAt(buf, at)
		}
	}
	w := s.WAV(s.Samples() / 2)
	_, _ = w.ReadAt(buf, 0)
	if d.HasGraphics {
		_, _ = d.ReadGraphics(0, 4)
	}
}

func FuzzCDText(f *testing.F) {
	f.Add(makePacks(0x80, "Title", "One", "Two"))
	f.Add(append(makePacks(0x81, "A", "\t", "B"), makePacks(0x8e, "1234567890123", "USTST2600001")...))
	f.Add(append([]byte{0, 0, 0, 0}, makePacks(0x80, "x")...))
	f.Fuzz(func(t *testing.T, b []byte) {
		d := &Disc{Tracks: []Track{{Number: 1}, {Number: 2}}}
		d.parseCDText(b)
	})
}

func FuzzRawImage(f *testing.F) {
	f.Add(make([]byte, 80*SectorBytes))
	f.Add(tone(0, 80, 440))
	f.Fuzz(func(t *testing.T, b []byte) {
		p := filepath.Join(t.TempDir(), "x.iso")
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		d, err := Open(p)
		if err != nil {
			return
		}
		defer d.Close()
		exercise(t, d)
	})
}
