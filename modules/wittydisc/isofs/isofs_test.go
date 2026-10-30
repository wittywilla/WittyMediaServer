package isofs

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// isoImage builds a small ISO 9660 image: a root directory holding one file
// and any extra directory records given.
func isoImage(extra ...[]byte) []byte {
	img := make([]byte, 32*Sector)
	put32 := func(b []byte, v uint32) { binary.LittleEndian.PutUint32(b, v) }
	rec := func(name string, flags byte, lba, size uint32) []byte {
		l := 33 + len(name)
		if l%2 == 1 {
			l++
		}
		r := make([]byte, l)
		r[0] = byte(l)
		put32(r[2:], lba)
		put32(r[10:], size)
		r[25] = flags
		r[32] = byte(len(name))
		copy(r[33:], name)
		return r
	}
	pvd := img[16*Sector:]
	pvd[0] = 1
	copy(pvd[1:], "CD001")
	copy(pvd[40:], "TEST_DISC")
	root := rec("\x00", 2, 20, Sector)
	copy(pvd[156:], root)
	term := img[17*Sector:]
	term[0] = 255
	copy(term[1:], "CD001")

	dir := img[20*Sector:]
	off := copy(dir, rec("\x00", 2, 20, Sector))
	off += copy(dir[off:], rec("\x01", 2, 20, Sector))
	off += copy(dir[off:], rec("HELLO.TXT;1", 0, 21, 5))
	for _, e := range extra {
		off += copy(dir[off:], e)
	}
	copy(img[21*Sector:], "hello")
	return img
}

func openBytes(t testing.TB, b []byte) (*Disc, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "disc.iso")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return Open(p)
}

func TestISO9660(t *testing.T) {
	d, err := openBytes(t, isoImage())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Format != "iso9660" || d.Label != "TEST_DISC" {
		t.Errorf("format %q, label %q", d.Format, d.Label)
	}
	b, err := d.ReadFile("hello.txt", 0)
	if err != nil || string(b) != "hello" {
		t.Errorf("read %q, %v", b, err)
	}
	if _, err := d.ReadFile("nope.txt", 0); err == nil {
		t.Error("a missing file was found")
	}
}

// A directory that contains itself must end, and quickly: this image makes
// every level of the walk look at the same 40 subdirectories.
func TestDirectoryLoopEnds(t *testing.T) {
	var loops [][]byte
	for i := 0; i < 40; i++ {
		name := string(rune('A'+i%26)) + string(rune('A'+i/26))
		l := 33 + len(name)
		if l%2 == 1 {
			l++
		}
		r := make([]byte, l)
		r[0] = byte(l)
		binary.LittleEndian.PutUint32(r[2:], 20)
		binary.LittleEndian.PutUint32(r[10:], Sector)
		r[25] = 2
		r[32] = byte(len(name))
		copy(r[33:], name)
		loops = append(loops, r)
	}
	d, err := openBytes(t, isoImage(loops...))
	if err == nil {
		defer d.Close()
		if n := len(d.Files()); n > maxEntries+1 {
			t.Fatalf("%d entries", n)
		}
	}
}

// Sizes in a damaged or hostile image can be anything: negative after a cast,
// or larger than the image. None may panic or allocate without limit.
func TestReadFileWithHostileSize(t *testing.T) {
	d, err := openBytes(t, isoImage())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	f, _ := d.Find("HELLO.TXT")
	f.Size = -1
	if b, _ := d.ReadFile("HELLO.TXT", 0); len(b) != 0 {
		t.Errorf("read %d bytes", len(b))
	}
	f.Size = 1 << 60
	if b, _ := d.ReadFile("HELLO.TXT", 0); len(b) > 5 {
		t.Errorf("read %d bytes", len(b))
	}
}

func TestUDFEntryWithHostileLengths(t *testing.T) {
	// A file entry whose allocation lengths don't fit a sector.
	img := make([]byte, 300*Sector)
	d, err := openBytes(t, img)
	if err == nil {
		d.Close()
	}
	p := filepath.Join(t.TempDir(), "x.iso")
	if err := os.WriteFile(p, img, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dd := &Disc{Path: p, f: f, size: int64(len(img)), files: map[string]*File{}}
	ctx := &udfCtx{parts: []udfPartition{{number: 0}}}
	sec := img[5*Sector : 6*Sector]
	binary.LittleEndian.PutUint16(sec, 261)
	binary.LittleEndian.PutUint32(sec[168:], 0xffffffff)
	binary.LittleEndian.PutUint32(sec[172:], 0xffffffff)
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	w, err := os.OpenFile(p, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteAt(sec, 5*Sector)
	w.Close()
	if _, err := dd.readUDFEntry(ctx, longAD{block: 5}); err == nil {
		t.Error("impossible lengths accepted")
	}
	// A directory whose size is negative once read as a signed number.
	binary.LittleEndian.PutUint32(sec[168:], 0)
	binary.LittleEndian.PutUint32(sec[172:], 0)
	binary.LittleEndian.PutUint64(sec[56:], 0xffffffffffffffff)
	sec[16+11] = 4
	w, err = os.OpenFile(p, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteAt(sec, 5*Sector)
	w.Close()
	if err := dd.walkUDFDir(ctx, longAD{block: 5}, "", 0); err == nil {
		t.Error("a directory of negative size was read")
	}
}

func FuzzOpen(f *testing.F) {
	f.Add(isoImage())
	f.Add(make([]byte, 300*Sector))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		d, err := openBytes(t, b)
		if err != nil {
			return
		}
		defer d.Close()
		for _, x := range d.Files() {
			if !x.IsDir && x.Size < 1<<20 {
				_, _ = d.ReadFile(x.Path, 1<<20)
			}
		}
	})
}
