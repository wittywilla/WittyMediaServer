package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
)

func auditRows(t *testing.T, ts *testServer, action string) int {
	t.Helper()
	var n int
	if err := ts.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM game_core_audit WHERE action = ?`, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A stolen session used to be able to move a module to its own repository, signed with its own key.
// Its code then ran on the server, and as a web bundle, in every viewer's browser.
func TestModuleForkAsksForThePasswordAndCanBeLocked(t *testing.T) {
	ts := newTestServer(t)
	ts.Modules = modmgr.New(modmgr.Config{Dir: filepath.Join(t.TempDir(), "modules"), Server: "0.6.0-alpha", Builtins: modmgr.ServerManifests(), Log: slog.New(slog.DiscardHandler)})
	t.Cleanup(ts.Modules.Close)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "adult", "adult", false, "")
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString(pub)
	const route = "/api/modules/wittyphotos/source"
	body := map[string]any{"repo": "someone/WittyPhotos", "key": key, "confirm": true}

	if w := ts.do(t, "PUT", route, adult, body); w.Code != 403 {
		t.Fatalf("an adult: %d", w.Code)
	}
	// The warning needs nothing; agreeing to it does.
	if w := ts.do(t, "PUT", route, admin, map[string]any{"repo": "someone/WittyPhotos", "key": key}); w.Code != 409 {
		t.Fatalf("the warning: %d %s", w.Code, w.Body)
	}
	w := ts.do(t, "PUT", route, admin, body)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 403 || out["code"] != "password-needed" {
		t.Fatalf("nothing asked: %d %s", w.Code, w.Body)
	}
	body["password"] = "not the password"
	if w := ts.do(t, "PUT", route, admin, body); w.Code != 403 {
		t.Fatalf("wrong password: %d %s", w.Code, w.Body)
	}
	if auditRows(t, ts, "password-refused") == 0 {
		t.Error("a refusal should be written down")
	}
	for _, st := range ts.Modules.Report().Modules {
		if st.ID == "wittyphotos" && st.Source != nil {
			t.Fatal("the source changed")
		}
	}

	body["password"] = "correct-horse-1"
	t.Setenv(modmgr.LockForks, "1")
	if w := ts.do(t, "PUT", route, admin, body); w.Code != 403 || !strings.Contains(w.Body.String(), modmgr.LockForks) {
		t.Fatalf("locked, right password: %d %s", w.Code, w.Body)
	}
	for _, st := range ts.Modules.Report().Modules {
		if st.ID == "wittyphotos" && !strings.Contains(st.NoFork, modmgr.LockForks) {
			t.Errorf("the page should be told forks are refused: %q", st.NoFork)
		}
	}
	t.Setenv(modmgr.LockForks, "")
	if w := ts.do(t, "PUT", route, admin, body); w.Code != 200 {
		t.Fatalf("right password: %d %s", w.Code, w.Body)
	}
	if auditRows(t, ts, "module-source") != 1 {
		t.Error("the change should be written down")
	}
}

// Going to an older version can bring back a hole that was fixed, so it asks too.
func TestModuleDowngradesAskForThePassword(t *testing.T) {
	ts := newTestServer(t)
	ts.Modules = installedBundle(t, "export default 1", "")
	_, admin := ts.user(t, "owner", "adult", true, "")
	needed := func(method, route string, body any) {
		t.Helper()
		w := ts.do(t, method, route, admin, body)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		if w.Code != 403 || out["code"] != "password-needed" {
			t.Errorf("%s %s: %d %s", method, route, w.Code, w.Body)
		}
	}
	needed("POST", "/api/modules/wittycomics/install", map[string]any{"version": "0.1.5", "confirm": true})
	needed("POST", "/api/modules/wittycomics/rollback", nil)

	// A newer one, or the warning alone, needs nothing.
	if w := ts.do(t, "POST", "/api/modules/wittycomics/install", admin, map[string]any{"version": "0.3.0", "confirm": true}); w.Code == 403 {
		t.Errorf("a newer version: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/modules/wittycomics/install", admin, map[string]any{"version": "0.1.5"}); w.Code == 403 {
		t.Errorf("no confirm yet: %d %s", w.Code, w.Body)
	}
	w := ts.do(t, "POST", "/api/modules/wittycomics/rollback", admin, map[string]any{"password": "correct-horse-1"})
	if w.Code != 200 {
		t.Fatalf("with the password: %d %s", w.Code, w.Body)
	}
	if auditRows(t, ts, "module-rollback") != 1 {
		t.Error("the change should be written down")
	}
}
