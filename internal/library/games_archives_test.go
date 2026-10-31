package library

import (
	"archive/zip"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/gamearchive"
	"github.com/wittywilla/wittyplay"
)

func copyArchive(t *testing.T, name, to string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "gamearchive", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverGamesInEveryKindOfArchive(t *testing.T) {
	root := t.TempDir()
	copyArchive(t, "cart.7z", filepath.Join(root, "Super Nintendo (SNES)", "A Game (USA).7z"))
	copyArchive(t, "cart.rar", filepath.Join(root, "SNES", "B Game (Europe).rar"))
	copyArchive(t, "disc-solid.7z", filepath.Join(root, "Playstation (PSX)", "C Game.7z"))
	copyArchive(t, "cart.7z", filepath.Join(root, "Unsorted", "D Game.7z")) // sorted by what's inside
	copyArchive(t, "disc-solid.7z", filepath.Join(root, "Unsorted", "E Game.7z"))
	copyArchive(t, "locked-names.7z", filepath.Join(root, "SNES", "F Locked.7z"))
	copyArchive(t, "split.part1.rar", filepath.Join(root, "SNES", "G Split.part1.rar"))
	copyArchive(t, "split.part2.rar", filepath.Join(root, "SNES", "G Split.part2.rar"))
	copyArchive(t, "split.part3.rar", filepath.Join(root, "SNES", "G Split.part3.rar"))
	copyArchive(t, "split7.7z.001", filepath.Join(root, "SNES", "H Split.7z.001"))
	copyArchive(t, "split7.7z.002", filepath.Join(root, "SNES", "H Split.7z.002"))
	copyArchive(t, "split7.7z.003", filepath.Join(root, "SNES", "H Split.7z.003"))
	writeFiles(t, root, "SNES/backup.001") // not an archive, whatever folder it's in
	got, err := discoverGames(root, "auto")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Playstation (PSX)/C Game.7z=ps1", "SNES/B Game (Europe).rar=snes", "SNES/G Split.part1.rar=snes", "SNES/H Split.7z.001=snes",
		"Super Nintendo (SNES)/A Game (USA).7z=snes", "Unsorted/D Game.7z=snes",
	}
	if !equal(names(got, root), want) {
		t.Fatalf("got %v, want %v", names(got, root), want)
	}
	skipped := skippedGameFiles(root, "auto")
	// E Game holds only a .cue and .bin, which say nothing without a folder to go by.
	if skipped["encrypted"] != 1 || skipped[".7z"] != 1 || len(skipped) != 2 {
		t.Fatalf("skipped: %v", skipped)
	}

	// A library of one console: anything in it holding a game for that console.
	got, err = discoverGames(filepath.Join(root, "Playstation (PSX)"), "ps1")
	if err != nil || len(got) != 1 {
		t.Fatalf("one console: %v %v", got, err)
	}
	got, _ = discoverGames(filepath.Join(root, "Playstation (PSX)"), "snes")
	if len(got) != 0 {
		t.Fatalf("a disc isn't a Super Nintendo game: %v", got)
	}
}

func TestArchiveTitlesKeepTheirTags(t *testing.T) {
	for in, want := range map[string]string{
		"Mega Game, The (USA) (Rev 1) [!].7z": "Mega Game, The (USA) (Rev 1) [!]",
		"Game (USA).tar.gz":                   "Game (USA)",
		"Game (USA).sfc.7z":                   "Game (USA)",
		"Game (Japan).part1.rar":              "Game (Japan)",
		"Game (Japan).7z.001":                 "Game (Japan)",
		"Game (Japan).sfc.xz":                 "Game (Japan)",
		"Game.sfc.gz":                         "Game",
		"Game.sfc":                            "Game",
		"Dr. Game.zip":                        "Dr. Game",
	} {
		if got := gameBaseName(in); got != want {
			t.Errorf("%s: got %s, want %s", in, got, want)
		}
	}
	title, note := ParseGameName(gameBaseName("Mega Game, The (USA) (Rev 1) [!].7z"))
	if title != "The Mega Game" || note != "USA, Rev 1" {
		t.Errorf("%q, %q", title, note)
	}
}

