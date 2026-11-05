package server

import (
	"context"
	"runtime"
	"slices"
	"strings"
	"time"

	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
	"github.com/wittywilla/wittymediaserver/internal/play/cores"
	"github.com/wittywilla/wittymediaserver/internal/play/stream"
)

// Games drawn on a graphics card, on a server that can't give them one. Every place that
// says a game can be played asks here first, so the game page, the console list and the
// Play button agree, and a game that can't run here is turned away before a runner starts.
// See docs/GAMING.md, "Consoles that draw with a graphics card".

// gpuPlatformReason is why a runner can't draw on a graphics card on this operating
// system: WittyPlay's runner makes an OpenGL context on Windows and Linux only.
const gpuPlatformReason = "hardware-rendered games run on Windows and Linux servers"

// gpuAskAgain is how long a graphics card check that got no answer is left before
// something about to start a game asks again. Settings asks whenever it's opened.
const gpuAskAgain = 5 * time.Minute

// gpuOS says whether the runner can draw on a graphics card on this operating system at all.
func gpuOS(goos string) bool { return goos == "windows" || goos == "linux" }

// gpuMissingFrom says why a game drawn on a graphics card can't run here, or "" when it
// can or nothing says otherwise. A final answer from the runner counts. Until there is one,
// a system the runner can't draw on at all says so, and anywhere else the game may try.
func gpuMissingFrom(p stream.GPUProbe, goos string) string {
	switch {
	case p.Name != "":
		return ""
	case p.Error != "" && !p.Again:
		return plainGPUReason(p.Error)
	case !gpuOS(goos):
		return gpuPlatformReason
	}
	return ""
}

// plainGPUReason takes the runner's "no usable graphics card" off the front of its answer,
// which the sentence it goes in already says, and keeps the reason.
func plainGPUReason(s string) string {
	s = strings.TrimSpace(s)
	for _, lead := range []string{"this server has no usable graphics card for this game", "this server has no usable graphics card", stream.NoGPUText} {
		if rest, ok := strings.CutPrefix(s, lead); ok {
			s = rest
			break
		}
	}
	s = strings.TrimSpace(strings.TrimLeft(s, ":;, "))
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = s[1 : len(s)-1]
	}
	return strings.TrimRight(strings.TrimSpace(s), ".")
}

// gpuMessage is what a person is told: not on this server, and why.
func gpuMessage(console, why string) string {
	who := "These games"
	if console != "" {
		who = console + " games"
	}
	msg := who + " can't play on this server. They're drawn on a graphics card, and this server has none it can use"
	if why != "" {
		msg += ": " + why
	}
	return msg + ". Games for older consoles still play."
}

// gpuAnswer is the graphics card check as it stands: its answer, whether that's final,
// when the last answer came in, and whether a check is running now.
func (g *gameService) gpuAnswer() (p stream.GPUProbe, final bool, at time.Time, checking bool) {
	g.gpuMu.Lock()
	defer g.gpuMu.Unlock()
	return g.gpu, g.gpu.Name != "" || (g.gpu.Error != "" && !g.gpu.Again), g.gpuAt, g.gpuChecking
}

// gpuMissing says why games drawn on a graphics card can't run on this server, or "" when
// they can, or may. The first time anything asks, the check starts in the background, and
// the answer counts from when it comes in, a second or two later.
func (s *Server) gpuMissing(ctx context.Context) string {
	g := s.games()
	p, final, at, checking := g.gpuAnswer()
	if !final && !checking && (at.IsZero() || time.Since(at) > gpuAskAgain) {
		// Finding the runner checks its signature, so it's only done when the check runs.
		runner, _, _ := s.gameRunner(ctx)
		p = s.gpuStatus(runner)
	}
	return gpuMissingFrom(p, runtime.GOOS)
}

// coreDrawsOnGPU says whether a core draws on a graphics card when it runs as it normally does.
// A custom core goes by what it was seen to do when it was added; "maybe" gets to try.
func (s *Server) coreDrawsOnGPU(c cores.Core) bool {
	if c.Source == cores.SourceCustom {
		for _, cc := range s.customCoreList() {
			if cc.ID == c.ID {
				return cc.WantsGPU
			}
		}
		return false
	}
	return wittyplay.Traits(c.ID).GPU
}

