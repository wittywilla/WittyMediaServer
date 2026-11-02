package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
)

// Homebrew built with libdragon only runs on one of the Nintendo 64's cores, so with only the
// other one here, pressing Play says so, rather than letting that core end the game.
func TestLibdragonGameSaysWhichCoreItNeeds(t *testing.T) {
	modtest.Need(t, "wittyplay")
	g := newGameServer(t)
	defer g.s.StopGames()
	runner := filepath.Join(t.TempDir(), "wittyplay-game-runner")
	if err := os.WriteFile(runner, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WMS_WITTYPLAY_RUNNER", runner)

	ext := ".so"
	if runtime.GOOS == "windows" {
		ext = ".dll"
	}
	core := filepath.Join(g.s.games().dir, "cores", "mupen64plus_next_libretro"+ext)
	if err := os.MkdirAll(filepath.Dir(core), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(core, []byte("core"), 0o644); err != nil {
		t.Fatal(err)
	}

	games := filepath.Join(t.TempDir(), "games")
	if err := os.MkdirAll(games, 0o755); err != nil {
		t.Fatal(err)
	}
	rom := make([]byte, 1<<20)
	copy(rom, []byte{0x80, 0x37, 0x12, 0x40})
	copy(rom[0x20:], "SOME HOMEBREW")
	copy(rom[0x500:], "Libdragon IPL3")
	if err := os.WriteFile(filepath.Join(games, "Homebrew.z64"), rom, 0o644); err != nil {
		t.Fatal(err)
	}
	w := g.do("POST", "/api/libraries", g.admin, map[string]any{"name": "N64", "type": "games", "console": "n64", "paths": []string{games}})
	var lib library.Library
	if err := json.Unmarshal(w.Body.Bytes(), &lib); err != nil || w.Code != 201 {
		t.Fatalf("library: %d %s", w.Code, w.Body.String())
	}
	var id int64
	for i := 0; i < 200 && id == 0; i++ {
		var r struct {
			Items []*library.Item `json:"items"`
		}
		_ = json.Unmarshal(g.do("GET", "/api/items?kind=game&library="+itoa64(lib.ID), g.admin, nil).Body.Bytes(), &r)
		if len(r.Items) > 0 {
			id = r.Items[0].ID
		}
		time.Sleep(50 * time.Millisecond)
	}
	if id == 0 {
		t.Fatal("the game was never found")
	}

	w = g.do("POST", "/api/games/"+itoa64(id)+"/play", g.admin, map[string]any{})
	if w.Code != 409 || !strings.Contains(w.Body.String(), "ParaLLEl N64") || !strings.Contains(w.Body.String(), `"otherCore":true`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
