// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/sduistore"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// Layout options (docs/SDUI.md, "Layout options"): the administrator chooses which layout
// each form factor starts with and which profiles may choose; each profile chooses its
// own for each of its devices. The layout in force is sent with the navigation and every
// page as the shell an app draws its frame with, and the server makes the layout's
// changes to pages itself.

// sduiDeviceID is what an app calls itself, so a profile can have a layout per device: a
// stable name of its own, not anything that identifies the hardware.
var sduiDeviceID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// layoutDevice is the key a profile's choice is kept under: the app's own device name
// when it sends one, else the form factor, so every phone without one shares a choice.
func layoutDevice(r *http.Request, formFactor string) (string, error) {
	if d := sduiPick(r, "device", "X-WMS-Device"); d != "" {
		if !sduiDeviceID.MatchString(d) {
			return "", errors.New("device: letters, digits, - and _, at most 64")
		}
		return "device:" + d, nil
	}
	return "form:" + formFactor, nil
}

// personalLayout is the profile's choice for a device, "" for none: its choice for this
// device, else for the form factor.
func (s *Server) personalLayout(r *http.Request, formFactor string) string {
	_, ps := s.readPersonal(r)
	if key, err := layoutDevice(r, formFactor); err == nil && ps.Layouts[key] != "" {
		return ps.Layouts[key]
	}
	return ps.Layouts["form:"+formFactor]
}

// sduiLayout is the layout a request is drawn with: one the request names (an app trying
// a layout before choosing it), the profile's for the device, the administrator's for
// the form factor, or the default; only ones the administrator allows.
func (s *Server) sduiLayout(r *http.Request, cfg *sduistore.Config, formFactor string) (*sdui.LayoutSpec, error) {
	var pol sdui.LayoutPolicy
	if cfg.Layouts != nil {
		pol = *cfg.Layouts
	}
	asked := sduiPick(r, "layout", "X-WMS-Layout")
	if asked != "" && sdui.LayoutByID(asked) == nil {
		return nil, errors.New("layout: one of the layout options in the catalogue")
	}
	if _, err := layoutDevice(r, formFactor); err != nil {
		return nil, err
	}
	personal := asked
	if personal == "" && !cfg.Personalisation.Disabled && s.access(r).Can(library.FeatureEditLayout) {
		personal = s.personalLayout(r, formFactor)
	}
	return sdui.LayoutByID(sdui.ChooseLayout(personal, pol.Default[formFactor], pol.Allowed, formFactor)), nil
}

// layoutChoice is what the profile may choose on this device, for the navigation's
// answer and the layout routes; nil for a profile kept from rearranging.
func (s *Server) layoutChoice(r *http.Request, cfg *sduistore.Config, formFactor, current string) *sdui.LayoutChoice {
	if cfg.Personalisation.Disabled || !s.access(r).Can(library.FeatureEditLayout) {
		return nil
	}
	chosen := ""
	_, ps := s.readPersonal(r)
	if key, err := layoutDevice(r, formFactor); err == nil {
		chosen = ps.Layouts[key]
	}
	return &sdui.LayoutChoice{Current: current, Chosen: chosen, Options: sdui.LayoutOptions(cfg.Layouts), URL: "/api/v1/sdui/layout"}
}

func (s *Server) sduiLayoutRoutes(m *http.ServeMux) {
	m.Handle("GET /api/sdui/layout", s.requireUser(s.handleSDUILayoutGet))
	m.Handle("PUT /api/sdui/layout", s.requireUser(s.handleSDUILayoutPut))
	m.Handle("DELETE /api/sdui/layout", s.requireUser(s.handleSDUILayoutDelete))
	m.Handle("GET /api/sdui/config/layouts", s.requireAdmin(s.handleSDUIConfigLayouts))
	m.Handle("PUT /api/sdui/config/layouts", s.requireAdmin(s.handleSDUIConfigLayoutsPut))
}

// layoutRequest is the device a layout request is about.
func (s *Server) layoutRequest(w http.ResponseWriter, r *http.Request) (*sduistore.Config, string, bool) {
	cfg, _, err := s.sduiConfig(r)
	if err != nil {
		code := 500
		if errors.Is(err, errPreview) {
			code = 403
		}
		writeErr(w, code, err.Error())
		return nil, "", false
	}
	ctx := &sdui.RenderContext{}
	if err := s.sduiDevice(r, ctx); err != nil {
		writeErr(w, 400, err.Error())
		return nil, "", false
	}
	return cfg, ctx.Profile.FormFactor, true
}

