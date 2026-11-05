package server

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
)

// The table that ships: every core in it, every platform, and what was missing when it was made.
func TestTheCoreBuildTableIsRead(t *testing.T) {
	if len(coreBuilds) < 30 {
		t.Fatalf("%d cores", len(coreBuilds))
	}
	for _, c := range []struct {
		core, goos, goarch string
		has                bool
	}{
		{"fceumm", "darwin", "arm64", true},
		{"mednafen_supafaust", "darwin", "arm64", false}, // the 404 someone met installing it on a Mac
		{"mednafen_supafaust", "windows", "amd64", true},
		{"azahar", "linux", "arm64", false},
		{"pcsx2", "darwin", "arm64", true},
	} {
		has, known := coreBuildFor(c.core, c.goos, c.goarch)
		if !known || has != c.has {
			t.Errorf("%s on %s/%s: %v (known %v), want %v", c.core, c.goos, c.goarch, has, known, c.has)
		}
	}
	if has, known := coreBuildFor("fceumm", "freebsd", "amd64"); has || !known {
		t.Error("nothing is published for FreeBSD")
	}
	if _, known := coreBuildFor("a_core_added_since", "linux", "amd64"); known {
		t.Error("a core missing from the table should be tried, not refused")
	}
}

// How a core draws on each kind of server.
func TestHowACoreDraws(t *testing.T) {
	for _, c := range []struct {
		core       string
		gpu        bool
		goos       string
		noGPU      string
		want, note string
	}{
		{"fceumm", false, "darwin", gpuPlatformReason, drawsCPU, ""},
		{"mupen64plus_next", true, "linux", "", drawsGPU, "graphics card"},
		{"mupen64plus_next", true, "linux", "OpenGL here is only llvmpipe", drawsCPUFallback, "M3 Pro"},
		{"mupen64plus_next", true, "darwin", "", drawsCPUFallback, "processor"},
		{"flycast", true, "darwin", "", drawsNone, "graphics card"},
		{"flycast", true, "windows", "", drawsGPU, ""},
		{"pcsx2", true, "freebsd", "", drawsCPUFallback, ""},
	} {
		got, note := drawsOn(c.core, c.gpu, c.goos, c.noGPU)
		if got != c.want || !strings.Contains(note, c.note) {
			t.Errorf("%s on %s (no card: %q): %s %q, want %s and %q", c.core, c.goos, c.noGPU, got, note, c.want, c.note)
		}
	}
	s := coreSupportFor("mednafen_supafaust", false, "")
	if len(s.Platforms) != len(buildPlatforms)+1 || s.Platforms[len(s.Platforms)-1].OS != "freebsd" {
		t.Fatalf("platforms: %+v", s.Platforms)
	}
	for _, p := range s.Platforms {
		if p.OS == "macos" && (p.Build || p.Draws != drawsNone) {
			t.Errorf("macOS: %+v", p)
		}
		if p.OS == "windows" && (!p.Build || p.Draws != drawsCPU) {
			t.Errorf("Windows: %+v", p)
		}
	}
	if has, known := coreBuildFor("mednafen_supafaust", runtime.GOOS, runtime.GOARCH); known && s.Here.Build != has {
		t.Errorf("here: %+v", s.Here)
	}
}

func TestANoBuildRefusalSaysWhatToUse(t *testing.T) {
	m := noBuildRefusal("Beetle Supafaust", &coreAlternative{ID: "bsnes2014_performance", Name: "bsnes 2014 Performance", Console: "Super Nintendo"})
	msg := m["error"].(string)
	if !strings.Contains(msg, "isn't published for") || !strings.Contains(msg, "bsnes 2014 Performance plays Super Nintendo games here instead") || m["noBuild"] != true {
		t.Errorf("%v", m)
	}
}

// Every core the module offers is in the table, so none is offered blind. WittyPlay adding a core
// makes this fail until the table is written again (scripts/corebuilds).
func TestEveryOfferedCoreIsInTheBuildTable(t *testing.T) {
	modtest.Need(t, "wittyplay")
	for _, c := range wittyplay.All() {
		for _, k := range c.Cores {
			if _, ok := coreBuilds[k.ID]; !ok && !wittyplay.IsCustomConsole(c.ID) {
				t.Errorf("%s (%s) isn't in internal/server/corebuilds.txt", k.ID, c.ID)
			}
		}
	}
}

// A core with no build for this server isn't installed: the answer says so and names one that
// plays the same games here. The consoles list says it too, before anyone tries.
func TestACoreWithNoBuildHereIsRefusedWithAnAlternative(t *testing.T) {
	modtest.Need(t, "wittyplay")
	var missing, console string
	for _, c := range wittyplay.All() {
		for _, k := range c.Cores {
			if has, known := coreBuildFor(k.ID, runtime.GOOS, runtime.GOARCH); known && !has && missing == "" {
				missing, console = k.ID, c.ID
			}
		}
	}
	if missing == "" {
		t.Skipf("every core is published for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	g := newGameServer(t)
	w := g.do("POST", "/api/gaming/cores/"+missing+"/install", g.admin, nil)
	var a struct {
		Error       string           `json:"error"`
		NoBuild     bool             `json:"noBuild"`
		Platform    string           `json:"platform"`
		Alternative *coreAlternative `json:"alternative"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	if w.Code != 409 || !a.NoBuild || a.Platform == "" || !strings.Contains(a.Error, "isn't published for") {
		t.Fatalf("%s: %d %s", missing, w.Code, w.Body.String())
	}
	if a.Alternative != nil {
		if has, known := coreBuildFor(a.Alternative.ID, runtime.GOOS, runtime.GOARCH); known && !has {
			t.Errorf("the alternative isn't published here either: %+v", a.Alternative)
		}
	}
	cs := consolesByID(t, g, g.other)[console]
	if s, ok := cs.CoreSupport[missing]; !ok || s.Here.Build || s.Here.Draws != drawsNone || len(s.Platforms) == 0 {
		t.Errorf("the consoles list: %+v", cs.CoreSupport[missing])
	}
}
