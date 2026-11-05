package server

import (
	"encoding/binary"
	"encoding/json"
	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/gameid/fixture"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/play/fakecore"
	"github.com/wittywilla/wittymediaserver/internal/play/stream"
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
	ext := fakecore.Ext() // .dylib on a Mac, where a .so is never listed
	for _, id := range ids {
		if err := os.WriteFile(filepath.Join(dir, id+"_libretro"+ext), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// canDraw gives a test server a graphics card games can draw on, whatever this machine has:
// for what happens on a server that can run these consoles.
func canDraw(g *gameServer) { setGPU(g, stream.GPUProbe{Name: "a test card"}) }

func setGPU(g *gameServer, p stream.GPUProbe) {
	gs := g.s.games()
	gs.gpuMu.Lock()
	gs.gpu, gs.gpuAt = p, time.Now()
	gs.gpuMu.Unlock()
}

func play(t *testing.T, g *gameServer, id int64) (int, playAnswer) {
	t.Helper()
	w := g.do("POST", "/api/games/"+itoa64(id)+"/play", g.admin, map[string]any{"client": "browser"})
	var a playAnswer
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	return w.Code, a
}

func TestPS2SaysWhichBIOSIsMissingAndFindsOne(t *testing.T) {
	modtest.Need(t, "wittyplay")
	t.Setenv("WMS_WITTYPLAY_RUNNER", filepath.Join(t.TempDir(), "no-such-runner"))
	g := newGameServer(t)
	canDraw(g)
	placeCores(t, g, "pcsx2")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Game.iso"), fixture.PS2("SLUS_123.45"), 0o644); err != nil {
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

// A package that really is unencrypted starts, whatever its table of contents still says
// and whether or not it kept its ticket, and the program copied out of it stays for next time.
// One whose program is scrambled is refused, and no copy is made. The runner here is missing,
// so these stop just after the point being tested.
func TestPlain3DSPackageGetsPastTheRefusal(t *testing.T) {
	modtest.Need(t, "wittyplay")
	t.Setenv("WMS_WITTYPLAY_RUNNER", filepath.Join(t.TempDir(), "no-such-runner"))
	g := newGameServer(t)
	canDraw(g)
	placeCores(t, g, "azahar")
	for _, c := range []struct {
		name      string
		data      []byte
		encrypted bool
	}{
		{"plain and marked plain", fixture.CIA(fixture.NCCH(true, false, 0x4000), 0, 0x00040000), false},
		{"plain and still marked encrypted", fixture.CIA(fixture.NCCH(true, false, 0x4000), 1, 0x00040000), false},
		{"scrambled by the package", fixture.CIA(fixture.Scrambled(0x4000), 1, 0x00040000), true},
		{"scrambled by the program", fixture.CIA(fixture.NCCH(false, false, 0x4000), 0, 0x00040000), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "Game.cia"), c.data, 0o644); err != nil {
				t.Fatal(err)
			}
			kept := filepath.Join(g.s.games().dir, "tmp", "3ds")
			_ = os.RemoveAll(kept)
			it := addGame(t, g, "3ds", dir)
			code, a := play(t, g, it.ID)
			if a.Encrypted != c.encrypted || (code == 422) != c.encrypted {
				t.Fatalf("%d %+v", code, a)
			}
			ents, _ := os.ReadDir(kept)
			if c.encrypted {
				if len(ents) != 0 {
					t.Errorf("copied something out of an encrypted file: %v", ents)
				}
				return
			}
			if len(ents) != 1 {
				t.Fatalf("kept %v", ents)
			}
			before, err := ents[0].Info()
			if err != nil {
				t.Fatal(err)
			}
			if code, _ = play(t, g, it.ID); code == 422 {
				t.Fatal("refused the second time")
			}
			ents, _ = os.ReadDir(kept)
			after, err := ents[0].Info()
			if err != nil || len(ents) != 1 || !after.ModTime().Equal(before.ModTime()) {
				t.Errorf("it should have used the copy it had: %v %v", ents, err)
			}
		})
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

// plainCCI is a 3DS cartridge dump whose program says it isn't encrypted: a copy someone
// decrypted themselves.
func plainCCI() []byte {
	b := encryptedCCI()
	copy(b[0x4000:], fixture.NCCH(true, false, 0x2000))
	return b
}

func TestEncrypted3DSIsRefusedPlainly(t *testing.T) {
	modtest.Need(t, "wittyplay")
	t.Setenv("WMS_WITTYPLAY_RUNNER", filepath.Join(t.TempDir(), "no-such-runner"))
	g := newGameServer(t)
	canDraw(g)
	placeCores(t, g, "azahar")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Game.3ds"), encryptedCCI(), 0o644); err != nil {
		t.Fatal(err)
	}
	it := addGame(t, g, "3ds", dir)
	// The game's page says so before Play is pressed, in Play's words, with a card or without:
	// no server can play this copy, so that comes before what this one lacks.
	check := func() map[string]any {
		var c map[string]any
		w := g.do("GET", "/api/games/"+itoa64(it.ID)+"/check", g.admin, nil)
		if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		return c
	}
	code, a := play(t, g, it.ID)
	if code != 422 || !a.Encrypted {
		t.Fatalf("%d %+v", code, a)
	}
	for _, p := range []stream.GPUProbe{{Name: "a test card"}, {Error: "no card here"}} {
		setGPU(g, p)
		if c := check(); c["ready"] != false || c["reason"] != "encrypted" || c["encrypted"] != true || c["message"] != a.Error {
			t.Errorf("check with %+v: %v", p, c)
		}
	}
	if !strings.Contains(a.Error, "encrypted") || !strings.Contains(a.Error, "won't decrypt") || !strings.Contains(a.Error, "decrypted yourself") {
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
