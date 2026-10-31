package imaging

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaplayer/engine/internal/disc"
)

type memDisc struct {
	*bytes.Reader
	// bad sectors fail to read, as a scratched disc does.
	bad map[int64]bool
	// tries counts reads, to check retries happen.
	tries int
}

func (m *memDisc) Size() int64 { return m.Reader.Size() }

func (m *memDisc) ReadAt(p []byte, off int64) (int, error) {
	m.tries++
	for s := off / sector; s*sector < off+int64(len(p)); s++ {
		if m.bad[s] {
			return 0, errors.New("the drive couldn't read this sector")
		}
	}
	return m.Reader.ReadAt(p, off)
}

func patterned(sectors int) []byte {
	b := make([]byte, sectors*sector)
	for s := 0; s < sectors; s++ {
		for i := 0; i < sector; i++ {
			b[s*sector+i] = byte(s + i)
		}
	}
	return b
}

func TestCopyIsExact(t *testing.T) {
	data := patterned(500)
	out := filepath.Join(t.TempDir(), "disc.iso")
	var reports int
	res, err := Copy(context.Background(), &memDisc{Reader: bytes.NewReader(data)}, Options{Out: out, Every: 1, Progress: func(Progress) { reports++ }})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Error("the copy differs from the disc")
	}
	if res.Done != int64(len(data)) || res.BadSectors != 0 || reports == 0 {
		t.Errorf("result %+v, %d reports", res, reports)
	}
	if _, err := os.Stat(out + ".part"); err == nil {
		t.Error("the temporary file was left behind")
	}
}

func TestScratchedSectorsBecomeZerosAndAreCounted(t *testing.T) {
	data := patterned(200)
	src := &memDisc{Reader: bytes.NewReader(data), bad: map[int64]bool{70: true, 71: true}}
	out := filepath.Join(t.TempDir(), "disc.img")
	res, err := Copy(context.Background(), src, Options{Out: out})
	if err != nil {
		t.Fatal(err)
	}
	if res.BadSectors != 2 {
		t.Errorf("%d bad sectors", res.BadSectors)
	}
	got, _ := os.ReadFile(out)
	want := bytes.Clone(data)
	clear(want[70*sector : 72*sector])
	if !bytes.Equal(got, want) {
		t.Error("everything readable should be there, and the rest zeros")
	}
}

func TestDeadDiscStops(t *testing.T) {
	bad := map[int64]bool{}
	for s := int64(0); s < maxBad+200; s++ {
		bad[s] = true
	}
	src := &memDisc{Reader: bytes.NewReader(make([]byte, (maxBad+200)*sector)), bad: bad}
	out := filepath.Join(t.TempDir(), "disc.iso")
	if _, err := Copy(context.Background(), src, Options{Out: out}); err == nil || !strings.Contains(err.Error(), "too much") {
		t.Fatalf("copying a dead disc said %v", err)
	}
	assertNothingLeft(t, out)
}

func assertNothingLeft(t *testing.T, out string) {
	t.Helper()
	for _, p := range []string{out, out + ".part"} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s was left behind", filepath.Base(p))
		}
	}
}

// A scrambled sector anywhere on a DVD stops the copy, and leaves nothing.
func TestScrambledSectorStopsTheCopy(t *testing.T) {
	data := make([]byte, 300*sector)
	pack := data[150*sector:]
	copy(pack, []byte{0, 0, 1, 0xba, 0x44})
	copy(pack[14:], []byte{0, 0, 1, 0xe0, 0x07, 0xec, 0x30})
	out := filepath.Join(t.TempDir(), "disc.iso")
	_, err := Copy(context.Background(), &memDisc{Reader: bytes.NewReader(data)}, Options{Out: out, Kind: disc.KindDVD})
	if !disc.IsProtected(err) {
		t.Fatalf("copying a scrambled disc said %v", err)
	}
	assertNothingLeft(t, out)
}

func TestNeverOverwrites(t *testing.T) {
	out := filepath.Join(t.TempDir(), "disc.iso")
	if err := os.WriteFile(out, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Copy(context.Background(), &memDisc{Reader: bytes.NewReader(patterned(10))}, Options{Out: out})
	if !errors.Is(err, ErrExists) {
		t.Fatalf("said %v", err)
	}
	if b, _ := os.ReadFile(out); string(b) != "precious" {
		t.Error("the existing file was changed")
	}
}

func TestDestinationChecks(t *testing.T) {
	dir := t.TempDir()
	if err := CheckOut(filepath.Join(dir, "disc.mkv"), 2048); !errors.Is(err, ErrFormat) {
		t.Errorf("wrong extension: %v", err)
	}
	if err := CheckOut(filepath.Join(dir, "missing", "disc.iso"), 2048); err == nil {
		t.Error("a missing folder was accepted")
	}
	if err := CheckOut(filepath.Join(dir, "disc.ISO"), 2048); err != nil {
		t.Errorf("a good name: %v", err)
	}
	if free, ok := freeSpace(dir); ok {
		if err := CheckOut(filepath.Join(dir, "disc.iso"), int64(free)); err == nil || !strings.Contains(err.Error(), "enough room") {
			t.Errorf("a disc bigger than the free space: %v", err)
		}
	}
}

func TestCancelRemovesThePartialFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := filepath.Join(t.TempDir(), "disc.iso")
	if _, err := Copy(ctx, &memDisc{Reader: bytes.NewReader(patterned(100))}, Options{Out: out}); !errors.Is(err, context.Canceled) {
		t.Fatalf("said %v", err)
	}
	assertNothingLeft(t, out)
}

// A real disc image copied through the same path as a drive is byte for byte the
// same, and the engines open the copy.
func TestRealImageRoundTrip(t *testing.T) {
	path := os.Getenv("WITTY_TEST_DVD")
	if path == "" {
		t.Skip("WITTY_TEST_DVD is not set")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	out := filepath.Join(t.TempDir(), "copy.iso")
	res, err := Copy(context.Background(), &fileSrc{f, st.Size()}, Options{Out: out, Kind: disc.KindDVD})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified {
		t.Error("the engines couldn't open the copy")
	}
	a, _ := os.Open(out)
	defer a.Close()
	buf1, buf2 := make([]byte, 1<<20), make([]byte, 1<<20)
	var off int64
	for {
		n1, err1 := io.ReadFull(a, buf1)
		n2, _ := f.ReadAt(buf2[:n1], off)
		if n1 != n2 || !bytes.Equal(buf1[:n1], buf2[:n2]) {
			t.Fatalf("differs near byte %d", off)
		}
		off += int64(n1)
		if err1 != nil {
			break
		}
	}
	if off != st.Size() {
		t.Errorf("%d bytes copied of %d", off, st.Size())
	}
}

type fileSrc struct {
	*os.File
	size int64
}

func (f *fileSrc) Size() int64 { return f.size }
