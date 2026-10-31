package gamearchive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
	"github.com/wittywilla/wittyplay"
)

// testdata holds archives made by the real tools (7-Zip and RAR), since nothing in
// Go writes those. Everything in them is random bytes and made-up names. See
// testdata/README.md. The rest are generated here.

func entryNames(l *Listing) []string {
	var out []string
	for _, e := range l.Entries {
		out = append(out, e.Name)
	}
	sort.Strings(out)
	return out
}

func join(s []string) string { return strings.Join(s, " | ") }

type zipFile struct {
	name string
	data []byte
	mode os.FileMode
}

func writeZip(t *testing.T, p string, files ...zipFile) {
	t.Helper()
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for _, z := range files {
		h := &zip.FileHeader{Name: z.name, Method: zip.Deflate}
		if z.mode != 0 {
			h.SetMode(z.mode)
		}
		e, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = e.Write(z.data)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

type tarFile struct {
	name string
	data []byte
	kind byte
	link string
}

func tarBytes(t *testing.T, files ...tarFile) []byte {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for _, f := range files {
		kind := f.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		if err := w.WriteHeader(&tar.Header{Name: f.name, Typeflag: kind, Linkname: f.link, Size: int64(len(f.data)), Mode: 0o644}); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(f.data)
	}
	_ = w.Close()
	return b.Bytes()
}

func gzBytes(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Name = name
	_, _ = w.Write(data)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func write(t *testing.T, p string, data []byte) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestListsEveryFormat(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "a.zip")
	writeZip(t, zp, zipFile{name: "Game.sfc", data: []byte("rom")}, zipFile{name: "sub/", data: nil}, zipFile{name: "__MACOSX/._Game.sfc", data: []byte("x")})
	tarData := tarBytes(t, tarFile{name: "Game.sfc", data: []byte("rom")}, tarFile{name: "folder", kind: tar.TypeDir})
	for _, c := range []struct{ path, want string }{
		{"testdata/cart.7z", "Game (USA).sfc | readme.txt"},
		{"testdata/cart.rar", "Game (USA).sfc | readme.txt"},
		{"testdata/cart-solid.rar", "Game (USA).sfc | readme.txt"},
		{"testdata/disc-solid.7z", "Game (Track 1).bin | Game (Track 2).bin | Game.cue | readme.txt"},
		{"testdata/disc-nested.7z", "inner/Game (Track 1).bin | inner/Game (Track 2).bin | inner/Game.cue"},
		{"testdata/split.part1.rar", "Game.sfc"},
		{zp, "Game.sfc"},
		{write(t, filepath.Join(dir, "a.tar"), tarData), "Game.sfc"},
		{write(t, filepath.Join(dir, "a.tar.gz"), gzBytes(t, tarData, "")), "Game.sfc"},
		{write(t, filepath.Join(dir, "a.tgz"), gzBytes(t, tarData, "")), "Game.sfc"},
		{write(t, filepath.Join(dir, "b.sfc.gz"), gzBytes(t, []byte("rom"), "")), "b.sfc"},
		{write(t, filepath.Join(dir, "c.gz"), gzBytes(t, []byte("rom"), "../../inner.smc")), "c.smc"},
	} {
		l, err := List(c.path)
		if err != nil {
			t.Errorf("%s: %v", c.path, err)
			continue
		}
		if got := join(entryNames(l)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.path, got, c.want)
		}
	}
}

func xzBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w, err := xz.NewWriter(&b)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(data)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func zstBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	w, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	return w.EncodeAll(data, nil)
}

func TestEveryCompressionPlays(t *testing.T) {
	dir := t.TempDir()
	rom := []byte("the game")
	tarData := tarBytes(t, tarFile{name: "Game.sfc", data: rom})
	bz2Rom, err := os.ReadFile("testdata/single.sfc.bz2")
	if err != nil {
		t.Fatal(err)
	}
	bz2Tar, err := os.ReadFile("testdata/cart.tar.bz2")
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"a.sfc.xz": xzBytes(t, rom), "a.tar.xz": xzBytes(t, tarData), "a.txz": xzBytes(t, tarData),
		"b.sfc.zst": zstBytes(t, rom), "b.tar.zst": zstBytes(t, tarData), "b.tzst": zstBytes(t, tarData),
		"c.sfc.bz2": bz2Rom, "c.tar.bz2": bz2Tar, "c.tbz2": bz2Tar, "c.tbz": bz2Tar,
	} {
		p := write(t, filepath.Join(dir, name), data)
		c := newCache(t, 1<<30)
		got, release, err := c.Acquire(context.Background(), p, wittyplay.ByID("snes"))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if filepath.Base(got) != "game.sfc" {
			t.Errorf("%s: %s", name, got)
		}
		if b, _ := os.ReadFile(got); strings.HasPrefix(name, "c.") == bytes.Equal(b, rom) {
			t.Errorf("%s: %d bytes", name, len(b)) // the bzip2 files were made elsewhere, of random bytes
		}
		release()
	}
	// Named one way, made another.
	wrong := write(t, filepath.Join(dir, "wrong.sfc.gz"), xzBytes(t, rom))
	if l, err := List(wrong); err != nil || join(entryNames(l)) != "wrong.sfc" {
		t.Errorf("mislabelled: %v %v", l, err)
	}
	for _, name := range []string{"rubbish.sfc.xz", "rubbish.tar.xz", "rubbish.sfc.zst", "rubbish.sfc.bz2"} {
		p := write(t, filepath.Join(dir, name), []byte("not compressed at all"))
		if _, err := List(p); !errors.Is(err, ErrDamaged) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A bomb stops at the limit however it was packed.
	bomb := write(t, filepath.Join(dir, "bomb.sfc.zst"), zstBytes(t, make([]byte, 8<<20)))
	_, err = unpackIn(context.Background(), unpackRequest{Path: bomb, Dst: t.TempDir(), Want: []Want{{Name: "bomb.sfc", Out: "game.sfc"}}, Limit: 1 << 20}, nil)
	if !errors.Is(err, ErrTooBig) {
		t.Errorf("bomb: %v", err)
	}
}

