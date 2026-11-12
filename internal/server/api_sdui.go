// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/bufpool"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/pace"
	"github.com/wittywilla/wittymediaserver/internal/sduistore"
	"github.com/wittywilla/wittymediaserver/internal/themes"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// Server-driven UI: the pages and navigation in ui_config.json (internal/sduistore) over
// the built-in ones, rendered by pkg/sdui for whoever asks. docs/SDUI.md, "HTTP API",
// describes the routes. What every app shows is here; what an administrator edits it with
// is in api_sdui_admin.go.

// sduiState is the store, opened when first wanted, and what is kept between requests:
// renders for diffs, a few seconds of finished renders, and preview tokens.
type sduiState struct {
	once  sync.Once
	store *sduistore.Store
	err   error

	mu      sync.Mutex
	renders map[string]*list.Element // user id + etag → rendered document
	order   *list.List
	recent  map[string]recentRender // request → a render finished moments ago
	preview map[string]previewToken

	personalMu sync.Mutex // one profile's arrangement is read, changed and written back whole

	mods sduiModules // the modules' SDUI, merged (sdui_modules.go)
}

type keptRender struct {
	key string
	uid int64
	doc sdui.Document
}

type recentRender struct {
	at  time.Time
	uid int64
	doc sdui.Document
}

type previewToken struct {
	uid     int64
	expires time.Time
}

// keptRendersFor bounds the renders kept so a client can ask for a diff against one. A
// render holds a page's worth of titles, up to a few hundred kilobytes, so a small machine
// keeps few; a client whose render has gone is sent the whole page, which is always right.
// One account keeps at most maxKeptPerAccount, so one busy client can't push out everyone
// else's.
func keptRendersFor(c pace.Class) int {
	switch c {
	case pace.Tiny:
		return 12
	case pace.Small:
		return 32
	}
	return 128
}

const (
	maxKeptPerAccount = 6
	// recentRenderFor is how long a finished render answers the same request again: long
	// enough that a page and its diff, or an app's burst of requests as it starts, are one
	// render; short enough that nobody sees a row that is out of date.
	recentRenderFor  = 3 * time.Second
	maxPreviewTokens = 16
	previewTokenFor  = 2 * time.Hour
)

// uiStore opens ui_config.json the first time it is needed.
func (s *Server) uiStore() (*sduistore.Store, error) {
	st := &s.sdui
	st.once.Do(func() {
		if s.Cfg == nil || s.Cfg.DataDir == "" {
			st.err = errors.New("no data folder for the UI configuration")
			return
		}
		log := s.Log
		if log == nil {
			log = slog.Default()
		}
		st.store, st.err = sduistore.Open(s.Cfg.DataDir, sduistore.Options{Log: log, CheckTheme: checkThemeSection})
	})
	return st.store, st.err
}

// checkThemeSection: the overrides in ui_config.json name colours themes have, as colours.
func checkThemeSection(t sduistore.Theme) []sdui.Issue {
	var out []sdui.Issue
	check := func(path string, toks map[string]string) {
		for k, v := range toks {
			if !themes.IsColorToken(k) {
				out = append(out, sdui.Issue{Path: path + "/" + k, Severity: "error", Message: "not a colour token themes have"})
			}
			if _, ok := themes.NormalizeColor(v); !ok {
				out = append(out, sdui.Issue{Path: path + "/" + k, Severity: "error", Message: "not a colour: use #rrggbb, #rrggbbaa or rgb()/rgba()"})
			}
		}
	}
	check("/theme/tokens", t.Tokens)
	for mod, toks := range t.Modules {
		check("/theme/modules/"+mod, toks)
	}
	return out
}

func (s *Server) sduiRoutes(m *http.ServeMux) {
	m.Handle("GET /api/sdui/catalog", s.requireUser(s.handleSDUICatalog))
	m.Handle("GET /api/sdui/navigation", s.requireUser(s.handleSDUINavigation))
	m.Handle("GET /api/sdui/navigation/personal", s.requireUser(s.handleSDUIPersonalGet))
	m.Handle("PUT /api/sdui/navigation/personal", s.requireUser(s.handleSDUIPersonalPut))
	m.Handle("DELETE /api/sdui/navigation/personal", s.requireUser(s.handleSDUIPersonalDelete))
	m.Handle("GET /api/sdui/pages/{page}", s.requireUser(s.handleSDUIPage))
	m.Handle("GET /api/sdui/pages/{page}/diff", s.requireUser(s.handleSDUIDiff))
	m.Handle("GET /api/sdui/pages/{page}/sections/{section}/items", s.requireUser(s.handleSDUISection))
	m.Handle("GET /api/sdui/pages/{page}/personal", s.requireUser(s.handleSDUIPersonalGet))
	m.Handle("PUT /api/sdui/pages/{page}/personal", s.requireUser(s.handleSDUIPersonalPut))
	m.Handle("DELETE /api/sdui/pages/{page}/personal", s.requireUser(s.handleSDUIPersonalDelete))
	m.Handle("GET /api/sdui/topshelf", s.requireUser(s.handleSDUITopShelf))
	s.sduiAdminRoutes(m)
	s.sduiModuleRoutes(m)
	s.sduiLayoutRoutes(m)
}

