package server

import (
	"context"
	_ "embed"
	"runtime"
	"slices"
	"strings"

	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
)

// Which game cores run on which server platforms, and how each draws there. Every device plays
// every game, since the server runs it and streams it: what decides whether a game plays is the
// server's platform. corebuilds.txt says where a core can be downloaded; whether it draws on a
// graphics card, the processor, or can't draw at all comes from its traits and this server's
// card (games_gpu.go). See docs/GAMING-API.md, "Which platforms a core runs on".

//go:embed corebuilds.txt
var coreBuildsText string

// buildPlatform is one column of corebuilds.txt: how it's named in the API, and Go's names for it.
type buildPlatform struct {
	OS, Arch     string // windows, linux, macos; x64, arm64, arm32
	GOOS, GOARCH string
}

var buildPlatforms = []buildPlatform{
	{"windows", "x64", "windows", "amd64"},
	{"linux", "x64", "linux", "amd64"},
	{"linux", "arm64", "linux", "arm64"},
	{"linux", "arm32", "linux", "arm"},
	{"macos", "x64", "darwin", "amd64"},
	{"macos", "arm64", "darwin", "arm64"},
}

// coreBuilds is corebuilds.txt: for each core, whether each platform has a build, in the order
// of buildPlatforms.
var coreBuilds = parseCoreBuilds(coreBuildsText)

func parseCoreBuilds(text string) map[string][]bool {
	out := map[string][]bool{}
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) != len(buildPlatforms)+1 || strings.HasPrefix(f[0], "#") {
			continue
		}
		has := make([]bool, len(buildPlatforms))
		for i, v := range f[1:] {
			has[i] = v == "y"
		}
		out[f[0]] = has
	}
	return out
}

// platformIndex is this server's column, or -1 where no cores are published (FreeBSD, Windows
// on ARM).
func platformIndex(goos, goarch string) int {
	for i, p := range buildPlatforms {
		if p.GOOS == goos && p.GOARCH == goarch {
			return i
		}
	}
	return -1
}

// platformWords names a platform for people.
func platformWords(goos, goarch string) string {
	switch goos + "/" + goarch {
	case "darwin/arm64":
		return "macOS on Apple silicon"
	case "darwin/amd64":
		return "macOS on Intel"
	case "windows/amd64":
		return "Windows"
	case "windows/arm64":
		return "Windows on ARM"
	case "linux/amd64":
		return "Linux"
	case "linux/arm64":
		return "64-bit ARM Linux"
	case "linux/arm":
		return "32-bit ARM Linux"
	}
	if goos == "freebsd" {
		return "FreeBSD"
	}
	return goos + " (" + goarch + ")"
}

// coreBuildFor says whether a core can be downloaded for a platform, and whether that's known:
// a core missing from the table may be new, and is tried.
func coreBuildFor(id, goos, goarch string) (has, known bool) {
	i := platformIndex(goos, goarch)
	if i < 0 {
		return false, true // nothing is published for this platform at all
	}
	row, ok := coreBuilds[id]
	if !ok {
		return false, false
	}
	return row[i], true
}

// How a core draws on a platform.
const (
	drawsGPU         = "gpu"          // on the server's graphics card
	drawsCPU         = "cpu"          // by the processor, as this core always is
	drawsCPUFallback = "cpu-fallback" // a graphics card's core, drawn by the processor where there's none
	drawsNone        = "none"         // it can't draw there
)

// cpuSpeedNotes are what was measured of a graphics card's core drawn by the processor, for
// the people deciding whether to play it that way (docs/GAMING.md, "Without a graphics card").
var cpuSpeedNotes = map[string]string{
	"mupen64plus_next": "Full speed on an Apple M3 Pro, on about one processor core.",
	"parallel_n64":     "Full speed on an Apple M3 Pro, on about one processor core.",
	"ppsspp":           "Mostly full speed on an Apple M3 Pro, with dips in heavy scenes, on three to five processor cores.",
	"pcsx2":            "Mostly full speed on an Apple M3 Pro, slower in places.",
}

// platformSupport is how a core runs on one server platform.
type platformSupport struct {
	OS    string `json:"os"`             // windows, linux, macos, freebsd
	Arch  string `json:"arch,omitempty"` // x64, arm64, arm32
	Build bool   `json:"build"`          // it can be downloaded for this platform
	Draws string `json:"draws"`          // gpu, cpu, cpu-fallback or none
	Note  string `json:"note,omitempty"` // in words: what that means, or how fast it was
}

