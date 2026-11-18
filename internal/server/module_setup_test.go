// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittymodule/host"
)

// A module's setup step with a page is a notice on an administrator's Home until the module says
// it was done or skipped, on an administrator's behalf; a new version of the step shows it again.
func TestModuleSetupSteps(t *testing.T) {
	ts := newTestServer(t)
	step := wittymodule.SetupStep{ID: "games", Title: "Games", Page: "wittyplay-setup", Later: &wittymodule.SetupLater{Where: "Settings, Games", To: "/settings/gaming"}}
	integ := &wittymodule.Integration{Module: "wittyplay", Web: &wittymodule.WebPart{Setup: []wittymodule.SetupStep{step, {ID: "webonly"}}}}
	ts.hostModsForTest = fakeHostModules{in: integ}
	api := httptest.NewServer(ts.HostAPI())
	defer api.Close()
	ctx := context.Background()
	c := host.NewAt(api.URL, "tok", nil)
	admin, _ := ts.user(t, "setup-admin", "adult", true, "")
	pal, _ := ts.user(t, "setup-pal", "adult", false, "")
	in := []modules.InUse{{ID: "wittyplay", Name: "WittyPlay", Integration: integ}}

	pending := func() []pendingModuleStep { return pendingSteps(in, ts.moduleStepsDone(ctx)) }
	if p := pending(); len(p) != 1 || p[0].Page != "wittyplay-setup" || p[0].Name != "WittyPlay" {
		t.Fatalf("before: %+v", p)
	}
	home := withSetupNotices(sdui.Page{ID: "home", Kind: "home", Sections: []sdui.Section{{ID: "hero", Component: "Hero"}}}, pending())
	if len(home.Sections) != 2 || home.Sections[0].Component != "Notice" || home.Sections[0].Props["to"] != "/page/wittyplay-setup" || home.Sections[0].Props["dismissOp"] != "setup.skip" {
		t.Errorf("home: %+v", home.Sections)
	}
	// The notice, rendered, has its Dismiss as {label, op, params}.
	home.Sections = home.Sections[:1]
	doc, err := sdui.Render(home, &sdui.RenderContext{Admin: true, Flags: map[string]bool{}, Vars: map[string]string{}, Params: map[string]string{}, Resolver: sdui.DemoResolver{}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(doc)
	if !strings.Contains(string(b), `"dismiss":{"label":"Dismiss","op":"setup.skip","params":{"module":"wittyplay","step":"games"}}`) {
		t.Errorf("rendered: %s", b)
	}
	if err := c.Do(ctx, host.On{User: pal.ID}, "POST", "/setup/games", map[string]bool{}, nil); !host.IsForbidden(err) {
		t.Errorf("someone who isn't an administrator: %v", err)
	}
	if err := c.Do(ctx, host.On{User: admin.ID}, "POST", "/setup/nope", map[string]bool{}, nil); !host.IsNotFound(err) {
		t.Errorf("a step it doesn't have: %v", err)
	}
	if err := c.Do(ctx, host.On{User: admin.ID}, "POST", "/setup/games", map[string]bool{"skip": true}, nil); err != nil {
		t.Fatal(err)
	}
	if p := pending(); len(p) != 0 {
		t.Errorf("after skipping: %+v", p)
	}
	integ.Web.Setup[0].Version = 2
	if p := pending(); len(p) != 1 {
		t.Errorf("a new version of the step: %+v", p)
	}
	if err := c.Do(ctx, host.On{User: admin.ID}, "POST", "/setup/games", nil, nil); err != nil {
		t.Fatal(err)
	}
	if done := ts.moduleStepsDone(ctx); len(done) != 1 || done[0] != "wittyplay.games@2" {
		t.Errorf("done: %v", done)
	}
	var state struct{ Done []string }
	if err := c.Do(ctx, host.On{}, "GET", "/setup", nil, &state); err != nil || len(state.Done) != 1 || state.Done[0] != "games" {
		t.Errorf("state: %+v %v", state, err)
	}
}

// First-run setup draws a module's step with the module's own page, which may never tell the
// server it was done. The wizard says so itself when someone moves on: done from the step's own
// Next, put off from Skip for now. Either way "Finish setting up" leaves Home.
func TestModuleStepDoneFromSetup(t *testing.T) {
	ts := newTestServer(t)
	integ := &wittymodule.Integration{Module: "wittytuner", Web: &wittymodule.WebPart{Setup: []wittymodule.SetupStep{{ID: "tv", Title: "Live TV", Page: "wittytuner-setup", Version: 1}}}}
	ts.hostModsForTest = fakeHostModules{in: integ}
	ctx := context.Background()
	_, admin := ts.user(t, "step-admin", "adult", true, "")
	_, pal := ts.user(t, "step-pal", "adult", false, "")
	in := []modules.InUse{{ID: "wittytuner", Name: "WittyTuner", Integration: integ}}
	pending := func() []pendingModuleStep { return pendingSteps(in, ts.moduleStepsDone(ctx)) }

	if rr := ts.do(t, "POST", "/api/setup/module-steps/skip", pal, map[string]any{"module": "wittytuner", "step": "tv", "done": true}); rr.Code != 403 {
		t.Errorf("someone who isn't an administrator: %d", rr.Code)
	}
	if rr := ts.do(t, "POST", "/api/setup/module-steps/skip", admin, map[string]any{"module": "wittytuner", "step": "nope", "done": true}); rr.Code != 404 {
		t.Errorf("a step it doesn't have: %d", rr.Code)
	}
	if len(pending()) != 1 {
		t.Fatalf("before: %+v", pending())
	}
	if rr := ts.do(t, "POST", "/api/setup/module-steps/skip", admin, map[string]any{"module": "wittytuner", "step": "tv", "done": true}); rr.Code != 204 {
		t.Fatalf("done: %d %s", rr.Code, rr.Body)
	}
	if p := pending(); len(p) != 0 {
		t.Errorf("after going through it: %+v", p)
	}
	if done := ts.moduleStepsDone(ctx); len(done) != 1 || done[0] != "wittytuner.tv@1" {
		t.Errorf("done: %v", done)
	}
	// Twice, as the wizard and the module's own page may both say so, is still one.
	if rr := ts.do(t, "POST", "/api/setup/module-steps/skip", admin, map[string]any{"module": "wittytuner", "step": "tv"}); rr.Code != 204 {
		t.Fatalf("again: %d", rr.Code)
	}
	if done := ts.moduleStepsDone(ctx); len(done) != 1 {
		t.Errorf("recorded twice: %v", done)
	}
}
