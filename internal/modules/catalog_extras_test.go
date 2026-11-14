// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/wittywilla/wittymodule"
)

// withExtras adds fields to every version of the named modules in a catalogue's JSON.
func withExtras(t *testing.T, raw []byte, extra map[string]map[string]any) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, m := range doc["modules"].([]any) {
		mod := m.(map[string]any)
		for _, v := range mod["versions"].([]any) {
			for k, x := range extra[mod["id"].(string)] {
				v.(map[string]any)[k] = x
			}
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func listedVersion(id, delivery string) wittymodule.CatalogModule {
	m := listedModule(id, strings.ToUpper(id[:1])+id[1:])
	m.Versions = []wittymodule.CatalogVersion{{Version: "1.0.0", Delivery: delivery, API: SDUIAPI, MinServer: MinModuleServer, MaxServer: "0.6.x", License: "MIT", Released: "2026-11-01",
		Sums: id + ".sums", Sig: id + ".sums.sig", Manifest: id + ".json",
		Platforms: map[string]wittymodule.CatalogFile{wittymodule.PlatformAll: {URL: id + ".tar.gz", Size: 10, SHA256: strings.Repeat("a", 64)}}}}
	return m
}

// The extras the store reads, from a catalogue as wittymodule.ParseCatalog reads it.
func extrasOf(t *testing.T, raw []byte, id string) (string, string, *StoreUI, *StoreIntegration) {
	t.Helper()
	c, err := wittymodule.ParseCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	return catalogExtras(&c.Find(id).Versions[0])
}

var bigclockExtras = map[string]any{
	"status": "experimental", "statusNote": "Alarms aren't done.",
	"ui": map[string]any{"pages": []any{map[string]any{"id": "clock", "title": "Big Clock"}}, "navigation": []any{"Clock"}, "settings": true, "sources": 2},
	"integration": map[string]any{"routes": 12, "screens": []any{"/clock", "/clock/:id"}, "settings": []any{"Clocks"}, "data": true, "feature": "clocks"},
}

// A store shows how finished a version says it is, the pages it adds and what it adds to the
// server before anything is downloaded, says what kind of module each is, and keeps all of it
// across a restart.
func TestTheStoreShowsStatusAndWhatAModuleAdds(t *testing.T) {
	needForks(t)
	w := newStoresWorld(t)
	pub, key := testKey(t)
	raw := withExtras(t, testStoreCatalog(2, listedVersion("bigclock", wittymodule.DeliveryWeb), listedVersion("tinyhelper", wittymodule.DeliveryHelper)),
		map[string]map[string]any{"bigclock": bigclockExtras, "tinyhelper": {"status": "beta"}})
	w.net.put(storeURL, raw)
	w.net.put(storeURL+".sig", signed(key, raw))
	if _, err := w.s.Add(context.Background(), storeURL, pub, false, "admin"); err != nil {
		t.Fatal(err)
	}
	check := func(s *Stores, when string) {
		t.Helper()
		items := map[string]StoreItem{}
		for _, it := range s.View().Items {
			items[it.ID] = it
		}
		clock, helper := items["bigclock"], items["tinyhelper"]
		if clock.ModuleKind != wittymodule.DeliveryWeb || helper.ModuleKind != wittymodule.DeliveryHelper {
			t.Errorf("%s: kinds %q %q", when, clock.ModuleKind, helper.ModuleKind)
		}
		if clock.ReleaseStatus != StatusExperimental || clock.StatusNote != "Alarms aren't done." || helper.ReleaseStatus != StatusBeta {
			t.Errorf("%s: status %q %q %q", when, clock.ReleaseStatus, clock.StatusNote, helper.ReleaseStatus)
		}
		if clock.UI == nil || len(clock.UI.Pages) != 1 || clock.UI.Pages[0].Title != "Big Clock" || clock.UI.Navigation[0] != "Clock" || !clock.UI.Settings || clock.UI.Sources != 2 {
			t.Errorf("%s: ui %+v", when, clock.UI)
		}
		if in := clock.Integration; in == nil || in.Routes != 12 || len(in.Screens) != 2 || in.Settings[0] != "Clocks" || !in.Data || in.Feature != "clocks" {
			t.Errorf("%s: integration %+v", when, clock.Integration)
		}
		if helper.UI != nil || helper.Integration != nil {
			t.Errorf("%s: a version that says nothing adds nothing: %+v %+v", when, helper.UI, helper.Integration)
		}
		if !clock.MatchesKinds("helper, web") || clock.MatchesKinds("helper") || !clock.MatchesKinds("") {
			t.Errorf("%s: kind filter", when)
		}
	}
	check(w.s, "added")
	check(NewStores(StoresConfig{Manager: w.m, Dir: w.dir, Transport: w.net}), "after a restart")
}

// What a catalogue says of a version is read from the signed catalogue itself, needing no
// sandbox: its status, its UI and its integration summary, and nothing for a version that
// says nothing.
func TestCatalogueExtrasAreRead(t *testing.T) {
	raw := withExtras(t, testStoreCatalog(1, listedVersion("bigclock", wittymodule.DeliveryWeb), listedVersion("quiet", wittymodule.DeliveryWeb)),
		map[string]map[string]any{"bigclock": bigclockExtras})
	status, note, ui, in := extrasOf(t, raw, "bigclock")
	if status != StatusExperimental || note != "Alarms aren't done." || ui == nil || ui.Pages[0].ID != "clock" {
		t.Errorf("status %q %q, ui %+v", status, note, ui)
	}
	if in == nil || in.Routes != 12 || !slices.Equal(in.Screens, []string{"/clock", "/clock/:id"}) || in.RouteKinds != nil || in.Setup != nil {
		t.Errorf("integration %+v", in)
	}
	if status, note, ui, in := extrasOf(t, raw, "quiet"); status != "" || note != "" || ui != nil || in != nil {
		t.Errorf("a version that says nothing: %q %q %+v %+v", status, note, ui, in)
	}
	// A status no server knows makes the whole catalogue unreadable, so it never looks finished.
	bad := withExtras(t, raw, map[string]map[string]any{"quiet": {"status": "something new"}})
	if _, err := wittymodule.ParseCatalog(bad); err == nil {
		t.Error("a catalogue with an unknown status was read")
	}
	// An integration summary larger than integration.json can be is refused with it.
	bad = withExtras(t, raw, map[string]map[string]any{"quiet": {"integration": map[string]any{"routes": 5000}}})
	if _, err := wittymodule.ParseCatalog(bad); err == nil {
		t.Error("a catalogue with an impossible integration summary was read")
	}
}

// What a catalogue adds is held to the limits sdui.json and integration.json have, and cleaned
// of what could fool a page, whoever wrote it.
func TestCatalogueExtrasAreHeldToTheirLimits(t *testing.T) {
	u := &wittymodule.CatalogUI{Sources: 9000, Inserts: -4}
	for i := range 40 {
		u.Pages = append(u.Pages, wittymodule.CatalogUIPage{ID: fmt.Sprintf("p%d", i), Title: "Page‮" + strings.Repeat("x", 200)})
	}
	u.Pages = append(u.Pages, wittymodule.CatalogUIPage{ID: "blank", Title: "  "})
	for range 20 {
		u.Navigation = append(u.Navigation, "Somewhere")
	}
	ui := storeUI(u)
	if len(ui.Pages) != 32 || len(ui.Navigation) != 16 || ui.Sources != 64 || ui.Inserts != 0 {
		t.Fatalf("limits: %+v", ui)
	}
	if n := len([]rune(ui.Pages[0].Title)); n != 120 || strings.ContainsRune(ui.Pages[0].Title, '‮') {
		t.Errorf("a title: %d %q", n, ui.Pages[0].Title)
	}
	s := &wittymodule.IntegrationSummary{Routes: -3, Feature: "ga​mes", Settings: []string{" ", "Clocks‮"}}
	for i := range 80 {
		s.Screens = append(s.Screens, fmt.Sprintf("/s%d", i))
	}
	in := storeIntegration(s)
	if in.Routes != 0 || len(in.Screens) != 64 || !slices.Equal(in.Settings, []string{"Clocks"}) || in.Feature != "games" {
		t.Errorf("integration limits: %+v", in)
	}
	if storeUI(nil) != nil || storeIntegration(nil) != nil || integrationAdds(nil) != nil {
		t.Error("nothing sums up to nothing")
	}
	_, note, _, _ := catalogExtras(&wittymodule.CatalogVersion{StatusNote: strings.Repeat("n", 900)})
	if len([]rune(note)) != 500 {
		t.Errorf("the note: %d", len([]rune(note)))
	}
}

// An installed copy says more than the catalogue: who may call its routes, what they do, its
// setup steps and the libraries whose pages it draws.
func TestWhatAnInstalledModuleAdds(t *testing.T) {
	in, err := wittymodule.ParseIntegration([]byte(`{"format":"wittymodule-integration","formatVersion":1,"module":"wittyclock","feature":"games",
		"http":[
			{"path":"/clocks","methods":["GET"],"auth":"user"},
			{"path":"/clocks/{id}/ring","methods":["POST"],"auth":"user","item":"id","plays":true},
			{"path":"/clocks/{id}/live","methods":["GET"],"auth":"user","item":"id","stream":true},
			{"path":"/admin/clocks","methods":["PUT"],"auth":"admin"},
			{"path":"/clocks/frame/{ticket}","methods":["GET"],"auth":"ticket","csp":"sandboxed-page"}],
		"web":{"routes":[{"path":"/clock","frame":"app"},{"path":"/clock/:id","frame":"tv"}],
			"takeover":[{"libraryKind":"games","route":"/clock/{library}"},{"libraryKind":"rig","route":"/clock/rig/{library}"}],
			"settings":[{"id":"clocks","label":"Clocks","admin":true}],
			"setup":[{"id":"clocks","title":"Clocks","after":"details"},{"id":"bells","after":"clocks"}]},
		"data":{"store":"sqlite","backup":"settings"}}`))
	if err != nil {
		t.Fatal(err)
	}
	got := integrationAdds(in)
	want := &StoreIntegration{Routes: 5, RouteKinds: &RouteKinds{User: 3, Admin: 1, Ticket: 1, Items: 2, Streams: 1, Plays: 1},
		Screens: []string{"/clock", "/clock/:id"}, Settings: []string{"Clocks"}, Setup: []string{"Clocks", "bells"}, Data: true, Feature: "games",
		Libraries: []string{"games", "rig"}}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	if string(a) != string(b) {
		t.Errorf("what it adds:\n got %s\nwant %s", a, b)
	}
}
