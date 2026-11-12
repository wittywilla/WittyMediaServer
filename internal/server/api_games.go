// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/appcontrol"
	"github.com/wittywilla/wittymediaserver/internal/gamearchive"
	"github.com/wittywilla/wittymediaserver/internal/gameregion"
	"github.com/wittywilla/wittymediaserver/internal/layout"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
	"github.com/wittywilla/wittymediaserver/internal/play/cores"
	"github.com/wittywilla/wittymediaserver/internal/play/ipc"
	"github.com/wittywilla/wittymediaserver/internal/play/stream"
	"github.com/wittywilla/wittymodule/service"
)

// Games: the API around WittyPlay. Sessions and streaming live in the module;
// this file checks who may do what, and turns an item into a game to run. See
// docs/GAMING.md.

func init() {
	settingKeys = append(settingKeys, "gamingMaxSessions", "gamingUdpPort", "gamingPublicIp", "gamingStun", "gamingRunner", "gamingCores", "gamingQuality", "gamingUnpackGB", "gamingMaxGpuSessions", "gamingResolution", "gamingMaxSpeed", "gamingRewindMB", "gamingRunAhead", "gamingRegionOrder")
	settingChecks["gamingRegionOrder"] = gameregion.Valid
	settingDefaults["gamingMaxSessions"] = "2"
	settingDefaults["gamingMaxGpuSessions"] = "2"
	settingDefaults["gamingRunner"] = "auto"
	settingDefaults["gamingUnpackGB"] = strconv.Itoa(defaultUnpackGB)
}

// gameRoutes registers the games API.
func (s *Server) gameRoutes(m routes) {
	a := s.requireUser
	gameregion.Set(s.DB.GetSetting(context.Background(), "gamingRegionOrder", ""))
	m.Handle("GET /api/games/consoles", a(s.handleGameConsoles))
	m.Handle("GET /api/games/modes", a(s.handleGameModes))
	m.Handle("GET /api/consoles/{console}/controls", a(s.handleGameControls))
	m.Handle("PUT /api/consoles/{console}/controls", a(s.handlePutGameControl))
	m.Handle("POST /api/games/{id}/prepare", a(s.handleGamePrepare))
	m.Handle("POST /api/games/{id}/play", a(s.handleGamePlay))
	m.Handle("GET /api/games/{id}/saves", a(s.handleGameSaves))
	m.Handle("GET /api/games/{id}/saves/{slot}/thumb", a(s.handleGameSaveThumb))
	m.Handle("DELETE /api/games/{id}/saves/{slot}", a(s.handleGameSaveDelete))
	m.Handle("GET /api/play/{sid}", a(s.handlePlayGet))
	m.Handle("DELETE /api/play/{sid}", a(s.handlePlayStop))
	m.Handle("POST /api/play/{sid}/webrtc", a(s.handlePlayWebRTC))
	m.Handle("GET /api/play/{sid}/ws", a(s.handlePlayWS))
	m.Handle("POST /api/play/{sid}/stream", a(s.handlePlayStream))
	m.Handle("GET /api/play/{sid}/stream", a(s.handlePlayStreamWS))
	m.Handle("POST /api/play/{sid}/action", a(s.handlePlayAction))
	s.seatRoutes(m)
	s.gameProfileRoutes(m)
	m.Handle("GET /api/gaming", s.requireAdmin(s.handleGaming))
	m.Handle("POST /api/gaming/cores/{id}/install", s.requireAdmin(s.handleCoreInstall))
	m.Handle("GET /api/gaming/cores/{id}/install", s.requireAdmin(s.handleCoreInstallProgress))
	m.Handle("DELETE /api/gaming/cores/{id}", s.requireAdmin(s.handleCoreRemove))
	m.Handle("DELETE /api/gaming/unpacked", s.requireAdmin(s.handleUnpackedClear))
	s.gameOptionRoutes(m)
	s.gameReviewRoutes(m)
	s.customRoutes(m)
}

// gameService is built the first time anything needs it, so a server nobody
// plays games on spends nothing on them.
type gameService struct {
	probeSlot   chan struct{} // asking a core what it offers, one at a time
	silentCores sync.Map      // cores that said nothing when asked, by ID, and when
	mgr         *stream.Manager
	cores       *cores.Manager
	archives    *gamearchive.Cache
	dir         string
	standalone  standaloneOffers // which consoles WittyPlay's emulators play here: games_standalone.go

	gpuMu       sync.Mutex
	gpu         stream.GPUProbe
	gpuAt       time.Time // when the last answer came in
	gpuChecking bool

	bios biosFinder // BIOS files found by their checksums

	custom *customService

	peers peerLimits

	// seats: who started each game, on what, and who joined it with a code (game_seats.go).
	seats seatBook

	programs sync.Mutex // the program copied out of a 3DS package, while it's looked at or made
}

// gpuStatus is what's known about the graphics card. Finding out takes a second or
// two, or half a minute if a security scanner holds the runner's first start, so it
// happens in the background and nothing waits for it. An answer about the card is
// final; a failure to ask gets asked again next time.
func (s *Server) gpuStatus(runner string) stream.GPUProbe {
	g := s.games()
	g.gpuMu.Lock()
	defer g.gpuMu.Unlock()
	if runner == "" || g.gpuChecking || (g.gpu.Name != "" || (g.gpu.Error != "" && !g.gpu.Again)) {
		out := g.gpu
		out.Checking = g.gpuChecking
		return out
	}
	started := s.background("graphics card check", func(ctx context.Context) {
		// What a fault leaves: nothing learnt about the card, so it's asked again next time.
		got := stream.GPUProbe{Error: "the check stopped on a fault in the server", Again: true}
		defer func() {
			g.gpuMu.Lock()
			defer g.gpuMu.Unlock()
			g.gpu, g.gpuAt, g.gpuChecking = got, time.Now(), false
		}()
		got = stream.ProbeGPU(ctx, runner, filepath.Join(g.dir, "probe"))
		if ctx.Err() != nil {
			got = stream.GPUProbe{Error: "the server stopped before the check finished", Again: true}
		}
	}, "cat", "games")
	if !started { // the server is stopping
		return g.gpu
	}
	g.gpuChecking = true
	return stream.GPUProbe{Checking: true}
}

// defaultUnpackGB is the space for unpacked games: room for a good many discs, or the
// largest single disc. An admin can change it, and free disk space always comes first.
const defaultUnpackGB = 16

// systemDir is the folder cores read BIOS files and their support files from.
func (g *gameService) systemDir() string { return filepath.Join(g.dir, "system") }

// testingGames: the built-in test console is only offered to developers.
func (s *Server) testingGames() bool {
	return (s.Cfg != nil && s.Cfg.Dev) || os.Getenv("WMS_GAMING_TESTCORE") == "1"
}

func (s *Server) games() *gameService {
	s.gameOnce.Do(func() {
		dir := filepath.Join(s.Cfg.DataDir, "gaming")
		// Games unpacked for a session that a crash or a power cut ended. No session
		// can be running yet, and these can be gigabytes.
		_ = os.RemoveAll(filepath.Join(dir, "tmp"))
		coresDir := filepath.Join(dir, "cores")
		s.gameSvc = &gameService{dir: dir, probeSlot: make(chan struct{}, 1), cores: &cores.Manager{Dir: coresDir, System: filepath.Join(dir, "system"), Client: library.SafeClient, Custom: s.customCoreList},
			custom: newCustomService(filepath.Join(dir, "custom-cores"), filepath.Join(dir, "system"), append([]string{coresDir}, cores.DefaultExtra()...), customFetchClient()),
			mgr:    stream.NewManager(stream.Config{FFmpeg: s.Tools.FFmpeg, HaveEncoder: func(n string) bool { return s.Tools.Encoders[n] }, Log: s.Log.With("cat", "games", "module", "wittyplay")})}
		s.gameSvc.archives = &gamearchive.Cache{Dir: filepath.Join(dir, "unpacked"), Limit: s.unpackLimit}
	})
	return s.gameSvc
}

