// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
	"github.com/wittywilla/wittymodule"
)

// The published wittymodule a server is built with must never be ahead of the API this
// server says it speaks, or a module built for it would be taken without its SDUI checked.
func TestServerAPIIsAtLeastWittyModules(t *testing.T) {
	if wittymodule.API > API || wittymodule.MinAPI < MinAPI {
		t.Fatalf("wittymodule speaks %d–%d and this server %d–%d: raise API here, with what it adds", wittymodule.MinAPI, wittymodule.API, MinAPI, API)
	}
	if SDUIAPI > API {
		t.Fatal("the API that makes SDUI part of a module is one this server doesn't take")
	}
}

const photosUI = `{"format":"wittymodule-sdui","formatVersion":1,"module":"wittyphotos","catalog":2,
"theme":{"scope":"wittyphotos","tokens":{"accent":"#2E7D32"}},
"sources":[{"type":"wittyphotos.picks","label":"Picks","yields":["game"],"cardStyle":"square","paged":true,"since":2}],
"pages":[{"id":"wittyphotos","kind":"custom","title":"Picks","sections":[{"id":"picks","component":"Shelf","source":{"type":"wittyphotos.picks"}}]}],
"navigation":[{"id":"wittyphotos","type":"page","page":"wittyphotos"}],
"settings":[{"id":"wittyphotos","title":"Picks","groups":[{"rows":[{"key":"household","kind":"int","label":"Players","default":4}]}]}]}`

func api4(ver string) []byte {
	return manifestJSON(ver, "0.4.0", "0.5.x", SDUIAPI)
}

func TestAnAPI4ModuleWithoutSDUIIsRefused(t *testing.T) {
	w := newWorld(t)
	w.publish("0.4.0", api4("0.4.0"))
	m := w.manager()
	defer m.Close()
	check(t, m)
	s := installWait(t, m)
	if s.Phase != PhaseFailed || !strings.Contains(s.Error, "sdui.json") {
		t.Fatalf("installed without SDUI: %+v", s)
	}
	if uis, _ := m.UIs(); len(uis) != 0 {
		t.Fatalf("UI from a refused version: %v", uis)
	}
}

func TestAnAPI4ModuleWithBadSDUIIsRefused(t *testing.T) {
	w := newWorld(t)
	bad := strings.Replace(photosUI, `"component":"Shelf"`, `"component":"Shelve"`, 1)
	w.publishWith("0.4.0", api4("0.4.0"), map[string][]byte{sdui.ModuleFile: []byte(bad)})
	m := w.manager()
	defer m.Close()
	check(t, m)
	s := installWait(t, m)
	if s.Phase != PhaseFailed || !strings.Contains(s.Error, "catalogue") {
		t.Fatalf("installed with a page that doesn't validate: %+v", s)
	}
}

func TestAnAPI4ModuleWithSDUIIsUsedAndAnswersItsSources(t *testing.T) {
	w := newWorld(t)
	w.publishWith("0.4.0", api4("0.4.0"), map[string][]byte{sdui.ModuleFile: []byte(photosUI)})
	m := w.manager()
	defer m.Close()
	check(t, m)
	if s := installWait(t, m); s.Version != "0.4.0" {
		t.Fatalf("%+v", s)
	}
	uis, gen := m.UIs()
	if len(uis) != 1 || uis[0].Module != "wittyphotos" || len(uis[0].Pages) != 1 {
		t.Fatalf("UIs %+v", uis)
	}
	if m.UI("wittyphotos") == nil {
		t.Fatal("no UI for the module")
	}
	got, err := m.UIItems("wittyphotos", ItemsRequest{Source: map[string]any{"type": "wittyphotos.picks"}, Limit: 12, Viewer: Viewer{ProfileKind: "kids"}, Settings: map[string]any{"household": 3}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Title != "wittyphotos.picks for kids 3" {
		t.Fatalf("items %+v", got)
	}
	img, err := m.UIImage("wittyphotos", "g.png")
	if err != nil || !bytes.HasPrefix(img, []byte("\x89PNG")) {
		t.Fatalf("image %q %v", img, err)
	}
	// Turning it off takes its UI away, and says so with a new generation.
	if err := m.SetEnabled("wittyphotos", false, nil); err != nil {
		t.Fatal(err)
	}
	uis, gen2 := m.UIs()
	if len(uis) != 0 || gen2 == gen {
		t.Fatalf("after turning it off: %d files, generation %d → %d", len(uis), gen, gen2)
	}
}

func TestAnAPI3ModuleNeedsNoSDUI(t *testing.T) {
	w := newWorld(t)
	w.publish("0.4.0", manifestJSON("0.4.0", "0.4.0", "0.5.x", 3))
	m := w.manager()
	defer m.Close()
	check(t, m)
	if s := installWait(t, m); s.Version != "0.4.0" {
		t.Fatalf("an older-API module without SDUI wasn't installed: %+v", s)
	}
	if uis, _ := m.UIs(); len(uis) != 0 {
		t.Fatalf("UIs %v", uis)
	}
}