// GET /api/sdui/layout: the layout in force on this device, the profile's own choice and
// what it may choose from, with the shell it makes.
func (s *Server) handleSDUILayoutGet(w http.ResponseWriter, r *http.Request) {
	cfg, ff, ok := s.layoutRequest(w, r)
	if !ok {
		return
	}
	l, err := s.sduiLayout(r, cfg, ff)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	choice := s.layoutChoice(r, cfg, ff, l.ID)
	writeJSON(w, 200, map[string]any{"layout": l.ID, "shell": sdui.ShellFor(l.ID, ff), "choice": choice, "editable": choice != nil})
}

// PUT /api/sdui/layout {layout}: the profile's layout for this device (device=, else the
// form factor).
func (s *Server) handleSDUILayoutPut(w http.ResponseWriter, r *http.Request) {
	if !s.mayPersonalise(w, r) {
		return
	}
	cfg, ff, ok := s.layoutRequest(w, r)
	if !ok {
		return
	}
	var body struct {
		Layout string `json:"layout"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeErr(w, 400, `expected {"layout": "<id>"}`)
		return
	}
	if sdui.LayoutByID(body.Layout) == nil {
		writeErr(w, 400, "layout: one of the layout options in the catalogue")
		return
	}
	if p := cfg.Layouts; p != nil && len(p.Allowed) > 0 && !containsString(p.Allowed, body.Layout) {
		writeErr(w, 403, "This server doesn't offer that layout. An administrator chooses which it offers.")
		return
	}
	if err := s.writeLayout(r, ff, body.Layout); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.handleSDUILayoutGet(w, r)
}

// DELETE /api/sdui/layout: this device follows the server's layout again.
func (s *Server) handleSDUILayoutDelete(w http.ResponseWriter, r *http.Request) {
	if !s.mayPersonalise(w, r) {
		return
	}
	_, ff, ok := s.layoutRequest(w, r)
	if !ok {
		return
	}
	if err := s.writeLayout(r, ff, ""); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.handleSDUILayoutGet(w, r)
}

// maxLayoutDevices bounds the devices one profile keeps a layout for.
const maxLayoutDevices = 32

func (s *Server) writeLayout(r *http.Request, formFactor, layout string) error {
	key, err := layoutDevice(r, formFactor)
	if err != nil {
		return err
	}
	s.sdui.personalMu.Lock()
	defer s.sdui.personalMu.Unlock()
	ui, ps := s.readPersonal(r)
	if ps.Layouts == nil {
		ps.Layouts = map[string]string{}
	}
	if layout == "" {
		delete(ps.Layouts, key)
	} else {
		if len(ps.Layouts) >= maxLayoutDevices && ps.Layouts[key] == "" {
			return errors.New("this profile has chosen layouts for too many devices: reset one first")
		}
		ps.Layouts[key] = layout
	}
	if len(ps.Layouts) == 0 {
		ps.Layouts = nil
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

// GET /api/sdui/config/layouts[?target=draft] (administrators): the server's layouts, the
// options there are, and the defaults.
func (s *Server) handleSDUIConfigLayouts(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	cfg, _, err := s.sduiBase(r, st)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	pol := sdui.LayoutPolicy{}
	if cfg.Layouts != nil {
		pol = *cfg.Layouts
	}
	defaults := map[string]string{}
	for _, ff := range sdui.FormFactors {
		defaults[ff] = sdui.DefaultLayout(ff)
	}
	writeJSON(w, 200, map[string]any{"etag": cfg.ETag, "layouts": pol, "options": sdui.Layouts, "builtin": defaults})
}

// PUT /api/sdui/config/layouts[?target=draft] (administrators): save them. If-Match as for
// the configuration.
func (s *Server) handleSDUIConfigLayoutsPut(w http.ResponseWriter, r *http.Request) {
	b, err := sduiBody(r, 16<<10)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var pol sdui.LayoutPolicy
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&pol); err != nil {
		writeErr(w, 400, `expected {"default": {"<form factor>": "<layout>"}, "allowed": ["<layout>", ...]}`)
		return
	}
	s.sduiMutate(w, r, "layouts saved", func(c *sduistore.Config) error {
		if len(pol.Default) == 0 && len(pol.Allowed) == 0 {
			c.Layouts = nil
		} else {
			c.Layouts = &pol
		}
		return nil
	})
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
