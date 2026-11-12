// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
	builtin "github.com/wittywilla/wittymediaserver/themes"
)

// Until first run is over every route an app uses answers 503 setup_required with the
// address to finish at; only the status route, the setup's own and the web app answer.
func TestFirstRunGatesEveryAppRoute(t *testing.T) {
	ts := newTestServer(t)
	if err := ts.Themes.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/sdui/pages/home", "/api/sdui/navigation", "/api/theme", "/api/libraries", "/api/capabilities"} {
		w := ts.do(t, "GET", path, "", nil)
		if w.Code != 503 || !strings.Contains(w.Body.String(), `"setup_required"`) || !strings.Contains(w.Body.String(), "/setup") {
			t.Errorf("%s before setup: %d %s", path, w.Code, w.Body)
		}
	}
	for _, path := range []string{"/api/status", "/api/health", "/api/setup/state", "/api/themes", "/api/sdui/experiences", "/"} {
		if w := ts.do(t, "GET", path, "", nil); w.Code == 503 {
			t.Errorf("%s answers 503 during setup", path)
		}
	}
	var st SetupState
	_ = json.Unmarshal(ts.do(t, "GET", "/api/setup/state", "", nil).Body.Bytes(), &st)
	if st.Complete || len(st.Steps) != 14 || st.Steps[0] != "welcome" || st.Steps[13] != "ready" || !strings.HasSuffix(st.URL, "/setup") {
		t.Fatalf("state: %+v", st)
	}
	// The account is the second step. Its administrator's browser carries on; an app's token doesn't.
	if w := ts.do(t, "POST", "/api/setup", "", map[string]string{"name": "Willow", "password": "correct-horse-1"}); w.Code != 200 && w.Code != 201 {
		t.Fatalf("setup: %d %s", w.Code, w.Body)
	}
	_, tok := ts.user(t, "Sam", "adult", true, "")
	if w := ts.do(t, "GET", "/api/sdui/pages/home", tok, nil); w.Code != 503 {
		t.Errorf("an app's token during setup: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/setup/state", tok, map[string]any{"step": "look", "done": []string{"welcome", "account", "name"}}); w.Code != 200 {
		t.Fatalf("saving the step: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", "/api/setup/state", tok, map[string]any{"step": "elsewhere"}); w.Code != 400 {
		t.Errorf("a step that isn't one: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/setup/complete", tok, nil); w.Code != 200 {
		t.Fatalf("finishing: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/sdui/pages/home", tok, nil); w.Code != 200 {
		t.Errorf("after setup: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", "/api/setup/state", tok, map[string]any{"step": "look"}); w.Code != 409 {
		t.Errorf("saving a step after setup: %d", w.Code)
	}
}

// Before anyone has signed in, any experience can be drawn from generated demo data.
func TestPreviewDrawsAnyExperienceBeforeSignIn(t *testing.T) {
	ts := newTestServer(t)
	if err := ts.Themes.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	for _, e := range sdui.Experiences {
		w := ts.do(t, "GET", "/api/sdui/preview?experience="+e.ID+"&platform=tvos", "", nil)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", e.ID, w.Code, w.Body)
		}
		var p struct {
			Document   sdui.Document    `json:"document"`
			Navigation sdui.NavDocument `json:"navigation"`
			Theme      struct {
				ID     string            `json:"id"`
				Tokens map[string]string `json:"tokens"`
			} `json:"theme"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		if p.Document.Layout == nil || p.Document.Layout.Shell == nil || p.Document.Layout.Shell.Experience != e.ID || len(p.Document.Root.Children) == 0 {
			t.Errorf("%s: no sample page: %+v", e.ID, p.Document.Layout)
		}
		if p.Theme.ID != e.Theme || p.Theme.Tokens["bg"] == "" || p.Navigation.Shell == nil {
			t.Errorf("%s: theme %q (want %q) or navigation missing", e.ID, p.Theme.ID, e.Theme)
		}
	}
	if w := ts.do(t, "GET", "/api/sdui/preview?experience=spiral", "", nil); w.Code != 400 {
		t.Errorf("an experience that isn't one: %d", w.Code)
	}
	var p struct{ Theme struct{ ID string } }
	_ = json.Unmarshal(ts.do(t, "GET", "/api/sdui/preview?experience=lumen&theme=aeroday", "", nil).Body.Bytes(), &p)
	if p.Theme.ID != "aeroday" {
		t.Errorf("a theme asked for: %q", p.Theme.ID)
	}
}
