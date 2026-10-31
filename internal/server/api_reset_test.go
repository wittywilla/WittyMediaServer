package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/reset"
)

const resetPassword = "password1" // the one newGalleryServer gives everyone

func TestResetAsksEveryoneToBeSure(t *testing.T) {
	ts := newGalleryServer(t)
	restarted := make(chan struct{}, 1)
	ts.s.Restart = func() { restarted <- struct{}{} }
	data := t.TempDir()
	ts.s.Cfg.DataDir = data

	if w := ts.do("POST", "/api/reset", "", map[string]any{"password": resetPassword, "confirm": "RESET"}); w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}
	if w := ts.do("POST", "/api/reset", ts.kid, map[string]any{"password": resetPassword, "confirm": "RESET"}); w.Code != 403 {
		t.Errorf("not an administrator: %d", w.Code)
	}
	if w := ts.do("POST", "/api/reset", ts.admin, map[string]any{"password": resetPassword}); w.Code != 400 {
		t.Errorf("without typing RESET: %d", w.Code)
	}
	if w := ts.do("POST", "/api/reset", ts.admin, map[string]any{"password": "not it", "confirm": "RESET"}); w.Code != 403 {
		t.Errorf("the wrong password: %d", w.Code)
	}
	if reset.Pending(data) != nil {
		t.Fatal("something was asked for by someone who may not")
	}

	w := ts.do("GET", "/api/reset", ts.admin, nil)
	var v resetView
	if err := json.Unmarshal(w.Body.Bytes(), &v); w.Code != 200 || err != nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if v.Blocked != "" || !v.CanRestart || v.DataDir != data {
		t.Errorf("%+v", v)
	}

	if w := ts.do("POST", "/api/reset", ts.admin, map[string]any{"password": resetPassword, "confirm": "RESET"}); w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if p := reset.Pending(data); p == nil || p.Mode != reset.ModeKeepCopy {
		t.Fatalf("%+v", p)
	}
	select {
	case <-restarted:
	case <-time.After(5 * time.Second):
		t.Fatal("it didn't restart")
	}

	// It can be taken back until the server starts.
	if w := ts.do("DELETE", "/api/reset/request", ts.admin, nil); w.Code != 200 || reset.Pending(data) != nil {
		t.Errorf("%d %s", w.Code, w.Body)
	}

	// Copies are only ever named by what the server made.
	for _, name := range []string{"..", "../..", "wms.db", "before-reset-20990101-000000"} {
		if w := ts.do("POST", "/api/reset/restore", ts.admin, map[string]any{"password": resetPassword, "copy": name}); w.Code == 202 {
			t.Errorf("restore %q", name)
		}
		if w := ts.do("POST", "/api/reset/copies/delete", ts.admin, map[string]any{"password": resetPassword, "copy": name}); w.Code == 200 {
			t.Errorf("delete %q", name)
		}
	}
	if reset.Pending(data) != nil {
		t.Error("a refused restore was kept")
	}
}

// Someone's media in the data folder stops it, however it's asked for.
func TestResetRefusesMediaInTheDataFolder(t *testing.T) {
	ts := newGalleryServer(t)
	ts.s.Restart = func() {}
	data := t.TempDir()
	ts.s.Cfg.DataDir = data
	if _, err := ts.s.Store.CreateLibrary(context.Background(), "Inside", "movies", []string{filepath.Join(data, "movies")}); err != nil {
		t.Fatal(err)
	}
	w := ts.do("POST", "/api/reset", ts.admin, map[string]any{"password": resetPassword, "confirm": "RESET", "delete": true})
	if w.Code != 409 || !strings.Contains(w.Body.String(), "inside the data folder") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	if reset.Pending(data) != nil {
		t.Error("it was asked for anyway")
	}
	var v resetView
	if err := json.Unmarshal(ts.do("GET", "/api/reset", ts.admin, nil).Body.Bytes(), &v); err != nil || v.Blocked == "" {
		t.Errorf("%+v %v", v, err)
	}
}
