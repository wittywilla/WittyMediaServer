// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

type layoutAnswer struct {
	Layout   string             `json:"layout"`
	Shell    sdui.Shell         `json:"shell"`
	Choice   *sdui.LayoutChoice `json:"choice"`
	Editable bool               `json:"editable"`
}

func TestLayoutOptionsAreChosenByTheServerAndEachProfilePerDevice(t *testing.T) {
	ts, admin, adult, kid := sduiAccounts(t)
	get := func(token, query string) layoutAnswer {
		t.Helper()
		w := ts.do(t, "GET", "/api/v1/sdui/layout?"+query, token, nil)
		if w.Code != 200 {
			t.Fatalf("GET layout %s: %d %s", query, w.Code, w.Body)
		}
		var a layoutAnswer
		_ = json.Unmarshal(w.Body.Bytes(), &a)
		return a
	}
	// Nobody chose: the apps as they always looked.
	if a := get(adult, "formFactor=phone"); a.Layout != "sidebar" || a.Shell.Nav != "bottomTabs" || a.Shell.Size != 56 || !a.Editable || len(a.Choice.Options) != 5 {
		t.Fatalf("default on a phone: %+v", a)
	}
	if a := get(adult, "formFactor=tv"); a.Shell.Nav != "tvTopMenu" {
		t.Fatalf("default on a television: %+v", a)
	}

	// The administrator: streaming on phones, and only three layouts on offer.
	w := ts.do(t, "GET", "/api/sdui/config/layouts", admin, nil)
	var cur struct{ ETag string }
	_ = json.Unmarshal(w.Body.Bytes(), &cur)
	pol := map[string]any{"default": map[string]string{"phone": "streaming"}, "allowed": []string{"sidebar", "streaming", "compactList"}}
	if w = ts.do(t, "PUT", "/api/sdui/config/layouts", admin, pol, "If-Match", cur.ETag); w.Code != 200 {
		t.Fatalf("saving layouts: %d %s", w.Code, w.Body)
	}
	if w = ts.do(t, "PUT", "/api/sdui/config/layouts", adult, pol); w.Code != 403 {
		t.Errorf("a non-admin saved the server's layouts: %d", w.Code)
	}
	_ = json.Unmarshal(ts.do(t, "GET", "/api/sdui/config/layouts", admin, nil).Body.Bytes(), &cur)
	bad := map[string]any{"default": map[string]string{"phone": "topTabs"}, "allowed": []string{"sidebar"}}
	if w = ts.do(t, "PUT", "/api/sdui/config/layouts", admin, bad, "If-Match", cur.ETag); w.Code != 400 || !strings.Contains(w.Body.String(), "allowed") {
		t.Errorf("a default that isn't allowed: %d %s", w.Code, w.Body)
	}
	if a := get(adult, "formFactor=phone"); a.Layout != "streaming" || !a.Shell.Overlay || len(a.Choice.Options) != 3 {
		t.Fatalf("after the administrator's choice: %+v", a)
	}

	// A profile's own choice is per device, and only among what is allowed.
	if w = ts.do(t, "PUT", "/api/v1/sdui/layout?formFactor=phone&device=sams-phone", adult, map[string]string{"layout": "compactList"}); w.Code != 200 {
		t.Fatalf("choosing: %d %s", w.Code, w.Body)
	}
	if w = ts.do(t, "PUT", "/api/v1/sdui/layout?formFactor=phone&device=sams-phone", adult, map[string]string{"layout": "topTabs"}); w.Code != 403 {
		t.Errorf("a layout the server doesn't offer: %d", w.Code)
	}
	if w = ts.do(t, "PUT", "/api/v1/sdui/layout?formFactor=phone", adult, map[string]string{"layout": "spiral"}); w.Code != 400 {
		t.Errorf("a layout that isn't one: %d", w.Code)
	}
	if a := get(adult, "formFactor=phone&device=sams-phone"); a.Layout != "compactList" || a.Choice.Chosen != "compactList" {
		t.Fatalf("on the chosen device: %+v", a)
	}
	if a := get(adult, "formFactor=phone&device=other-phone"); a.Layout != "streaming" {
		t.Fatalf("another device of the same profile: %+v", a)
	}
	if a := get(kid, "formFactor=phone&device=sams-phone"); a.Layout != "streaming" {
		t.Fatalf("another profile on that device name: %+v", a)
	}

	// Pages and the navigation follow it.
	doc := sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/home?formFactor=phone&device=sams-phone", adult, nil))
	if doc.Layout.Shell == nil || doc.Layout.Shell.Layout != "compactList" || doc.Layout.Density != "compact" {
		t.Fatalf("the page's layout: %+v", doc.Layout)
	}
	for _, n := range doc.Root.Children {
		if n.Type == "Shelf" || n.Type == "Grid" || n.Type == "Hero" {
			t.Errorf("%s %s on a compact list page", n.Type, n.ID)
		}
	}
	var nav sdui.NavDocument
	_ = json.Unmarshal(ts.do(t, "GET", "/api/v1/sdui/navigation?formFactor=phone&device=sams-phone", adult, nil).Body.Bytes(), &nav)
	if nav.Shell == nil || nav.Shell.Layout != "compactList" || nav.Layouts == nil || nav.Layouts.Chosen != "compactList" {
		t.Fatalf("the navigation's shell: %+v %+v", nav.Shell, nav.Layouts)
	}
	// A layout asked for by name, to try it, wins for that request only.
	doc = sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/home?formFactor=phone&device=sams-phone&layout=sidebar", adult, nil))
	if doc.Layout.Shell.Layout != "sidebar" {
		t.Errorf("trying a layout: %+v", doc.Layout.Shell)
	}

	// Going back to the server's.
	if w = ts.do(t, "DELETE", "/api/v1/sdui/layout?formFactor=phone&device=sams-phone", adult, nil); w.Code != 200 {
		t.Fatalf("reset: %d", w.Code)
	}
	if a := get(adult, "formFactor=phone&device=sams-phone"); a.Layout != "streaming" || a.Choice.Chosen != "" {
		t.Fatalf("after reset: %+v", a)
	}

}
