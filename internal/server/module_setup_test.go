// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"net/http/httptest"
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
	if len(home.Sections) != 2 || home.Sections[0].Component != "Notice" || home.Sections[0].Props["to"] != "/page/wittyplay-setup" {
		t.Errorf("home: %+v", home.Sections)
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
}
