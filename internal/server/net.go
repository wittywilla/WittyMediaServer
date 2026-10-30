package server

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/remote"
)

// Who a request comes from, and what the server does about that: the first
// thing every request meets. All of it reads one prepared snapshot of the
// settings, so it costs no database lookups.

// netState is the part of the remote access settings requests need, rebuilt
// whenever they change.
type netState struct {
	policy     *remote.Policy
	on         bool // outside connections are allowed
	forced     bool // ... because the config file or command line says so
	cfg        remoteSettings
	publicURL  string // where the server is reached from outside ("" = unknown)
	publicHost string
	capBits    int64  // stream limit away from home, in bits per second
	capTag     string // names those limits in stream IDs
}

func (s *Server) state() *netState {
	if st := s.netState.Load(); st != nil {
		return st
	}
	return &netState{}
}

// clientFrom is the request's sender, as identify decided. A request that did
// not pass through it believes no forwarded header.
func clientFrom(r *http.Request) remote.Client {
	if c, ok := r.Context().Value(clientKey).(remote.Client); ok {
		return c
	}
	return (*remote.Policy)(nil).Resolve(r)
}

func (s *Server) identify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := s.state().policy.Resolve(r)
		if !c.Where.AtHome() {
			s.outside.note(c)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientKey, c)))
	})
}

// gate keeps everything away from home out until the owner turned remote
// access on, whatever the router or the firewall happens to let through. Once
// on, the internet is only welcome over HTTPS: a password must never cross it
// in the clear. (A VPN encrypts by itself.)
func (s *Server) gate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := clientFrom(r)
		if c.Where.AtHome() {
			next.ServeHTTP(w, r)
			return
		}
		st := s.state()
		if !st.on {
			s.refuse(w, r, "This server is only available on its home network.")
			return
		}
		// An unverified proxy's word is good enough here: the only person
		// fooled by a false claim of HTTPS is whoever sent it.
		if c.Where == remote.Internet && !c.Secure && !(c.Unverified && remote.SecureHint(r)) {
			s.outside.notePlain()
			if (r.Method == http.MethodGet || r.Method == http.MethodHead) && strings.HasPrefix(st.publicURL, "https://") {
				http.Redirect(w, r, st.publicURL+r.URL.RequestURI(), http.StatusPermanentRedirect)
				return
			}
			s.refuse(w, r, "This server only accepts encrypted connections from outside its home network.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) refuse(w http.ResponseWriter, r *http.Request, msg string) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, 403, msg)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(403)
	_, _ = w.Write([]byte(msg + "\n"))
}

// touchSession keeps the list of signed-in devices up to date.
func (s *Server) touchSession(r *http.Request, token string) {
	c := clientFrom(r)
	p := r.URL.Path
	streaming := strings.HasPrefix(p, "/api/hls/") || strings.HasSuffix(p, "/file") || strings.HasSuffix(p, "/stream")
	s.Auth.Touch(r.Context(), token, c.IP.String(), c.Where.String(), viaName(c, r), streaming)
}

func viaName(c remote.Client, r *http.Request) string {
	switch {
	case c.Proxied || c.Unverified:
		return "proxy"
	case r.TLS != nil:
		return "https"
	}
	return "http"
}

// ---------------------------------------------------------------- limits on streams

// remoteCaps returns the limits for a viewer away from home (none at home).
func (s *Server) remoteCaps(r *http.Request) (maxHeight int, rate string) {
	if clientFrom(r).Where.AtHome() {
		return 0, ""
	}
	st := s.state()
	return st.cfg.MaxHeight, st.cfg.MaxBitrate
}

// fitsCap: a file can be sent as it is without going over the limit.
func (s *Server) fitsCap(r *http.Request, size int64, seconds float64) bool {
	_, rate := s.remoteCaps(r)
	limit := ffmpeg.ParseRate(rate)
	if limit <= 0 {
		return true
	}
	return seconds > 0 && size > 0 && float64(size)*8/seconds <= float64(limit)
}

// capWriter enforces the limit on what is actually sent, so a file can't be
// fetched faster than the limit by asking for it directly. A little room stays
// above the limit: streams made to the limit can't go over it anyway, and
// players need to get ahead of what they show.
func (s *Server) capWriter(w http.ResponseWriter, r *http.Request) http.ResponseWriter {
	_, rate := s.remoteCaps(r)
	if limit := ffmpeg.ParseRate(rate); limit > 0 {
		return remote.Throttle(r.Context(), w, limit+limit/4)
	}
	return w
}

// capTag names a viewer's limits for disc stream IDs (nothing for no limits).
func (s *Server) capTag(r *http.Request) string {
	if h, rate := s.remoteCaps(r); h > 0 || rate != "" {
		return s.state().capTag
	}
	return ""
}

// ---------------------------------------------------------------- what was seen from outside

// outsideSeen remembers the latest connections from away from home, the
// evidence behind the status in Settings. Memory only: addresses are not kept.
type outsideSeen struct {
	mu        sync.Mutex
	internet  time.Time
	vpn       time.Time
	proxied   time.Time
	plain     time.Time
	unverPeer string // a machine that passes on requests without being trusted
	unverAt   time.Time
}

func (o *outsideSeen) note(c remote.Client) {
	now := time.Now()
	o.mu.Lock()
	defer o.mu.Unlock()
	switch {
	case c.Where == remote.VPN:
		o.vpn = now
	case c.Where == remote.Internet && c.Secure && !c.Unverified:
		// Only this is evidence that the way in works: a plain or forged
		// request proves nothing, and can even be sent from inside the home.
		o.internet = now
	}
	if c.Proxied {
		o.proxied = now
	}
	// Only a machine on the home network can be a proxy worth suggesting: on
	// the internet, anyone can send these headers to have their address shown
	// to the owner as something to trust.
	if c.Unverified && (&remote.Policy{}).Classify(c.Peer).AtHome() {
		o.unverPeer, o.unverAt = c.Peer.String(), now
	}
}

func (o *outsideSeen) notePlain() {
	o.mu.Lock()
	o.plain = time.Now()
	o.mu.Unlock()
}

func unixOr0(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