func TestSplit7zSetsOpenThroughTheFirstPart(t *testing.T) {
	src := t.TempDir()
	first := copyFixture(t, src, "split7.7z.001")
	copyFixture(t, src, "split7.7z.002")
	copyFixture(t, src, "split7.7z.003")
	l, err := List(first)
	if err != nil || join(entryNames(l)) != "Game.sfc" {
		t.Fatalf("%v %v", l, err)
	}
	c := newCache(t, 1<<30)
	rom, release, err := c.Acquire(context.Background(), first, wittyplay.ByID("snes"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if st, err := os.Stat(rom); err != nil || st.Size() != 300000 {
		t.Fatalf("%v %v", st, err)
	}
	// Without all its parts it says so, rather than playing part of a game.
	if err := os.Remove(filepath.Join(src, "split7.7z.003")); err != nil {
		t.Fatal(err)
	}
	if _, err := List(first); err == nil {
		t.Log("a missing last part only shows once it's read")
	}
	future := time.Now().Add(time.Hour)
	_ = os.Chtimes(first, future, future)
	if _, _, err := c.Acquire(context.Background(), first, wittyplay.ByID("snes")); err == nil {
		t.Fatal("played a set with a part missing")
	}
}

func TestLinksAreNeverListed(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "a.zip")
	writeZip(t, zp, zipFile{name: "Game.sfc", data: []byte("rom")}, zipFile{name: "evil.sfc", data: []byte("/etc/passwd"), mode: os.ModeSymlink | 0o777})
	tp := write(t, filepath.Join(dir, "a.tar"), tarBytes(t, tarFile{name: "Game.sfc", data: []byte("rom")},
		tarFile{name: "evil.sfc", kind: tar.TypeSymlink, link: "/etc/passwd"}, tarFile{name: "hard.sfc", kind: tar.TypeLink, link: "../../x"}))
	for _, p := range []string{"testdata/link.7z", "testdata/link.rar", zp, tp} {
		l, err := List(p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		if got := join(entryNames(l)); got != "Game.sfc" {
			t.Errorf("%s: got %q", p, got)
		}
	}
}

func TestPasswordProtectedArchivesAreSaidToBe(t *testing.T) {
	for _, p := range []string{"testdata/locked-names.7z", "testdata/locked-names.rar", "testdata/locked-content.rar", "testdata/locked.zip"} {
		if _, err := List(p); !errors.Is(err, ErrEncrypted) {
			t.Errorf("%s: got %v", p, err)
		}
	}
	// A 7z can keep its names readable and lock only its contents. That can't be seen
	// from its headers, and reading a byte of it means decoding a whole block, so it's
	// listed like any other and says so when played: see TestClearAnswersForWhatCantBePlayed.
	if _, err := List("testdata/locked-content.7z"); err != nil {
		t.Errorf("locked contents: got %v", err)
	}
}

func TestNamesThatClimbOutAreRefused(t *testing.T) {
	dir := t.TempDir()
	evil := []string{"../evil.sfc", "..\\evil.sfc", "a/../../evil.sfc", "/etc/evil.sfc", "C:\\evil.sfc", "C:/evil.sfc", "a\x00b.sfc", "\\\\server\\share\\evil.sfc"}
	for i, name := range evil {
		zp := filepath.Join(dir, fmt.Sprintf("z%d.zip", i))
		writeZip(t, zp, zipFile{name: "good.sfc", data: []byte("x")}, zipFile{name: name, data: []byte("x")})
		if _, err := List(zp); !errors.Is(err, ErrUnsafe) {
			t.Errorf("zip %q: got %v", name, err)
		}
		if strings.ContainsRune(name, 0) {
			continue // a tar can't even be written with one
		}
		tp := write(t, filepath.Join(dir, fmt.Sprintf("t%d.tar", i)), tarBytes(t, tarFile{name: name, data: []byte("x")}))
		if _, err := List(tp); !errors.Is(err, ErrUnsafe) {
			t.Errorf("tar %q: got %v", name, err)
		}
	}
	for _, ok := range []string{"a/b.sfc", "./a.sfc", "a//b.sfc", "Game (USA) [!].sfc", "ゲーム.sfc"} {
		if _, err := cleanName(ok); err != nil {
			t.Errorf("%q should be fine: %v", ok, err)
		}
	}
}

// Even a name the checks miss can't leave the folder: the writing goes through os.Root.
func TestWritingCantLeaveTheFolder(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "dst")
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	zp := filepath.Join(dir, "a.zip")
	writeZip(t, zp, zipFile{name: "game.sfc", data: []byte("rom")})
	if _, err := unpackFile(zp, dst, []Want{{Name: "game.sfc", Out: "../outside.sfc"}}, 1<<20, nil); err == nil {
		t.Fatal("wrote outside the folder")
	}
	if _, err := os.Stat(filepath.Join(dir, "outside.sfc")); err == nil {
		t.Fatal("wrote outside the folder")
	}
}

func TestTooManyEntries(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "a.zip")
	f, err := os.Create(zp)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for i := range MaxEntries + 1 {
		if _, err := w.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("%d.sfc", i), Method: zip.Store}); err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Close()
	_ = f.Close()
	if _, err := List(zp); !errors.Is(err, ErrTooMany) {
		t.Fatalf("got %v", err)
	}
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	for i := range MaxEntries + 1 {
		_ = tw.WriteHeader(&tar.Header{Name: fmt.Sprintf("%d.sfc", i), Typeflag: tar.TypeReg, Mode: 0o644})
	}
	_ = tw.Close()
	if _, err := List(write(t, filepath.Join(dir, "a.tar"), b.Bytes())); !errors.Is(err, ErrTooMany) {
		t.Fatalf("tar: got %v", err)
	}
}