// Scanning an unchanged archive doesn't open it.
func TestScanRemembersWhatArchivesHold(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	store := &Store{DB: d}
	ctx := context.Background()
	root := t.TempDir()
	lib, err := store.CreateLibrary(ctx, "Games", "games", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(root, "Game (USA).7z")
	copyArchive(t, "cart.7z", game)
	gone := filepath.Join(root, "Gone.7z")
	copyArchive(t, "cart.7z", gone)
	unreadable := filepath.Join(root, "Rubbish.7z")
	if err := os.WriteFile(unreadable, []byte("not an archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	// How many games were found, and whether anything had to be read to find them:
	// a lister only exists once something did.
	scan := func() (int, bool) {
		ix := newArchiveIndex(store, lib.ID)
		defer ix.close()
		c, err := ix.discover(root, "snes")
		if err != nil {
			t.Fatal(err)
		}
		ix.save(ctx, true)
		return len(c), ix.lister != nil
	}
	if n, read := scan(); n != 2 || !read {
		t.Fatalf("first scan: %d, %v", n, read)
	}
	// The scan's progress hears about each archive it really opens, and only those.
	ix := newArchiveIndex(store, lib.ID)
	var told []string
	ix.reading = func(path string) { told = append(told, filepath.Base(path)) }
	fresh := filepath.Join(root, "Fresh.7z")
	copyArchive(t, "cart.7z", fresh)
	if _, err := ix.discover(root, "snes"); err != nil {
		t.Fatal(err)
	}
	ix.close()
	if len(told) != 1 || told[0] != "Fresh.7z" {
		t.Fatalf("told about %v", told)
	}
	if err := os.Remove(fresh); err != nil {
		t.Fatal(err)
	}
	if n, read := scan(); n != 2 || read {
		t.Fatalf("second scan: %d, %v", n, read)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(game, future, future); err != nil {
		t.Fatal(err)
	}
	if n, read := scan(); n != 2 || !read {
		t.Fatalf("a changed file wasn't read: %d, %v", n, read)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	if n, read := scan(); n != 1 || read {
		t.Fatalf("after removing one: %d, %v", n, read)
	}
	var rows int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM game_archives WHERE library_id = ?`, lib.ID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 { // Game (USA).7z, and that Rubbish.7z is rubbish
		t.Fatalf("rows: %d", rows)
	}
}

// A zip game stays the same item from scan to scan: its saves hang on that.
func TestZipGamesKeepTheirItem(t *testing.T) {
	s, lib, root := scanFixture(t)
	ctx := context.Background()
	s.WikidataOn = func() bool { return false }
	if _, err := s.Store.DB.ExecContext(ctx, `UPDATE libraries SET type = 'games', console = 'nes' WHERE id = ?`, lib.ID); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, "Game (USA).zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	e, _ := w.Create("Game (USA).nes")
	_, _ = e.Write([]byte("rom"))
	_ = w.Close()
	_ = f.Close()
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it, err := s.Store.itemByPath(ctx, lib.ID, p)
	if err != nil {
		t.Fatal(err)
	}
	if it.Console != "nes" || it.Title != "Game" || it.Tagline != "USA" {
		t.Fatalf("got %+v", it)
	}
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	again, err := s.Store.itemByPath(ctx, lib.ID, p)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != it.ID {
		t.Fatalf("item %d became %d", it.ID, again.ID)
	}
}

// Not a test of the code so much as of a real folder: run by hand with WMS_GAMES_FOLDER
// set, to see what a scan of it finds and how long it takes. It only reads that folder,
// keeps its database in a temporary one, and prints counts, never a name.
func TestRealGamesFolder(t *testing.T) {
	root := os.Getenv("WMS_GAMES_FOLDER")
	if root == "" {
		t.Skip("set WMS_GAMES_FOLDER to run this")
	}
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	if err := d.SetSetting(ctx, "saveMetadata", "0"); err != nil {
		t.Fatal(err)
	}
	store := &Store{DB: d}
	lib, err := store.CreateLibrary(ctx, "Games", "games", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `UPDATE libraries SET console = 'auto' WHERE id = ?`, lib.ID); err != nil {
		t.Fatal(err)
	}
	sc := &Scanner{Store: store, ImagesDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), WikidataOn: func() bool { return false }}

	start := time.Now()
	ix := newArchiveIndex(store, lib.ID)
	c, err := ix.discover(root, "auto")
	ix.close()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("reading every archive's headers, nothing remembered: %d games in %v", len(c), time.Since(start).Round(time.Millisecond))

	// How often there's a choice of which file to play.
	lister := gamearchive.NewLister()
	several := map[string]int{}
	for _, g := range c {
		if !gamearchive.IsArchive(g.path) {
			continue
		}
		l, err := lister.List(g.path, gamearchive.ScanBudget)
		if err != nil {
			continue
		}
		n := 0
		for _, e := range l.Entries {
			if wittyplay.ByID(g.console).Accepts(strings.ToLower(filepath.Ext(e.Name))) {
				n++
			}
		}
		if n > 1 {
			several[g.console]++
		}
	}
	lister.Close()
	t.Logf("archives with more than one candidate file, by console: %v", several)

	start = time.Now()
	if err := sc.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	t.Logf("first full scan, no online lookups: %v", time.Since(start).Round(time.Millisecond))
	counts, err := store.GameCounts(ctx, lib.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("  %-6s %d games", k, counts[k])
	}
	t.Logf("  left out: %v", store.SkippedGames(ctx, lib.ID))

	start = time.Now()
	if err := sc.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	t.Logf("second full scan, nothing changed: %v", time.Since(start).Round(time.Millisecond))
}
