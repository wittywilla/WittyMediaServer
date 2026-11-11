package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymodule/service"
)

// WittyPlay's Docker backend: routes on the module's own service that the server passes
// people's requests through to, under /api/modules/wittyplay. The server signs the person in,
// checks administrators' routes itself as well, drops every X-Witty- header the browser sent,
// and tells the module who is asking in X-Witty-User and X-Witty-Admin. WittyPlay checks the
// rest: who owns a session, and the KasmVNC ticket. See docs/MODULES.md, Who is asking.

// wittyPlayBase is where the module's routes are reached from outside.
const wittyPlayBase = "/api/modules/" + wittyPlayID

// kasmCSP lets a KasmVNC page run its own scripts and connect back to its session, and no more.
const kasmCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; " +
	"connect-src 'self'; font-src 'self' data:; media-src 'self' blob:; worker-src 'self' blob:; object-src 'none'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'self'"

func (s *Server) wittyPlayDockerRoutes(m routes) {
	a, ad := s.requireUser, s.requireAdmin
	m.Handle("GET /api/modules/wittyplay/docker/status", ad(s.handleWittyPlayDocker))
	m.Handle("GET /api/modules/wittyplay/docker/settings", ad(s.handleWittyPlayDocker))
	m.Handle("PUT /api/modules/wittyplay/docker/settings", ad(s.handleWittyPlayDocker))
	m.Handle("GET /api/modules/wittyplay/docker/emulators", ad(s.handleWittyPlayDocker))
	m.Handle("GET /api/modules/wittyplay/docker/installed", ad(s.handleWittyPlayDocker))
	m.Handle("POST /api/modules/wittyplay/docker/emulators/{id}/install", ad(s.handleWittyPlayDocker))
	m.Handle("GET /api/modules/wittyplay/docker/emulators/{id}/install", ad(s.handleWittyPlayDocker))
	m.Handle("DELETE /api/modules/wittyplay/docker/emulators/{id}", ad(s.handleWittyPlayDocker))
	m.Handle("POST /api/modules/wittyplay/docker/sessions/{sid}/connect", a(s.handleWittyPlayDocker))
	m.Handle("DELETE /api/modules/wittyplay/docker/sessions/{sid}", a(s.handleWittyPlayDocker))
	// The KasmVNC desktop: its page, its files and its WebSocket.
	m.Handle("GET /api/modules/wittyplay/kasm/{sid}/{rest...}", a(s.handleWittyPlayKasm))
	m.Handle("POST /api/modules/wittyplay/kasm/{sid}/{rest...}", a(s.handleWittyPlayKasm))
	m.Handle("PUT /api/modules/wittyplay/kasm/{sid}/{rest...}", a(s.handleWittyPlayKasm))
	m.Handle("PATCH /api/modules/wittyplay/kasm/{sid}/{rest...}", a(s.handleWittyPlayKasm))
	m.Handle("DELETE /api/modules/wittyplay/kasm/{sid}/{rest...}", a(s.handleWittyPlayKasm))
}

// handleWittyPlayDocker passes one of the Docker backend's routes to WittyPlay.
func (s *Server) handleWittyPlayDocker(w http.ResponseWriter, r *http.Request) {
	s.passToWittyPlay(w, r, nil)
}

// handleWittyPlayKasm passes the KasmVNC desktop through, WebSocket included. The module's
// answers name its own paths (/kasm/<session>), so cookies and redirects are moved under the
// address the browser used.
func (s *Server) handleWittyPlayKasm(w http.ResponseWriter, r *http.Request) {
	ext := r.URL.Path
	if u, err := url.ParseRequestURI(r.RequestURI); err == nil {
		ext = u.Path
	}
	i := strings.Index(ext, "/kasm/")
	if i < 0 {
		writeErr(w, 404, "not found")
		return
	}
	outside := ext[:i] // /api/modules/wittyplay, or /api/v1/modules/wittyplay
	w.Header().Set("Content-Security-Policy", kasmCSP)
	s.passToWittyPlay(w, r, func(resp *http.Response) error {
		rewriteModulePaths(resp, outside)
		return nil
	})
}

// passToWittyPlay sends the request on to the module's service, as the person signed in.
func (s *Server) passToWittyPlay(w http.ResponseWriter, r *http.Request, modify func(*http.Response) error) {
	u := userFrom(r)
	if u == nil {
		writeErr(w, 401, "authentication required")
		return
	}
	if service.Dial == nil {
		writeErr(w, http.StatusFailedDependency, "WittyPlay isn't on this server")
		return
	}
	c, err := service.Dial(wittyPlayID)
	if err != nil {
		if !writeModuleMissing(w, err) {
			s.Log.Warn("modules: reaching WittyPlay", "err", err)
			writeErr(w, http.StatusBadGateway, "the module isn't answering")
		}
		return
	}
	// A WebSocket from another site would carry the sign-in cookie; it isn't let through.
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") && s.crossOrigin(r) {
		writeErr(w, 403, "not from this site")
		return
	}
	out := r.WithContext(modmgr.WithIdentity(r.Context(), u.ID, u.IsAdmin))
	out.Header = withoutSignIn(r.Header)
	if q := r.URL.Query(); q.Has("token") || q.Has("st") {
		q.Del("token")
		q.Del("st")
		u2 := *r.URL
		u2.RawQuery = q.Encode()
		out.URL = &u2
	}
	// The module says how long its answers keep; the server's own default doesn't apply.
	w.Header().Del("Cache-Control")
	path := strings.TrimPrefix(r.URL.Path, wittyPlayBase)
	modmgr.ProxyToService(c, w, out, path, func(err error) {
		s.Log.Warn("modules: passing a request to WittyPlay", "err", err)
	}, modify)
}

// withoutSignIn is a copy of h without this server's sign-in: the module never needs it, and
// the service token and identity headers say all it may know.
func withoutSignIn(h http.Header) http.Header {
	out := h.Clone()
	out.Del("Authorization")
	out.Del("X-WMS-Token")
	if cookies := out.Values("Cookie"); len(cookies) > 0 {
		out.Del("Cookie")
		var keep []string
		for _, line := range cookies {
			for _, part := range strings.Split(line, ";") {
				name, _, _ := strings.Cut(strings.TrimSpace(part), "=")
				if name != "" && name != auth.CookieName {
					keep = append(keep, strings.TrimSpace(part))
				}
			}
		}
		if len(keep) > 0 {
			out.Set("Cookie", strings.Join(keep, "; "))
		}
	}
	return out
}

// rewriteModulePaths moves the paths in a module's answer (a cookie's Path, a redirect) from the
// module's own routes to where the browser reaches them.
func rewriteModulePaths(resp *http.Response, outside string) {
	if loc := resp.Header.Get("Location"); strings.HasPrefix(loc, "/kasm/") || strings.HasPrefix(loc, "/docker/") {
		resp.Header.Set("Location", outside+loc)
	}
	set := resp.Header.Values("Set-Cookie")
	if len(set) == 0 {
		return
	}
	resp.Header.Del("Set-Cookie")
	for _, line := range set {
		c, err := http.ParseSetCookie(line)
		if err != nil {
			continue // a cookie the server can't read isn't passed on
		}
		if c.Name == auth.CookieName {
			continue // a module never sets this server's sign-in
		}
		if c.Path == "" || c.Path == "/" || !strings.HasPrefix(c.Path, "/kasm/") {
			c.Path = outside + "/kasm/"
		} else {
			c.Path = outside + c.Path
		}
		resp.Header.Add("Set-Cookie", c.String())
	}
}
