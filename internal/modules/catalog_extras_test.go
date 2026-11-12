// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymodule"
)

// withExtras adds what a module API 4 catalogue says of each version, which this server's copy
// of the catalogue format doesn't read, to a catalogue's JSON.
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

// A store shows how finished a version says it is and the pages it adds before anything is
// downloaded, says what kind of module each is, and keeps all of it across a restart.
func TestTheStoreShowsStatusAndWhatAModuleAdds(t *testing.T) {
	needForks(t)
	w := newStoresWorld(t)
	pub, key := testKey(t)
	raw := withExtras(t, testStoreCatalog(2, listedVersion("bigclock", wittymodule.DeliveryWeb), listedVersion("tinyhelper", wittymodule.DeliveryHelper)), map[string]map[string]any{
		"bigclock": {"status": "experimental", "statusNote": "Alarms aren't done.\x07", "ui": map[string]any{
			"pages": []any{map[string]any{"id": "clock", "title": "Big Clock"}}, "navigation": []any{"Clock"}, "settings": true, "sources": 2}},
		"tinyhelper": {"status": "something new"},
	})
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
		if clock.ReleaseStatus != StatusExperimental || clock.StatusNote != "Alarms aren't done." {
			t.Errorf("%s: status %q %q", when, clock.ReleaseStatus, clock.StatusNote)
		}
		// A status this server doesn't know never looks finished.
		if helper.ReleaseStatus != StatusIncomplete {
			t.Errorf("%s: an unknown status: %q", when, helper.ReleaseStatus)
		}
		if clock.UI == nil || len(clock.UI.Pages) != 1 || clock.UI.Pages[0].Title != "Big Clock" || clock.UI.Navigation[0] != "Clock" || !clock.UI.Settings || clock.UI.Sources != 2 {
			t.Errorf("%s: ui %+v", when, clock.UI)
		}
		if helper.UI != nil {
			t.Errorf("%s: a version that says nothing adds nothing: %+v", when, helper.UI)
		}
		if !clock.MatchesKinds("helper, web") || clock.MatchesKinds("helper") || !clock.MatchesKinds("") {
			t.Errorf("%s: kind filter", when)
		}
	}
	check(w.s, "added")
	check(NewStores(StoresConfig{Manager: w.m, Dir: w.dir, Transport: w.net}), "after a restart")
}

// What a catalogue adds is held to the limits sdui.json itself has, whoever wrote it.
func TestCatalogueExtrasAreHeldToTheirLimits(t *testing.T) {
	var pages []any
	for i := range 40 {
		pages = append(pages, map[string]any{"id": fmt.Sprintf("p%d", i), "title": "Page‮" + strings.Repeat("x", 200)})
	}
	pages = append(pages, map[string]any{"id": "blank", "title": "  "})
	var nav []any
	for range 20 {
		nav = append(nav, "Somewhere")
	}
	raw, _ := json.Marshal(map[string]any{"modules": []any{map[string]any{"id": "big", "versions": []any{
		map[string]any{"version": "1.0.0", "statusNote": strings.Repeat("n", 900), "ui": map[string]any{"pages": pages, "navigation": nav, "sources": 9000, "inserts": -4}},
		map[string]any{"version": "1.0.1", "ui": "not an object"},
		map[string]any{"version": "1.0.2"},
	}}}})
	x := parseExtras(raw)
	v := x["big"]["1.0.0"]
	if v.UI == nil || len(v.UI.Pages) != 32 || len(v.UI.Navigation) != 16 || v.UI.Sources != 64 || v.UI.Inserts != 0 {
		t.Fatalf("limits: %+v", v.UI)
	}
	if n := len([]rune(v.UI.Pages[0].Title)); n != 120 || strings.ContainsRune(v.UI.Pages[0].Title, '‮') {
		t.Errorf("a title: %d %q", n, v.UI.Pages[0].Title)
	}
	if len([]rune(v.StatusNote)) != 500 || v.Status != "" { // a version that says no status is left unsaid: stable
		t.Errorf("the note: %d, status %q", len([]rune(v.StatusNote)), v.Status)
	}
	if _, ok := x["big"]["1.0.1"]; ok {
		t.Error("an unreadable summary should be dropped, leaving nothing")
	}
	if _, ok := x["big"]["1.0.2"]; ok {
		t.Error("a version that says nothing more takes no room")
	}
	if parseExtras([]byte("{")) != nil {
		t.Error("unreadable JSON")
	}
}

// A catalogue replaced takes what it said with it.
func TestCatalogueExtrasGoWithTheirCatalogue(t *testing.T) {
	_, key := testKey(t)
	raw := withExtras(t, testStoreCatalog(1, listedVersion("bigclock", wittymodule.DeliveryWeb)), map[string]map[string]any{"bigclock": {"status": "beta"}})
	c, err := verifyCatalog(raw, signed(key, raw), nil)
	if err == nil {
		t.Fatal("verified with no key")
	}
	c, err = verifyCatalog(raw, signed(key, raw), []ed25519.PublicKey{key.Public().(ed25519.PublicKey)})
	if err != nil {
		t.Fatal(err)
	}
	if extraOf(c, "bigclock", "1.0.0").Status != StatusBeta {
		t.Fatal("not kept")
	}
	if extraOf(nil, "bigclock", "1.0.0") != (versionExtra{}) {
		t.Error("no catalogue says nothing")
	}
	c = nil
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		runtime.GC()
		extrasMu.Lock()
		n := len(extras)
		extrasMu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d catalogues' extras outlived them", n)
		}
	}
}
