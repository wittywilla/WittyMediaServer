// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/play/ipc"
	"github.com/wittywilla/wittymodule/service"
)

// Written by hand: what holds functions or channels, or lives in the module and is reached by name.

const module = "wittyplay"

// Config says how games are run. It is kept here, and given to the module whenever its service
// starts, since a service that stopped has forgotten it.
type Config struct {
	Runner      string   // the game runner program, or another runner speaking ipc
	RunnerArgs  []string // extra arguments, before the ones added here
	FFmpeg      string
	HWEncoders  []string // hardware encoders known to work here and switched on, best first
	GPUEncoders []string // hardware encoders known to work here at all, best first
	// HaveEncoder says FFmpeg was built with this encoder. Encoders is that as a list, which is
	// what can reach the module.
	HaveEncoder func(string) bool `json:"-"`
	Encoders    []string
	MaxSessions int          // 0 means 2
	MaxGPU      int          // games drawn on the graphics card at once; 0 means 2
	MemoryMB    int64        // 0 means 2048
	GPUMemoryMB int64        // the same for a game with a graphics driver loaded; 0 means 8192
	Log         *slog.Logger `json:"-"`

	// WebRTC networking; all optional. A fixed UDP port is what a firewall or
	// a container needs; the public address is what NAT needs.
	UDPPort   int
	PublicIPs []string
	STUN      []string
}

// Options start a game.
type Options struct {
	UserID, ItemID int64
	Title          string
	Console        string
	Core           string // a shared library path, or ipc.TestCore
	ROM            string
	SystemDir      string
	SaveDir        string   // this account's folder for this game
	Resume         bool     // continue from auto.state
	Quality        string   // "" (auto) | low | high
	GPU            bool     // the core draws with a graphics card
	RunnerArgs     []string // extra arguments for this game's runner: wittyplay.RunnerArgs
	CoreID         string   // the core's name, for working out its starting options again
	CoreOptions    map[string]string
	PreferDevices  []string // the kind of controller to plug in, by words in its name
	CoreAssets     string   // support files of its own, if it has any
	// LoadLimit is how long it may take to load, when it's more than a game usually gets.
	LoadLimit time.Duration
	Cleanup   func() `json:"-"` // runs when the session has ended
	// Fallbacks are cores to try, in order, if this game won't load on Core.
	Fallbacks []CoreChoice

	Control       *ipc.Control                           // how the game runs from the first frame
	Cheats        []ipc.Cheat                            // given to the core before the first frame
	MaxSpeed      float64                                // the fastest it may be made to run; 0 means normal speed
	NoAutoSave    bool                                   // leaving doesn't write the resume state
	SafeMode      bool                                   // started with defaults
	Notices       []string                               // things to tell whoever started it
	OnCoreOptions func(coreID string, list []ipc.Option) `json:"-"` // a core has said what it offers
}

// Manager is the module's one set of running games, as the server reaches it.
type Manager struct {
	mu  sync.Mutex
	cfg Config
}

// NewManager prepares to run games. Nothing starts until one is played.
func NewManager(cfg Config) *Manager {
	m := &Manager{cfg: cfg}
	service.OnStart(module, func(c *service.Conn) { m.push(context.Background(), c) })
	return m
}

// MarshalJSON: a Manager says nothing, since there's only one at the other end.
func (m *Manager) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// wire is what reaches the module.
func (m *Manager) wire() Config {
	m.mu.Lock()
	c := m.cfg
	m.mu.Unlock()
	return c
}

// push gives the module the settings.
func (m *Manager) push(ctx context.Context, c *service.Conn) {
	cfg := m.wire()
	if cfg.HaveEncoder != nil {
		cfg.Encoders = nil
		for _, n := range knownEncoders {
			if cfg.HaveEncoder(n) {
				cfg.Encoders = append(cfg.Encoders, n)
			}
		}
	}
	_ = c.Call(ctx, "stream.Manager.Configure", nil, nil, []any{cfg}, nil)
}

// knownEncoders are the encoders the module can ask about.
var knownEncoders = []string{"libx264", "libopenh264", "h264_nvenc", "hevc_nvenc", "av1_nvenc", "h264_qsv", "hevc_qsv", "av1_qsv", "h264_vaapi", "hevc_vaapi", "av1_vaapi", "h264_amf", "hevc_amf", "av1_amf", "h264_videotoolbox", "hevc_videotoolbox", "h264_v4l2m2m", "h264_rkmpp", "libvpx", "libvpx-vp9", "libsvtav1", "libopus", "aac"}

// Configure changes the settings.
func (m *Manager) Configure(update func(*Config)) {
	m.mu.Lock()
	update(&m.cfg)
	m.mu.Unlock()
	if service.Dial == nil {
		return
	}
	if c, err := service.Dial(module); err == nil {
		m.push(context.Background(), c)
	}
}

// ErrHelper is in the error when a game couldn't start because the module's helper itself
// failed: it went away during the call, or answered something that isn't an answer. Anything
// the helper says went wrong (the runner stopping, a core crashing or refusing, a game file it
// can't use) is about that game, its core or its console, and comes back as the helper's own
// error, without this.
var ErrHelper = errors.New("WittyPlay's helper failed")

