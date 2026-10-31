package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
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

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/gamearchive"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittyplay"
	"github.com/wittywilla/wittyplay/cores"
	"github.com/wittywilla/wittyplay/ipc"
	"github.com/wittywilla/wittyplay/stream"
)

// Games: the API around WittyPlay. Sessions and streaming live in the module;
// this file checks who may do what, and turns an item into a game to run. See
// docs/GAMING.md.

func init() {
	settingKeys = append(settingKeys, "gamingMaxSessions", "gamingUdpPort", "gamingPublicIp", "gamingStun", "gamingRunner", "gamingCores", "gamingQuality", "gamingUnpackGB", "gamingMaxGpuSessions", "gamingResolution", "gamingMaxSpeed", "gamingRewindMB", "gamingRunAhead")
	settingDefaults["gamingMaxSessions"] = "2"
	settingDefaults["gamingMaxGpuSessions"] = "2"
	settingDefaults["gamingRunner"] = "auto"
	settingDefaults["gamingUnpackGB"] = strconv.Itoa(defaultUnpackGB)
}

// gameRoutes registers the games API.
func (s *Server) gameRoutes(m *http.ServeMux) {
	a := s.requireUser
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
	m.Handle("POST /api/play/{sid}/action", a(s.handlePlayAction))
	s.gameProfileRoutes(m)
	m.Handle("GET /api/gaming", s.requireAdmin(s.handleGaming))
	m.Handle("POST /api/gaming/cores/{id}/install", s.requireAdmin(s.handleCoreInstall))
	m.Handle("DELETE /api/gaming/cores/{id}", s.requireAdmin(s.handleCoreRemove))
	m.Handle("DELETE /api/gaming/unpacked", s.requireAdmin(s.handleUnpackedClear))
	s.gameOptionRoutes(m)
	s.customRoutes(m)
}

// gameService is built the first time anything needs it, so a server nobody
// plays games on spends nothing on them.
type gameService struct {
	mgr      *stream.Manager
	cores    *cores.Manager
	archives *gamearchive.Cache
	dir      string

	gpuMu       sync.Mutex
	gpu         stream.GPUProbe
	gpuChecking bool

	custom *customService
}

