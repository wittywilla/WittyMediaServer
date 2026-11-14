// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymodule/host"
)

// Live sessions a module started: a game being played. A game's picture doesn't pass through
// the server, and a game simply goes on with no requests, so none of the checks made on ordinary
// requests would see it. This is where the server keeps hold of it:
//
//   - it shows in "playing now" and metrics, and its time is added to "Continue playing";
//   - while a profile with limits plays, the time counts as screen time, and the session is
//     ended when the time is up or games are turned off for the profile;
//   - it is ended when the sign-in that started it ends: signing out, switching profile on that
//     device, the account being deleted.
//
// Ending one tells the module (host.EventSessionStop), which stops what it was running.

type moduleSession struct {
	host.Session
	Module  string
	Started time.Time
	done    chan struct{} // closed when it ends
}

type moduleSessions struct {
	mu       sync.Mutex
	next     int
	m        map[string]*moduleSession
	key      []byte // for signInHandle, made once per start
	watching bool
}

// moduleWatchEvery is how often a module's sessions are looked at. A variable for tests.
var moduleWatchEvery atomic.Int64

func init() { moduleWatchEvery.Store(int64(30 * time.Second)) }

// List is the live sessions modules reported, for metrics and "playing now".
func (ms *moduleSessions) List() []moduleSession {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	out := make([]moduleSession, 0, len(ms.m))
	for _, v := range ms.m {
		out = append(out, *v)
	}
	return out
}

// signInHandle is the opaque name a module is given for a sign-in (X-Witty-SignIn): the same for
// every request from it, of no use for anything else, and different every time the server starts.
func (s *Server) signInHandle(session string) string {
	if session == "" {
		return ""
	}
	ms := &s.modSessions
	ms.mu.Lock()
	if ms.key == nil {
		ms.key = make([]byte, 32)
		_, _ = rand.Read(ms.key)
	}
	key := ms.key
	ms.mu.Unlock()
	m := hmac.New(sha256.New, key)
	m.Write([]byte(session))
	return hex.EncodeToString(m.Sum(nil))[:32]
}

func (s *Server) hostSessionStart(w http.ResponseWriter, r *http.Request, module string) {
	var in host.Session
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil || in.User <= 0 || in.Kind == "" {
		writeJSON(w, 400, map[string]string{"error": "a session needs a kind and a person"})
		return
	}
	ms := &s.modSessions
	ms.mu.Lock()
	if ms.m == nil {
		ms.m = map[string]*moduleSession{}
	}
	ms.next++
	in.ID = module + "-" + strconv.Itoa(ms.next)
	v := &moduleSession{Session: in, Module: module, Started: time.Now(), done: make(chan struct{})}
	ms.m[in.ID] = v
	start := !ms.watching
	ms.watching = true
	ms.mu.Unlock()
	if start {
		logging.Go("module sessions", s.watchModuleSessions, "cat", "modules")
	}
	if in.Kind == "game" {
		name := ""
		if u, err := s.Auth.User(r.Context(), in.User); err == nil {
			name = u.Name
		}
		s.streams.game(in.ID, in.User, name, in.Title, v.done, func() GameView { return GameView{} })
	}
	writeJSON(w, 200, map[string]string{"id": in.ID})
}

func (s *Server) hostSessionEnd(w http.ResponseWriter, r *http.Request, module string) {
	ms := &s.modSessions
	ms.mu.Lock()
	v := ms.m[r.PathValue("id")]
	if v == nil || v.Module != module {
		ms.mu.Unlock()
		writeJSON(w, 404, map[string]string{"error": "no such session"})
		return
	}
	delete(ms.m, v.ID)
	close(v.done)
	ms.mu.Unlock()
	s.sessionPlayed(*v)
	w.WriteHeader(204)
}

// sessionPlayed adds a game's time to what "Continue playing" shows, in the same progress table
// videos use: the position is the total time played.
func (s *Server) sessionPlayed(v moduleSession) {
	if v.Kind != "game" || v.Item <= 0 {
		return
	}
	ctx := context.Background()
	played := time.Since(v.Started).Seconds()
	s.Log.Info("game session ended", "cat", "games", "module", v.Module, "session", v.ID, "item", v.Item, "played", time.Duration(played*float64(time.Second)).Round(time.Second).String())
	var total float64
	_ = s.DB.QueryRowContext(ctx, `SELECT position_sec FROM progress WHERE user_id = ? AND item_id = ?`, v.User, v.Item).Scan(&total)
	if err := s.Store.SetProgress(ctx, v.User, v.Item, library.Progress{PositionSec: total + played}); err != nil {
		s.Log.Warn("could not save time played", "cat", "games", "session", v.ID, "err", err)
	}
}

