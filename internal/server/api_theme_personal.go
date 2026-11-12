// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/sduistore"
)

// A profile's own colours: overrides it sets for itself over whichever theme it sees, and
// over each module scope, kept with its other interface choices (the "ui" of its account)
// under "themePersonal". Only its own: the routes take no profile, and read and write the
// signed-in one's. They go when the profile does.
//
// Unlike the administrator's overrides, which are saved with a warning when text becomes
// hard to read, a profile's own are refused then: it is choosing for itself, and nobody
// else will see the warning and put it right.

// themePersonal is what is kept.
type themePersonal struct {
	Tokens  map[string]string            `json:"tokens,omitempty"`
	Modules map[string]map[string]string `json:"modules,omitempty"`
}

const (
	themePersonalKey = "themePersonal"
	// maxPersonalScopes bounds the module scopes one profile colours.
	maxPersonalScopes = 16
	// maxPersonalBody bounds the request: 16 scopes of 64 tokens fit.
	maxPersonalBody = 32 << 10
)

// themePersonalView is what the routes answer.
type themePersonalView struct {
	// Tokens are the profile's own colours, Modules its own for each module scope.
	Tokens  map[string]string            `json:"tokens"`
	Modules map[string]map[string]string `json:"modules"`
	// Warnings are the profile's colours that are hard to read on the theme it sees now
	// (one saved for another theme can be, after the theme changes).
	Warnings []ThemeWarning `json:"warnings"`
	// ETag is the resolved theme's (GET /api/theme), with these colours in it.
	ETag string `json:"etag"`
}

func personalView(p themePersonal, t *ResolvedTheme) themePersonalView {
	v := themePersonalView{Tokens: overlay(nil, p.Tokens), Modules: map[string]map[string]string{}, Warnings: personalWarnings(t), ETag: t.ETag}
	for k, m := range p.Modules {
		v.Modules[k] = overlay(nil, m)
	}
	return v
}

// personalWarnings are the warnings about the profile's own colours.
func personalWarnings(t *ResolvedTheme) []ThemeWarning {
	out := []ThemeWarning{}
	for _, w := range t.Warnings {
		if w.Scope == "personal" || strings.HasPrefix(w.Scope, "personal:") {
			out = append(out, w)
		}
	}
	return out
}

// GET /api/theme/personal: the profile's own colours.
func (s *Server) handleThemePersonal(w http.ResponseWriter, r *http.Request) {
	t, err := s.resolveTheme(r)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSONTagged(w, 200, t.ETag, personalView(s.themeChoicesOf(r).Personal, t))
}

// readThemePersonal reads and checks a request: strict JSON, known colour tokens and real
// colours only, module scopes by form.
func readThemePersonal(r *http.Request) (themePersonal, error) {
	var b struct {
		Tokens  map[string]string            `json:"tokens"`
		Modules map[string]map[string]string `json:"modules"`
	}
	body, err := sduiBody(r, maxPersonalBody)
	if err == nil {
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.DisallowUnknownFields()
		err = dec.Decode(&b)
	}
	if err != nil {
		return themePersonal{}, errors.New(`expected {"tokens": {"<token>": "<colour>"}, "modules": {"<scope>": {"<token>": "<colour>"}}}: ` + err.Error())
	}
	var p themePersonal
	if p.Tokens, err = checkTokens(b.Tokens); err != nil {
		return p, err
	}
	if len(b.Modules) > maxPersonalScopes {
		return p, errors.New("too many module scopes: at most 16")
	}
	for _, scope := range sortedStringKeys(b.Modules) {
		if !sduistore.ValidModuleID(scope) {
			return p, errors.New("modules: " + scope + ": a scope is a module id, lower-case letters, digits, - and _")
		}
		toks, err := checkTokens(b.Modules[scope])
		if err != nil {
			return p, errors.New("modules: " + scope + ": " + err.Error())
		}
		if len(toks) == 0 {
			continue
		}
		if p.Modules == nil {
			p.Modules = map[string]map[string]string{}
		}
		p.Modules[scope] = toks
	}
	if len(p.Tokens) == 0 {
		p.Tokens = nil
	}
	return p, nil
}

// PUT /api/theme/personal {tokens, modules}: replace the profile's own colours. Refused
// with the pairs that fail when any text would fall below WCAG AA (4.5:1) on the theme it
// sees, in any of its looks.
func (s *Server) handleThemePersonalPut(w http.ResponseWriter, r *http.Request) {
	p, err := readThemePersonal(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	choices := s.themeChoicesOf(r)
	choices.Personal = p
	t := s.resolveThemeWith(r, st.Current(), s.moduleSet(nil), choices)
	if bad := personalWarnings(t); len(bad) > 0 {
		writeJSON(w, 400, map[string]any{
			"error":    "Those colours would make text hard to read: " + describeIssues(bad) + ". Text needs a contrast of at least 4.5:1 with what it sits on.",
			"contrast": bad,
		})
		return
	}
	if err := s.writeThemePersonal(r, p); err != nil {
		s.Log.InfoContext(r.Context(), "personal colours not saved", "cat", "sdui", "profile", adminName(r), "err", err)
		writeErr(w, 400, err.Error())
		return
	}
	s.Log.DebugContext(r.Context(), "personal colours saved", "cat", "sdui", "profile", adminName(r), "tokens", len(p.Tokens), "modules", len(p.Modules))
	writeJSONTagged(w, 200, t.ETag, personalView(p, t))
}

// DELETE /api/theme/personal: back to the theme's colours.
func (s *Server) handleThemePersonalDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.writeThemePersonal(r, themePersonal{}); err != nil {
		s.Log.ErrorContext(r.Context(), "could not reset personal colours", "cat", "sdui", "profile", adminName(r), "err", err)
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.DebugContext(r.Context(), "personal colours reset", "cat", "sdui", "profile", adminName(r))
	t, err := s.resolveTheme(r)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSONTagged(w, 200, t.ETag, personalView(themePersonal{}, t))
}

func describeIssues(ws []ThemeWarning) string {
	seen := map[string]bool{}
	var out []string
	for _, w := range ws {
		d := w.Text + " on " + w.Surface
		if !seen[d] && len(out) < 6 {
			seen[d] = true
			out = append(out, d)
		}
	}
	return strings.Join(out, ", ")
}

// writeThemePersonal keeps the profile's colours with its other interface choices,
// leaving those as they are.
func (s *Server) writeThemePersonal(r *http.Request, p themePersonal) error {
	s.sdui.personalMu.Lock()
	defer s.sdui.personalMu.Unlock()
	uid := userFrom(r).ID
	ui := map[string]json.RawMessage{}
	_ = json.Unmarshal([]byte(s.Auth.UI(r.Context(), uid)), &ui)
	if len(p.Tokens) == 0 && len(p.Modules) == 0 {
		delete(ui, themePersonalKey)
	} else {
		raw, err := json.Marshal(p)
		if err != nil {
			return err
		}
		ui[themePersonalKey] = raw
	}
	b, err := json.Marshal(ui)
	if err != nil {
		return err
	}
	if len(b) > 64<<10 {
		return errors.New("this profile's interface choices are too large")
	}
	s.forgetRecent(uid)
	return s.Auth.SetUI(r.Context(), uid, string(b))
}
