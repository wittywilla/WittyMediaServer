// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/wittywilla/wittymediaserver/internal/bufpool"
	"github.com/wittywilla/wittymediaserver/internal/pace"
	"github.com/wittywilla/wittymediaserver/internal/sduistore"
	"github.com/wittywilla/wittymediaserver/internal/themes"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// Server-driven UI: the screens in ui_config.json (internal/sduistore), rendered by pkg/sdui
// for whoever asks. docs/SDUI.md, "HTTP API", describes the routes.

// sduiState is the store, opened when first wanted, and the renders kept for diffs.
type sduiState struct {
	once  sync.Once
	store *sduistore.Store
	err   error

	mu      sync.Mutex
	renders map[string]*list.Element // user id + etag → rendered document
	order   *list.List
}

type keptRender struct {
	key string
	uid int64
	doc sdui.Document
}

// keptRendersFor bounds the renders kept so a client can ask for a diff against one. A
// render holds a screen's worth of titles, a few hundred kilobytes for a full home screen,
// so a small machine keeps few; a client whose render has gone is sent the whole screen,
// which is always right. One account keeps at most maxKeptPerAccount, so one busy client
// can't push out everyone else's.
func keptRendersFor(c pace.Class) int {
	switch c {
	case pace.Tiny:
		return 12
	case pace.Small:
		return 32
	}
	return 128
}

const maxKeptPerAccount = 6

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
		st.store, st.err = sduistore.Open(s.Cfg.DataDir, sduistore.Options{Log: log, CheckTheme: checkThemeSection, KnownToken: themes.IsColorToken})
	})
	return st.store, st.err
}

