// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymodule/service"
)

// WittyPlay's standalone emulators: routes on the module's own service that the server passes
// people's requests through to, under /api/modules/wittyplay. The server signs the person in,
// checks administrators' routes itself as well, drops every X-Witty- header the browser sent,
// and tells the module who is asking in X-Witty-User and X-Witty-Admin. WittyPlay checks the
// rest: who owns a session, whose settings a request changes, and the KasmVNC ticket. See
// docs/MODULES.md, Who is asking.
//
// Starting a game is not among them. That's the server's own route (api_games_emulators.go),
// because only the server knows whether an account may play a game and where its file is.

// wittyPlayBase is where the module's routes are reached from outside.
const wittyPlayBase = "/api/modules/" + wittyPlayID

// kasmCSP is the content policy of a KasmVNC desktop's page: its own scripts and its own
// connection, in a sandbox with an origin of its own.
//
// The page and its scripts come out of the container, and a container is treated as something
// that may have been taken over. Under the server's own origin such a page could read the app's
// storage and ask the API anything as the person signed in, so it never runs there: "sandbox"
// without allow-same-origin gives it an origin that is nobody's. That's the same rule every
// other answer under /api/ follows (apiCSP), with scripts allowed.
const kasmCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; " +
	"connect-src 'self'; font-src 'self' data:; media-src 'self' blob:; worker-src 'self' blob:; object-src 'none'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'self'; sandbox allow-scripts allow-pointer-lock"

func (s *Server) wittyPlayDockerRoutes(m routes) {
	a, ad := s.requireUser, s.requireAdmin
	m.Handle("GET /api/modules/wittyplay/docker/status", ad(s.handleWittyPlayDocker))
	m.Handle("GET /api/modules/wittyplay/docker/diagnostics", ad(s.handleWittyPlayDocker))
	m.Handle("GET /api/modules/wittyplay/docker/settings", ad(s.handleWittyPlayDocker))
	m.Handle("PUT /api/modules/wittyplay/docker/settings", ad(s.handleWittyPlayDocker))
	m.Handle("GET /api/modules/wittyplay/docker/emulators", ad(s.handleWittyPlayDocker))
	m.Handle("GET /api/modules/wittyplay/docker/installed", ad(s.handleWittyPlayDocker))
	m.Handle("POST /api/modules/wittyplay/docker/emulators/{id}/install", ad(s.handleWittyPlayDocker))
	m.Handle("GET /api/modules/wittyplay/docker/emulators/{id}/install", ad(s.handleWittyPlayDocker))
	m.Handle("DELETE /api/modules/wittyplay/docker/emulators/{id}/install", ad(s.handleWittyPlayDocker))
	m.Handle("DELETE /api/modules/wittyplay/docker/emulators/{id}", ad(s.handleWittyPlayDocker))
	// An emulator's settings: anyone's own, and an administrator's for everyone. WittyPlay
	// decides which a request may change.
	m.Handle("GET /api/modules/wittyplay/docker/emulators/{id}/settings", a(s.handleWittyPlayDocker))
	m.Handle("PUT /api/modules/wittyplay/docker/emulators/{id}/settings", a(s.handleWittyPlayDocker))
	m.Handle("GET /api/modules/wittyplay/docker/sessions", a(s.handleWittyPlayDocker))
	m.Handle("POST /api/modules/wittyplay/docker/sessions/{sid}/connect", a(s.handleWittyPlayDocker))
	m.Handle("POST /api/modules/wittyplay/docker/sessions/{sid}/pause", a(s.handleWittyPlayDocker))
	m.Handle("POST /api/modules/wittyplay/docker/sessions/{sid}/resume", a(s.handleWittyPlayDocker))
	m.Handle("DELETE /api/modules/wittyplay/docker/sessions/{sid}", a(s.handleWittyPlayDocker))
	// The KasmVNC desktop: its page, its files and its WebSocket. A browser sends no sign-in
	// from the sandboxed frame it's shown in, so this route can't ask for one: the session's
	// ticket in the address is the way in, and WittyPlay checks it. Reading only.
	m.Handle("GET /api/modules/wittyplay/kasm/{sid}/{ticket}/{rest...}", http.HandlerFunc(s.handleWittyPlayKasm))
	s.gameEmulatorRoutes(m)
}

// Limits on what passes through to WittyPlay's JSON routes. None of them carries anything
// large: the biggest request is a WebRTC offer, and the biggest answer the list of emulators.
// The KasmVNC desktop is a stream, and has neither limit.
const (
	wittyPlayMaxRequest  = 256 << 10
	wittyPlayMaxResponse = 4 << 20
	wittyPlayTimeout     = 45 * time.Second
)

