// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"crypto/ed25519"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymodule"
)

// A version made for an older server can run here, but its pages, settings and features from this
// server may be missing: what happened when WittyPlay 0.3.6 (maxServer 0.6.x) went onto a 0.8.0-alpha
// server in first-run setup. It is never chosen by itself, and installs only when confirmed.

func TestOlderServerDetection(t *testing.T) {
	man := func(min, max string) *wittymodule.Manifest {
		return &wittymodule.Manifest{Name: "WittyPlay", Version: "0.3.6", API: MinAPI, MinServer: min, MaxServer: max}
	}
	cases := []struct {
		min, max, server string
		older            bool
	}{
		{"0.6.0-alpha", "0.6.x", "0.8.0-alpha", true},
		{"0.8.0-alpha", "0.8.x", "0.8.0-alpha", false}, // made for this one
		{"0.6.0-alpha", "0.8.x", "0.8.0-alpha", false}, // its own range includes this server
		{"0.9.0", "0.9.x", "0.8.0-alpha", false},       // made for a newer server: refused, not older
		{"0.6.0-alpha", "0.6.0", "0.8.0-alpha", false}, // an exact maximum is never widened: refused
	}
	for _, c := range cases {
		madeFor, ok := olderServer(man(c.min, c.max), c.server)
		if ok != c.older {
			t.Errorf("%s..%s on %s: older=%v", c.min, c.max, c.server, ok)
		}
		if ok && madeFor != c.max {
			t.Errorf("made for %q", madeFor)
		}
	}
	if err := supports(man("0.9.0", "0.9.x"), "0.8.0-alpha"); err == nil {
		t.Error("a minServer above this server must stay refused")
	}
	w := OlderServerWarning("WittyPlay", "0.3.6", "0.6.x", "0.8.0-alpha")
	want := "WittyPlay 0.3.6 was made for WittyMediaServer 0.6; this server is 0.8.0-alpha. Pages, settings and features from 0.8 will be missing, and it may not work."
	if w != want {
		t.Errorf("the warning:\n%s\nwant\n%s", w, want)
	}
}

func TestOlderServerFitInTheStore(t *testing.T) {
	v := wittymodule.CatalogVersion{Version: "0.3.6", Delivery: wittymodule.DeliveryWeb, API: MinAPI, MinServer: MinModuleServer, MaxServer: "0.6.x", License: "MIT",
		Platforms: map[string]wittymodule.CatalogFile{wittymodule.PlatformAll: {URL: "a", Size: 1, SHA256: strings.Repeat("a", 64)}}}
	f := fitOf("WittyPlay", &v, "0.8.0-alpha")
	if f.OK || f.Kind != FitOlderServer || !strings.Contains(f.Why, "made for WittyMediaServer 0.6; this server is 0.8.0-alpha") {
		t.Fatalf("made for an older server: %+v", f)
	}
	v.MinServer, v.MaxServer = "0.8.0-alpha", "0.8.x"
	if f := fitOf("WittyPlay", &v, "0.8.0-alpha"); !f.OK {
		t.Fatalf("made for this server: %+v", f)
	}
}

// judge says so first, a version list marks it, and nothing automatic picks it.
func TestOlderServerIsNeverChosenByItself(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.3.0", "0.3.x", MinAPI)) // made for 0.3; this server is 0.4.0
	m := w.manager()
	defer m.Close()
	s := check(t, m)
	if s.Latest != nil {
		t.Fatalf("offered as the latest: %+v", s.Latest)
	}
	if s.OlderOffer == nil || !s.OlderOffer.OlderServer || s.OlderOffer.MadeFor != "0.3.x" || !strings.Contains(s.OlderOffer.Reason, "made for WittyMediaServer 0.3") {
		t.Fatalf("the offer that needs confirming: %+v", s.OlderOffer)
	}

	l, err := m.Versions("wittyphotos")
	if err != nil {
		t.Fatal(err)
	}
	var v *VersionInfo
	for i := range l.Versions {
		if l.Versions[i].Version == "0.2.0" {
			v = &l.Versions[i]
		}
	}
	if v == nil || !v.OlderServer || v.MadeFor != "0.3.x" || !v.Allowed || !strings.HasPrefix(v.Warning, "WittyPhotos 0.2.0 was made for WittyMediaServer 0.3; this server is 0.4.0.") {
		t.Fatalf("the version list: %+v", v)
	}

	// Every path that picks a version by itself, and every install that doesn't confirm.
	var older *ErrOlderServer
	if err := m.Install("wittyphotos"); !errors.As(err, &older) || older.Version != "0.2.0" || older.MadeFor != "0.3.x" {
		t.Fatalf("Install: %v", err)
	}
	if err := m.InstallVersion("wittyphotos", "0.2.0", true); !errors.As(err, &older) {
		t.Fatalf("InstallVersion with plain confirm: %v", err)
	}
	if err := m.InstallVersionWith("wittyphotos", "0.2.0", true, nil); !errors.As(err, &older) {
		t.Fatalf("InstallVersionWith: %v", err)
	}
	if err := m.ToLatest("wittyphotos"); err != nil {
		t.Fatalf("ToLatest: %v", err)
	}
	m.autoInstall()
	time.Sleep(50 * time.Millisecond)
	if s := one(t, m); s.Phase != PhaseIdle || s.Version != builtinVersion || w.installed("0.2.0") {
		t.Fatalf("something installed it by itself: %+v", s)
	}
}

