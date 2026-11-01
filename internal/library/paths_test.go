package library

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

func testScanner(t *testing.T, roots ...string) *Scanner {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	st := &Store{DB: d}
	if _, err := st.CreateLibrary(context.Background(), "lib", "movies", roots); err != nil {
		t.Fatal(err)
	}
	s := NewScanner(st, &ffmpeg.Tools{Encoders: map[string]bool{}}, t.TempDir(), func() string { return "" }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.ImagesDir = t.TempDir()
	return s
}

var pngHead = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 40)...)

// A sidecar in a media folder names the artwork. It must not be able to make
// the server copy its database, or any other file, next to the media.
func TestLoadImageOnlyReadsPicturesFromLibraries(t *testing.T) {
	lib, outside := t.TempDir(), t.TempDir()
	s := testScanner(t, lib)
	secret := filepath.Join(outside, "wms.jpg")
	if err := os.WriteFile(secret, pngHead, 0o644); err != nil {
		t.Fatal(err)
	}
	notPic := filepath.Join(lib, "notes.jpg")
	if err := os.WriteFile(notPic, []byte("password=hunter2"), 0o644); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(lib, "poster.jpg")
	if err := os.WriteFile(inside, pngHead, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := s.LoadImage(ctx, inside); err != nil {
		t.Errorf("a picture in the library: %v", err)
	}
	for _, ref := range []string{secret, notPic, "cache:../../wms.db", `cache:..\..\wms.db`, "cache:", "relative.jpg"} {
		if _, err := s.LoadImage(ctx, ref); err == nil {
			t.Errorf("%s was read", ref)
		}
	}
}

// http.DefaultClient would fetch anything on this network and save the answer
// beside the media, where whoever wrote the sidecar can read it.
func TestLoadImageRefusesInternalAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(pngHead) }))
	defer srv.Close()
	s := testScanner(t, t.TempDir())
	if _, err := s.LoadImage(context.Background(), srv.URL+"/x.png"); err == nil {
		t.Fatal("fetched a picture from this machine through a sidecar's address")
	}
}

func TestArtRef(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"":                      "",
		"https://x.test/a.jpg":  "https://x.test/a.jpg",
		"poster.jpg":            filepath.Join(dir, "poster.jpg"),
		"art/poster.jpg":        filepath.Join(dir, "art", "poster.jpg"),
		"../../etc/passwd":      "",
		"/etc/passwd":           "",
		`\\evil\share\x.jpg`:    "",
		`C:\Windows\win.ini`:    "",
		"cache:../../wms.db":    "",
		"file:///etc/passwd":    "",
		"subfile,,start,0,,:/x": "",
		"concat:/etc/passwd|/x": "",
	}
	for in, want := range cases {
		if got := artRef(in, dir); got != want {
			t.Errorf("artRef(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInternetAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8": true, "1.1.1.1": true, "2606:4700:4700::1111": true,
		"127.0.0.1": false, "10.0.0.1": false, "172.16.0.1": false, "192.168.1.1": false, "169.254.169.254": false, "100.64.0.1": false,
		"0.0.0.0": false, "0.1.2.3": false, "192.0.0.1": false, "198.18.0.1": false, "224.0.0.1": false, "240.0.0.1": false, "255.255.255.255": false,
		"::1": false, "::": false, "::ffff:127.0.0.1": false, "::ffff:10.0.0.1": false, "fe80::1": false, "fc00::1": false, "fd12::1": false, "ff02::1": false,
		"64:ff9b::7f00:1": false, "64:ff9b::a00:1": false, "2002:7f00:1::": false, "2002:a00:1::": false, "2001::1": false, "2001:db8::1": false, "fec0::1": false,
	} {
		ip := net.ParseIP(addr)
		if got := internetAddr(ip); got != want {
			t.Errorf("%s: got %v, want %v", addr, got, want)
		}
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("creating links needs developer mode on Windows")
		}
		t.Fatal(err)
	}
}