func TestAbsurdSizes(t *testing.T) {
	dir := t.TempDir()
	// A tar header promising more than anything could hold, with nothing behind it.
	var hdr bytes.Buffer
	tw := tar.NewWriter(&hdr)
	_ = tw.WriteHeader(&tar.Header{Name: "big.sfc", Typeflag: tar.TypeReg, Size: MaxUnpacked + 1, Mode: 0o644})
	if _, err := List(write(t, filepath.Join(dir, "big.tar"), hdr.Bytes())); !errors.Is(err, ErrTooBig) {
		t.Errorf("declared size: got %v", err)
	}
	// Far more out than in, without being enormous by itself.
	l := &lister{total: ratioFloor + 1}
	small := write(t, filepath.Join(dir, "small"), []byte("x"))
	if _, err := l.done(small); !errors.Is(err, ErrTooBig) {
		t.Errorf("ratio: got %v", err)
	}
	// A disc full of blank space is fine.
	l = &lister{total: 700 << 20}
	if _, err := l.done(small); err != nil {
		t.Errorf("under the floor: got %v", err)
	}
}

// What comes out may be far more than the headers said.
func TestLyingArchivesStopAtTheLimit(t *testing.T) {
	dir := t.TempDir()
	zeros := make([]byte, 8<<20)
	gz := gzBytes(t, zeros, "")
	gp := write(t, filepath.Join(dir, "a.sfc.gz"), gz)
	dst := t.TempDir()
	if _, err := unpackIn(context.Background(), unpackRequest{Path: gp, Dst: dst, Want: []Want{{Name: "a.sfc", Out: "game.sfc"}}, Limit: 1 << 20}, nil); !errors.Is(err, ErrTooBig) {
		t.Errorf("gz bomb: got %v", err)
	}
	if st, err := os.Stat(filepath.Join(dst, "game.sfc")); err == nil && st.Size() > 1<<20+1<<20 {
		t.Errorf("wrote %d bytes past the limit", st.Size())
	}

	// A zip whose headers say 100 bytes for a file of 8 MB.
	var deflated bytes.Buffer
	zw, _ := zipDeflate(&deflated)
	_, _ = zw.Write(zeros)
	_ = zw.Close()
	zp := filepath.Join(dir, "lie.zip")
	f, err := os.Create(zp)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	e, err := w.CreateRaw(&zip.FileHeader{Name: "lie.sfc", Method: zip.Deflate, CompressedSize64: uint64(deflated.Len()), UncompressedSize64: 100, CRC32: crc32.ChecksumIEEE(zeros)})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = e.Write(deflated.Bytes())
	_ = w.Close()
	_ = f.Close()
	// Which of the two checks catches it first doesn't matter.
	if _, err := unpackIn(context.Background(), unpackRequest{Path: zp, Dst: t.TempDir(), Want: []Want{{Name: "lie.sfc", Out: "game.sfc"}}, Limit: 1 << 30}, nil); !errors.Is(err, ErrTooBig) && !errors.Is(err, ErrDamaged) {
		t.Errorf("lying zip: got %v", err)
	}
}

// sevenZipHeader builds an archive with a valid checksum around any header.
func sevenZipHeader(next []byte) []byte {
	var start [20]byte
	binary.LittleEndian.PutUint64(start[0:], 0)
	binary.LittleEndian.PutUint64(start[8:], uint64(len(next)))
	binary.LittleEndian.PutUint32(start[16:], crc32.ChecksumIEEE(next))
	var b bytes.Buffer
	b.Write([]byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c, 0, 4})
	_ = binary.Write(&b, binary.LittleEndian, crc32.ChecksumIEEE(start[:]))
	b.Write(start[:])
	b.Write(next)
	return b.Bytes()
}

// A header that claims billions of files makes the decoder ask for more memory than
// exists, which ends a Go program outright. That must only ever end a worker.
func TestAnArchiveThatKillsTheWorkerOnlyKillsTheWorker(t *testing.T) {
	dir := t.TempDir()
	huge := []byte{0x01, 0x05, 0xff}
	huge = binary.LittleEndian.AppendUint64(huge, 0xfffffff0)
	bad := write(t, filepath.Join(dir, "bad.7z"), sevenZipHeader(huge))
	l := NewLister()
	defer l.Close()
	_, err := l.List(bad, 0)
	if err == nil || !errors.Is(err, ErrDamaged) || !Transient(err) {
		t.Fatalf("got %v", err)
	}
	got, err := l.List("testdata/cart.7z", 0)
	if err != nil {
		t.Fatalf("the next archive should work: %v", err)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("got %v", got.Entries)
	}
}

