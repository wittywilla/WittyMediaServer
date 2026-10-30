package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/desktop"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// First-run setup and the sign-in screen: the profile list, the system
// folder picker and API-key checks for the setup wizard.

// clientIP is the address the request came from. X-Forwarded-For is not
// trusted: behind a reverse proxy everyone counts as remote, which is the
// safe direction for everything that uses this.
func clientIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

func proxied(r *http.Request) bool {
	return r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("Forwarded") != ""
}

// onLocalNetwork: the same machine or a private LAN address.
func onLocalNetwork(r *http.Request) bool {
	ip := clientIP(r)
	return ip != nil && !proxied(r) && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

// onThisMachine: the browser runs on the server's own desktop.
func onThisMachine(r *http.Request) bool {
	ip := clientIP(r)
	return ip != nil && ip.IsLoopback() && !proxied(r)
}

// GET /api/profiles (public). The "Who's watching?" screen lists accounts by
// name, so by default it is only offered on the local network; remote
// visitors get the classic name + password form unless an admin allows it.
func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.profilesVisible(r) {
		writeJSON(w, 200, map[string]any{"style": "classic"})
		return
	}
	users, err := s.Auth.Users(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	type profile struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Kind        string `json:"kind"`
		HasPassword bool   `json:"hasPassword"`
	}
	out := make([]profile, 0, len(users))
	for _, u := range users {
		out = append(out, profile{ID: u.ID, Name: u.Name, Kind: u.Kind, HasPassword: u.HasPassword})
	}
	writeJSON(w, 200, map[string]any{"style": "profiles", "profiles": out})
}

// profilesVisible: the profile screen is on, and this device is on the home
// network (or the admin allowed remote devices). Password-free Kids and
// Guest profiles can only sign in where this is true.
func (s *Server) profilesVisible(r *http.Request) bool {
	ctx := r.Context()
	if s.DB.GetSetting(ctx, "loginStyle", "profiles") != "profiles" {
		return false
	}
	return onLocalNetwork(r) || s.DB.GetSetting(ctx, "profilesRemote", "0") == "1"
}

// POST /api/fs/pick (admin): opens the operating system's folder picker on
// the server's desktop. Only for a browser on the same machine; everyone
// else uses the in-page folder browser.
func (s *Server) handlePickFolder(w http.ResponseWriter, r *http.Request) {
	if !onThisMachine(r) {
		writeErr(w, 501, "the system folder picker only opens on the server's own screen")
		return
	}
	pickMu.Lock() // one dialog at a time
	defer pickMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	p, err := desktop.PickFolder(ctx, "Choose a folder with your media")
	switch {
	case errors.Is(err, desktop.ErrCancelled):
		writeJSON(w, 200, map[string]any{"cancelled": true})
	case err != nil:
		writeErr(w, 501, err.Error())
	default:
		writeJSON(w, 200, map[string]any{"path": p})
	}
}

var pickMu sync.Mutex

// POST /api/metadata/test (admin) {provider: tmdb|tvdb, key, pin}: checks a
// key before it is saved, so the wizard can say "Connected" right away.
func (s *Server) handleMetadataTest(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Provider string `json:"provider"`
		Key      string `json:"key"`
		PIN      string `json:"pin"`
	}
	if err := readJSON(r, &b); err != nil || strings.TrimSpace(b.Key) == "" {
		writeErr(w, 400, "paste a key first")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	var err error
	switch b.Provider {
	case "tmdb":
		err = (&library.TMDB{Key: strings.TrimSpace(b.Key)}).Check(ctx)
	case "tvdb":
		err = (&library.TVDB{Key: strings.TrimSpace(b.Key), PIN: strings.TrimSpace(b.PIN)}).Check(ctx)
	default:
		writeErr(w, 400, "unknown provider")
		return
	}
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "401") || strings.Contains(msg, "403") {
			msg = "The key was not accepted. Check that you copied all of it."
		} else if errors.Is(err, context.DeadlineExceeded) {
			msg = "No answer from the service. Is this server online?"
		}
		writeJSON(w, 200, map[string]any{"ok": false, "error": msg})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------------------------------------------------------------- sign-in limits

// loginSlots: password checks running at once.
var loginSlots = make(chan struct{}, 2)

// ipFailLimit: wrong passwords from one address, over all names, before that
// address has to slow down (it can't try one password on every account).
const ipFailLimit = 30

// maxLoginEntries bounds the limiter's memory whatever a flood sends.
const maxLoginEntries = 10000

// loginLimiter slows down password guessing: after 5 failures for one name
// from one address, each further attempt waits longer (up to 15 minutes).
type loginLimiter struct {
	mu sync.Mutex
	m  map[string]*attempts
}

type attempts struct {
	fails int
	last  time.Time
	until time.Time
}

// keys are the counters a sign-in attempt adds to: this name from this
// address, and this address over all names.
func (l *loginLimiter) keys(r *http.Request, name string) (nameKey, ipKey string) {
	ip := "?"
	if p := clientIP(r); p != nil {
		ip = p.String()
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if len(name) > 64 { // no account has a longer name; the key must stay small
		name = name[:64]
	}
	return ip + "|" + name, ip
}

// wait returns how long the caller must wait before trying again (0 = go).
func (l *loginLimiter) wait(k string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if a := l.m[k]; a != nil && time.Now().Before(a.until) {
		return time.Until(a.until)
	}
	return 0
}

func (l *loginLimiter) fail(k string, limit int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = map[string]*attempts{}
	}
	now := time.Now()
	// Forget stale entries so the map cannot grow without bound.
	if len(l.m) > maxLoginEntries {
		for key, a := range l.m {
			if now.Sub(a.last) > 30*time.Minute {
				delete(l.m, key)
			}
		}
		if len(l.m) > maxLoginEntries { // all recent: a flood. Start over.
			clear(l.m)
		}
	}
	a := l.m[k]
	if a == nil || now.Sub(a.last) > 30*time.Minute {
		a = &attempts{}
		l.m[k] = a
	}
	a.fails++
	a.last = now
	if a.fails >= limit {
		d := 30 * time.Second << min(a.fails-limit, 5)
		a.until = now.Add(min(d, 15*time.Minute))
	}
}

func (l *loginLimiter) ok(k string) {
	l.mu.Lock()
	delete(l.m, k)
	l.mu.Unlock()
}