// stopModuleSessions ends the sessions that match, and tells their modules why.
func (s *Server) stopModuleSessions(reason string, match func(*moduleSession) bool) {
	ms := &s.modSessions
	ms.mu.Lock()
	var gone []moduleSession
	for id, v := range ms.m {
		if match(v) {
			gone = append(gone, *v)
			delete(ms.m, id)
			close(v.done)
		}
	}
	ms.mu.Unlock()
	for _, v := range gone {
		s.Log.Info("a module's session was ended", "cat", "modules", "module", v.Module, "session", v.ID, "why", reason)
		s.hostEvents().publish(host.EventSessionStop, host.SessionStop{ID: v.ID, User: v.User, Reason: reason})
		s.sessionPlayed(v)
	}
}

// watchModuleSessions looks at each session's profile now and then while there are any: screen
// time is counted, and a session its profile may no longer have is ended.
func (s *Server) watchModuleSessions() {
	t := time.NewTicker(time.Duration(moduleWatchEvery.Load()))
	defer t.Stop()
	ctx := context.Background()
	for {
		select {
		case <-s.streamsEnding():
			return
		case <-t.C:
		}
		ms := &s.modSessions
		ms.mu.Lock()
		users := map[int64]bool{}
		for _, v := range ms.m {
			users[v.User] = true
		}
		if len(users) == 0 {
			ms.watching = false
			ms.mu.Unlock()
			return
		}
		ms.mu.Unlock()
		for id := range users {
			u, err := s.Auth.User(ctx, id)
			if err != nil {
				if ctx.Err() == nil {
					s.stopModuleSessions("gone", func(v *moduleSession) bool { return v.User == id })
				}
				continue
			}
			a := accessFor(u)
			if a == nil {
				continue
			}
			if !a.Can(library.FeatureGames) {
				s.stopModuleSessions("feature-off", func(v *moduleSession) bool { return v.User == id })
				continue
			}
			p := s.parentalInit()
			if !a.Time.Empty() && !a.Time.Check(s.clock(), p.tracker.Usage(ctx, id)).Allowed {
				s.stopModuleSessions("time-up", func(v *moduleSession) bool { return v.User == id })
				continue
			}
			p.tracker.Tick(ctx, id)
		}
	}
}

// hostUsers is every account, for a module's own lists of people.
func (s *Server) hostUsers(w http.ResponseWriter, r *http.Request, module string) {
	all, err := s.Auth.Users(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not list accounts"})
		return
	}
	seen := map[int64]int64{}
	if rows, err := s.DB.QueryContext(r.Context(), `SELECT user_id, MAX(IFNULL(last_seen, created_at)) FROM sessions WHERE expires_at > ? GROUP BY user_id`, time.Now().Unix()); err == nil {
		for rows.Next() {
			var id, at int64
			if rows.Scan(&id, &at) == nil {
				seen[id] = at
			}
		}
		rows.Close()
	}
	out := make([]host.User, 0, len(all))
	for _, u := range all {
		out = append(out, host.User{ID: u.ID, Name: u.Name, DisplayName: u.DisplayName, Kind: u.Kind, Admin: u.IsAdmin, LastSeen: seen[u.ID]})
	}
	writeJSON(w, 200, map[string]any{"users": out})
}

// hostTools is the server's FFmpeg and what it found it can encode with.
func (s *Server) hostTools(w http.ResponseWriter, r *http.Request, module string) {
	t := host.Tools{FFmpeg: s.Tools.FFmpeg, FFprobe: s.Tools.FFprobe, Version: s.Tools.Version, Encoders: s.Tools.Encoders}
	s.Tools.StartHardwareProbe()
	list, done := s.Tools.HardwareStatus()
	for _, h := range list {
		b, _ := json.Marshal(h)
		t.Hardware = append(t.Hardware, b)
	}
	t.HardwareDone = done
	writeJSON(w, 200, t)
}
