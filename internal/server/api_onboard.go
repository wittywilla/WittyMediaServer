package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"runtime"
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

// localHostName: the address in the browser's bar is one only this network can
// resolve. A page on the internet can make its own domain point at this server
// (DNS rebinding) and then talk to it from a visitor's browser, on the visitor's
// network, as if it were on the same site. It can't do that with these names,
// so a request naming any other host is never trusted for being "local".
func localHostName(r *http.Request) bool {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
	if net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return host != "" // an address, "localhost" or a computer's own name
	}
	for _, suffix := range []string{".localhost", ".local", ".lan", ".home", ".home.arpa", ".internal", ".intranet"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// onLocalNetwork: the same machine or a private LAN address.
func onLocalNetwork(r *http.Request) bool {
	ip := clientIP(r)
	return ip != nil && !proxied(r) && localHostName(r) && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

// onThisMachine: the browser runs on the server's own desktop.
func onThisMachine(r *http.Request) bool {
	ip := clientIP(r)
	return ip != nil && ip.IsLoopback() && !proxied(r) && localHostName(r)
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

// loginLimiter slows down password guessing: after 5 failures for one name
// from one address, each further attempt waits longer (up to 15 minutes).
// One address trying many names (one common password against every account)
// gets the same treatment after 30.
//
// An attempt counts when it starts, not when its password turns out wrong:
// otherwise hundreds of guesses sent at once all pass the check before the
// first one has finished.
type loginLimiter struct {
	mu sync.Mutex
	m  map[string]*attempts
}

type attempts struct {
	fails int
	last  time.Time
	until time.Time
}

const (
	freeTriesPerName    = 5
	freeTriesPerAddress = 30
	maxLimiterEntries   = 10000
)

func (l *loginLimiter) key(r *http.Request, name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if len(name) > 64 { // no name is this long, and the key is kept in memory
		sum := sha256.Sum256([]byte(name))
		name = hex.EncodeToString(sum[:])
	}
	return addressKey(r) + "|" + name
}

// addressKey groups the addresses one attacker can use: an IPv6 network hands
// each customer at least a /64.
func addressKey(r *http.Request) string {
	p := clientIP(r)
	if p == nil {
		return "?"
	}
	if p.To4() == nil {
		p = p.Mask(net.CIDRMask(64, 128))
	}
	return p.String()
}

// begin counts an attempt and says how long the caller must wait first (0 =
// go ahead). free is how many attempts are allowed before waiting starts.
func (l *loginLimiter) begin(k string, free int) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if a := l.m[k]; a != nil && now.Before(a.until) {
		return a.until.Sub(now)
	}
	if l.m == nil {
		l.m = map[string]*attempts{}
	}
	a := l.m[k]
	if a == nil || now.Sub(a.last) > 30*time.Minute {
		l.trim(now)
		a = &attempts{}
		l.m[k] = a
	}
	a.fails++
	a.last = now
	if a.fails >= free {
		d := 30 * time.Second << min(a.fails-free, 5)
		a.until = now.Add(min(d, 15*time.Minute))
	}
	return 0
}

// trim keeps the map from growing without bound: it forgets stale entries,
// and when that isn't enough, entries that aren't holding anyone back.
func (l *loginLimiter) trim(now time.Time) {
	if len(l.m) < maxLimiterEntries {
		return
	}
	for k, a := range l.m {
		if now.Sub(a.last) > 30*time.Minute || now.After(a.until) {
			delete(l.m, k)
		}
	}
	for k := range l.m {
		if len(l.m) < maxLimiterEntries {
			break
		}
		delete(l.m, k)
	}
}

// ok forgets the failures of a name once its password was right.
func (l *loginLimiter) ok(k string) {
	l.mu.Lock()
	delete(l.m, k)
	l.mu.Unlock()
}

// refund takes back one attempt that succeeded, for counts that span names:
// signing in now and then must not wear them out, nor wipe out an attacker's.
func (l *loginLimiter) refund(k string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if a := l.m[k]; a != nil && a.fails > 0 {
		a.fails--
		a.until = time.Time{}
	}
}

// passwordSlots limits how many password checks run at once: each takes a CPU
// core for a moment on purpose, and a small server must stay usable when
// someone sends a thousand sign-ins at the same time.
var passwordSlots = make(chan struct{}, max(2, runtime.NumCPU()))

func acquirePasswordSlot(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	select {
	case passwordSlots <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

// ---------------------------------------------------------------- first run

// A fresh server has no accounts, and whoever makes the first one owns it.
// From this network that is the owner setting it up. From anywhere else (a
// server left waiting on the internet, a reverse proxy, a domain name) it takes
// the code the server wrote to its log when it started, which only someone with
// access to the machine can read.

const setupAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no look-alikes (0/O, 1/I)

// SetupCode is this run's first-run code.
func (s *Server) SetupCode() string {
	s.setupCodeOnce.Do(func() {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		var out strings.Builder
		for i, c := range b {
			if i > 0 && i%4 == 0 {
				out.WriteByte('-')
			}
			out.WriteByte(setupAlphabet[int(c)%len(setupAlphabet)])
		}
		s.setupCode = out.String()
	})
	return s.setupCode
}

func (s *Server) setupCodeOK(given string) bool {
	given = strings.ToUpper(strings.TrimSpace(given))
	return subtle.ConstantTimeCompare([]byte(given), []byte(s.SetupCode())) == 1
}
