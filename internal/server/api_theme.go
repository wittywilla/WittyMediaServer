// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/brandname"
	"github.com/wittywilla/wittymediaserver/internal/sduistore"
	"github.com/wittywilla/wittymediaserver/internal/themes"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// The Master Theme Engine: one resolved theme per account, from
//
//  1. the server's default theme (the "theme" setting, as /api/themes reports it) with the
//     administrator's global colour overrides, kept in ui_config.json;
//  2. the account's own choice, the "theme" key of /api/me/ui, which replaces the theme and
//     with it the global overrides (they were chosen for the server's theme);
//  3. per-module overrides from ui_config.json, layered over either, for the parts of the
//     interface that belong to a module. The same overrides reach server-driven screens
//     through sdui.Render;
//  4. the profile's own colours (api_theme_personal.go), over all of those.
//
// resolveThemeWith has the exact order.
//
// /api/themes and /api/themes/{id}/tokens.json are unchanged; this is a layer over them.

func (s *Server) themeRoutes(m *http.ServeMux) {
	m.Handle("GET /api/theme", s.requireUser(s.handleTheme))
	m.Handle("PUT /api/theme/global", s.requireAdmin(s.handleThemeGlobalPut))
	m.Handle("GET /api/theme/modules/{module}", s.requireAdmin(s.handleThemeModule))
	m.Handle("PUT /api/theme/modules/{module}", s.requireAdmin(s.handleThemeModulePut))
	m.Handle("DELETE /api/theme/modules/{module}", s.requireAdmin(s.handleThemeModuleDelete))
	m.Handle("GET /api/theme/personal", s.requireUser(s.handleThemePersonal))
	m.Handle("PUT /api/theme/personal", s.requireUser(s.handleThemePersonalPut))
	m.Handle("DELETE /api/theme/personal", s.requireUser(s.handleThemePersonalDelete))
}

// ThemeWarning is text that would be hard to read: WCAG AA wants 4.5:1.
type ThemeWarning struct {
	// Scope is "global", "module:<id>", "personal" (the profile's own colours) or
	// "personal:<id>" (its own colours for a module).
	Scope string `json:"scope"`
	// Variant is "dark" or "light" when the warning is about one of an automatic theme's
	// two looks; empty for a theme with one look.
	Variant string `json:"variant,omitempty"`
	themes.ContrastIssue
}

// ResolvedTheme is what GET /api/theme answers.
type ResolvedTheme struct {
	// ID is the theme in use; Source says whose choice it is: "server" or "user".
	ID     string `json:"id"`
	Source string `json:"source"`
	// Tokens are its colours by token name, with the overrides applied: the global ones,
	// then the profile's own, each with the colours worked out from it (themes.Derive).
	Tokens map[string]string `json:"tokens"`
	// Overrides are the global overrides applied (none when the account chose its own theme).
	Overrides map[string]string `json:"overrides"`
	// Personal are the profile's own colour overrides (GET /api/theme/personal).
	Personal map[string]string `json:"personal"`
	// Modules are the colours for each module scope with colours of its own: the whole
	// palette, with the scope's colours and the profile's over Tokens.
	Modules map[string]ThemeModule `json:"modules"`
	// Scheme is "dark" or "light": which the theme is. "auto" for the automatic theme,
	// which is dark or light as the device is; Tokens and Modules are then its dark look
	// and Variants has both.
	Scheme string `json:"scheme"`
	// Shape is the theme's corners and focus ring in points, Type its typefaces and
	// weights: what SDUI components' tokens.shape and tokens.type name.
	Shape themes.Shape      `json:"shape"`
	Type  themes.TypeTokens `json:"type"`
	// TV, Elevation and Density are Design 2's additions (docs/plans/DESIGN-2.md section 2):
	// the ten-foot colours and units, the surface each elevation step fills with, and the
	// spacing steps. Clients that don't know them ignore them.
	TV        themes.TV                       `json:"tv"`
	Elevation map[string]themes.ElevationStep `json:"elevation"`
	Density   themes.Density                  `json:"density"`
	// Look is the experience and theme in force and who required what: the choice order is
	// required, the trial asked for, the person's own, the server's, the experience's own
	// (docs/plans/DESIGN-2.md section 8).
	Look ThemeLook `json:"look"`
	// Brand is the server's name ready to draw, and the colour token its coloured parts
	// take, resolved like every other colour.
	Brand ThemeBrand `json:"brand"`
	// Variants are the theme's looks by scheme, each resolved the same way: one entry for
	// an ordinary theme, "dark" and "light" for the automatic one. A client that follows
	// the device's setting picks from here.
	Variants map[string]ThemeVariant `json:"variants"`
	Warnings []ThemeWarning          `json:"warnings"`
	ETag     string                  `json:"etag"`
	// Revision is ui_config.json's revision the overrides come from.
	Revision int64 `json:"revision"`
}

