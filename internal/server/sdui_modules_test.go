// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"strings"
	"testing"

	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// A web bundle built for module API 4, with its own SDUI: a page, a row on Home, a menu
// entry, settings and colours.
const comicsUI = `{"format":"wittymodule-sdui","formatVersion":1,"module":"wittycomics","catalog":2,
"theme":{"scope":"wittycomics","label":"Comics","tokens":{"accent":"#7B1FA2"}},
"views":[{"id":"wittycomics.shelf","label":"The comics shelf","platforms":["web"]}],
"pages":[{"id":"wittycomics-kid","kind":"custom","title":"Comics for a profile","slot":"people.profile","sections":[{"id":"note","component":"Text","props":{"text":"Chosen for this profile."}}]},
{"id":"wittycomics","kind":"custom","title":"Comics","icon":"comic","sections":[
  {"id":"recent","component":"Shelf","props":{"title":"Comics lately"},"source":{"type":"recentlyAdded"}},
  {"id":"shelf","component":"NativeView","props":{"view":"wittycomics.shelf"},"children":[{"id":"shelf-note","component":"Notice","props":{"text":"The shelf is on the web app."}}]}]}],
"inserts":[{"page":"home","after":"jump","section":{"id":"wittycomics-reading","component":"Shelf","props":{"title":"Comics you're reading"},"source":{"type":"continueWatching","media":"read"}}}],
"presets":[{"id":"wittycomics-new","label":"New comics","section":{"id":"comics","component":"Shelf","props":{"title":"New comics"},"source":{"type":"recentlyAdded"}}}],
"navigation":[{"id":"wittycomics","type":"page","page":"wittycomics","placement":{"phone":"more"}}],
"settings":[
  {"id":"wittycomics","title":"Comics","slot":"you.games","groups":[{"rows":[
    {"key":"direction","kind":"enum","scope":"profile","label":"Reading direction","default":"ltr","options":[{"value":"ltr","label":"Left to right"},{"value":"rtl","label":"Right to left"}]}]}]},
  {"id":"wittycomics-admin","title":"Comics for the server","admin":true,"groups":[{"rows":[
    {"key":"pageCache","kind":"int","label":"Pages kept ready","default":8,"min":0,"max":64},
    {"key":"apiKey","kind":"secret","label":"Comic Vine key"}]}]}]}`

