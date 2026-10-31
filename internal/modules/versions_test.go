package modules

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

// wait follows a background install to its end.
func wait(t *testing.T, m *Manager) Status {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		s := one(t, m)
		if s.Phase == PhaseIdle || s.Phase == PhaseFailed {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the install never finished")
	return Status{}
}

// choose installs a version, having read the warning.
func choose(t *testing.T, m *Manager, ver string) Status {
	t.Helper()
	if err := m.InstallVersion("wittyphotos", ver, true); err != nil {
		t.Fatal(err)
	}
	return wait(t, m)
}

func find(t *testing.T, l VersionList, ver string) VersionInfo {
	t.Helper()
	for _, v := range l.Versions {
		if v.Version == ver {
			return v
		}
	}
	t.Fatalf("%s isn't listed: %+v", ver, l.Versions)
	return VersionInfo{}
}

func versions(t *testing.T, m *Manager) VersionList {
	t.Helper()
	l, err := m.Versions("wittyphotos")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// withdraw publishes a signed list of withdrawn versions on the fake GitHub.
func (w *world) withdraw(id string, serial int, key ed25519.PrivateKey, vs ...wittymodule.Withdrawn) {
	w.t.Helper()
	b, err := json.Marshal(wittymodule.Withdrawals{Module: id, Serial: serial, Versions: vs})
	if err != nil {
		w.t.Fatal(err)
	}
	const base = "/wittywilla/WittyPhotos/releases/download/withdrawals/"
	name := wittymodule.WithdrawalsName("wittyphotos")
	w.gh.set(base+name, b)
	w.gh.set(base+name+".sig", update.Sign(key, b))
	g := update.GHRelease{TagName: wittymodule.WithdrawalsTag, Name: "Withdrawn versions"}
	for _, n := range []string{name, name + ".sig"} {
		g.Assets = append(g.Assets, struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
			URL  string `json:"browser_download_url"`
		}{n, 1, "https://github.com" + base + n})
	}
	w.gh.mu.Lock()
	defer w.gh.mu.Unlock()
	for i, r := range w.gh.rels {
		if r.TagName == wittymodule.WithdrawalsTag {
			w.gh.rels[i] = g
			return
		}
	}
	w.gh.rels = append(w.gh.rels, g)
}

func threeVersions(w *world) {
	for _, v := range []string{"0.2.0", "0.3.0", "0.4.0"} {
		w.publish(v, manifestJSON(v, "0.4.0", "0.5.x", 1))
	}
}

// Every version is listed; one that doesn't fit is there and explained, not hidden,
// and can't be chosen.
func TestVersionsAreListedAndExplained(t *testing.T) {
	w := newWorld(t)
	threeVersions(w)
	w.publish("0.5.0", manifestJSON("0.5.0", "0.9.0", "0.9.x", 1))
	m := w.manager()
	defer m.Close()
	check(t, m)
	l := versions(t, m)
	if len(l.Versions) != 5 { // the four releases and the built-in copy
		t.Fatalf("%+v", l.Versions)
	}
	if v := find(t, l, "0.5.0"); v.Allowed || !strings.Contains(v.Refused, "0.9.0 or newer") {
		t.Fatalf("incompatible: %+v", v)
	}
	if err := m.InstallVersion("wittyphotos", "0.5.0", true); err == nil || !strings.Contains(err.Error(), "0.9.0 or newer") {
		t.Fatalf("installed something incompatible: %v", err)
	}
	if v := find(t, l, builtinVersion); !v.Builtin || !v.InUse {
		t.Fatalf("built-in: %+v", v)
	}
	if l.Versions[0].Version != "0.5.0" {
		t.Fatalf("not newest first: %+v", l.Versions)
	}
}

// Listing versions goes online only when a check does.
func TestListingVersionsDoesntGoOnline(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	versions(t, m)
	m.Report()
	w.gh.mu.Lock()
	n := w.gh.requests
	w.gh.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d requests", n)
	}
}

