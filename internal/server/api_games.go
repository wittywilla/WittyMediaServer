package server

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittyplay"
	"github.com/wittywilla/wittyplay/cores"
	"github.com/wittywilla/wittyplay/ipc"
	"github.com/wittywilla/wittyplay/stream"
)

// Games: the API around WittyPlay. Sessions and streaming live in the module;
// this file checks who may do what, and turns an item into a game to run. See
// docs/GAMING.md.

func init() {
	settingKeys = append(settingKeys, "gamingMaxSessions", "gamingUdpPort", "gamingPublicIp", "gamingStun", "gamingRunner", "gamingCores", "gamingQuality", "gamingMaxGpuSessions", "gamingResolution")
	settingDefaults["gamingMaxSessions"] = "2"
	settingDefaults["gamingMaxGpuSessions"] = "2"
	settingDefaults["gamingRunner"] = "auto"
}

// gameRoutes registers the games API.
func (s *Server) gameRoutes(m *http.ServeMux) {
	a := s.requireUser
	m.Handle("GET /api/games/consoles", a(s.handleGameConsoles))
	m.Handle("GET /api/games/modes", a(s.handleGameModes))
	m.Handle("GET /api/consoles/{console}/controls", a(s.handleGameControls))
	m.Handle("PUT /api/consoles/{console}/controls", a(s.handlePutGameControl))
	m.Handle("POST /api/games/{id}/play", a(s.handleGamePlay))
	m.Handle("GET /api/games/{id}/saves", a(s.handleGameSaves))
	m.Handle("GET /api/games/{id}/saves/{slot}/thumb", a(s.handleGameSaveThumb))
	m.Handle("DELETE /api/games/{id}/saves/{slot}", a(s.handleGameSaveDelete))
	m.Handle("GET /api/play/{sid}", a(s.handlePlayGet))
	m.Handle("DELETE /api/play/{sid}", a(s.handlePlayStop))
	m.Handle("POST /api/play/{sid}/webrtc", a(s.handlePlayWebRTC))
	m.Handle("GET /api/play/{sid}/ws", a(s.handlePlayWS))
	m.Handle("POST /api/play/{sid}/action", a(s.handlePlayAction))
	m.Handle("GET /api/gaming", s.requireAdmin(s.handleGaming))
	m.Handle("POST /api/gaming/cores/{id}/install", s.requireAdmin(s.handleCoreInstall))
	m.Handle("DELETE /api/gaming/cores/{id}", s.requireAdmin(s.handleCoreRemove))
}

// gameService is built the first time anything needs it, so a server nobody
// plays games on spends nothing on them.
type gameService struct {
	mgr   *stream.Manager
	cores *cores.Manager
	dir   string

	gpuOnce sync.Once
	gpu     stream.GPUProbe
}

// systemDir is the folder cores read BIOS files and their support files from.
func (g *gameService) systemDir() string { return filepath.Join(g.dir, "system") }

// testingGames: the built-in test console is only offered to developers.
func (s *Server) testingGames() bool {
	return (s.Cfg != nil && s.Cfg.Dev) || os.Getenv("WMS_GAMING_TESTCORE") == "1"
}

func (s *Server) games() *gameService {
	s.gameOnce.Do(func() {
		dir := filepath.Join(s.Cfg.DataDir, "gaming")
		s.gameSvc = &gameService{dir: dir, cores: &cores.Manager{Dir: filepath.Join(dir, "cores"), System: filepath.Join(dir, "system"), Client: library.SafeClient},
			mgr: stream.NewManager(stream.Config{FFmpeg: s.Tools.FFmpeg, HaveEncoder: func(n string) bool { return s.Tools.Encoders[n] }, Log: s.Log})}
	})
	return s.gameSvc
}

