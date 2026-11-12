// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"net/http"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/sduistore"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// Placements (docs/SDUI.md, "Placements"): where each region of the shell is. The
// experience sets them, the administrator changes them for everyone (the policy's
// placements, PUT /sdui/config/experience) and each profile for its own device (these
// routes). The server merges them and sends the result in shell.placements.

// placementsFor is the administrator's and the profile's changes to an experience's
// regions for this device, in that order, ready to lay over the experience's own.
func (s *Server) placementsFor(r *http.Request, cfg *sduistore.Config, exp *sdui.Experience, ff string) sdui.RegionPlacements {
	var layers []sdui.RegionPlacements
	if cfg.Experiences != nil {
		layers = append(layers, cfg.Experiences.Placements[exp.ID])
	}
	if !cfg.Personalisation.Disabled && s.access(r).Can(library.FeatureEditLayout) {
		if key, err := experienceDevice(r, ff); err == nil {
			_, ps := s.readPersonal(r)
			layers = append(layers, ps.Placements[key])
		}
	}
	if len(layers) == 0 {
		return nil
	}
	return sdui.MergePlacements(layers...)
}

// shellFor is an experience's shell on a device with the placements laid over it.
func (s *Server) shellFor(r *http.Request, cfg *sduistore.Config, exp *sdui.Experience, ff string) *sdui.Shell {
	sh := sdui.ShellFor(exp.ID, ff)
	if p := s.placementsFor(r, cfg, exp, ff); p != nil {
		sh.Placements = sdui.MergePlacements(sh.Placements, p)
	}
	return sh
}

func (s *Server) sduiPlacementRoutes(m *http.ServeMux) {
	m.Handle("GET /api/sdui/placements", s.requireUser(s.handleSDUIPlacements))
	m.Handle("PUT /api/sdui/placements", s.requireUser(s.handleSDUIPlacementsPut))
	m.Handle("DELETE /api/sdui/placements", s.requireUser(s.handleSDUIPlacementsDelete))
}

// GET /api/sdui/placements: the regions, the placements in force on this device (merged),
// the profile's own and whether it may change them.
func (s *Server) handleSDUIPlacements(w http.ResponseWriter, r *http.Request) {
	cfg, ff, ok := s.experienceRequest(w, r)
	if !ok {
		return
	}
	exp, err := s.sduiExperience(r, cfg, ff)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var own sdui.RegionPlacements
	if key, err := experienceDevice(r, ff); err == nil {
		_, ps := s.readPersonal(r)
		own = ps.Placements[key]
	}
	editable := !cfg.Personalisation.Disabled && s.access(r).Can(library.FeatureEditLayout)
	writeJSON(w, 200, map[string]any{"experience": exp.ID, "regions": sdui.Regions, "placements": s.shellFor(r, cfg, exp, ff).Placements, "own": own, "editable": editable})
}

// PUT /api/sdui/placements {placements: {<region>: {position, visible, style}}}: this
// profile's placements for this device, replacing its earlier ones.
func (s *Server) handleSDUIPlacementsPut(w http.ResponseWriter, r *http.Request) {
	if !s.mayPersonalise(w, r) {
		return
	}
	_, ff, ok := s.experienceRequest(w, r)
	if !ok {
		return
	}
	var body struct {
		Placements sdui.RegionPlacements `json:"placements"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, `expected {"placements": {"<region>": {"position": "...", "visible": true}}}`)
		return
	}
	if issues := body.Placements.Validate(""); len(issues) > 0 {
		writeJSON(w, 400, map[string]any{"error": issues[0].Message, "issues": issues})
		return
	}
	if err := s.writePlacements(r, ff, body.Placements); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.handleSDUIPlacements(w, r)
}

// DELETE /api/sdui/placements: back to the experience's (and the administrator's).
func (s *Server) handleSDUIPlacementsDelete(w http.ResponseWriter, r *http.Request) {
	if !s.mayPersonalise(w, r) {
		return
	}
	_, ff, ok := s.experienceRequest(w, r)
	if !ok {
		return
	}
	if err := s.writePlacements(r, ff, nil); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.handleSDUIPlacements(w, r)
}

func (s *Server) writePlacements(r *http.Request, ff string, p sdui.RegionPlacements) error {
	key, err := experienceDevice(r, ff)
	if err != nil {
		return err
	}
	s.sdui.personalMu.Lock()
	defer s.sdui.personalMu.Unlock()
	ui, ps := s.readPersonal(r)
	if ps.Placements == nil {
		ps.Placements = map[string]sdui.RegionPlacements{}
	}
	if len(p) == 0 {
		delete(ps.Placements, key)
	} else {
		if len(ps.Placements) >= maxLayoutDevices && ps.Placements[key] == nil {
			return errTooManyDevices
		}
		ps.Placements[key] = p
	}
	if len(ps.Placements) == 0 {
		ps.Placements = nil
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
	uid := userFrom(r).ID
	if err := s.Auth.SetUI(r.Context(), uid, string(b)); err != nil {
		return err
	}
	s.forgetRecent(uid)
	return nil
}