// headerSystemDir tells WittyPlay where this server's BIOS folder is, so its pages can say
// which firmware is there. It's set here on every request, over whatever a client sent.
const headerSystemDir = "X-Wittyplay-System-Dir"

// handleWittyPlayDocker passes one of the Docker backend's routes to WittyPlay.
func (s *Server) handleWittyPlayDocker(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > wittyPlayMaxRequest {
		writeErr(w, http.StatusRequestEntityTooLarge, "that request is too large")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, wittyPlayMaxRequest)
	ctx, cancel := context.WithTimeout(r.Context(), wittyPlayTimeout)
	defer cancel()
	s.passToWittyPlay(w, r.WithContext(ctx), func(resp *http.Response) error {
		if resp.ContentLength > wittyPlayMaxResponse {
			return errors.New("the module's answer is too large")
		}
		resp.Body = &cappedBody{ReadCloser: resp.Body, left: wittyPlayMaxResponse}
		// A module's answer is data for this server's own pages, never a page of its own.
		resp.Header.Del("Set-Cookie")
		resp.Header.Set("X-Content-Type-Options", "nosniff")
		return nil
	})
}

// cappedBody stops a module's answer at a size.
type cappedBody struct {
	io.ReadCloser
	left int64
}

func (c *cappedBody) Read(p []byte) (int, error) {
	if c.left <= 0 {
		return 0, errors.New("the module's answer is too large")
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.ReadCloser.Read(p)
	c.left -= int64(n)
	return n, err
}

// kasmName is the shape of a session's name and of its ticket: hex, as WittyPlay makes them.
// Anything else isn't passed on at all.
func kasmName(v string, least, most int) bool {
	if len(v) < least || len(v) > most {
		return false
	}
	for _, c := range v {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// handleWittyPlayKasm passes the KasmVNC desktop through, WebSocket included, to whoever holds
// the session's ticket. Nobody's identity goes with it, and nothing of this server's sign-in:
// the page it serves is not one of the server's own. Whatever comes back is boxed in (kasmCSP),
// sets no cookie, and redirects only within the session's own address.
func (s *Server) handleWittyPlayKasm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", kasmCSP)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	if !kasmName(r.PathValue("sid"), 8, 64) || !kasmName(r.PathValue("ticket"), 64, 64) {
		writeErr(w, 404, "not found")
		return
	}
	ext := r.URL.Path
	if u, err := url.ParseRequestURI(r.RequestURI); err == nil {
		ext = u.Path
	}
	i := strings.Index(ext, "/kasm/")
	if i < 0 || service.Dial == nil {
		writeErr(w, 404, "not found")
		return
	}
	outside := ext[:i] // /api/modules/wittyplay, or /api/v1/modules/wittyplay
	c, err := service.Dial(wittyPlayID)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	// The desktop's own WebSocket comes from its sandboxed frame, whose origin is "null", or
	// from an app with none. One from another site's page is that site's, and isn't let through.
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		if o := r.Header.Get("Origin"); o != "" && o != "null" && s.crossOrigin(r) {
			writeErr(w, 403, "not from this site")
			return
		}
	}
	out := r.Clone(r.Context())
	out.Header.Del("Authorization")
	out.Header.Del("X-WMS-Token")
	out.Header.Del("Cookie")
	out.Header.Del(headerSystemDir)
	if out.URL.RawQuery != "" {
		q := out.URL.Query()
		q.Del("token")
		q.Del("st")
		out.URL.RawQuery = q.Encode()
	}
	modmgr.ProxyToService(c, w, out, strings.TrimPrefix(r.URL.Path, wittyPlayBase), func(err error) {
		s.Log.Warn("modules: passing a desktop to WittyPlay", "err", err)
	}, func(resp *http.Response) error {
		resp.Header.Del("Set-Cookie")
		resp.Header.Del("Content-Security-Policy") // the one set above stands, alone
		resp.Header.Del("Referrer-Policy")
		resp.Header.Del("Cache-Control")
		resp.Header.Del("Access-Control-Allow-Origin")
		resp.Header.Del("Access-Control-Allow-Credentials")
		if loc := resp.Header.Get("Location"); loc != "" {
			if strings.HasPrefix(loc, "/kasm/"+r.PathValue("sid")+"/"+r.PathValue("ticket")+"/") {
				resp.Header.Set("Location", outside+loc)
			} else if strings.HasPrefix(loc, "/") || strings.Contains(loc, ":") {
				resp.Header.Del("Location")
			}
		}
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
	out.Header.Set(headerSystemDir, s.games().systemDir())
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