// The whole cycle: update, go back to an older version and have it stay, survive a
// restart and an automatic update, then return to the latest.
func TestDowngradePinAndReturnToLatest(t *testing.T) {
	w := newWorld(t)
	threeVersions(w)
	auto := true
	newManager := func() *Manager {
		return New(Config{Dir: w.dir, Server: w.server, Builtins: [][]byte{manifestJSON(builtinVersion, "0.1.0", "0.x", 1)},
			Keys: []ed25519.PublicKey{w.pub}, Fetch: &update.Fetcher{UserAgent: "test", Transport: w.gh},
			AutoInstall: func() bool { return auto }, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	}
	m := newManager()
	check(t, m)
	if s := installWait(t, m); s.Version != "0.4.0" {
		t.Fatalf("%+v", s)
	}
	usesHelper(t, m, "0.4.0")

	// An older version asks for confirmation, and says why.
	err := m.InstallVersion("wittyphotos", "0.2.0", false)
	var need *ErrNeedsConfirm
	if !errors.As(err, &need) || !strings.Contains(need.Warning, "older") {
		t.Fatalf("no warning: %v", err)
	}
	if s := one(t, m); s.Version != "0.4.0" {
		t.Fatalf("moved without confirming: %+v", s)
	}
	s := choose(t, m, "0.2.0")
	if s.Version != "0.2.0" || s.Pin != "0.2.0" {
		t.Fatalf("after going back: %+v", s)
	}
	usesHelper(t, m, "0.2.0")

	// Automatic updates leave it alone, though the newer one is still offered.
	check(t, m)
	m.autoInstall()
	if s := wait(t, m); s.Version != "0.2.0" || s.Latest == nil || s.Latest.Version != "0.4.0" {
		t.Fatalf("after an automatic update: %+v", s)
	}
	m.Close()

	// And across a restart.
	m = newManager()
	if s := one(t, m); s.Version != "0.2.0" || s.Pin != "0.2.0" || s.State != StateInstalled {
		t.Fatalf("after a restart: %+v", s)
	}
	usesHelper(t, m, "0.2.0")
	m.Check(context.Background(), 0)
	m.autoInstall()
	if s := wait(t, m); s.Version != "0.2.0" {
		t.Fatalf("an automatic update after a restart: %+v", s)
	}

	// Back to the latest, which also ends the pin.
	if err := m.ToLatest("wittyphotos"); err != nil {
		t.Fatal(err)
	}
	if s := wait(t, m); s.Version != "0.4.0" || s.Pin != "" {
		t.Fatalf("back at the latest: %+v", s)
	}
	usesHelper(t, m, "0.4.0")
	m.Close()
}

// Going back to the previous version takes no download, so it works offline.
func TestSwitchingToWhatIsInstalledNeedsNoNetwork(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	w.publish("0.3.0", manifestJSON("0.3.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	check(t, m)
	choose(t, m, "0.2.0")
	choose(t, m, "0.3.0")
	w.gh.mu.Lock()
	w.gh.offline = true
	w.gh.mu.Unlock()
	if s := choose(t, m, "0.2.0"); s.Version != "0.2.0" || s.Phase != PhaseIdle {
		t.Fatalf("offline: %+v", s)
	}
	usesHelper(t, m, "0.2.0")
}

// Pinning what is in use keeps it, with no downgrade involved.
func TestPinAndUnpin(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	check(t, m)
	if err := m.Pin("wittyphotos"); err != nil {
		t.Fatal(err)
	}
	if s := one(t, m); s.Pin != PinBuiltin {
		t.Fatalf("%+v", s)
	}
	m.autoInstall()
	if s := wait(t, m); s.Version != builtinVersion {
		t.Fatalf("installed past a pin on the built-in copy: %+v", s)
	}
	if err := m.Unpin("wittyphotos"); err != nil {
		t.Fatal(err)
	}
	m.autoInstall()
	if s := wait(t, m); s.Version != "0.2.0" {
		t.Fatalf("after unpinning: %+v", s)
	}
}

// Someone can go back to the built-in copy without blaming the version in use.
func TestUseBuiltinKeepsTheOtherVersionChoosable(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	check(t, m)
	installWait(t, m)
	if err := m.UseBuiltin("wittyphotos"); err != nil {
		t.Fatal(err)
	}
	usesBuiltin(t, m)
	if v := find(t, versions(t, m), "0.2.0"); !v.Allowed || v.Blocked != "" {
		t.Fatalf("%+v", v)
	}
	m.Close()
	m = w.manager() // it stays that way after a restart
	defer m.Close()
	usesBuiltin(t, m)
	choose(t, m, "0.2.0")
	usesHelper(t, m, "0.2.0")
}

// A release with a higher version can be chosen, but only a version the server's
// data can survive.
func TestDataFormatRefusesWhatCantReadTheData(t *testing.T) {
	w := newWorld(t)
	man := func(ver string, format, from int) []byte {
		var m wittymodule.Manifest
		if err := json.Unmarshal(manifestJSON(ver, "0.4.0", "0.5.x", 1), &m); err != nil {
			t.Fatal(err)
		}
		m.DataFormat, m.DataFrom = format, from
		b, _ := json.Marshal(m)
		return b
	}
	w.publish("0.2.0", man("0.2.0", 1, 1))
	w.publish("0.3.0", man("0.3.0", 2, 1))
	w.publish("0.4.0", man("0.4.0", 3, 2))
	m := w.manager()
	check(t, m)
	choose(t, m, "0.3.0") // reads 1 and 2, and the highest so far becomes 2
	v := find(t, versions(t, m), "0.2.0")
	if v.Allowed || !strings.Contains(v.Refused, "data format 2") {
		t.Fatalf("0.2.0 can't read format 2: %+v", v)
	}
	if err := m.InstallVersion("wittyphotos", "0.2.0", true); err == nil || !strings.Contains(err.Error(), "lose data") {
		t.Fatalf("%v", err)
	}
	if s := one(t, m); s.Version != "0.3.0" {
		t.Fatalf("%+v", s)
	}
	choose(t, m, "0.4.0")
	m.Close()

	// The refusal isn't only at the moment of choosing: a version that can't read the
	// data doesn't run, whatever the state says.
	m = w.manager()
	defer m.Close()
	if s := one(t, m); s.Version != "0.4.0" {
		t.Fatalf("%+v", s)
	}
	if v := find(t, versions(t, m), "0.3.0"); v.Allowed {
		t.Fatalf("0.3.0 can't read format 3: %+v", v)
	}
}

// A version withdrawn for security reasons can't be installed, and stops being used.
func TestSecurityWithdrawalStopsAVersionEvenWhenPinned(t *testing.T) {
	w := newWorld(t)
	threeVersions(w)
	m := w.manager()
	defer m.Close()
	check(t, m)
	choose(t, m, "0.3.0")
	choose(t, m, "0.2.0") // older, so pinned, with 0.3.0 before it
	if s := one(t, m); s.Pin != "0.2.0" {
		t.Fatalf("%+v", s)
	}
	w.withdraw("wittyphotos", 1, w.key, wittymodule.Withdrawn{Version: "0.2.0", Severity: wittymodule.SeveritySecurity, Reason: "it reads past a buffer"})
	s := check(t, m)
	if s.Version == "0.2.0" || !strings.Contains(s.Notice, "it reads past a buffer") {
		t.Fatalf("still in use: %+v", s)
	}
	v := find(t, versions(t, m), "0.2.0")
	if v.Allowed || !strings.Contains(v.Refused, "security") {
		t.Fatalf("%+v", v)
	}
	if err := m.InstallVersion("wittyphotos", "0.2.0", true); err == nil || !strings.Contains(err.Error(), "security") {
		t.Fatalf("installed: %v", err)
	}
}

// A version withdrawn because it is broken gets a warning, and is never installed by
// itself.
func TestBrokenWithdrawalWarnsAndIsNotAutomatic(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	w.publish("0.3.0", manifestJSON("0.3.0", "0.4.0", "0.5.x", 1))
	w.withdraw("wittyphotos", 1, w.key, wittymodule.Withdrawn{Version: "0.3.0", Severity: wittymodule.SeverityBroken, Reason: "thumbnails come out sideways"})
	m := w.manager()
	defer m.Close()
	s := check(t, m)
	if s.Latest == nil || s.Latest.Version != "0.2.0" {
		t.Fatalf("offered: %+v", s.Latest)
	}
	v := find(t, versions(t, m), "0.3.0")
	if !v.Allowed || !strings.Contains(v.Warning, "sideways") || v.Withdrawn == nil {
		t.Fatalf("%+v", v)
	}
	var need *ErrNeedsConfirm
	if err := m.InstallVersion("wittyphotos", "0.3.0", false); !errors.As(err, &need) {
		t.Fatalf("%v", err)
	}
}

// The list is believed only if it is signed, newer than the last and for this plugin.
func TestWithdrawalListCantBeForgedReplayedOrBorrowed(t *testing.T) {
	sec := wittymodule.Withdrawn{Version: "0.2.0", Severity: wittymodule.SeveritySecurity, Reason: "bad"}
	cases := map[string]func(w *world, stranger ed25519.PrivateKey){
		"signed by a stranger": func(w *world, stranger ed25519.PrivateKey) { w.withdraw("wittyphotos", 1, stranger, sec) },
		"for another plugin":   func(w *world, _ ed25519.PrivateKey) { w.withdraw("wittydvd", 1, w.key, sec) },
	}
	for name, plant := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
			_, stranger, _ := ed25519.GenerateKey(rand.Reader)
			plant(w, stranger)
			m := w.manager()
			defer m.Close()
			check(t, m)
			if v := find(t, versions(t, m), "0.2.0"); !v.Allowed || v.Withdrawn != nil {
				t.Fatalf("believed it: %+v", v)
			}
			if m.Report().CheckError == "" {
				t.Fatal("a bad list should be mentioned")
			}
		})
	}

	// An older signed list can't bring a withdrawn version back, and hiding the list
	// doesn't either.
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	w.withdraw("wittyphotos", 5, w.key, sec)
	m := w.manager()
	check(t, m)
	w.withdraw("wittyphotos", 4, w.key)
	check(t, m)
	if v := find(t, versions(t, m), "0.2.0"); v.Allowed {
		t.Fatalf("replayed: %+v", v)
	}
	w.gh.mu.Lock()
	w.gh.rels = w.gh.rels[:1]
	w.gh.mu.Unlock()
	check(t, m)
	if v := find(t, versions(t, m), "0.2.0"); v.Allowed {
		t.Fatalf("hidden: %+v", v)
	}
	m.Close()
	m = w.manager()
	defer m.Close()
	if v := find(t, versions(t, m), "0.2.0"); v.Allowed {
		t.Fatalf("forgotten across a restart: %+v", v)
	}
	// A newer list may restore it: the owner changed their mind.
	w.withdraw("wittyphotos", 6, w.key)
	check(t, m)
	if v := find(t, versions(t, m), "0.2.0"); !v.Allowed {
		t.Fatalf("not restored: %+v", v)
	}
}

// The list of versions survives without a network, and asks nothing of GitHub when
// nothing has changed.
func TestFeedIsRememberedAndConditional(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	check(t, m)
	check(t, m)
	w.gh.mu.Lock()
	api, full := w.gh.apiCalls, w.gh.full
	w.gh.mu.Unlock()
	if api != 2 || full != 1 {
		t.Fatalf("%d calls, %d full", api, full)
	}
	m.Close()

	w.gh.mu.Lock()
	w.gh.offline = true
	w.gh.mu.Unlock()
	m = w.manager()
	defer m.Close()
	if v := find(t, versions(t, m), "0.2.0"); !v.Allowed {
		t.Fatalf("offline: %+v", v)
	}
	if s := one(t, m); s.Latest == nil || s.Latest.Version != "0.2.0" {
		t.Fatalf("offline: %+v", s)
	}

	// A list on disk that someone changed is not believed.
	path := filepath.Join(w.dir, "wittyphotos", feedFile)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var c cachedFeed
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	c.Releases[0].RawMan = manifestJSON("0.2.0", "0.1.0", "9.x", 1)
	b, _ = json.Marshal(c)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	m2 := w.manager()
	defer m2.Close()
	for _, v := range versions(t, m2).Versions {
		if v.Version == "0.2.0" {
			t.Fatalf("believed a changed manifest: %+v", v)
		}
	}
}

// When GitHub says wait, everything waits, and the page says until when.
func TestRateLimitHoldsOff(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	w.gh.mu.Lock()
	w.gh.limited = true
	w.gh.mu.Unlock()
	r := m.Check(context.Background(), 0)
	if r.HoldUntil == 0 || !strings.Contains(r.CheckError, "limiting") {
		t.Fatalf("%+v", r)
	}
	w.gh.mu.Lock()
	n := w.gh.requests
	w.gh.limited = false
	w.gh.mu.Unlock()
	m.Check(context.Background(), 0)
	w.gh.mu.Lock()
	after := w.gh.requests
	w.gh.mu.Unlock()
	if after != n {
		t.Fatalf("asked again during the wait: %d then %d", n, after)
	}
}

// One plugin's release can't be passed off as another's, even properly signed.
func TestReleaseMustBelongToThisModule(t *testing.T) {
	for name, edit := range map[string]func(m *wittymodule.Manifest){
		"another repository": func(m *wittymodule.Manifest) { m.Repo = "someone/WittyPhotos" },
		"another plugin":     func(m *wittymodule.Manifest) { m.ID = "wittydvd" },
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			var man wittymodule.Manifest
			if err := json.Unmarshal(manifestJSON("0.2.0", "0.4.0", "0.5.x", 1), &man); err != nil {
				t.Fatal(err)
			}
			edit(&man)
			b, _ := json.Marshal(man)
			w.publish("0.2.0", b)
			m := w.manager()
			defer m.Close()
			if s := check(t, m); s.Latest != nil {
				t.Fatalf("offered: %+v", s.Latest)
			}
		})
	}
}

