// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"archive/zip"
	"context"
	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/gameid/fixture"
)

func writeFiles(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, gameBytes(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// gameBytes makes a file with what a real game of that kind has, so that a scan believes
// it. Anything else gets one byte.
func gameBytes(name string) []byte {
	b := headerBytes(name)
	// Different games have different contents, and checksums would say so.
	if len(b) > 1024 {
		copy(b[len(b)-256:], name)
	}
	return b
}

func headerBytes(name string) []byte {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".sfc", ".smc":
		return fixture.SNES("MADE UP GAME")
	case ".nes":
		return fixture.NES()
	case ".gba":
		return fixture.GBA("MADE UP", "AXXE")
	case ".gb", ".gbc":
		return fixture.GB("MADE UP")
	case ".n64", ".z64":
		return fixture.N64("MADE UP", "NXXE")
	case ".nds":
		return fixture.NDS("MADE UP", "AXXE")
	case ".md", ".gen":
		return fixture.MegaDrive("MADE UP", "GM 00000000-00")
	case ".sms":
		return fixture.MasterSystem(false)
	case ".gg":
		return fixture.MasterSystem(true)
	case ".pce", ".a26", ".ws", ".vb":
		return make([]byte, 64<<10)
	}
	return []byte("x")
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
	modtest.Need(t, "wittyplay")
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
	modtest.Need(t, "wittyplay")
	root := t.TempDir()
	writeFiles(t, root, "a.sfc", "b.gba", "mystery.bin", "readme.txt")
	write := func(rel string, b []byte) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Sega Genesis/c.bin", fixture.MegaDrive("MADE UP", "GM 00000000-00"))
	write("Atari 2600/d.bin", make([]byte, 4096))
	got, _ := discoverGames(root, "auto")
	want := []string{"Atari 2600/d.bin=a2600", "Sega Genesis/c.bin=md", "a.sfc=snes", "b.gba=gba"}
	if !equal(names(got, root), want) {
		t.Fatalf("got %v, want %v", names(got, root), want)
	}
}

func TestDiscoverGamesMultiFile(t *testing.T) {
	modtest.Need(t, "wittyplay")
	root := t.TempDir()
	writeFiles(t, root, "Game/Game (Track 2).bin", "Set/Disc 1.cue", "Set/Disc 2.cue")
	raw := fixture.Raw(fixture.PS1("SLUS_123.45"))
	_ = os.WriteFile(filepath.Join(root, "Game", "Game (Track 1).bin"), raw, 0o644)
	_ = os.WriteFile(filepath.Join(root, "Solo.bin"), raw, 0o644)
	_ = os.WriteFile(filepath.Join(root, "Set", "Disc 1.bin"), fixture.Raw(fixture.PS1("SLUS_123.46")), 0o644)
	_ = os.WriteFile(filepath.Join(root, "Set", "Disc 2.bin"), fixture.Raw(fixture.PS1("SLUS_123.47")), 0o644)
	_ = os.WriteFile(filepath.Join(root, "Game", "Game.cue"), []byte("FILE \"Game (Track 1).bin\" BINARY\n  TRACK 01 MODE2/2352\nFILE \"Game (Track 2).bin\" BINARY\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "Set", "Disc 1.cue"), []byte("FILE \"Disc 1.bin\" BINARY\n  TRACK 01 MODE2/2352\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "Set", "Disc 2.cue"), []byte("FILE \"Disc 2.bin\" BINARY\n  TRACK 01 MODE2/2352\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "Set", "Game.m3u"), []byte("# discs\nDisc 1.cue\nDisc 2.cue\n../../outside.bin\n"), 0o644)
	got, _ := discoverGames(root, "ps1")
	// Solo.bin is the same game as Game.cue: its checksum says so, so it isn't listed twice.
	want := []string{"Game/Game.cue=ps1", "Set/Game.m3u=ps1"}
	if !equal(names(got, root), want) {
		t.Fatalf("got %v, want %v", names(got, root), want)
	}
}

func TestDiscoverGamesZip(t *testing.T) {
	modtest.Need(t, "wittyplay")
	root := t.TempDir()
	mk := func(name, inner string) {
		f, err := os.Create(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		w := zip.NewWriter(f)
		e, _ := w.Create(inner)
		_, _ = e.Write(gameBytes(inner))
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
	modtest.Need(t, "wittyplay")
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

func TestSkippedGameFilesAreCounted(t *testing.T) {
	root := t.TempDir()
	snes := filepath.Join(root, "Super Nintendo (SNES)")
	if err := os.MkdirAll(snes, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Playable.sfc", "Packed one.7z", "Packed two.7z", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(snes, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Not really archives, whatever they're called: counted by that, not by extension.
	got := skippedGameFiles(root, "auto")
	if got["damaged"] != 2 || len(got) != 1 {
		t.Fatalf("want only two damaged archives reported, got %v", got)
	}
}

// With no WittyPlay nothing can be told to be a game, and what's there stays.
func TestGamesStayWithoutWittyPlay(t *testing.T) {
	if os.Getenv(modtest.EnvCatalog) != "" {
		t.Skip("this is about a server without it")
	}
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := &Store{DB: database}
	root := t.TempDir()
	writeFiles(t, root, "Hello.sfc")
	sc := NewScanner(store, nil, t.TempDir(), func() string { return "" }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	lib, err := store.CreateLibrary(ctx, "Games", "games", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	it := &Item{LibraryID: lib.ID, Kind: "game", Path: filepath.Join(root, "Hello.sfc"), Title: "Hello"}
	if err := store.upsertItem(ctx, it); err != nil {
		t.Fatal(err)
	}
	if err := sc.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	items, _, err := store.Items(ctx, ItemQuery{LibraryID: lib.ID})
	if err != nil || len(items) != 1 {
		t.Fatalf("%d items, %v", len(items), err)
	}
}

// A PS3 disc or a Wii U game kept as a folder is found by the one file each dump has, and the
// folder of games around them isn't mistaken for one.
func TestDumpGame(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "PS3/Some Game/PS3_DISC.SFB", "PS3/Some Game/PS3_GAME/USRDIR/EBOOT.BIN", "PS3/Some Game/PS3_GAME/USRDIR/data.bin",
		"Wii U/Other/code/game.rpx", "Wii U/Other/code/lib.rpl", "Wii U/Other/content/00000001.app", "Wii U/Other/meta/meta.xml",
		"Wii U/NotADump/code/game.rpx", "PS3/Loose/PS3_DISC.SFB")
	for dir, want := range map[string]string{
		"PS3/Some Game":          "PS3/Some Game/PS3_DISC.SFB",
		"Wii U/Other":            "Wii U/Other/code/game.rpx",
		"Wii U/NotADump":         "", // no meta folder: not a dump's root
		"PS3/Loose":              "", // no PS3_GAME beside it
		"PS3":                    "",
		"PS3/Some Game/PS3_GAME": "",
	} {
		got := dumpGame(filepath.Join(root, filepath.FromSlash(dir)))
		if want != "" {
			want = filepath.Join(root, filepath.FromSlash(want))
		}
		if got != want {
			t.Errorf("%s: %q, want %q", dir, got, want)
		}
	}
}