// Confirmed, it installs, and while it is in use it says so, offering a version made for this
// server once there is one, which automatic updates may then move to.
func TestOlderServerConfirmedInstallAndItsNotice(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.3.0", "0.3.x", MinAPI))
	m := w.manager()
	defer m.Close()
	check(t, m)
	if err := m.InstallOlder("wittyphotos", false); err == nil {
		t.Fatal("installed without confirmOlder")
	}
	if err := m.InstallVersionOlder("wittyphotos", "0.2.0", false, true); err != nil {
		t.Fatal(err)
	}
	s := waitPhotos(t, m)
	if s.Version != "0.2.0" || s.State != StateInstalled {
		t.Fatalf("after: %+v", s)
	}
	if !s.OlderServer || s.MadeFor != "0.3.x" || !strings.Contains(s.OlderWarning, "No version made for this server is published yet") {
		t.Fatalf("the notice: %+v", s)
	}
	noted := 0
	for _, ev := range s.Events {
		if ev.Kind == "older-server" && ev.Version == "0.2.0" {
			noted++
		}
	}
	if noted != 1 {
		t.Fatalf("the log should say so once: %+v", s.Events)
	}
	m.Close()

	// After a restart, still said, still once.
	m2 := w.manager()
	defer m2.Close()
	if s := one(t, m2); !s.OlderServer || s.OlderWarning == "" {
		t.Fatalf("after a restart: %+v", s)
	}

	// A version made for this server: offered, named in the notice, and automatic updates take it.
	w.publish("0.3.0", manifestJSON("0.3.0", "0.4.0", "0.5.x", MinAPI))
	s = check(t, m2)
	if s.Latest == nil || s.Latest.Version != "0.3.0" || s.OlderOffer != nil || !strings.Contains(s.OlderWarning, "Version 0.3.0, made for this server, is available") {
		t.Fatalf("with a newer version: %+v", s)
	}
	m2.autoInstall()
	if s := waitPhotos(t, m2); s.Version != "0.3.0" || s.OlderServer || s.OlderWarning != "" {
		t.Fatalf("after updating: %+v", s)
	}
}

// When a version made for this server is known, an older-server version newer than it is never
// "the latest", and isn't offered for confirming either.
func TestLatestSkipsOlderServerVersions(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", MinAPI))
	w.publish("0.3.0", manifestJSON("0.3.0", "0.3.0", "0.3.x", MinAPI))
	m := w.manager()
	defer m.Close()
	s := check(t, m)
	if s.Latest == nil || s.Latest.Version != "0.2.0" || s.OlderOffer != nil {
		t.Fatalf("latest: %+v older: %+v", s.Latest, s.OlderOffer)
	}
}

