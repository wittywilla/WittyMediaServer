// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/sduistore"
	"github.com/wittywilla/wittymediaserver/internal/themes"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// Modules' SDUI (docs/SDUI-MODULES.md): each module in use carries its pages, sections,
// components, sources, views, menu entries, settings and theme scope in its sdui.json,
// which the module manager has checked against the catalogue. Here they are merged into
// what every app is sent: pages after the administrator's and before the built-in ones,
// inserts into the built-in pages, entries into the menu, sources answered by the module's
// helper through the profile's limits, and the module's colours under the
// administrator's.

// sduiModules keeps the module sets made from the modules in use: one per set of modules
// a profile is kept from, which is a handful at most.
type sduiModules struct {
	mu      sync.Mutex
	gen     uint64
	built   bool
	sets    map[string]*sdui.ModuleSet
	catalog *sdui.Catalogue
	catETag string
	// settingsMu: a module's server settings are read, changed and written back whole.
	settingsMu sync.Mutex
}

const maxModuleSets = 16

// uiChecks are the theme engine's checks a module's sdui.json is held to.
var uiChecks = sdui.ModuleChecks{
	ColorToken: themes.IsColorToken,
	Color:      func(v string) bool { _, ok := themes.NormalizeColor(v); return ok },
}

// UIChecks is what the module manager checks a module's colours with.
func UIChecks() sdui.ModuleChecks { return uiChecks }

// moduleSet is the SDUI of the modules in use, without those in hidden.
func (s *Server) moduleSet(hidden []string) *sdui.ModuleSet {
	if s.Modules == nil {
		return nil
	}
	uis, gen := s.Modules.UIs()
	st := &s.sdui.mods
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.built || st.gen != gen {
		st.built, st.gen, st.sets, st.catalog, st.catETag = true, gen, map[string]*sdui.ModuleSet{}, nil, ""
	}
	hidden = slices.Clone(hidden)
	sort.Strings(hidden)
	key := strings.Join(hidden, ",")
	if set, ok := st.sets[key]; ok {
		return set
	}
	keep := uis[:0:0]
	for _, ui := range uis {
		if !slices.Contains(hidden, ui.Module) {
			keep = append(keep, ui)
		}
	}
	set := sdui.NewModuleSet(keep)
	if len(st.sets) >= maxModuleSets {
		clear(st.sets)
	}
	st.sets[key] = set
	return set
}

// modulesFor is the module set a request's profile sees: none of the modules its
// parental switches keep it from.
func (s *Server) modulesFor(r *http.Request) *sdui.ModuleSet {
	_, hidden := profileHidden(accessFor(userFrom(r)))
	return s.moduleSet(hidden)
}

// sduiCatalog is the catalogue with every module in use in it, and its ETag.
func (s *Server) sduiCatalog() (*sdui.Catalogue, string) {
	set := s.moduleSet(nil)
	st := &s.sdui.mods
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.catalog == nil {
		st.catalog = sdui.CatalogWith(set)
		st.catETag = hashETag("catalog", st.catalog)
	}
	return st.catalog, st.catETag
}

// effectivePages are the pages in force: the configuration's own first, then the modules'
// it doesn't replace, then every built-in page it doesn't replace, with the modules'
// inserts. Among pages of one kind that match as well, the first wins (sdui.SelectPage).
func effectivePages(cfg *sduistore.Config, set *sdui.ModuleSet) []sdui.Page {
	ids := make([]string, 0, len(cfg.Pages))
	for id := range cfg.Pages {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]sdui.Page, 0, len(ids)+32)
	for _, id := range ids {
		out = append(out, cfg.Pages[id])
	}
	for _, p := range set.Pages() {
		if _, replaced := cfg.Pages[p.ID]; !replaced {
			out = append(out, p)
		}
	}
	for _, p := range sdui.StockPages() {
		if _, replaced := cfg.Pages[p.ID]; !replaced {
			out = append(out, set.WithInserts(p))
		}
	}
	return out
}

