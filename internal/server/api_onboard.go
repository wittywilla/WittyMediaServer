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
	"os"
	"path/filepath"
	"runtime"
	"sort"
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

// onLocalNetwork: the same machine or the home network, and asked by this server's own
// pages or an app. A browser at home runs whatever page it was sent to, so being at home
// says nothing about a request that a page elsewhere made.
func onLocalNetwork(r *http.Request) bool {
	return clientFrom(r).Where.AtHome() && localHostName(r) && !pageElsewhere(r)
}

// pageElsewhere: a web page that isn't this server's made this request.
func pageElsewhere(r *http.Request) bool {
	return !appOrigins[r.Header.Get("Origin")] && fromElsewhere(r, "")
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
	host = strings.Trim(host, "[]")
	return net.ParseIP(host) != nil || remote.LocalName(host)
}

// onThisMachine: the browser runs on the server's own desktop, with nothing
// in between.
func onThisMachine(r *http.Request) bool {
	c := clientFrom(r)
	return c.Where == remote.Machine && c.Direct() && localHostName(r) && !pageElsewhere(r)
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
	failMemory                    = 6 * time.Hour                         // quiet for this long: start over
	limiterSize                   = 20000
	// maxNamesPerAddress is more names than anyone has reason to try from one address. Past it,
	// new names from there share one count, whether or not they belong to anyone.
	maxNamesPerAddress = 64
	overflowName       = "(many names)"
)

const (
	freeTriesPerName    = pairFails
	freeTriesPerAddress = addrFails
	maxLimiterEntries   = limiterSize
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
	names  map[string]map[string]bool // the per-name counts each address has
	recent []time.Time                // the latest failures, for Settings
}

type attempts struct {
	fails int
	last  time.Time
	until time.Time
}

func (l *loginLimiter) key(r *http.Request, name string) string {
	return addressKey(r) + "|" + limitName(name)
}

// pair is the count this client's try at this name goes on: its own, or the shared one once the
// address has run through more names than anyone would. Without this a flood of made-up names
// from one address would fill the table.
func (l *loginLimiter) pair(r *http.Request, name string) string {
	addr, k := addressKey(r), l.key(r, name)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m[k] != nil {
		return k
	}
	set := l.names[addr]
	if len(set) < maxNamesPerAddress {
		return k
	}
	now := time.Now()
	for known := range set {
		if a := l.m[known]; a == nil || now.Sub(a.last) > failMemory {
			l.forget(known)
		}
	}
	if len(l.names[addr]) < maxNamesPerAddress {
		return k
	}
	return addr + "|" + overflowName
}

// note and forget keep the list of what each address is counted for in step with the counts.
func (l *loginLimiter) note(k string) {
	addr, name, ok := strings.Cut(k, "|")
	if !ok || spanningCount(addr) || name == overflowName {
		return
	}
	if l.names == nil {
		l.names = map[string]map[string]bool{}
	}
	if l.names[addr] == nil {
		l.names[addr] = map[string]bool{}
	}
	l.names[addr][k] = true
}

func (l *loginLimiter) forget(k string) {
	delete(l.m, k)
	addr, _, _ := strings.Cut(k, "|")
	if set := l.names[addr]; set != nil {
		delete(set, k)
		if len(set) == 0 {
			delete(l.names, addr)
		}
	}
}

// spanningCount: a key's first part names a kind of count that spans many names or addresses.
func spanningCount(first string) bool { return first == "addr" || first == "acct" || first == "setup" }

func limitName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if len(name) > 64 { // no name is this long, and the key is kept in memory
		sum := sha256.Sum256([]byte(name))
		name = hex.EncodeToString(sum[:])
	}
	return name
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
	return l.bump(k, free, pairWait, pairCeil)
}

func (l *loginLimiter) fail(k string) { l.bump(k, pairFails, pairWait, pairCeil) }

func (l *loginLimiter) bump(k string, limit int, wait, ceil time.Duration) time.Duration {
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
	if a == nil || now.Sub(a.last) > failMemory {
		l.trim(now)
		a = &attempts{}
		l.m[k] = a
		l.note(k)
	}
	a.fails++
	a.last = now
	if a.fails >= limit {
		a.until = now.Add(min(wait<<min(a.fails-limit, 8), ceil))
	}
	return 0
}

// wait says how long k must wait before trying again, counting nothing.
func (l *loginLimiter) wait(k string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if a := l.m[k]; a != nil {
		if d := time.Until(a.until); d > 0 {
			return d
		}
	}
	return 0
}