func TestModuleSDUIIsMergedEverywhere(t *testing.T) {
	ts, admin, adult, kid := sduiAccounts(t)
	ts.Modules = installedBundleWith(t, `export default 1`, `a{}`, modmgr.SDUIAPI, map[string][]byte{sdui.ModuleFile: []byte(comicsUI)})
	if uis, _ := ts.Modules.UIs(); len(uis) != 1 {
		t.Fatalf("the module's SDUI isn't in use: %+v", ts.Modules.Report())
	}

	// The menu has its entry, before Settings.
	w := ts.do(t, "GET", "/api/v1/sdui/navigation?formFactor=desktop", adult, nil)
	var nav sdui.NavDocument
	_ = json.Unmarshal(w.Body.Bytes(), &nav)
	at, settings := -1, -1
	for i, it := range nav.Items {
		switch it.ID {
		case "wittycomics":
			at = i
		case "settings":
			settings = i
		}
	}
	if at < 0 || settings < 0 || at > settings {
		t.Fatalf("navigation %s", w.Body)
	}

	// Its page renders, in its scope with its colours, and the NativeView of a view this
	// client didn't say it has is its sections.
	w = ts.do(t, "GET", "/api/v1/sdui/pages/wittycomics?formFactor=desktop", adult, nil)
	if w.Code != 200 {
		t.Fatalf("page %d %s", w.Code, w.Body)
	}
	doc := sduiDoc(t, w)
	if doc.Theme == nil || doc.Theme.Scope != "wittycomics" || doc.Theme.Tokens["accent"] != "#7B1FA2" {
		t.Errorf("theme %+v", doc.Theme)
	}
	if !strings.Contains(w.Body.String(), "The shelf is on the web app.") {
		t.Errorf("no fallback: %s", w.Body)
	}
	w = ts.do(t, "GET", "/api/v1/sdui/pages/wittycomics?formFactor=desktop&views=wittycomics.shelf", adult, nil)
	if !strings.Contains(w.Body.String(), `"view":"wittycomics.shelf"`) {
		t.Errorf("the view wasn't sent to a client that has it: %s", w.Body)
	}

	if w = ts.do(t, "GET", "/api/v1/theme", adult, nil); !strings.Contains(w.Body.String(), `"wittycomics":{"tokens":{"accent":"#7B1FA2"`) {
		t.Errorf("the theme engine doesn't have the module's colours: %s", w.Body)
	}
	// The administrator's colours for the scope win over the module's.
	w = ts.do(t, "PUT", "/api/theme/modules/wittycomics", admin, map[string]any{"tokens": map[string]string{"accent": "#1565C0"}})
	if w.Code != 200 {
		t.Fatalf("theme override %d %s", w.Code, w.Body)
	}
	doc = sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/wittycomics?formFactor=desktop", adult, nil))
	if doc.Theme.Tokens["accent"] != "#1565C0" {
		t.Errorf("after the override: %+v", doc.Theme)
	}

	// The editor sees it: its page and where it comes from, its preset, its view in the catalogue.
	w = ts.do(t, "GET", "/api/sdui/config/pages", admin, nil)
	if !strings.Contains(w.Body.String(), `"id":"wittycomics","kind":"custom","title":"Comics","icon":"comic"`) || !strings.Contains(w.Body.String(), `"source":"module","module":"wittycomics"`) {
		t.Errorf("config pages %s", w.Body)
	}
	if w = ts.do(t, "GET", "/api/sdui/presets", admin, nil); !strings.Contains(w.Body.String(), `"wittycomics-new"`) {
		t.Errorf("presets %s", w.Body)
	}
	if w = ts.do(t, "GET", "/api/sdui/catalog", adult, nil); !strings.Contains(w.Body.String(), `"wittycomics.shelf"`) {
		t.Error("the catalogue doesn't list the module's view")
	}
	if w = ts.do(t, "GET", "/api/sdui/modules", admin, nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"module":"wittycomics"`) {
		t.Errorf("modules %d %s", w.Code, w.Body)
	}
	if w = ts.do(t, "GET", "/api/sdui/modules", adult, nil); w.Code != 403 {
		t.Errorf("modules for a non-admin: %d", w.Code)
	}
	// A menu saved in the editor may open the module's page, and never a page that isn't there.
	var navCfg struct{ ETag string }
	_ = json.Unmarshal(ts.do(t, "GET", "/api/sdui/config/navigation", admin, nil).Body.Bytes(), &navCfg)
	menu := func(page string) map[string]any {
		return map[string]any{"items": []any{map[string]any{"id": "home", "type": "page", "page": "home"}, map[string]any{"id": "comics", "type": "page", "page": page},
			map[string]any{"id": "settings", "type": "native", "screen": "settings"}}}
	}
	if w = ts.do(t, "PUT", "/api/sdui/config/navigation", admin, menu("nowhere"), "If-Match", navCfg.ETag); w.Code != 400 || !strings.Contains(w.Body.String(), "nowhere") {
		t.Errorf("a menu entry for a page that isn't there: %d %s", w.Code, w.Body)
	}
	if w = ts.do(t, "PUT", "/api/sdui/config/navigation", admin, menu("wittycomics"), "If-Match", navCfg.ETag); w.Code != 200 {
		t.Errorf("a menu entry for the module's page: %d %s", w.Code, w.Body)
	}
	// An administrator's page may use the module's view without a warning.
	w = ts.do(t, "POST", "/api/sdui/validate", admin, map[string]any{"page": map[string]any{"id": "mine", "kind": "custom", "title": "Mine",
		"sections": []any{map[string]any{"id": "v", "component": "NativeView", "props": map[string]any{"view": "wittycomics.shelf"}}}}})
	if !strings.Contains(w.Body.String(), `{"issues":[],"ok":true}`) {
		t.Errorf("validate %s", w.Body)
	}

	// Settings: each profile its own reading direction, the server's for administrators.
	w = ts.do(t, "GET", "/api/sdui/modules/wittycomics/settings", adult, nil)
	var view moduleSettingsView
	_ = json.Unmarshal(w.Body.Bytes(), &view)
	if len(view.Pages) != 1 || view.Values["direction"] != "ltr" || view.Values["pageCache"] != nil || view.Editable {
		t.Fatalf("an adult's settings %s", w.Body)
	}
	if w = ts.do(t, "PUT", "/api/sdui/modules/wittycomics/settings/personal", adult, map[string]any{"values": map[string]any{"direction": "rtl"}}); w.Code != 200 {
		t.Fatalf("personal %d %s", w.Code, w.Body)
	}
	if w = ts.do(t, "PUT", "/api/sdui/modules/wittycomics/settings/personal", adult, map[string]any{"values": map[string]any{"direction": "up"}}); w.Code != 400 {
		t.Errorf("a value that isn't a choice: %d", w.Code)
	}
	if w = ts.do(t, "PUT", "/api/sdui/modules/wittycomics/settings/personal", adult, map[string]any{"values": map[string]any{"pageCache": 3}}); w.Code != 400 {
		t.Errorf("a server setting from a profile: %d", w.Code)
	}
	if w = ts.do(t, "PUT", "/api/sdui/modules/wittycomics/settings", adult, map[string]any{"values": map[string]any{"pageCache": 3}}); w.Code != 403 {
		t.Errorf("server settings from a non-admin: %d", w.Code)
	}
	if w = ts.do(t, "PUT", "/api/sdui/modules/wittycomics/settings", admin, map[string]any{"values": map[string]any{"pageCache": 16, "apiKey": "s3cret"}}); w.Code != 200 {
		t.Fatalf("server settings %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "s3cret") || !strings.Contains(w.Body.String(), `"secretsSet":["apiKey"]`) {
		t.Errorf("the secret: %s", w.Body)
	}
	_ = json.Unmarshal(ts.do(t, "GET", "/api/sdui/modules/wittycomics/settings", adult, nil).Body.Bytes(), &view)
	if view.Values["direction"] != "rtl" {
		t.Errorf("the adult's own value: %+v", view.Values)
	}
	_ = json.Unmarshal(ts.do(t, "GET", "/api/sdui/modules/wittycomics/settings", kid, nil).Body.Bytes(), &view)
	if view.Values["direction"] != "ltr" {
		t.Errorf("another profile sees the adult's value: %+v", view.Values)
	}
	_ = json.Unmarshal(ts.do(t, "GET", "/api/sdui/modules/wittycomics/settings", admin, nil).Body.Bytes(), &view)
	if len(view.Pages) != 2 || view.Values["pageCache"] != float64(16) || view.Values["apiKey"] != nil {
		t.Errorf("the administrator's view: %+v", view)
	}
	if env := ts.ModuleSettingsEnv("wittycomics"); len(env) != 1 || !strings.Contains(env[0], `"pageCache":16`) || !strings.Contains(env[0], "s3cret") {
		t.Errorf("the helper's settings %v", env)
	}
	if w = ts.do(t, "GET", "/api/v1/sdui/modules/settings", adult, nil); !strings.Contains(w.Body.String(), `{"module":"wittycomics","label":"Comics","url":"/api/v1/sdui/modules/wittycomics/settings"}`) {
		t.Errorf("the list for Settings → Modules: %s", w.Body)
	}
	// Settings slots: a module's settings page, and a module's page (for people.profile, opened about a profile).
	if w = ts.do(t, "GET", "/api/v1/sdui/slots/you.games", adult, nil); !strings.Contains(w.Body.String(), `"id":"wittycomics"`) || !strings.Contains(w.Body.String(), `"url":"/api/v1/sdui/modules/wittycomics/settings"`) {
		t.Errorf("the you.games slot: %s", w.Body)
	}
	if w = ts.do(t, "GET", "/api/v1/sdui/slots/people.profile", adult, nil); strings.Contains(w.Body.String(), "wittycomics") {
		t.Errorf("an administrators' slot shows to an adult: %s", w.Body)
	}
	if w = ts.do(t, "GET", "/api/v1/sdui/slots/people.profile", admin, nil); !strings.Contains(w.Body.String(), `"route":"/page/wittycomics-kid?item={profile}"`) {
		t.Errorf("the people.profile slot: %s", w.Body)
	}
	if w = ts.do(t, "GET", "/api/v1/sdui/slots/nowhere", adult, nil); w.Code != 404 {
		t.Errorf("a slot that isn't one: %d", w.Code)
	}
	if w = ts.do(t, "GET", "/api/sdui/modules/wittynothing/settings", adult, nil); w.Code != 404 {
		t.Errorf("a module that isn't there: %d", w.Code)
	}
}

func TestModuleNavigationKeepsTheAdministratorsPlacement(t *testing.T) {
	set := sdui.NewModuleSet([]sdui.ModuleUI{{Module: "wittyx", Navigation: []sdui.NavItem{{ID: "wittyx", Type: "route", Route: "/page/wittyx", Label: "X"}}}})
	nav := moduleNavigation(sdui.StockNavigation(), set)
	if nav.Items[len(nav.Items)-2].ID != "wittyx" || nav.Items[len(nav.Items)-1].ID != "settings" {
		t.Fatalf("%+v", nav.Items)
	}
	placed := sdui.Navigation{Items: []sdui.NavItem{{ID: "wittyx", Type: "route", Route: "/page/wittyx", Label: "Mine", Placement: map[string]string{"phone": "hidden"}}, {ID: "settings", Type: "native", Screen: "settings"}}}
	nav = moduleNavigation(placed, set)
	if len(nav.Items) != 2 || nav.Items[0].Label != "Mine" {
		t.Fatalf("the administrator's entry was replaced: %+v", nav.Items)
	}
}
