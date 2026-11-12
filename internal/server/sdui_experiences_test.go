// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	builtin "github.com/wittywilla/wittymediaserver/themes"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

type experienceAnswer struct {
	Experience string                 `json:"experience"`
	Shell      sdui.Shell             `json:"shell"`
	Choice     *sdui.ExperienceChoice `json:"choice"`
	Editable   bool                   `json:"editable"`
}

func TestExperiencesAreChosenByTheServerAndEachProfilePerDevice(t *testing.T) {
	ts, admin, adult, kid := sduiAccounts(t)
	get := func(token, query string) experienceAnswer {
		t.Helper()
		w := ts.do(t, "GET", "/api/v1/sdui/experience?"+query, token, nil)
		if w.Code != 200 {
			t.Fatalf("GET layout %s: %d %s", query, w.Code, w.Body)
		}
		var a experienceAnswer
		_ = json.Unmarshal(w.Body.Bytes(), &a)
		return a
	}
	// Nobody chose: the apps as they always looked.
	if a := get(adult, "formFactor=phone"); a.Experience != "lumen" || a.Shell.Nav != "bottomTabs" || a.Shell.Size != 56 || !a.Editable || len(a.Choice.Options) != len(sdui.Experiences) {
		t.Fatalf("default on a phone: %+v", a)
	}
	if a := get(adult, "formFactor=tv"); a.Shell.Nav != "tvTopMenu" {
		t.Fatalf("default on a television: %+v", a)
	}

	// The administrator: streaming on phones, and only three layouts on offer.
	w := ts.do(t, "GET", "/api/sdui/config/experience", admin, nil)
	var cur struct{ ETag string }
	_ = json.Unmarshal(w.Body.Bytes(), &cur)
	pol := map[string]any{"default": map[string]string{"phone": "stage"}, "allowed": []string{"classic", "stage", "compact"}}
	if w = ts.do(t, "PUT", "/api/sdui/config/experience", admin, pol, "If-Match", cur.ETag); w.Code != 200 {
		t.Fatalf("saving layouts: %d %s", w.Code, w.Body)
	}
	if w = ts.do(t, "PUT", "/api/sdui/config/experience", adult, pol); w.Code != 403 {
		t.Errorf("a non-admin saved the server's layouts: %d", w.Code)
	}
	_ = json.Unmarshal(ts.do(t, "GET", "/api/sdui/config/experience", admin, nil).Body.Bytes(), &cur)
	bad := map[string]any{"default": map[string]string{"phone": "paper"}, "allowed": []string{"classic"}}
	if w = ts.do(t, "PUT", "/api/sdui/config/experience", admin, bad, "If-Match", cur.ETag); w.Code != 400 || !strings.Contains(w.Body.String(), "allowed") {
		t.Errorf("a default that isn't allowed: %d %s", w.Code, w.Body)
	}
	if a := get(adult, "formFactor=phone"); a.Experience != "stage" || !a.Shell.Overlay || len(a.Choice.Options) != 3 {
		t.Fatalf("after the administrator's choice: %+v", a)
	}

	// A profile's own choice is per device, and only among what is allowed.
	if w = ts.do(t, "PUT", "/api/v1/sdui/experience?formFactor=phone&device=sams-phone", adult, map[string]string{"experience": "compact"}); w.Code != 200 {
		t.Fatalf("choosing: %d %s", w.Code, w.Body)
	}
	if w = ts.do(t, "PUT", "/api/v1/sdui/experience?formFactor=phone&device=sams-phone", adult, map[string]string{"experience": "paper"}); w.Code != 403 {
		t.Errorf("a layout the server doesn't offer: %d", w.Code)
	}
	if w = ts.do(t, "PUT", "/api/v1/sdui/experience?formFactor=phone", adult, map[string]string{"experience": "spiral"}); w.Code != 400 {
		t.Errorf("a layout that isn't one: %d", w.Code)
	}
	if a := get(adult, "formFactor=phone&device=sams-phone"); a.Experience != "compact" || a.Choice.Chosen != "compact" {
		t.Fatalf("on the chosen device: %+v", a)
	}
	if a := get(adult, "formFactor=phone&device=other-phone"); a.Experience != "stage" {
		t.Fatalf("another device of the same profile: %+v", a)
	}
	if a := get(kid, "formFactor=phone&device=sams-phone"); a.Experience != "stage" {
		t.Fatalf("another profile on that device name: %+v", a)
	}

	// Pages and the navigation follow it.
	doc := sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/home?formFactor=phone&device=sams-phone", adult, nil))
	if doc.Layout.Shell == nil || doc.Layout.Shell.Experience != "compact" || doc.Layout.Density != "compact" {
		t.Fatalf("the page's layout: %+v", doc.Layout)
	}
	for _, n := range doc.Root.Children {
		if n.Type == "Shelf" || n.Type == "Grid" || n.Type == "Hero" {
			t.Errorf("%s %s on a compact list page", n.Type, n.ID)
		}
	}
	var nav sdui.NavDocument
	_ = json.Unmarshal(ts.do(t, "GET", "/api/v1/sdui/navigation?formFactor=phone&device=sams-phone", adult, nil).Body.Bytes(), &nav)
	if nav.Shell == nil || nav.Shell.Experience != "compact" || nav.Experiences == nil || nav.Experiences.Chosen != "compact" {
		t.Fatalf("the navigation's shell: %+v %+v", nav.Shell, nav.Experiences)
	}
	// A layout asked for by name, to try it, wins for that request only.
	doc = sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/home?formFactor=phone&device=sams-phone&experience=classic", adult, nil))
	if doc.Layout.Shell.Experience != "classic" {
		t.Errorf("trying a layout: %+v", doc.Layout.Shell)
	}

	// Going back to the server's.
	if w = ts.do(t, "DELETE", "/api/v1/sdui/experience?formFactor=phone&device=sams-phone", adult, nil); w.Code != 200 {
		t.Fatalf("reset: %d", w.Code)
	}
	if a := get(adult, "formFactor=phone&device=sams-phone"); a.Experience != "stage" || a.Choice.Chosen != "" {
		t.Fatalf("after reset: %+v", a)
	}

}

