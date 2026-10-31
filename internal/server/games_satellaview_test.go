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
)

// A broadcast game shows a blank picture on any core that lacks what it needs, with
// no error, so pressing Play has to say what's missing.
func TestSatellaviewGameSaysWhatIsMissing(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	runner := filepath.Join(t.TempDir(), "wittyplay-host")
	if err := os.WriteFile(runner, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WMS_WITTYPLAY_RUNNER", runner)

	coreFile := func(id string) string {
		ext := ".so"
		if runtime.GOOS == "windows" {
			ext = ".dll"
		}
		return filepath.Join(g.s.games().dir, "cores", id+"_libretro"+ext)
	}
	if err := os.MkdirAll(filepath.Dir(coreFile("x")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coreFile("mednafen_supafaust"), []byte("core"), 0o644); err != nil {
		t.Fatal(err)
	}

	games := filepath.Join(t.TempDir(), "games")
	if err := os.MkdirAll(games, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(games, "Game.bs"), make([]byte, 1<<16), 0o644); err != nil {
		t.Fatal(err)
	}
	w := g.do("POST", "/api/libraries", g.admin, map[string]any{"name": "SNES", "type": "games", "console": "snes", "paths": []string{games}})
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
	play := func() (int, string) {
		w := g.do("POST", "/api/games/"+itoa64(id)+"/play", g.admin, map[string]any{})
		return w.Code, w.Body.String()
	}

	if code, body := play(); code != 409 || !strings.Contains(body, "BS-X.bin") || !strings.Contains(body, "BIOS") {
		t.Fatalf("no BIOS: %d %s", code, body)
	}
	bios := filepath.Join(games, "BIOS")
	if err := os.MkdirAll(bios, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bios, "bs-x.BIN"), []byte("cartridge"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Found beside the games. Now the only thing missing is the one core that plays it.
	if code, body := play(); code != 409 || !strings.Contains(body, "Mesen-S") {
		t.Fatalf("no Mesen-S: %d %s", code, body)
	}
	if _, err := os.Stat(filepath.Join(g.s.games().systemDir(), "BS-X.bin")); err != nil {
		t.Fatal(err)
	}
}