// cpuDrawing are cores that draw on a graphics card where there is one, and can have the
// processor draw instead, with these options laid over everything else. A server with no card
// it can use starts them this way. Each was seen drawing games correctly on an Apple M3 Pro with
// no graphics context at all: Nintendo 64 at full speed on about one core (angrylion draws, cxd4
// runs the graphics microcode, as angrylion needs), PSP mostly at full speed on four or five
// (PPSSPP's own software renderer; "none" never asks the runner for a context), and PlayStation 2
// mostly at full speed, slower in places (LRPS2's "Software (SW)" renderer, which hands frames
// over without a context). docs/GAMING.md has the figures. Cores missing here have no such mode:
// Play!, Flycast, Azahar and the faster PlayStation cores.
var cpuDrawing = map[string]map[string]string{
	"mupen64plus_next": {"mupen64plus-rdp-plugin": "angrylion", "mupen64plus-rsp-plugin": "cxd4"},
	"parallel_n64":     {"parallel-n64-gfxplugin": "angrylion", "parallel-n64-rspplugin": "cxd4"},
	"ppsspp":           {"ppsspp_backend": "none"},
	"pcsx2":            {"pcsx2_renderer": "Software (SW)"},
}

// softwareNotice is the speed caveat for a game the processor draws, shown before it starts and
// when it does.
const softwareNotice = "This server has no graphics card games can use, so its processor draws this game instead, at the console's own resolution. It may run slower than on a server with a card, and keeps several processor cores busy while it plays."

// cardFaults are cores whose drawing on a graphics card goes wrong on a system, as seen with real
// games, so that there they're drawn by the processor even where the runner can use a card. On a
// Mac (an Apple M3 Pro, OpenGL 4.1 through a CGL context), with the owner's games:
//   - Mupen64Plus-Next's GLideN64 handed over black pictures for Super Mario 64 and Banjo-Kazooie.
//     Without its framebuffer emulation it drew Super Mario 64 at full speed, but Banjo-Kazooie at
//     38 frames a second, going black after half a minute. The processor draws both at 60.
//   - ParaLLEl N64's OpenGL plugins drew nothing at all.
//   - LRPS2's OpenGL renderer needs GL_ARB_shading_language_420pack, which macOS doesn't have, and
//     made no picture.
//
// PPSSPP, SwanStation, Beetle PSX HW and Play! drew their games correctly at full speed there, on
// the card, and aren't listed. Each listed core has a way for the processor to draw it.
var cardFaults = map[string]map[string]string{
	"darwin": {
		"mupen64plus_next": "GLideN64 draws black pictures with a Mac's OpenGL",
		"parallel_n64":     "its OpenGL plugins draw nothing with a Mac's OpenGL",
		"pcsx2":            "its OpenGL renderer needs an extension a Mac's OpenGL doesn't have",
	},
}

// hostOS is the system this server runs on. Tests change it.
var hostOS = runtime.GOOS

// drawsWrongHere says why a core's drawing on a graphics card goes wrong on this server's
// system, or "" when it doesn't, as far as is known. Never for a custom core.
func drawsWrongHere(c cores.Core) string {
	if c.Source == cores.SourceCustom {
		return ""
	}
	return cardFaults[hostOS][c.ID]
}

// cardFaultNotice is the speed caveat for a game the processor draws because its core draws
// wrongly on this system's graphics card.
func cardFaultNotice() string {
	where := "this server's system"
	if hostOS == "darwin" {
		where = "a Mac"
	}
	return "On " + where + " this console's games are drawn by the server's processor, at the console's own resolution, since its emulator doesn't draw them correctly on the graphics card there. They may run slower than on a card, and keep several processor cores busy while they play."
}

// cpuOptions are the options that have a core drawn by the processor, or nil if it can't be.
// Never for a custom core: what it offers isn't known.
func cpuOptions(c cores.Core) map[string]string {
	if c.Source == cores.SourceCustom {
		return nil
	}
	return cpuDrawing[c.ID]
}

