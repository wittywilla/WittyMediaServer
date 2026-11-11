// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/sduistore"
	"github.com/wittywilla/wittymediaserver/internal/themes"
)

// The Master Theme Engine: one resolved theme per account, from
//
//  1. the server's default theme (the "theme" setting, as /api/themes reports it) with the
//     administrator's global colour overrides, kept in ui_config.json;
//  2. the account's own choice, the "theme" key of /api/me/ui, which replaces the theme and
//     with it the global overrides (they were chosen for the server's theme);
//  3. per-module overrides from ui_config.json, layered over either, for the parts of the
//     interface that belong to a module. The same overrides reach server-driven screens
//     through sdui.Render.
//
// /api/themes and /api/themes/{id}/tokens.json are unchanged; this is a layer over them.

func (s *Server) themeRoutes(m *http.ServeMux) {
	m.Handle("GET /api/theme", s.requireUser(s.handleTheme))
	m.Handle("PUT /api/theme/global", s.requireAdmin(s.handleThemeGlobalPut))
	m.Handle("GET /api/theme/modules/{module}", s.requireAdmin(s.handleThemeModule))
	m.Handle("PUT /api/theme/modules/{module}", s.requireAdmin(s.handleThemeModulePut))
	m.Handle("DELETE /api/theme/modules/{module}", s.requireAdmin(s.handleThemeModuleDelete))
}

// ThemeWarning is text that would be hard to read: WCAG AA wants 4.5:1.
type ThemeWarning struct {
	// Scope is "global" or "module:<id>".
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
	// Tokens are its colours by token name, with the overrides applied.
	Tokens map[string]string `json:"tokens"`
	// Overrides are the global overrides applied (none when the account chose its own theme).
	Overrides map[string]string `json:"overrides"`
	// Modules are the colours for each module with overrides: the whole palette, with the
	// module's own colours over Tokens.
	Modules map[string]ThemeModule `json:"modules"`
	// Scheme is "dark" or "light": which the theme is. "auto" for the automatic theme,
	// which is dark or light as the device is; Tokens and Modules are then its dark look
	// and Variants has both.
	Scheme string `json:"scheme"`
	// Variants are the theme's looks by scheme, each resolved the same way: one entry for
	// an ordinary theme, "dark" and "light" for the automatic one. A client that follows
	// the device's setting picks from here.
	Variants map[string]ThemeVariant `json:"variants"`
	Warnings []ThemeWarning          `json:"warnings"`
	ETag     string                 `json:"etag"`
	// Revision is ui_config.json's revision the overrides come from.
	Revision int64 `json:"revision"`
}

// ThemeVariant is one look of a theme: the built-in theme behind it, its colours with the
// global overrides applied, and each module's whole palette.
type ThemeVariant struct {
	ID      string                       `json:"id"`
	Tokens  map[string]string            `json:"tokens"`
	Modules map[string]map[string]string `json:"modules"`
}

// The built-in themes the automatic theme is, by the device's setting.
const (
	autoDark  = "default"
	autoLight = "light"
)

// themeScheme is "dark" or "light" for a theme, from its own theme.json.
func (s *Server) themeScheme(id string) string {
	if s.Themes != nil {
		for _, m := range s.Themes.List() {
			if m.ID == id && (m.Scheme == "light" || m.Scheme == "dark") {
				return m.Scheme
			}
		}
	}
	if id == autoLight {
		return "light"
	}
	return "dark"
}

// ThemeModule is one module's colours.
type ThemeModule struct {
	Tokens    map[string]string `json:"tokens"`
	Overrides map[string]string `json:"overrides"`
}

// userThemeChoice is the theme the account picked for itself, if it is here.
func (s *Server) userThemeChoice(r *http.Request) string {
	raw := s.Auth.UI(r.Context(), userFrom(r).ID)
	if raw == "" {
		return ""
	}
	var ui struct {
		Theme any `json:"theme"`
	}
	if json.Unmarshal([]byte(raw), &ui) != nil {
		return ""
	}
	id, _ := ui.Theme.(string)
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" || id == "auto" || !s.themeKnown(id) {
		return ""
	}
	return id
}

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

// resolveTheme is the theme for this request.
func (s *Server) resolveTheme(r *http.Request) (*ResolvedTheme, error) {
	st, err := s.uiStore()
	if err != nil {
		return nil, err
	}
	cfg := st.Current()
	t := &ResolvedTheme{ID: s.serverTheme(r.Context()), Source: "server", Modules: map[string]ThemeModule{},
		Overrides: map[string]string{}, Warnings: []ThemeWarning{}, Revision: cfg.Revision}
	if own := s.userThemeChoice(r); own != "" {
		t.ID, t.Source = own, "user"
	} else {
		t.Overrides = overlay(nil, cfg.Theme.Tokens)
	}
	// Each look is resolved the same way: the theme's colours, the global overrides, then
	// each module's own over those. An override that reads well on the dark look can be
	// unreadable on the light one, so each look gets its own warnings.
	looks := []struct{ scheme, id string }{{s.themeScheme(t.ID), t.ID}}
	t.Scheme = looks[0].scheme
	if t.ID == "auto" {
		looks = []struct{ scheme, id string }{{"dark", autoDark}, {"light", autoLight}}
		t.Scheme = "auto"
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
		v := ThemeVariant{ID: look.id, Tokens: overlay(s.baseColors(look.id), t.Overrides), Modules: map[string]map[string]string{}}
		if len(t.Overrides) > 0 {
			warn("global", v.Tokens, t.Overrides)
		}
		for _, mod := range sortedStringKeys(cfg.Theme.Modules) {
			ov := cfg.Theme.Modules[mod]
			v.Modules[mod] = overlay(v.Tokens, ov)
			warn("module:"+mod, v.Modules[mod], ov)
			if i == 0 {
				t.Modules[mod] = ThemeModule{Tokens: v.Modules[mod], Overrides: overlay(nil, ov)}
			}
		}
		t.Variants[look.scheme] = v
		if i == 0 {
			t.Tokens = v.Tokens
		}
	}
	t.ETag = themeETag(t)
	return t, nil
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
			writeUIStoreErr(w, err)
			return
		}
	}
	if b.ID != nil {
		if err := s.DB.SetSetting(r.Context(), "theme", id); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	s.Log.InfoContext(r.Context(), "server theme changed", "cat", "ui", "by", adminName(r))
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
	base := s.baseColors(s.serverTheme(r.Context()))
	v.Warnings = append(v.Warnings, contrastWarnings("module:"+mod, overlay(overlay(base, cfg.Theme.Tokens), ov), ov)...)
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
		writeUIStoreErr(w, err)
		return
	}
	s.Log.InfoContext(r.Context(), "module colours changed", "cat", "ui", "module", mod, "by", adminName(r))
	writeJSONTagged(w, 200, cfg.ETag, s.moduleView(r, mod, cfg))
}
