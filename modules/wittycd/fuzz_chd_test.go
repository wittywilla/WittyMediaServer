package wittycd

import (
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// These open whatever the fuzzer writes and read a little of what comes back. All
// that is asked is that nothing panics, hangs or asks for far too much memory.

func fuzzSeedFile(f *testing.F, name string) {
	f.Helper()
	if b, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
		f.Add(b)
	}
}

func FuzzCHD(f *testing.F) {
	audio := framesOf(intTone(0, 16))
	f.Add((&chdImage{frames: audio, meta: []string{trackLine(1, "AUDIO", 16, 0, "NONE", 0)}}).build())
	f.Add((&chdImage{frames: audio, meta: []string{trackLine(1, "MODE1_RAW", 4, 2, "VMODE1_RAW", 0), trackLine(2, "AUDIO", 12, 150, "AUDIO", 8)}}).build())
	for _, name := range []string{"tones_cdlz.chd", "tones_cdzl.chd", "tones_cdfl.chd", "mixed.chd"} {
		fuzzSeedFile(f, name)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		p := filepath.Join(t.TempDir(), "x.chd")
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Skip()
		}
		d, err := Open(p)
		if err != nil {
			return
		}
		defer d.Close()
		s, err := d.NewStream(nil, 1, false)
		if err != nil {
			return
		}
		buf := make([]byte, 20000)
		s.ReadAt(buf, 0)
		s.ReadAt(buf, s.Size()/2)
		d.ReadGraphics(0, 4)
	})
}

func FuzzFlacSource(f *testing.F) {
	pcm := intTone(0, 40)
	f.Add(flacOf(f, pcm[:4*SectorBytes], 0))
	f.Add(flacOf(f, pcm[:SectorBytes], 0))
	f.Add(flacOf(f, pcm, 0))
	fuzzSeedFile(f, "lpc.flac")
	f.Fuzz(func(t *testing.T, b []byte) {
		r, n, err := newFlacSource(bytesReaderAt(b), int64(len(b)))
		if err != nil {
			return
		}
		if n%SampleBytes != 0 || n < 0 || n > MaxSectors*SectorBytes {
			t.Fatalf("length %d", n)
		}
		buf := make([]byte, 20000)
		for _, off := range []int64{0, n / 3, n - 100, n / 2, 1} {
			if m, err := r.ReadAt(buf, max(off, 0)); err != nil && err != io.EOF || m < 0 || m > len(buf) {
				t.Fatalf("read %d: %v", m, err)
			}
		}
	})
}

// FuzzCHDHunk goes straight to the decompressors: a map's checksum rarely survives a
// change, so few changed files get this far by the front door.
func FuzzCHDHunk(f *testing.F) {
	f.Add(byte(0), []byte{0, 0, 0})
	for _, name := range []string{"tones_cdlz.chd", "tones_cdzl.chd", "tones_cdfl.chd"} {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			continue
		}
		c, err := openCHD(bytesReaderAt(b), int64(len(b)))
		if err != nil || c.hunkBytes == 0 || c.readMap() != nil {
			continue
		}
		e := c.entries[0]
		for i, id := range chdFuzzCodecs {
			if e.kind <= chdType3 && id == c.codecs[e.kind] && uint64(len(b)) >= e.offset+uint64(e.length) {
				f.Add(byte(i), b[e.offset:e.offset+uint64(e.length)])
			}
		}
	}
	c := &chdFile{hunkBytes: chdFrameBytes * 8}
	dst := make([]byte, c.hunkBytes)
	f.Fuzz(func(t *testing.T, which byte, src []byte) {
		c.decompress(chdFuzzCodecs[int(which)%len(chdFuzzCodecs)], src, dst)
	})
}

var chdFuzzCodecs = []uint32{chdCDLZ, chdCDZL, chdCDFL, chdZLIB, chdLZMA, chdFLAC, chdHUFF}

type bytesReaderAt []byte

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func FuzzLZMA(f *testing.F) {
	if v, err := hex.DecodeString(lzmaVector); err == nil {
		f.Add(uint16(len(lzmaPlain())), byte(3), byte(0), byte(2), v)
	}
	f.Add(uint16(16), byte(3), byte(0), byte(2), []byte{0, 0, 0, 0, 0, 0})
	f.Add(uint16(100), byte(0), byte(4), byte(4), []byte{0, 255, 255, 255, 255, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	var z lzmaDecoder
	f.Fuzz(func(t *testing.T, size uint16, lc, lp, pb byte, src []byte) {
		dst := make([]byte, size)
		z.decode(dst, src, uint(lc), uint(lp), uint(pb))
	})
}
