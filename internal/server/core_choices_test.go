package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
	"github.com/wittywilla/wittymediaserver/internal/play/cores"
)

// Every core the server knows where to download has a line saying what it's good at, and no
// line is for a core it doesn't know.
func TestEveryCoreHasANote(t *testing.T) {
	for id := range coreBuilds {
		n, ok := coreNotes[id]
		if !ok {
			t.Errorf("%s has no note in core_notes.go", id)
			continue
		}
		if n.About == "" || !strings.HasSuffix(n.About, ".") {
			t.Errorf("%s: %q should be a sentence", id, n.About)
		}
		switch n.Strength {
		case "", strengthAccuracy, strengthSpeed, strengthBalanced:
		default:
			t.Errorf("%s: strength %q", id, n.Strength)
		}
	}
	for id := range coreNotes {
		if _, ok := coreBuilds[id]; !ok {
			t.Errorf("%s has a note, and no line in corebuilds.txt", id)
		}
	}
}

// The picker's view of a console: every core it can play on, installed or not, with what each is
// good at, where it runs, and whether it can be installed here; and only for administrators.
func TestCoreChoicesForAConsole(t *testing.T) {
	modtest.Need(t, "wittyplay")
	g := newGameServer(t)
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/gaming/consoles/nes/core"}, {"GET", "/api/gaming/cores/fceumm/install"},
	} {
		if w := g.do(c.method, c.path, g.other, nil); w.Code != 403 {
			t.Errorf("%s %s for someone who isn't an admin: %d", c.method, c.path, w.Code)
		}
	}
	if w := g.do("GET", "/api/gaming/consoles/nope/core", g.admin, nil); w.Code != 404 {
		t.Errorf("an unknown console: %d", w.Code)
	}
	if w := g.do("GET", "/api/gaming/cores/..%2Fevil/install", g.admin, nil); w.Code != 404 {
		t.Errorf("a path is not a core name: %d", w.Code)
	}

	// One core installed, as an administrator would have placed it.
	dir := g.s.games().cores.Dir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nestopia_libretro"+cores.LibraryExt()), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := g.do("GET", "/api/gaming/consoles/nes/core", g.admin, nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var v coreChoicesView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Console != "nes" || len(v.Choices) < 3 || v.Choices[0].ID != "fceumm" {
		t.Fatalf("%+v", v)
	}
	byID := map[string]coreChoice{}
	for _, c := range v.Choices {
		byID[c.ID] = c
		if c.About == "" || c.Support == nil || c.License == "" {
			t.Errorf("%s: about %q, support %v, licence %q", c.ID, c.About, c.Support, c.License)
		}
	}
	if c := byID["nestopia"]; !c.Installed || c.CanInstall {
		t.Errorf("an installed core: %+v", c)
	}
	if c := byID["fceumm"]; c.Installed || c.CanInstall != cores.CanDownload() || !c.Recommended || c.Strength == "" {
		t.Errorf("a core to install: %+v", c)
	}
	if v.Effective != "nestopia" || v.Choice != "" {
		t.Errorf("with only Nestopia installed it plays, and nothing was chosen: %q %q", v.Effective, v.Choice)
	}
	// Chosen as the console's default, it says so.
	if w := g.do("PUT", "/api/gaming/consoles/nes/core", g.admin, map[string]any{"core": "nestopia"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(g.do("GET", "/api/gaming/consoles/nes/core", g.admin, nil).Body.Bytes(), &v)
	if v.Choice != "nestopia" {
		t.Errorf("choice %q", v.Choice)
	}

	// Nothing is being installed.
	var p struct {
		Installing      bool
		Received, Total int64
	}
	if err := json.Unmarshal(g.do("GET", "/api/gaming/cores/fceumm/install", g.admin, nil).Body.Bytes(), &p); err != nil || p.Installing {
		t.Errorf("%+v %v", p, err)
	}

	// The consoles every signed-in account sees carry the same lines.
	var consoles []struct {
		ID        string              `json:"id"`
		CoreAbout map[string]coreNote `json:"coreAbout"`
	}
	_ = json.Unmarshal(g.do("GET", "/api/games/consoles", g.other, nil).Body.Bytes(), &consoles)
	found := false
	for _, c := range consoles {
		if c.ID == "nes" {
			found = c.CoreAbout["fceumm"].About == coreNotes["fceumm"].About
		}
	}
	if !found {
		t.Error("GET /games/consoles has no coreAbout for the NES's cores")
	}
}

// A core that won't start without a BIOS says so, and says when one is there.
func TestCoreBIOSNeed(t *testing.T) {
	system := t.TempDir()
	if b := coreBIOS("fceumm", system); b != nil {
		t.Errorf("FCEUmm needs no BIOS: %+v", b)
	}
	b := coreBIOS("mednafen_psx", system)
	if b == nil || b.Found || len(b.Files) == 0 {
		t.Fatalf("Beetle PSX with no BIOS: %+v", b)
	}
	if err := os.WriteFile(filepath.Join(system, "scph5502.bin"), make([]byte, 512<<10), 0o644); err != nil {
		t.Fatal(err)
	}
	if b := coreBIOS("mednafen_psx", system); b == nil || !b.Found {
		t.Fatalf("Beetle PSX with a European BIOS: %+v", b)
	}
}