func effectivePage(cfg *sduistore.Config, id string, set *sdui.ModuleSet) (sdui.Page, bool) {
	if p, ok := cfg.Pages[id]; ok {
		return p, true
	}
	if p, ok := set.Page(id); ok {
		return p, true
	}
	p, ok := sdui.StockPage(id)
	if ok {
		p = set.WithInserts(p)
	}
	return p, ok
}

// sduiScopes are the theme scopes' colours: each module's own, with the administrator's
// overrides for the scope over them token by token.
func sduiScopes(cfg *sduistore.Config, set *sdui.ModuleSet) map[string]map[string]string {
	out := set.Scopes()
	if out == nil {
		out = map[string]map[string]string{}
	}
	for scope, toks := range cfg.Theme.Modules {
		out[scope] = overlay(out[scope], toks)
	}
	return out
}

// moduleNavigation is the menu with the modules' entries put in before Settings, except
// those the menu already holds: an administrator who placed or hid one keeps that.
func moduleNavigation(nav sdui.Navigation, set *sdui.ModuleSet) sdui.Navigation {
	extra := set.Navigation()
	if len(extra) == 0 {
		return nav
	}
	have := map[string]bool{}
	at := len(nav.Items)
	for i, it := range nav.Items {
		have[it.ID] = true
		if it.Type == "native" && it.Screen == "settings" && at == len(nav.Items) {
			at = i
		}
	}
	var add []sdui.NavItem
	for _, it := range extra {
		if !have[it.ID] && len(nav.Items)+len(add) < sdui.MaxNavItems {
			have[it.ID] = true
			add = append(add, it)
		}
	}
	items := append(append(append([]sdui.NavItem(nil), nav.Items[:at]...), add...), nav.Items[at:]...)
	return sdui.Navigation{Items: items}
}

// moduleViewer is who a module is told is asking.
func (rv *sduiResolver) moduleViewer() modmgr.Viewer {
	return modmgr.Viewer{ProfileKind: rv.u.Kind, Admin: rv.u.IsAdmin, Cannot: sduiCannot(rv.acc), FormFactor: rv.formFactor, Platform: rv.platform, Locale: rv.locale}
}

// moduleItems asks a module's helper for one of its sources and keeps only what is safe to
// send and the profile may see.
func (rv *sduiResolver) moduleItems(mod string, req sdui.SourceRequest) (sdui.SourceResult, error) {
	s := rv.s
	if s.Modules == nil {
		return sdui.SourceResult{}, nil
	}
	ask := modmgr.ItemsRequest{Source: req.Source, Filter: req.Filter, Offset: req.Offset, Limit: req.Limit, Viewer: rv.moduleViewer(),
		Settings: s.moduleSettingValues(rv.r, mod)}
	ans, err := s.Modules.UIItems(mod, ask)
	if err != nil {
		return sdui.SourceResult{}, err
	}
	out := sdui.SourceResult{Total: ans.Total, More: ans.More}
	var yields []string
	if ui := s.Modules.UI(mod); ui != nil {
		for _, src := range ui.Sources {
			if src.Type == req.Source.Type() {
				yields = src.Yields
			}
		}
	}
	for i := range ans.Items {
		if len(out.Items) >= req.Limit {
			out.More = true
			break
		}
		it, ok := rv.cleanModuleItem(mod, ans.Items[i], yields)
		if ok {
			out.Items = append(out.Items, it)
		}
	}
	if out.Total < len(out.Items)+req.Offset {
		out.Total = -1
	}
	return out, nil
}

var moduleItemCan = []string{"play", "download", "share", "watchLater", "watched", "playOn", "watchParty"}

