package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/reset"
)

func waitRepair(t *testing.T, ts *galleryServer) repairView {
	t.Helper()
	for i := 0; i < 400; i++ {
		w := ts.do("GET", "/api/repair", ts.admin, nil)
		var v repairView
		if err := json.Unmarshal(w.Body.Bytes(), &v); w.Code != 200 || err != nil {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		if v.Running == "" {
			return v
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("it never finished")
	return repairView{}
}

func TestRepairAsksTheRightPeople(t *testing.T) {
	ts := newGalleryServer(t)
	for _, c := range []struct{ method, path string }{{"GET", "/api/repair"}, {"POST", "/api/repair/check"}, {"POST", "/api/repair/fix"}, {"POST", "/api/repair/remap"}, {"POST", "/api/repair/restart"}} {
		if w := ts.do(c.method, c.path, "", map[string]any{}); w.Code != 401 {
			t.Errorf("signed out, %s: %d", c.path, w.Code)
		}
		if w := ts.do(c.method, c.path, ts.kid, map[string]any{}); w.Code != 403 {
			t.Errorf("not an administrator, %s: %d", c.path, w.Code)
		}
	}
}

func TestRepairLooksThenFixes(t *testing.T) {
	ts := newGalleryServer(t)
	data := t.TempDir()
	ts.s.Cfg.DataDir = data
	ts.s.Cfg.CacheDir = filepath.Join(data, "cache")
	ts.s.Cfg.ImagesDir = filepath.Join(data, "cache", "images")
	ts.s.Cfg.TranscodeDir = filepath.Join(data, "transcode")
	ts.s.Cfg.LogDir = filepath.Join(data, "logs")
	old := time.Now().Add(-72 * time.Hour)
	stray := filepath.Join(data, "left.tmp")
	if err := os.WriteFile(stray, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(stray, old, old); err != nil {
		t.Fatal(err)
	}
	if err := reset.Ask(data, reset.Request{Mode: reset.ModeKeepCopy, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	waiting := filepath.Join(data, "reset-request.json")
	if err := os.Chtimes(waiting, old, old); err != nil {
		t.Fatal(err)
	}

	if w := ts.do("POST", "/api/repair/fix", ts.admin, map[string]any{"allSafe": true}); w.Code != 409 {
		t.Errorf("a fix with nothing checked yet: %d", w.Code)
	}
	if w := ts.do("POST", "/api/repair/check", ts.admin, map[string]any{"only": []string{"no-such-check"}}); w.Code != 400 {
		t.Errorf("an unknown check: %d", w.Code)
	}
	if w := ts.do("POST", "/api/repair/check", ts.admin, map[string]any{"only": []string{"data"}}); w.Code != 202 {
		t.Fatalf("check: %d %s", w.Code, w.Body)
	}
	v := waitRepair(t, ts)
	if v.Report == nil || v.Report.Find("temp-files") == nil || v.Report.Find("reset-state:stale") == nil {
		t.Fatalf("found: %+v", v.Report)
	}
	if _, err := os.Stat(stray); err != nil {
		t.Fatal("looking changed something")
	}

	// What changes data asks for the password. What can't lose anything doesn't.
	if w := ts.do("POST", "/api/repair/fix", ts.admin, map[string]any{"keys": []string{"reset-state:stale"}, "password": "not it"}); w.Code != 403 {
		t.Errorf("the wrong password: %d %s", w.Code, w.Body)
	}
	if reset.Pending(data) == nil {
		t.Fatal("it went ahead without the password")
	}
	if w := ts.do("POST", "/api/repair/fix", ts.admin, map[string]any{"keys": []string{"nothing-like-this"}}); w.Code != 404 {
		t.Errorf("an unknown issue: %d", w.Code)
	}
	if w := ts.do("POST", "/api/repair/fix", ts.admin, map[string]any{"allSafe": true}); w.Code != 202 {
		t.Fatalf("safe fixes: %d %s", w.Code, w.Body)
	}
	v = waitRepair(t, ts)
	if _, err := os.Stat(stray); err == nil {
		t.Fatalf("the safe fix didn't run: %+v", v.Results)
	}
	if reset.Pending(data) == nil {
		t.Fatal("a safe batch ran a fix that isn't safe")
	}
	if len(v.Results) != 1 || !v.Results[0].OK || v.Results[0].Copy == "" {
		t.Fatalf("results: %+v", v.Results)
	}
	if w := ts.do("POST", "/api/repair/fix", ts.admin, map[string]any{"keys": []string{"reset-state:stale"}, "password": resetPassword}); w.Code != 202 {
		t.Fatalf("with the password: %d %s", w.Code, w.Body)
	}
	v = waitRepair(t, ts)
	if reset.Pending(data) != nil {
		t.Fatalf("it's still waiting: %+v", v.Results)
	}

	// No repair is waiting, so nobody gets to restart the server through here.
	ts.s.Restart = func() { t.Error("restarted") }
	if w := ts.do("POST", "/api/repair/restart", ts.admin, nil); w.Code != 409 {
		t.Errorf("restart with nothing waiting: %d", w.Code)
	}
}

func TestRepairRemapChecksBeforeAnything(t *testing.T) {
	ts := newGalleryServer(t)
	data := t.TempDir()
	ts.s.Cfg.DataDir = data
	if w := ts.do("POST", "/api/repair/remap", ts.admin, map[string]any{"from": "", "to": ""}); w.Code != 400 {
		t.Errorf("no paths: %d %s", w.Code, w.Body)
	}
	if w := ts.do("POST", "/api/repair/remap", ts.admin, map[string]any{"from": "/a", "to": "relative"}); w.Code != 400 {
		t.Errorf("a relative path: %d %s", w.Code, w.Body)
	}
	if w := ts.do("POST", "/api/repair/remap", ts.admin, map[string]any{"from": "/a", "to": data, "apply": true, "password": "not it"}); w.Code != 403 {
		t.Errorf("the wrong password: %d %s", w.Code, w.Body)
	}
}