func waitPhotos(t *testing.T, m *Manager) Status {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		s := one(t, m)
		if s.Phase == PhaseIdle || s.Phase == PhaseFailed {
			if s.Phase == PhaseFailed {
				t.Fatalf("failed: %s", s.Error)
			}
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("never finished")
	return Status{}
}

// olderStoreWorld is a 0.8.0-alpha server whose catalogue has only versions made for 0.6, as the
// public catalogue had when first-run setup installed WittyPlay 0.3.6.
func olderStoreWorld(t *testing.T) (*Manager, *Stores, func(serial int64, mods ...wittymodule.CatalogModule), string, ed25519.PrivateKey) {
	t.Helper()
	pub, key, _ := ed25519.GenerateKey(nil)
	dir := filepath.Join(t.TempDir(), "modules")
	starter := filepath.Join(t.TempDir(), "starter")
	if err := os.MkdirAll(starter, 0o755); err != nil {
		t.Fatal(err)
	}
	publish := func(serial int64, mods ...wittymodule.CatalogModule) {
		raw := testStoreCatalog(serial, mods...)
		_ = os.WriteFile(filepath.Join(starter, wittymodule.CatalogName), raw, 0o644)
		_ = os.WriteFile(filepath.Join(starter, wittymodule.CatalogName+".sig"), signed(key, raw), 0o644)
	}
	ink := webRelease(t, starter, key, "wittyink", "WittyInk", "wittywilla/WittyInk", "0.2.0")
	comics := webRelease(t, starter, key, "wittycomics", "WittyComics", "wittywilla/WittyComics", "0.2.0", "wittyink")
	ebook := webRelease(t, starter, key, "wittyebook", "WittyEbook", "wittywilla/WittyEbook", "0.2.0", "wittyink")
	publish(1, catalogMod("wittyink", "WittyInk", nil, ink), catalogMod("wittycomics", "WittyComics", []string{"wittyink"}, comics), catalogMod("wittyebook", "WittyEbook", []string{"wittyink"}, ebook))
	m := New(Config{Dir: dir, Server: "0.8.0-alpha", Starter: starter, Keys: []ed25519.PublicKey{pub}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	t.Cleanup(m.Close)
	s := NewStores(StoresConfig{Manager: m, Dir: dir, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	return m, s, publish, starter, key
}

func catalogMod(id, name string, requires []string, vers ...wittymodule.CatalogVersion) wittymodule.CatalogModule {
	m := listedModule(id, name)
	m.Repo = "wittywilla/" + name
	m.Requires = requires
	m.Versions = vers
	return m
}

func statusFor(t *testing.T, m *Manager, id string) Status {
	t.Helper()
	for _, st := range m.Report().Modules {
		if st.ID == id {
			return st
		}
	}
	t.Fatalf("no %s", id)
	return Status{}
}

// Every install path refuses it without confirmOlder; the store and the setup's set say why.
func TestOlderServerRefusedOnEveryInstallPath(t *testing.T) {
	m, s, _, _, _ := olderStoreWorld(t)
	st := statusFor(t, m, "wittyink")
	if st.Latest != nil || st.OlderOffer == nil || st.OlderOffer.Version != "0.2.0" {
		t.Fatalf("status: latest %+v older %+v", st.Latest, st.OlderOffer)
	}
	for _, it := range s.View().Items {
		if it.ID == "wittyink" && it.Official {
			if it.Fit.OK || it.Fit.Kind != FitOlderServer || !it.OlderServer || it.MadeFor != "0.6.x" ||
				!strings.HasPrefix(it.Fit.Why, "No version of WittyInk for this server is published yet. WittyInk 0.2.0 was made for WittyMediaServer 0.6") {
				t.Fatalf("the store item: %+v", it)
			}
		}
	}
	plan, err := s.InstallPlan("wittycomics")
	if err != nil || len(plan) != 2 || !plan[0].OlderServer || !plan[1].OlderServer || plan[1].To != "0.2.0" || plan[1].Warning == "" {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	if older := s.OlderInPlan(plan); len(older) != 2 || older[0].Module != "wittyink" {
		t.Fatalf("named: %+v", older)
	}

	var older *ErrOlderServer
	if _, err := s.InstallWithNeeds("wittycomics", "admin"); !errors.As(err, &older) {
		t.Fatalf("store install: %v", err)
	}
	if r := s.CurrentRun(); r != nil {
		t.Fatalf("a run started: %+v", r)
	}
	if err := m.AddModule("wittyink"); !errors.As(err, &older) || older.Name != "WittyInk" || older.Server != "0.8.0-alpha" {
		t.Fatalf("add: %v", err)
	}
	if err := m.Install("wittyink"); !errors.As(err, &older) {
		t.Fatalf("install the latest: %v", err)
	}
	if err := m.InstallVersion("wittyink", "0.2.0", true); !errors.As(err, &older) {
		t.Fatalf("install a version: %v", err)
	}
	if err := m.ToLatest("wittyink"); err != nil {
		t.Fatalf("latest: %v", err)
	}
	if _, err := s.UpdateAll("admin"); err == nil {
		t.Fatal("Update All found something to do")
	}
	s.AutoUpdate()
	m.autoInstall()
	// What first-run setup sends, with nothing confirmed: each one says why, and nothing installs.
	if err := m.SetInstalled([]string{"wittyink", "wittyebook"}, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for statusFor(t, m, "wittyebook").Error == "" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if e := statusFor(t, m, "wittyebook").Error; !strings.Contains(e, "was made for WittyMediaServer 0.6") {
		t.Fatalf("why it wasn't added: %q", e)
	}
	time.Sleep(100 * time.Millisecond)
	for _, id := range []string{"wittyink", "wittycomics", "wittyebook"} {
		if st := statusFor(t, m, id); st.Installed || st.Phase != PhaseIdle {
			t.Fatalf("%s was installed without confirming: %+v", id, st)
		}
	}
}

// Confirmed, each path installs it, and it says so afterwards.
func TestOlderServerConfirmedOnEachPath(t *testing.T) {
	m, s, _, _, _ := olderStoreWorld(t)
	if _, err := s.InstallWithNeedsOlder("wittycomics", "admin", true); err != nil {
		t.Fatal(err)
	}
	if r := waitRun(t, s); r.Updated != 2 || r.Failed != 0 {
		t.Fatalf("the run: %+v", r)
	}
	for _, id := range []string{"wittyink", "wittycomics"} {
		st := statusFor(t, m, id)
		if !st.Installed || st.Version != "0.2.0" || !st.OlderServer || !strings.Contains(st.OlderWarning, "Remove it") {
			t.Fatalf("%s: %+v", id, st)
		}
	}
	if err := m.SetInstalledOlder([]string{"wittyink", "wittycomics", "wittyebook"}, nil, []string{"wittyebook"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for !statusFor(t, m, "wittyebook").Installed && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if st := statusFor(t, m, "wittyebook"); !st.Installed || !st.OlderServer {
		t.Fatalf("setup's confirmed module: %+v", st)
	}
}

// With a version made for this server listed beside an older one, setup's paths take that one.
func TestOlderServerNotUsedWhenANewerFits(t *testing.T) {
	m, s, publish, starter, key := olderStoreWorld(t)
	old := webRelease(t, starter, key, "wittyink", "WittyInk", "wittywilla/WittyInk", "0.2.0")
	fits := webReleaseFor(t, starter, key, "wittyink", "WittyInk", "wittywilla/WittyInk", "0.4.0", "0.8.0-alpha", "0.8.x")
	publish(2, catalogMod("wittyink", "WittyInk", nil, fits, old))
	m.Close()
	m = New(Config{Dir: m.cfg.Dir, Server: "0.8.0-alpha", Starter: starter, Keys: m.keys, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	defer m.Close()
	s = NewStores(StoresConfig{Manager: m, Dir: m.cfg.Dir, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if _, err := s.InstallWithNeeds("wittyink", "admin"); err != nil {
		t.Fatal(err)
	}
	waitRun(t, s)
	if st := statusFor(t, m, "wittyink"); st.Version != "0.4.0" || st.OlderServer {
		t.Fatalf("installed: %+v", st)
	}
}

// From a file, a version made for an older server needs the out-of-range go-ahead, in its words.
func TestOlderServerFromAFile(t *testing.T) {
	w := newWorld(t) // a 0.4.0 server
	m := w.manager()
	defer m.Close()
	man, files := newBundle()
	man.MinServer, man.MaxServer = "0.3.0", "0.3.x"
	path, _, _ := archiveOf(t, t.TempDir(), man, files, nil)
	info := stage(t, m, path, nil, nil, "")
	if info.Refusal != "" || !slices.Contains(info.NeedsOverride.Kinds, OverrideOutOfRange) {
		t.Fatalf("staged: %+v", info)
	}
	said := false
	for _, p := range info.Problems {
		said = said || (p.Kind == OverrideOutOfRange && strings.Contains(p.Text, "was made for WittyMediaServer 0.3; this server is 0.4.0"))
	}
	if !said {
		t.Fatalf("the warning: %+v", info.Problems)
	}
	if err := m.InstallUpload(info.Token, 1, &OverrideAccept{Accept: []string{OverrideUnsigned, OverrideCustom, OverrideLocalFile}, Typed: "wittynew"}); err == nil {
		t.Fatal("installed without the out-of-range go-ahead")
	}
}