// ThemeBrand is the server's name as GET /api/status gives it (brandParts), and how its
// parts are coloured: parts with accent in Token's colour (Color, from Tokens; a variant's
// from its own tokens), the rest in fg.
type ThemeBrand struct {
	Parts []brandname.Part `json:"parts"`
	Token string           `json:"token"`
	Color string           `json:"color"`
	// Mark is the badge for small spaces (the rail's header); Warnings say where a fixed
	// colour of the name is under 4.5:1 on a surface it is drawn on, in this theme.
	Mark     ThemeMark      `json:"mark"`
	Warnings []BrandWarning `json:"warnings"`
}

// ThemeMark is the name's badge: what it holds and the text it shows.
type ThemeMark struct {
	brandname.Mark
	// Shown is the characters the badge draws (the first letter, the initials or the owner's own).
	Shown string `json:"shown"`
}

// ShownFor fills Shown.
func (m ThemeMark) ShownFor(name string) string { return m.Mark.Shown(name) }

// BrandWarning is one part of the name that is hard to read on a surface.
type BrandWarning struct {
	Text string `json:"text"`
	// Surface is the colour token the name sits on (bg, bg2, bg3) and Where what is drawn on it:
	// the top bar and page (bg), the side menu and bars (bg2), the bevelled bars (bg3).
	Surface string  `json:"surface"`
	Where   string  `json:"where"`
	Ratio   float64 `json:"ratio"`
	Need    float64 `json:"need"`
	// Variant is "dark" or "light" for one of an automatic theme's two looks, else empty.
	Variant string `json:"variant,omitempty"`
}

// brandSurfaces are the surfaces the server's name is drawn on, and where they are.
var brandSurfaces = []struct{ token, where string }{
	{"bg", "top bar and page"}, {"bg2", "side menu and bars"}, {"bg3", "bevelled bars"},
}

// ThemeVariant is one look of a theme: the built-in theme behind it, its colours with the
// global and the profile's overrides applied, each module's whole palette, and its shape
// and type.
type ThemeVariant struct {
	ID      string                       `json:"id"`
	Tokens  map[string]string            `json:"tokens"`
	Modules map[string]map[string]string `json:"modules"`
	Shape   themes.Shape                 `json:"shape"`
	Type    themes.TypeTokens            `json:"type"`
	// The same Design 2 additions, in this look's colours.
	TV        themes.TV                       `json:"tv"`
	Elevation map[string]themes.ElevationStep `json:"elevation"`
}

// The built-in themes the automatic theme is, by the device's setting.
const (
	autoDark  = "default"
	autoLight = "light"
)

// themeScheme is "dark" or "light" for a theme, from its own theme.json.
func (s *Server) themeScheme(id string) string {
	if s.Themes != nil {
		if m, ok := s.Themes.One(id); ok {
			return m.Scheme
		}
	}
	if id == autoLight {
		return "light"
	}
	return "dark"
}

// ThemeModule is one module scope's colours.
type ThemeModule struct {
	Tokens    map[string]string `json:"tokens"`
	Overrides map[string]string `json:"overrides"`
	// Personal are the profile's own colours for the scope.
	Personal map[string]string `json:"personal,omitempty"`
}

// themeChoices are what the account chose for itself, from its interface choices: the
// theme (the "theme" key of /api/me/ui) and its own colours ("themePersonal").
type themeChoices struct {
	Theme    string
	Personal themePersonal
}

