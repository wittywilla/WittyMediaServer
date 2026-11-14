// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/themes"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// First run (docs/FIRST-TIME-SETUP.md): fourteen steps in a web browser. Until the last is
// done the server answers every route an app uses with 503 and the code setup_required and
// the address to finish at; only /setup, the status route, the setup routes and the web
// app's own files answer, and the administrator's browser session for the setup's own
// calls. The apps say "This server isn't set up yet" with that address and a QR code.

// SetupSteps are the first run's steps, in order.
var SetupSteps = []string{"welcome", "account", "name", "look", "theme", "modules", "libraries", "details", "livetv", "games", "discs", "people", "remote", "ready"}

// The settings: setupDone is "0" while first run goes on and "1" once it is over (an older
// server that never said has it over when it has an account); the step and the steps done
// are kept beside it.
const (
	settingSetupFlag  = "setupDone"
	settingSetupStep  = "setupStep"
	settingSetupSteps = "setupSteps"
)

// setupComplete reports whether first run is over: marked complete, or never begun on a
// server that already has accounts (one that was set up before the first run had steps).
func (s *Server) setupComplete(ctx context.Context) bool {
	if s.setupFinished.Load() {
		return true
	}
	if flag := s.DB.GetSetting(ctx, settingSetupFlag, ""); flag == "1" || (flag == "" && !s.Auth.NoAccounts(ctx)) {
		s.setupFinished.Store(true)
		return true
	}
	return false
}

// markSetupStarted records that first run has begun, so the server isn't taken for one set
// up long ago when its first account exists.
func (s *Server) markSetupStarted(ctx context.Context) {
	if s.DB.GetSetting(ctx, settingSetupFlag, "") == "" {
		_ = s.DB.SetSetting(ctx, settingSetupFlag, "0")
	}
}

// setupAddress is where first run is finished: this server's own address and /setup.
func setupAddress(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/setup"
}

// setupOpenPaths are the routes that answer while first run is going: the status and health
// routes, the setup's own, the public look routes and the preview.
func setupOpenPath(p string) bool {
	switch {
	case p == "/api/status", p == "/api/health", p == "/api/setup", strings.HasPrefix(p, "/api/setup/"),
		strings.HasPrefix(p, "/api/backup"), strings.HasPrefix(p, "/api/legal"), p == "/api/identity", p == "/api/capabilities", p == "/api/login", p == "/api/logout", p == "/api/sdui/preview", p == "/api/themes", strings.HasPrefix(p, "/api/themes/"), p == "/api/sdui/experiences":
		return true
	}
	return false
}

// setupGate answers 503 setup_required to everything an app uses while first run goes on.
func (s *Server) setupGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if rest, ok := strings.CutPrefix(p, apiPrefix); ok {
			p = "/api/" + rest
		}
		if !strings.HasPrefix(p, "/api/") || setupOpenPath(p) || s.setupComplete(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}
		if s.setupAdminBrowser(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Retry-After", "30")
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "setup_required", "message": "This server isn't set up yet. Finish setup in a web browser.", "setupURL": setupAddress(r),
		})
	})
}

// setupAdminBrowser: the request is the administrator's own browser session, which the setup's
// steps need for their calls. The rule keys on the session cookie, not on the absence of a
// header: the web app signs every call with the same token it holds as the cookie
// (Authorization: Bearer), and that is still one browser session. An app that holds a token but
// no cookie is refused until first run is over, and so is a request whose header carries some
// other token than the cookie's (an app's token beside a browser's cookie).
func (s *Server) setupAdminBrowser(r *http.Request) bool {
	headers := []string{strings.TrimSpace(r.Header.Get("X-WMS-Token"))}
	if h := r.Header.Get("Authorization"); h != "" {
		if len(h) < 7 || !strings.EqualFold(h[:7], "bearer ") {
			return false
		}
		headers = append(headers, strings.TrimSpace(h[7:]))
	}
	for _, c := range r.Cookies() {
		if c.Name != auth.CookieName || c.Value == "" {
			continue
		}
		same := true
		for _, h := range headers {
			same = same && (h == "" || h == c.Value)
		}
		if !same {
			continue
		}
		if u, err := s.Auth.Resolve(r.Context(), c.Value); err == nil && u != nil && u.IsAdmin {
			return true
		}
	}
	return false
}

// SetupState is what GET /api/setup/state answers.
type SetupState struct {
	Complete bool     `json:"complete"`
	Steps    []string `json:"steps"`
	// Step is the one the setup is on; Done the steps finished.
	Step string   `json:"step"`
	Done []string `json:"done"`
	// URL is where to finish it, in a web browser.
	URL string `json:"url"`
}

func (s *Server) setupState(r *http.Request) SetupState {
	ctx := r.Context()
	st := SetupState{Complete: s.setupComplete(ctx), Steps: SetupSteps, Step: s.DB.GetSetting(ctx, settingSetupStep, SetupSteps[0]), Done: []string{}, URL: setupAddress(r)}
	for _, d := range strings.Split(s.DB.GetSetting(ctx, settingSetupSteps, ""), ",") {
		if d != "" {
			st.Done = append(st.Done, d)
		}
	}
	return st
}

func (s *Server) setupRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/setup/state", s.handleSetupState)
	m.HandleFunc("PUT /api/setup/state", s.handleSetupStatePut)
	m.Handle("POST /api/setup/complete", s.requireAdmin(s.handleSetupComplete))
	m.Handle("POST /api/setup/module-steps/skip", s.requireAdmin(s.handleModuleStepSkip))
	m.HandleFunc("GET /api/sdui/preview", s.handleSDUIExperiencePreview)
}