// An administrator can require a look, a theme or both, for everyone, for kids or for one
// profile: the server resolves it and says who required it; the person can't choose over it.
func TestRequiredLookIsResolvedAndLocked(t *testing.T) {
	ts, admin, adult, kid := sduiAccounts(t)
	if err := ts.Themes.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	etag := func() string {
		var cur struct{ ETag string }
		_ = json.Unmarshal(ts.do(t, "GET", "/api/sdui/config/experience", admin, nil).Body.Bytes(), &cur)
		return cur.ETag
	}
	put := func(body any) int {
		return ts.do(t, "PUT", "/api/sdui/config/experience", admin, body, "If-Match", etag()).Code
	}
	if c := put(map[string]any{"required": map[string]any{"everyone": map[string]string{"experience": "spiral"}}}); c != 400 {
		t.Errorf("an experience that isn't one: %d", c)
	}
	if c := put(map[string]any{"required": map[string]any{"everyone": map[string]string{"theme": "no-such-theme"}}}); c != 400 {
		t.Errorf("a theme that isn't installed: %d", c)
	}
	if c := put(map[string]any{"required": map[string]any{"everyone": map[string]string{"experience": "rail", "theme": "cinema"}, "kids": map[string]string{"experience": "bigpicture"}}}); c != 200 {
		t.Fatalf("saving the requirement: %d", c)
	}
	get := func(token string) experienceAnswer {
		var a experienceAnswer
		_ = json.Unmarshal(ts.do(t, "GET", "/api/v1/sdui/experience?formFactor=desktop", token, nil).Body.Bytes(), &a)
		return a
	}
	if a := get(adult); a.Experience != "rail" || a.Editable || a.Choice == nil || a.Choice.Required == nil || a.Choice.Required.Scope != "everyone" || a.Choice.Required.By == "" {
		t.Fatalf("an adult: %+v %+v", a, a.Choice)
	}
	if a := get(kid); a.Experience != "bigpicture" || a.Choice.Required.Scope != "kids" {
		t.Fatalf("a kid: %+v", a)
	}
	if w := ts.do(t, "PUT", "/api/v1/sdui/experience?formFactor=desktop", adult, map[string]string{"experience": "paper"}); w.Code != 403 || !strings.Contains(w.Body.String(), "required") {
		t.Errorf("choosing over a requirement: %d %s", w.Code, w.Body)
	}
	th := getTheme(t, ts, adult)
	if th.ID != "cinema" || th.Source != "required" || th.Look.RequiredTheme == nil || th.Look.Experience != "rail" || th.Look.RequiredExperience == nil {
		t.Errorf("the finished look: %q %q %+v", th.ID, th.Source, th.Look)
	}
	if len(th.Look.Finish.Gloss) == 0 || th.Look.Finish.Glass == nil || len(th.Look.Scene.Layers) == 0 {
		t.Errorf("finish and scene: %+v", th.Look)
	}
	// Nothing required: the person's choice, and the experience's own theme.
	if c := put(map[string]any{}); c != 200 {
		t.Fatalf("clearing: %d", c)
	}
	th = getTheme(t, ts, adult)
	if th.Source != "server" || th.Look.RequiredTheme != nil || th.Look.Experience != "lumen" {
		t.Errorf("after clearing: %q %+v", th.Source, th.Look)
	}
}