// StopGames saves and ends every game; the server is shutting down.
func (s *Server) StopGames() {
	if s.gameSvc != nil {
		s.gameSvc.mgr.StopAll()
	}
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

// gameRunner picks the program that runs a game: the built-in host, or
// WittyRetroArch when it is installed. Both speak the same protocol over pipes,
// which is what lets a GPL program be used by this Apache-licensed one.
func (s *Server) gameRunner(ctx context.Context) (path string, args []string) {
	if p := os.Getenv("WMS_WITTYPLAY_RUNNER"); p != "" {
		return p, nil
	}
	choice := s.DB.GetSetting(ctx, "gamingRunner", "auto")
	if choice == "auto" || choice == "retroarch" {
		if p := findProgram("wittyretroarch"); p != "" {
			return p, []string{"--wittyplay"}
		}
	}
	return findProgram("wittyplay-host"), nil
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

func (s *Server) configureGames(ctx context.Context) error {
	g := s.games()
	runner, args := s.gameRunner(ctx)
	if runner == "" {
		return errors.New("the game runner (wittyplay-host) is missing from this install")
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
	return nil
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
	Installed []cores.Core        `json:"installed"`
	NeedsGPU  bool                `json:"needsGpu,omitempty"` // every core on offer draws with a graphics card
	GPUCores  []string            `json:"gpuCores,omitempty"`
	Ready     bool                `json:"ready"`
	Pad       wittyplay.PadLayout `json:"pad"`
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
	for _, c := range wittyplay.Consoles {
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
		// A core someone placed themselves shows up under every console it could
		// serve only when we know it; anything else is chosen by name in Settings.
		v.Ready = c.Unavailable == "" && (len(v.Installed) > 0 || c.ID == wittyplay.TestConsole)
		out = append(out, v)
	}
	return out
}

func (s *Server) handleGameConsoles(w http.ResponseWriter, r *http.Request) {
	views := s.consoleViews(r.Context())
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
	_ = s.configureGames(r.Context()) // so that hardware encoders count
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
	saved, err := s.Store.GameControls(r.Context(), userFrom(r).ID, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
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
		"defaults": map[string]any{
			"gamepad": wittyplay.DefaultGamepad(), "keyboard": wittyplay.DefaultKeyboard(), "remote": wittyplay.DefaultRemote(),
			"touch": map[string]any{wittyplay.Landscape: wittyplay.DefaultTouch(id, wittyplay.Landscape), wittyplay.Portrait: wittyplay.DefaultTouch(id, wittyplay.Portrait)},
		},
		"saved": map[string]any{"gamepad": gamepads, "keyboard": keyboard, "touch": touch},
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
		Resume bool   `json:"resume"`
		Client string `json:"client"` // browser, or anything else for a native app
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	console := wittyplay.ByID(it.Console)
	if console == nil || (console.Hidden && !s.testingGames()) {
		writeErr(w, 404, "not found")
		return
	}
	if console.Unavailable != "" {
		writeErr(w, 409, console.Unavailable)
		return
	}
	ctx := r.Context()
	g := s.games()
	systemDir := g.systemDir()
	if err := os.MkdirAll(systemDir, 0o755); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	corePath := ipc.TestCore
	var coreOptions map[string]string
	gpu := false
	if console.ID != wittyplay.TestConsole {
		preferred := s.preferredCores(ctx)[console.ID]
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
			writeJSON(w, 409, map[string]any{"error": "No core is installed for " + console.Name + ".", "needCore": true, "console": console.ID})
			return
		}
		corePath = core.Path
		gpu = wittyplay.Traits(core.ID).GPU
		coreOptions = wittyplay.CoreOptions(console.ID, core.ID, s.DB.GetSetting(ctx, "gamingResolution", ""))
	}
	if gpu {
		s.awaitHardwareProbe(ctx)
	}
	if err := s.configureGames(ctx); err != nil {
		writeErr(w, 503, err.Error())
		return
	}
	if _, err := os.Stat(it.Path); err != nil {
		writeErr(w, 404, "the game file is missing")
		return
	}
	rom, cleanup, err := g.prepareROM(it.Path, console)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	userID := userFrom(r).ID
	sess, err := g.mgr.Start(stream.Options{
		UserID: userID, ItemID: it.ID, Title: it.Title, Console: console.ID, Core: corePath, ROM: rom, SystemDir: systemDir,
		SaveDir: g.saveDir(userID, it.ID), Resume: b.Resume, Quality: s.DB.GetSetting(ctx, "gamingQuality", ""), Cleanup: cleanup,
			GPU: gpu, CoreOptions: coreOptions,
	})
	if err != nil {
		if errors.Is(err, stream.ErrBusy) || errors.Is(err, stream.ErrGPUBusy) {
			writeErr(w, 503, err.Error())
			return
		}
		if strings.Contains(err.Error(), stream.NoGPUText) {
			writeErr(w, 409, "This game draws with a graphics card, and this server doesn't have one it can use. Games for older consoles still play.")
			return
		}
		s.Log.Warn("starting a game", "item", it.ID, "err", err)
		writeErr(w, 500, "This game could not start: "+err.Error())
		return
	}
	go s.recordPlayTime(sess, userID, it.ID)
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
		s.Log.Warn("recording play time", "err", err)
	}
}

// prepareROM returns the file a core should open. A .zip is unpacked first, into
// a folder only this session uses, under a name we choose.
func (g *gameService) prepareROM(path string, console *wittyplay.Console) (rom string, cleanup func(), err error) {
	if !strings.EqualFold(filepath.Ext(path), ".zip") {
		return path, nil, nil
	}
	entry := library.ZipGame(path, console)
	if entry == "" {
		return "", nil, errors.New("this archive holds no game for " + console.Name)
	}
	tmpRoot := filepath.Join(g.dir, "tmp")
	if err := os.MkdirAll(tmpRoot, 0o700); err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp(tmpRoot, "rom-")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	out := filepath.Join(dir, "game"+strings.ToLower(filepath.Ext(entry)))
	if err := extractZipEntry(path, entry, out); err != nil {
		cleanup()
		return "", nil, err
	}
	return out, cleanup, nil
}

// maxUnpackedROM is above the largest cartridge; disc images aren't zipped this way.
const maxUnpackedROM = 1 << 30

func extractZipEntry(path, entry, out string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != entry {
			continue
		}
		if f.UncompressedSize64 > maxUnpackedROM {
			return errors.New("the game in this archive is too large")
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		dst, err := os.Create(out)
		if err != nil {
			return err
		}
		n, err := io.Copy(dst, io.LimitReader(rc, maxUnpackedROM+1))
		if cerr := dst.Close(); err == nil {
			err = cerr
		}
		if err == nil && n > maxUnpackedROM {
			err = errors.New("the game in this archive is too large")
		}
		return err
	}
	return errors.New("the game is not in the archive")
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
		s.Log.Warn("webrtc", "err", err)
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
		Type string `json:"type"`
		Slot int    `json:"slot"`
		On   bool   `json:"on"`
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
	Slot      int   `json:"slot"`
	UpdatedAt int64 `json:"updatedAt"`
	Size      int64 `json:"size"`
	HasThumb  bool  `json:"hasThumb"`
}

func (s *Server) handleGameSaves(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	dir := s.games().saveDir(userFrom(r).ID, it.ID)
	out := []saveSlot{}
	for slot := 0; slot <= 9; slot++ {
		name, _ := stream.SlotName(slot)
		st, err := os.Stat(filepath.Join(dir, name+".state"))
		if err != nil {
			continue
		}
		_, terr := os.Stat(filepath.Join(dir, name+".png"))
		out = append(out, saveSlot{Slot: slot, UpdatedAt: st.ModTime().Unix(), Size: st.Size(), HasThumb: terr == nil})
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
	runner, _ := s.gameRunner(ctx)
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
	g.gpuOnce.Do(func() {
		if runner != "" {
			g.gpu = stream.ProbeGPU(runner, g.dir, s.Log)
		}
	})
	writeJSON(w, 200, map[string]any{
		"runner": runnerName, "encoders": enc, "consoles": s.consoleViews(ctx), "cores": g.cores.List(), "canDownload": cores.CanDownload(),
		"coresDir": g.cores.Dir, "systemDir": g.systemDir(), "running": g.mgr.Count(), "gpu": g.gpu, "biosFolder": wittyplay.BIOSFolderName,
	})
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
		s.Log.Warn("installing a core", "core", id, "err", err)
		writeErr(w, 502, "The core could not be installed: "+err.Error())
		return
	}
	s.Log.Info("core installed", "core", id, "sha256", c.SHA256)
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