// Start starts a game. What's for this side stays here: Cleanup runs when it's over, and
// OnCoreOptions hears what a core says it offers.
func (m *Manager) Start(opt Options) (*Session, error) {
	var s *Session
	if service.Subscribe != nil && opt.OnCoreOptions != nil {
		fn := opt.OnCoreOptions
		_ = service.Subscribe(module, "stream.coreoptions", func(raw json.RawMessage) {
			var n struct {
				CoreID string       `json:"coreId"`
				List   []ipc.Option `json:"list"`
			}
			if json.Unmarshal(raw, &n) == nil {
				go fn(n.CoreID, n.List)
			}
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if service.Dial == nil {
		return nil, service.ErrNoService
	}
	// A helper that can't be started has been counted against its version already, by
	// whatever started it: that error goes back as it is.
	c, err := service.Dial(module)
	if err != nil {
		return nil, err
	}
	if err := c.Call(ctx, "stream.Manager.Start", nil, nil, []any{opt}, []any{&s}); err != nil {
		var ce *service.CallError
		if !errors.As(err, &ce) && ctx.Err() == nil {
			return nil, fmt.Errorf("%w: %w", ErrHelper, err)
		}
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("%w: it answered without a game", ErrHelper)
	}
	st := stateOf(s.Sid)
	st.cleanup = opt.Cleanup
	return s, nil
}

// StopAll ends every game, saving each one. It doesn't start a service to do it.
func (m *Manager) StopAll() {
	if service.Running == nil || !service.Running(module) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_ = service.Call(ctx, module, "stream.Manager.StopAll", nil, nil, nil, nil)
}

// Session is a game being played, in the module.
type Session struct {
	Sid string `json:"id"`
}

// MarshalJSON names a session to the module, by the name it gave.
func (s *Session) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID string `json:"id"`
	}{s.Sid})
}

// sessionState is what this side keeps of a session: its end, and what's to run then.
type sessionState struct {
	done    chan struct{}
	cleanup func()
}

var (
	statesMu sync.Mutex
	states   = map[string]*sessionState{}
)

func stateOf(id string) *sessionState {
	statesMu.Lock()
	defer statesMu.Unlock()
	st := states[id]
	if st == nil {
		st = &sessionState{done: make(chan struct{})}
		states[id] = st
		go st.wait(id)
	}
	return st
}

// wait asks until the game has ended. The service going away ends it too: games don't outlive
// their process.
func (st *sessionState) wait(id string) {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		var over bool
		err := service.Call(ctx, module, "stream.Session.Wait", nil, nil, []any{id}, []any{&over})
		cancel()
		if over || (err != nil && !errors.Is(err, context.DeadlineExceeded)) {
			break
		}
	}
	close(st.done)
	if st.cleanup != nil {
		st.cleanup()
	}
	statesMu.Lock()
	delete(states, id)
	statesMu.Unlock()
}

// Done closes when the game has ended.
func (s *Session) Done() <-chan struct{} { return stateOf(s.Sid).done }

// ServeWS passes a browser's connection straight to the module.
func (s *Session) ServeWS(w http.ResponseWriter, r *http.Request, mode string) {
	if service.Proxy == nil {
		http.Error(w, "WittyPlay isn't on this server", http.StatusFailedDependency)
		return
	}
	q := r.URL.Query()
	q.Set("video", mode)
	r.URL.RawQuery = q.Encode()
	if err := service.Proxy(module, w, r, "/ws/"+s.Sid); err != nil {
		http.Error(w, err.Error(), http.StatusFailedDependency)
	}
}

// ServeStream passes a version 2 WebSocket straight to the module. away is the server's word on
// where the device is, and replaces anything the address said.
func (s *Session) ServeStream(w http.ResponseWriter, r *http.Request, away bool) {
	if service.Proxy == nil {
		writeStreamError(w, &StreamError{Status: http.StatusFailedDependency, Code: "no-module", Text: "WittyPlay isn't on this server"})
		return
	}
	q := r.URL.Query()
	q.Del("token")
	q.Del("away")
	if away {
		q.Set("away", "1")
	}
	r.URL.RawQuery = q.Encode()
	if err := service.Proxy(module, w, r, "/stream/"+s.Sid); err != nil {
		writeStreamError(w, &StreamError{Status: http.StatusFailedDependency, Code: "no-module", Text: err.Error()})
	}
}

// writeStreamError answers a refused stream as docs/GAMING-STREAM.md says: the words and a code.
func writeStreamError(w http.ResponseWriter, e *StreamError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": e.Text, "code": e.Code})
}

// ProbeGPU finds out whether a core can draw on this machine's graphics card. Ending ctx ends
// the check, and nothing is learnt.
func ProbeGPU(ctx context.Context, runner, dir string) GPUProbe {
	var p GPUProbe
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	_ = service.Call(ctx, module, "stream.ProbeGPU", nil, nil, []any{runner, dir}, []any{&p})
	return p
}

func init() {
	service.RegisterErrorType[ProbeError]("stream.ProbeError")
	service.RegisterErrorType[StreamError]("stream.StreamError")
}
