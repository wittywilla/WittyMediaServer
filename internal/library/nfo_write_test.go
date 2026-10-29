package library

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
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
	s := &Scanner{CacheDir: dir, Log: slog.Default()}
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