// checkThemeSection: the overrides in ui_config.json name colours themes have, as colours.
func checkThemeSection(t sduistore.Theme) []sdui.Issue {
	var out []sdui.Issue
	check := func(path string, toks map[string]string) {
		for k, v := range toks {
			if !themes.IsColorToken(k) {
				out = append(out, sdui.Issue{Path: path + "/" + k, Message: "not a colour token themes have"})
			}
			if _, ok := themes.NormalizeColor(v); !ok {
				out = append(out, sdui.Issue{Path: path + "/" + k, Message: "not a colour: use #rrggbb, #rrggbbaa or rgb()/rgba()"})
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
	m.Handle("GET /api/sdui/screens/{screen}", s.requireUser(s.handleSDUIScreen))
	m.Handle("GET /api/sdui/screens/{screen}/diff", s.requireUser(s.handleSDUIDiff))
	m.Handle("GET /api/sdui/topshelf", s.requireUser(s.handleSDUITopShelf))
	m.Handle("GET /api/sdui/config", s.requireAdmin(s.handleSDUIConfig))
	m.Handle("PUT /api/sdui/config", s.requireAdmin(s.handleSDUIConfigPut))
	m.Handle("GET /api/sdui/config/history", s.requireAdmin(s.handleSDUIHistory))
	m.Handle("POST /api/sdui/config/rollback/{rev}", s.requireAdmin(s.handleSDUIRollback))
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

var (
	sduiWord     = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	sduiCapWord  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]{0,63}$`)
	sduiLocale   = regexp.MustCompile(`^[A-Za-z]{2,8}([-_][A-Za-z0-9]{1,8}){0,3}$`)
	sduiViewport = regexp.MustCompile(`^([0-9]{1,5})x([0-9]{1,5})$`)
)

// sduiPlatforms are the platforms pkg/sdui's conditions know; form factors map onto them.
var sduiPlatforms = map[string]string{
	"web": "web", "tv": "tv", "tvos": "tv", "ios": "ios", "android": "android", "desktop": "desktop",
	// form factors
	"phone": "ios", "tablet": "ios", "tenfoot": "tv", "10ft": "tv", "browser": "web",
}

// sduiFormFactors: what a client calls its form factor, and the profile it gets. A
// platform stands for one when the client names no form factor.
var sduiFormFactors = map[string]string{
	"tv": sdui.FormTV, "tvos": sdui.FormTV, "tenfoot": sdui.FormTV, "10ft": sdui.FormTV,
	"phone": sdui.FormPhone, "ios": sdui.FormPhone, "android": sdui.FormPhone,
	"tablet": sdui.FormTablet, "desktop": sdui.FormDesktop, "web": sdui.FormWeb, "browser": sdui.FormWeb,
}

// sduiProfile is the device profile for a request: its form factor (named, or implied by
// the platform), and for a TV its class, "hd" or "4k". A TV that doesn't say gets the HD
// profile: the smaller one is safe on both.
func sduiProfile(platform, formFactor, class string) *sdui.Layout {
	ff, ok := sduiFormFactors[formFactor]
	if !ok {
		ff = sduiFormFactors[platform]
	}
	return sdui.ProfileFor(ff, class)
}

// renderContext is who is asking, on what, for pkg/sdui. The client says what it is in the
// query or in headers (the query wins); the server says who is signed in and what's installed.
func (s *Server) renderContext(r *http.Request, doc sdui.Document, ov map[string]map[string]string) (sdui.RenderContext, error) {
	rawPlatform := ""
	q := r.URL.Query()
	pick := func(param, header string) string {
		if v := strings.TrimSpace(q.Get(param)); v != "" {
			return v
		}
		return strings.TrimSpace(r.Header.Get(header))
	}
	ctx := sdui.RenderContext{Platform: "web", ThemeOverrides: ov, Flags: map[string]bool{}}
	// Platform, or a form factor that stands for one.
	if p := strings.ToLower(firstNonEmpty(pick("platform", "X-WMS-Platform"), pick("formFactor", "X-WMS-Form-Factor"))); p != "" {
		plat, ok := sduiPlatforms[p]
		if !ok {
			if !sduiWord.MatchString(p) {
				return ctx, errors.New("platform: a short lower-case name such as web, tv, ios, android or desktop")
			}
			plat = p // a platform a newer client knows; conditions on it simply don't match
		}
		ctx.Platform, rawPlatform = plat, p
	}
	ff := strings.ToLower(pick("formFactor", "X-WMS-Form-Factor"))
	if ff != "" {
		if !sduiWord.MatchString(ff) {
			return ctx, errors.New("formFactor: a short lower-case name such as phone, tablet, tv or desktop")
		}
		ctx.Flags["formFactor:"+ff] = true
	}
	class := strings.ToLower(pick("deviceClass", "X-WMS-Device-Class"))
	if class != "" && class != "hd" && class != "4k" {
		return ctx, errors.New("deviceClass: hd or 4k")
	}
	ctx.Profile = sduiProfile(firstNonEmpty(rawPlatform, ctx.Platform), ff, class)
	// The catalogue revision the client was built against: newer types reach it as older ones.
	if v := pick("catalog", "X-WMS-SDUI-Catalog"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1_000_000 {
			return ctx, errors.New("catalog: the catalogue revision the client knows, a whole number from 1")
		}
		if n < sdui.CatalogRevision {
			ctx.Catalog = n
		}
	}
	// Viewport: width and height, or "1440x900".
	if v := pick("viewport", "X-WMS-Viewport"); v != "" {
		m := sduiViewport.FindStringSubmatch(v)
		if m == nil {
			return ctx, errors.New("viewport: width x height in CSS pixels, such as 1440x900")
		}
		ctx.Viewport.Width, _ = strconv.Atoi(m[1])
		ctx.Viewport.Height, _ = strconv.Atoi(m[2])
	}
	for _, d := range []struct {
		name string
		to   *int
	}{{"width", &ctx.Viewport.Width}, {"height", &ctx.Viewport.Height}} {
		if v := q.Get(d.name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 || n > 100000 {
				return ctx, errors.New(d.name + ": a whole number of CSS pixels")
			}
			*d.to = n
		}
	}
	// Locale: asked for, or the first language the client accepts.
	loc := pick("locale", "X-WMS-Locale")
	if loc == "" {
		loc, _, _ = strings.Cut(r.Header.Get("Accept-Language"), ",")
		loc, _, _ = strings.Cut(strings.TrimSpace(loc), ";")
		if !sduiLocale.MatchString(loc) {
			loc = ""
		}
	}
	if loc != "" {
		if !sduiLocale.MatchString(loc) {
			return ctx, errors.New("locale: a language tag such as en-GB")
		}
		ctx.Locale = loc
	}
	// Capabilities the client has: each becomes a flag the screens can test.
	if caps := pick("capabilities", "X-WMS-Capabilities"); caps != "" {
		parts := strings.Split(caps, ",")
		if len(parts) > 64 {
			return ctx, errors.New("capabilities: at most 64")
		}
		for _, c := range parts {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			if !sduiCapWord.MatchString(c) {
				return ctx, errors.New("capabilities: comma-separated names of letters, digits and . _ : -")
			}
			ctx.Flags[c] = true
		}
	}
	// What the server says, which a client can't.
	u := userFrom(r)
	ctx.Role = u.Kind
	if u.IsAdmin {
		ctx.Role = "admin"
	}
	ctx.Flags["setupIncomplete"] = u.IsAdmin && s.DB.GetSetting(r.Context(), "setupDone", "") == "0"
	ctx.Features = append(append([]string(nil), serverFeatures...), "history", "playback")
	for _, mod := range conditionModules(doc) {
		if s.Modules != nil && s.Modules.Installed(mod) {
			ctx.Modules = append(ctx.Modules, mod)
		}
	}
	ctx.Data = map[string]any{
		"user":   map[string]any{"name": firstNonEmpty(u.DisplayName, u.Name), "kind": u.Kind, "isAdmin": u.IsAdmin},
		"server": map[string]any{"name": s.ServerName(r.Context()), "version": s.Version},
	}
	// What this profile has been kept from isn't in the conditions (sdui_limits.go); what
	// it may not see isn't in the data.
	if _, err := s.sduiLimits(r, doc.Screen, &ctx); err != nil {
		return ctx, err
	}
	if err := s.sduiData(r, doc.Screen, ctx.Profile, ctx.Data, ctx.Flags); err != nil {
		return ctx, err
	}
	return ctx, nil
}

// conditionModules are the modules a document's conditions ask about: the only ones worth
// looking up.
func conditionModules(doc sdui.Document) []string {
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
	var node func(n *sdui.Node)
	node = func(n *sdui.Node) {
		cond(n.Visibility)
		for i := range n.Children {
			node(&n.Children[i])
		}
	}
	node(&doc.Root)
	return out
}

// renderScreen renders one screen for this request, and keeps the result for diffs.
func (s *Server) renderScreen(w http.ResponseWriter, r *http.Request) (sdui.Document, bool) {
	name := r.PathValue("screen")
	if !sduistore.ScreenName.MatchString(name) {
		writeErr(w, 400, "screen names are lower-case letters, digits, - and _")
		return sdui.Document{}, false
	}
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return sdui.Document{}, false
	}
	cfg := st.Current()
	// A stock screen the configuration has lost is still served, from the template: an
	// edit that removes "home" must not leave every client without one.
	stock, isStock := stockScreens()[name]
	doc, ok := cfg.Screens[name]
	if !ok {
		if !isStock {
			writeErr(w, 404, "no such screen")
			return sdui.Document{}, false
		}
		doc = stock
	}
	ctx, err := s.renderContext(r, doc, cfg.Theme.Modules)
	if errors.Is(err, errNoScreenData) {
		writeErr(w, 404, "not found")
		return sdui.Document{}, false
	}
	if errors.Is(err, errScreenTimeUp) {
		// The player's frame is for playing: the same answer as the playback routes give.
		u := userFrom(r)
		v := accessFor(u).Time.Check(s.clock(), s.parentalInit().tracker.Usage(r.Context(), u.ID))
		w.Header().Set("X-WMS-Screen-Time-Left", "0")
		writeJSON(w, 403, map[string]any{"error": v.Message(), "code": "screen-time", "screenTime": screenTimeView(v)})
		return sdui.Document{}, false
	}
	if err != nil {
		writeErr(w, 400, err.Error())
		return sdui.Document{}, false
	}
	out, err := sdui.Render(doc, ctx)
	if err != nil && ok && isStock {
		// The configured one can't be shown to this account (its root is hidden, say): the
		// stock one can.
		s.Log.WarnContext(r.Context(), "a configured screen couldn't be rendered; the stock one was sent", "cat", "ui", "screen", name, "err", err)
		out, err = sdui.Render(stock, ctx)
	}
	if err != nil {
		writeErr(w, 422, "this screen can't be shown to this account on this device: "+err.Error())
		return sdui.Document{}, false
	}
	if out.Meta.Source == "" {
		out.Meta.Source = sduistore.FileName
	}
	out.Meta.ETag = sdui.ETag(out)
	s.keepRender(userFrom(r).ID, out)
	return out, true
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

// stockScreens are the templates, built once.
var stockScreens = sync.OnceValue(sdui.DefaultScreens)

// GET /api/sdui/topshelf: content for a tvOS Top Shelf extension: what this account was
// watching and what's new, within its limits. deviceClass (hd or 4k) sizes the pictures.
func (s *Server) handleSDUITopShelf(w http.ResponseWriter, r *http.Request) {
	class := strings.ToLower(firstNonEmpty(r.URL.Query().Get("deviceClass"), r.Header.Get("X-WMS-Device-Class")))
	if class != "" && class != "hd" && class != "4k" {
		writeErr(w, 400, "deviceClass: hd or 4k")
		return
	}
	ts := sdui.NewTopShelf(s.sduiTopShelfRows(r), sdui.ProfileFor(sdui.FormTV, class), 900)
	b, _ := json.Marshal(ts)
	sum := sha256.Sum256(b)
	etag := `W/"ts-` + hex.EncodeToString(sum[:])[:32] + `"`
	if notModified(w, r, etag) {
		return
	}
	writeJSONTagged(w, 200, etag, ts)
}

// GET /api/sdui/screens/{screen}: the screen, rendered for this account and device.
func (s *Server) handleSDUIScreen(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.renderScreen(w, r)
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

// GET /api/sdui/screens/{screen}/diff?since=<etag>: what changed since the render the client has.
func (s *Server) handleSDUIDiff(w http.ResponseWriter, r *http.Request) {
	since := strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("since"), r.Header.Get("If-None-Match")))
	if since == "" || len(since) > 200 {
		writeErr(w, 400, "since: the ETag of the render the client has")
		return
	}
	doc, ok := s.renderScreen(w, r)
	if !ok {
		return
	}
	if sduistore.SameETag(since, doc.Meta.ETag) {
		w.Header().Set("ETag", doc.Meta.ETag)
		w.Header().Set("Cache-Control", "private, no-cache")
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if old, ok := s.keptRenderFor(userFrom(r).ID, since); ok && old.Root.ID == doc.Root.ID {
		patches := sdui.Diff(old, doc)
		if patches == nil {
			patches = []sdui.Patch{}
		}
		writeJSONTagged(w, 200, doc.Meta.ETag, sduiDiff{ETag: doc.Meta.ETag, Since: since, Patches: patches})
		return
	}
	writeJSONTagged(w, 200, doc.Meta.ETag, sduiDiff{ETag: doc.Meta.ETag, Document: &doc})
}

// GET /api/sdui/config: all of ui_config.json.
func (s *Server) handleSDUIConfig(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	c := st.Current()
	if notModified(w, r, c.ETag) {
		return
	}
	writeJSONTagged(w, 200, c.ETag, c)
}

// writeStoreErr answers for an error from the store.
func writeUIStoreErr(w http.ResponseWriter, err error) {
	var inv *sduistore.InvalidError
	switch {
	case errors.Is(err, sduistore.ErrConflict):
		writeErr(w, 412, err.Error()+": read it again and redo the change")
	case errors.Is(err, sduistore.ErrNoRevision):
		writeErr(w, 404, err.Error())
	case errors.As(err, &inv):
		writeJSON(w, 400, map[string]any{"error": inv.Error(), "issues": inv.Issues})
	default:
		writeErr(w, 500, err.Error())
	}
}

func adminName(r *http.Request) string {
	if u := userFrom(r); u != nil {
		return u.Name
	}
	return ""
}

// PUT /api/sdui/config: replace ui_config.json. If-Match must be the ETag it was read with.
func (s *Server) handleSDUIConfigPut(w http.ResponseWriter, r *http.Request) {
	ifMatch := strings.TrimSpace(r.Header.Get("If-Match"))
	if ifMatch == "" {
		writeErr(w, 428, "If-Match: send the ETag of the configuration this change was made to")
		return
	}
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, sduistore.MaxBytes+1))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	c, err := sduistore.Parse(b)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if c.Version == 0 {
		c.Version = sduistore.FormatVersion
	}
	saved, err := st.Replace(c, ifMatch, adminName(r))
	if err != nil {
		writeUIStoreErr(w, err)
		return
	}
	s.Log.InfoContext(r.Context(), "UI configuration saved", "cat", "ui", "by", adminName(r), "revision", saved.Revision)
	writeJSONTagged(w, 200, saved.ETag, saved)
}

// GET /api/sdui/config/history: the revisions kept, newest first.
func (s *Server) handleSDUIHistory(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h, err := st.History()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	c := st.Current()
	writeJSON(w, 200, map[string]any{"current": c.Revision, "etag": c.ETag, "keep": sduistore.MaxHistory, "revisions": h})
}

// POST /api/sdui/config/rollback/{rev}: make a kept revision current again, as a new one.
func (s *Server) handleSDUIRollback(w http.ResponseWriter, r *http.Request) {
	rev, ok := pathInt(r, "rev")
	if !ok || rev < 1 {
		writeErr(w, 400, "rev: a revision number")
		return
	}
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	saved, err := st.Rollback(rev, strings.TrimSpace(r.Header.Get("If-Match")), adminName(r))
	if err != nil {
		writeUIStoreErr(w, err)
		return
	}
	s.Log.InfoContext(r.Context(), "UI configuration rolled back", "cat", "ui", "by", adminName(r), "to", rev, "revision", saved.Revision)
	writeJSONTagged(w, 200, saved.ETag, saved)
}
