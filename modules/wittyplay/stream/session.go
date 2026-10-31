// Package stream runs game sessions on the server: it starts a runner process,
// reads its raw frames, encodes them with FFmpeg and sends them to viewers over
// WebRTC or a WebSocket. docs/GAMING.md has the reasoning.
package stream

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittyplay/ipc"
)

// Config is how the server runs games.
type Config struct {
	Runner      string   // the wittyplay-host program, or another runner speaking ipc
	RunnerArgs  []string // extra arguments, before the ones added here
	FFmpeg      string
	HWEncoders  []string          // hardware encoders known to work here, best first
	HaveEncoder func(string) bool // FFmpeg was built with this encoder
	MaxSessions int               // 0 means 2
	MemoryMB    int64             // 0 means 2048
	Log         *slog.Logger

	// WebRTC networking; all optional. A fixed UDP port is what a firewall or
	// a container needs; the public address is what NAT needs.
	UDPPort   int
	PublicIPs []string
	STUN      []string
}

// Options start one game.
type Options struct {
	UserID, ItemID int64
	Title          string
	Console        string
	Core           string // a shared library path, or ipc.TestCore
	ROM            string
	SystemDir      string
	SaveDir        string // this account's folder for this game
	Resume         bool   // continue from auto.state
	Quality        string // "" (auto) | low | high
	CoreOptions    map[string]string
	Cleanup        func() // runs when the session has ended
	// Fallbacks are cores to try, in order, if this game won't load on Core. Dumps
	// that one core refuses often play on another, and nobody should have to know.
	Fallbacks []CoreChoice

	keep *keptCleanup
}

// CoreChoice is a core and the settings it starts with.
type CoreChoice struct {
	Core    string
	Options map[string]string
}

