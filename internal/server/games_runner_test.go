package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittyplay"
	"github.com/wittywilla/wittyplay/ipc"
	"github.com/wittywilla/wittyplay/stream"
)

// downloadedRunner is what an install of a WittyPlay version leaves on disk. This test
// program is the runner: it refuses the flags it is started with and exits at once,
// which is just a runner dying before its game is ready.
func downloadedRunner(t *testing.T) (*modmgr.Manager, string) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(t.TempDir(), "modules")
	const ver = "0.9.0"
	var man wittymodule.Manifest
	if err := json.Unmarshal(wittyplay.ManifestJSON, &man); err != nil {
		t.Fatal(err)
	}
	man.Version = ver
	raw, _ := json.Marshal(man)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	prog, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, man.ID, "versions", ver)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var sums strings.Builder
	for n, b := range map[string][]byte{wittymodule.ManifestFile: raw, man.EntryFile(runtime.GOOS): prog} {
		if err := os.WriteFile(filepath.Join(dir, n), b, 0o755); err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(b)
		fmt.Fprintf(&sums, "%s  %s/%s\n", hex.EncodeToString(h[:]), man.Top(runtime.GOOS, runtime.GOARCH), n)
	}
	list := []byte(sums.String())
	if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS.txt"), list, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS.txt.sig"), update.Sign(key, list), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "state.json"), []byte(`{"wittyplay":{"active":"`+ver+`"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := modmgr.New(modmgr.Config{Dir: base, Server: "0.4.0", Builtins: [][]byte{wittyplay.ManifestJSON}, Keys: []ed25519.PublicKey{pub},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	t.Cleanup(m.Close)
	return m, dir
}

func gameItem(t *testing.T, g *gameServer) int64 {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Game.wittytest"), []byte("game"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := g.do("POST", "/api/libraries", g.admin, map[string]any{"name": "Test", "type": "games", "console": "wittytest", "paths": []string{dir}})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var lib library.Library
	_ = json.Unmarshal(w.Body.Bytes(), &lib)
	for range 200 {
		items, _, err := g.s.Store.Items(context.Background(), library.ItemQuery{LibraryID: lib.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) > 0 {
			return items[0].ID
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the scan did not finish in time")
	return 0
}

// The order of runners, a fresh check each time, and a runner that keeps dying before
// its game is ready going out of use, with no restart.
func TestDownloadedGameRunner(t *testing.T) {
	t.Setenv("WMS_WITTYPLAY_RUNNER", "")
	g := newGameServer(t)
	defer g.s.StopGames()
	ctx := context.Background()
	if err := g.s.DB.SetSetting(ctx, "gamingRunner", "builtin"); err != nil {
		t.Fatal(err)
	}
	m, dir := downloadedRunner(t)
	g.s.Modules = m

	path, _, ver := g.s.gameRunner(ctx)
	if ver != "0.9.0" || filepath.Dir(path) != dir {
		t.Fatalf("%q %q %s", path, ver, m.Report().Modules[0].Notice)
	}
	t.Setenv("WMS_WITTYPLAY_RUNNER", "my-runner")
	if path, _, ver := g.s.gameRunner(ctx); path != "my-runner" || ver != "" {
		t.Fatalf("the setting for developers comes first: %q %q", path, ver)
	}
	t.Setenv("WMS_WITTYPLAY_RUNNER", "")

	// A scanner looking over a new program can hold it past the time a game has to load,
	// which doesn't count against it, so a few more tries are allowed for that.
	item := gameItem(t, g)
	tries := 0
	for ; tries < 6 && m.Report().Modules[0].Version == "0.9.0"; tries++ {
		w := g.do("POST", "/api/games/"+itoa64(item)+"/play", g.admin, map[string]any{})
		if w.Code != 500 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	if tries < 3 {
		t.Fatalf("out of use after %d", tries)
	}
	st := m.Report().Modules[0]
	if st.Version != wittyplayVersion(t) || !strings.Contains(st.Notice, "kept failing") || !strings.Contains(st.LastFail, "game runner stopped") {
		t.Fatalf("%+v", st)
	}
	if _, _, ver := g.s.gameRunner(ctx); ver != "" {
		t.Fatalf("still %q", ver)
	}
}

func wittyplayVersion(t *testing.T) string {
	t.Helper()
	man, err := wittymodule.ParseManifest(wittyplay.ManifestJSON)
	if err != nil {
		t.Fatal(err)
	}
	return man.Version
}

// A signature is checked for each game, so a changed file never gets to run.
func TestChangedRunnerIsNotRun(t *testing.T) {
	t.Setenv("WMS_WITTYPLAY_RUNNER", "")
	g := newGameServer(t)
	defer g.s.StopGames()
	ctx := context.Background()
	if err := g.s.DB.SetSetting(ctx, "gamingRunner", "builtin"); err != nil {
		t.Fatal(err)
	}
	m, dir := downloadedRunner(t)
	g.s.Modules = m
	if _, _, ver := g.s.gameRunner(ctx); ver == "" {
		t.Fatal("not in use to begin with")
	}
	if err := os.WriteFile(filepath.Join(dir, "extra"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if path, _, ver := g.s.gameRunner(ctx); ver != "" || strings.HasPrefix(path, dir) {
		t.Fatalf("%q %q", path, ver)
	}
}

// The errors this server counts against a runner are the ones the session really gives.
func TestRunnerFailedKnowsSessionErrors(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	start := func(runner string) error {
		m := stream.NewManager(stream.Config{Runner: runner, FFmpeg: "ffmpeg", HaveEncoder: func(string) bool { return false }, Log: log})
		defer m.StopAll()
		_, err := m.Start(stream.Options{UserID: 1, ItemID: 1, Core: ipc.TestCore, ROM: exe, SaveDir: t.TempDir()})
		if err == nil {
			t.Fatal("it started")
		}
		return err
	}
	if err := start(filepath.Join(t.TempDir(), "no-such-program")); !runnerFailed(err) {
		t.Errorf("won't start: %v", err)
	}
	// A virus scanner can hold a new program long enough to pass the time a game has to
	// load, which is a different and proper answer, and says nothing about this.
	if err := start(exe); !runnerFailed(err) && !strings.Contains(err.Error(), "too long") {
		t.Errorf("dies: %v", err)
	}
	if runnerFailed(fmt.Errorf("the core could not load this game")) {
		t.Error("a runner saying what is wrong is working")
	}
}
