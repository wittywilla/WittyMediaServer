// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/remote"
	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittymodule/service"
)

// headerLimited, headerReached and headerMachine are told to a module's routes beside
// WittyModule's own (service.Who has no field for them yet; modules read them by name).
const (
	headerLimited = "X-Witty-Limited"
	headerReached = "X-Witty-Reached"
	headerMachine = "X-Witty-Machine"
)

// Module routes (module API 5, docs/MODULES-API5.md). A module's integration.json lists the
// HTTP routes it answers. The server mounts each at /api/modules/<id><path> (and /api/v1/...),
// checks who may call it, the profile's features and screen time and the item it names, then
// passes the request to the module's service at /http<path>, saying who is asking. The module
// never sees this server's sign-in, and never a file path a browser chose.

// Defaults for a module route that sets no limits of its own.
const (
	moduleRouteBody   = 1 << 20
	moduleRouteAnswer = 16 << 20
	moduleRouteTime   = time.Minute
)

// moduleRouting is what the modules in use answer, made once for each set of versions in use.
type moduleRouting struct {
	gen    uint64
	direct map[string]*http.ServeMux            // module → its own routes
	alias  map[string]map[string]moduleRouteRef // module → "METHOD /api/..." of the server's copy → the route
	routes map[string]map[string]moduleRouteRef // module → "METHOD /api/modules/<id><path>" → the route
	names  map[string]*wittymodule.Integration  // module → its integration
}

type moduleRouteRef struct {
	module  string
	feature string
	route   wittymodule.Route
}

var routingMu sync.Mutex

// routing is what the modules in use answer now.
func (s *Server) routing() *moduleRouting {
	integrations := s.integrations
	if integrations == nil {
		if s.Modules == nil {
			return &moduleRouting{}
		}
		integrations = s.Modules.Integrations
	}
	in, gen := integrations()
	routingMu.Lock()
	defer routingMu.Unlock()
	if rt := s.modRouting; rt != nil && rt.gen == gen {
		return rt
	}
	rt := &moduleRouting{gen: gen, direct: map[string]*http.ServeMux{}, names: map[string]*wittymodule.Integration{}}
	for _, u := range in {
		mux := http.NewServeMux()
		rt.names[u.ID] = u.Integration
		for _, r := range u.Integration.HTTP {
			ref := moduleRouteRef{module: u.ID, feature: u.Integration.Feature, route: r}
			for _, meth := range r.Methods {
				mux.Handle(meth+" /api/modules/"+u.ID+r.Path, s.moduleRouteHandler(ref))
			}
		}
		rt.direct[u.ID] = mux
	}
	s.modRouting = rt
	return rt
}

// handleModuleRoute answers /api/modules/{id}/{path...}: one of a module's own routes.
func (s *Server) handleModuleRoute(w http.ResponseWriter, r *http.Request) {
	mux := s.routing().direct[r.PathValue("id")]
	if mux == nil {
		// A module that isn't here, or isn't turned on: 424 says which, so an app can say so and
		// offer to add it. Who isn't signed in learns nothing about which modules are here.
		s.requireUser(func(w http.ResponseWriter, _ *http.Request) {
			if s.Modules == nil || !writeModuleMissing(w, s.Modules.Missing(r.PathValue("id"))) {
				writeErr(w, 404, "not found")
			}
		}).ServeHTTP(w, r)
		return
	}
	if _, pattern := mux.Handler(r); pattern == "" {
		s.requireUser(func(w http.ResponseWriter, _ *http.Request) { writeErr(w, 404, "not found") }).ServeHTTP(w, r)
		return
	}
	mux.ServeHTTP(w, r)
}

// classKey carries a module route's class for parental controls (parentalGate), which can't be
// read from the pattern of a route the server doesn't know.
type classKey struct{}

// moduleRouteHandler checks a request for one module route and passes it on.
func (s *Server) moduleRouteHandler(ref moduleRouteRef) http.Handler {
	pass := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.passToModule(w, r, ref) })
	switch ref.route.Auth {
	case wittymodule.AuthTicket:
		return pass
	case wittymodule.AuthAdmin:
		return s.classed(ref, s.requireAdmin(pass))
	default:
		return s.classed(ref, s.requireUser(pass))
	}
}

// classed has parental controls class a module's route by what its integration says.
func (s *Server) classed(ref moduleRouteRef, next http.Handler) http.Handler {
	c := routeClass{feature: ref.feature}
	if ref.route.Plays {
		c.time = timePlay
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.time == timePlay && r.Method == http.MethodDelete {
			c := c
			c.time = timeNone // leaving is always allowed
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), classKey{}, c)))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), classKey{}, c)))
	})
}