func TestListerWorksThroughManyAndSurvivesRubbish(t *testing.T) {
	dir := t.TempDir()
	rubbish := write(t, filepath.Join(dir, "rubbish.7z"), []byte("not an archive at all"))
	empty := write(t, filepath.Join(dir, "empty.rar"), nil)
	l := NewLister()
	defer l.Close()
	for range 3 {
		if _, err := l.List(rubbish, 0); !errors.Is(err, ErrDamaged) || Transient(err) {
			t.Fatalf("rubbish: %v", err)
		}
		if _, err := l.List(empty, 0); !errors.Is(err, ErrDamaged) {
			t.Fatalf("empty: %v", err)
		}
		if _, err := l.List(filepath.Join(dir, "missing.7z"), 0); err == nil {
			t.Fatal("missing")
		}
		if _, err := l.List("testdata/cart.rar", 0); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScanBudgetStopsAtAGlance(t *testing.T) {
	dir := t.TempDir()
	first := bytes.Repeat([]byte{1, 2, 3, 4}, 1<<18)
	p := write(t, filepath.Join(dir, "a.tar.gz"), gzBytes(t, tarBytes(t, tarFile{name: "a.iso", data: first}, tarFile{name: "b.cue", data: []byte("x")}), ""))
	l := NewLister()
	defer l.Close()
	got, err := l.List(p, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Partial || join(entryNames(got)) != "a.iso" {
		t.Fatalf("got %+v", got)
	}
	got, err = l.List(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Partial || join(entryNames(got)) != "a.iso | b.cue" {
		t.Fatalf("got %+v", got)
	}
}

func TestBaseName(t *testing.T) {
	for in, want := range map[string]string{
		"Game (USA).7z": "Game (USA)", "Game (USA) [!].zip": "Game (USA) [!]", "Game (USA).tar.gz": "Game (USA)", "Game.TGZ": "Game",
		"Game.part1.rar": "Game", "Game.part01.rar": "Game", "Game.sfc.gz": "Game.sfc", "Dr. Who.7z": "Dr. Who",
		"Game (USA).7z.001": "Game (USA)", "Game.tar.xz": "Game", "Game.tbz2": "Game", "Game.sfc.zst": "Game.sfc", "Game.tar.ZST": "Game",
	} {
		if got := BaseName(in); got != want {
			t.Errorf("%s: %s", in, got)
		}
	}
	if !IsLaterVolume("Game.part2.rar") || !IsLaterVolume("Game.part10.RAR") || IsLaterVolume("Game.part1.rar") || IsLaterVolume("Game.part01.rar") || IsLaterVolume("Game.rar") {
		t.Error("later volumes")
	}
}

func entries(names ...string) *Listing {
	l := &Listing{}
	for _, n := range names {
		l.Entries = append(l.Entries, Entry{Name: n, Size: 10})
	}
	return l
}

func TestChoose(t *testing.T) {
	ps1, snes, n64 := wittyplay.ByID("ps1"), wittyplay.ByID("snes"), wittyplay.ByID("n64")
	for _, c := range []struct {
		name    string
		list    *Listing
		console *wittyplay.Console
		archive string
		entry   string
		want    int
		discs   int
	}{
		{"one cartridge", entries("Game.sfc"), snes, "Game.7z", "game.sfc", 1, 0},
		{"cartridge with notes", entries("readme.txt", "Game.SMC"), snes, "Game.7z", "game.smc", 1, 0},
		{"n64", entries("Game.z64"), n64, "Game.zip", "game.z64", 1, 0},
		{"cue over bin", entries("Game.bin", "Game.cue", "readme.txt", "art.jpg"), ps1, "Game.7z", "Game.cue", 2, 0},
		{"tracks and .sbi come too", entries("Game (Track 1).bin", "Game (Track 2).bin", "Game.cue", "Game.sbi"), ps1, "Game.7z", "Game.cue", 4, 0},
		{"nested", entries("inner/Game.bin", "inner/Game.cue"), ps1, "Game.7z", "inner/Game.cue", 2, 0},
		{"playlist first", entries("Game (Disc 1).cue", "Game (Disc 2).cue", "Game (Disc 1).bin", "Game (Disc 2).bin", "Game.m3u"), ps1, "Game.7z", "Game.m3u", 5, 0},
		{"discs get a playlist", entries("Game (Disc 2).cue", "Game (Disc 1).cue", "Game (Disc 1).bin", "Game (Disc 2).bin"), ps1, "Game.7z", "game.m3u", 4, 2},
		{"versions of one disc don't", entries("Game (USA).cue", "Game (Europe).cue", "Game (USA).bin", "Game (Europe).bin"), ps1, "Game.7z", "Game (USA).cue", 4, 0},
		{"the archive's own name wins", entries("Game (USA).cue", "Game (Europe).cue", "Game (USA).bin", "Game (Europe).bin"), ps1, "Game (Europe).7z", "Game (Europe).cue", 4, 0},
		{"bare discs", entries("Game (Disc 1).chd", "Game (Disc 2).chd"), ps1, "Game.7z", "game.m3u", 2, 2},
		{"one bare image", entries("Game.iso", "readme.txt"), ps1, "Game.7z", "game.iso", 1, 0},
		{"a bin with no cue", entries("Game.bin"), ps1, "Game.7z", "game.bin", 1, 0},
	} {
		p, err := Choose(c.list, c.console, c.archive)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if p.Entry != c.entry || len(p.Want) != c.want || len(p.Discs) != c.discs {
			t.Errorf("%s: entry %q, %d files, %d discs: %+v", c.name, p.Entry, len(p.Want), len(p.Discs), p)
		}
	}
	if _, err := Choose(entries("photo.jpg", "Game.gba"), snes, "x.7z"); !errors.Is(err, ErrNoGame) {
		t.Errorf("no game: %v", err)
	}
}

func TestChooseAmongMergedVersions(t *testing.T) {
	snes := wittyplay.ByID("snes")
	p, err := Choose(entries("Game (J).sfc", "Game (U) [b1].sfc", "Game (E).sfc", "Game (U) [!].sfc", "Game (U) [h1].sfc"), snes, "Merged.7z")
	if err != nil {
		t.Fatal(err)
	}
	if p.Want[0].Name != "Game (U) [!].sfc" {
		t.Fatalf("picked %s", p.Want[0].Name)
	}
	// All else equal, the first in the archive, as has always been the case for a .zip.
	p, _ = Choose(entries("b.sfc", "a.sfc"), snes, "x.zip")
	if p.Want[0].Name != "b.sfc" {
		t.Fatalf("picked %s", p.Want[0].Name)
	}
}

func TestDiscsSortNaturally(t *testing.T) {
	var discs []Entry
	for _, n := range []string{"Game (Disc 10).cue", "Game (Disc 2).cue", "Game (Disc 1).cue"} {
		discs = append(discs, Entry{Name: n})
	}
	naturalSort(discs)
	if discs[0].Name != "Game (Disc 1).cue" || discs[2].Name != "Game (Disc 10).cue" {
		t.Fatalf("%v", discs)
	}
}

func TestDescriptorsAreCheckedAndCorrected(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Game.bin"), []byte("x"))
	write(t, filepath.Join(dir, "sub", "Track 2.bin"), []byte("x"))
	files := []string{"Game.bin", "sub/Track 2.bin", "Game.cue"}
	cue := "FILE \"GAME.BIN\" BINARY\r\n  TRACK 01 MODE1/2352\r\nFILE \"C:\\dumps\\Track 2.bin\" BINARY\r\n  TRACK 02 AUDIO\r\n"
	write(t, filepath.Join(dir, "Game.cue"), []byte(cue))
	if err := checkReferences(dir, "Game.cue", files); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "Game.cue"))
	if !strings.Contains(string(got), `FILE "Game.bin" BINARY`) || !strings.Contains(string(got), `FILE "sub/Track 2.bin" BINARY`) || !strings.Contains(string(got), "\r\n") {
		t.Fatalf("got %q", got)
	}

	write(t, filepath.Join(dir, "Missing.cue"), []byte("FILE \"Gone.bin\" BINARY\n"))
	err := checkReferences(dir, "Missing.cue", append(files, "Missing.cue"))
	if err == nil || !strings.Contains(err.Error(), "Gone.bin") {
		t.Fatalf("missing track: %v", err)
	}

	// A list can't point at anything outside the game, with or without climbing.
	write(t, filepath.Join(dir, "Evil.cue"), []byte("FILE \"../../../../secret.bin\" BINARY\n"))
	if err := checkReferences(dir, "Evil.cue", append(files, "Evil.cue")); err == nil {
		t.Fatal("a track outside the folder got through")
	}
	write(t, filepath.Join(dir, "Evil.gdi"), []byte("1\n1 0 4 2352 \"../secret.bin\" 0\n"))
	if err := checkReferences(dir, "Evil.gdi", append(files, "Evil.gdi")); err == nil {
		t.Fatal("a .gdi track outside the folder got through")
	}
	write(t, filepath.Join(dir, "Good.gdi"), []byte("1\n1 0 4 2352 game.bin 0\n"))
	if err := checkReferences(dir, "Good.gdi", append(files, "Good.gdi")); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(filepath.Join(dir, "Good.gdi"))
	if !strings.Contains(string(got), "2352 Game.bin 0") {
		t.Fatalf("got %q", got)
	}

	write(t, filepath.Join(dir, "Set.m3u"), []byte("# discs\nGame.cue\n../outside.cue\n"))
	if err := checkReferences(dir, "Set.m3u", append(files, "Set.m3u")); err == nil {
		t.Fatal("a disc outside the folder got through")
	}

	// A list too long to be one.
	write(t, filepath.Join(dir, "Huge.cue"), bytes.Repeat([]byte("REM x\n"), maxDescriptor))
	if err := checkReferences(dir, "Huge.cue", append(files, "Huge.cue")); !errors.Is(err, ErrTooBig) {
		t.Fatalf("huge: %v", err)
	}
}

func newCache(t *testing.T, limit int64) *Cache {
	t.Helper()
	c := &Cache{Dir: filepath.Join(t.TempDir(), "unpacked"), Limit: func() int64 { return limit }}
	t.Cleanup(c.Close)
	return c
}

func copyFixture(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return write(t, filepath.Join(dir, name), b)
}

func TestUnpacksADiscWithEveryTrack(t *testing.T) {
	for _, name := range []string{"disc-solid.7z", "disc-notsolid.7z", "disc-solid.rar", "disc-nested.7z"} {
		t.Run(name, func(t *testing.T) {
			src := t.TempDir()
			p := copyFixture(t, src, name)
			c := newCache(t, 1<<30)
			rom, release, err := c.Acquire(context.Background(), p, wittyplay.ByID("ps1"))
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			if filepath.Base(rom) != "Game.cue" {
				t.Fatalf("got %s", rom)
			}
			dir := filepath.Dir(rom)
			for f, size := range map[string]int64{"Game (Track 1).bin": 30000, "Game (Track 2).bin": 20000} {
				st, err := os.Stat(filepath.Join(dir, f))
				if err != nil || st.Size() != size {
					t.Errorf("%s: %v %v", f, st, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "readme.txt")); err == nil {
				t.Error("unpacked what the game doesn't need")
			}
			// Nothing beside the owner's file.
			list, _ := os.ReadDir(src)
			if len(list) != 1 {
				t.Errorf("wrote beside the archive: %v", list)
			}
			if !strings.HasPrefix(rom, c.Dir) {
				t.Errorf("not under the cache: %s", rom)
			}
		})
	}
}

func TestSeveralDiscsBecomeAPlaylist(t *testing.T) {
	p := copyFixture(t, t.TempDir(), "discs.7z")
	c := newCache(t, 1<<30)
	rom, release, err := c.Acquire(context.Background(), p, wittyplay.ByID("ps1"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if filepath.Base(rom) != "game.m3u" {
		t.Fatalf("got %s", rom)
	}
	got, _ := os.ReadFile(rom)
	if string(got) != "Game (Disc 1).cue\nGame (Disc 2).cue\n" {
		t.Fatalf("got %q", got)
	}
	for _, f := range []string{"Game (Disc 1).bin", "Game (Disc 2).bin", "Game (Disc 1).cue", "Game (Disc 2).cue"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(rom), f)); err != nil {
			t.Error(err)
		}
	}
}

// A one-file game's name doesn't depend on the archive or the cache, as a core may
// name its saves after it.
func TestCartridgeNameIsStable(t *testing.T) {
	for _, name := range []string{"cart.7z", "cart.rar", "cart-solid.rar"} {
		p := copyFixture(t, t.TempDir(), name)
		c := newCache(t, 1<<30)
		rom, release, err := c.Acquire(context.Background(), p, wittyplay.ByID("snes"))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if filepath.Base(rom) != "game.sfc" {
			t.Errorf("%s: got %s", name, rom)
		}
		release()
	}
	// A split RAR comes together through its first part.
	src := t.TempDir()
	first := copyFixture(t, src, "split.part1.rar")
	copyFixture(t, src, "split.part2.rar")
	copyFixture(t, src, "split.part3.rar")
	c := newCache(t, 1<<30)
	rom, release, err := c.Acquire(context.Background(), first, wittyplay.ByID("snes"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if st, err := os.Stat(rom); err != nil || st.Size() != 300000 {
		t.Fatalf("%v %v", st, err)
	}
}

func TestEveryOtherFormatPlays(t *testing.T) {
	dir := t.TempDir()
	rom := []byte("the game")
	tarData := tarBytes(t, tarFile{name: "Game.sfc", data: rom})
	zp := filepath.Join(dir, "z.zip")
	writeZip(t, zp, zipFile{name: "Game.sfc", data: rom})
	for _, p := range []string{zp, write(t, filepath.Join(dir, "t.tar"), tarData), write(t, filepath.Join(dir, "t.tar.gz"), gzBytes(t, tarData, "")),
		write(t, filepath.Join(dir, "t.tgz"), gzBytes(t, tarData, "")), write(t, filepath.Join(dir, "g.sfc.gz"), gzBytes(t, rom, ""))} {
		c := newCache(t, 1<<30)
		got, release, err := c.Acquire(context.Background(), p, wittyplay.ByID("snes"))
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		b, _ := os.ReadFile(got)
		if !bytes.Equal(b, rom) || filepath.Base(got) != "game.sfc" {
			t.Errorf("%s: %s %q", p, got, b)
		}
		release()
	}
}

func TestRenamedArchivesStillOpen(t *testing.T) {
	src, err := os.ReadFile("testdata/cart.7z")
	if err != nil {
		t.Fatal(err)
	}
	p := write(t, filepath.Join(t.TempDir(), "really a 7z.zip"), src)
	c := newCache(t, 1<<30)
	rom, release, err := c.Acquire(context.Background(), p, wittyplay.ByID("snes"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if filepath.Base(rom) != "game.sfc" {
		t.Fatal(rom)
	}
}

func TestReusedAndNotUnpackedTwice(t *testing.T) {
	p := copyFixture(t, t.TempDir(), "disc-solid.7z")
	c := newCache(t, 1<<30)
	ps1 := wittyplay.ByID("ps1")
	const n = 8
	var wg sync.WaitGroup
	roms := make([]string, n)
	errs := make([]error, n)
	rel := make([]func(), n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			roms[i], rel[i], errs[i] = c.Acquire(context.Background(), p, ps1)
		}()
	}
	wg.Wait()
	for i := range n {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if roms[i] != roms[0] {
			t.Fatalf("%s and %s", roms[i], roms[0])
		}
		rel[i]()
	}
	if got := c.builds.Load(); got != 1 {
		t.Fatalf("unpacked %d times", got)
	}
	again, release, err := c.Acquire(context.Background(), p, ps1)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if again != roms[0] || c.builds.Load() != 1 {
		t.Fatalf("the second play unpacked again: %s, %d", again, c.builds.Load())
	}
	// A new cache on the same folder, as after a restart.
	c2 := &Cache{Dir: c.Dir, Limit: c.Limit}
	defer c2.Close()
	if st := c2.Prepare(p, ps1); st.State != stateReady {
		t.Fatalf("after a restart: %+v", st)
	}
	// A changed archive isn't the same game any more.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	if st := c2.Prepare(p, ps1); st.State == stateReady {
		t.Fatal("reused what a changed archive held")
	}
}

func TestPartlyUnpackedIsNeverUsed(t *testing.T) {
	p := copyFixture(t, t.TempDir(), "disc-solid.7z")
	ps1 := wittyplay.ByID("ps1")
	c := newCache(t, 1<<30)
	rom, release, err := c.Acquire(context.Background(), p, ps1)
	if err != nil {
		t.Fatal(err)
	}
	release()
	key, _ := keyOf(p, ps1)
	track := filepath.Join(filepath.Dir(rom), "Game (Track 1).bin")

	// A crash mid-unpack leaves only a temporary folder, which is ignored and cleared.
	leftover := filepath.Join(c.Dir, tmpPrefix+key+"-123")
	write(t, filepath.Join(leftover, gameDirName, "Game (Track 1).bin"), []byte("partial"))
	// A crash during an eviction leaves a folder with no manifest.
	half := filepath.Join(c.Dir, "0123456789abcdef")
	write(t, filepath.Join(half, gameDirName, "x.bin"), []byte("partial"))
	c2 := &Cache{Dir: c.Dir, Limit: c.Limit}
	defer c2.Close()
	if st := c2.Prepare(p, ps1); st.State != stateReady {
		t.Fatalf("got %+v", st)
	}
	for _, gone := range []string{leftover, half} {
		if _, err := os.Stat(gone); err == nil {
			t.Fatalf("still there: %s", gone)
		}
	}

	// A file that lost its end, or vanished, is unpacked again.
	if err := os.Truncate(track, 100); err != nil {
		t.Fatal(err)
	}
	rom2, release2, err := c2.Acquire(context.Background(), p, ps1)
	if err != nil {
		t.Fatal(err)
	}
	defer release2()
	if st, err := os.Stat(filepath.Join(filepath.Dir(rom2), "Game (Track 1).bin")); err != nil || st.Size() != 30000 {
		t.Fatalf("%v %v", st, err)
	}
	if c2.builds.Load() != 1 {
		t.Fatalf("builds: %d", c2.builds.Load())
	}
}

func TestOldestGoFirstAndPlayingStays(t *testing.T) {
	dir := t.TempDir()
	snes := wittyplay.ByID("snes")
	mk := func(name string) string {
		p := filepath.Join(dir, name+".zip")
		writeZip(t, p, zipFile{name: name + ".sfc", data: bytes.Repeat([]byte(name), 100)})
		return p
	}
	a, b, c3, d := mk("aaaa"), mk("bbbb"), mk("cccc"), mk("dddd")
	c := newCache(t, 1000) // room for two
	play := func(p string) (string, func()) {
		t.Helper()
		rom, release, err := c.Acquire(context.Background(), p, snes)
		if err != nil {
			t.Fatal(err)
		}
		return rom, release
	}
	gone := func(p string) bool {
		key, _ := keyOf(p, snes)
		_, err := os.Stat(filepath.Join(c.Dir, key))
		return err != nil
	}
	age := func(p string, by time.Duration) {
		key, _ := keyOf(p, snes)
		when := time.Now().Add(-by)
		_ = os.Chtimes(filepath.Join(c.Dir, key, manifestName), when, when)
	}
	_, ra := play(a)
	ra()
	age(a, 3*time.Hour)
	_, rb := play(b)
	rb()
	age(b, 2*time.Hour)
	_, rc := play(c3) // a goes
	rc()
	age(c3, time.Hour)
	if !gone(a) || gone(b) || gone(c3) {
		t.Fatalf("a %v, b %v, c %v", gone(a), gone(b), gone(c3))
	}
	// Playing b makes it the newest, and it can't be evicted while it's being played.
	romB, rb2 := play(b)
	defer rb2()
	age(b, 5*time.Hour)
	_, rd := play(d) // b is oldest, but in use: c goes
	defer rd()
	if gone(b) || !gone(c3) {
		t.Fatalf("b %v, c %v", gone(b), gone(c3))
	}
	if _, err := os.Stat(romB); err != nil {
		t.Fatal(err)
	}
}

func TestClearGivesTheSpaceBack(t *testing.T) {
	dir := t.TempDir()
	snes := wittyplay.ByID("snes")
	mk := func(name string) string {
		p := filepath.Join(dir, name+".zip")
		writeZip(t, p, zipFile{name: name + ".sfc", data: []byte(name)})
		return p
	}
	played, idle := mk("played"), mk("idle")
	c := newCache(t, 1<<30)
	rom, release, err := c.Acquire(context.Background(), played, snes)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_, r2, err := c.Acquire(context.Background(), idle, snes)
	if err != nil {
		t.Fatal(err)
	}
	r2()
	before := c.Used()
	c.Clear()
	if c.Used() >= before || c.Used() == 0 {
		t.Fatalf("%d, then %d", before, c.Used())
	}
	if _, err := os.Stat(rom); err != nil {
		t.Fatalf("emptied a game being played: %v", err)
	}
	if st := c.Prepare(played, snes); st.State != stateReady {
		t.Fatalf("got %+v", st)
	}
	// It comes back when played.
	builds := c.builds.Load()
	_, r3, err := c.Acquire(context.Background(), idle, snes)
	if err != nil {
		t.Fatal(err)
	}
	r3()
	if c.builds.Load() != builds+1 {
		t.Fatal("wasn't unpacked again")
	}
}

func TestTooLargeForTheCache(t *testing.T) {
	p := copyFixture(t, t.TempDir(), "disc-solid.7z")
	c := newCache(t, 1000)
	_, _, err := c.Acquire(context.Background(), p, wittyplay.ByID("ps1"))
	if err == nil || !strings.Contains(err.Error(), "Settings") {
		t.Fatalf("got %v", err)
	}
	if list, _ := os.ReadDir(c.Dir); len(list) != 0 {
		t.Fatalf("left %v", list)
	}
}

func TestClearAnswersForWhatCantBePlayed(t *testing.T) {
	dir := t.TempDir()
	snes := wittyplay.ByID("snes")
	zp := filepath.Join(dir, "other.zip")
	writeZip(t, zp, zipFile{name: "Game.gba", data: []byte("x")})
	c := newCache(t, 1<<30)
	if _, _, err := c.Acquire(context.Background(), zp, snes); err == nil || !strings.Contains(err.Error(), "no game for Super Nintendo") {
		t.Errorf("wrong console: %v", err)
	}
	if _, _, err := c.Acquire(context.Background(), "testdata/locked-names.7z", snes); !errors.Is(err, ErrEncrypted) {
		t.Errorf("encrypted: %v", err)
	}
	if _, _, err := c.Acquire(context.Background(), "testdata/locked-content.7z", snes); !errors.Is(err, ErrEncrypted) {
		t.Errorf("encrypted contents: %v", err)
	}
	if _, _, err := c.Acquire(context.Background(), filepath.Join(dir, "gone.7z"), snes); err == nil {
		t.Error("missing file")
	}
	if st := c.Prepare("testdata/locked-names.7z", snes); st.State != stateFailed || st.Error == "" {
		t.Errorf("a poller should read the same error: %+v", st)
	}
	// Failures leave nothing behind.
	if list, _ := os.ReadDir(c.Dir); len(list) != 0 {
		t.Errorf("left %v", list)
	}
}

func TestPrepareFollowsTheUnpack(t *testing.T) {
	p := copyFixture(t, t.TempDir(), "disc-solid.7z")
	c := newCache(t, 1<<30)
	ps1 := wittyplay.ByID("ps1")
	deadline := time.Now().Add(60 * time.Second)
	for {
		st := c.Prepare(p, ps1)
		if st.State == stateReady {
			if st.Total == 0 || st.Done != st.Total {
				t.Fatalf("got %+v", st)
			}
			break
		}
		if st.State == stateFailed {
			t.Fatal(st.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("still %+v", st)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if c.builds.Load() != 1 {
		t.Fatalf("builds: %d", c.builds.Load())
	}
}

func TestGivingUpLeavesTheUnpackRunning(t *testing.T) {
	p := copyFixture(t, t.TempDir(), "disc-solid.7z")
	c := newCache(t, 1<<30)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ps1 := wittyplay.ByID("ps1")
	if _, _, err := c.Acquire(ctx, p, ps1); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	rom, release, err := c.Acquire(context.Background(), p, ps1)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if filepath.Base(rom) != "Game.cue" || c.builds.Load() != 1 {
		t.Fatalf("%s, %d", rom, c.builds.Load())
	}
}

func TestClosingStopsAnUnpack(t *testing.T) {
	dir := t.TempDir()
	// Random data, so this takes long enough to be stopped halfway.
	big := make([]byte, 64<<20)
	for i := range big {
		big[i] = byte(i * 2654435761 >> 13)
	}
	p := write(t, filepath.Join(dir, "big.tar"), tarBytes(t, tarFile{name: "big.sfc", data: big}))
	c := &Cache{Dir: filepath.Join(dir, "unpacked"), Limit: func() int64 { return 1 << 30 }}
	snes := wittyplay.ByID("snes")
	c.Prepare(p, snes)
	c.Close()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		n := len(c.jobs)
		c.mu.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st := c.Prepare(p, snes); st.State == stateUnpacking {
		t.Fatalf("got %+v", st)
	}
	// Nothing half-made is left to be taken for the game.
	key, _ := keyOf(p, snes)
	if _, err := os.Stat(filepath.Join(c.Dir, key, manifestName)); err == nil {
		// It may have finished before Close got there, which is fine, if it's whole.
		c2 := &Cache{Dir: c.Dir, Limit: c.Limit}
		defer c2.Close()
		if st := c2.Prepare(p, snes); st.State != stateReady {
			t.Fatalf("got %+v", st)
		}
	}
}

func TestAbandonedUnpacksStop(t *testing.T) {
	saved := abandonAfter
	abandonAfter = 80 * time.Millisecond
	defer func() { abandonAfter = saved }()
	dir := t.TempDir()
	p := write(t, filepath.Join(dir, "big.tar"), tarBytes(t, tarFile{name: "big.sfc", data: make([]byte, 1<<20)}))
	c := newCache(t, 1<<30)
	snes := wittyplay.ByID("snes")
	// Holds the job in its queue, where it can be seen to give up.
	c.init()
	for range maxJobs {
		c.slots <- struct{}{}
	}
	defer func() {
		for range maxJobs {
			<-c.slots
		}
	}()
	if st := c.Prepare(p, snes); st.State != stateWaiting {
		t.Fatalf("got %+v", st)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		n := len(c.jobs)
		c.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("still waiting, with nobody asking")
}

// The worker's own errors don't leak its words, and it can't be talked into anything
// by its input.
func TestWorkerAnswersOnlyWithCodes(t *testing.T) {
	if got := CodeOf(plainError(io.ErrUnexpectedEOF)); got != ErrDamaged.Code {
		t.Fatalf("got %q", got)
	}
	bad := &Listing{Entries: []Entry{{Name: "../evil"}}}
	if err := bad.check(); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("got %v", err)
	}
}

func zipDeflate(w io.Writer) (io.WriteCloser, error) { return flate.NewWriter(w, flate.BestSpeed) }