func (s *Server) themeChoicesOf(r *http.Request) themeChoices {
	var out themeChoices
	raw := s.Auth.UI(r.Context(), userFrom(r).ID)
	if raw == "" {
		return out
	}
	var ui struct {
		Theme    any             `json:"theme"`
		Personal json.RawMessage `json:"themePersonal"`
	}
	if json.Unmarshal([]byte(raw), &ui) != nil {
		return out
	}
	if len(ui.Personal) > 0 {
		_ = json.Unmarshal(ui.Personal, &out.Personal)
	}
	id, _ := ui.Theme.(string)
	id = strings.ToLower(strings.TrimSpace(id))
	if id != "" && id != "auto" && s.themeKnown(id) {
		out.Theme = id
	}
	return out
}

// userThemeChoice is the theme the account picked for itself, if it is here.
func (s *Server) userThemeChoice(r *http.Request) string { return s.themeChoicesOf(r).Theme }

func (s *Server) themeKnown(id string) bool {
	_, err := s.Themes.Colors(id)
	return err == nil
}

// baseColors are a theme's own colours; "auto" (light or dark by the device) and an id
// that's gone are the default theme's.
func (s *Server) baseColors(id string) map[string]string {
	if id != "auto" {
		if c, err := s.Themes.Colors(id); err == nil {
			return c
		}
	}
	c, _ := s.Themes.Colors("default")
	if c == nil {
		c = map[string]string{}
	}
	return c
}

