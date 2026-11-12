// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/sduistore"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// Experiences (docs/SDUI.md, "Experiences"): the administrator chooses which look
// each form factor starts with and which profiles may choose; each profile chooses its
// own for each of its devices. The layout in force is sent with the navigation and every
// page as the shell an app draws its frame with, and the server makes the layout's
// changes to pages itself.

// sduiDeviceID is what an app calls itself, so a profile can have a layout per device: a
// stable name of its own, not anything that identifies the hardware.
var sduiDeviceID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// experienceDevice is the key a profile's choice is kept under: the app's own device name
// when it sends one, else the form factor, so every phone without one shares a choice.
func experienceDevice(r *http.Request, formFactor string) (string, error) {
	if d := sduiPick(r, "device", "X-WMS-Device"); d != "" {
		if !sduiDeviceID.MatchString(d) {
			return "", errors.New("device: letters, digits, - and _, at most 64")
		}
		return "device:" + d, nil
	}
	return "form:" + formFactor, nil
}

// personalExperience is the profile's choice for a device, "" for none: its choice for this
// device, else for the form factor.
func (s *Server) personalExperience(r *http.Request, formFactor string) string {
	_, ps := s.readPersonal(r)
	if key, err := experienceDevice(r, formFactor); err == nil && ps.Experiences[key] != "" {
		return ps.Experiences[key]
	}
	return ps.Experiences["form:"+formFactor]
}

// sduiExperience is the layout a request is drawn with: one the request names (an app trying
// a layout before choosing it), the profile's for the device, the administrator's for
// the form factor, or the default; only ones the administrator allows.
func (s *Server) sduiExperience(r *http.Request, cfg *sduistore.Config, formFactor string) (*sdui.Experience, error) {
	var pol sdui.ExperiencePolicy
	if cfg.Experiences != nil {
		pol = *cfg.Experiences
	}
	asked := sduiPick(r, "experience", "X-WMS-Experience")
	if asked != "" && sdui.ExperienceByID(asked) == nil {
		return nil, errors.New("experience: one of the experiences in the catalogue")
	}
	if _, err := experienceDevice(r, formFactor); err != nil {
		return nil, err
	}
	personal := asked
	if personal == "" && !cfg.Personalisation.Disabled && s.access(r).Can(library.FeatureEditLayout) {
		personal = s.personalExperience(r, formFactor)
	}
	required, _ := s.requiredLook(r, cfg)
	req := ""
	if required != nil {
		req = required.Value
	}
	return sdui.ExperienceByID(sdui.ChooseExperience(req, personal, pol.Default[formFactor], pol.Allowed, formFactor)), nil
}

// requiredLook is what an administrator requires of the profile asking: its experience and
// its theme, nil for a part nobody requires. A required theme that isn't installed any more
// is not required.
func (s *Server) requiredLook(r *http.Request, cfg *sduistore.Config) (experience, theme *sdui.RequiredPart) {
	u := userFrom(r)
	if u == nil || cfg.Experiences == nil {
		return nil, nil
	}
	experience, theme = cfg.Experiences.Required.Resolve(strconv.FormatInt(u.ID, 10), u.Kind == "kids")
	if theme != nil && (s.Themes == nil || !s.Themes.Has(theme.Value)) {
		theme = nil
	}
	return experience, theme
}

// experienceChoice is what the profile may choose on this device, for the navigation's
// answer and the layout routes; nil for a profile kept from rearranging.
func (s *Server) experienceChoice(r *http.Request, cfg *sduistore.Config, formFactor, current string) *sdui.ExperienceChoice {
	if req, _ := s.requiredLook(r, cfg); req != nil {
		return &sdui.ExperienceChoice{Current: current, Options: []sdui.EnumOption{}, URL: "/api/v1/sdui/experience", Required: req}
	}
	if cfg.Personalisation.Disabled || !s.access(r).Can(library.FeatureEditLayout) {
		return nil
	}
	chosen := ""
	_, ps := s.readPersonal(r)
	if key, err := experienceDevice(r, formFactor); err == nil {
		chosen = ps.Experiences[key]
	}
	return &sdui.ExperienceChoice{Current: current, Chosen: chosen, Options: sdui.ExperienceOptions(cfg.Experiences), URL: "/api/v1/sdui/experience"}
}

