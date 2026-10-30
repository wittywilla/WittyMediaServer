package library

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

func TestSaveItemMetadataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	movieDir := filepath.Join(dir, "Shrek (2001)")
	if err := os.MkdirAll(movieDir, 0o755); err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(movieDir, "Shrek (2001).mkv")
	art := filepath.Join(dir, "picked.jpg")
	for _, f := range []string{media, art} {
		if err := os.WriteFile(f, pngHead, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A hand-written NFO must be kept as a backup, not lost.
	userNFO := filepath.Join(movieDir, "Shrek (2001).nfo")
	if err := os.WriteFile(userNFO, []byte("<movie><title>Mine</title></movie>"), 0o644); err != nil {
		t.Fatal(err)
	}
	y, r := 2001, 7.9
	it := &Item{Kind: "video", Path: media, Title: "Shrek", Year: &y, Overview: "An ogre.", Genres: []string{"Animation", "Comedy"},
		Rating: &r, Provider: "tmdb", ProviderID: "808", Poster: art}
	s := testScanner(t, dir)
	if err := s.SaveItemMetadata(context.Background(), it); err != nil {
		t.Fatal(err)
	}
	// Own folder: the universal names.
	if want := filepath.Join(movieDir, "poster.jpg"); it.Poster != want {
		t.Fatalf("poster stored as %q, want %q", it.Poster, want)
	}
	if _, err := os.Stat(userNFO + ".bak"); err != nil {
		t.Fatalf("user NFO not backed up: %v", err)
	}
	var back Item
	if !applyNFO(&back, userNFO) {
		t.Fatal("written NFO does not parse")
	}
	if back.Title != "Shrek" || back.Year == nil || *back.Year != 2001 || back.Overview != "An ogre." ||
		len(back.Genres) != 2 || back.ProviderID != "808" || back.Rating == nil {
		t.Fatalf("round trip lost data: %+v", back)
	}
	// Movies sharing a folder use <name>-poster.jpg.
	other := filepath.Join(movieDir, "Shrek 2 (2004).mkv")
	_ = os.WriteFile(other, []byte("x"), 0o644)
	p, b, n := ArtPaths(&Item{Kind: "video", Path: media})
	if !strings.HasSuffix(p, "Shrek (2001)-poster.jpg") || !strings.HasSuffix(b, "Shrek (2001)-fanart.jpg") || !strings.HasSuffix(n, "Shrek (2001).nfo") {
		t.Fatalf("shared-folder paths: %s %s %s", p, b, n)
	}
}

var testJPEG = append([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 16, 'J', 'F', 'I', 'F', 0}, make([]byte, 40)...)

// Sidecar files and feeds name artwork; saving metadata must not turn that
// into a copy of any file on the machine, or a fetch from the local network.
func TestLoadImageOnlyPictures(t *testing.T) {
	dir := t.TempDir()
	images := filepath.Join(dir, "images")
	if err := os.MkdirAll(images, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "wms.db")
	if err := os.WriteFile(secret, []byte("SQLite format 3 and everyone's data"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	st := &Store{DB: d}
	if _, err := st.CreateLibrary(ctx, "lib", "movies", []string{dir}); err != nil {
		t.Fatal(err)
	}
	s := &Scanner{ImagesDir: images, Log: slog.Default(), Store: st}
	if _, err := s.LoadImage(ctx, secret); err == nil {
		t.Error("a file that is no picture was read as artwork")
	}
	if _, err := s.LoadImage(ctx, filepath.Join(t.TempDir(), "elsewhere.jpg")); err == nil {
		t.Error("artwork outside the libraries was read")
	}
	if _, err := s.LoadImage(ctx, "cache:../wms.db"); err == nil {
		t.Error("a cache name left the cache")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(testJPEG) }))
	defer srv.Close()
	if _, err := s.LoadImage(ctx, srv.URL+"/x.jpg"); err == nil {
		t.Error("artwork was fetched from this machine")
	}
	pic := filepath.Join(dir, "p.jpg")
	if err := os.WriteFile(pic, testJPEG, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadImage(ctx, pic); err != nil {
		t.Errorf("a real picture was refused: %v", err)
	}
}

func TestSaveShowNFOReadsBack(t *testing.T) {
	dir := t.TempDir()
	y := 2002
	sh := &Show{Path: dir, Title: "Codename: Kids Next Door", Year: &y, Overview: "Kids vs adults.", Genres: []string{"Animation"},
		Network: "Cartoon Network", Status: "Ended", Provider: "tvdb", ProviderID: "76163"}
	if err := writeNFO(filepath.Join(dir, "tvshow.nfo"), nfoShowOut{Title: sh.Title, Year: *sh.Year, Plot: sh.Overview, Genres: sh.Genres,
		Studio: sh.Network, Status: sh.Status, UniqueIDs: []nfoUniqueID{{Type: "tvdb", Default: true, ID: sh.ProviderID}}, TVDBIDCompat: sh.ProviderID}); err != nil {
		t.Fatal(err)
	}
	var back Show
	if !applyShowNFO(&back, filepath.Join(dir, "tvshow.nfo")) {
		t.Fatal("tvshow.nfo does not parse")
	}
	if back.Title != sh.Title || back.ProviderID != "76163" || back.Network != "Cartoon Network" || back.Year == nil || *back.Year != 2002 {
		t.Fatalf("round trip lost data: %+v", back)
	}
}
