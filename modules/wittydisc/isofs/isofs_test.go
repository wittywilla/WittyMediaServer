package isofs

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// isoDirRecord builds one ISO 9660 directory record.
func isoDirRecord(name string, lba, size uint32, dir bool) []byte {
	l := 33 + len(name)
	if l%2 == 1 {
		l++
	}
	r := make([]byte, l)
	r[0] = byte(l)
	binary.LittleEndian.PutUint32(r[2:], lba)
	binary.LittleEndian.PutUint32(r[10:], size)
	if dir {
		r[25] = 2
	}
	r[32] = byte(len(name))
	copy(r[33:], name)
	return r
}

// isoImage builds an image whose root directory (sector 20) holds entries.
func isoImage(entries ...[]byte) []byte {
	img := make([]byte, 64*Sector)
	pvd := img[16*Sector:]
	pvd[0] = 1
	copy(pvd[1:], "CD001")
	copy(pvd[156:], isoDirRecord("\x00", 20, Sector, true)[:34])
	end := img[17*Sector:]
	end[0] = 255
	copy(end[1:], "CD001")
	off := 20 * Sector
	for _, e := range entries {
		off += copy(img[off:], e)
	}
	return img
}

type memImage struct{ *bytes.Reader }

func (memImage) Close() error { return nil }

func openBytes(t testing.TB, b []byte) (*Disc, error) {
	t.Helper()
	d := &Disc{Path: "test", f: memImage{bytes.NewReader(b)}, size: int64(len(b)), files: map[string]*File{}}
	if err := d.index(); err != nil {
		return nil, err
	}
	return d, nil
}

func TestOpenFromFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.iso")
	if err := os.WriteFile(p, isoImage(isoDirRecord("A;1", 30, 4, false)), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, ok := d.Find("A"); !ok {
		t.Fatal("file not found")
	}
}

func TestOpenISO(t *testing.T) {
	img := isoImage(isoDirRecord("VIDEO_TS.IFO;1", 30, 100, false))
	copy(img[30*Sector:], "hello")
	d, err := openBytes(t, img)
	if err != nil {
		t.Fatal(err)
	}
	b, err := d.ReadFile("VIDEO_TS.IFO", 5)
	if err != nil || string(b) != "hello" {
		t.Fatalf("read %q, %v", b, err)
	}
}

// A directory listing itself many times: walking every path through it is
// exponential (40 entries, 16 levels deep), so a 128 KB file could hang a scan
// for ever.
func TestDirectoryLoopsDontExplode(t *testing.T) {
	var entries [][]byte
	for i := 0; i < 40; i++ {
		entries = append(entries, isoDirRecord(string(rune('A'+i%26))+string(rune('A'+i/26)), 20, Sector, true))
	}
	img := isoImage(entries...)
	done := make(chan error, 1)
	go func() {
		_, err := openBytes(t, img)
		done <- err
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("a small image with a directory loop keeps the walk going")
	}
}

// A file can claim any size; reading it must not be able to allocate that.
func TestReadFileClaimingHugeSize(t *testing.T) {
	d, err := openBytes(t, isoImage(isoDirRecord("X;1", 30, 0xfffffff0, false)))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := d.ReadFile("X", 0)
	if int64(len(b)) > d.Size() {
		t.Fatalf("read %d bytes from a %d byte image", len(b), d.Size())
	}
	f, _ := d.Find("X")
	f.Size = -1
	if b, _ := d.ReadFile("X", 0); len(b) != 0 {
		t.Fatalf("negative size: %d bytes", len(b))
	}
}

// udfImage builds the skeleton of a UDF image: anchor, volume descriptors and
// file set, with the root's file entry at block 3 of a partition at sector 300.
func udfImage(rootEntry func(b []byte)) []byte {
	img := make([]byte, 400*Sector)
	avdp := img[256*Sector:]
	binary.LittleEndian.PutUint16(avdp, 2)
	binary.LittleEndian.PutUint32(avdp[16:], 4*Sector)
	binary.LittleEndian.PutUint32(avdp[20:], 260)
	pd := img[260*Sector:]
	binary.LittleEndian.PutUint16(pd, 5)
	binary.LittleEndian.PutUint32(pd[188:], 300)
	binary.LittleEndian.PutUint32(pd[192:], 100)
	lvd := img[261*Sector:]
	binary.LittleEndian.PutUint16(lvd, 6)
	binary.LittleEndian.PutUint32(lvd[252:], 1) // file set at block 1
	binary.LittleEndian.PutUint16(img[262*Sector:], 8)
	fsd := img[301*Sector:]
	binary.LittleEndian.PutUint16(fsd, 256)
	binary.LittleEndian.PutUint32(fsd[404:], 3) // root ICB at block 3
	rootEntry(img[303*Sector:])
	return img
}

// A directory's size comes straight from the image and is signed once read.
func TestUDFNegativeSizes(t *testing.T) {
	img := udfImage(func(b []byte) {
		binary.LittleEndian.PutUint16(b, 261)
		b[16+11] = 4 // a directory
		binary.LittleEndian.PutUint64(b[56:], 1<<63)
	})
	if _, err := openBytes(t, img); err == nil {
		t.Log("opened")
	}
}

// A descriptor length that wraps on 32-bit systems must not panic there.
func TestUDFHugeDescriptorLengths(t *testing.T) {
	img := udfImage(func(b []byte) {
		binary.LittleEndian.PutUint16(b, 261)
		b[16+11] = 4
		binary.LittleEndian.PutUint32(b[168:], 0xfffffff0)
		binary.LittleEndian.PutUint32(b[172:], 0xfffffff0)
	})
	if _, err := openBytes(t, img); err == nil {
		t.Log("opened")
	}
}

// Disc images come from anywhere, and no image may crash or hang the server.
// `go test -fuzz=FuzzOpen` explores further; the seeds run with every test.
func FuzzOpen(f *testing.F) {
	f.Add(isoImage(isoDirRecord("VIDEO_TS.IFO;1", 30, 100, false)), "VIDEO_TS.IFO")
	f.Add(isoImage(isoDirRecord("A", 20, Sector, true), isoDirRecord("B;1", 0xffffffff, 0xffffffff, false)), "B")
	f.Add(udfImage(func(b []byte) {
		binary.LittleEndian.PutUint16(b, 266)
		b[16+11] = 4
	}), "")
	f.Fuzz(func(t *testing.T, b []byte, name string) {
		d, err := openBytes(t, b)
		if err != nil {
			return
		}
		for _, x := range d.Files() {
			buf, _ := d.ReadFile(x.Path, 4096)
			_ = buf
			if x.Size > 0 {
				one := make([]byte, 16)
				_, _ = d.ReadAtFile(x, one, x.Size/2)
			}
		}
		_, _ = d.ReadFile(name, 0)
		_, _ = d.ReadSectors(1, 1)
	})
}