// Info is what a browser needs to know about a session.
type Info struct {
	ID         string  `json:"id"`
	ItemID     int64   `json:"itemId"`
	Title      string  `json:"title"`
	Console    string  `json:"console"`
	Core       string  `json:"core"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Aspect     float64 `json:"aspect"`
	FPS        float64 `json:"fps"`
	SampleRate float64 `json:"sampleRate"`
	Resumed    bool    `json:"resumed"`
	Encoder    string  `json:"encoder,omitempty"`
	Turns      int     `json:"turns,omitempty"` // quarter turns anticlockwise to draw the picture
	Disc       int     `json:"disc,omitempty"`  // the disc in the drive, from 0
	Discs      int     `json:"discs,omitempty"` // how many the game has
	Modes      []Mode  `json:"modes"` // what this client can connect with, best first
}

// Errors a caller shows to people.
var (
	ErrBusy    = errors.New("this server is already running as many games as it allows; stop one first")
	ErrStopped = errors.New("the game has stopped")
	ErrNoSlot  = errors.New("there is no such save slot")
)

// Manager holds the running sessions.
type Manager struct {
	cfg Config
	log *slog.Logger

	mu       sync.Mutex
	sessions map[string]*Session
	badEnc   map[string]bool // encoders that failed to start here
	janitor  sync.Once
	udp      *udpShared
}

// NewManager makes a manager; sessions start with Start.
func NewManager(cfg Config) *Manager {
	if cfg.MaxSessions <= 0 {
		cfg.MaxSessions = 2
	}
	if cfg.MemoryMB <= 0 {
		cfg.MemoryMB = 2048
	}
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.HaveEncoder == nil {
		cfg.HaveEncoder = func(string) bool { return true }
	}
	return &Manager{cfg: cfg, log: cfg.Log, sessions: map[string]*Session{}, badEnc: map[string]bool{}}
}

// Configure replaces the settings new sessions use.
func (m *Manager) Configure(update func(*Config)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	update(&m.cfg)
}

func (m *Manager) config() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// Get finds a session.
func (m *Manager) Get(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// Count says how many games are running.
func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

// StopAll ends every session, saving each one. Used at shutdown.
func (m *Manager) StopAll() {
	m.mu.Lock()
	all := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, s := range all {
		wg.Add(1)
		go func() { defer wg.Done(); s.Stop() }()
	}
	wg.Wait()
}

// Start launches a game. An account plays one game at a time: starting another
// saves and ends the first, on any device.
func (m *Manager) Start(opt Options) (*Session, error) {
	m.janitor.Do(func() { go m.sweep() })
	m.mu.Lock()
	var mine []*Session
	for _, s := range m.sessions {
		if s.opt.UserID == opt.UserID {
			mine = append(mine, s)
		}
	}
	m.mu.Unlock()
	for _, s := range mine {
		s.Stop()
	}

	s, err := m.startOn(opt, len(opt.Fallbacks) > 0)
	if err == nil || errors.Is(err, ErrBusy) {
		return s, err
	}
	// What the core they would have chosen said is the error worth showing.
	first := err
	for i, next := range opt.Fallbacks {
		m.log.Info("the game would not load; trying the next core", "game", opt.Title, "err", err)
		try := opt
		try.Core, try.CoreOptions = next.Core, next.Options
		last := i == len(opt.Fallbacks)-1
		if s, err = m.startOn(try, !last); err == nil {
			return s, nil
		}
		if errors.Is(err, ErrBusy) {
			if !last && opt.Cleanup != nil {
				opt.Cleanup()
			}
			return nil, err
		}
	}
	return nil, first
}

// startOn starts a game on one core. With more cores to try, a failure leaves the
// unpacked game in place for the next.
func (m *Manager) startOn(opt Options, moreToTry bool) (*Session, error) {
	cfg := m.config()
	m.mu.Lock()
	if len(m.sessions) >= cfg.MaxSessions {
		m.mu.Unlock()
		return nil, ErrBusy
	}
	if moreToTry {
		final := opt.Cleanup
		kept := &keptCleanup{run: final}
		opt.Cleanup = kept.onEnd
		opt.keep = kept
	}
	id := newID()
	s := &Session{id: id, m: m, opt: opt, cfg: cfg, log: m.log.With("game", opt.Title, "session", id[:8]), done: make(chan struct{}), paused: true,
		viewers: map[*viewer]bool{}, stateWait: map[uint32]chan []byte{}, frameReady: make(chan struct{}, 1), lastInput: time.Now(), noViewerSince: time.Now(),
		audioCh: make(chan []byte, 32), stderr: &tailWriter{max: 1 << 16}}
	m.sessions[id] = s
	m.mu.Unlock()
	if err := s.start(); err != nil {
		m.remove(s)
		s.cleanup()
		return nil, err
	}
	if opt.keep != nil {
		opt.keep.commit()
	}
	return s, nil
}

// keptCleanup holds a session's cleanup back until the session is known to have
// started, so that a failed attempt doesn't remove what the next one needs.
type keptCleanup struct {
	mu        sync.Mutex
	run       func()
	committed bool
	ended     bool
}

func (k *keptCleanup) onEnd() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.ended = true
	if k.committed && k.run != nil {
		k.run()
		k.run = nil
	}
}

func (k *keptCleanup) commit() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.committed = true
	if k.ended && k.run != nil {
		k.run()
		k.run = nil
	}
}

func (m *Manager) remove(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, s.id)
}

const (
	noViewerLimit = 60 * time.Second
	idleLimit     = 30 * time.Minute
	stuckLimit    = 30 * time.Second
)

// sweep ends games nobody is watching or playing, because an abandoned tab must
// not keep a low-end server busy.
func (m *Manager) sweep() {
	for range time.Tick(10 * time.Second) {
		m.mu.Lock()
		all := make([]*Session, 0, len(m.sessions))
		for _, s := range m.sessions {
			all = append(all, s)
		}
		m.mu.Unlock()
		for _, s := range all {
			if s.idleTooLong() {
				s.log.Info("stopping an unattended game")
				go s.Stop()
			} else if s.stuck() {
				s.log.Warn("the game has stopped making frames; ending it")
				s.exitErr.CompareAndSwap(nil, "The emulator stopped responding while running this game. Another core may work; you can choose one in Settings, Gaming.")
				s.kill()
			}
		}
	}
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Session is one running game.
type Session struct {
	id  string
	m   *Manager
	opt Options
	cfg Config
	log *slog.Logger

	cmd     *exec.Cmd
	box     func() // releases the process's sandbox resources
	stdinMu sync.Mutex
	stdin   io.WriteCloser
	info    ipc.Ready
	resumed bool

	done      chan struct{} // closed when the runner has exited
	stopOnce  sync.Once
	cleanOnce sync.Once
	exitErr   atomic.Value

	frameMu    sync.Mutex
	frame      []byte // the newest frame
	frameHdr   ipc.VideoHeader
	frameReady chan struct{}

	viewMu        sync.Mutex
	viewers       map[*viewer]bool
	paused        bool // what the runner was last told; it starts paused
	menuPaused    bool // someone has a menu open
	noViewerSince time.Time
	lastInput     time.Time
	lastPad       [MaxPlayers][ipc.InputSize]byte
	h264, vp8     *videoEncoder
	opus          *audioEncoder
	jpeg          *jpegEncoder
	encName       string
	audioCh       chan []byte
	rate          float64
	turns         int

	stopping  atomic.Bool  // Stop was called, so the runner ending is no surprise
	lastFrame atomic.Int64 // when the runner last made a frame, in Unix nanoseconds
	stderr    *tailWriter

	stateMu   sync.Mutex
	stateWait map[uint32]chan []byte
	stateNext uint32
	wrote     sync.WaitGroup
}

// ID is the session's unguessable name.
func (s *Session) ID() string { return s.id }

// UserID is the account that started it.
func (s *Session) UserID() int64 { return s.opt.UserID }

// ItemID is the game being played.
func (s *Session) ItemID() int64 { return s.opt.ItemID }

// Info describes the session for a browser.
func (s *Session) Info() Info {
	s.viewMu.Lock()
	defer s.viewMu.Unlock()
	return Info{ID: s.id, ItemID: s.opt.ItemID, Title: s.opt.Title, Console: s.opt.Console, Modes: s.m.Modes(ClientNative), Core: s.info.Name, Width: s.info.Width, Height: s.info.Height,
		Aspect: s.info.Aspect, FPS: s.info.FPS, SampleRate: s.rate, Resumed: s.resumed, Encoder: s.encName, Turns: s.turns, Disc: s.info.Disc, Discs: s.info.Discs}
}

func (s *Session) start() error {
	if err := os.MkdirAll(s.opt.SaveDir, 0o700); err != nil {
		return err
	}
	rn, err := StartRunner(s.cfg.Runner, s.cfg.RunnerArgs, s.opt.SaveDir, s.cfg.MemoryMB, s.log)
	if err != nil {
		return err
	}
	cmd, stdout, stderr := rn.Cmd, rn.Stdout, rn.Stderr
	s.cmd, s.stdin, s.box = cmd, rn.Stdin, rn.Release // boxed in before any core is loaded
	s.wrote.Add(2)
	go s.logStderr(stderr)

	// Give up on a game that won't load, however it fails.
	readyCh := make(chan error, 1)
	go s.readRunner(stdout, readyCh)
	go s.audioLoop()
	go s.videoLoop()
	go func() {
		// Wait closes the pipes, so it comes after the last save has been read.
		s.wrote.Wait()
		if err := cmd.Wait(); err != nil && s.exitErr.Load() == nil {
			s.log.Warn("the runner ended unexpectedly", "err", err)
			s.exitErr.Store(describeExit(s.stderr.String()))
		}
		s.finish()
		close(s.done)
	}()

	load, _ := json.Marshal(ipc.Load{Core: s.opt.Core, ROM: s.opt.ROM, System: s.opt.SystemDir, Save: s.opt.SaveDir, Options: s.opt.CoreOptions})
	if err := s.command(ipc.CmdLoad, load); err != nil {
		s.kill()
		return err
	}
	select {
	case err := <-readyCh:
		if err != nil {
			s.kill()
			<-s.done
			if v := s.exitErr.Load(); v != nil { // it knows why by now
				return errors.New(v.(string))
			}
			return err
		}
	case <-time.After(30 * time.Second):
		s.stopping.Store(true)
		s.kill()
		<-s.done
		return errors.New("the game took too long to load")
	}

	// Everything is restored before the first frame: the runner waits, paused.
	if b, err := os.ReadFile(filepath.Join(s.opt.SaveDir, "save.srm")); err == nil && len(b) > 0 {
		_ = s.command(ipc.CmdSaveRAM, b)
	}
	if s.opt.Resume {
		if b, err := readLimited(filepath.Join(s.opt.SaveDir, "auto.state")); err == nil && len(b) > 0 {
			_ = s.command(ipc.CmdState, b)
			s.resumed = true
		}
	}
	return nil
}

func readLimited(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Size() > ipc.MaxBlob {
		return nil, errors.New("the save state is too large")
	}
	return os.ReadFile(path)
}

// command sends a runner one message.
func (s *Session) command(t byte, parts ...[]byte) error {
	s.stdinMu.Lock()
	defer s.stdinMu.Unlock()
	return ipc.Write(s.stdin, t, parts...)
}

// logStderr keeps the end of what the runner says: cores can be chatty, and the
// last thing said before a crash is the part worth reading.
func (s *Session) logStderr(r io.Reader) {
	defer s.wrote.Done()
	_, _ = io.Copy(s.stderr, r)
	if text := s.stderr.String(); text != "" {
		s.log.Info("runner says", "output", text)
	}
}

// describeExit says in plain words why a runner ended by itself, from what it left
// on its error stream.
func describeExit(stderr string) string {
	switch {
	case strings.Contains(stderr, "out of memory"), strings.Contains(stderr, "cannot allocate memory"):
		return "The emulator ran out of memory running this game."
	case strings.Contains(stderr, "Exception 0xc000001d"), strings.Contains(stderr, "SIGILL"):
		return "The emulator crashed: it needs something this processor doesn't have. Another core may work; you can choose one in Settings, Gaming."
	}
	return "The emulator crashed while running this game. Another core may work; you can choose one in Settings, Gaming."
}

// readRunner handles everything the runner sends. Nothing here may block for
// long, or the game itself stalls behind the pipe.
func (s *Session) readRunner(r io.Reader, ready chan<- error) {
	defer s.wrote.Done()
	rd := ipc.NewReader(r)
	isReady := false
	fail := func(err error) {
		if !isReady {
			ready <- err
			isReady = true
		}
	}
	for {
		t, p, err := rd.Next()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
				fail(s.exitError())
			} else {
				s.log.Warn("the runner sent something invalid; stopping it", "err", err)
				fail(err)
				s.kill()
			}
			return
		}
		switch t {
		case ipc.MsgReady:
			var info ipc.Ready
			if err := json.Unmarshal(p, &info); err != nil || info.FPS <= 0 || info.FPS > 240 || info.SampleRate < 0 || info.SampleRate > 384000 {
				fail(errors.New("the runner described the game in a way that makes no sense"))
				s.kill()
				return
			}
			s.viewMu.Lock()
			s.info, s.rate = info, info.SampleRate
			s.viewMu.Unlock()
			isReady = true
			ready <- nil
		case ipc.MsgVideo:
			hdr, data, err := ipc.ParseVideo(p)
			if err != nil {
				s.log.Warn("bad video from the runner", "err", err)
				continue
			}
			s.frameMu.Lock()
			s.frame = append(s.frame[:0], data...)
			s.frameHdr = hdr
			s.frameMu.Unlock()
			s.lastFrame.Store(time.Now().UnixNano())
			s.signalFrame()
		case ipc.MsgDupe:
			s.lastFrame.Store(time.Now().UnixNano())
			s.signalFrame()
		case ipc.MsgAudio:
			if len(p)%4 == 0 {
				select {
				case s.audioCh <- append([]byte(nil), p...):
				default: // a stalled encoder drops sound rather than the game
				}
			}
		case ipc.MsgState:
			if len(p) >= 4 {
				s.deliverState(binary.LittleEndian.Uint32(p), p[4:])
			}
		case ipc.MsgSaveRAM:
			if err := writeAtomic(filepath.Join(s.opt.SaveDir, "save.srm"), p); err != nil {
				s.log.Warn("writing battery save", "err", err)
			}
		case ipc.MsgEvent:
			s.handleEvent(p)
		case ipc.MsgError:
			msg := string(p)
			if len(msg) > 500 {
				msg = msg[:500]
			}
			s.exitErr.Store(msg)
			s.log.Warn("the runner failed", "err", msg)
			fail(errors.New(msg))
		}
	}
}

func (s *Session) exitError() error {
	if v := s.exitErr.Load(); v != nil {
		return errors.New(v.(string))
	}
	return ErrStopped
}

func (s *Session) signalFrame() {
	select {
	case s.frameReady <- struct{}{}:
	default:
	}
}

func (s *Session) handleEvent(p []byte) {
	var e ipc.Event
	if err := json.Unmarshal(p, &e); err != nil {
		return
	}
	switch e.Type {
	case "log":
		s.log.Debug("core", "text", truncate(e.Text, 300))
		return
	case "shutdown":
		s.log.Info("the game shut itself down")
	case "geometry":
		s.noteGeometry(e)
	case "rotation":
		// Cores say this while loading, before anyone is watching.
		s.viewMu.Lock()
		s.turns = e.Turns & 3
		s.viewMu.Unlock()
	case "disc":
		s.viewMu.Lock()
		s.info.Disc, s.info.Discs = e.Index, e.Count
		s.viewMu.Unlock()
	case "avinfo":
		s.noteGeometry(e)
		if e.Rate > 0 && e.Rate <= 384000 {
			s.viewMu.Lock()
			if s.rate != e.Rate {
				s.rate = e.Rate
				s.stopOpusLocked()
				if s.anyWebRTCLocked() {
					s.startOpusLocked()
				}
			}
			s.viewMu.Unlock()
		}
	}
	e.Text = truncate(e.Text, 300)
	b, _ := json.Marshal(e)
	s.broadcastEvent(b)
}

// noteGeometry keeps the picture's shape up to date for whoever connects next.
func (s *Session) noteGeometry(e ipc.Event) {
	if e.Width <= 0 || e.Height <= 0 || e.Width > ipc.MaxDimension || e.Height > ipc.MaxDimension {
		return
	}
	s.viewMu.Lock()
	s.info.Width, s.info.Height, s.info.Aspect = e.Width, e.Height, e.Aspect
	s.viewMu.Unlock()
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Input applies an 11-byte pad message from a viewer. Malformed ones and
// players beyond what the player offers are ignored.
func (s *Session) Input(p []byte) {
	port, _, _, _, _, _, ok := ipc.ParseInput(p)
	if !ok || port >= MaxPlayers {
		return
	}
	// Clients repeat their state now and then, so that a lost packet heals. A repeat
	// isn't someone playing, so it doesn't hold off the idle limit, and the runner
	// already has it.
	s.viewMu.Lock()
	changed := !bytes.Equal(s.lastPad[port][:], p)
	if changed {
		copy(s.lastPad[port][:], p)
		s.lastInput = time.Now()
	}
	s.viewMu.Unlock()
	if changed {
		_ = s.command(ipc.CmdInput, p)
	}
}

// MaxPlayers is how many players a browser may drive.
const MaxPlayers = 4

// Reset is the console's reset button.
func (s *Session) Reset() error { return s.command(ipc.CmdReset) }

// ChangeDisc swaps the disc in a game that has several.
func (s *Session) ChangeDisc(index int) error {
	s.viewMu.Lock()
	count := s.info.Discs
	s.viewMu.Unlock()
	if index < 0 || index >= count || index > 255 {
		return errors.New("this game has no such disc")
	}
	return s.command(ipc.CmdDisc, []byte{byte(index)})
}

// Pause stops or restarts the game, as a menu does.
func (s *Session) Pause(on bool) error {
	s.viewMu.Lock()
	defer s.viewMu.Unlock()
	s.menuPaused = on
	s.applyPauseLocked()
	return nil
}

// SlotName is the state file for a slot: 0 is the automatic save, 1-9 the
// manual ones. Paths are built only from this number.
func SlotName(slot int) (string, error) {
	switch {
	case slot == 0:
		return "auto", nil
	case slot >= 1 && slot <= 9:
		return "state-" + strconv.Itoa(slot), nil
	}
	return "", ErrNoSlot
}

// SaveState writes a save slot, and a picture of the game beside it.
func (s *Session) SaveState(ctx context.Context, slot int) error {
	name, err := SlotName(slot)
	if err != nil {
		return err
	}
	b, err := s.requestState(ctx)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return errors.New("this core has no save states")
	}
	if err := writeAtomic(filepath.Join(s.opt.SaveDir, name+".state"), b); err != nil {
		return err
	}
	s.writeThumb(name)
	return nil
}

// LoadState restores a save slot.
func (s *Session) LoadState(slot int) error {
	name, err := SlotName(slot)
	if err != nil {
		return err
	}
	b, err := readLimited(filepath.Join(s.opt.SaveDir, name+".state"))
	if err != nil {
		return ErrNoSlot
	}
	return s.command(ipc.CmdState, b)
}

func (s *Session) requestState(ctx context.Context) ([]byte, error) {
	s.stateMu.Lock()
	s.stateNext++
	id := s.stateNext
	ch := make(chan []byte, 1)
	s.stateWait[id] = ch
	s.stateMu.Unlock()
	defer func() {
		s.stateMu.Lock()
		delete(s.stateWait, id)
		s.stateMu.Unlock()
	}()
	var req [4]byte
	binary.LittleEndian.PutUint32(req[:], id)
	if err := s.command(ipc.CmdSaveState, req[:]); err != nil {
		return nil, err
	}
	select {
	case b := <-ch:
		return b, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(20 * time.Second):
		return nil, errors.New("the game did not answer in time")
	case <-s.done:
		return nil, ErrStopped
	}
}

func (s *Session) deliverState(id uint32, b []byte) {
	if id == 0 { // the state sent on the way out
		if len(b) > 0 {
			if err := writeAtomic(filepath.Join(s.opt.SaveDir, "auto.state"), b); err != nil {
				s.log.Warn("writing the resume state", "err", err)
			}
			s.writeThumb("auto")
		}
		return
	}
	s.stateMu.Lock()
	ch := s.stateWait[id]
	s.stateMu.Unlock()
	if ch != nil {
		ch <- append([]byte(nil), b...)
	}
}

// Stop saves the game and ends it. It returns once everything is written.
func (s *Session) Stop() {
	s.stopOnce.Do(func() {
		s.stopping.Store(true)
		if err := s.command(ipc.CmdQuit, []byte{1}); err != nil {
			s.kill()
		}
		select {
		case <-s.done:
		case <-time.After(15 * time.Second):
			s.log.Warn("the runner did not stop in time; ending it")
			s.kill()
		}
		_ = s.stdin.Close()
	})
	<-s.done
}

func (s *Session) kill() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}

// Done is closed when the game has ended.
func (s *Session) Done() <-chan struct{} { return s.done }

// finish runs once the runner has gone. If nobody asked it to go, viewers are told
// why before their connections close, or they would only see a lost connection.
func (s *Session) finish() {
	s.m.remove(s)
	why := "This game was stopped and saved. It may have been started on another device."
	if v := s.exitErr.Load(); v != nil && !s.stopping.Load() {
		why = v.(string)
	}
	if b, err := json.Marshal(ipc.Event{Type: "ended", Text: truncate(why, 300)}); err == nil && len(s.snapshotViewers()) > 0 {
		s.broadcastEvent(b)
		time.Sleep(300 * time.Millisecond)
	}
	s.viewMu.Lock()
	vs := make([]*viewer, 0, len(s.viewers))
	for v := range s.viewers {
		vs = append(vs, v)
	}
	s.stopEncodersLocked()
	s.viewMu.Unlock()
	for _, v := range vs {
		v.close()
	}
	s.cleanup()
}

func (s *Session) cleanup() {
	s.cleanOnce.Do(func() {
		if s.box != nil {
			s.box()
		}
		if s.opt.Cleanup != nil {
			s.opt.Cleanup()
		}
	})
}

// stuck says whether a running game has stopped making frames. Its process is
// alive, so nothing else would notice, and the player would show one still picture.
func (s *Session) stuck() bool {
	s.viewMu.Lock()
	paused := s.paused
	s.viewMu.Unlock()
	if paused {
		return false
	}
	last := s.lastFrame.Load()
	return last != 0 && time.Since(time.Unix(0, last)) > stuckLimit
}

func (s *Session) idleTooLong() bool {
	s.viewMu.Lock()
	defer s.viewMu.Unlock()
	if len(s.viewers) == 0 {
		return time.Since(s.noViewerSince) > noViewerLimit
	}
	return time.Since(s.lastInput) > idleLimit
}

// writeAtomic replaces a file in one step, so a crash leaves the old save whole.
func writeAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".save-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