// coreSupport is how a core runs everywhere, and on this server.
type coreSupport struct {
	Here      platformSupport   `json:"here"`
	Platforms []platformSupport `json:"platforms"`
}

// drawsOn says how a core draws on a platform with a build, given whether it's a graphics card's
// core and, for this server, why it has no card ("" when it has one, or for another platform).
func drawsOn(id string, gpuCore bool, goos, noGPU string) (string, string) {
	switch {
	case !gpuCore:
		return drawsCPU, ""
	case noGPU == "" && gpuOS(goos):
		return drawsGPU, "Needs a graphics card in the server."
	case cpuDrawing[id] != nil:
		note := cpuSpeedNotes[id]
		if note == "" {
			note = "Slower than on a graphics card."
		}
		return drawsCPUFallback, "Drawn by the processor, as there's no graphics card it can use. " + note
	}
	return drawsNone, "Needs a graphics card, and the server can't give it one here."
}

// coreSupportFor works out where a core runs. gpuCore is whether it draws on a graphics card;
// noGPU is why this server has none it can use, or "".
func coreSupportFor(id string, gpuCore bool, noGPU string) coreSupport {
	var out coreSupport
	for _, p := range buildPlatforms {
		has, known := coreBuildFor(id, p.GOOS, p.GOARCH)
		ps := platformSupport{OS: p.OS, Arch: p.Arch, Build: has || !known}
		if !ps.Build {
			ps.Draws, ps.Note = drawsNone, "Not published for this platform."
		} else {
			ps.Draws, ps.Note = drawsOn(id, gpuCore, p.GOOS, "")
		}
		out.Platforms = append(out.Platforms, ps)
	}
	out.Platforms = append(out.Platforms, platformSupport{OS: "freebsd", Draws: drawsNone,
		Note: "No prebuilt cores for FreeBSD: build one, or install it from ports, and put it in the cores folder."})
	// This server.
	has, known := coreBuildFor(id, runtime.GOOS, runtime.GOARCH)
	here := platformSupport{OS: osName(runtime.GOOS), Arch: archName(runtime.GOARCH), Build: has || !known}
	if here.Build {
		here.Draws, here.Note = drawsOn(id, gpuCore, runtime.GOOS, noGPU)
	} else {
		here.Draws, here.Note = drawsNone, "Not published for "+platformWords(runtime.GOOS, runtime.GOARCH)+"."
	}
	out.Here = here
	return out
}

func osName(goos string) string {
	if goos == "darwin" {
		return "macos"
	}
	return goos
}

func archName(goarch string) string {
	switch goarch {
	case "amd64":
		return "x64"
	case "arm":
		return "arm32"
	}
	return goarch
}

// coreAlternative is a core people can install instead: on a console that offers this one, the
// next core that's published for this server and can draw here.
type coreAlternative struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Console string `json:"console"`
}

func (s *Server) alternativeCore(ctx context.Context, id string) *coreAlternative {
	noGPU := ""
	asked := false
	for _, c := range wittyplay.All() {
		if !slices.ContainsFunc(c.Cores, func(k wittyplay.Core) bool { return k.ID == id }) {
			continue
		}
		for _, other := range c.Cores {
			if other.ID == id {
				continue
			}
			if has, known := coreBuildFor(other.ID, runtime.GOOS, runtime.GOARCH); known && !has {
				continue
			}
			if wittyplay.Traits(other.ID).GPU {
				if !asked {
					noGPU, asked = s.gpuMissing(ctx), true
				}
				if d, _ := drawsOn(other.ID, true, runtime.GOOS, noGPU); d == drawsNone {
					continue
				}
			}
			return &coreAlternative{ID: other.ID, Name: other.Name, Console: c.Name}
		}
	}
	return nil
}

// noBuildRefusal is the answer to installing a core that isn't published for this server.
func noBuildRefusal(name string, alt *coreAlternative) map[string]any {
	msg := name + " isn't published for " + platformWords(runtime.GOOS, runtime.GOARCH) + ", so it can't be installed on this server."
	if alt != nil {
		msg += " " + alt.Name + " plays " + alt.Console + " games here instead."
	}
	out := map[string]any{"error": msg, "noBuild": true, "platform": osName(runtime.GOOS) + "/" + archName(runtime.GOARCH)}
	if alt != nil {
		out["alternative"] = alt
	}
	return out
}

// offeredCoreName is a core's name from the console table, or its id.
func offeredCoreName(id string) string {
	for _, c := range wittyplay.All() {
		for _, k := range c.Cores {
			if k.ID == id {
				return k.Name
			}
		}
	}
	return id
}