// cleanModuleItem holds an item a module gave to what a card may carry: its own kinds, a
// place in the app, pictures on this server, nothing over length, and nothing the profile
// may not see. A title it says can be played must be one of the library's that the profile
// may play.
func (rv *sduiResolver) cleanModuleItem(mod string, it sdui.Item, yields []string) (sdui.Item, bool) {
	if it.ID == "" || len(it.ID) > 200 || !slices.Contains(yields, it.Kind) || strings.TrimSpace(it.Title) == "" {
		return it, false
	}
	if !rv.acc.AllowsOutside(it.Kind, it.ContentRating, it.Genres) {
		return it, false
	}
	clip := func(s string, n int) string {
		if r := []rune(s); len(r) > n {
			return string(r[:n])
		}
		return s
	}
	it.Title, it.Subtitle, it.Overview, it.Tagline = clip(it.Title, 300), clip(it.Subtitle, 300), clip(it.Overview, 2000), clip(it.Tagline, 300)
	if it.Route != "" && !sdui.ValidRoute(it.Route) {
		it.Route = ""
	}
	for k, v := range it.Images {
		if !slices.Contains([]string{"poster", "backdrop", "square", "logo"}, k) || !sdui.ValidRoute(v) || !strings.HasPrefix(v, "/api/") {
			delete(it.Images, k)
		}
	}
	it.Can = slices.DeleteFunc(it.Can, func(c string) bool { return !slices.Contains(moduleItemCan, c) })
	for _, id := range []*string{&it.PlayID, &it.NextID} {
		if *id == "" {
			continue
		}
		n, err := strconv.ParseInt(*id, 10, 64)
		li, ierr := rv.s.Store.Item(rv.ctx, n)
		if err != nil || ierr != nil || !rv.acc.AllowsItem(li, nil) || rv.noPlay {
			*id = ""
		}
	}
	// What a title page offers acts on a title of the library: an item that isn't one
	// offers nothing.
	if it.PlayID == "" {
		it.Can = nil
	}
	if len(it.Badges) > 3 {
		it.Badges = it.Badges[:3]
	}
	for i := range it.Badges {
		it.Badges[i] = clip(it.Badges[i], 12)
	}
	if it.Icon != "" && !slices.Contains(sdui.Icons, it.Icon) {
		it.Icon = ""
	}
	if it.Scope == "" {
		it.Scope = mod
	}
	return it, true
}

// ---------------------------------------------------------------------------
// Routes.
// ---------------------------------------------------------------------------

func (s *Server) sduiModuleRoutes(m *http.ServeMux) {
	m.Handle("GET /api/sdui/modules", s.requireAdmin(s.handleSDUIModules))
	m.Handle("GET /api/sdui/modules/{module}/images/{name}", s.requireUser(s.handleSDUIModuleImage))
	m.Handle("GET /api/sdui/modules/settings", s.requireUser(s.handleModuleSettingsList))
	m.Handle("GET /api/sdui/modules/{module}/settings", s.requireUser(s.handleModuleSettingsGet))
	m.Handle("PUT /api/sdui/modules/{module}/settings", s.requireAdmin(s.handleModuleSettingsPut))
	m.Handle("PUT /api/sdui/modules/{module}/settings/personal", s.requireUser(s.handleModuleSettingsPersonalPut))
}

// GET /api/sdui/modules (administrators): every module in use with its SDUI, as the page
// editor shows where pages, rows and entries come from.
func (s *Server) handleSDUIModules(w http.ResponseWriter, r *http.Request) {
	out := []sdui.ModuleUI{}
	if s.Modules != nil {
		uis, _ := s.Modules.UIs()
		out = append(out, uis...)
	}
	writeJSON(w, 200, map[string]any{"modules": out})
}

// moduleInUse answers the module a request names when it is in use and the profile isn't
// kept from it; it has written the refusal when not.
func (s *Server) moduleInUse(w http.ResponseWriter, r *http.Request) (*sdui.ModuleUI, bool) {
	mod := r.PathValue("module")
	if !sdui.ValidModuleID(mod) || s.Modules == nil {
		writeErr(w, 404, "no such module")
		return nil, false
	}
	_, hidden := profileHidden(accessFor(userFrom(r)))
	ui := s.Modules.UI(mod)
	if ui == nil || slices.Contains(hidden, mod) {
		writeErr(w, 404, "no such module")
		return nil, false
	}
	return ui, true
}