// A delivery kind can't change: a helper's release can't become a bundle in browsers.
func TestDeliveryMustMatch(t *testing.T) {
	w := newWorld(t)
	var man wittymodule.Manifest
	if err := json.Unmarshal(manifestJSON("0.2.0", "0.4.0", "0.5.x", 1), &man); err != nil {
		t.Fatal(err)
	}
	man.Delivery, man.Entry = wittymodule.DeliveryWeb, "index.js"
	b, _ := json.Marshal(man)
	w.publish("0.2.0", b)
	m := w.manager()
	defer m.Close()
	check(t, m)
	v := find(t, versions(t, m), "0.2.0")
	if v.Allowed || !strings.Contains(v.Refused, "delivered") {
		t.Fatalf("%+v", v)
	}
}

// An attached helper keeps its state between calls for as long as the job needs it.
func TestAttachedHelper(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	if a := m.Attach("wittyphotos", 0); a != nil {
		t.Fatal("the built-in copy has no helper")
	}
	check(t, m)
	installWait(t, m)
	a := m.Attach("wittyphotos", time.Minute)
	if a == nil || a.Version() != "0.2.0" {
		t.Fatalf("%+v", a)
	}
	var in map[string]any
	if _, err := a.Call("read", pathParams{"x.jpg"}, &in); err != nil || in["Make"] != "fake 0.2.0" {
		t.Fatalf("%v %v", in, err)
	}
	a.p.mu.Lock()
	if a.p.idle != nil {
		t.Error("an attached helper must not stop when idle")
	}
	a.p.mu.Unlock()
	a.Close()
	m.mu.Lock()
	n := len(m.attached)
	m.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d left", n)
	}
}

