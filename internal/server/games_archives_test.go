package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittyplay"
)

func tarGz(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	w := tar.NewWriter(z)
	if err := w.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Size: int64(len(data)), Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(data)
	_ = w.Close()
	_ = z.Close()
	return b.Bytes()
}

// addArchiveGame makes a one-game library and returns the game's item.
func addArchiveGame(t *testing.T, g *gameServer, file string, data []byte) *library.Item {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, file), data, 0o644); err != nil {
		t.Fatal(err)
	}
	w := g.do("POST", "/api/libraries", g.admin, map[string]any{"name": "Archives", "type": "games", "console": wittyplay.TestConsole, "paths": []string{dir}})
	if w.Code != 201 {
		t.Fatalf("library: %d %s", w.Code, w.Body)
	}
	var lib library.Library
	if err := json.Unmarshal(w.Body.Bytes(), &lib); err != nil {
		t.Fatal(err)
	}
	var it *library.Item
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		items, _, err := g.s.Store.Items(context.Background(), library.ItemQuery{LibraryID: lib.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) > 0 && g.s.Scanner.Snapshot(lib.ID) == nil {
			it = items[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if it == nil {
		t.Fatal("the scan found no game")
	}
	return it
}

func TestPreparingAGameInAnArchive(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	it := addArchiveGame(t, g, "Mega Game (USA).tar.gz", tarGz(t, "Mega Game (USA).wittytest", []byte("game")))
	if it.Title != "Mega Game" || it.Tagline != "USA" {
		t.Fatalf("title %q, note %q", it.Title, it.Tagline)
	}
	path := fmt.Sprintf("/api/games/%d/prepare", it.ID)
	deadline := time.Now().Add(30 * time.Second)
	for {
		w := g.do("POST", path, g.admin, nil)
		if w.Code != 200 {
			t.Fatalf("prepare: %d %s", w.Code, w.Body)
		}
		var st struct {
			State string `json:"state"`
			Total int64  `json:"total"`
			Done  int64  `json:"done"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
			t.Fatal(err)
		}
		if st.State == "ready" {
			if st.Done != 4 || st.Total != 4 {
				t.Fatalf("got %+v", st)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("still %+v", st)
		}
		time.Sleep(10 * time.Millisecond)
	}

	gs := g.s.games()
	rom, cleanup, err := gs.prepareROM(context.Background(), it.Path, wittyplay.ByID(wittyplay.TestConsole))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if filepath.Ext(rom) != ".wittytest" || !strings.HasPrefix(rom, gs.dir) {
		t.Fatalf("got %s", rom)
	}
	if b, _ := os.ReadFile(rom); string(b) != "game" {
		t.Fatalf("got %q", b)
	}
	// A save belongs to the account and the item, whatever the cache does.
	if want := filepath.Join(gs.dir, "saves", "u1", "i"+fmt.Sprint(it.ID)); gs.saveDir(1, it.ID) != want {
		t.Fatalf("got %s", gs.saveDir(1, it.ID))
	}
	// Someone who can't see the game can't make the server unpack it.
	if w := g.do("POST", path, "", nil); w.Code != 401 {
		t.Fatalf("no sign-in: %d", w.Code)
	}
}

func TestPreparingAGameNotInAnArchiveIsInstant(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	it := addArchiveGame(t, g, "Plain.wittytest", []byte("game"))
	w := g.do("POST", fmt.Sprintf("/api/games/%d/prepare", it.ID), g.admin, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ready"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	gs := g.s.games()
	rom, cleanup, err := gs.prepareROM(context.Background(), it.Path, wittyplay.ByID(wittyplay.TestConsole))
	if err != nil || rom != it.Path || cleanup != nil {
		t.Fatalf("%s %v", rom, err)
	}
}

func TestPreparingAnArchiveWithNoCoreSaysSo(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	it := addArchiveGame(t, g, "Game.tar.gz", tarGz(t, "Game.wittytest", []byte("game")))
	if _, err := g.s.DB.ExecContext(context.Background(), `UPDATE items SET console = 'snes' WHERE id = ?`, it.ID); err != nil {
		t.Fatal(err)
	}
	w := g.do("POST", fmt.Sprintf("/api/games/%d/prepare", it.ID), g.admin, nil)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "needCore") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

// The whole way: a game in an archive, started with a real runner, over the same routes
// any client uses, and started again from what's already unpacked.
func TestPlayingAGameFromAnArchive(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no FFmpeg on PATH")
	}
	runner := filepath.Join(t.TempDir(), "wittyplay-host")
	if runtime.GOOS == "windows" {
		runner += ".exe"
	}
	build := exec.Command("go", "build", "-o", runner, "./cmd/wittyplay-host")
	build.Dir = filepath.Join("..", "..", "modules", "wittyplay")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	t.Setenv("WMS_WITTYPLAY_RUNNER", runner)
	g := newGameServer(t)
	defer g.s.StopGames()
	g.s.Tools.FFmpeg = ffmpegPath
	out, _ := exec.Command(ffmpegPath, "-hide_banner", "-encoders").Output()
	for _, n := range []string{"libx264", "libvpx", "libopus"} {
		g.s.Tools.Encoders[n] = strings.Contains(string(out), " "+n+" ")
	}
	it := addArchiveGame(t, g, "Game (World).tar.gz", tarGz(t, "Game (World).wittytest", []byte("game")))
	for round := range 2 {
		w := g.do("POST", fmt.Sprintf("/api/games/%d/play", it.ID), g.admin, map[string]any{"resume": false, "client": "native"})
		if w.Code != 201 {
			t.Fatalf("round %d: play: %d %s", round, w.Code, w.Body)
		}
		var sess struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &sess); err != nil || sess.ID == "" {
			t.Fatalf("session: %v %s", err, w.Body)
		}
		if w := g.do("DELETE", "/api/play/"+sess.ID, g.admin, nil); w.Code != 200 {
			t.Fatalf("stop: %d %s", w.Code, w.Body)
		}
	}
	if g.s.games().archives.Used() == 0 {
		t.Fatal("nothing in the cache")
	}
}
