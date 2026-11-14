// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http"
	"sort"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// A module's secrets (module API 5): keys and passwords an administrator gives a module, such as
// a game metadata source's. The server keeps them (module.<id>.secret.<name>) and gives each only
// to its own module, through the host API's GET /secrets/{name}. A value is never sent back to
// a browser, nor written to the log or the audit log: only that it was set or removed.

// maxSecret bounds one secret's value.
const maxSecret = 4096

// The routes are a mux of their own, asked by /api/modules/{id}/{path...} before a module's
// routes ("secrets" is one of modules.ReservedPaths, so no module's route starts with it): in
// the server's mux they would overlap PUT /api/modules/libraries/{type}/label.
func (s *Server) moduleSecretRoutes() *http.ServeMux {
	m := http.NewServeMux()
	m.Handle("GET /api/modules/{id}/secrets", s.requireAdmin(s.handleModuleSecrets))
	m.Handle("PUT /api/modules/{id}/secrets/{name}", s.requireAdmin(s.handleModuleSecretPut))
	return m
}

// serveModuleSecrets answers a request for a module's secrets, and says whether it was one.
func (s *Server) serveModuleSecrets(w http.ResponseWriter, r *http.Request) bool {
	rest := r.PathValue("path")
	if rest != "secrets" && !strings.HasPrefix(rest, "secrets/") {
		return false
	}
	if m := s.moduleSecretRoutes(); func() bool { _, p := m.Handler(r); return p != "" }() {
		m.ServeHTTP(w, r) // which gives the handler the path's names
		return true
	}
	s.requireAdmin(func(w http.ResponseWriter, _ *http.Request) { writeErr(w, 404, "not found") }).ServeHTTP(w, r)
	return true
}

// knownModule is a module of this server's, by the ID in the path; false (and 404) otherwise.
func (s *Server) knownModule(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !validSecretName(id) || s.Modules == nil || !s.Modules.Known(id) {
		writeErr(w, 404, "no such module")
		return "", false
	}
	return id, true
}

// GET /api/modules/{id}/secrets -> {"set": [names]}: which secrets the module has, never what
// they are.
func (s *Server) handleModuleSecrets(w http.ResponseWriter, r *http.Request) {
	id, ok := s.knownModule(w, r)
	if !ok {
		return
	}
	prefix := secretKey(id, "")
	rows, err := s.DB.QueryContext(r.Context(), `SELECT key FROM settings WHERE key LIKE ? ESCAPE '\' AND value <> ''`, likeEscape(prefix)+"%")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	set := []string{}
	for rows.Next() {
		var k string
		if rows.Scan(&k) == nil {
			if name, ok := strings.CutPrefix(k, prefix); ok && validSecretName(name) {
				set = append(set, name)
			}
		}
	}
	sort.Strings(set)
	writeJSON(w, 200, map[string]any{"set": set})
}

// PUT /api/modules/{id}/secrets/{name} {value}: gives the module a secret; "" removes it.
func (s *Server) handleModuleSecretPut(w http.ResponseWriter, r *http.Request) {
	id, ok := s.knownModule(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !validSecretName(name) {
		writeErr(w, 400, "a secret's name is letters, digits, '-', '_' and '.', up to 64")
		return
	}
	var b struct {
		Value *string `json:"value"`
	}
	if err := readJSON(r, &b); err != nil || b.Value == nil {
		writeErr(w, 400, "say the secret's value, or \"\" to remove it")
		return
	}
	v := strings.TrimSpace(*b.Value)
	if len(v) > maxSecret {
		writeErr(w, 400, "that secret is too long")
		return
	}
	key := secretKey(id, name)
	action := "secret set"
	if v == "" {
		action = "secret removed"
		if _, err := s.DB.ExecContext(r.Context(), `DELETE FROM settings WHERE key = ?`, key); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else {
		logging.AddSecret(v) // hidden from the log from now on
		if err := s.DB.SetSetting(r.Context(), key, v); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	s.writeAudit(r.Context(), userFrom(r), id+" "+action, id, action, "", name)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// likeEscape makes s match only itself in a LIKE with ESCAPE '\'.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