// modulePath is the path on the module's service for a request: its route's path with the
// request's parameters in it, under /http.
func modulePath(r *http.Request, route wittymodule.Route) (string, error) {
	var b strings.Builder
	b.WriteString("/http")
	rest := route.Path
	for {
		i := strings.IndexByte(rest, '{')
		if i < 0 {
			b.WriteString(rest)
			break
		}
		j := strings.IndexByte(rest[i:], '}')
		if j < 0 {
			return "", errors.New("a broken route")
		}
		b.WriteString(rest[:i])
		name := strings.TrimSuffix(rest[i+1:i+j], "...")
		v := r.PathValue(name)
		if v == "" && !strings.HasSuffix(rest[i+1:i+j], "...") {
			return "", fmt.Errorf("no %s", name)
		}
		if strings.Contains(v, "..") {
			return "", errors.New("not a path")
		}
		b.WriteString(v)
		rest = rest[i+j+1:]
	}
	return b.String(), nil
}

// passToModule sends a checked request on to a module's service.
// The headers a request carries the person's password and code in, for a route with "confirm".
const (
	confirmPassword = "X-WMS-Confirm-Password"
	confirmCode     = "X-WMS-Confirm-Code"
)

func (s *Server) passToModule(w http.ResponseWriter, r *http.Request, ref moduleRouteRef) {
	route := ref.route
	upgrade := strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
	if upgrade && route.Upgrade != "websocket" {
		writeErr(w, 400, "this address doesn't take a WebSocket")
		return
	}
	// A WebSocket from another site's page would carry the sign-in cookie; it isn't let through.
	if upgrade && route.Auth != wittymodule.AuthTicket && s.crossOrigin(r) {
		writeErr(w, 403, "not from this site")
		return
	}
	path, err := modulePath(r, route)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	u := userFrom(r)
	ctx := r.Context()
	var item *service.Item
	if route.Item != "" {
		id, err := strconv.ParseInt(r.PathValue(route.Item), 10, 64)
		if err != nil {
			writeErr(w, 404, "not found")
			return
		}
		it, err := s.visibleItem(r, id)
		if err != nil {
			writeErr(w, 404, "not found")
			return
		}
		item = s.moduleItemFull(ctx, it)
	}

	// Limits: what a request may carry, and how long an answer may take, unless it's a stream.
	body, answer, limit := int64(moduleRouteBody), int64(moduleRouteAnswer), moduleRouteTime
	if l := route.Limits; l != nil {
		if n, err := wittymodule.ParseSize(l.Body); err == nil {
			body = n
		}
		if n, err := wittymodule.ParseSize(l.Answer); err == nil {
			answer = n
		}
		if d, err := time.ParseDuration(l.Time); err == nil {
			limit = d
		}
	}
	if r.ContentLength > body {
		writeErr(w, http.StatusRequestEntityTooLarge, "that request is too large")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, body)
	if !route.Stream && !upgrade {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, limit)
		defer cancel()
	}

	// A route with "confirm": the person's password, checked here; the module hears only that it
	// was right. A wrong one is refused here, and counts as a failed sign-in.
	confirmed := false
	if pw := r.Header.Get(confirmPassword); pw != "" && route.Confirm && u != nil && route.Auth != wittymodule.AuthTicket {
		if len(pw) > 4<<10 || !s.confirmOwner(w, r, pw, r.Header.Get(confirmCode)) {
			if len(pw) > 4<<10 {
				writeErr(w, 400, "that password is too long")
			}
			s.writeAudit(ctx, u, ref.module+" password-refused", ref.module, "password-refused", "", r.Method+" "+route.Path)
			return
		}
		confirmed = true
	}

	if service.Dial == nil {
		writeErr(w, http.StatusFailedDependency, "modules aren't available on this server")
		return
	}
	c, err := service.Dial(ref.module)
	if err != nil {
		if !writeModuleMissing(w, err) {
			s.Log.WarnContext(ctx, "could not reach a module", "cat", "modules", "module", ref.module, "err", err)
			writeErr(w, http.StatusBadGateway, "the module isn't answering")
		}
		return
	}

	out := r.WithContext(ctx)
	out.Header = withoutSignIn(r.Header)
	if route.Auth == wittymodule.AuthTicket {
		out.Header.Del("Cookie") // nothing of anyone's sign-in, and nobody's identity
	} else if u != nil {
		out = out.WithContext(modmgr.WithIdentity(ctx, u.ID, u.IsAdmin))
	}
	// Whatever the browser sent under these names never arrives (ProxyToService drops every
	// X-Witty- header); these are set afresh after that, on the way out, by the director below.
	extra := http.Header{}
	if item != nil {
		extra.Set(service.HeaderItem, service.EncodeItem(*item))
	}
	if u != nil && route.Auth != wittymodule.AuthTicket {
		extra.Set(service.HeaderFeatures, strings.Join(s.featuresOf(r, u), " "))
		extra.Set(service.HeaderKind, u.Kind) // adult, kids or guest: a guest can only look
		if confirmed {
			extra.Set(service.HeaderConfirmed, "1")
		}
		if h := s.signInHandle(sessionOf(r)); h != "" {
			extra.Set(service.HeaderSignIn, h)
		}
		if !clientFrom(r).Where.AtHome() {
			extra.Set(service.HeaderAway, "1")
		}
		// A profile with limits set on it (WittyCouch keeps a PC's desktop from it).
		if a := accessFor(u); a != nil && a.Limited() {
			extra.Set(headerLimited, "1")
		}
		// The name this device reached the server by, and whether it is this machine itself:
		// what a module that hands out addresses (WittyRigs: this machine's own Sunshine) needs.
		extra.Set(headerReached, r.Host)
		if clientFrom(r).Where == remote.Machine {
			extra.Set(headerMachine, "1")
		}
	}
	// A query's sign-in (a stream token) isn't the module's.
	if q := r.URL.Query(); q.Has("token") || q.Has("st") {
		q.Del("token")
		q.Del("st")
		u2 := *r.URL
		u2.RawQuery = q.Encode()
		out.URL = &u2
	}
	w.Header().Del("Cache-Control") // the module says how long its answers keep
	if route.CSP == "sandboxed-page" {
		w.Header().Set("Content-Security-Policy", kasmCSP)
		w.Header().Set("Referrer-Policy", "no-referrer")
	}
	modmgr.ProxyToServiceWith(c, w, out, path, extra, func(err error) {
		s.Log.WarnContext(ctx, "could not pass a request to a module", "cat", "modules", "module", ref.module, "err", redactPath(err.Error()))
	}, func(resp *http.Response) error {
		resp.Header.Del("Set-Cookie")
		resp.Header.Set("X-Content-Type-Options", "nosniff")
		if route.CSP == "sandboxed-page" {
			resp.Header.Del("Content-Security-Policy")
		}
		if !route.Stream && resp.StatusCode != http.StatusSwitchingProtocols {
			if resp.ContentLength > answer {
				return errors.New("the module's answer is too large")
			}
			resp.Body = &cappedBody{ReadCloser: resp.Body, left: answer}
		}
		return nil
	})
}