func overlay(base map[string]string, over map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// layer lays over on pal with the colours worked out from it, and says which tokens it
// changed (for the contrast check).
func layer(pal, over map[string]string) (out, changed map[string]string) {
	changed = themes.Derive(pal, over)
	return overlay(pal, changed), changed
}

// union is the keys of a and b, as a set to range over.
func union(a, b map[string]string) map[string]string {
	if len(a) == 0 {
		return b
	}
	return overlay(a, b)
}

// resolveTheme is the theme for this request.
func (s *Server) resolveTheme(r *http.Request) (*ResolvedTheme, error) {
	st, err := s.uiStore()
	if err != nil {
		return nil, err
	}
	cfg := st.Current()
	choices := s.themeChoicesOf(r)
	return s.resolveThemeWith(r, cfg, s.moduleSet(nil), choices), nil
}

// resolveThemeWith resolves a theme from the server's settings and the account's choices.
// The order, lowest first (docs/THEMES.md "Precedence"):
//
//  1. the theme pack (the server's, or the account's own choice);
//  2. the administrator's global overrides (left out when the account chose its own theme:
//     they were chosen for the server's);
//  3. a module scope's own colours (its sdui.json), then 4. the administrator's overrides
//     for the scope;
//  5. the profile's own overrides, then 6. the profile's own for the scope.
//
// Each layer brings the colours worked out from it (themes.Derive), over what is under it,
// so a layer that sets only the accent gets a hover, tints and focus ring to match.
func (s *Server) resolveThemeWith(r *http.Request, cfg *sduistore.Config, set *sdui.ModuleSet, choices themeChoices) *ResolvedTheme {
	look := s.lookFor(r, cfg, choices)
	t := &ResolvedTheme{ID: look.Theme, Source: look.ThemeSource, Modules: map[string]ThemeModule{},
		Overrides: map[string]string{}, Personal: overlay(nil, choices.Personal.Tokens), Warnings: []ThemeWarning{}, Revision: cfg.Revision, Look: look.ThemeLook}
	if t.Source != "user" && t.Source != "trial" {
		t.Overrides = overlay(nil, cfg.Theme.Tokens)
	}
	// Each look is resolved the same way. An override that reads well on the dark look can
	// be unreadable on the light one, so each look gets its own warnings.
	looks := []struct{ scheme, id string }{{s.themeScheme(t.ID), t.ID}}
	t.Scheme = looks[0].scheme
	if t.ID == "auto" {
		looks = []struct{ scheme, id string }{{"dark", autoDark}, {"light", autoLight}}
		t.Scheme = "auto"
	}
	scopes := sduiScopes(cfg, set)
	own := choices.Personal.Modules
	names := map[string]bool{}
	for k := range scopes {
		names[k] = true
	}
	for k := range own {
		names[k] = true
	}
	t.Variants = map[string]ThemeVariant{}
	for i, look := range looks {
		label := ""
		if len(looks) > 1 {
			label = look.scheme
		}
		warn := func(scope string, colors, changed map[string]string) {
			for _, w := range contrastWarnings(scope, colors, changed) {
				w.Variant = label
				t.Warnings = append(t.Warnings, w)
			}
		}
		shape, typ := s.Themes.Look(look.id)
		admin, g := layer(s.baseColors(look.id), t.Overrides)
		if len(t.Overrides) > 0 {
			warn("global", admin, g)
		}
		tokens, p := layer(admin, t.Personal)
		if len(t.Personal) > 0 {
			warn("personal", tokens, p)
		}
		v := ThemeVariant{ID: look.id, Tokens: tokens, Modules: map[string]map[string]string{}, Shape: shape, Type: typ,
			TV: themes.TVFor(s.Themes.TVColorsOf(look.id), s.baseColors(look.id), tokens), Elevation: themes.ElevationFor(tokens)}
		for _, mod := range sortedStringKeys(names) {
			// The scope's colours over the administrator's palette, warned about as
			// before; then the profile's own, and its own for the scope, over those.
			modPal, m := layer(admin, scopes[mod])
			if len(scopes[mod]) > 0 {
				warn("module:"+mod, modPal, m)
			}
			withOwn, p2 := layer(modPal, t.Personal)
			final, pm := layer(withOwn, own[mod])
			if mine := union(p2, pm); len(mine) > 0 && (len(t.Personal) > 0 || len(own[mod]) > 0) {
				warn("personal:"+mod, final, mine)
			}
			v.Modules[mod] = final
			if i == 0 {
				t.Modules[mod] = ThemeModule{Tokens: final, Overrides: overlay(nil, cfg.Theme.Modules[mod]), Personal: own[mod]}
			}
		}
		t.Variants[look.scheme] = v
		if i == 0 {
			t.Tokens, t.Shape, t.Type = v.Tokens, shape, typ
			t.TV, t.Elevation, t.Density = v.TV, v.Elevation, themes.DensityTable()
		}
	}
	t.Warnings = dedupeWarnings(t.Warnings)
	name, _, parts := s.brand(r.Context())
	t.Brand = ThemeBrand{Parts: parts, Token: "brandAccent", Color: t.Tokens["brandAccent"], Mark: ThemeMark{Mark: s.brandMark(r.Context())}, Warnings: brandWarningsOf(parts, t)}
	t.Brand.Mark.Shown = t.Brand.Mark.Mark.Shown(firstNonEmpty(name, brandname.Default))
	t.ETag = themeETag(t)
	return t
}

// dedupeWarnings drops a scope's personal warning that the profile's global one already
// gives for the same pair and look.
func dedupeWarnings(in []ThemeWarning) []ThemeWarning {
	seen := map[string]bool{}
	out := in[:0]
	for _, w := range in {
		key := w.Variant + "|" + w.Text + "|" + w.Surface + "|" + strconv.FormatFloat(w.Ratio, 'f', 2, 64)
		if strings.HasPrefix(w.Scope, "personal:") && seen["personal|"+key] {
			continue
		}
		seen[w.Scope+"|"+key] = true
		out = append(out, w)
	}
	return out
}

func themeETag(t *ResolvedTheme) string {
	cp := *t
	cp.ETag = ""
	b, _ := json.Marshal(cp) // map keys sorted
	sum := sha256.Sum256(b)
	return `W/"theme-` + hex.EncodeToString(sum[:])[:32] + `"`
}

func contrastWarnings(scope string, colors, changed map[string]string) []ThemeWarning {
	var out []ThemeWarning
	for _, is := range themes.CheckContrast(colors, changed) {
		out = append(out, ThemeWarning{Scope: scope, ContrastIssue: is})
	}
	return out
}

func sortedStringKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// GET /api/theme: the theme for the signed-in account, resolved.
func (s *Server) handleTheme(w http.ResponseWriter, r *http.Request) {
	t, err := s.resolveTheme(r)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if notModified(w, r, t.ETag) {
		return
	}
	writeJSONTagged(w, 200, t.ETag, t)
}

// maxThemeTokens bounds one set of overrides.
const maxThemeTokens = 64

// checkTokens normalises a set of overrides: known token names, real colours. The error
// names everything wrong.
func checkTokens(in map[string]string) (map[string]string, error) {
	if len(in) > maxThemeTokens {
		return nil, errors.New("too many tokens: at most 64")
	}
	out := make(map[string]string, len(in))
	var bad []string
	for _, k := range sortedStringKeys(in) {
		if !themes.IsColorToken(k) {
			bad = append(bad, k+": not a colour token themes have")
			continue
		}
		c, ok := themes.NormalizeColor(in[k])
		if !ok {
			bad = append(bad, k+": not a colour (#rrggbb, #rrggbbaa or rgb()/rgba())")
			continue
		}
		out[k] = c
	}
	if len(bad) > 0 {
		return nil, errors.New(strings.Join(bad, "; ") + ". Known tokens: " + strings.Join(themes.ColorTokens(), ", "))
	}
	return out, nil
}

// PUT /api/theme/global {id?, tokens?}: the server's default theme and its colour overrides.
// A field left out stays as it is; "tokens": {} takes the overrides away.
func (s *Server) handleThemeGlobalPut(w http.ResponseWriter, r *http.Request) {
	var b struct {
		ID     *string            `json:"id"`
		Tokens *map[string]string `json:"tokens"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON object: {id, tokens}")
		return
	}
	if b.ID == nil && b.Tokens == nil {
		writeErr(w, 400, "nothing to change: send id, tokens or both")
		return
	}
	var id string
	if b.ID != nil {
		id = strings.ToLower(strings.TrimSpace(*b.ID))
		if id != "auto" && !s.themeKnown(id) {
			writeErr(w, 400, "no such theme")
			return
		}
	}
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if b.Tokens != nil {
		toks, err := checkTokens(*b.Tokens)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if _, err := st.Update(strings.TrimSpace(r.Header.Get("If-Match")), adminName(r), func(c *sduistore.Config) error {
			c.Theme.Tokens = toks
			if len(toks) == 0 {
				c.Theme.Tokens = nil
			}
			return nil
		}); err != nil {
			s.writeUIStoreErr(w, r, "server theme", err)
			return
		}
	}
	if b.ID != nil {
		if err := s.DB.SetSetting(r.Context(), "theme", id); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	s.Log.InfoContext(r.Context(), "server theme changed", "cat", "sdui", "by", adminName(r), "theme", id, "tokens", b.Tokens != nil)
	s.answerTheme(w, r)
}

func (s *Server) answerTheme(w http.ResponseWriter, r *http.Request) {
	t, err := s.resolveTheme(r)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSONTagged(w, 200, t.ETag, t)
}

// themeModuleView is one module's overrides, as the administrator's routes answer.
type themeModuleView struct {
	Module   string            `json:"module"`
	Tokens   map[string]string `json:"tokens"`
	Warnings []ThemeWarning    `json:"warnings"`
	ETag     string            `json:"etag"`
}

func (s *Server) moduleView(r *http.Request, mod string, cfg *sduistore.Config) themeModuleView {
	ov := overlay(nil, cfg.Theme.Modules[mod])
	v := themeModuleView{Module: mod, Tokens: ov, Warnings: []ThemeWarning{}, ETag: cfg.ETag}
	admin, _ := layer(s.baseColors(s.serverTheme(r.Context())), cfg.Theme.Tokens)
	pal, changed := layer(admin, ov)
	v.Warnings = append(v.Warnings, contrastWarnings("module:"+mod, pal, changed)...)
	return v
}

func moduleParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	mod := r.PathValue("module")
	if !sduistore.ValidModuleID(mod) {
		writeErr(w, 400, "module: a module id, lower-case letters, digits, - and _")
		return "", false
	}
	return mod, true
}

// GET /api/theme/modules/{module}: a module's colour overrides.
func (s *Server) handleThemeModule(w http.ResponseWriter, r *http.Request) {
	mod, ok := moduleParam(w, r)
	if !ok {
		return
	}
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	cfg := st.Current()
	writeJSONTagged(w, 200, cfg.ETag, s.moduleView(r, mod, cfg))
}

// PUT /api/theme/modules/{module} {tokens}: replace a module's colour overrides.
func (s *Server) handleThemeModulePut(w http.ResponseWriter, r *http.Request) {
	mod, ok := moduleParam(w, r)
	if !ok {
		return
	}
	var b struct {
		Tokens map[string]string `json:"tokens"`
	}
	if err := readJSON(r, &b); err != nil || b.Tokens == nil {
		writeErr(w, 400, `expected {"tokens": {"<token>": "<colour>"}}`)
		return
	}
	toks, err := checkTokens(b.Tokens)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.setModuleTokens(w, r, mod, toks)
}

// DELETE /api/theme/modules/{module}: the module goes back to the theme's colours.
func (s *Server) handleThemeModuleDelete(w http.ResponseWriter, r *http.Request) {
	mod, ok := moduleParam(w, r)
	if !ok {
		return
	}
	s.setModuleTokens(w, r, mod, nil)
}

func (s *Server) setModuleTokens(w http.ResponseWriter, r *http.Request, mod string, toks map[string]string) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	cfg, err := st.Update(strings.TrimSpace(r.Header.Get("If-Match")), adminName(r), func(c *sduistore.Config) error {
		if len(toks) == 0 {
			delete(c.Theme.Modules, mod)
			if len(c.Theme.Modules) == 0 {
				c.Theme.Modules = nil
			}
			return nil
		}
		if c.Theme.Modules == nil {
			c.Theme.Modules = map[string]map[string]string{}
		}
		c.Theme.Modules[mod] = toks
		return nil
	})
	if err != nil {
		s.writeUIStoreErr(w, r, "module colours", err)
		return
	}
	if len(toks) == 0 {
		s.Log.InfoContext(r.Context(), "module colours reset", "cat", "sdui", "module", mod, "by", adminName(r))
	} else {
		s.Log.InfoContext(r.Context(), "module colours changed", "cat", "sdui", "module", mod, "by", adminName(r), "tokens", len(toks))
	}
	writeJSONTagged(w, 200, cfg.ETag, s.moduleView(r, mod, cfg))
}

// sduiThemeScopes are the colours each theme scope's nodes carry in a rendered page
// (node and document tokens): what the scope's palette in GET /api/theme modules has that
// the account's own tokens don't, so a renderer that lays a node's tokens over the
// resolved theme draws exactly that palette, derived colours and the profile's own
// included. They are the theme's first look's (the dark one of the automatic theme): a
// client following the device takes variants[scheme].modules[scope] instead.
func (s *Server) sduiThemeScopes(r *http.Request, cfg *sduistore.Config, set *sdui.ModuleSet) map[string]map[string]string {
	choices := s.themeChoicesOf(r)
	if len(sduiScopes(cfg, set)) == 0 && len(choices.Personal.Modules) == 0 {
		return map[string]map[string]string{}
	}
	t := s.resolveThemeWith(r, cfg, set, choices)
	out := make(map[string]map[string]string, len(t.Modules))
	for mod, m := range t.Modules {
		diff := map[string]string{}
		for k, v := range m.Tokens {
			if t.Tokens[k] != v {
				diff[k] = v
			}
		}
		if len(diff) > 0 {
			out[mod] = diff
		}
	}
	return out
}

// brandWarnings are the parts of the name drawn in a fixed colour (static, or a gradient's
// stops) that fall under 4.5:1 on the surfaces the name sits on in this theme. Parts that
// follow the theme are the theme's to get right.
func brandWarnings(parts []brandname.Part, tokens map[string]string) []BrandWarning {
	out := []BrandWarning{}
	for _, p := range parts {
		if p.Color == nil {
			continue
		}
		var hexes []string
		switch p.Color.Kind {
		case brandname.KindStatic:
			hexes = []string{p.Color.Hex}
		case brandname.KindGradient:
			hexes = p.Color.Stops
		}
		for _, h := range hexes {
			for _, surf := range brandSurfaces {
				if r, ok := themes.ContrastRatio(h, tokens[surf.token]); ok && r < themes.ContrastAA {
					out = append(out, BrandWarning{Text: p.Text, Surface: surf.token, Where: surf.where, Ratio: math.Round(r*100) / 100, Need: themes.ContrastAA})
				}
			}
		}
	}
	return out
}

// brandWarningsOf checks the name on the theme's look; an automatic theme has two, each
// checked and named.
func brandWarningsOf(parts []brandname.Part, t *ResolvedTheme) []BrandWarning {
	if len(t.Variants) < 2 {
		return brandWarnings(parts, t.Tokens)
	}
	out := []BrandWarning{}
	for _, name := range []string{"dark", "light"} {
		if v, ok := t.Variants[name]; ok {
			for _, w := range brandWarnings(parts, v.Tokens) {
				w.Variant = name
				out = append(out, w)
			}
		}
	}
	return out
}
