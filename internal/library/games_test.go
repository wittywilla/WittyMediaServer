package library

import (
	"archive/zip"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

func writeFiles(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func names(c []candidate, root string) []string {
	var out []string
	for _, x := range c {
		rel, _ := filepath.Rel(root, x.path)
		out = append(out, filepath.ToSlash(rel)+"="+x.console)
	}
	sort.Strings(out)
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDiscoverGamesOneConsole(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "Hello.sfc", "sub/World.smc", "notes.txt", "other.gba", "bios/skip.sfc", ".hidden/skip.sfc")
	got, err := discoverGames(root, "snes")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Hello.sfc=snes", "sub/World.smc=snes"}; !equal(names(got, root), want) {
		t.Fatalf("got %v, want %v", names(got, root), want)
	}
}

func TestDiscoverGamesAuto(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "a.sfc", "b.gba", "Sega Genesis/c.bin", "mystery.bin", "Atari 2600/d.bin", "readme.txt")
	got, _ := discoverGames(root, "auto")
	want := []string{"Atari 2600/d.bin=a2600", "Sega Genesis/c.bin=md", "a.sfc=snes", "b.gba=gba"}
	if !equal(names(got, root), want) {
		t.Fatalf("got %v, want %v", names(got, root), want)
	}
}

func TestDiscoverGamesMultiFile(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "Game/Game (Track 1).bin", "Game/Game (Track 2).bin", "Solo.bin", "Set/Disc 1.cue", "Set/Disc 2.cue", "Set/Disc 1.bin", "Set/Disc 2.bin")
	_ = os.WriteFile(filepath.Join(root, "Game", "Game.cue"), []byte("FILE \"Game (Track 1).bin\" BINARY\n  TRACK 01 MODE2/2352\nFILE \"Game (Track 2).bin\" BINARY\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "Set", "Disc 1.cue"), []byte("FILE \"Disc 1.bin\" BINARY\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "Set", "Disc 2.cue"), []byte("FILE \"Disc 2.bin\" BINARY\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "Set", "Game.m3u"), []byte("# discs\nDisc 1.cue\nDisc 2.cue\n../../outside.bin\n"), 0o644)
	got, _ := discoverGames(root, "ps1")
	want := []string{"Game/Game.cue=ps1", "Set/Game.m3u=ps1", "Solo.bin=ps1"}
	if !equal(names(got, root), want) {
		t.Fatalf("got %v, want %v", names(got, root), want)
	}
}

func TestDiscoverGamesZip(t *testing.T) {
	root := t.TempDir()
	mk := func(name, inner string) {
		f, err := os.Create(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		w := zip.NewWriter(f)
		e, _ := w.Create(inner)
		_, _ = e.Write([]byte("x"))
		_ = w.Close()
		_ = f.Close()
	}
	mk("good.zip", "game.nes")
	mk("other.zip", "photo.jpg")
	got, _ := discoverGames(root, "nes")
	if want := []string{"good.zip=nes"}; !equal(names(got, root), want) {
		t.Fatalf("got %v, want %v", names(got, root), want)
	}
	got, _ = discoverGames(root, "auto")
	if want := []string{"good.zip=nes"}; !equal(names(got, root), want) {
		t.Fatalf("auto: got %v, want %v", names(got, root), want)
	}
}

func TestParseGameName(t *testing.T) {
	for _, c := range []struct{ in, title, note string }{
		{"Mega Game (USA) (Rev 1) [!]", "Mega Game", "USA, Rev 1"},
		{"Mega Game, The (Europe)", "The Mega Game", "Europe"},
		{"Legend of Stuff, The - Part Two (Japan)", "The Legend of Stuff - Part Two", "Japan"},
		{"plain_name", "plain name", ""},
		{"(USA)", "(USA)", "USA"},
	} {
		title, note := ParseGameName(c.in)
		if title != c.title || note != c.note {
			t.Errorf("ParseGameName(%q) = %q, %q; want %q, %q", c.in, title, note, c.title, c.note)
		}
	}
}

// A whole scan: two libraries of one console stay apart, each item has its console,
// and a file that goes away is marked missing.
func TestScanGamesLibrary(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := &Store{DB: database}
	a, b := t.TempDir(), t.TempDir()
	writeFiles(t, a, "One (USA).sfc", "One (USA).png")
	writeFiles(t, b, "Two.smc")
	sc := NewScanner(store, nil, t.TempDir(), func() string { return "" }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mine, err := store.CreateLibrary(ctx, "My SNES", "games", []string{a})
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := store.CreateLibrary(ctx, "Kids' SNES", "games", []string{b})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{mine.ID, theirs.ID} {
		if err := store.SetLibraryConsole(ctx, id, "snes"); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := store.Library(ctx, mine.ID); got.Console != "snes" {
		t.Fatalf("console = %q", got.Console)
	}
	if err := sc.scan(ctx, mine.ID); err != nil {
		t.Fatal(err)
	}
	if err := sc.scan(ctx, theirs.ID); err != nil {
		t.Fatal(err)
	}
	items, _, err := store.Items(ctx, ItemQuery{LibraryID: mine.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}
	it := items[0]
	if it.Kind != "game" || it.Console != "snes" || it.Title != "One" || it.Tagline != "USA" || it.Container != "sfc" || it.Poster == "" {
		t.Fatalf("unexpected item: %+v", it)
	}

	if err := os.Remove(filepath.Join(a, "One (USA).sfc")); err != nil {
		t.Fatal(err)
	}
	if err := sc.scan(ctx, mine.ID); err != nil {
		t.Fatal(err)
	}
	if items, _, _ = store.Items(ctx, ItemQuery{LibraryID: mine.ID}); len(items) != 0 {
		t.Fatalf("the removed game should be gone from the list: %+v", items)
	}
}
