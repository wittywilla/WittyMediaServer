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
// changed to, and a store is someone's code.
func TestModuleStoreRoutesAreForAdmins(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	_, kid := ts.user(t, "kid", "kids", false, "")
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

// What changes which code runs asks for the password, as a stolen session is what it's there for.
func TestModuleStoreChangesAskForThePassword(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	dir := filepath.Join(t.TempDir(), "modules")
	m := modmgr.New(modmgr.Config{Dir: dir, Server: "0.6.0-alpha", Log: slog.New(slog.DiscardHandler)})
	t.Cleanup(m.Close)
	ts.Modules = m
	ts.ModuleStores = modmgr.NewStores(modmgr.StoresConfig{Manager: m, Dir: dir, Log: slog.New(slog.DiscardHandler)})

	for _, r := range []struct{ path, body string }{
		{"/api/modulestore/stores", `{"url":"https://example.com/catalog.json","agreed":true}`},
		{"/api/modulestore/stores/s000000000000/fork/wittydvd", `{"confirm":true}`},
		{"/api/modulestore/run/undo", ""},
	} {
		w := ts.do(t, "POST", r.path, admin, []byte(r.body))
		if w.Code != 403 || !strings.Contains(w.Body.String(), "password-needed") {
			t.Errorf("POST %s with no password: %d %s", r.path, w.Code, w.Body)
		}
		wrong := `{"password":"not the password"}`
		if r.body != "" {
			wrong = r.body[:len(r.body)-1] + `,"password":"not the password"}`
		}
		if w := ts.do(t, "POST", r.path, admin, []byte(wrong)); w.Code != 403 {
			t.Errorf("POST %s with the wrong password: %d %s", r.path, w.Code, w.Body)
		}
	}
	if auditRows(t, ts, "password-refused") < 3 {
		t.Error("each refusal should be written down")
	}
	// The warning comes first, and needs nothing.
	if w := ts.do(t, "POST", "/api/modulestore/stores/s000000000000/fork/wittydvd", admin, []byte(`{}`)); w.Code == 403 {
		t.Errorf("before agreeing: %d %s", w.Code, w.Body)
	}
	// Past the password, the store itself is what's checked.
	right := []byte(`{"url":"https://127.0.0.1/catalog.json","agreed":true,"key":"x","password":"correct-horse-1"}`)
	if w := ts.do(t, "POST", "/api/modulestore/stores", admin, right); w.Code == 403 || w.Code == 201 {
		t.Errorf("the right password and a store on this machine: %d %s", w.Code, w.Body)
	}
}

// A store's picture answers a header, and not a token in its address.
func TestModuleStorePictureTakesNoTokenInTheAddress(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	dir := filepath.Join(t.TempDir(), "modules")
	m := modmgr.New(modmgr.Config{Dir: dir, Server: "0.6.0-alpha", Log: slog.New(slog.DiscardHandler)})
	t.Cleanup(m.Close)
	ts.Modules = m
	ts.ModuleStores = modmgr.NewStores(modmgr.StoresConfig{Manager: m, Dir: dir, Log: slog.New(slog.DiscardHandler)})
	path := "/api/modulestore/media/official/" + strings.Repeat("a", 64)
	if w := ts.do(t, "GET", path+"?token="+admin, "", nil); w.Code != 401 {
		t.Errorf("a token in the address: %d", w.Code)
	}
	if w := ts.do(t, "GET", path, admin, nil); w.Code != 404 {
		t.Errorf("with a header, a picture no catalogue lists: %d", w.Code)
	}
}
