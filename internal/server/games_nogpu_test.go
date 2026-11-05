package server

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/gameid/fixture"
	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
	"github.com/wittywilla/wittymediaserver/internal/play/fakecore"
	"github.com/wittywilla/wittymediaserver/internal/play/stream"
)

// A game drawn on a graphics card, on a server that has none it can use, says so wherever
// it's listed and is turned away before a runner starts. And a console whose games keep
// failing is that console's problem, never every other console's.

// What the runner answers on a Mac, and on Linux with only a software OpenGL.
const (
	macNoGPU   = "this server has no usable graphics card for this game: hardware-rendered games run on Windows and Linux servers"
	llvmpipeNo = "this server has no usable graphics card for this game (OpenGL here is only llvmpipe)"
)

func TestGPUMissingSaysWhy(t *testing.T) {
	for _, c := range []struct {
		name string
		p    stream.GPUProbe
		goos string
		want string
	}{
		{"a card", stream.GPUProbe{Name: "Apple M3 Pro"}, "darwin", ""},
		{"the runner said no, on a Mac", stream.GPUProbe{Error: macNoGPU}, "darwin", gpuPlatformReason},
		{"the runner said no, on Linux", stream.GPUProbe{Error: llvmpipeNo}, "linux", "OpenGL here is only llvmpipe"},
		{"no answer yet, on a Mac", stream.GPUProbe{Checking: true}, "darwin", gpuPlatformReason},
		{"couldn't ask, on FreeBSD", stream.GPUProbe{Error: "the runner could not check: exit status 1", Again: true}, "freebsd", gpuPlatformReason},
		{"no answer yet, on Linux", stream.GPUProbe{}, "linux", ""},
		{"couldn't ask, on Windows", stream.GPUProbe{Error: "the runner could not check: exit status 1", Again: true}, "windows", ""},
	} {
		if got := gpuMissingFrom(c.p, c.goos); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	if got := plainGPUReason("this server has no usable graphics card for this game: no OpenGL library (libEGL) is installed"); got != "no OpenGL library (libEGL) is installed" {
		t.Errorf("%q", got)
	}
}

func TestGPUMessageSaysNotHereAndWhy(t *testing.T) {
	m := gpuMessage("Nintendo 64", gpuPlatformReason)
	for _, want := range []string{"Nintendo 64 games can't play on this server", "graphics card", gpuPlatformReason} {
		if !strings.Contains(m, want) {
			t.Errorf("%q doesn't say %q", m, want)
		}
	}
	if m := gpuMessage("PSP", ""); strings.Contains(m, ": .") || !strings.HasSuffix(m, ".") {
		t.Errorf("%q", m)
	}
}

type gameCheckAnswer struct {
	Ready      bool   `json:"ready"`
	Reason     string `json:"reason"`
	Message    string `json:"message"`
	NeedsGPU   bool   `json:"needsGpu"`
	GPUMissing string `json:"gpuMissing"`
	Software   bool   `json:"software"`
	Notice     string `json:"notice"`
	Core       *struct {
		ID string `json:"id"`
	} `json:"core"`
}

func checkGame(t *testing.T, g *gameServer, id int64) gameCheckAnswer {
	t.Helper()
	w := g.do("GET", "/api/games/"+itoa64(id)+"/check", g.admin, nil)
	var a gameCheckAnswer
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &a) != nil {
		t.Fatalf("check: %d %s", w.Code, w.Body.String())
	}
	return a
}

func consolesByID(t *testing.T, g *gameServer, tok string) map[string]consoleView {
	t.Helper()
	w := g.do("GET", "/api/games/consoles", tok, nil)
	var list []consoleView
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("consoles: %d %s", w.Code, w.Body.String())
	}
	out := map[string]consoleView{}
	for _, c := range list {
		out[c.ID] = c
	}
	return out
}

func gameFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The cores are placed, never loaded: anything that got as far as the runner would fail
// with 500 on the runner that isn't there. 3DS and Dreamcast cores have no way to draw
// without a card.
func TestGPUGameSaysNotOnThisServerEverywhere(t *testing.T) {
	modtest.Need(t, "wittyplay")
	t.Setenv("WMS_WITTYPLAY_RUNNER", filepath.Join(t.TempDir(), "no-such-runner"))
	g := newGameServer(t)
	setGPU(g, stream.GPUProbe{Error: macNoGPU})
	placeCores(t, g, "azahar", "fceumm")
	threeDS := addGame(t, g, "3ds", gameFile(t, "Game.3ds", plainCCI()))
	nes := addGame(t, g, "nes", gameFile(t, "Game.nes", append([]byte("NES\x1a\x01\x01"), make([]byte, 16384+8192)...)))

	// The game's page.
	a := checkGame(t, g, threeDS.ID)
	if a.Ready || a.Reason != "gpu" || !a.NeedsGPU || a.GPUMissing != gpuPlatformReason || a.Core != nil {
		t.Fatalf("check: %+v", a)
	}
	if !strings.Contains(a.Message, "can't play on this server") || !strings.Contains(a.Message, gpuPlatformReason) {
		t.Errorf("not here, and why: %q", a.Message)
	}
	if a := checkGame(t, g, nes.ID); !a.Ready || a.NeedsGPU || a.Software {
		t.Errorf("an older console still plays: %+v", a)
	}

	// The console list, for anyone: with a core installed, with none, and older consoles.
	for _, tok := range []string{g.admin, g.other} {
		cs := consolesByID(t, g, tok)
		if c := cs["3ds"]; c.Ready || c.Reason != "gpu" || c.Message != gpuMessage("Nintendo 3DS", gpuPlatformReason) || c.GPUMissing != gpuPlatformReason {
			t.Errorf("3ds in the list: %+v", c)
		}
		if c := cs["dc"]; c.Ready || c.Reason != "gpu" {
			t.Errorf("a console with no core yet, which no core could play here: %+v", c)
		}
		if c := cs["psp"]; c.Ready || c.Reason != "core" {
			t.Errorf("a console with no core yet, whose core could play here: %+v", c)
		}
		if c := cs["nes"]; !c.Ready || c.Reason != "" || c.Software {
			t.Errorf("nes: %+v", c)
		}
		if c := cs["snes"]; c.Ready || c.Reason != "core" || c.Message == "" {
			t.Errorf("a console that only needs a core says so: %+v", c)
		}
	}

	// Play: refused up front, in the same words, and nothing started.
	w := g.do("POST", "/api/games/"+itoa64(threeDS.ID)+"/play", g.admin, map[string]any{"client": "native"})
	var p struct {
		Error      string `json:"error"`
		Reason     string `json:"reason"`
		NeedsGPU   bool   `json:"needsGpu"`
		GPUMissing string `json:"gpuMissing"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if w.Code != 409 || p.Reason != "gpu" || !p.NeedsGPU || p.Error != a.Message || p.GPUMissing != gpuPlatformReason {
		t.Fatalf("play: %d %s", w.Code, w.Body.String())
	}
	if n := g.s.games().mgr.Count(); n != 0 {
		t.Errorf("%d games running", n)
	}
	// Getting it ready says the same, rather than "ready".
	if w := g.do("POST", "/api/games/"+itoa64(threeDS.ID)+"/prepare", g.admin, nil); w.Code != 409 || !strings.Contains(w.Body.String(), `"reason":"gpu"`) {
		t.Errorf("prepare: %d %s", w.Code, w.Body.String())
	}

	// The same server with a card it can draw on.
	canDraw(g)
	if a := checkGame(t, g, threeDS.ID); !a.Ready || !a.NeedsGPU || a.Core == nil || a.Core.ID != "azahar" {
		t.Errorf("with a card: %+v", a)
	}
	if c := consolesByID(t, g, g.admin)["3ds"]; !c.Ready || c.Reason != "" {
		t.Errorf("with a card: %+v", c)
	}
}

// A Nintendo 64 core that draws on a card has the processor draw instead, on a server without
// one: the game is ready, says it may run slower, and reaches the runner with the options that
// draw on the processor, over anyone's own, and without a graphics driver's room.
func TestTheProcessorDrawsWhereACoreCan(t *testing.T) {
	modtest.Need(t, "wittyplay")
	runner := filepath.Join(t.TempDir(), "recordrunner")
	if out, err := exec.Command("go", "build", "-o", runner, "./testdata/recordrunner").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	t.Setenv("WMS_WITTYPLAY_RUNNER", runner)
	g := newGameServer(t)
	setGPU(g, stream.GPUProbe{Error: macNoGPU})
	placeCores(t, g, "mupen64plus_next")
	n64 := addGame(t, g, "n64", gameFile(t, "Game.z64", append([]byte{0x80, 0x37, 0x12, 0x40}, make([]byte, 4092)...)))
	// Someone chose the card's renderer for this game.
	if w := g.do("PUT", "/api/games/settings", g.admin, map[string]any{"level": wittyplay.LevelGame, "item": n64.ID, "coreOptions": map[string]string{"mupen64plus-rdp-plugin": "gliden64"}}); w.Code >= 300 {
		t.Fatalf("a game's own core option: %d %s", w.Code, w.Body.String())
	}

	a := checkGame(t, g, n64.ID)
	if !a.Ready || !a.Software || a.NeedsGPU || a.Notice != softwareNotice || a.Core == nil || a.Core.ID != "mupen64plus_next" {
		t.Fatalf("check: %+v", a)
	}
	if c := consolesByID(t, g, g.other)["n64"]; !c.Ready || !c.Software || c.Notice == "" || c.Reason != "" {
		t.Errorf("n64 in the list: %+v", c)
	}

	if w := g.do("POST", "/api/games/"+itoa64(n64.ID)+"/play", g.admin, map[string]any{"client": "native"}); w.Code == 409 || w.Code == 201 {
		t.Fatalf("play: %d %s", w.Code, w.Body.String())
	}
	var got struct {
		Args []string `json:"args"`
		Load struct {
			Options map[string]string `json:"options"`
		} `json:"load"`
	}
	found := false
	_ = filepath.WalkDir(g.s.games().dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Name() == "runner.json" {
			b, _ := os.ReadFile(p)
			found = json.Unmarshal(b, &got) == nil
		}
		return nil
	})
	if !found {
		t.Fatal("the runner was never started")
	}
	for k, v := range cpuDrawing["mupen64plus_next"] {
		if got.Load.Options[k] != v {
			t.Errorf("%s = %q, want %q: %v", k, got.Load.Options[k], v, got.Load.Options)
		}
	}
	if slices.Contains(got.Args, "-gpu") {
		t.Errorf("given a graphics driver's room: %v", got.Args)
	}

	// With a card, it draws on the card as it always did, where the card draws it right.
	t.Cleanup(func() { hostOS = runtime.GOOS })
	hostOS = "linux"
	canDraw(g)
	if a := checkGame(t, g, n64.ID); !a.Ready || a.Software || !a.NeedsGPU {
		t.Errorf("with a card: %+v", a)
	}
}

// A console with a core of each kind plays on the one that needs no card, even where
// someone chose the other.
func TestSoftwareCoreStandsInForAGPUCore(t *testing.T) {
	modtest.Need(t, "wittyplay")
	t.Setenv("WMS_WITTYPLAY_RUNNER", filepath.Join(t.TempDir(), "no-such-runner"))
	g := newGameServer(t)
	setGPU(g, stream.GPUProbe{Error: llvmpipeNo})
	placeCores(t, g, "swanstation", "pcsx_rearmed")
	if err := g.s.DB.SetSetting(context.Background(), "gamingCores", `{"ps1":"swanstation"}`); err != nil {
		t.Fatal(err)
	}
	disc := gameFile(t, "Game.cue", []byte("FILE \"Game.bin\" BINARY\n  TRACK 01 MODE2/2352\n    INDEX 01 00:00:00\n"))
	if err := os.WriteFile(filepath.Join(disc, "Game.bin"), make([]byte, 2352*32), 0o644); err != nil {
		t.Fatal(err)
	}
	it := addGame(t, g, "ps1", disc)
	if a := checkGame(t, g, it.ID); !a.Ready || a.NeedsGPU || a.Core == nil || a.Core.ID != "pcsx_rearmed" {
		t.Errorf("without a card: %+v", a)
	}
	if c := consolesByID(t, g, g.admin)["ps1"]; !c.Ready {
		t.Errorf("ps1: %+v", c)
	}
	canDraw(g)
	if a := checkGame(t, g, it.ID); !a.Ready || !a.NeedsGPU || a.Core == nil || a.Core.ID != "swanstation" {
		t.Errorf("with a card: %+v", a)
	}
}

// One console's core crashing, again and again, is recorded against that core. WittyPlay
// stays in use, and every other console still plays. This used to take WittyPlay's version
// out of use for everyone after the third crash in a row.
func TestOneConsoleFailingLeavesTheModuleAndOtherConsoles(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("needs ffmpeg, to start a game that works")
	}
	m := modtest.Need(t, "wittyplay")
	g := newGameServer(t)
	g.s.Modules = m // games run on the module's own runner, as they do on a real server
	g.s.Tools.FFmpeg = ffmpegPath
	before := m.VersionInUse(wittyPlayID)
	if _, _, ver := g.s.gameRunner(context.Background()); ver == "" || ver != before {
		t.Fatalf("not WittyPlay's own runner: %q, %q", ver, before)
	}
	dir := g.s.games().cores.Dir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := fakecore.Build(dir, "fceumm_libretro", fakecore.CrashAtInit); err != nil {
		t.Skip(err)
	}
	nes := addGame(t, g, "nes", gameFile(t, "Game.nes", append([]byte("NES\x1a\x01\x01"), make([]byte, 16384+8192)...)))
	for i := 0; i < 4; i++ {
		w := g.do("POST", "/api/games/"+itoa64(nes.ID)+"/play", g.admin, map[string]any{"client": "native"})
		if w.Code != 500 {
			t.Fatalf("try %d: the crashing core started? %d %s", i+1, w.Code, w.Body.String())
		}
	}
	if got := m.VersionInUse(wittyPlayID); got != before {
		t.Fatalf("WittyPlay %q went out of use after one console failed: now %q", before, got)
	}
	for _, st := range m.Report().Modules {
		if st.ID == wittyPlayID && (st.State != "installed" || st.Notice != "") {
			t.Errorf("WittyPlay: %s, %q", st.State, st.Notice)
		}
	}
	// Recorded against the core that failed, with why.
	if p := g.s.corePlayTotals(context.Background())["fceumm"]; p.Failures != 4 {
		t.Errorf("fceumm's record: %+v", p)
	}
	var why string
	if err := g.s.DB.QueryRow(`SELECT last_error FROM game_core_plays WHERE core_id = 'fceumm' AND item_id = ?`, nes.ID).Scan(&why); err != nil || why == "" {
		t.Errorf("why it failed: %q %v", why, err)
	}
	// Another console plays.
	other := addGame(t, g, "wittytest", gameFile(t, "Game.wittytest", []byte("game")))
	w := g.do("POST", "/api/games/"+itoa64(other.ID)+"/play", g.admin, map[string]any{"client": "native"})
	if w.Code != 201 {
		t.Fatalf("another console: %d %s", w.Code, w.Body.String())
	}
	var info struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &info)
	g.do("DELETE", "/api/play/"+info.ID, g.admin, nil)
}

// PlayStation 2 on a server without a card: LRPS2 can have the processor draw, so a game says
// it needs a BIOS until there's one, and then that it plays, slowly. Play! can't, so it's never
// what a game falls back to here.
func TestPS2DrawsOnTheProcessorWithABIOS(t *testing.T) {
	modtest.Need(t, "wittyplay")
	t.Setenv("WMS_WITTYPLAY_RUNNER", filepath.Join(t.TempDir(), "no-such-runner"))
	g := newGameServer(t)
	setGPU(g, stream.GPUProbe{Error: macNoGPU})
	placeCores(t, g, "pcsx2", "play")
	it := addGame(t, g, "ps2", gameFile(t, "Game.iso", fixture.PS2("SLUS_123.45")))

	a := checkGame(t, g, it.ID)
	if a.Ready || a.Reason != "bios" {
		t.Fatalf("without a BIOS: %+v", a)
	}
	bios := filepath.Join(g.s.games().systemDir(), "pcsx2", "bios", "SCPH-39001_USA.BIN")
	if err := os.MkdirAll(filepath.Dir(bios), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bios, make([]byte, 4<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	if a := checkGame(t, g, it.ID); !a.Ready || !a.Software || a.Core == nil || a.Core.ID != "pcsx2" {
		t.Errorf("with a BIOS: %+v", a)
	}
	if c := consolesByID(t, g, g.admin)["ps2"]; !c.Ready || !c.Software {
		t.Errorf("ps2 in the list: %+v", c)
	}
}

// Every core said to draw wrongly on a system's graphics card can be drawn by the processor there:
// otherwise the rule would take its console away rather than move it.
func TestEveryCardFaultHasAProcessorPath(t *testing.T) {
	for goos, faults := range cardFaults {
		for id, why := range faults {
			if cpuDrawing[id] == nil {
				t.Errorf("%s on %s: listed (%s) with no way for the processor to draw it", id, goos, why)
			}
		}
	}
}

// What a server says about how a core draws here follows its own card: on a Mac whose runner can
// use the card, the cores that draw right there use it and the rest are drawn by the processor.
func TestHowACoreDrawsOnAMacWithACard(t *testing.T) {
	t.Cleanup(func() { hostOS = runtime.GOOS })
	hostOS = "darwin"
	for _, c := range []struct {
		id, noGPU, want string
	}{
		{"ppsspp", "", drawsGPU},
		{"play", "", drawsGPU},
		{"swanstation", "", drawsGPU},
		{"mupen64plus_next", "", drawsCPUFallback},
		{"parallel_n64", "", drawsCPUFallback},
		{"pcsx2", "", drawsCPUFallback},
		// No card it can use: as before.
		{"ppsspp", macNoGPU, drawsCPUFallback},
		{"play", macNoGPU, drawsNone},
		{"mupen64plus_next", macNoGPU, drawsCPUFallback},
	} {
		got, note := drawsHere(c.id, true, c.noGPU)
		if got != c.want {
			t.Errorf("%s (no card: %q): %s, want %s: %s", c.id, c.noGPU, got, c.want, note)
		}
		if c.noGPU == "" && got == drawsCPUFallback && !strings.Contains(note, "Mac") {
			t.Errorf("%s: the note doesn't say why: %s", c.id, note)
		}
	}
	// Elsewhere, a card draws them all.
	hostOS = "linux"
	if got, _ := drawsHere("mupen64plus_next", true, ""); got != drawsGPU {
		t.Errorf("on Linux with a card: %s", got)
	}
}

// On a Mac whose runner can use the graphics card, a Nintendo 64 game is still drawn by the
// processor, as GLideN64 draws black pictures there, and says why; a PSP game uses the card.
func TestAMacCardDrawsOnlyWhatItDrawsRight(t *testing.T) {
	modtest.Need(t, "wittyplay")
	t.Cleanup(func() { hostOS = runtime.GOOS })
	hostOS = "darwin"
	runner := filepath.Join(t.TempDir(), "recordrunner")
	if out, err := exec.Command("go", "build", "-o", runner, "./testdata/recordrunner").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	t.Setenv("WMS_WITTYPLAY_RUNNER", runner)
	g := newGameServer(t)
	setGPU(g, stream.GPUProbe{Name: "Apple M3 Pro"})
	placeCores(t, g, "mupen64plus_next", "ppsspp")
	n64 := addGame(t, g, "n64", gameFile(t, "Game.z64", append([]byte{0x80, 0x37, 0x12, 0x40}, make([]byte, 4092)...)))

	a := checkGame(t, g, n64.ID)
	if !a.Ready || !a.Software || a.NeedsGPU || a.Notice != cardFaultNotice() || !strings.Contains(a.Notice, "On a Mac") {
		t.Fatalf("check: %+v", a)
	}
	if c := consolesByID(t, g, g.other)["n64"]; !c.Ready || !c.Software || c.Notice != cardFaultNotice() {
		t.Errorf("n64 in the list: %+v", c)
	}
	if w := g.do("POST", "/api/games/"+itoa64(n64.ID)+"/play", g.admin, map[string]any{"client": "native"}); w.Code == 409 || w.Code == 201 {
		t.Fatalf("play: %d %s", w.Code, w.Body.String())
	}
	var got struct {
		Args []string `json:"args"`
		Load struct {
			Options map[string]string `json:"options"`
		} `json:"load"`
	}
	found := false
	_ = filepath.WalkDir(g.s.games().dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Name() == "runner.json" {
			b, _ := os.ReadFile(p)
			found = json.Unmarshal(b, &got) == nil
		}
		return nil
	})
	if !found {
		t.Fatal("the runner was never started")
	}
	for k, v := range cpuDrawing["mupen64plus_next"] {
		if got.Load.Options[k] != v {
			t.Errorf("%s = %q, want %q: %v", k, got.Load.Options[k], v, got.Load.Options)
		}
	}
	if slices.Contains(got.Args, "-gpu") {
		t.Errorf("given the card: %v", got.Args)
	}

	psp := addGame(t, g, "psp", gameFile(t, "Game.iso", fixture.PSP("GAME", "ULUS10000")))
	if a := checkGame(t, g, psp.ID); !a.Ready || a.Software || !a.NeedsGPU {
		t.Errorf("psp on the card: %+v", a)
	}
}
