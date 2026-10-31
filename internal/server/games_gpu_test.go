package server

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// What someone is told when a game can't start, and what is found for them, through
// the same route the player uses. The core files here are empty and never loaded:
// each of these stops before one is, or fails at the missing runner.

type playAnswer struct {
	Error     string `json:"error"`
	NeedBIOS  bool   `json:"needBios"`
	File      string `json:"file"`
	Encrypted bool   `json:"encrypted"`
}

func addGame(t *testing.T, g *gameServer, console, dir string) *library.Item {
	t.Helper()
	var lib library.Library
	w := g.do("POST", "/api/libraries", g.admin, map[string]any{"name": console, "type": "games", "console": console, "paths": []string{dir}})
	if err := json.Unmarshal(w.Body.Bytes(), &lib); err != nil || lib.ID == 0 {
		t.Fatalf("library: %s", w.Body.String())
	}
	for i := 0; i < 200; i++ {
		var r struct {
			Items []*library.Item `json:"items"`
		}
		w := g.do("GET", "/api/items?kind=game&library="+itoa64(lib.ID), g.admin, nil)
		_ = json.Unmarshal(w.Body.Bytes(), &r)
		if len(r.Items) > 0 {
			if len(r.Items) != 1 {
				t.Fatalf("expected one game, got %d", len(r.Items))
			}
			return r.Items[0]
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the game was never found")
	return nil
}

func placeCores(t *testing.T, g *gameServer, ids ...string) {
	t.Helper()
	dir := g.s.games().cores.Dir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ext := ".so"
	if runtime.GOOS == "windows" {
		ext = ".dll"
	}
	for _, id := range ids {
		if err := os.WriteFile(filepath.Join(dir, id+"_libretro"+ext), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func play(t *testing.T, g *gameServer, id int64) (int, playAnswer) {
	t.Helper()
	w := g.do("POST", "/api/games/"+itoa64(id)+"/play", g.admin, map[string]any{"client": "browser"})
	var a playAnswer
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	return w.Code, a
}

func TestPS2SaysWhichBIOSIsMissingAndFindsOne(t *testing.T) {
	t.Setenv("WMS_WITTYPLAY_RUNNER", filepath.Join(t.TempDir(), "no-such-runner"))
	g := newGameServer(t)
	placeCores(t, g, "pcsx2")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Game.iso"), []byte("game"), 0o644); err != nil {
		t.Fatal(err)
	}
	it := addGame(t, g, "ps2", dir)

	code, a := play(t, g, it.ID)
	if code != 409 || !a.NeedBIOS || a.File == "" {
		t.Fatalf("%d %+v", code, a)
	}
	if !strings.Contains(a.Error, "BIOS") || !strings.Contains(a.Error, "folder named BIOS") {
		t.Errorf("the message should say where it goes: %q", a.Error)
	}

	// Put one where the message says.
	bios := filepath.Join(dir, "BIOS", "SCPH-39001")
	if err := os.MkdirAll(bios, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bios, "SCPH-39001_USA.BIN"), make([]byte, 4<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	code, a = play(t, g, it.ID)
	if a.NeedBIOS {
		t.Fatalf("the BIOS beside the game should have been found: %d %+v", code, a)
	}
	copied := filepath.Join(g.s.games().systemDir(), "pcsx2", "bios", "SCPH-39001_USA.BIN")
	if _, err := os.Stat(copied); err != nil {
		t.Errorf("it should be in the system folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bios, "SCPH-39001_USA.BIN")); err != nil {
		t.Errorf("the original should stay: %v", err)
	}
}

// A generated card image: real headers, and a program that can't be read.
func encryptedCCI() []byte {
	b := make([]byte, 0x6000)
	copy(b[0x100:], "NCSD")
	binary.LittleEndian.PutUint32(b[0x120:], 0x4000/0x200)
	binary.LittleEndian.PutUint32(b[0x124:], 0x2000/0x200)
	copy(b[0x4100:], "NCCH") // no "no encryption" flag
	return b
}

func TestEncrypted3DSIsRefusedPlainly(t *testing.T) {
	t.Setenv("WMS_WITTYPLAY_RUNNER", filepath.Join(t.TempDir(), "no-such-runner"))
	g := newGameServer(t)
	placeCores(t, g, "azahar")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Game.3ds"), encryptedCCI(), 0o644); err != nil {
		t.Fatal(err)
	}
	it := addGame(t, g, "3ds", dir)
	code, a := play(t, g, it.ID)
	if code != 422 || !a.Encrypted {
		t.Fatalf("%d %+v", code, a)
	}
	if !strings.Contains(a.Error, "encrypted") || !strings.Contains(a.Error, "won't decrypt") {
		t.Errorf("message: %q", a.Error)
	}
	// Nothing was started, and nothing is left behind.
	if n := g.s.games().mgr.Count(); n != 0 {
		t.Errorf("%d games running", n)
	}
	if ents, _ := os.ReadDir(filepath.Join(g.s.games().dir, "tmp")); len(ents) != 0 {
		t.Errorf("left behind: %v", ents)
	}
}
