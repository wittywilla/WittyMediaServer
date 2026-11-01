package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
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
	if w := ts.do(t, "PUT", "/api/libtranscode/tdarr", admin, map[string]any{"url": home.URL, "apiKey": "THE-SECRET-KEY"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/libtranscode/tdarr/test", admin, map[string]any{}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/libtranscode/tdarr/test", admin, map[string]any{"url": other.URL}); w.Code != 200 {
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
	if w := ts.do(t, "PUT", "/api/libtranscode/tdarr", admin, map[string]any{"url": other.URL}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/libtranscode/tdarr/test", admin, map[string]any{}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if k := seen("other"); k != "" {
		t.Errorf("saving a new address carried the key to it: %q", k)
	}
}
