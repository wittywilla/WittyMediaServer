package server

import (
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
)

// Everything about the store is for admins: it shows what a server has and what it can be
// changed to, a store is someone's code, and a picture's address can carry a token.
func TestModuleStoreRoutesAreForAdmins(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	_, kid := ts.user(t, "kid", "kid", false, "")
	dir := filepath.Join(t.TempDir(), "modules")
	m := modmgr.New(modmgr.Config{Dir: dir, Server: "0.6.0-alpha", Log: slog.New(slog.DiscardHandler)})
	t.Cleanup(m.Close)
	ts.Modules = m
	ts.ModuleStores = modmgr.NewStores(modmgr.StoresConfig{Manager: m, Dir: dir, Log: slog.New(slog.DiscardHandler)})

	routes := []struct{ method, path, body string }{
		{"GET", "/api/modulestore", ""},
		{"POST", "/api/modulestore/check", ""},
		{"GET", "/api/modulestore/item/official/wittydvd", ""},
		{"POST", "/api/modulestore/install", `{"id":"wittydvd"}`},
		{"POST", "/api/modulestore/update-all", ""},
		{"GET", "/api/modulestore/run", ""},
		{"POST", "/api/modulestore/run/undo", ""},
		{"DELETE", "/api/modulestore/run", ""},
		{"GET", "/api/modulestore/modules/wittydvd/log", ""},
		{"GET", "/api/modulestore/media/official/" + strings.Repeat("a", 64), ""},
		{"POST", "/api/modulestore/stores/preview", `{"url":"https://example.com/catalog.json"}`},
		{"POST", "/api/modulestore/stores", `{"url":"https://example.com/catalog.json","agreed":true}`},
		{"DELETE", "/api/modulestore/stores/s000000000000", ""},
		{"PUT", "/api/modulestore/stores/s000000000000/enabled", `{"on":false}`},
		{"POST", "/api/modulestore/stores/s000000000000/check", ""},
		{"POST", "/api/modulestore/stores/s000000000000/fork/wittydvd", `{"confirm":true}`},
	}
	for _, c := range routes {
		if w := ts.do(t, c.method, c.path, "", []byte(c.body)); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s signed out: %d", c.method, c.path, w.Code)
		}
		if w := ts.do(t, c.method, c.path, kid, []byte(c.body)); w.Code != http.StatusForbidden {
			t.Errorf("%s %s as a profile: %d", c.method, c.path, w.Code)
		}
	}
	// A store can't be added without the warning having been agreed to.
	if w := ts.do(t, "POST", "/api/modulestore/stores", admin, []byte(`{"url":"https://example.com/catalog.json","key":"x"}`)); w.Code != 400 {
		t.Errorf("adding a store with no agreement: %d", w.Code)
	}
	// An address on this machine is refused in words, with a failure a page can show.
	w := ts.do(t, "POST", "/api/modulestore/stores/preview", admin, []byte(`{"url":"https://127.0.0.1/catalog.json"}`))
	if w.Code != 409 || !strings.Contains(w.Body.String(), `"failure"`) {
		t.Errorf("a private address: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/modulestore", admin, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "nothing is promised until the beta") {
		t.Errorf("the view: %d", w.Code)
	}
}