// featuresOf is the features this profile may use now.
func (s *Server) featuresOf(r *http.Request, u *auth.User) []string {
	a := accessFor(u)
	var out []string
	for _, f := range library.Features {
		if a == nil || a.Can(f) {
			out = append(out, f)
		}
	}
	return out
}

// moduleItem describes an item to a module: what it is, and its files to read.
func moduleItem(it *library.Item) *service.Item {
	out := &service.Item{ID: it.ID, Library: it.LibraryID, Kind: it.Kind, Title: it.Title}
	if it.Console != "" {
		out.Facets = map[string]string{"console": it.Console}
	}
	if it.Path != "" {
		f := service.File{Path: it.Path}
		if st, err := os.Stat(it.Path); err == nil && !st.IsDir() {
			f.Size = st.Size()
		}
		out.Files = []service.File{f}
	}
	return out
}

// moduleItemFull is moduleItem with what only a single item's answer carries: a game's discs, each
// as one of its files, in order, and the file in its archive someone chose (facet "entry").
func (s *Server) moduleItemFull(ctx context.Context, it *library.Item) *service.Item {
	out := moduleItem(it)
	if it.Kind != "game" || s.Store == nil {
		return out
	}
	if discs, err := s.Store.GameDiscs(ctx, it.ID); err == nil && len(discs) > 1 {
		out.Files = out.Files[:0]
		for _, d := range discs {
			f := service.File{Path: d.Path}
			if st, err := os.Stat(d.Path); err == nil && !st.IsDir() {
				f.Size = st.Size()
			}
			out.Files = append(out.Files, f)
		}
	}
	set := func(k, v string) {
		if v == "" {
			return
		}
		if out.Facets == nil {
			out.Facets = map[string]string{}
		}
		out.Facets[k] = v
	}
	set("entry", s.Store.GameEntry(ctx, it.ID))
	set("serial", s.Store.GameSerial(ctx, it.ID)) // what the scan read in the file, or its name
	return out
}

// IntegrationCheck holds a module's integration.json to what this server can do.
func IntegrationCheck(id string, in *wittymodule.Integration) error {
	if in.Feature != "" && !slices.Contains(library.Features, in.Feature) {
		return fmt.Errorf("it belongs to a feature this server doesn't know, %q", in.Feature)
	}
	return nil
}

// withoutSignIn is a copy of h without this server's sign-in: the module never needs it, and
// the service token and identity headers say all it may know.
func withoutSignIn(h http.Header) http.Header {
	out := h.Clone()
	out.Del("Authorization")
	out.Del("X-WMS-Token")
	out.Del(confirmPassword) // checked by the server on a "confirm" route; never the module's
	out.Del(confirmCode)
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

// kasmCSP is for a module's page that runs someone else's code, such as an emulator's desktop
// that may have been taken over. Under the server's own origin such a page could read the app's
// storage and ask the API anything as the person signed in, so it never runs there: "sandbox"
// without allow-same-origin gives it an origin that is nobody's. That's the same rule every
// other answer under /api/ follows (apiCSP), with scripts allowed.
const kasmCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; " +
	"connect-src 'self'; font-src 'self' data:; media-src 'self' blob:; worker-src 'self' blob:; object-src 'none'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'self'; sandbox allow-scripts allow-pointer-lock"

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