func (s *Server) sduiExperienceRoutes(m *http.ServeMux) {
	s.sduiPlacementRoutes(m)
	m.Handle("GET /api/sdui/experiences", s.requireUser(s.handleSDUIExperiences))
	m.Handle("GET /api/sdui/experience", s.requireUser(s.handleSDUIExperienceGet))
	m.Handle("PUT /api/sdui/experience", s.requireUser(s.handleSDUIExperiencePut))
	m.Handle("DELETE /api/sdui/experience", s.requireUser(s.handleSDUIExperienceDelete))
	m.Handle("GET /api/sdui/config/experience", s.requireAdmin(s.handleSDUIConfigExperience))
	m.Handle("PUT /api/sdui/config/experience", s.requireAdmin(s.handleSDUIConfigExperiencePut))
}

// experienceRequest is the device a layout request is about.
func (s *Server) experienceRequest(w http.ResponseWriter, r *http.Request) (*sduistore.Config, string, bool) {
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

// GET /api/sdui/experience: the layout in force on this device, the profile's own choice and
// what it may choose from, with the shell it makes.
func (s *Server) handleSDUIExperienceGet(w http.ResponseWriter, r *http.Request) {
	cfg, ff, ok := s.experienceRequest(w, r)
	if !ok {
		return
	}
	l, err := s.sduiExperience(r, cfg, ff)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	choice := s.experienceChoice(r, cfg, ff, l.ID)
	writeJSON(w, 200, map[string]any{"experience": l.ID, "shell": s.shellFor(r, cfg, l, ff), "choice": choice, "editable": choice != nil && choice.Required == nil})
}

// PUT /api/sdui/experience {layout}: the profile's layout for this device (device=, else the
// form factor).
func (s *Server) handleSDUIExperiencePut(w http.ResponseWriter, r *http.Request) {
	if !s.mayPersonalise(w, r) {
		return
	}
	cfg, ff, ok := s.experienceRequest(w, r)
	if !ok {
		return
	}
	if req, _ := s.requiredLook(r, cfg); req != nil {
		writeErr(w, 403, "The look is required"+byAdmin(req)+".")
		return
	}
	var body struct {
		Experience string `json:"experience"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeErr(w, 400, `expected {"experience": "<id>"}`)
		return
	}
	if sdui.ExperienceByID(body.Experience) == nil {
		writeErr(w, 400, "experience: one of the experiences in the catalogue")
		return
	}
	if p := cfg.Experiences; p != nil && len(p.Allowed) > 0 && !containsString(p.Allowed, body.Experience) {
		writeErr(w, 403, "This server doesn't offer that look. An administrator chooses which it offers.")
		return
	}
	if err := s.writeExperience(r, ff, body.Experience); err != nil {
		s.Log.InfoContext(r.Context(), "experience choice not saved", "cat", "sdui", "profile", adminName(r), "formFactor", ff, "err", err)
		writeErr(w, 400, err.Error())
		return
	}
	s.Log.DebugContext(r.Context(), "experience chosen", "cat", "sdui", "profile", adminName(r), "formFactor", ff, "experience", body.Experience)
	s.handleSDUIExperienceGet(w, r)
}

// DELETE /api/sdui/experience: this device follows the server's layout again.
func (s *Server) handleSDUIExperienceDelete(w http.ResponseWriter, r *http.Request) {
	if !s.mayPersonalise(w, r) {
		return
	}
	_, ff, ok := s.experienceRequest(w, r)
	if !ok {
		return
	}
	if err := s.writeExperience(r, ff, ""); err != nil {
		s.Log.InfoContext(r.Context(), "experience choice not reset", "cat", "sdui", "profile", adminName(r), "formFactor", ff, "err", err)
		writeErr(w, 400, err.Error())
		return
	}
	s.Log.DebugContext(r.Context(), "experience choice reset", "cat", "sdui", "profile", adminName(r), "formFactor", ff)
	s.handleSDUIExperienceGet(w, r)
}

// maxLayoutDevices bounds the devices one profile keeps a layout for.
const maxLayoutDevices = 32

func (s *Server) writeExperience(r *http.Request, formFactor, layout string) error {
	key, err := experienceDevice(r, formFactor)
	if err != nil {
		return err
	}
	s.sdui.personalMu.Lock()
	defer s.sdui.personalMu.Unlock()
	ui, ps := s.readPersonal(r)
	if ps.Experiences == nil {
		ps.Experiences = map[string]string{}
	}
	if layout == "" {
		delete(ps.Experiences, key)
	} else {
		if len(ps.Experiences) >= maxLayoutDevices && ps.Experiences[key] == "" {
			return errors.New("this profile has chosen looks for too many devices: reset one first")
		}
		ps.Experiences[key] = layout
	}
	if len(ps.Experiences) == 0 {
		ps.Experiences = nil
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

// GET /api/sdui/config/experience[?target=draft] (administrators): the server's layouts, the
// options there are, and the defaults.
func (s *Server) handleSDUIConfigExperience(w http.ResponseWriter, r *http.Request) {
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
	pol := sdui.ExperiencePolicy{}
	if cfg.Experiences != nil {
		pol = *cfg.Experiences
	}
	defaults := map[string]string{}
	for _, ff := range sdui.FormFactors {
		defaults[ff] = sdui.DefaultExperience(ff)
	}
	writeJSON(w, 200, map[string]any{"etag": cfg.ETag, "experiences": pol, "options": sdui.Experiences, "builtin": defaults})
}

// PUT /api/sdui/config/experience[?target=draft] (administrators): save them. If-Match as for
// the configuration.
func (s *Server) handleSDUIConfigExperiencePut(w http.ResponseWriter, r *http.Request) {
	b, err := sduiBody(r, 16<<10)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var pol sdui.ExperiencePolicy
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&pol); err != nil {
		writeErr(w, 400, `expected {"default": {"<form factor>": "<layout>"}, "allowed": ["<layout>", ...]}`)
		return
	}
	for _, id := range pol.Required.RequiredThemes() {
		if s.Themes == nil || !s.Themes.Has(id) {
			writeErr(w, 400, "required theme "+id+" isn't installed")
			return
		}
	}
	by := adminName(r)
	s.sduiMutate(w, r, "experiences saved", func(c *sduistore.Config) error {
		pol.Required = stampRequired(pol.Required, requiredOf(c), by, time.Now().UTC())
		if len(pol.Default) == 0 && len(pol.Allowed) == 0 && pol.Required == nil && !pol.ProfileThemes && len(pol.Placements) == 0 {
			c.Experiences = nil
		} else {
			c.Experiences = &pol
		}
		return nil
	})
}

func requiredOf(c *sduistore.Config) *sdui.RequiredPolicy {
	if c.Experiences == nil {
		return nil
	}
	return c.Experiences.Required
}

// stampRequired says who required what and when: a requirement that is the same as the one
// in force keeps its stamp; a new or changed one is stamped by the administrator saving it.
func stampRequired(next, cur *sdui.RequiredPolicy, by string, now time.Time) *sdui.RequiredPolicy {
	if next == nil || (next.Everyone == nil && next.Kids == nil && len(next.Profiles) == 0) {
		return nil
	}
	stamp := func(n, c *sdui.Requirement) *sdui.Requirement {
		if n == nil || (n.Experience == "" && n.Theme == "") {
			return nil
		}
		out := *n
		if c != nil && c.Experience == n.Experience && c.Theme == n.Theme {
			out.By, out.At = c.By, c.At
		} else {
			out.By, out.At = by, now
		}
		return &out
	}
	var ce, ck *sdui.Requirement
	var cp map[string]sdui.Requirement
	if cur != nil {
		ce, ck, cp = cur.Everyone, cur.Kids, cur.Profiles
	}
	out := &sdui.RequiredPolicy{Everyone: stamp(next.Everyone, ce), Kids: stamp(next.Kids, ck)}
	for id, r := range next.Profiles {
		var c *sdui.Requirement
		if old, ok := cp[id]; ok {
			c = &old
		}
		r := r
		if st := stamp(&r, c); st != nil {
			if out.Profiles == nil {
				out.Profiles = map[string]sdui.Requirement{}
			}
			out.Profiles[id] = *st
		}
	}
	if out.Everyone == nil && out.Kids == nil && len(out.Profiles) == 0 {
		return nil
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// byAdmin is " by <name>" for a requirement that says who set it.
func byAdmin(p *sdui.RequiredPart) string {
	if p == nil || p.By == "" {
		return ""
	}
	return " by " + p.By
}

// GET /api/sdui/experiences: every experience as data (docs/SDUI.md, "Experiences"), the
// themes there are and which one each experience wears by default, so a chooser can draw
// them all. The administrator's policy decides which a person may pick (GET
// /sdui/experience); this is the catalogue.
func (s *Server) handleSDUIExperiences(w http.ResponseWriter, r *http.Request) {
	themeList := []string{}
	if s.Themes != nil {
		for _, m := range s.Themes.List() {
			themeList = append(themeList, m.ID)
		}
	}
	writeJSON(w, 200, map[string]any{"experiences": sdui.Experiences, "navStyles": sdui.NavStyles, "themes": themeList, "default": sdui.ExpLumen})
}

var errTooManyDevices = errors.New("this profile has changed the layout on too many devices: reset one first")