// A link in a shared folder must not turn into a way to read any file here.
func TestScanSkipsLinksLeavingTheLibrary(t *testing.T) {
	lib, outside := t.TempDir(), t.TempDir()
	secret := filepath.Join(outside, "wms.db")
	if err := os.WriteFile(secret, []byte("secrets"), 0o644); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(lib, "real.pdf")
	if err := os.WriteFile(real, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlink(t, secret, filepath.Join(lib, "steal.pdf"))
	symlink(t, real, filepath.Join(lib, "alias.pdf"))
	got, err := discoverBooks(lib)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range got {
		names = append(names, filepath.Base(c.path))
	}
	if strings.Contains(strings.Join(names, " "), "steal") {
		t.Errorf("indexed a link out of the library: %v", names)
	}
	if !strings.Contains(strings.Join(names, " "), "alias") {
		t.Errorf("a link inside the library should work: %v", names)
	}
	s := testScanner(t, lib)
	if s.Store.LibraryFile(context.Background(), filepath.Join(lib, "steal.pdf")) {
		t.Error("a link swapped in after a scan is still refused when serving")
	}
}

// writeNFO keeps a copy of an NFO it did not write. If that NFO is a link,
// the copy would hand over the file it points at, and writing through the
// link would overwrite it.
func TestWriteNFODoesNotFollowLinks(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	victim := filepath.Join(outside, "victim.txt")
	if err := os.WriteFile(victim, []byte("do not touch"), 0o644); err != nil {
		t.Fatal(err)
	}
	nfo := filepath.Join(dir, "movie.nfo")
	symlink(t, victim, nfo)
	if err := writeNFO(nfo, nfoMovieOut{Title: "x"}, []string{dir}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "do not touch" {
		t.Errorf("wrote through the link: %q", b)
	}
	if _, err := os.Stat(nfo + ".bak"); err == nil {
		t.Error("copied the link's target into the library")
	}
}

func TestSidecarArtIsContained(t *testing.T) {
	dir := t.TempDir()
	side := filepath.Join(dir, "metadata.json")
	if err := os.WriteFile(side, []byte(`{"poster":"/etc/passwd","backdrop":"https://x.test/b.jpg"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	it := &Item{}
	if !applySidecarJSON(it, side) {
		t.Fatal("sidecar not read")
	}
	if it.Poster != "" || it.Backdrop != "https://x.test/b.jpg" {
		t.Errorf("poster %q, backdrop %q", it.Poster, it.Backdrop)
	}
}

func TestSafeName(t *testing.T) {
	for in, bad := range map[string][]string{`../../etc/x`: {"/"}, `a\b`: {`\`}, "CON": nil, "nul": nil, "x:y": {":"}, "..": {".."}} {
		got := safeName(in)
		for _, b := range bad {
			if strings.Contains(got, b) {
				t.Errorf("safeName(%q) = %q", in, got)
			}
		}
		if !filepath.IsLocal(got) {
			t.Errorf("safeName(%q) = %q is not a plain name", in, got)
		}
		switch strings.ToUpper(got) {
		case "CON", "NUL", "AUX", "PRN":
			t.Errorf("safeName(%q) = %q is a reserved device name", in, got)
		}
	}
}

// junction makes a link to a folder that any account can make on Windows, and a symbolic link
// elsewhere.
func junction(t *testing.T, target, link string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if real, err := filepath.EvalSymlinks(target); err == nil {
			target = real // writing through a junction to a short name fails by itself
		}
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		return
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// Every kind of library skips them, games and galleries too, which used to index anything.
func TestEveryLibraryKindSkipsLinksOut(t *testing.T) {
	outside := t.TempDir()
	for _, n := range []string{"secret.jpg", "secret.mp4", "secret.sfc"} {
		if err := os.WriteFile(filepath.Join(outside, n), []byte("secrets"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, find := range map[string]func(string) ([]candidate, error){
		"photos":   discoverPhotos,
		"creators": discoverCreatorVideos,
		"games":    func(root string) ([]candidate, error) { return discoverGames(root, "snes") },
	} {
		lib := t.TempDir()
		for _, n := range []string{"secret.jpg", "secret.mp4", "secret.sfc"} {
			symlink(t, filepath.Join(outside, n), filepath.Join(lib, "steal-"+n))
		}
		got, err := find(lib)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range got {
			if strings.Contains(c.path, "steal") {
				t.Errorf("%s indexed a link out of the library: %s", name, c.path)
			}
		}
	}
}

func TestOpenInRootsStaysInside(t *testing.T) {
	lib, other, outside := t.TempDir(), t.TempDir(), t.TempDir()
	roots := []string{lib, other}
	write := func(p, data string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(lib, "sub", "film.mp4"), "film")
	write(filepath.Join(other, "there", "film.mp4"), "other library")
	write(filepath.Join(outside, "film.mp4"), "SECRET")
	read := func(p string) (string, error) {
		f, st, err := openInRoots(p, roots)
		if err != nil {
			return "", err
		}
		defer f.Close()
		if st.Size() == 0 {
			return "", nil
		}
		b, err := io.ReadAll(f)
		return string(b), err
	}
	if got, err := read(filepath.Join(lib, "sub", "film.mp4")); err != nil || got != "film" {
		t.Errorf("a plain file: %q %v", got, err)
	}
	if _, err := read(filepath.Join(outside, "film.mp4")); !errors.Is(err, ErrNotInLibrary) {
		t.Errorf("outside: %v", err)
	}
	if _, err := read(lib); err == nil {
		t.Error("the folder itself opened")
	}
	if _, err := read(filepath.Join(lib, "missing.mp4")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing: %v", err)
	}

	// A folder link leading out is refused, wherever it is in the path.
	junction(t, outside, filepath.Join(lib, "away"))
	if got, err := read(filepath.Join(lib, "away", "film.mp4")); err == nil {
		t.Errorf("through a link out: %q", got)
	}
	if resolvesInside(filepath.Join(lib, "away", "film.mp4"), roots) {
		t.Error("the check says a link out is inside")
	}
	// One leading inside, or to another library, still works: an absolute link can't be followed
	// by os.Root by itself, and real setups have them.
	junction(t, filepath.Join(lib, "sub"), filepath.Join(lib, "alias"))
	if got, err := read(filepath.Join(lib, "alias", "film.mp4")); err != nil || got != "film" {
		t.Errorf("a link inside the library: %q %v", got, err)
	}
	junction(t, filepath.Join(other, "there"), filepath.Join(lib, "next"))
	if got, err := read(filepath.Join(lib, "next", "film.mp4")); err != nil || got != "other library" {
		t.Errorf("a link to another library: %q %v", got, err)
	}
}

func TestFileLinksInOpenInRoots(t *testing.T) {
	lib, outside := t.TempDir(), t.TempDir()
	secret := filepath.Join(outside, "wms.db")
	if err := os.WriteFile(secret, []byte("SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(lib, "real.mp4")
	if err := os.WriteFile(real, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlink(t, secret, filepath.Join(lib, "steal.mp4"))
	symlink(t, real, filepath.Join(lib, "absolute.mp4"))
	symlink(t, "real.mp4", filepath.Join(lib, "relative.mp4"))
	roots := []string{lib}
	if f, _, err := openInRoots(filepath.Join(lib, "steal.mp4"), roots); err == nil {
		f.Close()
		t.Error("a link out opened")
	}
	for _, n := range []string{"absolute.mp4", "relative.mp4"} {
		f, _, err := openInRoots(filepath.Join(lib, n), roots)
		if err != nil {
			t.Errorf("%s: %v", n, err)
			continue
		}
		f.Close()
	}
}

// A folder swapped for a link between finding a film and saving its artwork: nothing lands where
// the link leads.
func TestWritesDontFollowAFolderSwappedForALink(t *testing.T) {
	lib, outside := t.TempDir(), t.TempDir()
	s := testScanner(t, lib)
	pic := filepath.Join(lib, "cover.png")
	if err := os.WriteFile(pic, pngHead, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	junction(t, outside, filepath.Join(lib, "film"))
	dst := filepath.Join(lib, "film", "poster.png")
	if err := s.writeArt(ctx, pic, dst); err == nil {
		t.Error("the write said it went fine")
	}
	if names, _ := os.ReadDir(outside); len(names) != 0 {
		t.Errorf("wrote through the link: %v", names)
	}
	// An ordinary folder is written to, with no temporary file left behind.
	if err := os.Mkdir(filepath.Join(lib, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.writeArt(ctx, pic, filepath.Join(lib, "real", "poster.png")); err != nil {
		t.Fatal(err)
	}
	names, _ := os.ReadDir(filepath.Join(lib, "real"))
	if len(names) != 1 || names[0].Name() != "poster.png" {
		t.Errorf("%v", names)
	}
}

func TestCreateInLibrary(t *testing.T) {
	lib, outside := t.TempDir(), t.TempDir()
	dest := filepath.Join(lib, "Uploads", "someone", "2026-11")
	f, p, err := CreateInLibrary(lib, dest, "a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if p != filepath.Join(dest, "a.jpg") {
		t.Errorf("%s", p)
	}
	f, p, err = CreateInLibrary(lib, dest, "a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if p != filepath.Join(dest, "a (1).jpg") {
		t.Errorf("a file was replaced: %s", p)
	}
	junction(t, outside, filepath.Join(lib, "away"))
	if f, p, err := CreateInLibrary(lib, filepath.Join(lib, "away", "x"), "a.jpg"); err == nil {
		f.Close()
		t.Errorf("made %s through a link", p)
	}
	if names, _ := os.ReadDir(outside); len(names) != 0 {
		t.Errorf("wrote through the link: %v", names)
	}
}
