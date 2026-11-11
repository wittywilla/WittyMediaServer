// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/licenses"
)

// The modules on this server, with the version of each in use, for Settings, About.

type moduleInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Role    string `json:"role"`
}

func (s *Server) modulesInUse() []moduleInfo {
	out := []moduleInfo{}
	if s.Modules == nil {
		return out
	}
	for _, m := range s.Modules.Report().Modules {
		if m.Enabled && m.Delivery != "builtin" && m.Version != "" {
			out = append(out, moduleInfo{Name: m.Name, Version: m.Version, Role: m.Summary})
		}
	}
	return out
}

// GET /api/licenses: everything in this server that someone else made, with its license.
func (s *Server) handleLicenses(w http.ResponseWriter, r *http.Request) {
	all := licenses.All()
	out := make([]licenses.Component, len(all))
	copy(out, all)
	// The versions really in this program, which a list made earlier can't know.
	versions := map[string]string{}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			versions[d.Path] = strings.TrimPrefix(d.Version, "v")
		}
	}
	for i := range out {
		out[i].Version = versions[out[i].Name]
	}
	writeJSON(w, 200, map[string]any{"groups": licenses.Groups, "components": out})
}

// GET /api/licenses/{id}: one component's full license text.
func (s *Server) handleLicenseText(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	text, ok := licenses.Text(id)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, map[string]any{"text": text})
}

// GET /api/about: the server's version and its engines (any signed-in user).
func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"version": s.Version, "modules": s.modulesInUse()})
}