// needsCard says a core can't run at all without a graphics card.
func (s *Server) needsCard(c cores.Core) bool { return s.coreDrawsOnGPU(c) && cpuOptions(c) == nil }

// withoutGPUArg is a runner's arguments for a core the processor draws: none of what a
// graphics driver needs.
func withoutGPUArg(args []string) []string {
	return slices.DeleteFunc(slices.Clone(args), func(a string) bool { return a == "-gpu" })
}

// corePick is the core a game would start on, as pressing Play would choose it.
type corePick struct {
	core     *cores.Core // what it starts on, or nil
	software bool        // it draws on a graphics card, which this server hasn't got or draws it wrongly, so the processor draws it
	notice   string      // with software: the speed caveat, and why
	blocked  *cores.Core // with no core: the one wanted, which is missing a BIOS
	noGPU    string      // with no core: why what could play it can't draw on this server
}

// pickCore chooses a game's core. On a server without a graphics card it can use, a core that
// draws on one starts with the processor drawing, if it can; if not, it gives way to another
// installed core that needs no card, and then to one that can do without. When none can play it
// here, noGPU says why, ahead of a missing BIOS or core: finding either would change nothing.
func (s *Server) pickCore(ctx context.Context, console *wittyplay.Console, itemID int64, systemDir string) corePick {
	g := s.games()
	// Goes through preferredCore: it's what keeps a file nobody approved from being used.
	preferred := s.preferredCore(ctx, console, itemID)
	core, blocked := g.cores.Playable(console, preferred, systemDir)
	if core != nil && !s.coreDrawsOnGPU(*core) {
		return corePick{core: core}
	}
	if core == nil && blocked == nil && !s.allOfferedNeedCard(console) {
		return corePick{} // nothing installed, and a core that would play here could be
	}
	if core == nil && blocked != nil && !s.needsCard(*blocked) {
		return corePick{blocked: blocked}
	}
	why := s.gpuMissing(ctx)
	if why == "" {
		if core != nil && drawsWrongHere(*core) != "" && cpuOptions(*core) != nil {
			return corePick{core: core, software: true, notice: cardFaultNotice()}
		}
		return corePick{core: core, blocked: blocked}
	}
	if core != nil && cpuOptions(*core) != nil {
		return corePick{core: core, software: true, notice: softwareNotice}
	}
	var needsBIOS, onCPU *cores.Core
	for _, c := range g.cores.AllForConsole(console, preferred) {
		// A custom core only ever runs where an administrator chose it.
		if c.Source == cores.SourceCustom {
			continue
		}
		bios := wittyplay.MissingBIOS(c.ID, systemDir) == ""
		switch {
		case s.coreDrawsOnGPU(c):
			switch {
			case cpuOptions(c) == nil:
			case bios && onCPU == nil:
				onCPU = &c
			case !bios && needsBIOS == nil:
				needsBIOS = &c // it would play here, drawn by the processor, with a BIOS
			}
		case bios:
			return corePick{core: &c}
		case needsBIOS == nil:
			needsBIOS = &c
		}
	}
	switch {
	case onCPU != nil:
		return corePick{core: onCPU, software: true, notice: softwareNotice}
	case needsBIOS != nil:
		return corePick{blocked: needsBIOS} // it would play here, with a BIOS
	}
	return corePick{noGPU: why}
}

// allOfferedNeedCard says no core offered for a console could run without a graphics card.
func (s *Server) allOfferedNeedCard(console *wittyplay.Console) bool {
	if len(console.Cores) == 0 {
		return false
	}
	for _, c := range console.Cores {
		if !wittyplay.Traits(c.ID).GPU || cpuDrawing[c.ID] != nil {
			return false
		}
	}
	return true
}

// gpuRefusal is the answer to a request for a game that can't draw here.
func gpuRefusal(console *wittyplay.Console, why string) map[string]any {
	return map[string]any{"error": gpuMessage(console.Name, why), "reason": "gpu", "needsGpu": true, "gpuMissing": why, "console": console.ID}
}