// trim keeps the map from growing without bound. It forgets what is stale, and when that isn't
// enough makes room by dropping the least useful: the oldest per-name counts first, then the
// emptiest counts across names or addresses, and what is holding someone back last of all. Nobody
// can wipe a count that way by filling the table.
func (l *loginLimiter) trim(now time.Time) {
	if len(l.m) < maxLimiterEntries {
		return
	}
	for k, a := range l.m {
		if now.Sub(a.last) > failMemory {
			l.forget(k)
		}
	}
	if len(l.m) < maxLimiterEntries {
		return
	}
	type entry struct {
		k    string
		rank int
		a    *attempts
	}
	list := make([]entry, 0, len(l.m))
	for k, a := range l.m {
		rank := 0
		if first, _, ok := strings.Cut(k, "|"); ok && spanningCount(first) {
			rank = 1
		}
		if now.Before(a.until) {
			rank += 2
		}
		list = append(list, entry{k, rank, a})
	}
	sort.Slice(list, func(i, j int) bool {
		x, y := list[i], list[j]
		switch {
		case x.rank != y.rank:
			return x.rank < y.rank
		case x.rank == 1 && x.a.fails != y.a.fails:
			return x.a.fails < y.a.fails
		}
		return x.a.last.Before(y.a.last)
	})
	for _, e := range list[:len(list)/10+1] {
		l.forget(e.k)
	}
}

// ok forgets the failures of a name once its password was right.
func (l *loginLimiter) ok(k string) {
	l.mu.Lock()
	l.forget(k)
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

// setupCodeFile holds the setup code until setup is done, for a server with no console to
// read: a service, a container that restarted, a box with no screen. Only an account that can
// read the data folder can read it, and that account could make itself the owner anyway.
const setupCodeFile = "setup-code.txt"

// WriteSetupCode leaves the code where the person at the server can read it.
func (s *Server) WriteSetupCode() error {
	return os.WriteFile(filepath.Join(s.Cfg.DataDir, setupCodeFile), []byte(s.SetupCode()+"\n"), 0o600)
}

func (s *Server) forgetSetupCode() {
	if s.Cfg != nil && s.Cfg.DataDir != "" {
		_ = os.Remove(filepath.Join(s.Cfg.DataDir, setupCodeFile))
	}
}

func (s *Server) setupCodeOK(given string) bool {
	given = strings.ToUpper(strings.TrimSpace(given))
	return subtle.ConstantTimeCompare([]byte(given), []byte(s.SetupCode())) == 1
}

// gate is how long this client must wait before trying this name again.
func (l *loginLimiter) gate(r *http.Request, name string) time.Duration {
	d := l.wait(l.pair(r, name))
	if c := clientFrom(r); !c.Where.AtHome() {
		d = max(d, l.wait("addr|"+remote.LimitKey(c.IP)), l.wait("acct|"+limitName(name)))
	}
	return d
}

// failed counts a wrong password or code.
func (l *loginLimiter) failed(r *http.Request, name string) {
	l.fail(l.pair(r, name))
	if c := clientFrom(r); !c.Where.AtHome() {
		l.bump("addr|"+remote.LimitKey(c.IP), addrFails, addrWait, addrCeil)
		l.bump("acct|"+limitName(name), acctFails, acctWait, acctCeil)
	}
	l.noteFailure()
}

// attempt counts a sign-in as it starts, and says how long this client must
// wait instead (0 = go ahead). It stops at the first lock, counting nothing further: otherwise a
// locked-out address could go on filling the table, and lock any name for everyone else.
func (l *loginLimiter) attempt(r *http.Request, name string) time.Duration {
	c := clientFrom(r)
	away := !c.Where.AtHome()
	if away {
		if d := l.bump("addr|"+remote.LimitKey(c.IP), addrFails, addrWait, addrCeil); d > 0 {
			return d
		}
	}
	if d := l.bump(l.pair(r, name), pairFails, pairWait, pairCeil); d > 0 || !away {
		return d
	}
	return l.bump("acct|"+limitName(name), acctFails, acctWait, acctCeil)
}

// uncount takes back an attempt that turned out to guess nothing.
func (l *loginLimiter) uncount(r *http.Request, name string) {
	l.refund(l.pair(r, name))
	l.refundAway(r, name)
}

func (l *loginLimiter) refundAway(r *http.Request, name string) {
	if c := clientFrom(r); !c.Where.AtHome() {
		l.refund("addr|" + remote.LimitKey(c.IP))
		l.refund("acct|" + limitName(name))
	}
}

// noteFailure remembers when a failure happened, for Settings.
func (l *loginLimiter) noteFailure() {
	l.mu.Lock()
	l.recent = append(l.recent, time.Now())
	if len(l.recent) > 500 {
		l.recent = l.recent[len(l.recent)-500:]
	}
	l.mu.Unlock()
}

// passed clears this client's count for this name, and takes back this attempt
// from the address and account counts: signing in now and then must not wear
// them out, nor wipe out what an attacker has used up.
func (l *loginLimiter) passed(r *http.Request, name string) {
	if k := l.pair(r, name); strings.HasSuffix(k, "|"+overflowName) {
		l.refund(k) // shared with the names others are trying from there
	} else {
		l.ok(k)
	}
	l.refundAway(r, name)
}

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
		case strings.HasPrefix(k, "setup|"):
			v.Kind, v.Addr = "address", k[6:]
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
	l.m, l.names = nil, nil
	l.mu.Unlock()
}