// GET /api/setup/state (public): whether first run is over, its steps and where it is.
func (s *Server) handleSetupState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.setupState(r))
}

// PUT /api/setup/state {step, done} saves where first run is. Before the first account it is
// open to whoever may set the server up (the setup code, as for POST /api/setup); after it,
// to administrators. Not once first run is over.
func (s *Server) handleSetupStatePut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.setupComplete(ctx) {
		writeErr(w, 409, "setup is finished")
		return
	}
	if s.Auth.NoAccounts(ctx) {
		if s.setupNeedsCode(r) && !s.setupCodeOK(r.Header.Get("X-WMS-Setup-Code")) {
			writeErr(w, 403, "Setting up this server from another device takes its setup code.")
			return
		}
	} else if !s.setupAdminBrowser(r) {
		if u, _ := s.signedIn(r); u == nil || !u.IsAdmin {
			writeErr(w, 403, "administrators only")
			return
		}
	}
	var body struct {
		Step string   `json:"step"`
		Done []string `json:"done"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, `expected {"step": "<step>", "done": ["<step>", ...]}`)
		return
	}
	valid := func(id string) bool {
		for _, x := range SetupSteps {
			if x == id {
				return true
			}
		}
		return false
	}
	if !valid(body.Step) {
		writeErr(w, 400, "step: one of "+strings.Join(SetupSteps, ", "))
		return
	}
	var done []string
	for _, d := range body.Done {
		if !valid(d) {
			writeErr(w, 400, "done: one of "+strings.Join(SetupSteps, ", "))
			return
		}
		done = append(done, d)
	}
	s.markSetupStarted(ctx)
	_ = s.DB.SetSetting(ctx, settingSetupStep, body.Step)
	_ = s.DB.SetSetting(ctx, settingSetupSteps, strings.Join(done, ","))
	writeJSON(w, 200, s.setupState(r))
}

// POST /api/setup/complete (administrators): first run is over; apps may connect.
func (s *Server) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.DB.SetSetting(ctx, settingSetupFlag, "1"); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.setupFinished.Store(true)
	s.Log.InfoContext(ctx, "first run finished", "cat", "auth", "by", adminName(r))
	writeJSON(w, 200, s.setupState(r))
}

// GET /api/sdui/preview?experience=<id>[&theme=<id>][&platform=<p>][&formFactor=<f>][&viewport=WxH]:
// a sample Home and menu in an experience and theme, made from generated demo data with no
// library behind it, so the first-run chooser can draw every look with the client's own
// renderer before anyone has signed in. Public, and rate limited.
func (s *Server) handleSDUIExperiencePreview(w http.ResponseWriter, r *http.Request) {
	if d := s.logins.begin("preview|"+addressKey(r), 240); d > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		writeErr(w, 429, "Too many previews. Try again in a moment.")
		return
	}
	exp := sdui.ExperienceByID(sduiPick(r, "experience", "X-WMS-Experience"))
	if exp == nil {
		writeErr(w, 400, "experience: one of "+strings.Join(sdui.ExperienceIDs(), ", "))
		return
	}
	ctx := &sdui.RenderContext{ProfileKind: "adult", Admin: false, UserName: "You", ServerName: s.ServerName(r.Context()), Features: serverFeatures,
		Flags: map[string]bool{}, Vars: map[string]string{}, Params: map[string]string{}, Resolver: sdui.DemoResolver{}, BasePath: "/api/v1/sdui",
		Source: "preview", Now: time.Now(), Experience: exp}
	if err := s.sduiDevice(r, ctx); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ff := ctx.Profile.FormFactor
	themeID := strings.ToLower(strings.TrimSpace(sduiPick(r, "theme", "X-WMS-Theme")))
	if themeID == "" || !s.themeKnown(themeID) {
		themeID = exp.Theme
		if !s.themeKnown(themeID) {
			themeID = "default"
		}
	}
	home, _ := sdui.StockPage("home")
	doc, err := sdui.Render(home, ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	libs := []sdui.NavLibrary{{ID: 1, Name: "Films", Kind: "movies", Count: 12}, {ID: 2, Name: "Shows", Kind: "shows", Count: 8}, {ID: 3, Name: "Music", Kind: "music", Count: 40}}
	nav := sdui.RenderNavigation(sdui.StockNavigation(), func(id string) (sdui.Page, bool) { return sdui.StockPage(id) }, libs, ctx)
	nav.Shell = sdui.ShellFor(exp.ID, ff)
	shape, typ := s.Themes.Look(themeID)
	finish, scene := s.Themes.LookOf(themeID)
	pal := s.baseColors(themeID)
	_, _, parts := s.brand(r.Context())
	writeJSON(w, 200, map[string]any{
		"experience": exp, "document": doc, "navigation": nav,
		"theme": map[string]any{"id": themeID, "tokens": pal, "shape": shape, "type": typ, "finish": finish, "scene": scene,
			"tv": themesTV(s, themeID, pal), "elevation": themesElevation(pal), "brand": map[string]any{"parts": parts, "token": "brandAccent", "color": pal["brandAccent"]}},
	})
}

func themesTV(s *Server, id string, pal map[string]string) any {
	return themes.TVFor(s.Themes.TVColorsOf(id), pal, pal)
}

func themesElevation(pal map[string]string) any { return themes.ElevationFor(pal) }
