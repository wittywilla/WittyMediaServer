// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package archive

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
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
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

// A small file must cost little to read, however it was compressed: this runs on servers
// with very little memory.
func TestSmallFilesTakeLittleMemory(t *testing.T) {
	rom := bytes.Repeat([]byte("the game"), 1024)
	for how, data := range map[string][]byte{formatGz: gzBytes(t, rom, ""), formatXz: xzBytes(t, rom), formatZst: zstBytes(t, rom)} {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		src, done, err := decompress(how, bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", how, err)
		}
		got, err := io.ReadAll(src)
		done()
		if err != nil || !bytes.Equal(got, rom) {
			t.Fatalf("%s: %v, %d bytes", how, err, len(got))
		}
		runtime.ReadMemStats(&after)
		if used := after.TotalAlloc - before.TotalAlloc; used > 128<<20 {
			t.Errorf("%s: %d MB to read %d KB", how, used>>20, len(rom)>>10)
		}
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
	// On 64 bits this ends the worker. On 32 the decoder can tell it's impossible, and
	// says so. Either way it's a damaged archive, and only that.
	if _, err := l.List(bad, 0); !errors.Is(err, ErrDamaged) {
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

func copyFixture(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return write(t, filepath.Join(dir, name), b)
}

// A one-file game's name doesn't depend on the archive or the cache, as a core may
// name its saves after it.
// A compressed file doesn't say how large it is, so the room left on the disk has to
// hold while it unpacks.
// The worker's own errors don't leak its words, and it can't be talked into anything
// by its input.
func TestWorkerAnswersOnlyWithCodes(t *testing.T) {
	if got := CodeOf(plainError(io.ErrUnexpectedEOF)); got != ErrDamaged.Code {
		t.Fatalf("got %q", got)
	}
	// A worker that stopped says nothing lasting about the file; a damaged file does.
	if !Transient(errWorkerGone) || !errors.Is(errWorkerGone, ErrDamaged) || Transient(ErrDamaged) || CodeOf(errWorkerGone) != ErrDamaged.Code {
		t.Fatal("a worker that stopped should count as damage for now, and be asked about again")
	}
	bad := &Listing{Entries: []Entry{{Name: "../evil"}}}
	if err := bad.check(); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("got %v", err)
	}
}

func zipDeflate(w io.Writer) (io.WriteCloser, error) { return flate.NewWriter(w, flate.BestSpeed) }

// A panic while unpacking comes back as an error, and whoever is waiting hears about it. Nothing
// else would catch one on this goroutine: it would end the server.
// A set names its archive after its main game, and that's the one to play, unless all that's
// on offer in its name is a bad dump or someone's hack of it.

// Every kind of compression unpacks through Unpack, in a worker of its own.
func TestEveryCompressionUnpacks(t *testing.T) {
	dir := t.TempDir()
	page := []byte("a page")
	tarData := tarBytes(t, tarFile{name: "Page.jpg", data: page})
	for name, data := range map[string][]byte{
		"a.jpg.xz": xzBytes(t, page), "a.tar.xz": xzBytes(t, tarData), "a.txz": xzBytes(t, tarData),
		"b.jpg.zst": zstBytes(t, page), "b.tar.zst": zstBytes(t, tarData), "b.tzst": zstBytes(t, tarData),
		"c.tar": tarData, "c.tar.gz": gzBytes(t, tarData, ""),
	} {
		p := write(t, filepath.Join(dir, name), data)
		l, err := List(p)
		if err != nil || len(l.Entries) != 1 {
			t.Errorf("%s: %v %v", name, l, err)
			continue
		}
		out := t.TempDir()
		if _, err := Unpack(context.Background(), p, out, []Want{{Name: l.Entries[0].Name, Out: "00001.jpg"}}, 1<<20); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if b, _ := os.ReadFile(filepath.Join(out, "00001.jpg")); !bytes.Equal(b, page) {
			t.Errorf("%s: %q", name, b)
		}
	}
	// A bomb stops at the limit however it was packed.
	bomb := write(t, filepath.Join(dir, "bomb.jpg.zst"), zstBytes(t, make([]byte, 8<<20)))
	if _, err := Unpack(context.Background(), bomb, t.TempDir(), []Want{{Name: "bomb.jpg", Out: "00001.jpg"}}, 1<<20); !errors.Is(err, ErrTooBig) {
		t.Errorf("bomb: %v", err)
	}
}

// A stopped unpack stops its worker and says so.
func TestCancelledUnpackStops(t *testing.T) {
	dir := t.TempDir()
	big := make([]byte, 64<<20)
	for i := range big {
		big[i] = byte(uint64(i) * 2654435761 >> 13)
	}
	p := write(t, filepath.Join(dir, "big.tar"), tarBytes(t, tarFile{name: "big.jpg", data: big}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Unpack(ctx, p, t.TempDir(), []Want{{Name: "big.jpg", Out: "00001.jpg"}}, 1<<30); err == nil {
		t.Fatal("a stopped unpack went on")
	}
}
