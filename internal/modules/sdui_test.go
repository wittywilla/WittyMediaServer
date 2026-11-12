// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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

// An API 4 version without its SDUI, or with one that doesn't fit, isn't installed by itself: it
// is downloaded, checked and held for an administrator's "incomplete" override, which installs it
// with nothing of its own in any app.
func TestAnAPI4ModuleWithoutGoodSDUINeedsAnOverride(t *testing.T) {
	bad := strings.Replace(photosUI, `"component":"Shelf"`, `"component":"Shelve"`, 1)
	for name, extra := range map[string]map[string][]byte{"none": nil, "bad": {sdui.ModuleFile: []byte(bad)}} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			w.publishWith("0.4.0", api4("0.4.0"), extra)
			m := w.manager()
			defer m.Close()
			check(t, m)
			s := installWait(t, m)
			if s.Phase != PhaseNeedsOverride || s.Pending == nil || s.Pending.Version != "0.4.0" || !slices.Equal(s.Pending.NeedsOverride.Kinds, []string{OverrideIncomplete}) {
				t.Fatalf("not held for an override: %+v", s)
			}
			if s.Version == "0.4.0" {
				t.Fatal("in use without a go-ahead")
			}
			// Without the go-ahead, or with the wrong one, nothing happens.
			var need *ErrNeedsOverride
			if err := m.InstallVersionWith("wittyphotos", "0.4.0", true, nil); !errors.As(err, &need) {
				t.Fatalf("no go-ahead: %v", err)
			}
			if err := m.InstallVersionWith("wittyphotos", "0.4.0", true, &OverrideAccept{Accept: []string{OverrideUnsigned}, By: "owner"}); err == nil {
				t.Fatal("a go-ahead for something else was taken")
			}
			if err := m.InstallVersionWith("wittyphotos", "0.4.0", true, &OverrideAccept{Accept: []string{OverrideIncomplete}, By: "owner"}); err != nil {
				t.Fatal(err)
			}
			s = settled(t, m)
			if s.Version != "0.4.0" || s.Override == nil || !slices.Equal(s.Override.Kinds, []string{OverrideIncomplete}) || s.Override.By != "owner" || s.Pin != "0.4.0" {
				t.Fatalf("after the go-ahead: %+v", s)
			}
			if uis, _ := m.UIs(); len(uis) != 0 {
				t.Fatalf("UI from a version whose SDUI didn't fit: %v", uis)
			}
			// It survives a restart, overridden.
			m.Close()
			m2 := w.manager()
			defer m2.Close()
			if s := one(t, m2); s.Version != "0.4.0" || s.Override == nil {
				t.Fatalf("after a restart: %+v", s)
			}
		})
	}
}

func settled(t *testing.T, m *Manager) Status {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Minute); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if s := one(t, m); s.Phase == PhaseIdle || s.Phase == PhaseFailed {
			return s
		}
	}
	t.Fatal("never finished")
	return Status{}
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

func TestOnlyAHelperMayDeclareSources(t *testing.T) {
	dir := t.TempDir()
	ui := strings.ReplaceAll(photosUI, "wittyphotos", "wittycomics")
	if err := os.WriteFile(filepath.Join(dir, sdui.ModuleFile), []byte(ui), 0o644); err != nil {
		t.Fatal(err)
	}
	web := &wittymodule.Manifest{ID: "wittycomics", Delivery: wittymodule.DeliveryWeb, API: SDUIAPI}
	if _, err := readUI(dir, web, sdui.ModuleChecks{}); err == nil || !strings.Contains(err.Error(), "only a helper") {
		t.Fatalf("a web bundle with sources: %v", err)
	}
	helper := &wittymodule.Manifest{ID: "wittycomics", Delivery: wittymodule.DeliveryHelper, API: SDUIAPI}
	if _, err := readUI(dir, helper, sdui.ModuleChecks{}); err != nil {
		t.Fatalf("a helper with sources: %v", err)
	}
}

// A helper that answers one call and leaves isn't failing: the next call starts a new one
// and nothing is counted against the version.
func TestAHelperThatLeavesAfterACallIsStartedAgain(t *testing.T) {
	w := newWorld(t)
	w.publishWith("0.4.0", manifestJSON("0.4.0", "0.4.0", "0.5.x", SDUIAPI, "one-call"), map[string][]byte{sdui.ModuleFile: []byte(photosUI)})
	m := w.manager()
	defer m.Close()
	check(t, m)
	if s := installWait(t, m); s.Version != "0.4.0" {
		t.Fatalf("%+v", s)
	}
	for i := range 4 {
		if _, err := m.UIImage("wittyphotos", "g.png"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		time.Sleep(300 * time.Millisecond) // it has left by now
	}
	if s := one(t, m); s.LastFail != "" || s.Version != "0.4.0" {
		t.Fatalf("counted as failing: %+v", s)
	}
}
