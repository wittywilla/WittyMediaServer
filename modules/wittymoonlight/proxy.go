package wittymoonlight

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The proxy stands between people's browsers and the bridge, on a port of its own. The
// bridge's own roles can't limit anyone to a rig or a game, and any user of it could start
// any app on a PC, so this is where those limits are:
//
//   - A person arrives with a one-time ticket from the server's API, which checked who they
//     are and what they may play. The ticket becomes a session for that one rig and game.
//   - Only what the stream page needs is passed on. Everything else, including the bridge's
//     host list and admin pages, is refused here.
//   - A stream request is read before it goes on, and must name that rig and that game once.
//   - Anything from the browser that could name someone is taken off, and the server's own
//     cookie or token never reaches the bridge.
//
// It has its own port because the bridge's web pages are someone else's code. On another
// origin, a flaw in them can't read this server's pages or make signed-in API calls that
// read anything back.

const (
	ticketLife  = 2 * time.Minute
	sessionLife = 8 * time.Hour
	cookieName  = "wms_rigplay"
	maxOffer    = 256 << 10
)

// Session is one person's permission to stream one game from one rig.
type Session struct {
	User    int64
	RigID   int64
	AppID   int
	HostID  string // the rig's id inside the bridge
	expires time.Time
}

// Hooks are the checks that belong to the server.
type Hooks struct {
	// MayStart is asked just before a stream begins. It re-checks the person's access and that
	// nobody else's game is running, since the bridge would join that one instead.
	MayStart func(ctx context.Context, s Session) error
	// Started is told a stream began.
	Started func(s Session)
}

// Proxy is the handler for the bridge's port.
type Proxy struct {
	Upstream func() int // the bridge's port, 0 if it isn't running
	Hooks    Hooks
	Secure   bool // this listener uses HTTPS

	mu       sync.Mutex
	tickets  map[string]Session
	sessions map[string]*Session
	last     atomic.Int64
}

// NewProxy makes a proxy for a bridge.
func NewProxy(upstream func() int, hooks Hooks) *Proxy {
	p := &Proxy{Upstream: upstream, Hooks: hooks, tickets: map[string]Session{}, sessions: map[string]*Session{}}
	p.last.Store(time.Now().Unix())
	return p
}

// LastUse is when anything last came through, so the bridge can be stopped when it's idle.
func (p *Proxy) LastUse() time.Time { return time.Unix(p.last.Load(), 0) }

func hashed(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// NewTicket makes a one-time ticket, valid for a couple of minutes. Only its hash is kept.
func (p *Proxy) NewTicket(s Session) (string, error) {
	t, err := randomToken()
	if err != nil {
		return "", err
	}
	s.expires = time.Now().Add(ticketLife)
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for k, v := range p.tickets {
		if now.After(v.expires) {
			delete(p.tickets, k)
		}
	}
	for k, v := range p.sessions {
		if now.After(v.expires) {
			delete(p.sessions, k)
		}
	}
	p.tickets[hashed(t)] = s
	return t, nil
}

// RevokeUser ends the sessions of an account, or of everyone with user 0.
func (p *Proxy) RevokeUser(user int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range p.sessions {
		if user == 0 || v.User == user {
			delete(p.sessions, k)
		}
	}
}

// RevokeRig ends the sessions for a rig.
func (p *Proxy) RevokeRig(rig int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range p.sessions {
		if v.RigID == rig {
			delete(p.sessions, k)
		}
	}
	for k, v := range p.tickets {
		if v.RigID == rig {
			delete(p.tickets, k)
		}
	}
}

func (p *Proxy) session(r *http.Request) *Session {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.sessions[hashed(c.Value)]
	if s == nil || time.Now().After(s.expires) {
		return nil
	}
	return s
}

// redeem swaps a ticket for a session cookie, and goes to the stream page for that game.
func (p *Proxy) redeem(w http.ResponseWriter, r *http.Request, ticket string) {
	p.mu.Lock()
	key := hashed(ticket)
	s, ok := p.tickets[key]
	if ok {
		delete(p.tickets, key)
	}
	p.mu.Unlock()
	if !ok || time.Now().After(s.expires) {
		page(w, http.StatusForbidden, "This link has expired", "Go back to the game's page and press Play again.")
		return
	}
	id, err := randomToken()
	if err != nil {
		page(w, http.StatusInternalServerError, "Something went wrong", "Try again.")
		return
	}
	s.expires = time.Now().Add(sessionLife)
	p.mu.Lock()
	// One stream page per person: an older one for the same person stops working.
	for k, v := range p.sessions {
		if v.User == s.User {
			delete(p.sessions, k)
		}
	}
	p.sessions[hashed(id)] = &s
	p.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: p.Secure, MaxAge: int(sessionLife.Seconds())})
	q := url.Values{"hostId": {s.HostID}, "appId": {strconv.Itoa(s.AppID)}, "dataTransport": {"webrtc"}}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/stream.html?"+q.Encode(), http.StatusFound)
}

