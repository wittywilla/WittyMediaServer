// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// A module's own setup steps (integration.json web.setup) that have an SDUI page are shown to an
// administrator as "Finish setting up <module>" on Home, on every app, until the step is done or
// skipped. What was done is remembered as <module>.<step>@<version>, so a module that raises a
// step's version shows it again.

const settingModuleSetupDone = "moduleSetupDone"

// pendingModuleStep is one step of an installed module's that this server hasn't done yet.
type pendingModuleStep struct {
	Module string `json:"module"`
	Name   string `json:"name"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Page   string `json:"page"`
	Later  string `json:"later,omitempty"`
	To     string `json:"to,omitempty"`
}

func moduleStepKey(module, id string, version int) string {
	return module + "." + id + "@" + strconv.Itoa(max(version, 1))
}

func (s *Server) moduleStepsDone(ctx context.Context) []string {
	var out []string
	for _, k := range strings.Split(s.DB.GetSetting(ctx, settingModuleSetupDone, ""), ",") {
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}

// pendingModuleSteps are the steps with an SDUI page of the modules in use that aren't done yet.
func (s *Server) pendingModuleSteps(ctx context.Context) []pendingModuleStep {
	if s.Modules == nil {
		return nil
	}
	in, _ := s.Modules.Integrations()
	return pendingSteps(in, s.moduleStepsDone(ctx))
}

func pendingSteps(in []modules.InUse, done []string) []pendingModuleStep {
	var out []pendingModuleStep
	for _, m := range in {
		if m.Integration == nil || m.Integration.Web == nil {
			continue
		}
		for _, st := range m.Integration.Web.Setup {
			if st.Page == "" || slices.Contains(done, moduleStepKey(m.ID, st.ID, st.Version)) {
				continue
			}
			p := pendingModuleStep{Module: m.ID, Name: firstNonEmpty(m.Name, m.ID), ID: st.ID, Title: firstNonEmpty(st.Title, st.ID), Page: st.Page}
			if st.Later != nil {
				p.Later, p.To = st.Later.Where, st.Later.To
			}
			out = append(out, p)
		}
	}
	return out
}

// hostSetupDone records that a step of the calling module's was done or skipped: POST
// /host/v1/setup/{id}, on behalf of an administrator. Only a step the module declares, at the version it declares.
func (s *Server) hostSetupDone(w http.ResponseWriter, r *http.Request, module string) {
	u, _, err := s.onBehalf(r)
	if err != nil || u == nil || !u.IsAdmin {
		writeJSON(w, 403, map[string]string{"error": "setup is an administrator's"})
		return
	}
	id := r.PathValue("id")
	integ := s.hostMods().Integration(module)
	version := 0
	found := false
	if integ != nil && integ.Web != nil {
		for _, st := range integ.Web.Setup {
			if st.ID == id {
				version, found = st.Version, true
			}
		}
	}
	if !found {
		writeJSON(w, 404, map[string]string{"error": "the module has no setup step " + strconv.Quote(id)})
		return
	}
	var in struct {
		Skip bool `json:"skip"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "a step's end is {skip}"})
			return
		}
	}
	ctx := r.Context()
	key := moduleStepKey(module, id, version)
	done := slices.DeleteFunc(s.moduleStepsDone(ctx), func(k string) bool { return strings.HasPrefix(k, module+"."+id+"@") })
	done = append(done, key)
	if err := s.DB.SetSetting(ctx, settingModuleSetupDone, strings.Join(done, ",")); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	action := "setup-done"
	if in.Skip {
		action = "setup-skipped"
	}
	s.writeAudit(ctx, u, module+" "+action, module, action, "", id)
	w.WriteHeader(204)
}

// withModuleSetup puts a "Finish setting up" notice for each module step left at the top of
// Home, for an administrator.
func (s *Server) withModuleSetup(ctx context.Context, page sdui.Page, admin bool) sdui.Page {
	if page.Kind != "home" || !admin {
		return page
	}
	return withSetupNotices(page, s.pendingModuleSteps(ctx))
}

func withSetupNotices(page sdui.Page, steps []pendingModuleStep) sdui.Page {
	if len(steps) == 0 {
		return page
	}
	var notices []sdui.Section
	for _, st := range steps {
		text := st.Title + " is ready to set up."
		if st.Later != "" {
			text += " You can also do it later, in " + st.Later + "."
		}
		notices = append(notices, sdui.Section{ID: "module-setup-" + st.Module + "-" + st.ID, Component: "Notice",
			Props: map[string]any{"tone": "info", "title": "Finish setting up " + st.Name, "text": text, "buttonLabel": "Set up", "to": "/page/" + st.Page}})
	}
	page.Sections = append(notices, page.Sections...)
	return page
}
