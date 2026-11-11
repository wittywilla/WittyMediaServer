// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/libtranscode"
)

// Asking what a move would change looks at the new place, and asks nothing else. So it keeps to what
// any path from a browser keeps to: looking at a share on another machine sends this one's sign-in there.
func TestRemapWontLookAtAnotherMachine(t *testing.T) {
	ts := newGalleryServer(t)
	ts.s.Cfg.DataDir = t.TempDir()
	for _, to := range []string{`\\attacker.example\share`, `//attacker.example/share`} {
		w := ts.do("POST", "/api/repair/remap", ts.admin, map[string]any{"from": "/old", "to": to})
		if w.Code != 400 || !strings.Contains(w.Body.String(), errOtherMachine.Error()) {
			t.Errorf("%s: %d %s", to, w.Code, w.Body)
		}
	}
}

// Which FFmpeg a server runs is something to look up flaws by, and only its administrator needs it.
func TestMediaTypesOnlyToldToAdminsWhichFFmpeg(t *testing.T) {
	ts := newTestServer(t)
	ts.Tools.Version = "ffmpeg version 9.9.9-telltale"
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, kid := ts.user(t, "pip", "kids", false, "")
	if w := ts.do(t, "GET", "/api/media-types", kid, nil); w.Code != 200 || strings.Contains(w.Body.String(), "telltale") {
		t.Errorf("a kid: %d, told: %v", w.Code, strings.Contains(w.Body.String(), "telltale"))
	}
	if w := ts.do(t, "GET", "/api/media-types", admin, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "telltale") {
		t.Errorf("an administrator: %d, told: %v", w.Code, strings.Contains(w.Body.String(), "telltale"))
	}
}

// The key is saved for one address, and a stolen session can type another. Tdarr's key is someone else's
// secret that the page never shows, so nothing here may hand it to a new place.
func TestTdarrKeyStaysWithItsAddress(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	var mu sync.Mutex
	got := map[string]string{}
	listen := func(name string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			got[name] = r.Header.Get("x-api-key")
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"good","isProduction":true,"version":"1"}`))
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	seen := func(name string) string {
		mu.Lock()
		defer mu.Unlock()
		return got[name]
	}
	home, other := listen("home"), listen("other")
	if w := ts.do(t, "PUT", "/api/libtranscode/tdarr", admin, map[string]any{"url": home.URL, "apiKey": "THE-SECRET-KEY", "password": "correct-horse-1"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/libtranscode/tdarr/test", admin, map[string]any{}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/libtranscode/tdarr/test", admin, map[string]any{"url": other.URL, "password": "correct-horse-1"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if k := seen("home"); k != "THE-SECRET-KEY" {
		t.Errorf("the saved address should get its key: %q", k)
	}
	if k := seen("other"); k != "" {
		t.Errorf("a typed address was sent the saved key: %q", k)
	}
	// Typing the saved address is that address.
	if w := ts.do(t, "POST", "/api/libtranscode/tdarr/test", admin, map[string]any{"url": home.URL + "/"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if k := seen("home"); k != "THE-SECRET-KEY" {
		t.Errorf("the saved address, as typed: %q", k)
	}
	// Saving another address doesn't take the key along either.
	if w := ts.do(t, "PUT", "/api/libtranscode/tdarr", admin, map[string]any{"url": other.URL, "password": "correct-horse-1"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/libtranscode/tdarr/test", admin, map[string]any{}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if k := seen("other"); k != "" {
		t.Errorf("saving a new address carried the key to it: %q", k)
	}
}

// What answers at an address says what's listening there, so a stolen session can't look around
// this machine's network through Tdarr's test without the password.
func TestTdarrNewAddressAsksForThePassword(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	ts.LibConvert = &libtranscode.Engine{DB: ts.DB, Store: ts.Store}
	var asked atomic.Int32
	inside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { asked.Add(1) }))
	defer inside.Close()
	for _, try := range []struct{ method, path string }{{"POST", "/api/libtranscode/tdarr/test"}, {"PUT", "/api/libtranscode/tdarr"}} {
		w := ts.do(t, try.method, try.path, admin, map[string]any{"url": inside.URL})
		if w.Code != 403 || !strings.Contains(w.Body.String(), "password-needed") {
			t.Errorf("%s with no password: %d %s", try.path, w.Code, w.Body)
		}
		if w := ts.do(t, try.method, try.path, admin, map[string]any{"url": inside.URL, "password": "not the password"}); w.Code != 403 {
			t.Errorf("%s with the wrong password: %d %s", try.path, w.Code, w.Body)
		}
	}
	if asked.Load() != 0 {
		t.Error("it went to the address before asking for the password")
	}
	if w := ts.do(t, "PUT", "/api/libtranscode/tdarr", admin, map[string]any{"url": inside.URL, "password": "correct-horse-1"}); w.Code != 200 {
		t.Fatalf("with the password: %d %s", w.Code, w.Body)
	}
	// The address it already has asks for nothing.
	if w := ts.do(t, "POST", "/api/libtranscode/tdarr/test", admin, map[string]any{"url": inside.URL}); w.Code != 200 {
		t.Errorf("the saved address: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/libtranscode/tdarr/test", admin, map[string]any{}); w.Code != 200 {
		t.Errorf("no address: %d %s", w.Code, w.Body)
	}
}

// Sanitizing is turned off only by asking in so many words.
func TestBundleSanitizedUnlessAskedOtherwise(t *testing.T) {
	for q, want := range map[string]bool{"": true, "sanitize=1": true, "sanitize=maybe": true, "sanitize=": true, "redact=x": true, "sanitize=0": false, "sanitize=false": false, "redact=no": false} {
		r := httptest.NewRequest("GET", "/api/logs/bundle?"+q, nil)
		if got := bundleSanitized(r); got != want {
			t.Errorf("%q: %v", q, got)
		}
	}
}

// The address a cloud machine keeps its credentials at, wrapped in IPv6 for a gateway to unwrap.
func TestLyricsMirrorCantReachLinkLocalInDisguise(t *testing.T) {
	for _, addr := range []string{"[64:ff9b::a9fe:a9fe]:80", "[2002:a9fe:a9fe::]:80", "169.254.169.254:80", "[fe80::1]:80"} {
		if err := lyricsMirrorAddress("tcp", addr, nil); err == nil {
			t.Errorf("%s was allowed", addr)
		}
	}
	for _, addr := range []string{"192.168.1.20:3300", "[fd00::1]:3300", "8.8.8.8:443"} {
		if err := lyricsMirrorAddress("tcp", addr, nil); err != nil {
			t.Errorf("%s: %v", addr, err)
		}
	}
}