// A profile may import a theme of its own when the administrator allows it; it is offered to
// that profile only, and a stylesheet with rules for the interface's classes is refused.
func TestProfileThemeImport(t *testing.T) {
	ts, admin, adult, kid := sduiAccounts(t)
	if err := ts.Themes.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	zipOf := func(css string) []byte {
		var b bytes.Buffer
		zw := zip.NewWriter(&b)
		for n, body := range map[string]string{"theme.json": `{"name":"Mine","id":"mine"}`, "theme.css": css} {
			w, _ := zw.Create(n)
			_, _ = w.Write([]byte(body))
		}
		zw.Close()
		return b.Bytes()
	}
	good := zipOf(":root { color-scheme: dark; --bg: #101820; --fg: #f0f0f0; --accent: #66ccff; }")
	if w := ts.do(t, "POST", "/api/themes/mine", adult, good); w.Code != 403 {
		t.Errorf("before the administrator allows it: %d", w.Code)
	}
	var cur struct{ ETag string }
	_ = json.Unmarshal(ts.do(t, "GET", "/api/sdui/config/experience", admin, nil).Body.Bytes(), &cur)
	if w := ts.do(t, "PUT", "/api/sdui/config/experience", admin, map[string]any{"profileThemes": true}, "If-Match", cur.ETag); w.Code != 200 {
		t.Fatalf("allowing it: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/themes/mine", adult, zipOf(".hero h1 { color: red }")); w.Code != 400 || !strings.Contains(w.Body.String(), "variables") {
		t.Errorf("a stylesheet with class rules: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/themes/mine", adult, good); w.Code != 201 {
		t.Fatalf("importing: %d %s", w.Code, w.Body)
	}
	listed := func(token string) bool {
		var l struct{ Themes []struct{ ID string } }
		_ = json.Unmarshal(ts.do(t, "GET", "/api/themes", token, nil).Body.Bytes(), &l)
		for _, m := range l.Themes {
			if m.ID == "mine" {
				return true
			}
		}
		return false
	}
	if !listed(adult) || listed(kid) || listed(admin) {
		t.Errorf("offered to: adult %v kid %v admin %v", listed(adult), listed(kid), listed(admin))
	}
	if w := ts.do(t, "POST", "/api/themes/mine", kid, good); w.Code != 409 {
		t.Errorf("another profile taking the id: %d", w.Code)
	}
}

// The rail opens a drawer for each library, headed by its name, with its views.
func TestRailNavigationCarriesLibraryDrawers(t *testing.T) {
	f := newSDUIFixture(t)
	ts, adult := f.ts, f.admin
	get := func(exp string) sdui.NavDocument {
		var nav sdui.NavDocument
		_ = json.Unmarshal(ts.do(t, "GET", "/api/v1/sdui/navigation?formFactor=desktop&experience="+exp, adult, nil).Body.Bytes(), &nav)
		return nav
	}
	rail := get("rail")
	if rail.Shell == nil || rail.Shell.Nav != "railDrawer" || rail.Shell.Open != 240 || rail.Shell.Drawer != 360 {
		t.Fatalf("shell: %+v", rail.Shell)
	}
	found := 0
	for _, e := range rail.Items {
		if strings.HasPrefix(e.ID, "lib-") {
			found++
			if e.Drawer == nil || e.Drawer.Title != e.Label || len(e.Drawer.Views) < 2 || e.Drawer.Views[0].Params["library"] == "" {
				t.Errorf("%s has no drawer: %+v", e.ID, e.Drawer)
			}
		}
	}
	if found == 0 {
		t.Error("no library entries to open a drawer from")
	}
	for _, e := range get("classic").Items {
		if e.Drawer != nil {
			t.Errorf("%s has a drawer in an experience that has none", e.ID)
		}
	}
}

// Every region of the shell has a place: the experience's, the administrator's for everyone,
// the profile's own for its device, merged by the server; the way to Settings and You stays.
func TestPlacementsAreMergedAndLocked(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	shell := func(token string) *sdui.Shell {
		return sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/home?formFactor=desktop&experience=lumen&device=d1", token, nil)).Layout.Shell
	}
	if sh := shell(adult); len(sh.Placements) != len(sdui.Regions) || *sh.Placements["search"].Visible != true {
		t.Fatalf("every region has a place: %+v", sh.Placements)
	}
	// The administrator moves the search for everyone.
	var cur struct{ ETag string }
	_ = json.Unmarshal(ts.do(t, "GET", "/api/sdui/config/experience", admin, nil).Body.Bytes(), &cur)
	pol := map[string]any{"placements": map[string]any{"lumen": map[string]any{"search": map[string]string{"position": "floating"}}}}
	if w := ts.do(t, "PUT", "/api/sdui/config/experience", admin, pol, "If-Match", cur.ETag); w.Code != 200 {
		t.Fatalf("admin placements: %d %s", w.Code, w.Body)
	}
	if sh := shell(adult); sh.Placements["search"].Position != "floating" {
		t.Errorf("for everyone: %+v", sh.Placements["search"])
	}
	// A profile's own, for its device only; what can't be hidden is refused.
	url := "/api/v1/sdui/placements?formFactor=desktop&experience=lumen&device=d1"
	if w := ts.do(t, "PUT", url, adult, map[string]any{"placements": map[string]any{"clock": map[string]any{"visible": false}, "dock": map[string]string{"position": "top"}}}); w.Code != 200 {
		t.Fatalf("own placements: %d %s", w.Code, w.Body)
	}
	sh := shell(adult)
	if sh.Placements["clock"].Visible == nil || *sh.Placements["clock"].Visible || sh.Placements["dock"].Position != "top" || sh.Placements["search"].Position != "floating" {
		t.Errorf("merged: %+v", sh.Placements)
	}
	if other := sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/home?formFactor=desktop&experience=lumen&device=d2", adult, nil)).Layout.Shell; *other.Placements["clock"].Visible != true {
		t.Error("another device took the change")
	}
	for name, body := range map[string]any{
		"the menu hidden":     map[string]any{"placements": map[string]any{"nav": map[string]any{"visible": false}}},
		"You hidden":          map[string]any{"placements": map[string]any{"profile": map[string]any{"visible": false}}},
		"a region that isn't": map[string]any{"placements": map[string]any{"sparkle": map[string]any{"visible": true}}},
		"a place it can't be": map[string]any{"placements": map[string]any{"clock": map[string]any{"position": "start"}}},
	} {
		if w := ts.do(t, "PUT", url, adult, body); w.Code != 400 {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	if w := ts.do(t, "DELETE", url, adult, nil); w.Code != 200 || *shell(adult).Placements["clock"].Visible != true {
		t.Errorf("reset: %d", w.Code)
	}
}