// page is a small page of this proxy's own, for when there's nothing else to show.
func page(w http.ResponseWriter, code int, title, text string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>`+title+
		`</title><body style="font:16px system-ui;background:#111;color:#eee;display:grid;place-items:center;min-height:90vh;text-align:center"><main><h1 style="font-size:1.4em">`+title+`</h1><p>`+text+`</p></main>`)
}

// csp is what the bridge's pages need, and no more: its protocol code is WebAssembly, its
// styles are injected at run time, and it decodes in a worker.
const csp = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; media-src blob:; worker-src 'self' blob:; connect-src 'self' ws: wss:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// ServeHTTP is the whole of what reaches the bridge.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.last.Store(time.Now().Unix())
	if t, ok := strings.CutPrefix(r.URL.Path, "/wms-ticket/"); ok && r.Method == http.MethodGet {
		p.redeem(w, r, t)
		return
	}
	s := p.session(r)
	if s == nil {
		page(w, http.StatusForbidden, "Start from your library", "Open the game on your media server and press Play. This page only works from there.")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		if !sameOrigin(r) {
			http.Error(w, "that request came from another site", http.StatusForbidden)
			return
		}
	}
	allowed, err := p.allowed(r, s)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if !allowed {
		if r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/api/") {
			// The stream page's own Exit comes here.
			page(w, http.StatusOK, "The stream has ended", "You can close this tab.")
			return
		}
		http.Error(w, "not allowed", http.StatusForbidden)
		return
	}
	port := p.Upstream()
	if port == 0 {
		page(w, http.StatusServiceUnavailable, "Browser play isn't running", "Go back to the game's page and press Play again.")
		return
	}
	target := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = target.Host
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Del("X-Forwarded-For")
			pr.Out.Header.Del("Forwarded")
			pr.Out.Header.Del("Origin")
			pr.Out.Header.Del("Referer")
			for k := range pr.Out.Header {
				if strings.HasPrefix(strings.ToLower(k), "x-wms") || strings.HasPrefix(strings.ToLower(k), "x-forwarded") {
					pr.Out.Header.Del(k)
				}
			}
			pr.Out.Header.Set(BridgeUserHeader, "wms-u"+strconv.FormatInt(s.User, 10))
		},
		// The bridge streams pairing and host lists a line at a time, so nothing is held back.
		FlushInterval: -1,
		ModifyResponse: func(res *http.Response) error {
			res.Header.Del("Set-Cookie")
			res.Header.Set("Content-Security-Policy", csp)
			res.Header.Set("X-Content-Type-Options", "nosniff")
			res.Header.Set("Referrer-Policy", "no-referrer")
			res.Header.Set("Cross-Origin-Resource-Policy", "same-origin")
			res.Header.Set("Permissions-Policy", "gamepad=(self), fullscreen=(self), keyboard-map=(self)")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "browser play isn't answering", http.StatusBadGateway)
		},
	}
	rp.ServeHTTP(w, r)
}

func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true // not a browser: it can't be carrying someone's cookie by accident
	}
	u, err := url.Parse(o)
	return err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host)
}

// staticBlocked are the bridge's pages that lead anywhere but the one stream.
var staticBlocked = map[string]bool{"": true, "/": true, "/index.html": true, "/admin.html": true}

// allowed decides whether a request goes on. An error is a reason worth showing.
func (p *Proxy) allowed(r *http.Request, s *Session) (bool, error) {
	path := r.URL.Path
	if !strings.HasPrefix(path, "/api/") {
		return (r.Method == http.MethodGet || r.Method == http.MethodHead) && !staticBlocked[path], nil
	}
	host := r.URL.Query().Get("host_id")
	const stream = "/api/host/stream/webrtc"
	switch {
	case r.Method == http.MethodGet && (path == "/api/authenticate" || path == "/api/settings/default" || path == "/api/settings/permissions"):
		return true, nil
	case r.Method == http.MethodGet && path == "/api/user":
		return r.URL.RawQuery == "", nil
	case r.Method == http.MethodGet && path == "/api/role":
		return true, nil
	case r.Method == http.MethodGet && (path == "/api/host" || path == "/api/apps"):
		return subtle.ConstantTimeCompare([]byte(host), []byte(s.HostID)) == 1, nil
	case r.Method == http.MethodOptions && path == stream:
		return true, nil
	case r.Method == http.MethodPost && path == stream:
		return true, p.checkOffer(r, s)
	case (r.Method == http.MethodPatch || r.Method == http.MethodDelete) && strings.HasPrefix(path, stream+"/") && !strings.Contains(path[len(stream)+1:], "/"):
		return true, nil
	}
	return false, nil
}

// ErrWrongGame: a browser asked to stream something other than what its ticket was for.
var ErrWrongGame = errors.New("that isn't the game this page was opened for")

// checkOffer reads the request that starts a stream, and puts it back for the bridge. The
// bridge takes the last copy of each attribute it finds, so more than one is refused rather
// than guessed at.
func (p *Proxy) checkOffer(r *http.Request, s *Session) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxOffer+1))
	if err != nil {
		return err
	}
	if len(body) > maxOffer {
		return errors.New("that request is too big")
	}
	host, app, hostN, appN := parseOffer(body)
	if hostN != 1 || appN != 1 || host != s.HostID || app != strconv.Itoa(s.AppID) {
		return ErrWrongGame
	}
	if p.Hooks.MayStart != nil {
		if err := p.Hooks.MayStart(r.Context(), *s); err != nil {
			return err
		}
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	if p.Hooks.Started != nil {
		p.Hooks.Started(*s)
	}
	return nil
}

// parseOffer finds the rig and game named in an SDP offer, counting every way of writing
// them that a more forgiving parser than this one might still read: other cases, spaces,
// and any line ending.
func parseOffer(sdp []byte) (host, app string, hostN, appN int) {
	for _, line := range strings.FieldsFunc(string(sdp), func(r rune) bool { return r == '\n' || r == '\r' }) {
		line = strings.TrimSpace(line)
		low := strings.ToLower(line)
		switch {
		case strings.HasPrefix(low, "a=x-moonlight-host-id"):
			hostN++
			_, host, _ = strings.Cut(line, ":")
		case strings.HasPrefix(low, "a=x-moonlight-app-id"):
			appN++
			_, app, _ = strings.Cut(line, ":")
		}
	}
	return strings.TrimSpace(host), strings.TrimSpace(app), hostN, appN
}