// writeJSONTagged is writeJSON for answers a client may keep: an ETag, and a cache that
// keeps the answer only for this account and asks again each time.
func writeJSONTagged(w http.ResponseWriter, code int, etag string, v any) {
	b := bufpool.Get()
	defer bufpool.Put(b)
	enc := json.NewEncoder(b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		writeErr(w, 500, "Something went wrong on the server. It is in the log.")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "private, no-cache")
	if etag != "" {
		h.Set("ETag", etag)
	}
	w.WriteHeader(code)
	_, _ = w.Write(b.Bytes())
}

// notModified answers 304 when the client already has etag.
func notModified(w http.ResponseWriter, r *http.Request, etag string) bool {
	if inm := r.Header.Get("If-None-Match"); inm != "" && sduistore.SameETag(inm, etag) {
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "private, no-cache")
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

func hashETag(prefix string, v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return `W/"` + prefix + "-" + hex.EncodeToString(sum[:])[:32] + `"`
}

var (
	sduiCapWord  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]{0,63}$`)
	sduiLocale   = regexp.MustCompile(`^[A-Za-z]{2,8}([-_][A-Za-z0-9]{1,8}){0,3}$`)
	sduiViewport = regexp.MustCompile(`^([0-9]{1,5})x([0-9]{1,5})$`)
)

// sduiPlatforms: what a client calls itself, the platform conditions know it by, and the
// form factor it has when it names none.
var sduiPlatforms = map[string][2]string{
	"web": {"web", sdui.FormWeb}, "browser": {"web", sdui.FormWeb},
	"tvos": {"tvos", sdui.FormTV}, "tv": {"tvos", sdui.FormTV},
	"androidtv": {"androidtv", sdui.FormTV}, "ios": {"ios", sdui.FormPhone}, "android": {"android", sdui.FormPhone},
	"desktop": {"desktop", sdui.FormDesktop},
}

// sduiFormFactors: what a client calls its form factor.
var sduiFormFactors = map[string]string{
	"tv": sdui.FormTV, "tenfoot": sdui.FormTV, "10ft": sdui.FormTV, "phone": sdui.FormPhone, "tablet": sdui.FormTablet,
	"desktop": sdui.FormDesktop, "web": sdui.FormWeb, "browser": sdui.FormWeb,
}

func sduiPick(r *http.Request, param, header string) string {
	if v := strings.TrimSpace(r.URL.Query().Get(param)); v != "" {
		return v
	}
	return strings.TrimSpace(r.Header.Get(header))
}

// sduiList reads a comma-separated list of names, at most max of them.
func sduiList(v, what string, max int) ([]string, error) {
	if v == "" {
		return nil, nil
	}
	parts := strings.Split(v, ",")
	if len(parts) > max {
		return nil, errors.New(what + ": at most " + strconv.Itoa(max))
	}
	out := make([]string, 0, len(parts))
	for _, c := range parts {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !sduiCapWord.MatchString(c) {
			return nil, errors.New(what + ": comma-separated names of letters, digits and . _ : -")
		}
		out = append(out, c)
	}
	return out, nil
}

// sduiDevice fills in what the client says it is: for each thing the query parameter wins
// over the header. docs/SDUI.md, "What a client sends", is the list.
func (s *Server) sduiDevice(r *http.Request, ctx *sdui.RenderContext) error {
	ctx.Platform = "web"
	ff := sdui.FormWeb
	if p := strings.ToLower(sduiPick(r, "platform", "X-WMS-Platform")); p != "" {
		known, ok := sduiPlatforms[p]
		if !ok {
			return errors.New("platform: web, tvos, ios, android, androidtv or desktop")
		}
		ctx.Platform, ff = known[0], known[1]
	}
	if v := strings.ToLower(sduiPick(r, "formFactor", "X-WMS-Form-Factor")); v != "" {
		known, ok := sduiFormFactors[v]
		if !ok {
			return errors.New("formFactor: tv, phone, tablet, desktop or web")
		}
		ff = known
	}
	class := strings.ToLower(sduiPick(r, "deviceClass", "X-WMS-Device-Class"))
	if class != "" && class != "hd" && class != "4k" {
		return errors.New("deviceClass: hd or 4k")
	}
	ctx.Profile = sdui.ProfileFor(ff, class)
	// The catalogue revision the client was built against, the optional components it
	// draws and the views of its own it has: what it can't draw reaches it as a fallback.
	if v := sduiPick(r, "catalog", "X-WMS-SDUI-Catalog"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1_000_000 {
			return errors.New("catalog: the catalogue revision the client knows, a whole number from 1")
		}
		if n < sdui.CatalogRevision {
			ctx.Catalog = n
		}
	}
	if v := sduiPick(r, "components", "X-WMS-SDUI-Components"); v != "" {
		list, err := sduiList(v, "components", 64)
		if err != nil {
			return err
		}
		ctx.Components = []string{}
		for _, c := range list {
			if c != "none" {
				ctx.Components = append(ctx.Components, c)
			}
		}
	}
	var err error
	if ctx.NativeViews, err = sduiList(sduiPick(r, "views", "X-WMS-SDUI-Views"), "views", 16); err != nil {
		return err
	}
	if ctx.Capabilities, err = sduiList(sduiPick(r, "capabilities", "X-WMS-Capabilities"), "capabilities", 64); err != nil {
		return err
	}
	if v := sduiPick(r, "viewport", "X-WMS-Viewport"); v != "" {
		m := sduiViewport.FindStringSubmatch(v)
		if m == nil {
			return errors.New("viewport: width x height in points, such as 1440x900")
		}
		ctx.Viewport.Width, _ = strconv.Atoi(m[1])
		ctx.Viewport.Height, _ = strconv.Atoi(m[2])
	}
	loc := sduiPick(r, "locale", "X-WMS-Locale")
	if loc != "" && !sduiLocale.MatchString(loc) {
		return errors.New("locale: a language tag such as en-GB")
	}
	if loc == "" {
		loc, _, _ = strings.Cut(r.Header.Get("Accept-Language"), ",")
		loc, _, _ = strings.Cut(strings.TrimSpace(loc), ";")
		if !sduiLocale.MatchString(loc) {
			loc = ""
		}
	}
	ctx.Locale = loc
	return nil
}

// sduiCannot lists the feature switches that are off for an account.
func sduiCannot(acc *library.Access) []string {
	var out []string
	for _, f := range library.Features {
		if !acc.Can(f) {
			out = append(out, f)
		}
	}
	if acc != nil && !acc.LiveTV {
		out = append(out, "liveTv")
	}
	return out
}

// sduiConfig is the configuration a request is answered from: the published one, or the
// draft for the administrator who holds a preview token for it.
func (s *Server) sduiConfig(r *http.Request) (*sduistore.Config, string, error) {
	st, err := s.uiStore()
	if err != nil {
		return nil, "", err
	}
	tok := sduiPick(r, "preview", "X-WMS-SDUI-Preview")
	if tok == "" {
		return st.Current(), "config", nil
	}
	u := userFrom(r)
	s.sdui.mu.Lock()
	p, ok := s.sdui.preview[tok]
	s.sdui.mu.Unlock()
	if !ok || !u.IsAdmin || p.uid != u.ID || time.Now().After(p.expires) {
		return nil, "", errPreview
	}
	if d := st.Draft(); d != nil {
		return d, "draft", nil
	}
	return st.Current(), "config", nil
}

var errPreview = errors.New("that preview has ended, or isn't this account's: make a new one in the editor")

func isPageKind(name string) bool {
	for _, k := range sdui.Catalog().PageKinds {
		if k.Kind == name && name != "custom" {
			return true
		}
	}
	return false
}

// sduiRender is everything one page request comes to.
type sduiRender struct {
	page sdui.Page
	ctx  *sdui.RenderContext
	cfg  *sduistore.Config
}

// The rest are the filters of the web app's own Filters panel and saved views (the same as
// GET /browse takes), so a view saved there opens on a server-driven library page too.
// All but platform, which names the device here.
// "service" is a streaming service: the library's "platform" filter, which a page's query
// can't call "platform", the device's.
var sduiFilterParams = []string{"tab", "sort", "desc", "genre", "decade", "year", "content", "resolution", "watched", "studio", "network", "tag", "service",
	"series", "kind", "disc", "minRating", "audio", "subtitle", "format", "edition", "search", "seed"}

// sduiPrepare works out which page a request is for and who is asking, on what. name is a
// kind of page (home, library, detail, search, ...), chosen among the pages of that kind by
// what the request names, or the ID of one page.
func (s *Server) sduiPrepare(r *http.Request, name string) (*sduiRender, int, error) {
	if !sdui.ValidID(name) {
		return nil, 400, errors.New("page names are lower-case letters, digits, - and _")
	}
	cfg, source, err := s.sduiConfig(r)
	if errors.Is(err, errPreview) {
		return nil, 403, err
	}
	if err != nil {
		return nil, 500, err
	}
	u, acc := userFrom(r), s.access(r)
	set := s.modulesFor(r)
	q := r.URL.Query()
	// Which page, and so which of the request's parameters mean anything: a page takes
	// only what its kind lists, and nothing else of the query is read or sent back.
	var pg sdui.Page
	kind := name
	if !isPageKind(name) {
		p, ok := effectivePage(cfg, name, set)
		if !ok {
			return nil, 404, errors.New("no such page")
		}
		pg, kind = p, p.Kind
	}
	takes := map[string]bool{}
	for _, k := range sdui.Catalog().PageKinds {
		if k.Kind == kind {
			for _, p := range k.Params {
				takes[p] = true
			}
		}
	}
	vars := map[string]string{}
	for _, k := range []string{"library", "item", "show", "season", "collection", "artist", "album", "person"} {
		if v := q.Get(k); v != "" && takes[k] {
			if len(v) > 300 {
				return nil, 400, errors.New(k + ": too long")
			}
			vars[k] = v
		}
	}
	if term := strings.TrimSpace(q.Get("q")); term != "" && takes["q"] {
		if len(term) > 100 {
			term = term[:100]
		}
		vars["query"] = term
	}
	rv := s.newSDUIResolver(r, vars)
	ctx := &sdui.RenderContext{ProfileKind: u.Kind, Admin: u.IsAdmin, UserName: firstNonEmpty(u.DisplayName, u.Name), ServerName: s.ServerName(r.Context()),
		Features: serverFeatures, Cannot: sduiCannot(acc), Flags: map[string]bool{}, Vars: vars, Params: map[string]string{}, Scopes: s.sduiThemeScopes(r, cfg, set),
		Resolver: rv, UI: set, BasePath: "/api/v1/sdui", Source: source, ConfigRevision: cfg.Revision}
	if err := s.sduiDevice(r, ctx); err != nil {
		return nil, 400, err
	}
	rv.formFactor, rv.platform, rv.locale = ctx.Profile.FormFactor, ctx.Platform, ctx.Locale
	if ctx.Layout, err = s.sduiLayout(r, cfg, ctx.Profile.FormFactor); err != nil {
		return nil, 400, err
	}
	// What this profile has been kept from isn't in the conditions, and with its screen
	// time over nothing offers to play (sdui_limits.go); what it may not see isn't in the
	// data. After the device's own parameters, so a client can't say otherwise.
	ctx.Features = append(slices.Clone(serverFeatures), "playback")
	s.sduiLimits(r, ctx)
	rv.noPlay = ctx.Flags["screenTimeUp"]
	_, hiddenModules := profileHidden(accessFor(u))
	for _, k := range sduiFilterParams {
		if v := q.Get(k); v != "" && len(v) <= 200 && takes[k] {
			ctx.Params[k] = v
		}
	}
	// The library and the title the request names, read with the account's limits: they
	// choose the page, and a page about something the account may not see is not found.
	var libID int64
	libKind, mediaKind := "", ""
	pages := effectivePages(cfg, set)
	if kind == "library" || (kind == "detail" && (vars["album"] != "" || isArtistPage(vars))) {
		lib, ok := rv.library(vars["library"])
		if !ok {
			return nil, 404, errors.New("not found")
		}
		libID, libKind = lib.ID, lib.Type
		vars["libraryKind"], vars["libraryName"] = lib.Type, lib.Name
	}
	if kind == "detail" {
		subj, err := rv.Subject()
		if err != nil {
			return nil, 404, errors.New("not found")
		}
		mediaKind = subj.Kind
		vars["title"] = subj.Title
		if subj.ShowID != "" {
			vars["show"] = subj.ShowID
			if vars["season"] == "" {
				vars["season"] = strconv.Itoa(subj.Season)
			}
		}
		if id, err := strconv.ParseInt(vars["item"], 10, 64); err == nil && libID == 0 {
			if it, err := s.Store.Item(r.Context(), id); err == nil {
				libID = it.LibraryID
				if lib, err := s.Store.Library(r.Context(), libID); err == nil {
					libKind = lib.Type
					vars["libraryKind"], vars["libraryName"] = lib.Type, lib.Name
					if vars["library"] == "" {
						vars["library"] = itoa(libID)
					}
				}
			}
		}
	}
	if isPageKind(name) {
		p, ok := sdui.SelectPage(pages, kind, libID, libKind, mediaKind)
		if !ok {
			return nil, 404, errors.New("no such page")
		}
		pg = p
	}
	// What the server knows that a condition may ask about.
	libs := rv.libraries()
	seen := map[string]bool{}
	for _, l := range libs {
		if !seen[l.Type] {
			seen[l.Type] = true
			ctx.LibraryKinds = append(ctx.LibraryKinds, l.Type)
		}
		ctx.Flags["scanning"] = ctx.Flags["scanning"] || l.Scanning
	}
	ctx.Flags["librariesEmpty"] = len(libs) == 0
	ctx.Flags["setupIncomplete"] = u.IsAdmin && s.DB.GetSetting(r.Context(), "setupDone", "") == "0"
	ctx.Flags["hasQuery"] = len([]rune(vars["query"])) >= 2
	for _, ui := range set.Modules() {
		ctx.Modules = append(ctx.Modules, ui.Module)
	}
	for _, mod := range pageModules(&pg) {
		if slices.Contains(ctx.Modules, mod) {
			continue
		}
		if s.Modules != nil && s.Modules.Installed(mod) && !slices.Contains(hiddenModules, mod) {
			ctx.Modules = append(ctx.Modules, mod)
		}
	}
	ctx.CanPersonalise = !cfg.Personalisation.Disabled && acc.Can(library.FeatureEditLayout)
	if ctx.CanPersonalise {
		ctx.Personal = s.sduiPersonal(r, pg.ID)
	}
	return &sduiRender{page: pg, ctx: ctx, cfg: cfg}, 200, nil
}

// pageModules are the modules a page's conditions ask about: the only ones worth looking up.
func pageModules(p *sdui.Page) []string {
	seen := map[string]bool{}
	var out []string
	var cond func(c *sdui.Condition)
	cond = func(c *sdui.Condition) {
		if c == nil {
			return
		}
		if c.Module != "" && !seen[c.Module] {
			seen[c.Module] = true
			out = append(out, c.Module)
		}
		for i := range c.All {
			cond(&c.All[i])
		}
		for i := range c.Any {
			cond(&c.Any[i])
		}
		cond(c.Not)
	}
	var walk func(list []sdui.Section)
	walk = func(list []sdui.Section) {
		for i := range list {
			cond(list[i].When)
			walk(list[i].Children)
		}
	}
	walk(p.Sections)
	for i := range p.Variants {
		cond(p.Variants[i].When)
		walk(p.Variants[i].Sections)
	}
	return out
}

// renderPage renders the page a request is for, and keeps the result for diffs. The same
// request from the same account within a few seconds is answered from the render before.
func (s *Server) renderPage(w http.ResponseWriter, r *http.Request) (sdui.Document, bool) {
	st := &s.sdui
	u := userFrom(r)
	cfg, _, _ := s.sduiConfig(r)
	key := ""
	if cfg != nil {
		h := sha256.New()
		for _, part := range []string{cfg.ETag, itoa(u.ID), r.URL.Path, r.URL.Query().Encode(), r.Header.Get("Accept-Language")} {
			h.Write([]byte(part))
			h.Write([]byte{0})
		}
		for _, name := range []string{"X-WMS-Platform", "X-WMS-Form-Factor", "X-WMS-Device-Class", "X-WMS-SDUI-Catalog", "X-WMS-SDUI-Components", "X-WMS-SDUI-Views", "X-WMS-Capabilities", "X-WMS-Viewport", "X-WMS-Locale", "X-WMS-SDUI-Preview", "X-WMS-Device", "X-WMS-Layout"} {
			h.Write([]byte(r.Header.Get(name)))
			h.Write([]byte{0})
		}
		// The profile's limits, and whether its time is up, are part of what was asked: a
		// render from before a parent changed them, or before the time ran out, is not the
		// answer to this request.
		h.Write([]byte(u.Restrictions))
		h.Write([]byte{0})
		if a := accessFor(u); a != nil && !a.Time.Empty() {
			if v := a.Time.Check(s.clock(), s.parentalInit().tracker.Usage(r.Context(), u.ID)); !v.Allowed {
				h.Write([]byte("time-up"))
			}
		}
		key = hex.EncodeToString(h.Sum(nil))
		st.mu.Lock()
		got, ok := st.recent[key]
		st.mu.Unlock()
		if ok && got.uid == u.ID && time.Since(got.at) < recentRenderFor {
			return got.doc, true
		}
	}
	rn, code, err := s.sduiPrepare(r, strings.TrimSuffix(r.PathValue("page"), ".json"))
	if err != nil {
		writeErr(w, code, err.Error())
		return sdui.Document{}, false
	}
	doc, err := sdui.Render(rn.page, rn.ctx)
	if errors.Is(err, sdui.ErrNotFound) {
		writeErr(w, 404, "not found")
		return sdui.Document{}, false
	}
	if err != nil {
		s.Log.WarnContext(r.Context(), "a page could not be rendered", "cat", "sdui", "page", rn.page.ID, "err", err)
		writeErr(w, 500, "This page could not be put together. It is in the log.")
		return sdui.Document{}, false
	}
	s.keepRender(u.ID, doc)
	if key != "" {
		st.mu.Lock()
		if st.recent == nil {
			st.recent = map[string]recentRender{}
		}
		now := time.Now()
		if len(st.recent) >= keptRendersFor(pace.Machine()) {
			for k, v := range st.recent {
				if now.Sub(v.at) >= recentRenderFor {
					delete(st.recent, k)
				}
			}
		}
		if len(st.recent) < keptRendersFor(pace.Machine()) {
			st.recent[key] = recentRender{at: now, uid: u.ID, doc: doc}
		}
		st.mu.Unlock()
	}
	return doc, true
}

// forgetRecent drops an account's just-finished renders: it changed its own arrangement.
func (s *Server) forgetRecent(uid int64) {
	st := &s.sdui
	st.mu.Lock()
	defer st.mu.Unlock()
	for k, v := range st.recent {
		if v.uid == uid {
			delete(st.recent, k)
		}
	}
}

func renderKey(uid int64, etag string) string { return strconv.FormatInt(uid, 10) + " " + etag }

// keepRender remembers a render for this account: its newest few, and the newest of all up to
// what the machine has room for.
func (s *Server) keepRender(uid int64, doc sdui.Document) {
	st := &s.sdui
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.renders == nil {
		st.renders, st.order = map[string]*list.Element{}, list.New()
	}
	key := renderKey(uid, doc.Meta.ETag)
	if e, ok := st.renders[key]; ok {
		st.order.MoveToFront(e)
		return
	}
	st.renders[key] = st.order.PushFront(&keptRender{key: key, uid: uid, doc: doc})
	// This account's oldest go first, then the oldest of all.
	mine := 0
	for e := st.order.Front(); e != nil; {
		next := e.Next()
		if k := e.Value.(*keptRender); k.uid == uid {
			if mine++; mine > maxKeptPerAccount {
				st.order.Remove(e)
				delete(st.renders, k.key)
			}
		}
		e = next
	}
	for limit := keptRendersFor(pace.Machine()); st.order.Len() > limit; {
		last := st.order.Back()
		st.order.Remove(last)
		delete(st.renders, last.Value.(*keptRender).key)
	}
}

// keptRenderFor is a render this account was given, by its ETag.
func (s *Server) keptRenderFor(uid int64, etag string) (sdui.Document, bool) {
	st := &s.sdui
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.renders == nil {
		return sdui.Document{}, false
	}
	// The ETag as the client sends it, weak or not.
	for _, k := range []string{etag, "W/" + strings.TrimPrefix(etag, "W/")} {
		if e, ok := st.renders[renderKey(uid, k)]; ok {
			st.order.MoveToFront(e)
			return e.Value.(*keptRender).doc, true
		}
	}
	return sdui.Document{}, false
}

// GET /api/sdui/catalog: every component, card style, data source, condition, icon and
// device profile, as data: what an editor builds its palette and forms from.
func (s *Server) handleSDUICatalog(w http.ResponseWriter, r *http.Request) {
	cat, etag := s.sduiCatalog()
	if notModified(w, r, etag) {
		return
	}
	writeJSONTagged(w, 200, etag, cat)
}

// GET /api/sdui/navigation: the main menu for this account on this device.
func (s *Server) handleSDUINavigation(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := s.sduiConfig(r)
	if err != nil {
		code := 500
		if errors.Is(err, errPreview) {
			code = 403
		}
		writeErr(w, code, err.Error())
		return
	}
	u, acc := userFrom(r), s.access(r)
	ctx := &sdui.RenderContext{ProfileKind: u.Kind, Admin: u.IsAdmin, Features: serverFeatures, Cannot: sduiCannot(acc), Flags: map[string]bool{}, Vars: map[string]string{}}
	if err := s.sduiDevice(r, ctx); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx.Features = slices.Clone(serverFeatures)
	s.sduiLimits(r, ctx)
	_, hiddenModules := profileHidden(acc)
	rv := s.newSDUIResolver(r, ctx.Vars)
	var libs []sdui.NavLibrary
	seen := map[string]bool{}
	for _, l := range rv.libraries() {
		libs = append(libs, sdui.NavLibrary{ID: l.ID, Name: l.Name, Kind: l.Type})
		if !seen[l.Type] {
			seen[l.Type] = true
			ctx.LibraryKinds = append(ctx.LibraryKinds, l.Type)
		}
	}
	ctx.Flags["librariesEmpty"] = len(libs) == 0
	set := s.modulesFor(r)
	nav := sdui.StockNavigation()
	if cfg.Navigation != nil {
		nav = *cfg.Navigation
	}
	nav = moduleNavigation(nav, set)
	for _, ui := range set.Modules() {
		ctx.Modules = append(ctx.Modules, ui.Module)
	}
	for i := range nav.Items {
		if c := nav.Items[i].When; c != nil && c.Module != "" && !slices.Contains(ctx.Modules, c.Module) && s.Modules != nil && s.Modules.Installed(c.Module) && !slices.Contains(hiddenModules, c.Module) {
			ctx.Modules = append(ctx.Modules, c.Module)
		}
	}
	ctx.CanPersonalise = !cfg.Personalisation.Disabled && acc.Can(library.FeatureEditLayout)
	if ctx.CanPersonalise {
		ctx.Personal = s.sduiPersonal(r, sduiNavKey)
	}
	layout, err := s.sduiLayout(r, cfg, ctx.Profile.FormFactor)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	doc := sdui.RenderNavigation(nav, func(id string) (sdui.Page, bool) { return effectivePage(cfg, id, set) }, libs, ctx)
	doc.Shell = sdui.ShellFor(layout.ID, ctx.Profile.FormFactor)
	doc.Layouts = s.layoutChoice(r, cfg, ctx.Profile.FormFactor, layout.ID)
	doc.ETag = ""
	doc.ETag = hashETag("nav", doc)
	if notModified(w, r, doc.ETag) {
		return
	}
	writeJSONTagged(w, 200, doc.ETag, doc)
}

// GET /api/sdui/topshelf: content for a tvOS Top Shelf extension: what this account was
// watching and what's new, within its limits. deviceClass (hd or 4k) sizes the pictures.
func (s *Server) handleSDUITopShelf(w http.ResponseWriter, r *http.Request) {
	class := strings.ToLower(sduiPick(r, "deviceClass", "X-WMS-Device-Class"))
	if class != "" && class != "hd" && class != "4k" {
		writeErr(w, 400, "deviceClass: hd or 4k")
		return
	}
	ts := sdui.NewTopShelf(s.sduiTopShelfRows(r), sdui.ProfileFor(sdui.FormTV, class), 900)
	etag := hashETag("ts", ts)
	if notModified(w, r, etag) {
		return
	}
	writeJSONTagged(w, 200, etag, ts)
}

// GET /api/sdui/pages/{page}: the page, rendered for this account and device.
func (s *Server) handleSDUIPage(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.renderPage(w, r)
	if !ok {
		return
	}
	if notModified(w, r, doc.Meta.ETag) {
		return
	}
	writeJSONTagged(w, 200, doc.Meta.ETag, doc)
}

// sduiDiff is the answer to a diff: patches from the render the client has, or the whole
// document when the server no longer has that render.
type sduiDiff struct {
	ETag     string         `json:"etag"`
	Since    string         `json:"since,omitempty"`
	Patches  []sdui.Patch   `json:"patches,omitempty"`
	Document *sdui.Document `json:"document,omitempty"`
}

// GET /api/sdui/pages/{page}/diff?since=<etag>: what changed since the render the client has.
func (s *Server) handleSDUIDiff(w http.ResponseWriter, r *http.Request) {
	since := strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("since"), r.Header.Get("If-None-Match")))
	if since == "" || len(since) > 200 {
		writeErr(w, 400, "since: the ETag of the render the client has")
		return
	}
	doc, ok := s.renderPage(w, r)
	if !ok {
		return
	}
	if sduistore.SameETag(since, doc.Meta.ETag) {
		w.Header().Set("ETag", doc.Meta.ETag)
		w.Header().Set("Cache-Control", "private, no-cache")
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if old, ok := s.keptRenderFor(userFrom(r).ID, since); ok && old.Root.ID == doc.Root.ID && old.Page.ID == doc.Page.ID {
		patches := sdui.Diff(old, doc)
		if patches == nil {
			patches = []sdui.Patch{}
		}
		writeJSONTagged(w, 200, doc.Meta.ETag, sduiDiff{ETag: doc.Meta.ETag, Since: since, Patches: patches})
		return
	}
	writeJSONTagged(w, 200, doc.Meta.ETag, sduiDiff{ETag: doc.Meta.ETag, Document: &doc})
}

// GET /api/sdui/pages/{page}/sections/{section}/items?cursor=: one page of a section's
// items: the next cards of a row or grid, a deferred row's first, another season.
func (s *Server) handleSDUISection(w http.ResponseWriter, r *http.Request) {
	section := r.PathValue("section")
	if section == "" || len(section) > sdui.MaxNodeIDLength {
		writeErr(w, 400, "section: the ID of a section of the page")
		return
	}
	cursor := qInt(r, "cursor", 0)
	if cursor < 0 || cursor > 100000 {
		writeErr(w, 400, "cursor: what the page before gave as next")
		return
	}
	rn, code, err := s.sduiPrepare(r, r.PathValue("page"))
	if err != nil {
		writeErr(w, code, err.Error())
		return
	}
	season := ""
	if v := r.URL.Query().Get("season"); v != "" {
		if n, err := strconv.Atoi(v); err != nil || n < 0 || n > 10000 {
			writeErr(w, 400, "season: a season number")
			return
		}
		season = v
		delete(rn.ctx.Vars, "season") // the season is the request's, not the page's
	}
	out, err := sdui.RenderSection(rn.page, section, cursor, season, rn.ctx)
	if errors.Is(err, sdui.ErrNotFound) {
		writeErr(w, 404, "not found")
		return
	}
	if err != nil {
		s.Log.WarnContext(r.Context(), "a section could not be read", "cat", "sdui", "page", rn.page.ID, "section", section, "err", err)
		writeErr(w, 500, "This row could not be loaded. It is in the log.")
		return
	}
	etag := hashETag("sec", out)
	if notModified(w, r, etag) {
		return
	}
	writeJSONTagged(w, 200, etag, out)
}

// ---- a profile's own arrangement ----

// A profile's arrangements are kept with its other interface choices (the "ui" of its
// account, which /api/me/ui also reads), under "sdui": {"pages": {<page>: Personal},
// "nav": Personal}. They go when the profile does.

const sduiNavKey = "@nav"

type sduiPersonalStore struct {
	Pages map[string]*sdui.Personal `json:"pages,omitempty"`
	Nav   *sdui.Personal            `json:"nav,omitempty"`
	// Layouts are the profile's layout per device (sdui_layouts.go).
	Layouts map[string]string `json:"layouts,omitempty"`
}

func (s *Server) readPersonal(r *http.Request) (map[string]json.RawMessage, sduiPersonalStore) {
	ui := map[string]json.RawMessage{}
	_ = json.Unmarshal([]byte(s.Auth.UI(r.Context(), userFrom(r).ID)), &ui)
	var ps sduiPersonalStore
	if raw, ok := ui["sdui"]; ok {
		_ = json.Unmarshal(raw, &ps)
	}
	return ui, ps
}

// sduiPersonal is the profile's arrangement of one page, nil when it has none. Home falls
// back to the order and hidden sections the web app's Home kept before pages were
// server-driven: the sections carry the same IDs.
func (s *Server) sduiPersonal(r *http.Request, page string) *sdui.Personal {
	ui, ps := s.readPersonal(r)
	if page == sduiNavKey {
		return ps.Nav
	}
	if p := ps.Pages[page]; p != nil {
		return p
	}
	if page == "home" {
		var old struct {
			V      int      `json:"v"`
			Order  []string `json:"order"`
			Hidden []string `json:"hidden"`
		}
		if json.Unmarshal(ui["home"], &old) == nil && (len(old.Order) > 0 || len(old.Hidden) > 0) {
			p := &sdui.Personal{Hidden: old.Hidden}
			if old.V == 2 {
				p.Order = old.Order
			}
			if !sdui.HasErrors(sdui.ValidatePersonal(p)) {
				return p
			}
		}
	}
	return nil
}

func (s *Server) personalKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	page := r.PathValue("page")
	if page == "" {
		return sduiNavKey, true
	}
	if !sdui.ValidID(page) {
		writeErr(w, 400, "page names are lower-case letters, digits, - and _")
		return "", false
	}
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return "", false
	}
	if _, ok := effectivePage(st.Current(), page, s.modulesFor(r)); !ok {
		writeErr(w, 404, "no such page")
		return "", false
	}
	return page, true
}

// GET /api/sdui/pages/{page}/personal, GET /api/sdui/navigation/personal: this profile's
// own arrangement, and whether it may change it.
func (s *Server) handleSDUIPersonalGet(w http.ResponseWriter, r *http.Request) {
	key, ok := s.personalKey(w, r)
	if !ok {
		return
	}
	st, _ := s.uiStore()
	p := s.sduiPersonal(r, key)
	if p == nil {
		p = &sdui.Personal{}
	}
	editable := !st.Current().Personalisation.Disabled && s.access(r).Can(library.FeatureEditLayout)
	writeJSON(w, 200, map[string]any{"editable": editable, "personal": p})
}

func (s *Server) mayPersonalise(w http.ResponseWriter, r *http.Request) bool {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return false
	}
	if st.Current().Personalisation.Disabled {
		writeErr(w, 403, "Rearranging pages is turned off on this server.")
		return false
	}
	if !s.access(r).Can(library.FeatureEditLayout) {
		writeErr(w, 403, "This profile can't rearrange pages. Whoever manages it can allow that.")
		return false
	}
	return true
}

func (s *Server) writePersonal(r *http.Request, key string, p *sdui.Personal) error {
	s.sdui.personalMu.Lock()
	defer s.sdui.personalMu.Unlock()
	ui, ps := s.readPersonal(r)
	if key == sduiNavKey {
		ps.Nav = p
	} else {
		if ps.Pages == nil {
			ps.Pages = map[string]*sdui.Personal{}
		}
		if p == nil {
			delete(ps.Pages, key)
			// The arrangement Home kept before goes too, or it would come back.
			if key == "home" {
				delete(ui, "home")
			}
		} else {
			if len(ps.Pages) >= 64 && ps.Pages[key] == nil {
				return errors.New("this profile has arranged too many pages: reset one first")
			}
			ps.Pages[key] = p
		}
	}
	raw, err := json.Marshal(ps)
	if err != nil {
		return err
	}
	ui["sdui"] = raw
	if string(raw) == "{}" {
		delete(ui, "sdui")
	}
	b, err := json.Marshal(ui)
	if err != nil {
		return err
	}
	if len(b) > 64<<10 {
		return errors.New("this profile's interface choices are too large")
	}
	s.forgetRecent(userFrom(r).ID)
	return s.Auth.SetUI(r.Context(), userFrom(r).ID, string(b))
}

// PUT /api/sdui/pages/{page}/personal, PUT /api/sdui/navigation/personal: replace this
// profile's arrangement. Only its own; refused when the profile may not rearrange pages.
func (s *Server) handleSDUIPersonalPut(w http.ResponseWriter, r *http.Request) {
	key, ok := s.personalKey(w, r)
	if !ok || !s.mayPersonalise(w, r) {
		return
	}
	var p sdui.Personal
	body, err := sduiBody(r, sdui.MaxPersonalBytes)
	if err == nil {
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.DisallowUnknownFields()
		err = dec.Decode(&p)
	}
	if err != nil {
		writeErr(w, 400, "That arrangement can't be read: "+err.Error())
		return
	}
	if issues := sdui.ValidatePersonal(&p); sdui.HasErrors(issues) {
		writeJSON(w, 400, map[string]any{"error": "That arrangement can't be saved.", "issues": issues})
		return
	}
	var store *sdui.Personal
	if !p.IsZero() {
		store = &p
	}
	if err := s.writePersonal(r, key, store); err != nil {
		s.Log.InfoContext(r.Context(), "personal arrangement not saved", "cat", "sdui", "profile", adminName(r), "key", key, "err", err)
		writeErr(w, 400, err.Error())
		return
	}
	s.Log.DebugContext(r.Context(), "personal arrangement saved", "cat", "sdui", "profile", adminName(r), "key", key)
	writeJSON(w, 200, map[string]any{"editable": true, "personal": p})
}

// DELETE /api/sdui/pages/{page}/personal, DELETE /api/sdui/navigation/personal: back to the
// page as everyone has it.
func (s *Server) handleSDUIPersonalDelete(w http.ResponseWriter, r *http.Request) {
	key, ok := s.personalKey(w, r)
	if !ok || !s.mayPersonalise(w, r) {
		return
	}
	if err := s.writePersonal(r, key, nil); err != nil {
		s.Log.ErrorContext(r.Context(), "could not reset a personal arrangement", "cat", "sdui", "profile", adminName(r), "key", key, "err", err)
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.DebugContext(r.Context(), "personal arrangement reset", "cat", "sdui", "profile", adminName(r), "key", key)
	writeJSON(w, 200, map[string]any{"editable": true, "personal": sdui.Personal{}})
}