var moduleImageName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// GET /api/sdui/modules/{module}/images/{name}: a picture a module's cards show, from its
// helper. Only pictures are passed on, as the type they really are.
func (s *Server) handleSDUIModuleImage(w http.ResponseWriter, r *http.Request) {
	ui, ok := s.moduleInUse(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !moduleImageName.MatchString(name) || strings.Contains(name, "..") {
		writeErr(w, 400, "a picture's name is letters, digits, . _ and -")
		return
	}
	b, err := s.Modules.UIImage(ui.Module, name)
	if err != nil {
		writeErr(w, 404, "the module has no such picture")
		return
	}
	ct := http.DetectContentType(b)
	if !slices.Contains([]string{"image/png", "image/jpeg", "image/webp", "image/gif"}, ct) {
		writeErr(w, 502, "the module answered something that isn't a picture")
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(b)
}

// ---------------------------------------------------------------------------
// Settings a module describes. Every app's own Settings draws them; the server keeps
// the values: the server's in its settings, a profile's own with the profile.
// ---------------------------------------------------------------------------

func moduleSettingKey(mod string) string { return "module." + mod + ".settings" }

// settingRows are a module's settings rows that hold a value, by key.
func settingRows(ui *sdui.ModuleUI) map[string]*sdui.SettingRow {
	out := map[string]*sdui.SettingRow{}
	for i := range ui.Settings {
		for g := range ui.Settings[i].Groups {
			for r := range ui.Settings[i].Groups[g].Rows {
				row := &ui.Settings[i].Groups[g].Rows[r]
				if row.Kind != sdui.SettingInfo {
					out[row.Key] = row
				}
			}
		}
	}
	return out
}

func rowScope(row *sdui.SettingRow) string {
	if row.Scope == "" {
		return sdui.SettingScopeServer
	}
	return row.Scope
}

func (s *Server) serverModuleSettings(ctx context.Context, mod string) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal([]byte(s.DB.GetSetting(ctx, moduleSettingKey(mod), "{}")), &out)
	return out
}

func (s *Server) personalModuleSettings(r *http.Request, mod string) map[string]any {
	var ui struct {
		Modules map[string]map[string]any `json:"modules"`
	}
	_ = json.Unmarshal([]byte(s.Auth.UI(r.Context(), userFrom(r).ID)), &ui)
	if ui.Modules[mod] == nil {
		return map[string]any{}
	}
	return ui.Modules[mod]
}

// withDefaults fills in each row's default where no value was saved, and drops values for
// rows the module no longer has or that no longer fit.
func withDefaults(rows map[string]*sdui.SettingRow, scope string, saved map[string]any) map[string]any {
	out := map[string]any{}
	for k, row := range rows {
		if rowScope(row) != scope {
			continue
		}
		if v, ok := saved[k]; ok && sdui.CheckSetting(row, v) == nil {
			out[k] = v
		} else if row.Default != nil {
			out[k] = row.Default
		}
	}
	return out
}

// moduleSettingValues are a module's settings for the profile asking: the server's, then
// the profile's own, with defaults. What a module's helper is told with each question.
func (s *Server) moduleSettingValues(r *http.Request, mod string) map[string]any {
	if s.Modules == nil {
		return nil
	}
	ui := s.Modules.UI(mod)
	if ui == nil || len(ui.Settings) == 0 {
		return nil
	}
	rows := settingRows(ui)
	out := withDefaults(rows, sdui.SettingScopeServer, s.serverModuleSettings(r.Context(), mod))
	for k, v := range withDefaults(rows, sdui.SettingScopeProfile, s.personalModuleSettings(r, mod)) {
		out[k] = v
	}
	return out
}

// ModuleSettingsEnv is a module's server settings, with defaults, as environment for its
// helper when it starts.
func (s *Server) ModuleSettingsEnv(mod string) []string {
	if s.Modules == nil {
		return nil
	}
	ui := s.Modules.UI(mod)
	if ui == nil || len(ui.Settings) == 0 {
		return nil
	}
	vals := withDefaults(settingRows(ui), sdui.SettingScopeServer, s.serverModuleSettings(context.Background(), mod))
	b, err := json.Marshal(vals)
	if err != nil {
		return nil
	}
	return []string{modmgr.SettingsEnv + "=" + string(b)}
}

// moduleSettingsView is what GET …/settings answers: the pages the profile may see, the
// values, and which secrets are set (their values never leave the server).
type moduleSettingsView struct {
	Module     string              `json:"module"`
	Label      string              `json:"label,omitempty"`
	Pages      []sdui.SettingsPage `json:"pages"`
	Values     map[string]any      `json:"values"`
	SecretsSet []string            `json:"secretsSet"`
	// Editable says the profile may change the server's values (administrators only).
	Editable bool `json:"editable"`
}

func (s *Server) moduleSettingsView(r *http.Request, ui *sdui.ModuleUI) moduleSettingsView {
	u := userFrom(r)
	rows := settingRows(ui)
	label := ui.Theme.Label
	if label == "" {
		label = s.Modules.Name(ui.Module)
	}
	v := moduleSettingsView{Module: ui.Module, Label: label, Pages: []sdui.SettingsPage{}, Values: map[string]any{}, SecretsSet: []string{}, Editable: u.IsAdmin}
	for _, p := range ui.Settings {
		if !p.Admin || u.IsAdmin {
			v.Pages = append(v.Pages, p)
		}
	}
	saved := s.serverModuleSettings(r.Context(), ui.Module)
	for k, val := range withDefaults(rows, sdui.SettingScopeServer, saved) {
		if rows[k].Kind == sdui.SettingSecret {
			continue
		}
		v.Values[k] = val
	}
	for k, row := range rows {
		if row.Kind == sdui.SettingSecret {
			if sv, _ := saved[k].(string); sv != "" && u.IsAdmin {
				v.SecretsSet = append(v.SecretsSet, k)
			}
		}
	}
	sort.Strings(v.SecretsSet)
	for k, val := range withDefaults(rows, sdui.SettingScopeProfile, s.personalModuleSettings(r, ui.Module)) {
		v.Values[k] = val
	}
	// A profile that isn't an administrator sees only the values of the pages it sees.
	if !u.IsAdmin {
		seen := map[string]bool{}
		for _, p := range v.Pages {
			for _, g := range p.Groups {
				for _, row := range g.Rows {
					seen[row.Key] = true
				}
			}
		}
		for k := range v.Values {
			if !seen[k] {
				delete(v.Values, k)
			}
		}
	}
	return v
}

// moduleSettingsEntry is one module with settings this profile sees, for the list under
// Settings → Modules.
type moduleSettingsEntry struct {
	Module string `json:"module"`
	Label  string `json:"label"`
	Icon   string `json:"icon,omitempty"`
	URL    string `json:"url"`
}

// GET /api/sdui/modules/settings: the modules in use with settings this profile sees.
func (s *Server) handleModuleSettingsList(w http.ResponseWriter, r *http.Request) {
	out := []moduleSettingsEntry{}
	for _, ui := range s.modulesFor(r).Modules() {
		v := s.moduleSettingsView(r, &ui)
		if len(v.Pages) == 0 {
			continue
		}
		out = append(out, moduleSettingsEntry{Module: ui.Module, Label: v.Label, Icon: v.Pages[0].Icon, URL: "/api/v1/sdui/modules/" + ui.Module + "/settings"})
	}
	writeJSON(w, 200, map[string]any{"modules": out})
}

// GET /api/sdui/modules/{module}/settings: the module's settings for this profile.
func (s *Server) handleModuleSettingsGet(w http.ResponseWriter, r *http.Request) {
	ui, ok := s.moduleInUse(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, s.moduleSettingsView(r, ui))
}

// readSettingValues reads {"values": {key: value}} and checks every value against its row
// and scope. null removes a saved value, so the default applies again.
func readSettingValues(r *http.Request, rows map[string]*sdui.SettingRow, scope string) (map[string]any, error) {
	var body struct {
		Values map[string]any `json:"values"`
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		return nil, errors.New("expected {\"values\": {...}}")
	}
	keys := make([]string, 0, len(body.Values))
	for k := range body.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		row := rows[k]
		if row == nil {
			return nil, errors.New(k + ": the module has no such setting")
		}
		if rowScope(row) != scope {
			if scope == sdui.SettingScopeServer {
				return nil, errors.New(k + ": each profile sets this for itself")
			}
			return nil, errors.New(k + ": only an administrator sets this, for the whole server")
		}
		if v := body.Values[k]; v != nil {
			if err := sdui.CheckSetting(row, v); err != nil {
				return nil, err
			}
		}
	}
	return body.Values, nil
}