// StopGames saves and ends every game; the server is shutting down.
func (s *Server) StopGames() {
	if s.gameSvc != nil {
		s.gameSvc.mgr.StopAll()
		s.gameSvc.archives.Close()
	}
}

// unpackLimit is the size of the unpacked games cache, in bytes.
func (s *Server) unpackLimit() int64 {
	gb, err := strconv.Atoi(s.DB.GetSetting(context.Background(), "gamingUnpackGB", ""))
	if err != nil || gb < 1 {
		gb = defaultUnpackGB
	}
	return int64(gb) << 30
}

// findProgram looks in the program's bin folder, beside it, and in the development
// build folder.
func (s *Server) findProgram(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dirs := layout.Current().ProgramDirs()
	// Relative to wherever it was started, so never for a real install: that can be a
	// folder other people can write to.
	if s.Cfg != nil && s.Cfg.Dev {
		dirs = append(dirs, ".build")
	}
	for _, d := range dirs {
		p := filepath.Join(d, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			if abs, err := filepath.Abs(p); err == nil {
				return abs
			}
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

// wittyPlayID is the module whose downloaded versions can run games.
const wittyPlayID = "wittyplay"

// gameRunner picks the program that runs a game: the built-in host, or
// WittyRetroArch when it is installed. Both speak the same protocol over pipes,
// so a GPL program runs as a separate process beside this AGPL-licensed one. A
// downloaded version of the host comes before the installed one, and version says
// which it is; asking checks its signature, so this is asked again for each game.
func (s *Server) gameRunner(ctx context.Context) (path string, args []string, version string) {
	if p := os.Getenv("WMS_WITTYPLAY_RUNNER"); p != "" {
		return p, nil, ""
	}
	choice := s.DB.GetSetting(ctx, "gamingRunner", "auto")
	if choice == "auto" || choice == "retroarch" {
		if p := s.findProgram("wittyretroarch"); p != "" {
			return p, []string{"--wittyplay"}, ""
		}
	}
	if s.Modules != nil {
		if p, a, ver, ok := s.Modules.ProgramCommand(wittyPlayID); ok {
			return p, a, ver
		}
	}
	return "", nil, ""
}

// awaitHardwareProbe finds out which hardware encoders work, the first time a game
// needs one. It waits long enough for a working encoder to answer, and no longer:
// the game can start without it, and the result is there for the next.
func (s *Server) awaitHardwareProbe(ctx context.Context) {
	s.Tools.StartHardwareProbe()
	deadline := time.After(8 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, done := s.Tools.HardwareStatus(); done {
			return
		}
		select {
		case <-tick.C:
		case <-deadline:
			return
		case <-ctx.Done():
			return
		}
	}
}

// configureGames sets how the next game runs, with no restart needed for a new
// version of the runner. It returns which downloaded version that is, if any.
func (s *Server) configureGames(ctx context.Context) (version string, err error) {
	g := s.games()
	runner, args, version := s.gameRunner(ctx)
	if runner == "" {
		return "", errors.New("WittyPlay's game runner is missing from this install")
	}
	var hw []string
	list, _ := s.Tools.HardwareStatus()
	enabled := map[string]bool{}
	for _, n := range strings.Split(s.DB.GetSetting(ctx, "hwEncoders", ""), ",") {
		enabled[strings.TrimSpace(n)] = true
	}
	var gpuEnc []string
	for _, e := range list {
		if e.Working && enabled[e.Name] {
			hw = append(hw, e.Name)
		}
		if e.Working {
			gpuEnc = append(gpuEnc, e.Name)
		}
	}
	maxSessions, _ := strconv.Atoi(s.DB.GetSetting(ctx, "gamingMaxSessions", "2"))
	maxGPU, _ := strconv.Atoi(s.DB.GetSetting(ctx, "gamingMaxGpuSessions", "2"))
	port, _ := strconv.Atoi(s.DB.GetSetting(ctx, "gamingUdpPort", "0"))
	if port < 0 || port > 65535 {
		port = 0
	}
	g.mgr.Configure(func(c *stream.Config) {
		c.Runner, c.RunnerArgs, c.HWEncoders, c.GPUEncoders, c.MaxSessions, c.MaxGPU, c.UDPPort = runner, args, hw, gpuEnc, maxSessions, maxGPU, port
		c.PublicIPs = splitNonEmpty(s.DB.GetSetting(ctx, "gamingPublicIp", ""))
		c.STUN = splitNonEmpty(s.DB.GetSetting(ctx, "gamingStun", ""))
	})
	return version, nil
}

func splitNonEmpty(v string) []string {
	var out []string
	for _, x := range strings.Split(v, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func (s *Server) preferredCores(ctx context.Context) map[string]string {
	out := map[string]string{}
	_ = json.Unmarshal([]byte(s.DB.GetSetting(ctx, "gamingCores", "{}")), &out)
	return out
}

// consoleView is a console with what this server has for it.
type consoleView struct {
	wittyplay.Console
	Installed []cores.Core `json:"installed"`
	NeedsGPU  bool         `json:"needsGpu,omitempty"` // every core on offer draws with a graphics card
	GPUCores  []string     `json:"gpuCores,omitempty"`
	// For an administrator: approved custom cores assigned to it, and which core it
	// uses unless a game says otherwise.
	Custom      []cores.Core `json:"customCores,omitempty"`
	DefaultCore string       `json:"defaultCore,omitempty"`
	Ready       bool         `json:"ready"`
	// When it isn't ready: why, as a word (unavailable, core or gpu) and in words for people.
	// GPUMissing is the reason this server can't draw it, with reason gpu.
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
	GPUMissing string `json:"gpuMissing,omitempty"`
	// Software: there's no graphics card here, so the processor draws its games, and Notice
	// says what that means for how they run.
	Software bool   `json:"software,omitempty"`
	Notice   string `json:"notice,omitempty"`
	// CoreSupport is, for each core on offer, the server platforms it runs on and how it draws
	// on each, and here. Every device plays what any of them runs: the server streams it.
	CoreSupport map[string]coreSupport `json:"coreSupport,omitempty"`
	// CoreAbout is, for each core on offer, what it's good at beside the console's others
	// (core_notes.go).
	CoreAbout map[string]coreNote `json:"coreAbout,omitempty"`
	Pad       wittyplay.PadLayout `json:"pad"`
	// Count: with ?library=<id>, how many games that library holds for this console.
	Count int `json:"count,omitempty"`
	// Era, years, how its games come and what it can do (game_traits.go).
	Era          string               `json:"era,omitempty"`
	Launched     int                  `json:"launched,omitempty"`
	Ended        int                  `json:"ended,omitempty"`
	Media        string               `json:"media,omitempty"`
	Capabilities *ConsoleCapabilities `json:"capabilities,omitempty"`
	Shelf        *ShelfLook           `json:"shelf,omitempty"` // its case and cartridge, for the games home
	// For Settings, Consoles & cores: BIOS files in place, what's stopping it from starting
	// if that's a missing BIOS, and how many games every library holds for it.
	BiosFound  []string       `json:"biosFound,omitempty"`
	BiosFiles  []biosFileView `json:"biosFiles,omitempty"` // in the system folder, and where each came from
	Blocked    string         `json:"blocked,omitempty"`
	TotalGames int            `json:"totalGames,omitempty"`
	// LogoName is the name to ask the logo service for, kind "console". See docs/METADATA.md.
	LogoName string `json:"logoName,omitempty"`
	// Emulators are the standalone emulators set up to play it here (games_standalone.go), and
	// Standalone the one Play starts when no core plays it.
	Emulators  []emulatorOffer `json:"standaloneEmulators,omitempty"`
	Standalone string          `json:"standalone,omitempty"`
}

func (s *Server) consoleViews(ctx context.Context) []consoleView {
	all := s.games().cores.List()
	byID := map[string]cores.Core{}
	for _, c := range all {
		byID[c.ID] = c
	}
	testing := s.testingGames()
	out := []consoleView{}
	prefs := s.preferredCores(ctx)
	customOn := s.customCoresOn(ctx)
	var gpuWhy *string // asked once, and only if a console needs to know
	offers := s.consoleOffers(ctx)
	for _, c := range wittyplay.All() {
		if c.Hidden && !testing {
			continue
		}
		v := consoleView{Console: c, Installed: []cores.Core{}, Pad: wittyplay.Pad(c.ID), LogoName: library.ConsoleLogoName(c.ID)}
		addTraits(&v)
		if v.Cores == nil {
			v.Cores = []wittyplay.Core{}
		}
		v.CoreSupport = map[string]coreSupport{}
		for _, want := range c.Cores {
			if got, ok := byID[want.ID]; ok {
				v.Installed = append(v.Installed, got)
			}
			gpuCore := wittyplay.Traits(want.ID).GPU
			if gpuCore {
				v.GPUCores = append(v.GPUCores, want.ID)
				if gpuWhy == nil {
					why := s.gpuMissing(ctx)
					gpuWhy = &why
				}
			}
			noGPU := ""
			if gpuWhy != nil {
				noGPU = *gpuWhy
			}
			v.CoreSupport[want.ID] = coreSupportFor(want.ID, gpuCore, noGPU)
			if n := coreNoteFor(want.ID); n != nil {
				if v.CoreAbout == nil {
					v.CoreAbout = map[string]coreNote{}
				}
				v.CoreAbout[want.ID] = *n
			}
		}
		v.NeedsGPU = len(c.Cores) > 0 && len(v.GPUCores) == len(c.Cores)
		for _, got := range all {
			if got.Source == cores.SourceCustom && slices.Contains(got.Consoles, c.ID) {
				v.Custom = append(v.Custom, got)
				if !slices.ContainsFunc(v.Installed, func(x cores.Core) bool { return x.ID == got.ID }) {
					v.Installed = append(v.Installed, got)
				}
			}
		}
		v.DefaultCore = prefs[c.ID]
		// An administrator assigning a core has answered "not playable yet" for themselves.
		if len(v.Custom) > 0 {
			v.Unavailable = ""
		}
		if wittyplay.IsCustomConsole(c.ID) && !customOn && len(v.Installed) == 0 {
			v.Unavailable = "Custom cores are turned off on this server."
		}
		// A core someone placed themselves shows up under every console it could
		// serve only when we know it; anything else is chosen by name in Settings.
		v.Ready = v.Unavailable == "" && (len(v.Installed) > 0 || c.ID == wittyplay.TestConsole)
		// Every core it could play on draws on a graphics card, and this server has none it can use:
		// the processor draws, where a core can have it, and otherwise its games can't play here.
		if c.ID != wittyplay.TestConsole && v.Unavailable == "" && s.drawsOnlyOnGPU(&v) {
			if gpuWhy == nil {
				why := s.gpuMissing(ctx)
				gpuWhy = &why
			}
			switch {
			case *gpuWhy == "":
				// A card, but the core it would start on draws wrongly on it on this system, and
				// no other installed core draws right on it (see pickCore).
				if v.Ready && len(v.Installed) > 0 && drawsWrongHere(v.Installed[0]) != "" && cpuOptions(v.Installed[0]) != nil &&
					(v.DefaultCore != "" || rightOnTheCard(v.Installed, v.Installed[0], s.coreDrawsOnGPU) == nil) {
					v.Software, v.Notice = true, cardFaultNotice()
				}
			case s.canDrawOnCPU(&v):
				if v.Ready { // with nothing installed, a core it could be is still needed first
					v.Software, v.Notice = true, softwareNotice
				}
			default:
				v.Ready, v.Reason, v.Message, v.GPUMissing = false, "gpu", gpuMessage(c.Name, *gpuWhy), *gpuWhy
			}
		}
		if v.Ready || len(c.Cores) == 0 || len(offers[c.ID]) > 0 {
			applyStandalone(&v, offers[c.ID])
		}
		switch {
		case v.Ready || v.Reason != "":
		case v.Unavailable != "":
			v.Reason, v.Message = "unavailable", v.Unavailable
		default:
			v.Reason, v.Message = "core", "No core is installed for "+c.Name+"."
		}
		out = append(out, v)
	}
	return out
}

// canDrawOnCPU says one of the cores a console could play on here can have the processor draw:
// those installed for it, or with none installed, those on offer.
func (s *Server) canDrawOnCPU(v *consoleView) bool {
	if len(v.Installed) == 0 {
		return slices.ContainsFunc(v.Cores, func(c wittyplay.Core) bool { return cpuDrawing[c.ID] != nil })
	}
	return slices.ContainsFunc(v.Installed, func(c cores.Core) bool { return cpuOptions(c) != nil })
}

// drawsOnlyOnGPU says every core a console could play on here draws on a graphics card:
// those installed for it, or with none installed, those on offer.
func (s *Server) drawsOnlyOnGPU(v *consoleView) bool {
	if len(v.Installed) == 0 {
		return v.NeedsGPU
	}
	for _, c := range v.Installed {
		if !s.coreDrawsOnGPU(c) {
			return false
		}
	}
	return true
}

// hideCustom takes out what an administrator chose to add, for everyone else: which custom
// cores there are, their versions and hashes. Whether a console is ready stays.
func hideCustom(views []consoleView) {
	for i := range views {
		views[i].Custom, views[i].DefaultCore = nil, ""
		views[i].Installed = slices.DeleteFunc(views[i].Installed, func(c cores.Core) bool { return c.Source == cores.SourceCustom })
	}
}

func (s *Server) handleGameConsoles(w http.ResponseWriter, r *http.Request) {
	views := s.consoleViews(r.Context())
	if !userFrom(r).IsAdmin {
		hideCustom(views)
	}
	if id := int64(qInt(r, "library", 0)); id != 0 {
		if counts, err := s.Store.GameCounts(r.Context(), id, s.access(r)); err == nil {
			for i := range views {
				views[i].Count = counts[views[i].ID]
			}
		}
	}
	writeJSON(w, 200, views)
}

// handleGameModes says how a kind of client can connect to a game on this server.
func (s *Server) handleGameModes(w http.ResponseWriter, r *http.Request) {
	if _, err := s.configureGames(r.Context()); err != nil { // so that hardware encoders count
		s.Log.WarnContext(r.Context(), "could not set up games", "cat", "games", "err", err)
	}
	client := gameClient(r.URL.Query().Get("client"))
	writeJSON(w, 200, map[string]any{"client": client, "modes": s.games().mgr.Modes(client)})
}

// gameClient: only a browser says it is one, so a mode that needs a browser never
// reaches an app that hasn't heard of this field.
func gameClient(v string) string {
	if v == stream.ClientBrowser {
		return v
	}
	return stream.ClientNative
}

// gameConsole finds a console this account may be told about.
func (s *Server) gameConsole(id string) *wittyplay.Console {
	c := wittyplay.ByID(id)
	if c == nil || (c.Hidden && !s.testingGames()) {
		return nil
	}
	return c
}

// A client's name for itself, chosen by the client and kept on the device. It only
// labels saved layouts, so it proves nothing and needs to be nothing more than tidy.
var gameDeviceRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func touchKey(device, orientation string) string { return device + "/" + orientation }

// handleGameControls gives a client everything it needs to draw and map a console's
// controller: what the console has, the defaults, and what this person saved. Asking
// with ?device= includes the touch layouts saved for that device.
func (s *Server) handleGameControls(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("console")
	if s.gameConsole(id) == nil {
		writeErr(w, 404, "not found")
		return
	}
	ctx, userID := r.Context(), userFrom(r).ID
	saved, err := s.Store.GameControls(ctx, userID, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	profiles, err := s.Store.GameProfiles(ctx, userID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	background, err := s.Store.GetGameBackground(ctx, userID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var game int64
	if g, err := strconv.ParseInt(r.URL.Query().Get("game"), 10, 64); err == nil && g > 0 {
		if it, err := s.visibleItem(r, g); err == nil && it.Kind == "game" {
			game = g
		}
	}
	gamepads := saved[wittyplay.KindGamepad]
	if gamepads == nil {
		gamepads = map[string]json.RawMessage{}
	}
	var keyboard json.RawMessage
	if k, ok := saved[wittyplay.KindKeyboard][""]; ok {
		keyboard = k
	}
	// What Edit layout kept, for every shape of device: they follow the account (game_layouts.go).
	layouts := map[string]json.RawMessage{}
	for shape, l := range saved[kindLayout] {
		if layoutShapes[shape] {
			layouts[shape] = l
		}
	}
	touch := map[string]json.RawMessage{}
	if device := r.URL.Query().Get("device"); gameDeviceRe.MatchString(device) {
		for _, o := range []string{wittyplay.Landscape, wittyplay.Portrait} {
			if l, ok := saved[wittyplay.KindTouch][touchKey(device, o)]; ok {
				touch[o] = l
			}
		}
	}
	writeJSON(w, 200, map[string]any{
		"console":  id,
		"pad":      wittyplay.Pad(id),
		"controls": wittyplay.Controls(id),
		"targets":  wittyplay.Targets(id),
		"defaults": map[string]any{
			"gamepad": wittyplay.DefaultGamepadFor(id), "keyboard": wittyplay.DefaultKeyboardFor(id), "remote": wittyplay.DefaultRemote(), "options": wittyplay.DefaultOptions(id),
			"touch": map[string]any{wittyplay.Landscape: wittyplay.DefaultTouch(id, wittyplay.Landscape), wittyplay.Portrait: wittyplay.DefaultTouch(id, wittyplay.Portrait)},
		},
		"saved":      map[string]any{"gamepad": gamepads, "keyboard": keyboard, "touch": touch, "layouts": layouts},
		"game":       game,
		"profiles":   activeProfilesFor(profiles, id, game),
		"background": background,
	})
}

// handlePutGameControl saves one layout for the signed-in account, or removes it
// when data is null. Everything is checked: a layout comes from a client.
func (s *Server) handlePutGameControl(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("console")
	if s.gameConsole(id) == nil {
		writeErr(w, 404, "not found")
		return
	}
	var b struct {
		Kind        string          `json:"kind"`
		Key         string          `json:"key"` // gamepad: its model; touch: this device
		Orientation string          `json:"orientation"`
		Data        json.RawMessage `json:"data"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, wittyplay.MaxControlSize+1024)
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "this layout can't be read")
		return
	}
	key := ""
	switch b.Kind {
	case wittyplay.KindGamepad:
		key = b.Key
		if key == "" || len(key) > 120 {
			writeErr(w, 400, "a gamepad's model name is needed")
			return
		}
	case wittyplay.KindKeyboard:
	case wittyplay.KindTouch:
		if !gameDeviceRe.MatchString(b.Key) || (b.Orientation != wittyplay.Landscape && b.Orientation != wittyplay.Portrait) {
			writeErr(w, 400, "a touch layout needs a device and an orientation")
			return
		}
		key = touchKey(b.Key, b.Orientation)
	case kindLayout:
		if !layoutShapes[b.Key] {
			writeErr(w, 400, "a layout is for a shape of device: phone-portrait, phone-landscape, tablet-portrait, tablet-landscape, desktop or tv")
			return
		}
		key = b.Key
	default:
		writeErr(w, 400, "unknown kind of layout")
		return
	}
	var clean json.RawMessage
	if len(b.Data) > 0 && string(b.Data) != "null" {
		var err error
		if b.Kind == kindLayout {
			clean, err = cleanLayout(controlKeys(id), b.Data)
		} else {
			clean, err = wittyplay.CleanControl(id, b.Kind, b.Data)
		}
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	if err := s.Store.SetGameControl(r.Context(), userFrom(r).ID, id, b.Kind, key, clean); err != nil {
		if errors.Is(err, library.ErrTooManyControls) {
			writeErr(w, 409, err.Error())
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// saveDir is one account's folder for one game: built only from numbers.
func (g *gameService) saveDir(userID, itemID int64) string {
	return filepath.Join(g.dir, "saves", "u"+strconv.FormatInt(userID, 10), "i"+strconv.FormatInt(itemID, 10))
}

// gameItem loads a game this account may see.
func (s *Server) gameItem(r *http.Request) (*library.Item, bool) {
	id, ok := pathInt(r, "id")
	if !ok {
		return nil, false
	}
	it, err := s.visibleItem(r, id)
	if err != nil || it.Kind != "game" {
		return nil, false
	}
	return it, true
}

func (s *Server) handleGamePlay(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	var b struct {
		Resume   bool   `json:"resume"`
		Client   string `json:"client"`   // browser, or anything else for a native app
		SafeMode bool   `json:"safeMode"` // with default settings, no cheats and no patches
		// Device names what's starting it, for "Continue here" on the account's other devices.
		Device string `json:"device"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if it.Console == library.RigConsole {
		// A rig's game runs on that PC, not here: see POST /rigs/games/{id}/play.
		writeJSON(w, 409, map[string]any{"error": "This game plays on your own PC, not on this server.", "rig": true})
		return
	}
	console := wittyplay.ByID(it.Console)
	if console == nil || (console.Hidden && !s.testingGames()) {
		writeErr(w, 404, "not found")
		return
	}
	ctx := r.Context()
	if console.Unavailable != "" && !s.customFor(ctx, console.ID) {
		writeErr(w, 409, console.Unavailable)
		return
	}
	began := time.Now()
	g := s.games()
	systemDir := g.systemDir()
	if err := os.MkdirAll(systemDir, 0o755); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	corePath, coreID := ipc.TestCore, "test"
	var coreOptions map[string]string
	var fallbacks []stream.CoreChoice
	var runnerArgs []string
	gpu := false
	var playing *cores.Core
	// Why a game drawn on a graphics card can't run here, once it's been asked: such a core is
	// never started, nor tried when another fails, unless the processor can draw for it. software
	// says the processor draws this one.
	noGPU := ""
	software, notice := false, ""
	if console.ID != wittyplay.TestConsole {
		// BIOS files left in a folder beside the games are fetched first, for
		// every core that might use one: by what they are, then by their names.
		s.findBIOSNear(r.Context(), it.Path)
		for _, want := range console.Cores {
			if err := wittyplay.CollectBIOS(console, want.ID, it.Path, systemDir); err != nil {
				s.Log.Warn("copying a BIOS", "console", console.ID, "err", err)
			}
		}
		pick := s.pickCore(ctx, console, it.ID, systemDir)
		if pick.noGPU != "" {
			writeJSON(w, 409, gpuRefusal(console, pick.noGPU))
			return
		}
		core, blocked := pick.core, pick.blocked
		software, notice = pick.software, pick.notice
		if core == nil && blocked != nil {
			writeJSON(w, 409, map[string]any{"error": wittyplay.BIOSHelp(blocked.ID, console.Name), "needBios": true, "console": console.ID,
				"file": wittyplay.MissingBIOS(blocked.ID, systemDir)})
			return
		}
		if core == nil {
			s.Log.WarnContext(ctx, "a game couldn't start: no core is installed for its console", "cat", "games", "module", "wittyplay", "console", console.ID, "game", it.Title)
			writeJSON(w, 409, map[string]any{"error": "No core is installed for " + console.Name + ".", "needCore": true, "console": console.ID})
			return
		}
		if core.Source == cores.SourceCustom {
			if err := s.checkCustomCore(ctx, core); err != nil {
				writeErr(w, 409, err.Error())
				return
			}
		}
		corePath, coreID = core.Path, core.ID
		playing = core
		gpu = !software && (wittyplay.Traits(core.ID).GPU || (core.Source == cores.SourceCustom && core.WantsGPU))
		runnerArgs = wittyplay.RunnerArgs(core.ID)
		if software {
			runnerArgs = withoutGPUArg(runnerArgs)
		}
		resolution := s.DB.GetSetting(ctx, "gamingResolution", "")
		coreOptions = wittyplay.StartOptions(console.ID, core.ID, resolution, systemDir)
		if rule := wittyplay.Traits(core.ID).RequiredBIOS; rule != nil && rule.Option != "" {
			if bios := s.regionBIOSFor(ctx, it, rule.Folder, coreOptions[rule.Option]); bios != "" {
				coreOptions = withOption(coreOptions, rule.Option, bios)
			}
		}
		// If the game won't load on this core, the others get their turn.
		others := g.cores.AllForConsole(console, core.ID)[1:]
		if slices.ContainsFunc(others, func(c cores.Core) bool { return s.coreDrawsOnGPU(c) }) {
			noGPU = s.gpuMissing(ctx)
		}
		for _, other := range others {
			// A custom core only ever runs where an administrator assigned it, and after its check.
			if other.Source == cores.SourceCustom || wittyplay.MissingBIOS(other.ID, systemDir) != "" {
				continue
			}
			next := stream.CoreChoice{Core: other.Path, CoreID: other.ID, RunnerArgs: wittyplay.RunnerArgs(other.ID), GPU: wittyplay.Traits(other.ID).GPU, Options: wittyplay.StartOptions(console.ID, other.ID, resolution, systemDir)}
			if s.coreDrawsOnGPU(other) && (noGPU != "" || drawsWrongHere(other) != "") {
				cpu := cpuOptions(other)
				if cpu == nil {
					continue
				}
				next.Options, next.GPU, next.RunnerArgs = overOptions(next.Options, cpu), false, withoutGPUArg(next.RunnerArgs)
			}
			fallbacks = append(fallbacks, next)
		}
	}
	if gpu {
		s.awaitHardwareProbe(ctx)
	}
	version, err := s.configureGames(ctx)
	if err != nil {
		writeErr(w, 503, err.Error())
		return
	}
	if _, err := os.Stat(it.Path); err != nil || !s.Store.LibraryFile(ctx, it.Path) {
		writeErr(w, 404, "the game file is missing")
		return
	}
	// Full is full, whatever the game. Unpacking it, copying it and applying its patches
	// would all be for nothing.
	if s.gamesFull(ctx, g, userFrom(r).ID) {
		writeErr(w, 503, stream.ErrBusy.Error())
		return
	}
	rom, cleanup, err := g.prepareGame(ctx, s.Store, it, console)
	if err != nil {
		if ctx.Err() != nil {
			return // whoever asked has gone; the unpack carries on for next time
		}
		writeErr(w, 422, err.Error())
		return
	}
	rom, cleanup, newOnly, err := g.prepare3DS(rom, cleanup, console)
	if err != nil {
		if errors.Is(err, wittyplay.ErrEncrypted3DS) {
			writeJSON(w, 422, map[string]any{"error": encryptedMessage(), "encrypted": true, "console": console.ID})
			return
		}
		writeErr(w, 422, err.Error())
		return
	}
	if newOnly {
		// A game made for the New 3DS won't start on the older model, which is what cores
		// are out of the box. A core with no such option refuses the key and carries on.
		coreOptions = withOption(coreOptions, wittyplay.NewThreeDSOption, wittyplay.NewThreeDSValue)
		for i := range fallbacks {
			fallbacks[i].Options = withOption(fallbacks[i].Options, wittyplay.NewThreeDSOption, wittyplay.NewThreeDSValue)
		}
	}
	if why := wittyplay.Refusal(rom, console.ID); why != "" {
		if cleanup != nil {
			cleanup()
		}
		writeErr(w, 422, why)
		return
	}
	if console.ID == "snes" && wittyplay.IsSatellaview(rom) {
		// No other core will load one, so nothing else is tried.
		core := g.cores.AllForConsole(console, wittyplay.SatellaviewCore)
		problem := ""
		if _, err := os.Stat(filepath.Join(systemDir, wittyplay.SatellaviewBIOS)); err != nil {
			problem = "This is a Satellaview broadcast game, which needs the receiver's cartridge. Put a copy of yours named " + wittyplay.SatellaviewBIOS + " in a folder named " + wittyplay.BIOSFolderName + " beside your " + console.Name + " games, and it will be found when you play. BIOS files are never downloaded for you."
		} else if len(core) == 0 || core[0].ID != wittyplay.SatellaviewCore {
			problem = "This is a Satellaview broadcast game, which only plays on Mesen-S. An admin can install that core in Settings, Consoles & cores."
		}
		if problem != "" {
			if cleanup != nil {
				cleanup()
			}
			writeJSON(w, 409, map[string]any{"error": problem, "console": console.ID, "file": wittyplay.SatellaviewBIOS})
			return
		}
		corePath, fallbacks = core[0].Path, nil
		coreOptions = wittyplay.StartOptions(console.ID, core[0].ID, s.DB.GetSetting(ctx, "gamingResolution", ""), systemDir)
	}
	// A custom core only runs where an administrator put it, so it's left to them.
	if console.ID == "n64" && (playing == nil || playing.Source != cores.SourceCustom) && wittyplay.IsLibdragon(rom) {
		core := g.cores.AllForConsole(console, wittyplay.LibdragonCore)
		if len(core) == 0 || core[0].ID != wittyplay.LibdragonCore {
			if cleanup != nil {
				cleanup()
			}
			writeJSON(w, 409, map[string]any{"error": wittyplay.LibdragonNeedsCore, "otherCore": true, "console": console.ID})
			return
		}
		// It runs on the processor alone: WittyPlay gives it the settings for that.
		corePath, coreID, playing, fallbacks = core[0].Path, core[0].ID, &core[0], nil
		runnerArgs = wittyplay.RunnerArgs(core[0].ID)
		if software {
			runnerArgs = withoutGPUArg(runnerArgs)
		}
		coreOptions = wittyplay.StartOptions(console.ID, core[0].ID, s.DB.GetSetting(ctx, "gamingResolution", ""), systemDir)
	}
	s.Log.Debug("game prepared", "item", it.ID, "seconds", time.Since(began).Seconds())
	userID := userFrom(r).ID
	var coreAssets string
	var loadLimit time.Duration
	if playing != nil && playing.Source == cores.SourceCustom {
		// A virus scanner may look again at a file that has only just moved here.
		coreAssets, loadLimit = playing.Assets, stream.ProbeTimeout
	}
	opts := stream.Options{
		UserID: userID, ItemID: it.ID, Title: it.Title, Console: console.ID, Core: corePath, ROM: rom, SystemDir: systemDir,
		CoreAssets: coreAssets, LoadLimit: loadLimit,
		SaveDir: g.saveDir(userID, it.ID), Resume: b.Resume, Quality: s.DB.GetSetting(ctx, "gamingQuality", ""), Cleanup: cleanup,
		GPU: gpu, RunnerArgs: runnerArgs, CoreID: coreID, CoreOptions: coreOptions, Fallbacks: fallbacks,
		PreferDevices: wittyplay.Pad(console.ID).PreferDevices,
	}
	s.applyGameSettings(ctx, &opts, it, console, coreID, b.SafeMode)
	if software {
		// Over whatever anyone chose: without these, the core asks for a graphics card that isn't here.
		opts.CoreOptions = overOptions(opts.CoreOptions, cpuDrawing[coreID])
		opts.Notices = append(opts.Notices, notice)
	}
	sess, err := g.mgr.Start(opts)
	if err != nil {
		if opts.Cleanup != nil {
			opts.Cleanup() // it may never have got as far as a session that would
		}
		if errors.Is(err, stream.ErrBusy) || errors.Is(err, stream.ErrGPUBusy) {
			writeErr(w, 503, err.Error())
			return
		}
		s.Log.WarnContext(r.Context(), "could not start a game", "cat", "games", "item", it.ID, "game", it.Title, "console", console.ID, "core", coreID, "err", err)
		// Only WittyPlay's helper failing counts against its version. The runner stopping, a
		// core crashing or refusing, a file it can't load: those belong to this game, its core
		// or its console, and are recorded against the core, so that one console that can't
		// run here never takes every other game off the server with it.
		helper := errors.Is(err, stream.ErrHelper)
		if version != "" && helper {
			_ = s.Modules.ReportFailure(wittyPlayID, version, "its helper failed while starting a game: "+err.Error())
		}
		if playing != nil && !helper {
			s.noteCorePlay(ctx, playing.ID, it.ID, 1, 1, 0, err.Error())
			if playing.Source == cores.SourceCustom {
				s.auditCore(ctx, userFrom(r), "use-failed", playing.ID, fmt.Sprintf("item %d: %v", it.ID, err))
			}
		}
		if msg := err.Error(); strings.Contains(msg, stream.NoGPUText) {
			why := ""
			if i := strings.Index(msg, "this server has "+stream.NoGPUText); i >= 0 {
				why = plainGPUReason(msg[i:])
			}
			writeJSON(w, 409, gpuRefusal(console, why))
			return
		}
		if helper {
			writeErr(w, 500, "This game could not start: WittyPlay's helper stopped while starting it. Try again.")
			return
		}
		writeErr(w, 500, "This game could not start. "+err.Error())
		return
	}
	if version != "" {
		s.Modules.ReportWorking(wittyPlayID, version)
	}
	logging.Go("game play time", func() { s.recordPlayTime(sess, userID, it.ID) }, "cat", "games", "session", sess.ID())
	s.streams.game(sess.ID(), userID, userFrom(r).Name, it.Title, sess.Done(), func() GameView {
		st := sess.Stats()
		return GameView{Core: st.Core, Encoder: st.Encoder, GameFPS: st.GameFPS, SentFPS: st.SentFPS, EncodeMs: st.EncodeMs, Kbps: st.Kbps, DroppedFrames: st.DroppedFrames, Width: st.Width, Height: st.Height, Paused: st.Paused}
	})
	if playing != nil {
		s.noteCorePlay(ctx, playing.ID, it.ID, 1, 0, 0, "")
		watcher := userFrom(r)
		logging.Go("game core watch", func() { s.watchCorePlay(sess, playing, watcher, it.ID) }, "cat", "games", "session", sess.ID())
	}
	host := userFrom(r)
	hostName := host.DisplayName
	if hostName == "" {
		hostName = host.Name
	}
	s.noteGameStarted(sess, userID, hostName, deviceName(b.Device, r))
	if err := s.Store.NoteGamePlayed(ctx, userID, it.ID); err != nil {
		s.Log.Warn("recording a play", "err", err)
	}
	info := sess.Info()
	info.Modes = g.mgr.Modes(gameClient(b.Client))
	writeJSON(w, 201, info)
}

// gamesFull says this person starting a game would go over the limit. Their own game
// doesn't count, since starting another ends it. A slot may come free by the time it
// matters, or go: this is only there so that nobody unpacks or patches a game with nowhere
// to run it, and the graphics card's own limit is still checked when the game starts.
func (s *Server) gamesFull(ctx context.Context, g *gameService, userID int64) bool {
	limit, _ := strconv.Atoi(s.DB.GetSetting(ctx, "gamingMaxSessions", "2"))
	running := g.mgr.Count()
	if g.mgr.ForUser(userID) != nil {
		running--
	}
	return running >= limit
}

// recordPlayTime adds the time played to what "Continue playing" shows, in the
// same progress table videos use: position is the total time played.
func (s *Server) recordPlayTime(sess *stream.Session, userID, itemID int64) {
	start := time.Now()
	<-sess.Done()
	ctx := context.Background()
	played := time.Since(start).Seconds()
	var total float64
	_ = s.DB.QueryRowContext(ctx, `SELECT position_sec FROM progress WHERE user_id = ? AND item_id = ?`, userID, itemID).Scan(&total)
	if err := s.Store.SetProgress(ctx, userID, itemID, library.Progress{PositionSec: total + played}); err != nil {
		s.Log.Warn("could not save time played", "cat", "games", "session", sess.ID(), "err", err)
	}
}

// encryptedMessage is why an encrypted 3DS game can't play, and what would.
func encryptedMessage() string {
	return strings.TrimRight(wittyplay.ErrEncrypted3DS.Error(), ". ") + ". " + library.DecryptedCopyPlays
}

// prepareROM returns the file a core should open. An archive is unpacked first, or
// already has been: see gamearchive.Cache. This waits for that, and cleanup lets the
// cache have the space back once the game has ended.
func (g *gameService) prepareROM(ctx context.Context, path string, console *wittyplay.Console, entry string) (rom string, cleanup func(), err error) {
	if !gamearchive.IsArchive(path) {
		return path, nil, nil
	}
	return g.archives.AcquireEntry(ctx, path, console, entry)
}

// prepare3DS checks a 3DS file before a core sees it. An encrypted one is refused:
// nothing here decrypts or looks for keys. An unencrypted install package holds the
// game as a plain program, which is copied out because the cores can't open a package.
// On failure the earlier cleanup has already run. Whether it's refused goes by the bytes
// where a program's header belongs, then its NoCrypto flag, and not by what the package's
// table of contents says or whether it has a ticket: see threeds.go in WittyPlay.
func (g *gameService) prepare3DS(rom string, cleanup func(), console *wittyplay.Console) (string, func(), bool, error) {
	if console.ID != "3ds" {
		return rom, cleanup, false, nil
	}
	fail := func(err error) (string, func(), bool, error) {
		if cleanup != nil {
			cleanup()
		}
		return "", nil, false, err
	}
	info, err := wittyplay.Inspect3DS(rom)
	if err != nil {
		return fail(err)
	}
	if info.Encrypted {
		return fail(wittyplay.ErrEncrypted3DS)
	}
	if !info.Package {
		return rom, cleanup, info.NewOnly, nil
	}
	out, err := g.packageProgram(rom, info)
	if err != nil {
		return fail(err)
	}
	return out, cleanup, info.NewOnly, nil
}

// packageProgram gives the program out of a 3DS package as a file of its own. A big game's
// copy takes a long while and as much room as the game, so it stays for next time rather than
// being made at every start. Only the last one stays, unless a game is running that may still
// be using another. The folder is emptied when the server starts.
func (g *gameService) packageProgram(rom string, info wittyplay.ThreeDSFile) (string, error) {
	st, err := os.Stat(rom)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(g.dir, "tmp", "3ds")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%d|%d", rom, st.Size(), st.ModTime().UnixNano()))
	out := filepath.Join(dir, hex.EncodeToString(sum[:8])+".cxi")
	g.programs.Lock()
	defer g.programs.Unlock()
	if have, err := os.Stat(out); err == nil && have.Size() == info.ProgramLen {
		return out, nil
	}
	if g.mgr.Count() == 0 {
		if ents, err := os.ReadDir(dir); err == nil {
			for _, e := range ents {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	part := out + ".part"
	if err := info.CopyProgram(rom, part); err != nil {
		_ = os.Remove(part)
		return "", err
	}
	if err := os.Rename(part, out); err != nil {
		_ = os.Remove(part)
		return "", err
	}
	return out, nil
}

func withOption(options map[string]string, key, value string) map[string]string {
	out := make(map[string]string, len(options)+1)
	for k, v := range options {
		out[k] = v
	}
	out[key] = value
	return out
}

// handleGamePrepare gets a game ready without waiting for it, and says how that's
// going, so a client can show progress for a slow unpack. Call it again to follow it.
// Starting a game works without it: POST play simply takes as long as unpacking does.
func (s *Server) handleGamePrepare(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	console := wittyplay.ByID(it.Console)
	if console == nil || (console.Hidden && !s.testingGames()) {
		writeErr(w, 404, "not found")
		return
	}
	if console.Unavailable != "" && !s.customFor(r.Context(), console.ID) {
		writeErr(w, 409, console.Unavailable)
		return
	}
	g := s.games()
	// Not on this server says so here too, as play would, before anything is unpacked for it.
	if console.ID != wittyplay.TestConsole {
		if pick := s.pickCore(r.Context(), console, it.ID, g.systemDir()); pick.noGPU != "" {
			writeJSON(w, 409, gpuRefusal(console, pick.noGPU))
			return
		}
	}
	ready := gamearchive.Status{State: "ready"}
	discs, _ := s.Store.GameDiscs(r.Context(), it.ID)
	anyArchive := gamearchive.IsArchive(it.Path)
	for _, d := range discs {
		anyArchive = anyArchive || gamearchive.IsArchive(d.Path)
	}
	if !anyArchive {
		writeJSON(w, 200, ready)
		return
	}
	// Unpacking a disc only to be told no core is installed would be a poor way to learn it.
	if console.ID != wittyplay.TestConsole && g.cores.ForConsole(console, s.preferredCores(r.Context())[console.ID]) == nil {
		writeJSON(w, 409, map[string]any{"error": "No core is installed for " + console.Name + ".", "needCore": true, "console": console.ID})
		return
	}
	st := ready
	if gamearchive.IsArchive(it.Path) {
		st = g.archives.PrepareEntry(it.Path, console, s.Store.GameEntry(r.Context(), it.ID))
	}
	if len(discs) > 1 && console.Accepts(".m3u") {
		// Each disc in an archive has to be there. What's asked about is whichever is furthest behind.
		for _, d := range discs {
			if gamearchive.IsArchive(d.Path) {
				if one := g.archives.Prepare(d.Path, console); one.State != "ready" {
					st = one
					break
				}
			}
		}
	}
	if st.Error != "" {
		writeErr(w, 422, st.Error)
		return
	}
	writeJSON(w, 200, st)
}

// session finds a session for the account that started it. To anyone else it
// doesn't exist.
func (s *Server) session(r *http.Request) *stream.Session {
	if s.gameSvc == nil {
		return nil
	}
	sess := s.gameSvc.mgr.Get(r.PathValue("sid"))
	if sess == nil || sess.UserID() != userFrom(r).ID {
		return nil
	}
	return sess
}

func (s *Server) handlePlayGet(w http.ResponseWriter, r *http.Request) {
	sess := s.watchSession(r)
	if sess == nil {
		writeErr(w, 404, "not found")
		return
	}
	info := sess.Info()
	info.Modes = s.games().mgr.Modes(gameClient(r.URL.Query().Get("client")))
	writeJSON(w, 200, info)
}

func (s *Server) handlePlayStop(w http.ResponseWriter, r *http.Request) {
	sess := s.session(r)
	if sess == nil {
		writeErr(w, 404, "not found")
		return
	}
	sess.Stop()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handlePlayWebRTC(w http.ResponseWriter, r *http.Request) {
	sess := s.watchSession(r)
	if sess == nil {
		writeErr(w, 404, "not found")
		return
	}
	if !s.games().peers.offer(sess.ID(), time.Now()) {
		tooManyPeers(w, "60")
		return
	}
	var b struct {
		SDP    string `json:"sdp"`
		Prefer string `json:"prefer"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, stream.MaxOfferSize+1024)
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "bad offer")
		return
	}
	answer, err := sess.Answer(r.Context(), b.SDP, b.Prefer)
	if err != nil {
		s.Log.WarnContext(r.Context(), "could not connect to a game by WebRTC", "cat", "games", "session", sess.ID(), "err", err)
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"sdp": answer})
}

func (s *Server) handlePlayWS(w http.ResponseWriter, r *http.Request) {
	sess := s.watchSession(r)
	if sess == nil {
		writeErr(w, 404, "not found")
		return
	}
	release, ok := s.games().peers.socket(sess.ID(), time.Now())
	if !ok {
		tooManyPeers(w, "10")
		return
	}
	defer release()
	sess.ServeWS(w, r, r.URL.Query().Get("video"))
}

// streamRefused answers a refused version 2 connection: docs/GAMING-STREAM.md, Errors.
func streamRefused(w http.ResponseWriter, status int, code, text string) {
	writeJSON(w, status, map[string]string{"error": text, "code": code})
}

// handlePlayStream connects a device over WebRTC, version 2: its hello and its offer in, what it
// gets and the answer out. The server says whether it's away from home; the device doesn't.
// A WittyPlay from before version 2 of the stream has no Session.Connect and no /stream route. Its
// modes don't offer version 2 then, so clients keep to version 1, which works as it always has; a
// client that asks for version 2 anyway is told what to update, not that something failed.
const needsStreamV2 = "Version 2 of the stream needs a newer WittyPlay (0.3.5 or later). An administrator can update it in Settings, Modules. Version 1 (/play/{id}/webrtc and /play/{id}/ws) works meanwhile."

// moduleLacks says a call failed because the module's version doesn't have it.
func moduleLacks(err error) bool {
	var ce *service.CallError
	return errors.As(err, &ce) && ce.Code == "unknown-method"
}

// offersStreamV2 says the module's modes can be reached through version 2.
func offersStreamV2(modes []stream.Mode) bool {
	for _, m := range modes {
		if m.Stream >= stream.StreamVersion {
			return true
		}
	}
	return false
}

func (s *Server) handlePlayStream(w http.ResponseWriter, r *http.Request) {
	sess := s.watchSession(r)
	if sess == nil {
		streamRefused(w, 404, "no-session", "No such game is being played.")
		return
	}
	if !s.games().peers.offer(sess.ID(), time.Now()) {
		w.Header().Set("Retry-After", "60")
		streamRefused(w, 429, "busy", "Too many connections to this game. Try again in a minute.")
		return
	}
	var b struct {
		Hello stream.Hello `json:"hello"`
		SDP   string       `json:"sdp"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, stream.MaxOfferSize+8<<10)
	if err := readJSON(r, &b); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			streamRefused(w, 413, "too-large", "The connection offer is too large.")
			return
		}
		streamRefused(w, 400, "bad-hello", "The body must be {\"hello\": {...}, \"sdp\": \"...\"}.")
		return
	}
	// Checked here as well as in the module, so the answer is the same whichever WittyPlay is in.
	if b.Hello.V != stream.StreamVersion {
		streamRefused(w, 400, "bad-hello", fmt.Sprintf("This server speaks version %d of the stream.", stream.StreamVersion))
		return
	}
	// Someone who joined with a code drives the seat it gave them (game_seats.go).
	if sess.UserID() != userFrom(r).ID {
		if seat, ok := s.gameSvc.seats.seatOf(sess.ID(), userFrom(r).ID); ok {
			b.Hello.Seats = joinedSeats(seat)
		}
	}
	got, err := sess.Connect(r.Context(), b.Hello, b.SDP, !clientFrom(r).Where.AtHome())
	if err != nil {
		if moduleLacks(err) {
			streamRefused(w, 501, "needs-update", needsStreamV2)
			return
		}
		var se *stream.StreamError
		if errors.As(err, &se) && se.Status >= 400 && se.Status < 600 {
			streamRefused(w, se.Status, se.Code, se.Text)
			return
		}
		s.Log.WarnContext(r.Context(), "could not connect to a game", "cat", "games", "session", sess.ID(), "err", err)
		streamRefused(w, 500, "failed", err.Error())
		return
	}
	writeJSON(w, 200, got)
}

// handlePlayStreamWS is version 2's fallback: the same messages on a WebSocket.
func (s *Server) handlePlayStreamWS(w http.ResponseWriter, r *http.Request) {
	// Someone who joined with a code may connect here too. The hello on this socket goes straight to
	// WittyPlay, so their seat is the one their app asks for: GAMING-API.md, Seats.
	sess := s.watchSession(r)
	if sess == nil {
		streamRefused(w, 404, "no-session", "No such game is being played.")
		return
	}
	release, ok := s.games().peers.socket(sess.ID(), time.Now())
	if !ok {
		w.Header().Set("Retry-After", "10")
		streamRefused(w, 429, "busy", "Too many connections to this game. Try again in a minute.")
		return
	}
	defer release()
	// An older WittyPlay has no route to pass this to, and would only say "not found".
	if !offersStreamV2(s.games().mgr.Modes(stream.ClientNative)) {
		streamRefused(w, 501, "needs-update", needsStreamV2)
		return
	}
	sess.ServeStream(w, r, !clientFrom(r).Where.AtHome())
}

func (s *Server) handlePlayAction(w http.ResponseWriter, r *http.Request) {
	sess := s.session(r)
	if sess == nil {
		writeErr(w, 404, "not found")
		return
	}
	var b struct {
		Type   string  `json:"type"`
		Slot   int     `json:"slot"`
		On     bool    `json:"on"`
		Value  float64 `json:"value"`
		Key    string  `json:"key"`
		Text   string  `json:"text"`
		Port   int     `json:"port"`
		Device uint32  `json:"device"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var err error
	switch b.Type {
	case "save":
		if b.Slot < 1 {
			err = stream.ErrNoSlot // the automatic slot is written on stop
		} else {
			err = sess.SaveState(r.Context(), b.Slot)
		}
	case "load":
		err = sess.LoadState(b.Slot)
	case "reset":
		err = sess.Reset()
	case "pause":
		err = sess.Pause(b.On)
	case "disc":
		err = sess.ChangeDisc(b.Slot)
	case "device":
		err = sess.SetDevice(b.Port, b.Device)
	case "speed":
		var held float64
		if held, err = sess.SetSpeed(b.Value); err == nil {
			writeJSON(w, 200, map[string]any{"ok": true, "speed": held})
			return
		}
	case "rewind":
		err = sess.SetRewinding(b.On)
	case "advance":
		err = sess.Advance(max(int(b.Value), 1))
	case "undo":
		err = sess.Undo(r.Context())
	case "option":
		err = sess.SetOption(b.Key, b.Text)
	default:
		writeErr(w, 400, "unknown action")
		return
	}
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

type saveSlot struct {
	Slot      int    `json:"slot"`
	UpdatedAt int64  `json:"updatedAt"`
	Size      int64  `json:"size"`
	HasThumb  bool   `json:"hasThumb"`
	Core      string `json:"core,omitempty"` // the core that made it, when known
}

func (s *Server) handleGameSaves(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	dir := s.games().saveDir(userFrom(r).ID, it.ID)
	// One look at the folder, rather than asking after every slot there could be.
	found := map[string]os.FileInfo{}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if info, err := e.Info(); err == nil && !e.IsDir() {
				found[e.Name()] = info
			}
		}
	}
	out := []saveSlot{}
	for slot := 0; slot <= stream.SlotPrevAuto; slot++ {
		name, err := stream.SlotName(slot)
		if err != nil {
			continue
		}
		st, ok := found[name+".state"]
		if !ok {
			continue
		}
		_, thumb := found[name+".png"]
		out = append(out, saveSlot{Slot: slot, UpdatedAt: st.ModTime().Unix(), Size: st.Size(), HasThumb: thumb, Core: slotCore(dir, name)})
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleGameSaveThumb(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	slot, err := strconv.Atoi(r.PathValue("slot"))
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	name, err := stream.SlotName(slot)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, no-cache")
	http.ServeFile(w, r, filepath.Join(s.games().saveDir(userFrom(r).ID, it.ID), name+".png"))
}

func (s *Server) handleGameSaveDelete(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	slot, err := strconv.Atoi(r.PathValue("slot"))
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	name, err := stream.SlotName(slot)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	dir := s.games().saveDir(userFrom(r).ID, it.ID)
	_ = os.Remove(filepath.Join(dir, name+".png"))
	_ = os.Remove(filepath.Join(dir, name+".json"))
	if err := os.Remove(filepath.Join(dir, name+".state")); err != nil {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handleGaming is the settings page's view of what this server can do.
func (s *Server) handleGaming(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g := s.games()
	runner, _, _ := s.gameRunner(ctx)
	runnerName := ""
	if runner != "" {
		runnerName = strings.TrimSuffix(filepath.Base(runner), ".exe")
	}
	enc := []string{}
	for _, n := range []string{"libx264", "libopenh264", "libvpx", "libopus"} {
		if s.Tools.Encoders[n] {
			enc = append(enc, n)
		}
	}
	list, _ := s.Tools.HardwareStatus()
	for _, e := range list {
		if e.Working {
			enc = append(enc, e.Name)
		}
	}
	views := s.consoleViews(ctx)
	counts := s.gameCountsEverywhere(ctx)
	systemDir := g.systemDir()
	s.sweepBIOS(false) // what's found shows the next time this is asked
	records := g.biosRecords()
	for i := range views {
		v := &views[i]
		v.TotalGames = counts[v.ID]
		for _, b := range v.BIOS {
			if _, err := os.Stat(filepath.Join(systemDir, filepath.FromSlash(b.File))); err == nil || (b.Required && biosRuleMet(&v.Console, systemDir)) {
				v.BiosFound = append(v.BiosFound, b.File)
			}
		}
		v.BiosFiles = biosFilesFor(v.ID, systemDir, records, defaultBIOS(&v.Console, systemDir))
		if len(v.Installed) > 0 && v.Reason != "gpu" { // a BIOS wouldn't help
			console := v.Console
			if core, blocked := g.cores.Playable(&console, v.DefaultCore, systemDir); core == nil && blocked != nil {
				v.Blocked = wittyplay.MissingBIOS(blocked.ID, systemDir)
			}
		}
	}
	writeJSON(w, 200, map[string]any{
		"runner": runnerName, "encoders": enc, "consoles": views, "cores": g.cores.List(), "canDownload": cores.CanDownload(),
		"coresDir": g.cores.Dir, "systemDir": g.systemDir(), "running": g.mgr.Count(), "unpackedBytes": g.archives.Used(), "gpu": s.gpuStatus(runner), "biosFolder": wittyplay.BIOSFolderName,
	})
}

// handleUnpackedClear empties the unpacked games cache, apart from what's being played.
func (s *Server) handleUnpackedClear(w http.ResponseWriter, r *http.Request) {
	g := s.games()
	g.archives.Clear()
	writeJSON(w, 200, map[string]any{"unpackedBytes": g.archives.Used()})
}

var coreIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func (s *Server) handleCoreInstall(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !coreIDRe.MatchString(id) || !cores.Offered(id) {
		writeErr(w, 404, "this server does not offer that core")
		return
	}
	// Not published for this platform: said plainly, with what to use instead, rather than
	// found out from the download server's 404.
	if has, known := coreBuildFor(id, runtime.GOOS, runtime.GOARCH); known && !has {
		writeJSON(w, 409, noBuildRefusal(offeredCoreName(id), s.alternativeCore(r.Context(), id)))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	c, err := s.games().cores.Install(ctx, id)
	if err != nil {
		s.Log.WarnContext(r.Context(), "could not install a core", "cat", "games", "core", id, "err", err)
		if strings.Contains(err.Error(), "404") {
			// The table said it was there; the download server says not now.
			out := noBuildRefusal(offeredCoreName(id), s.alternativeCore(r.Context(), id))
			out["error"] = offeredCoreName(id) + " isn't on the download server for " + platformWords(runtime.GOOS, runtime.GOARCH) + " right now."
			if alt, ok := out["alternative"].(*coreAlternative); ok {
				out["error"] = out["error"].(string) + " " + alt.Name + " plays " + alt.Console + " games here instead."
			}
			writeJSON(w, 502, out)
			return
		}
		writeErr(w, 502, "The core could not be installed: "+err.Error())
		return
	}
	// The cores offered come unsigned from libretro's build server, and Windows' Smart App Control
	// wouldn't let one load: taken out again, with the reason, rather than left to fail a game.
	lib := filepath.Join(s.games().cores.Dir, id+"_libretro.dll")
	if runtime.GOOS == "windows" {
		if err := appcontrol.CheckInstall(lib, "this core", "Signed game cores are planned; until then, games can't start on a PC with Smart App Control on."); err != nil {
			_ = s.games().cores.Remove(id)
			s.Log.WarnContext(r.Context(), "core refused: Smart App Control", "cat", "games", "core", id)
			writeErr(w, 409, err.Error())
			return
		}
	}
	s.Log.InfoContext(r.Context(), "core installed", "cat", "games", "core", id, "sha256", c.SHA256)
	writeJSON(w, 200, c)
}

func (s *Server) handleCoreRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !coreIDRe.MatchString(id) {
		writeErr(w, 404, "not found")
		return
	}
	if err := s.games().cores.Remove(id); err != nil {
		writeErr(w, 404, "this core isn't one this server installed")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// Hijack lets a WebSocket through when request logging is on.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}