// gpuStatus is what's known about the graphics card. Finding out takes a second or
// two, or half a minute if a security scanner holds the runner's first start, so it
// happens in the background and nothing waits for it. An answer about the card is
// final; a failure to ask gets asked again next time.
func (g *gameService) gpuStatus(runner string, log *slog.Logger) stream.GPUProbe {
	g.gpuMu.Lock()
	defer g.gpuMu.Unlock()
	if runner == "" || g.gpuChecking || (g.gpu.Name != "" || (g.gpu.Error != "" && !g.gpu.Again)) {
		out := g.gpu
		out.Checking = g.gpuChecking
		return out
	}
	g.gpuChecking = true
	go func() {
		got := stream.ProbeGPU(runner, filepath.Join(g.dir, "probe"), log)
		g.gpuMu.Lock()
		defer g.gpuMu.Unlock()
		g.gpu, g.gpuChecking = got, false
	}()
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
		s.gameSvc = &gameService{dir: dir, cores: &cores.Manager{Dir: coresDir, System: filepath.Join(dir, "system"), Client: library.SafeClient, Custom: s.customCoreList},
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

// findProgram looks beside the server, in bin/ and in the development build folder.
func findProgram(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dirs := []string{config.ProgramDir(), filepath.Join(config.ProgramDir(), "bin"), ".build"}
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
// which is what lets a GPL program be used by this Apache-licensed one. A
// downloaded version of the host comes before the installed one, and version says
// which it is; asking checks its signature, so this is asked again for each game.
func (s *Server) gameRunner(ctx context.Context) (path string, args []string, version string) {
	if p := os.Getenv("WMS_WITTYPLAY_RUNNER"); p != "" {
		return p, nil, ""
	}
	choice := s.DB.GetSetting(ctx, "gamingRunner", "auto")
	if choice == "auto" || choice == "retroarch" {
		if p := findProgram("wittyretroarch"); p != "" {
			return p, []string{"--wittyplay"}, ""
		}
	}
	if s.Modules != nil {
		if p, a, ver, ok := s.Modules.ProgramCommand(wittyPlayID); ok {
			return p, a, ver
		}
	}
	return findProgram("wittyplay-host"), nil, ""
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

// runnerFailed says a game couldn't start because of the runner itself: it wouldn't
// start, it went away before the game was ready, or it spoke nonsense. A runner that
// says what is wrong is working, since that is usually a core or a game file.
func runnerFailed(err error) bool {
	var pe *fs.PathError
	msg := err.Error()
	return errors.Is(err, stream.ErrStopped) || (errors.As(err, &pe) && pe.Op == "write") ||
		strings.HasPrefix(msg, "starting the game runner") || strings.HasPrefix(msg, "ipc:") || strings.HasPrefix(msg, "the runner described")
}

// configureGames sets how the next game runs, with no restart needed for a new
// version of the runner. It returns which downloaded version that is, if any.
func (s *Server) configureGames(ctx context.Context) (version string, err error) {
	g := s.games()
	runner, args, version := s.gameRunner(ctx)
	if runner == "" {
		return "", errors.New("the game runner (wittyplay-host) is missing from this install")
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
	Custom      []cores.Core        `json:"customCores,omitempty"`
	DefaultCore string              `json:"defaultCore,omitempty"`
	Ready       bool                `json:"ready"`
	Pad         wittyplay.PadLayout `json:"pad"`
	// Count: with ?library=<id>, how many games that library holds for this console.
	Count int `json:"count,omitempty"`
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
	for _, c := range wittyplay.All() {
		if c.Hidden && !testing {
			continue
		}
		v := consoleView{Console: c, Installed: []cores.Core{}, Pad: wittyplay.Pad(c.ID)}
		if v.Cores == nil {
			v.Cores = []wittyplay.Core{}
		}
		for _, want := range c.Cores {
			if got, ok := byID[want.ID]; ok {
				v.Installed = append(v.Installed, got)
			}
			if wittyplay.Traits(want.ID).GPU {
				v.GPUCores = append(v.GPUCores, want.ID)
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
		out = append(out, v)
	}
	return out
}

func (s *Server) handleGameConsoles(w http.ResponseWriter, r *http.Request) {
	views := s.consoleViews(r.Context())
	if !userFrom(r).IsAdmin {
		for i := range views {
			views[i].Custom, views[i].DefaultCore = nil, ""
		}
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
		"saved":      map[string]any{"gamepad": gamepads, "keyboard": keyboard, "touch": touch},
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
	default:
		writeErr(w, 400, "unknown kind of layout")
		return
	}
	var clean json.RawMessage
	if len(b.Data) > 0 && string(b.Data) != "null" {
		var err error
		if clean, err = wittyplay.CleanControl(id, b.Kind, b.Data); err != nil {
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
	if console.ID != wittyplay.TestConsole {
		// Goes through preferredCore: it's what keeps a file nobody approved from being used.
		preferred := s.preferredCore(ctx, console, it.ID)
		// BIOS files left in a folder beside the games are fetched first, for
		// every core that might use one.
		for _, want := range console.Cores {
			if err := wittyplay.CollectBIOS(console, want.ID, it.Path, systemDir); err != nil {
				s.Log.Warn("copying a BIOS", "console", console.ID, "err", err)
			}
		}
		core, blocked := g.cores.Playable(console, preferred, systemDir)
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
		gpu = wittyplay.Traits(core.ID).GPU || (core.Source == cores.SourceCustom && core.WantsGPU)
		runnerArgs = wittyplay.RunnerArgs(core.ID)
		resolution := s.DB.GetSetting(ctx, "gamingResolution", "")
		coreOptions = wittyplay.StartOptions(console.ID, core.ID, resolution, systemDir)
		// If the game won't load on this core, the others get their turn.
		for _, other := range g.cores.AllForConsole(console, core.ID)[1:] {
			// A custom core only ever runs where an administrator assigned it, and after its check.
			if other.Source != cores.SourceCustom && wittyplay.MissingBIOS(other.ID, systemDir) == "" {
				fallbacks = append(fallbacks, stream.CoreChoice{Core: other.Path, GPU: wittyplay.Traits(other.ID).GPU, Options: wittyplay.StartOptions(console.ID, other.ID, resolution, systemDir)})
			}
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
	if _, err := os.Stat(it.Path); err != nil {
		writeErr(w, 404, "the game file is missing")
		return
	}
	rom, cleanup, err := g.prepareROM(ctx, it.Path, console)
	if err != nil {
		if ctx.Err() != nil {
			return // whoever asked has gone; the unpack carries on for next time
		}
		writeErr(w, 422, err.Error())
		return
	}
	rom, cleanup, err = g.prepare3DS(rom, cleanup, console)
	if err != nil {
		if errors.Is(err, wittyplay.ErrEncrypted3DS) {
			writeJSON(w, 422, map[string]any{"error": err.Error(), "encrypted": true, "console": console.ID})
			return
		}
		writeErr(w, 422, err.Error())
		return
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
			problem = "This is a Satellaview broadcast game, which only plays on Mesen-S. An admin can install that core in Settings, Gaming."
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
	sess, err := g.mgr.Start(opts)
	if err != nil {
		if opts.Cleanup != nil {
			opts.Cleanup() // it may never have got as far as a session that would
		}
		if errors.Is(err, stream.ErrBusy) || errors.Is(err, stream.ErrGPUBusy) {
			writeErr(w, 503, err.Error())
			return
		}
		if strings.Contains(err.Error(), stream.NoGPUText) {
			writeErr(w, 409, "This game draws with a graphics card, and this server doesn't have one it can use. Games for older consoles still play.")
			return
		}
		s.Log.WarnContext(r.Context(), "could not start a game", "cat", "games", "item", it.ID, "game", it.Title, "err", err)
		if version != "" && runnerFailed(err) {
			_ = s.Modules.ReportFailure(wittyPlayID, version, "the game runner stopped before the game was ready: "+err.Error())
		}
		if playing != nil {
			s.noteCorePlay(ctx, playing.ID, it.ID, 1, 1, 0, err.Error())
			if playing.Source == cores.SourceCustom {
				s.auditCore(ctx, userFrom(r), "use-failed", playing.ID, fmt.Sprintf("item %d: %v", it.ID, err))
			}
		}
		writeErr(w, 500, "This game could not start. "+err.Error())
		return
	}
	if version != "" {
		s.Modules.ReportWorking(wittyPlayID, version)
	}
	logging.Go("game play time", func() { s.recordPlayTime(sess, userID, it.ID) }, "cat", "games", "session", sess.ID())
	if playing != nil {
		s.noteCorePlay(ctx, playing.ID, it.ID, 1, 0, 0, "")
		watcher := userFrom(r)
		logging.Go("game core watch", func() { s.watchCorePlay(sess, playing, watcher, it.ID) }, "cat", "games", "session", sess.ID())
	}
	if err := s.Store.NoteGamePlayed(ctx, userID, it.ID); err != nil {
		s.Log.Warn("recording a play", "err", err)
	}
	info := sess.Info()
	info.Modes = g.mgr.Modes(gameClient(b.Client))
	writeJSON(w, 201, info)
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

// prepareROM returns the file a core should open. An archive is unpacked first, or
// already has been: see gamearchive.Cache. This waits for that, and cleanup lets the
// cache have the space back once the game has ended.
func (g *gameService) prepareROM(ctx context.Context, path string, console *wittyplay.Console) (rom string, cleanup func(), err error) {
	if !gamearchive.IsArchive(path) {
		return path, nil, nil
	}
	return g.archives.Acquire(ctx, path, console)
}

// prepare3DS checks a 3DS file before a core sees it. An encrypted one is refused:
// nothing here decrypts or looks for keys. An unencrypted install package holds the
// game as a plain program, which is copied out because the cores can't open a package.
// On failure the earlier cleanup has already run.
func (g *gameService) prepare3DS(rom string, cleanup func(), console *wittyplay.Console) (string, func(), error) {
	if console.ID != "3ds" {
		return rom, cleanup, nil
	}
	fail := func(err error) (string, func(), error) {
		if cleanup != nil {
			cleanup()
		}
		return "", nil, err
	}
	info, err := wittyplay.Inspect3DS(rom)
	if err != nil {
		return fail(err)
	}
	if info.Encrypted {
		return fail(wittyplay.ErrEncrypted3DS)
	}
	if !info.Package {
		return rom, cleanup, nil
	}
	tmpRoot := filepath.Join(g.dir, "tmp")
	if err := os.MkdirAll(tmpRoot, 0o700); err != nil {
		return fail(err)
	}
	dir, err := os.MkdirTemp(tmpRoot, "rom-")
	if err != nil {
		return fail(err)
	}
	both := func() {
		_ = os.RemoveAll(dir)
		if cleanup != nil {
			cleanup()
		}
	}
	out := filepath.Join(dir, "game.cxi")
	if err := info.CopyProgram(rom, out); err != nil {
		both()
		return "", nil, err
	}
	return out, both, nil
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
	ready := gamearchive.Status{State: "ready"}
	if !gamearchive.IsArchive(it.Path) {
		writeJSON(w, 200, ready)
		return
	}
	// Unpacking a disc only to be told no core is installed would be a poor way to learn it.
	if console.ID != wittyplay.TestConsole && g.cores.ForConsole(console, s.preferredCores(r.Context())[console.ID]) == nil {
		writeJSON(w, 409, map[string]any{"error": "No core is installed for " + console.Name + ".", "needCore": true, "console": console.ID})
		return
	}
	st := g.archives.Prepare(it.Path, console)
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
	sess := s.session(r)
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
	sess := s.session(r)
	if sess == nil {
		writeErr(w, 404, "not found")
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
	sess := s.session(r)
	if sess == nil {
		writeErr(w, 404, "not found")
		return
	}
	sess.ServeWS(w, r, r.URL.Query().Get("video"))
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
	writeJSON(w, 200, map[string]any{
		"runner": runnerName, "encoders": enc, "consoles": s.consoleViews(ctx), "cores": g.cores.List(), "canDownload": cores.CanDownload(),
		"coresDir": g.cores.Dir, "systemDir": g.systemDir(), "running": g.mgr.Count(), "unpackedBytes": g.archives.Used(), "gpu": g.gpuStatus(runner, s.Log), "biosFolder": wittyplay.BIOSFolderName,
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
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	c, err := s.games().cores.Install(ctx, id)
	if err != nil {
		s.Log.WarnContext(r.Context(), "could not install a core", "cat", "games", "core", id, "err", err)
		writeErr(w, 502, "The core could not be installed: "+err.Error())
		return
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