// Browsers can report a bundle failing, but only about the version in use.
func TestReportFailureRollsBack(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	check(t, m)
	installWait(t, m)
	for range failLimit {
		if err := m.ReportFailure("wittyphotos", "0.1.5", "an old tab"); err != nil {
			t.Fatal(err)
		}
	}
	if s := one(t, m); s.Version != "0.2.0" {
		t.Fatalf("a report about another version counted: %+v", s)
	}
	for i := range failLimit {
		if err := m.ReportFailure("wittyphotos", "0.2.0", fmt.Sprint("failure ", i)); err != nil {
			t.Fatal(err)
		}
	}
	s := one(t, m)
	if s.Version != builtinVersion || !strings.Contains(s.Notice, "kept failing") || s.LastFail == "" {
		t.Fatalf("%+v", s)
	}
	var kinds []string
	for _, ev := range s.Events {
		kinds = append(kinds, ev.Kind)
	}
	if !strings.Contains(strings.Join(kinds, " "), "taken-out-of-use") {
		t.Fatalf("no record of it: %v", kinds)
	}
}

// A web bundle: one package for every platform, served only by its signed names.
func TestWebBundle(t *testing.T) {
	w := newWorld(t)
	const id = "wittyphotos" // the fake GitHub knows this one; the id doesn't matter here
	man := wittymodule.Manifest{ID: id, Name: "WittyPhotos", Version: "0.2.0", Repo: "wittywilla/WittyPhotos", Delivery: wittymodule.DeliveryWeb, Entry: "index.js", Style: "style.css", API: 2, MinServer: "0.4.0", MaxServer: "0.5.x"}
	raw, _ := json.Marshal(man)
	top := wittymodule.WebTopFolder(id, "0.2.0")
	files := map[string][]byte{wittymodule.ManifestFile: raw, "index.js": []byte("export default 1"), "style.css": []byte("a{}"), "chunk-a.js": []byte("export const a = 1")}
	archive := webPack(t, top, files)
	archiveName := wittymodule.WebArchiveName(id, "0.2.0")
	manName := wittymodule.ManifestName(id, "0.2.0")
	var list strings.Builder
	fmt.Fprintf(&list, "%s  %s\n%s  %s\n", sha(archive), archiveName, sha(raw), manName)
	for n, b := range files {
		fmt.Fprintf(&list, "%s  %s/%s\n", sha(b), top, n)
	}
	sums := []byte(list.String())
	sumsName := wittymodule.SumsName(id, "0.2.0")
	base := "/wittywilla/WittyPhotos/releases/download/v0.2.0/"
	g := update.GHRelease{TagName: "v0.2.0"}
	for n, b := range map[string][]byte{archiveName: archive, manName: raw, sumsName: sums, sumsName + ".sig": update.Sign(w.key, sums)} {
		w.gh.set(base+n, b)
		g.Assets = append(g.Assets, struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
			URL  string `json:"browser_download_url"`
		}{n, int64(len(b)), "https://github.com" + base + n})
	}
	w.gh.rels = []update.GHRelease{g}
	builtin, _ := json.Marshal(wittymodule.Manifest{ID: id, Name: "WittyPhotos", Version: "0.1.0", Repo: "wittywilla/WittyPhotos", Delivery: wittymodule.DeliveryWeb, API: 2, MinServer: "0.1.0", MaxServer: "0.x"})
	newM := func() *Manager {
		return New(Config{Dir: w.dir, Server: w.server, Builtins: [][]byte{builtin}, Keys: []ed25519.PublicKey{w.pub},
			Fetch: &update.Fetcher{UserAgent: "test", Transport: w.gh}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	}
	m := newM()
	if b := m.WebBundles(); len(b) != 0 {
		t.Fatalf("%+v", b)
	}
	m.Check(context.Background(), 0)
	if s := installWait(t, m); s.Version != "0.2.0" || s.Error != "" {
		t.Fatalf("%+v", s)
	}
	b := m.WebBundles()
	if len(b) != 1 || b[0].Entry != "index.js" || !strings.HasPrefix(b[0].Base, "/api/modules/wittyphotos/web/0.2.0~") {
		t.Fatalf("%+v", b)
	}
	addr := strings.TrimSuffix(strings.TrimPrefix(b[0].Base, "/api/modules/wittyphotos/web/"), "/")
	p, h, err := m.WebFile(id, addr, "chunk-a.js")
	if err != nil || h != sha(files["chunk-a.js"]) || !strings.HasSuffix(filepath.ToSlash(p), "chunk-a.js") {
		t.Fatalf("%q %q %v", p, h, err)
	}
	for _, c := range [][2]string{{addr, "../state.json"}, {addr, "..\\..\\state.json"}, {addr, "SHA256SUMS.txt"}, {addr, wittymodule.ManifestFile},
		{addr, "nothing.js"}, {"0.1.9", "index.js"}, {"0.2.0", "index.js"}, {addr, ""}, {addr, "/etc/passwd"}} {
		if _, _, err := m.WebFile(id, c[0], c[1]); !errors.Is(err, ErrNoFile) {
			t.Errorf("%v: %v", c, err)
		}
	}
	m.Close()

	// A file changed after installing is refused, here and on the next start.
	m = newM()
	defer m.Close()
	if _, _, err := m.WebFile(id, addr, "index.js"); err != nil {
		t.Fatalf("after a restart: %v", err)
	}
	// The same size, and the same date: it is the hash that sees it.
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, bytes.Repeat([]byte("x"), int(st.Size())), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.WebFile(id, addr, "chunk-a.js"); !errors.Is(err, ErrNoFile) {
		t.Fatalf("served a changed file: %v", err)
	}
	if s := one(t, m); s.Version != "0.1.0" || len(m.WebBundles()) != 0 {
		t.Fatalf("still in use: %+v", s)
	}
}

// webPack makes a bundle's archive, which is a .tar.gz on every system.
func webPack(t *testing.T, top string, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for n, b := range files {
		if err := tw.WriteHeader(&tar.Header{Name: top + "/" + n, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(b))}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write(b)
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func storeManager(t *testing.T, w *world) *Manager {
	t.Helper()
	m := func(id, name string, extra string) []byte {
		return []byte(`{"id":"` + id + `","name":"` + name + `","version":"0.1.0","repo":"wittywilla/WittyMediaServer","delivery":"builtin","api":1,"minServer":"0.1.0","maxServer":"0.x",` + extra + `}`)
	}
	return New(Config{Dir: w.dir, Server: w.server, Builtins: [][]byte{
		m("wittyvideo", "WittyVideo", `"libraries":["movies","shows"]`),
		m("wittydvd", "WittyDVD", `"libraries":["discs"],"requires":["wittydisc"]`),
		m("wittybluray", "WittyBluray", `"libraries":["discs"],"requires":["wittydisc"]`),
		m("wittydisc", "WittyDisc", `"core":true`),
	}, Keys: []ed25519.PublicKey{w.pub}, Fetch: &update.Fetcher{UserAgent: "test", Transport: w.gh}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
}

func available(m *Manager, libType string) bool { return m.LibraryAllowed(libType) == nil }

// What is installed decides what libraries can be made, across a restart. With two
// modules able to play discs, either is enough.
func TestInstalledModulesLimitLibraries(t *testing.T) {
	w := newWorld(t)
	m := storeManager(t, w)
	if !available(m, "movies") || !available(m, "discs") || !available(m, "anything-else") {
		t.Fatal("a server that hasn't chosen has everything")
	}
	if err := m.RemoveModule("wittydvd", nil); err != nil {
		t.Fatal(err)
	}
	if !available(m, "discs") {
		t.Fatal("WittyBluray still makes discs possible")
	}
	if err := m.RemoveModule("wittybluray", nil); err != nil {
		t.Fatal(err)
	}
	err := m.LibraryAllowed("discs")
	var missing *ErrModuleMissing
	if !errors.As(err, &missing) || len(missing.Modules) != 2 || !strings.Contains(err.Error(), "WittyBluray or WittyDVD") {
		t.Fatalf("%v", err)
	}
	m.Close()

	m = storeManager(t, w)
	defer m.Close()
	if available(m, "discs") || m.Installed("wittydvd") {
		t.Fatal("forgotten across a restart")
	}
	if err := m.AddModule("wittydvd"); err != nil {
		t.Fatal(err)
	}
	if !available(m, "discs") {
		t.Fatal("adding it should work straight away")
	}
}

// A module can't be removed while libraries depend on it, and what they're called is
// the reason given.
func TestModuleInUseStays(t *testing.T) {
	w := newWorld(t)
	m := storeManager(t, w)
	defer m.Close()
	libs := map[string][]string{"movies": {"Films", "Kids films"}, "discs": {"Discs"}}
	err := m.RemoveModule("wittyvideo", libs)
	var inUse *ErrInUse
	if !errors.As(err, &inUse) || !strings.Contains(err.Error(), "Films, Kids films") {
		t.Fatalf("%v", err)
	}
	if !m.Installed("wittyvideo") {
		t.Fatal("removed anyway")
	}
	// WittyBluray can still play them, so WittyDVD may go, but not both.
	if err := m.RemoveModule("wittydvd", libs); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveModule("wittybluray", libs); !errors.As(err, &inUse) {
		t.Fatalf("%v", err)
	}
	if err := m.RemoveModule("wittydisc", libs); err == nil || !strings.Contains(err.Error(), "built on") {
		t.Fatalf("%v", err)
	}
}

// The first-run choice: everything left out goes, core modules stay, and a choice that
// strands a library changes nothing it shouldn't.
func TestSetInstalled(t *testing.T) {
	w := newWorld(t)
	m := storeManager(t, w)
	defer m.Close()
	if err := m.SetInstalled([]string{"wittydvd"}, nil); err != nil {
		t.Fatal(err)
	}
	if available(m, "movies") || !available(m, "discs") || !m.Installed("wittydisc") {
		t.Fatalf("%+v", m.LibraryTypes())
	}
	if err := m.SetInstalled([]string{"nothing"}, nil); !errors.Is(err, ErrUnknown) {
		t.Fatalf("%v", err)
	}
	var inUse *ErrInUse
	if err := m.SetInstalled([]string{"wittyvideo"}, map[string][]string{"discs": {"Discs"}}); !errors.As(err, &inUse) {
		t.Fatalf("%v", err)
	}
}

// A module taken off this server asks nobody anything, and can't be installed until
// it is added back.
func TestRemovedModuleStaysQuiet(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	check(t, m)
	installWait(t, m)
	if err := m.RemoveModule("wittyphotos", nil); err != nil {
		t.Fatal(err)
	}
	usesBuiltin(t, m)
	if w.installed("0.2.0") {
		t.Fatal("its downloaded versions should go")
	}
	w.gh.mu.Lock()
	before := w.gh.requests
	w.gh.mu.Unlock()
	m.Check(context.Background(), 0)
	w.gh.mu.Lock()
	after := w.gh.requests
	w.gh.mu.Unlock()
	if after != before {
		t.Fatalf("%d requests for a removed module", after-before)
	}
	if err := m.InstallVersion("wittyphotos", "0.2.0", true); err == nil || !strings.Contains(err.Error(), "add it first") {
		t.Fatalf("%v", err)
	}
}

// A fork takes a stock module's place: its releases come from its repository, are held
// to its key alone, and everything else works the same.
func TestForkTakesAModulesPlace(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1)) // the stock release
	m := w.manager()
	defer m.Close()
	check(t, m)
	installWait(t, m)

	forkPub, forkKey, _ := ed25519.GenerateKey(rand.Reader)
	keyText := base64.StdEncoding.EncodeToString(forkPub)
	const repo = "someone/WittyPhotos"

	// Nothing changes until someone has read the warning.
	var need *ErrNeedsConfirm
	if err := m.SetSource("wittyphotos", repo, keyText, "admin", false); !errors.As(err, &need) || !strings.Contains(need.Warning, "trust") || !strings.Contains(need.Warning, Fingerprint(forkPub)) {
		t.Fatalf("%v", err)
	}
	if s := one(t, m); s.Source != nil || s.Version != "0.2.0" {
		t.Fatalf("%+v", s)
	}
	for _, bad := range [][2]string{{"not a repo", keyText}, {repo, "not a key"}, {repo, ""}, {"../../x", keyText}} {
		if err := m.SetSource("wittyphotos", bad[0], bad[1], "admin", true); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
	if err := m.SetSource("wittyphotos", repo, keyText, "admin", true); err != nil {
		t.Fatal(err)
	}
	// What the stock key signed goes, and the built-in copy carries on.
	s := one(t, m)
	if s.Version != builtinVersion || s.Repo != repo || s.StockRepo != "wittywilla/WittyPhotos" || s.Source == nil || s.Source.Fingerprint != Fingerprint(forkPub) {
		t.Fatalf("%+v", s)
	}
	if w.installed("0.2.0") {
		t.Fatal("the other publisher's files should go")
	}

	// A release signed with the stock key isn't taken from the fork's feed.
	w.gh.mu.Lock()
	w.gh.rels = nil
	w.gh.mu.Unlock()
	forged := func(ver string, key ed25519.PrivateKey) {
		var man wittymodule.Manifest
		if err := json.Unmarshal(manifestJSON(ver, "0.4.0", "0.5.x", 1), &man); err != nil {
			t.Fatal(err)
		}
		man.Repo = repo
		b, _ := json.Marshal(man)
		old := w.key
		w.key = key
		w.publish(ver, b)
		w.key = old
	}
	forged("0.3.0", w.key)
	if s := check(t, m); s.Latest != nil {
		t.Fatalf("took a release its key didn't sign: %+v", s.Latest)
	}
	forged("0.4.0", forkKey)
	s = check(t, m)
	if s.Latest == nil || s.Latest.Version != "0.4.0" {
		t.Fatalf("%+v %s", s, m.Report().CheckError)
	}

	// It never updates by itself, however the settings are, and the switch can't say so.
	if err := m.SetAutoUpdate("wittyphotos", true); err == nil {
		t.Fatal("a fork can't be set to update by itself")
	}
	if s := one(t, m); s.AutoUpdate {
		t.Fatalf("%+v", s)
	}
	m.autoInstall()
	if s := wait(t, m); s.Version != builtinVersion {
		t.Fatalf("installed a fork by itself: %+v", s)
	}
	// But it can be chosen, and then it works like any other, across a restart.
	if s := choose(t, m, "0.4.0"); s.Version != "0.4.0" || s.Error != "" {
		t.Fatalf("%+v", s)
	}
	usesHelper(t, m, "0.4.0")
	m.mu.Lock()
	network := m.mods["wittyphotos"].proc.network
	m.mu.Unlock()
	if network {
		t.Fatal("a fork must not have a network")
	}
	m.Close()
	m = w.manager()
	defer m.Close()
	if s := one(t, m); s.Version != "0.4.0" || s.Source == nil {
		t.Fatalf("after a restart: %+v", s)
	}
	usesHelper(t, m, "0.4.0")

	// And back to the module's own repository.
	if err := m.ClearSource("wittyphotos"); err != nil {
		t.Fatal(err)
	}
	if s := one(t, m); s.Source != nil || s.Repo != "wittywilla/WittyPhotos" || s.Version != builtinVersion {
		t.Fatalf("%+v", s)
	}
}

// A fork's key is good for its one module, and a state file edited to say otherwise gets
// nothing: only a key on file passes, and an unreadable one means nothing does.
func TestForkKeyInAnEditedStateFile(t *testing.T) {
	w := newWorld(t)
	m := w.manager()
	m.mu.Lock()
	e := m.mods["wittyphotos"]
	e.state.Source = &Source{Repo: "someone/WittyPhotos", Key: "garbage"}
	keys := m.keysFor(e)
	m.mu.Unlock()
	if len(keys) != 0 {
		t.Fatalf("%v", keys)
	}
	m.Close()
}

// WittyTuner holds the line on encrypted broadcasts, and a fork could cross it unseen.
func TestWittyTunerCantBeForked(t *testing.T) {
	w := newWorld(t)
	raw := []byte(`{"id":"wittytuner","name":"WittyTuner","version":"0.1.0","repo":"wittywilla/WittyTuner","delivery":"helper","entry":"wittytuner-module","api":2,"minServer":"0.1.0","maxServer":"0.x"}`)
	m := New(Config{Dir: w.dir, Server: w.server, Builtins: [][]byte{raw}, Keys: []ed25519.PublicKey{w.pub}, Fetch: &update.Fetcher{UserAgent: "test", Transport: w.gh}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	defer m.Close()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	err := m.SetSource("wittytuner", "someone/WittyTuner", base64.StdEncoding.EncodeToString(pub), "admin", true)
	if err == nil || !strings.Contains(err.Error(), "encrypted") {
		t.Fatalf("%v", err)
	}
	if s := one(t, m); s.NoFork == "" || s.Source != nil {
		t.Fatalf("%+v", s)
	}
}

// A module's own switch: it stays on its version when automatic updates run, still offers
// what is new, and can be updated by hand. A part of the server has no such switch.
func TestModuleCanLeaveItsUpdatesToYou(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	w.publish("0.3.0", manifestJSON("0.3.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	check(t, m)
	if s := one(t, m); !s.AutoUpdate {
		t.Fatalf("on by default: %+v", s)
	}
	if err := m.SetAutoUpdate("wittyphotos", false); err != nil {
		t.Fatal(err)
	}
	m.autoInstall()
	if s := wait(t, m); s.Version != builtinVersion || s.AutoUpdate || s.Latest == nil {
		t.Fatalf("should stay, and still offer: %+v", s)
	}
	m.Close()
	m = w.manager() // it survives a restart
	defer m.Close()
	m.Check(context.Background(), 0)
	m.autoInstall()
	if s := wait(t, m); s.Version != builtinVersion || s.AutoUpdate {
		t.Fatalf("after a restart: %+v", s)
	}
	// By hand still works, and unlike a pin this doesn't hold a version.
	if s := choose(t, m, "0.2.0"); s.Version != "0.2.0" || s.Pin != "" {
		t.Fatalf("by hand: %+v", s)
	}
	if err := m.SetAutoUpdate("wittyphotos", true); err != nil {
		t.Fatal(err)
	}
	m.autoInstall()
	if s := wait(t, m); s.Version != "0.3.0" {
		t.Fatalf("back on: %+v", s)
	}
	if err := m.SetAutoUpdate("nothing", false); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
}

func TestBuiltinPartsHaveNoAutoUpdateSwitch(t *testing.T) {
	w := newWorld(t)
	m := storeManager(t, w)
	defer m.Close()
	if err := m.SetAutoUpdate("wittyvideo", false); err == nil || !strings.Contains(err.Error(), "updates with it") {
		t.Fatalf("%v", err)
	}
	for _, s := range m.Report().Modules {
		if s.AutoUpdate {
			t.Errorf("%s says it updates by itself", s.ID)
		}
	}
}

// A release that fails its signature costs one look, and fake tags can't hide what is known.
func TestFakeReleasesCantHideRealOnes(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	if s := check(t, m); s.Latest == nil || s.Latest.Version != "0.2.0" {
		t.Fatalf("%+v", s)
	}
	_, stranger, _ := ed25519.GenerateKey(rand.Reader)
	real := w.key
	w.key = stranger
	for i := range maxNewChecks + 3 {
		v := fmt.Sprintf("9.0.%d", i)
		w.publish(v, manifestJSON(v, "0.4.0", "0.5.x", 1))
	}
	w.key = real
	if s := check(t, m); s.Latest == nil || s.Latest.Version != "0.2.0" {
		t.Fatalf("what was known went away: %+v", s)
	}
	if v := find(t, versions(t, m), "0.2.0"); !v.Allowed {
		t.Fatalf("%+v", v)
	}
	// A real one published after them is found once the fakes have been looked at.
	w.publish("0.3.0", manifestJSON("0.3.0", "0.4.0", "0.5.x", 1))
	var s Status
	for range 3 {
		s = check(t, m)
	}
	if s.Latest == nil || s.Latest.Version != "0.3.0" {
		t.Fatalf("%+v", s)
	}
}

// The list of withdrawn versions is read even when thirty newer releases hide its release
// from the list of the newest.
func TestWithdrawalsAreFoundByName(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	w.withdraw("wittyphotos", 1, w.key, wittymodule.Withdrawn{Version: "0.2.0", Severity: wittymodule.SeveritySecurity, Reason: "bad"})
	all := w.gh.rels
	hidden := []update.GHRelease{}
	for _, r := range all {
		if r.TagName != wittymodule.WithdrawalsTag {
			hidden = append(hidden, r)
		}
	}
	m := w.manager()
	defer m.Close()
	w.gh.feedOnly = hidden
	check(t, m)
	if v := find(t, versions(t, m), "0.2.0"); v.Allowed {
		t.Fatalf("%+v", v)
	}
}

// A 403 about one repository isn't a reason to stop asking about the others.
func TestA403WithoutTheLimitDoesntHoldEveryone(t *testing.T) {
	w := newWorld(t)
	m := w.manager()
	defer m.Close()
	w.gh.mu.Lock()
	w.gh.forbidden = true
	w.gh.mu.Unlock()
	if r := m.Check(context.Background(), 0); r.HoldUntil != 0 || r.CheckError == "" {
		t.Fatalf("%+v", r)
	}
}

// What a fork said about data doesn't stay to refuse the stock releases, and its list of
// withdrawn versions doesn't replace the module's own.
func TestGoingBackFromAForkLeavesNothingBehind(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	w.withdraw("wittyphotos", 5, w.key, wittymodule.Withdrawn{Version: "0.2.0", Severity: wittymodule.SeverityBroken, Reason: "stock list"})
	m := w.manager()
	defer m.Close()
	check(t, m)
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := m.SetSource("wittyphotos", "someone/WittyPhotos", base64.StdEncoding.EncodeToString(pub), "admin", true); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	e := m.mods["wittyphotos"]
	e.state.DataHigh = 1 << 30 // what a fork could have declared
	m.mu.Unlock()
	if err := m.ClearSource("wittyphotos"); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	high, serial := e.state.DataHigh, 0
	if e.state.Withdrawn != nil {
		serial = e.state.Withdrawn.Serial
	}
	m.mu.Unlock()
	if high != 0 || serial != 5 {
		t.Fatalf("data %d, list %d", high, serial)
	}
}

// A state file edited to name a fork that can't be chosen gets nothing.
func TestSavedSourceIsChecked(t *testing.T) {
	w := newWorld(t)
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	key := base64.StdEncoding.EncodeToString(pub)
	raw := []byte(`{"id":"wittytuner","name":"WittyTuner","version":"0.1.0","repo":"wittywilla/WittyTuner","delivery":"helper","entry":"wittytuner-module","api":2,"minServer":"0.1.0","maxServer":"0.x"}`)
	state, _ := json.Marshal(map[string]moduleState{"wittytuner": {Active: "0.2.0", Source: &Source{Repo: "someone/WittyTuner", Key: key}}})
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.dir, "state.json"), state, 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{Dir: w.dir, Server: w.server, Builtins: [][]byte{raw}, Keys: []ed25519.PublicKey{w.pub}, Fetch: &update.Fetcher{UserAgent: "test", Transport: w.gh}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	defer m.Close()
	if s := one(t, m); s.Source != nil || s.Version != "0.1.0" || s.Repo != "wittywilla/WittyTuner" {
		t.Fatalf("%+v", s)
	}
}

// A report from someone who isn't an admin is kept for an admin to see, and takes nothing
// away from anyone. An admin's count, at most one a minute.
func TestBrowserReports(t *testing.T) {
	old := browserReportEvery
	browserReportEvery = 0
	defer func() { browserReportEvery = old }()
	w := newWorld(t)
	raw, _ := json.Marshal(wittymodule.Manifest{ID: "wittyphotos", Name: "WittyPhotos", Version: "0.1.0", Repo: "wittywilla/WittyPhotos", Delivery: wittymodule.DeliveryHelper, Entry: "x", API: 1, MinServer: "0.1.0", MaxServer: "0.x"})
	m := New(Config{Dir: w.dir, Server: w.server, Builtins: [][]byte{raw}, Keys: []ed25519.PublicKey{w.pub}, Fetch: &update.Fetcher{UserAgent: "test", Transport: w.gh}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	defer m.Close()
	// A helper: nothing a browser says counts.
	if err := m.ReportBrowserFailure("wittyphotos", "0.1.0", "x", true); err != nil {
		t.Fatal(err)
	}
	if ev := one(t, m).Events; len(ev) != 0 {
		t.Fatalf("%+v", ev)
	}
	if err := m.ReportBrowserFailure("nothing", "0.1.0", "x", true); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
}

// Two modules can bring one kind of library, which the store says, and a kind can have a
// name of this server's own that lasts and can't be the same as another's.
func TestLibraryKindsCanBeNamed(t *testing.T) {
	w := newWorld(t)
	m := storeManager(t, w)
	lt := func(typ string) LibraryType {
		t.Helper()
		for _, l := range m.LibraryTypes() {
			if l.Type == typ {
				return l
			}
		}
		t.Fatalf("%s isn't listed", typ)
		return LibraryType{}
	}
	if !lt("discs").Shared || lt("movies").Shared {
		t.Fatalf("%+v %+v", lt("discs"), lt("movies"))
	}
	taken := map[string]string{"movies": "Movies", "shows": "TV shows"}
	if err := m.SetLibraryLabel("discs", "Blu-rays and DVDs", taken); err != nil {
		t.Fatal(err)
	}
	if lt("discs").Label != "Blu-rays and DVDs" {
		t.Fatalf("%+v", lt("discs"))
	}
	for _, bad := range []string{"movies", "MOVIES", strings.Repeat("x", maxLabel+1), "two\nlines"} {
		if err := m.SetLibraryLabel("discs", bad, taken); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if err := m.SetLibraryLabel("movies", "blu-rays and dvds", taken); err == nil {
		t.Error("two kinds with one name")
	}
	if err := m.SetLibraryLabel("nothing", "A name", nil); !errors.Is(err, ErrUnknown) {
		t.Errorf("%v", err)
	}
	m.Close()
	m = storeManager(t, w)
	defer m.Close()
	if lt("discs").Label != "Blu-rays and DVDs" {
		t.Fatalf("after a restart: %+v", lt("discs"))
	}
	if err := m.SetLibraryLabel("discs", "", nil); err != nil {
		t.Fatal(err)
	}
	if lt("discs").Label != "" {
		t.Fatalf("%+v", lt("discs"))
	}
}

// Two modules bringing the same kind of library share it, on purpose. Each pair is written
// down here, so a new module that happens to pick a kind already in use is caught when it is
// added rather than found by someone whose library plays the wrong way.
func TestLibraryKindsAreSharedOnlyOnPurpose(t *testing.T) {
	onPurpose := map[string]string{
		"discs": "wittybluray,wittydvd",            // a disc library holds either kind
		"books": "wittycomics,wittyebook,wittypdf", // a book library holds any of these
	}
	all := append(GoManifests(), ServerManifests()...)
	for _, dir := range []string{"wittycomics", "wittyebook", "wittyink", "wittypdf"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "web", "src", "modules", dir, "module.json"))
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, b)
	}
	byKind := map[string][]string{}
	for _, raw := range all {
		man, err := wittymodule.ParseManifest(raw)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range man.Libraries {
			byKind[k] = append(byKind[k], man.ID)
		}
	}
	for kind, ids := range byKind {
		sort.Strings(ids)
		if len(ids) > 1 && onPurpose[kind] != strings.Join(ids, ",") {
			t.Errorf("%s all bring %q. If they should share it, say so here; if not, one needs a kind of its own.", strings.Join(ids, ", "), kind)
		}
	}
}
