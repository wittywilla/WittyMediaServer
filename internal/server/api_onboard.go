package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/desktop"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/remote"
)

// First-run setup and the sign-in screen: the profile list, the system
// folder picker and API-key checks for the setup wizard.

// clientIP is the address the request came from. X-Forwarded-For is trusted
// only from the reverse proxies the owner listed in Settings: behind any other
// proxy everyone counts as remote, which is the safe direction for everything
// that uses this.
func clientIP(r *http.Request) net.IP {
	if a := clientFrom(r).IP; a.IsValid() {
		return net.IP(a.AsSlice())
	}
	return nil
}

// onLocalNetwork: the same machine or the home network.
func onLocalNetwork(r *http.Request) bool { return clientFrom(r).Where.AtHome() }

// onThisMachine: the browser runs on the server's own desktop, with nothing
// in between.
func onThisMachine(r *http.Request) bool {
	c := clientFrom(r)
	return c.Where == remote.Machine && c.Direct()
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
		// Away from home nobody can sign in without a password, and nobody
		// needs to know which profiles have none.
		out = append(out, profile{ID: u.ID, Name: u.Name, Kind: u.Kind, HasPassword: u.HasPassword || !onLocalNetwork(r)})
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

// Thresholds of the sign-in limits. Limits for one address and one account
// apply only away from home: nobody on the internet can lock the owner out of
// their server at home, and a child guessing at home locks nobody else out.
const (
	pairFails, pairWait, pairCeil = 5, 30 * time.Second, 15 * time.Minute // one name from one address
	addrFails, addrWait, addrCeil = 20, 5 * time.Minute, time.Hour        // one address trying many names
	acctFails, acctWait, acctCeil = 30, 15 * time.Minute, time.Hour       // one name from many addresses
	failMemory                    = 30 * time.Minute                      // quiet for this long: start over
	limiterSize                   = 20000
)

// loginConcurrency: checking a password takes real work, so a flood of sign-in
// attempts must not take the whole machine.
func loginConcurrency() int { return max(2, runtime.NumCPU()/2) }

// loginLimiter slows down password guessing: after 5 failures for one name
// from one address, each further attempt waits longer (up to 15 minutes). Away
// from home there are also limits per address and per account.
type loginLimiter struct {
	mu     sync.Mutex
	m      map[string]*attempts
	recent []time.Time // the latest failures, for Settings
}

type attempts struct {
	fails int
	last  time.Time
	until time.Time
}

func (l *loginLimiter) key(r *http.Request, name string) string {
	return remote.LimitKey(clientFrom(r).IP) + "|" + limitName(name)
}

func limitName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if len(name) > 64 {
		name = name[:64]
	}
	return name
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

func (l *loginLimiter) fail(k string) { l.bump(k, pairFails, pairWait, pairCeil) }

func (l *loginLimiter) bump(k string, limit int, wait, ceil time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = map[string]*attempts{}
	}
	now := time.Now()
	// Forget stale entries so the map cannot grow without bound, and if that
	// frees nothing, everything that isn't locked at the moment.
	if len(l.m) >= limiterSize {
		for key, a := range l.m {
			if now.Sub(a.last) > failMemory {
				delete(l.m, key)
			}
		}
		if len(l.m) >= limiterSize {
			for key, a := range l.m {
				if now.After(a.until) {
					delete(l.m, key)
				}
			}
		}
	}
	a := l.m[k]
	if a == nil || now.Sub(a.last) > failMemory {
		a = &attempts{}
		l.m[k] = a
	}
	a.fails++
	a.last = now
	if a.fails >= limit {
		a.until = now.Add(min(wait<<min(a.fails-limit, 8), ceil))
	}
}

func (l *loginLimiter) ok(k string) {
	l.mu.Lock()
	delete(l.m, k)
	l.mu.Unlock()
}

// gate is how long this client must wait before trying this name again.
func (l *loginLimiter) gate(r *http.Request, name string) time.Duration {
	d := l.wait(l.key(r, name))
	if c := clientFrom(r); !c.Where.AtHome() {
		d = max(d, l.wait("addr|"+remote.LimitKey(c.IP)), l.wait("acct|"+limitName(name)))
	}
	return d
}

// failed counts a wrong password or code.
func (l *loginLimiter) failed(r *http.Request, name string) {
	l.fail(l.key(r, name))
	if c := clientFrom(r); !c.Where.AtHome() {
		l.bump("addr|"+remote.LimitKey(c.IP), addrFails, addrWait, addrCeil)
		l.bump("acct|"+limitName(name), acctFails, acctWait, acctCeil)
	}
	l.mu.Lock()
	l.recent = append(l.recent, time.Now())
	if len(l.recent) > 500 {
		l.recent = l.recent[len(l.recent)-500:]
	}
	l.mu.Unlock()
}

// passed clears this client's count for this name. The address and account
// counts stay: signing in to one's own account must not reset guessing others.
func (l *loginLimiter) passed(r *http.Request, name string) { l.ok(l.key(r, name)) }

// lockout is one active sign-in lock, as Settings shows it.
type lockout struct {
	Kind  string `json:"kind"` // pair | address | account
	Addr  string `json:"addr,omitempty"`
	Name  string `json:"name,omitempty"`
	Until int64  `json:"until"`
	Fails int    `json:"fails"`
}

// snapshot lists the active locks and counts the failures of the last day.
func (l *loginLimiter) snapshot() (locks []lockout, failures int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	locks = []lockout{}
	for k, a := range l.m {
		if !now.Before(a.until) {
			continue
		}
		v := lockout{Until: a.until.Unix(), Fails: a.fails}
		switch {
		case strings.HasPrefix(k, "addr|"):
			v.Kind, v.Addr = "address", k[5:]
		case strings.HasPrefix(k, "acct|"):
			v.Kind, v.Name = "account", k[5:]
		default:
			v.Kind = "pair"
			v.Addr, v.Name, _ = strings.Cut(k, "|")
		}
		locks = append(locks, v)
	}
	for _, t := range l.recent {
		if now.Sub(t) < 24*time.Hour {
			failures++
		}
	}
	return locks, failures
}

func (l *loginLimiter) clear() {
	l.mu.Lock()
	l.m = nil
	l.mu.Unlock()
}
