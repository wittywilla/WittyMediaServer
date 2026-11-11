// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymodule/service"
)

// The games in standalone emulators this server started, and whose they are.
//
// WittyPlay runs the emulator and checks the ticket that opens its desktop, but only the
// server knows about profiles: which sign-in started a game, whether that profile may still
// play games, and whether its screen time has run out. A desktop is reached by its ticket
// alone, from a frame that can't sign in, and a WebRTC picture doesn't pass through the
// server at all, so none of the checks made on ordinary requests would ever see a game that
// simply goes on. This is where the server keeps hold of it:
//
//   - a desktop is passed through only for a game this server started and that hasn't ended;
//   - while a profile with limits plays, the time counts as screen time, and the game is
//     stopped when the time is up or games are turned off for the profile;
//   - a game is stopped when the sign-in that started it ends: signing out, switching to
//     another profile on that device, the account being deleted.

type emulatorGame struct {
	id      string // WittyPlay's name for the session
	session string // the sign-in that started it
	user    int64
	item    int64
}

type emulatorGames struct {
	mu sync.Mutex
	m  map[string]*emulatorGame
}

func (e *emulatorGames) add(g *emulatorGame) {
	e.mu.Lock()
	if e.m == nil {
		e.m = map[string]*emulatorGame{}
	}
	e.m[g.id] = g
	e.mu.Unlock()
}

func (e *emulatorGames) remove(id string) {
	e.mu.Lock()
	delete(e.m, id)
	e.mu.Unlock()
}

func (e *emulatorGames) get(id string) (emulatorGame, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	g := e.m[id]
	if g == nil {
		return emulatorGame{}, false
	}
	return *g, true
}

// stop asks WittyPlay to end every game that match picks. It doesn't wait for the answers.
func (e *emulatorGames) stop(s *Server, match func(*emulatorGame) bool) int {
	e.mu.Lock()
	var which []emulatorGame
	for _, g := range e.m {
		if match(g) {
			which = append(which, *g)
		}
	}
	e.mu.Unlock()
	for _, g := range which {
		logging.Go("stop an emulator", func() { s.stopEmulator(g) }, "cat", "games", "session", g.id)
	}
	return len(which)
}

// emulatorWatchEvery is how often a game played by a profile with limits is looked at. It
// has to be well inside parental.Gap, so that playing counts as one stretch of screen time.
// It is read by games as they start, so a test that shortens it does so safely.
var emulatorWatchEvery atomic.Int64

func init() { emulatorWatchEvery.Store(int64(20 * time.Second)) }

// watchEmulator keeps a game started by a profile with limits within them until it ends.
func (s *Server) watchEmulator(g emulatorGame, ended <-chan struct{}) {
	t := time.NewTicker(time.Duration(emulatorWatchEvery.Load()))
	defer t.Stop()
	ctx := context.Background()
	for {
		select {
		case <-ended:
			return
		case <-s.streamsEnding():
			return
		case <-t.C:
		}
		u, err := s.Auth.User(ctx, g.user)
		if err != nil {
			s.stopEmulator(g)
			return
		}
		if !s.emulatorMayGoOn(ctx, u, true) {
			s.Log.Info("a game in an emulator was stopped: the profile may not play now", "cat", "games", "session", g.id, "name", u.Name)
			s.stopEmulator(g)
			return
		}
	}
}

// emulatorMayGoOn: this account may be playing a game in an emulator now. With tick, the
// time since the last look counts as screen time.
func (s *Server) emulatorMayGoOn(ctx context.Context, u *auth.User, tick bool) bool {
	a := accessFor(u)
	if a == nil {
		return true
	}
	if !a.Can(library.FeatureGames) {
		return false
	}
	p := s.parentalInit()
	if !a.Time.Empty() && !a.Time.Check(s.clock(), p.tracker.Usage(ctx, u.ID)).Allowed {
		return false
	}
	if tick {
		p.tracker.Tick(ctx, u.ID)
	}
	return true
}

// stopEmulator asks WittyPlay to end a game, as the account that is playing it.
func (s *Server) stopEmulator(g emulatorGame) {
	if service.Dial == nil {
		return
	}
	c, err := service.Dial(wittyPlayID)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	path := "/docker/sessions/" + g.id
	r, err := http.NewRequestWithContext(modmgr.WithIdentity(ctx, g.user, false), http.MethodDelete, path, nil)
	if err != nil {
		return
	}
	w := &discardAnswer{h: http.Header{}}
	modmgr.ProxyToService(c, w, r, path, func(err error) {
		s.Log.Warn("could not stop a game in an emulator", "cat", "games", "session", g.id, "err", err)
	})
	if w.code >= 400 && w.code != 404 {
		s.Log.Warn("WittyPlay would not stop a game in an emulator", "cat", "games", "session", g.id, "status", w.code)
	}
}

// discardAnswer takes an answer nobody reads, and remembers only its status.
type discardAnswer struct {
	h    http.Header
	code int
}

func (d *discardAnswer) Header() http.Header { return d.h }
func (d *discardAnswer) WriteHeader(code int) {
	if d.code == 0 {
		d.code = code
	}
}
func (d *discardAnswer) Write(b []byte) (int, error) {
	if d.code == 0 {
		d.code = 200
	}
	return len(b), nil
}
