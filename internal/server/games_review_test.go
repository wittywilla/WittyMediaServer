package server

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/gameid/fixture"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittyplay"
)

// reviewLibrary makes an auto library from files, and waits for its scan.
func reviewLibrary(t *testing.T, g *gameServer, files map[string][]byte) *library.Library {
	t.Helper()
	dir := t.TempDir()
	for name, b := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w := g.do("POST", "/api/libraries", g.admin, map[string]any{"name": "Games", "type": "games", "console": "auto", "paths": []string{dir}})
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var lib library.Library
	jsonBody(t, w, &lib)
	waitForScan(t, g, lib.ID)
	return &lib
}

func writeTestZip(t *testing.T, to string, files map[string][]byte) {
	t.Helper()
	f, err := os.Create(to)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, b := range files {
		e, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func consoleForTest(t *testing.T, id string) *wittyplay.Console {
	t.Helper()
	c := wittyplay.ByID(id)
	if c == nil {
		t.Fatalf("no console %s", id)
	}
	return c
}

func waitForScan(t *testing.T, g *gameServer, id int64) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	time.Sleep(50 * time.Millisecond)
	for g.s.Scanner.Snapshot(id) != nil || g.s.Scanner.Running(id) {
		if time.Now().After(deadline) {
			t.Fatal("the scan did not finish in time")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNeedsALookThroughTheAPI(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	lib := reviewLibrary(t, g, map[string][]byte{
		"NES/Odd Dump (USA).nes":    make([]byte, 64<<10),
		"NES/Readable (USA).nes":    fixture.NES(),
		"Playstation/Album.bin":     make([]byte, 4096),
		"Playstation/Album.cue":     []byte("FILE \"Album.bin\" BINARY\n  TRACK 01 AUDIO\n"),
		"Nintendo 3DS/Sealed.cia":   fixture.CIA(fixture.Scrambled(1<<14), 1, 0x00040000),
		"Nintendo 3DS/Readable.cia": fixture.CIA(fixture.NCCH(true, true, 1<<14), 0, 0x00040000),
	})
	path := fmt.Sprintf("/api/libraries/%d/games/review", lib.ID)
	// Only administrators, on every route of it.
	for _, c := range []struct{ method, path string }{
		{"GET", path}, {"POST", path + "/apply"}, {"GET", fmt.Sprintf("/api/libraries/%d/games/import", lib.ID)},
		{"PUT", "/api/games/1/version"}, {"POST", "/api/games/1/split"}, {"PUT", "/api/games/1/console"}, {"POST", "/api/games/1/not-a-game"},
	} {
		if w := g.do(c.method, c.path, g.other, map[string]any{}); w.Code != 403 {
			t.Errorf("%s %s as someone else: %d", c.method, c.path, w.Code)
		}
		if w := g.do(c.method, c.path, "", map[string]any{}); w.Code != 401 {
			t.Errorf("%s %s signed out: %d", c.method, c.path, w.Code)
		}
	}

	var list library.GameReview
	w := g.do("GET", path+"?kind=needs", g.admin, nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	jsonBody(t, w, &list)
	if list.Total != 1 || list.Items[0].Name != "Odd Dump (USA).nes" || list.Items[0].Reason == "" || list.Items[0].Suggest[0].Console != "nes" {
		t.Fatalf("%+v", list)
	}
	if strings.Contains(list.Items[0].Path, ":") || filepath.IsAbs(list.Items[0].Path) {
		t.Errorf("the list gave away a path on the server: %s", list.Items[0].Path)
	}
	if list.Counts["blocked"] != 1 || list.Counts["audio"] != 1 {
		t.Fatalf("counts: %v", list.Counts)
	}
	// Bad requests change nothing.
	for _, body := range []map[string]any{
		{"ids": []int64{list.Items[0].ID}, "action": "add", "console": "no-such-console"},
		{"ids": []int64{list.Items[0].ID}, "action": "delete"},
		{"ids": []int64{}, "action": "ignore"},
	} {
		if w := g.do("POST", path+"/apply", g.admin, body); w.Code != 400 {
			t.Errorf("%v: %d", body, w.Code)
		}
	}
	// One click.
	if w := g.do("POST", path+"/apply", g.admin, map[string]any{"ids": []int64{list.Items[0].ID}, "action": "add", "console": "nes"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	waitForScan(t, g, lib.ID)
	items, _, err := g.s.Store.Items(context.Background(), library.ItemQuery{LibraryID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]*library.Item{}
	for _, it := range items {
		found[filepath.Base(it.Path)] = it
	}
	odd := found["Odd Dump (USA).nes"]
	if odd == nil || odd.Console != "nes" {
		t.Fatalf("%+v", items)
	}
	if found["Album.cue"] != nil {
		t.Error("a music CD is listed as a game")
	}

	// An encrypted 3DS game is listed, and says so before anyone presses Play, in plain
	// words. Who can't fix it doesn't get the details.
	sealed := found["Sealed.cia"]
	if sealed == nil {
		t.Fatalf("%+v", items)
	}
	var d library.GameDetails
	w = g.do("GET", fmt.Sprintf("/api/games/%d/metadata", sealed.ID), g.admin, nil)
	jsonBody(t, w, &d)
	if d.Problem != "encrypted" || !strings.Contains(d.ProblemText, "won't decrypt") || d.DetectedBy != "bytes" || !strings.Contains(d.DetectWhy, "NCCH") {
		t.Fatalf("%+v", d)
	}
	w = g.do("GET", fmt.Sprintf("/api/games/%d/metadata", sealed.ID), g.other, nil)
	var plain library.GameDetails
	jsonBody(t, w, &plain)
	if plain.Problem != "encrypted" || plain.ProblemText == "" || plain.DetectWhy != "" {
		t.Fatalf("%+v", plain)
	}

	// Fixing one game from its page.
	if w := g.do("POST", fmt.Sprintf("/api/games/%d/not-a-game", odd.ID), g.admin, nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	waitForScan(t, g, lib.ID)
	if it, err := g.s.Store.Item(context.Background(), odd.ID); err == nil && !it.Missing {
		t.Fatalf("still listed: %+v", it)
	}
	w = g.do("GET", path+"?kind=ignored", g.admin, nil)
	jsonBody(t, w, &list)
	if list.Total != 1 {
		t.Fatalf("ignored: %+v", list)
	}
	readable := found["Readable.cia"]
	if w := g.do("PUT", fmt.Sprintf("/api/games/%d/console", readable.ID), g.admin, map[string]any{"console": "no-such"}); w.Code != 400 {
		t.Errorf("no such console: %d", w.Code)
	}
}

// A choice in an archive has to be a file that's in it.
func TestChoosingAVersionInAnArchive(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "Mega Game.zip")
	writeTestZip(t, zipPath, map[string][]byte{"Mega Game (U).smc": fixture.SNES("MEGA GAME"), "Mega Game (J).smc": fixture.SNES("MEGA GAME J"), "Mega Game (U) [b1].smc": fixture.SNES("BAD")})
	w := g.do("POST", "/api/libraries", g.admin, map[string]any{"name": "SNES", "type": "games", "console": "snes", "paths": []string{dir}})
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var lib library.Library
	jsonBody(t, w, &lib)
	waitForScan(t, g, lib.ID)
	items, _, _ := g.s.Store.Items(context.Background(), library.ItemQuery{LibraryID: lib.ID})
	if len(items) != 1 {
		t.Fatalf("%d", len(items))
	}
	id := items[0].ID
	var v struct {
		Inside []struct {
			Entry   string `json:"entry"`
			Current bool   `json:"current"`
			Altered bool   `json:"altered"`
		} `json:"inside"`
	}
	jsonBody(t, g.do("GET", fmt.Sprintf("/api/games/%d/versions", id), g.other, nil), &v)
	if len(v.Inside) != 3 {
		t.Fatalf("%+v", v)
	}
	for _, x := range v.Inside {
		if x.Current != (x.Entry == "Mega Game (U).smc") {
			t.Errorf("the plain American one should be chosen: %+v", v.Inside)
		}
		if x.Altered != strings.Contains(x.Entry, "[b1]") {
			t.Errorf("%+v", x)
		}
	}
	path := fmt.Sprintf("/api/games/%d/version", id)
	if w := g.do("PUT", path, g.admin, map[string]any{"entry": "../../elsewhere.smc"}); w.Code != 400 {
		t.Errorf("a file not in the archive: %d", w.Code)
	}
	if w := g.do("PUT", path, g.admin, map[string]any{"entry": "Mega Game (J).smc"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if got := g.s.Store.GameEntry(context.Background(), id); got != "Mega Game (J).smc" {
		t.Fatalf("%q", got)
	}
	v.Inside = nil
	jsonBody(t, g.do("GET", fmt.Sprintf("/api/games/%d/versions", id), g.other, nil), &v)
	for _, x := range v.Inside {
		if x.Current != (x.Entry == "Mega Game (J).smc") {
			t.Errorf("%+v", x)
		}
	}
}

// A game on several discs is offered to a core as a playlist of its discs, wherever they
// are, and everything made for it goes when the game ends.
func TestDiscsBecomeAPlaylist(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	cue := func(bin string) []byte { return []byte("FILE \"" + bin + "\" BINARY\n  TRACK 01 MODE2/2352\n") }
	dir := t.TempDir()
	mk := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	mk("Mega Game Disc 1.bin", fixture.Raw(fixture.PS1("SLUS_100.01")))
	first := mk("Mega Game Disc 1.cue", cue("Mega Game Disc 1.bin"))
	second := filepath.Join(dir, "mega-game-disc2of2-u-slus-10002-.zip")
	writeTestZip(t, second, map[string][]byte{"Mega Game Disc 2.bin": fixture.Raw(fixture.PS1("SLUS_100.02")), "Mega Game Disc 2.cue": cue("Mega Game Disc 2.bin")})
	w := g.do("POST", "/api/libraries", g.admin, map[string]any{"name": "PS1", "type": "games", "console": "ps1", "paths": []string{dir}})
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var lib library.Library
	jsonBody(t, w, &lib)
	waitForScan(t, g, lib.ID)
	ctx := context.Background()
	items, _, _ := g.s.Store.Items(ctx, library.ItemQuery{LibraryID: lib.ID})
	if len(items) != 1 || items[0].Title != "Mega Game" || items[0].Path != first {
		t.Fatalf("%+v", items)
	}
	discs, err := g.s.Store.GameDiscs(ctx, items[0].ID)
	if err != nil || len(discs) != 2 || discs[1].Path != second {
		t.Fatalf("%+v %v", discs, err)
	}
	gs := g.s.games()
	rom, cleanup, err := gs.prepareDiscs(ctx, discs, consoleForTest(t, "ps1"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := os.ReadFile(rom)
	if err != nil {
		t.Fatal(err)
	}
	if string(list) != "disc1/Mega Game Disc 1.cue\ndisc2/Mega Game Disc 2.cue\n" {
		t.Fatalf("%q", list)
	}
	for _, f := range []string{"disc1/Mega Game Disc 1.bin", "disc1/Mega Game Disc 1.cue", "disc2/Mega Game Disc 2.bin", "disc2/Mega Game Disc 2.cue"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(rom), filepath.FromSlash(f))); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	cleanup()
	if _, err := os.Stat(filepath.Dir(rom)); !os.IsNotExist(err) {
		t.Errorf("the folder stayed: %v", err)
	}
	if _, err := os.Stat(first); err != nil {
		t.Errorf("the original went with it: %v", err)
	}
}

// A full server says so before unpacking or patching anything.
func TestAFullServerSaysSoFirst(t *testing.T) {
	t.Setenv("WMS_WITTYPLAY_RUNNER", os.Args[0])
	g := newGameServer(t)
	defer g.s.StopGames()
	item := runnerGameItem(t, g)
	if err := g.s.DB.SetSetting(context.Background(), "gamingMaxSessions", "0"); err != nil {
		t.Fatal(err)
	}
	w := g.do("POST", fmt.Sprintf("/api/games/%d/play", item), g.admin, map[string]any{})
	if w.Code != 503 || !strings.Contains(w.Body.String(), "as many games") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