func mergeValues(saved, changes map[string]any) map[string]any {
	for k, v := range changes {
		if v == nil {
			delete(saved, k)
		} else {
			saved[k] = v
		}
	}
	return saved
}

// PUT /api/sdui/modules/{module}/settings (administrators): the server's values.
func (s *Server) handleModuleSettingsPut(w http.ResponseWriter, r *http.Request) {
	ui, ok := s.moduleInUse(w, r)
	if !ok {
		return
	}
	rows := settingRows(ui)
	changes, err := readSettingValues(r, rows, sdui.SettingScopeServer)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.sdui.mods.settingsMu.Lock()
	saved := mergeValues(s.serverModuleSettings(r.Context(), ui.Module), changes)
	b, _ := json.Marshal(saved)
	err = s.DB.SetSetting(r.Context(), moduleSettingKey(ui.Module), string(b))
	s.sdui.mods.settingsMu.Unlock()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Modules.SettingsChanged(ui.Module, withDefaults(rows, sdui.SettingScopeServer, saved))
	writeJSON(w, 200, s.moduleSettingsView(r, ui))
}

// PUT /api/sdui/modules/{module}/settings/personal: this profile's own values.
func (s *Server) handleModuleSettingsPersonalPut(w http.ResponseWriter, r *http.Request) {
	ui, ok := s.moduleInUse(w, r)
	if !ok {
		return
	}
	changes, err := readSettingValues(r, settingRows(ui), sdui.SettingScopeProfile)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// The profile's whole UI document is read, changed and written back, as its page
	// arrangements are.
	s.sdui.personalMu.Lock()
	defer s.sdui.personalMu.Unlock()
	uid := userFrom(r).ID
	doc := map[string]json.RawMessage{}
	_ = json.Unmarshal([]byte(s.Auth.UI(r.Context(), uid)), &doc)
	mods := map[string]map[string]any{}
	_ = json.Unmarshal(doc["modules"], &mods)
	if mods[ui.Module] == nil {
		mods[ui.Module] = map[string]any{}
	}
	mods[ui.Module] = mergeValues(mods[ui.Module], changes)
	if len(mods[ui.Module]) == 0 {
		delete(mods, ui.Module)
	}
	raw, _ := json.Marshal(mods)
	doc["modules"] = raw
	b, _ := json.Marshal(doc)
	if len(b) > 256<<10 {
		writeErr(w, 413, "this profile keeps too much already")
		return
	}
	if err := s.Auth.SetUI(r.Context(), uid, string(bytes.TrimSpace(b))); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, s.moduleSettingsView(r, ui))
}
