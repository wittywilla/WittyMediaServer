// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymodule"
)

// Attacks on the store from the second security pass. Each failed against the code as it was.

// aForkWorld is a server with a store listing a version of a module it knows.
func aForkWorld(t *testing.T) (*storesWorld, string, func(serial int64, vers ...wittymodule.CatalogVersion), func(ver string) wittymodule.CatalogVersion) {
	t.Helper()
	w := newStoresWorld(t, listedModule("wittycomics", "WittyComics"))
	pub, key := testKey(t)
	release := filepath.Join(t.TempDir(), "release")
	const base = "https://store.example/modules/"
	make1 := func(ver string) wittymodule.CatalogVersion {
		v := webRelease(t, release, key, "wittycomics", "WittyComics", "someone/wittycomics", ver)
		for _, rel := range []string{v.Sums, v.Sig, v.Manifest, v.Platforms[wittymodule.PlatformAll].URL} {
			b, err := os.ReadFile(filepath.Join(release, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatal(err)
			}
			w.net.put(base+rel, b)
		}
		return v
	}
	publish := func(serial int64, vers ...wittymodule.CatalogVersion) {
		mod := listedModule("wittycomics", "WittyComics")
		mod.Versions = vers
		publishStore(w.net, key, pub, serial, mod)
	}
	publish(1, make1("0.9.0"))
	if _, err := w.s.Add(context.Background(), storeURL, pub, false, "admin"); err != nil {
		t.Fatal(err)
	}
	return w, w.s.List()[0].ID, publish, make1
}

func waitIdle(t *testing.T, w *storesWorld, id string) Status {
	t.Helper()
	for i := 0; i < 500; i++ {
		st, _ := w.s.statusOf(id)
		if st.Phase == PhaseIdle || st.Phase == PhaseFailed {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("it never finished")
	return Status{}
}

// Whoever runs the machine said no forks. The old fork route obeyed; the store's went round it.
func TestStoreForkObeysTheMachinesLock(t *testing.T) {
	w, store, _, _ := aForkWorld(t)
	t.Setenv(LockForks, "1")
	err := w.s.UseFork(store, "wittycomics", "admin", true, "")
	if err == nil || !strings.Contains(err.Error(), LockForks) {
		t.Fatalf("a fork from a store past the machine's lock: %v", err)
	}
	if st, _ := w.s.statusOf("wittycomics"); st.Source != nil {
		t.Fatalf("it took the store as a source: %+v", st.Source)
	}
	for _, it := range w.s.View().Items {
		if it.Store == store && it.Kind != KindRefused {
			t.Fatalf("the store offers it as %q, on a machine that refuses it", it.Kind)
		}
	}
}

// "Off, it isn't fetched or shown, and nothing new can be installed from it."
func TestATurnedOffStoreInstallsNothingNew(t *testing.T) {
	needForks(t)
	w, store, publish, make1 := aForkWorld(t)
	if err := w.s.UseFork(store, "wittycomics", "admin", true, ""); err != nil {
		t.Fatal(err)
	}
	if st := waitIdle(t, w, "wittycomics"); st.Version != "0.9.0" {
		t.Fatalf("the fork: %+v", st)
	}
	ctx := context.Background()
	newer := make1("0.9.1")
	first := make1("0.9.0")
	publish(2, newer, first)
	if err := w.s.Refresh(ctx, store); err != nil {
		t.Fatal(err)
	}
	if err := w.s.SetEnabled(store, false, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := w.m.InstallVersion("wittycomics", "0.9.1", true); err == nil {
		t.Fatal("a version from a store that's turned off started installing")
	}
	if st, _ := w.s.statusOf("wittycomics"); st.Latest != nil {
		t.Fatalf("a store that's off still offers %+v", st.Latest)
	}
	// Nor is it asked anything, even when somebody presses check.
	n := w.net.asks("catalog.json")
	if err := w.s.Refresh(ctx, store); err == nil {
		t.Fatal("checked a store that's off, with no complaint")
	}
	if w.net.asks("catalog.json") != n {
		t.Fatal("a store that's off was fetched")
	}
	// A restart mustn't bring its versions back.
	again := NewStores(StoresConfig{Manager: w.m, Dir: w.dir, Transport: w.net})
	if st, _ := again.statusOf("wittycomics"); st.Latest != nil {
		t.Fatalf("after a restart, a store that's off offers %+v", st.Latest)
	}
	if err := w.m.InstallVersion("wittycomics", "0.9.1", true); err == nil {
		t.Fatal("after a restart, a version from a store that's off started installing")
	}
	// Turned on again, it comes back.
	if err := again.SetEnabled(store, true, "admin"); err != nil {
		t.Fatal(err)
	}
	if st, _ := again.statusOf("wittycomics"); st.Latest == nil || st.Latest.Version != "0.9.1" {
		t.Fatalf("back on: %+v", st.Latest)
	}
}

// A store lists as many pictures as a catalogue can hold, and changes them at every serial. What
// the server keeps of them is bounded, and what's no longer listed goes.
func TestAStoresPicturesCantFillTheDisk(t *testing.T) {
	w := newStoresWorld(t)
	pub, key := testKey(t)
	ctx := context.Background()
	const shot = maxMediaShot // valid to the header, and padded out to the most that's allowed
	var mods []wittymodule.CatalogModule
	var hashes []string
	head := pngOf(t)
	for i := 0; i < 5; i++ {
		mod := listedModule(fmt.Sprintf("big%d", i), fmt.Sprintf("Big %d", i))
		for j := 0; j < 8; j++ {
			b := append(append([]byte(nil), head...), make([]byte, shot-len(head))...)
			copy(b[len(head):], fmt.Sprintf("%d-%d", i, j))
			name := fmt.Sprintf("s%d-%d.png", i, j)
			w.net.put("https://store.example/modules/"+name, b)
			mod.Screenshots = append(mod.Screenshots, wittymodule.CatalogFile{URL: name, Size: int64(len(b)), SHA256: sum(b)})
			hashes = append(hashes, sum(b))
		}
		mods = append(mods, mod)
	}
	publishStore(w.net, key, pub, 1, mods...)
	if _, err := w.s.Add(ctx, storeURL, pub, false, "admin"); err != nil {
		t.Fatal(err)
	}
	id := w.s.List()[0].ID
	saved := func() (total int64, n int) {
		es, _ := os.ReadDir(filepath.Join(w.dir, storesDir, id, "media"))
		for _, e := range es {
			if i, err := e.Info(); err == nil {
				total += i.Size()
				n++
			}
		}
		return
	}
	served := 0
	for _, h := range hashes {
		if p, _, err := w.s.Media(ctx, id, h); err == nil {
			if _, err := os.Stat(p); err != nil {
				t.Fatalf("it said a picture was at %s: %v", p, err)
			}
			served++
		}
	}
	if total, _ := saved(); total > maxStoreMedia {
		t.Fatalf("kept %d bytes of one store's pictures, over the limit of %d", total, maxStoreMedia)
	}
	if served == 0 {
		t.Fatal("no pictures were served at all")
	}
	// A new list, with none of those pictures: they go.
	publishStore(w.net, key, pub, 2, listedModule("big0", "Big 0"))
	if err := w.s.Refresh(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, n := saved(); n != 0 {
		t.Fatalf("%d pictures the store no longer lists were kept", n)
	}
}

// The metadata address of a cloud machine, wrapped in IPv6 so a check that only looks at the
// outside lets it through, for a store allowed on the admin's own network.
func TestPrivateStoreCantReachLinkLocalInDisguise(t *testing.T) {
	for _, raw := range []string{"64:ff9b::a9fe:a9fe", "2002:a9fe:a9fe::"} {
		if !carriesLinkLocal(netip.MustParseAddr(raw)) {
			t.Errorf("%s wasn't seen for what it is", raw)
		}
	}
	for _, raw := range []string{"64:ff9b::a00:1", "2001:db8::1", "10.0.0.1"} {
		if carriesLinkLocal(netip.MustParseAddr(raw)) {
			t.Errorf("%s was refused for no reason", raw)
		}
	}
}

// A name made to pass for one of this project's modules.
func TestLookalikeNamesAreCaught(t *testing.T) {
	for _, name := range []string{"WittyComics", "wittycomics", "Witty Comics", "Witty-Comics", "WіttyComics" /* Cyrillic i */, "ＷittyComics" /* full width */, "WittyCоmics" /* Cyrillic o */, "Witty​Comics"} {
		w := newStoresWorld(t, listedModule("wittycomics", "WittyComics"))
		pub, key := testKey(t)
		publishStore(w.net, key, pub, 1, listedModule("sneaky", name))
		if _, err := w.s.Add(context.Background(), storeURL, pub, false, "admin"); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, it := range w.s.View().Items {
			if it.ID == "sneaky" {
				found = true
				if it.Lookalike != "wittycomics" {
					t.Errorf("%q wasn't flagged as looking like WittyComics: %+v", name, it.Lookalike)
				}
			}
		}
		if !found {
			t.Fatal("the store's module isn't listed")
		}
	}
	w := newStoresWorld(t, listedModule("wittycomics", "WittyComics"))
	pub, key := testKey(t)
	publishStore(w.net, key, pub, 1, listedModule("clock", "Комиксы Clock"))
	if _, err := w.s.Add(context.Background(), storeURL, pub, false, "admin"); err != nil {
		t.Fatal(err)
	}
	for _, it := range w.s.View().Items {
		if it.ID == "clock" && it.Lookalike != "" {
			t.Errorf("a name that looks like nothing was flagged: %+v", it.Lookalike)
		}
	}
}

// The key shown when an admin was asked has to be the key that gets pinned, whatever the store has
// said since.
func TestForkPinsTheKeyTheAdminSaw(t *testing.T) {
	needForks(t)
	w, store, _, _ := aForkWorld(t)
	d, err := w.s.Detail(store, "wittycomics")
	if err != nil || d.Fingerprint == "" {
		t.Fatalf("no fingerprint to show: %v", err)
	}
	seen := d.Fingerprint
	if err := w.s.UseFork(store, "wittycomics", "admin", true, strings.Repeat("0", len(seen))); err == nil {
		t.Fatal("a fork went ahead with a different key from the one the admin was shown")
	}
	if st, _ := w.s.statusOf("wittycomics"); st.Source != nil {
		t.Fatalf("it took the store as a source: %+v", st.Source)
	}
	if err := w.s.UseFork(store, "wittycomics", "admin", true, seen); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, w, "wittycomics")
}

// A fork's manifest names the program the server will start. It can't name one outside its folder.
func TestAManifestCantNameAProgramOutsideItsFolder(t *testing.T) {
	for _, bad := range []string{"../../../bin/sh", "/bin/sh", "a/b", `a\b`, "..", ".hidden", "x\x00y", "C:evil"} {
		for _, field := range []string{`"entry"`, `"programs"`, `"webEntry"`} {
			value := `"` + strings.ReplaceAll(bad, `\`, `\\`) + `"`
			entry, extra := `"entry":"good"`, ""
			switch field {
			case `"entry"`:
				entry = `"entry":` + value
			case `"programs"`:
				extra = `,"programs":[` + value + `]`
			case `"webEntry"`:
				extra = `,"webEntry":` + value
			}
			raw := `{"id":"wittycomics","name":"X","version":"0.9.0","repo":"a/b","delivery":"helper",` + entry + `,"api":` + fmt.Sprint(wittymodule.API) + `,"minServer":"0.6.0-alpha","maxServer":"0.6.x","license":"MIT"` + extra + `}`
			if _, err := wittymodule.ParseManifest([]byte(raw)); err == nil {
				t.Errorf("%s of %q was taken", field, bad)
			}
		}
	}
}
