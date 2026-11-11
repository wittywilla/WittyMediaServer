// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/sduistore"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

func sduiAccounts(t *testing.T) (ts *testServer, admin, adult, kid string) {
	t.Helper()
	ts = newTestServer(t)
	_, admin = ts.user(t, "owner", auth.KindAdult, true, "")
	_, adult = ts.user(t, "alex", auth.KindAdult, false, "")
	_, kid = ts.user(t, "kit", auth.KindKids, false, "")
	return
}

func TestSDUIRoutesNeedTheRightAccount(t *testing.T) {
	ts, _, adult, kid := sduiAccounts(t)
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/sdui/screens/home"}, {"GET", "/api/v1/sdui/screens/home/diff?since=x"},
		{"GET", "/api/sdui/config"}, {"PUT", "/api/sdui/config"}, {"GET", "/api/sdui/config/history"}, {"POST", "/api/sdui/config/rollback/1"},
		{"GET", "/api/theme"}, {"PUT", "/api/theme/global"}, {"GET", "/api/theme/modules/wittyplay"},
		{"PUT", "/api/theme/modules/wittyplay"}, {"DELETE", "/api/v1/theme/modules/wittyplay"},
	} {
		if w := ts.do(t, c.method, c.path, "", nil); w.Code != 401 {
			t.Errorf("%s %s signed out: %d", c.method, c.path, w.Code)
		}
	}
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/sdui/config"}, {"PUT", "/api/sdui/config"}, {"GET", "/api/sdui/config/history"}, {"POST", "/api/sdui/config/rollback/1"},
		{"PUT", "/api/theme/global"}, {"GET", "/api/theme/modules/wittyplay"}, {"PUT", "/api/theme/modules/wittyplay"}, {"DELETE", "/api/theme/modules/wittyplay"},
	} {
		for _, tok := range []string{adult, kid} {
			if w := ts.do(t, c.method, c.path, tok, nil); w.Code != 403 {
				t.Errorf("%s %s as a non-admin: %d", c.method, c.path, w.Code)
			}
		}
	}
	for _, tok := range []string{adult, kid} {
		if w := ts.do(t, "GET", "/api/sdui/screens/home", tok, nil); w.Code != 200 {
			t.Errorf("screen for any account: %d %s", w.Code, w.Body)
		}
		if w := ts.do(t, "GET", "/api/theme", tok, nil); w.Code != 200 {
			t.Errorf("theme for any account: %d %s", w.Code, w.Body)
		}
	}
}

func TestSDUIScreenRendersFor304AndContext(t *testing.T) {
	ts, admin, adult, kid := sduiAccounts(t)
	w := ts.do(t, "GET", "/api/v1/sdui/screens/home?platform=web&viewport=1440x900", admin, nil)
	if w.Code != 200 {
		t.Fatalf("home: %d %s", w.Code, w.Body)
	}
	etag := w.Header().Get("ETag")
	doc, err := sdui.Unmarshal(w.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if etag == "" || doc.Meta.ETag != etag || doc.Screen != "home" {
		t.Fatalf("etag header %q, meta %q", etag, doc.Meta.ETag)
	}
	if !strings.Contains(w.Body.String(), `"owner"`) && !strings.Contains(w.Body.String(), `"Owner"`) {
		t.Errorf("the greeting isn't bound to the account: %s", w.Body)
	}
	// The same again: 304, with the tag and nothing else.
	w = ts.do(t, "GET", "/api/sdui/screens/home?platform=web&viewport=1440x900", admin, nil, "If-None-Match", etag)
	if w.Code != 304 || w.Header().Get("ETag") != etag || w.Body.Len() != 0 {
		t.Errorf("If-None-Match: %d %q", w.Code, w.Header().Get("ETag"))
	}
	// A phone-sized viewport hides the search bar: another render, another tag. Headers do as the query does.
	w = ts.do(t, "GET", "/api/sdui/screens/home", admin, nil, "If-None-Match", etag, "X-WMS-Viewport", "390x844", "X-WMS-Platform", "ios")
	if w.Code != 200 || w.Header().Get("ETag") == etag || strings.Contains(w.Body.String(), "home-search") {
		t.Errorf("narrow viewport: %d, same tag %v, search shown %v", w.Code, w.Header().Get("ETag") == etag, strings.Contains(w.Body.String(), "home-search"))
	}
	// Settings: administrators' sections only for administrators.
	a := ts.do(t, "GET", "/api/sdui/screens/settings", admin, nil)
	u := ts.do(t, "GET", "/api/sdui/screens/settings", adult, nil)
	if a.Code != 200 || u.Code != 200 || a.Header().Get("ETag") == u.Header().Get("ETag") || len(u.Body.Bytes()) >= len(a.Body.Bytes()) {
		t.Errorf("settings for an admin and an adult should differ: %d %d", a.Code, u.Code)
	}
	if w := ts.do(t, "GET", "/api/sdui/screens/settings", kid, nil); w.Code != 200 {
		t.Errorf("kids: %d", w.Code)
	}
	for _, c := range []struct {
		path string
		want int
	}{
		{"/api/sdui/screens/nosuch", http.StatusNotFound}, {"/api/sdui/screens/Home!", http.StatusBadRequest},
		{"/api/sdui/screens/home?viewport=wide", http.StatusBadRequest}, {"/api/sdui/screens/home?platform=%3Cscript%3E", http.StatusBadRequest},
		{"/api/sdui/screens/home?capabilities=a,b%20c", http.StatusBadRequest}, {"/api/sdui/screens/home?width=-3", http.StatusBadRequest},
	} {
		if w := ts.do(t, "GET", c.path, adult, nil); w.Code != c.want {
			t.Errorf("%s: %d, want %d", c.path, w.Code, c.want)
		}
	}
}

func TestSDUIDiff(t *testing.T) {
	ts, admin, _, _ := sduiAccounts(t)
	w := ts.do(t, "GET", "/api/sdui/screens/home?viewport=1440x900", admin, nil)
	old := w.Header().Get("ETag")
	if w := ts.do(t, "GET", "/api/sdui/screens/home/diff", admin, nil); w.Code != 400 {
		t.Errorf("no since: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/sdui/screens/home/diff?viewport=1440x900&since="+urlq(old), admin, nil); w.Code != 304 {
		t.Errorf("nothing changed: %d", w.Code)
	}
	// Narrower: patches from the render the client has.
	w = ts.do(t, "GET", "/api/sdui/screens/home/diff?viewport=390x844&since="+urlq(old), admin, nil)
	if w.Code != 200 {
		t.Fatalf("diff: %d %s", w.Code, w.Body)
	}
	d := decode[sduiDiff](t, w)
	if d.Document != nil || len(d.Patches) == 0 || d.ETag != w.Header().Get("ETag") || d.Since != old {
		t.Fatalf("diff answer: %+v", d)
	}
	// The patches turn the old render into the new one.
	oldDoc, _ := sdui.Unmarshal(ts.do(t, "GET", "/api/sdui/screens/home?viewport=1440x900", admin, nil).Body.Bytes())
	got, err := sdui.Apply(oldDoc, d.Patches)
	if err != nil {
		t.Fatal(err)
	}
	if sdui.ETag(got) != d.ETag {
		t.Errorf("applied patches give %s, want %s", sdui.ETag(got), d.ETag)
	}
	// A tag the server never gave this account: the whole document.
	w = ts.do(t, "GET", "/api/sdui/screens/home/diff?since="+urlq(`W/"sha256-unknown"`), admin, nil)
	if d := decode[sduiDiff](t, w); w.Code != 200 || d.Document == nil || d.Patches != nil {
		t.Errorf("unknown since: %d %+v", w.Code, d)
	}
}

func urlq(s string) string { return strings.NewReplacer(`"`, "%22", "/", "%2F", " ", "%20").Replace(s) }

func TestSDUIConfigPutNeedsIfMatchAndValidates(t *testing.T) {
	ts, admin, _, _ := sduiAccounts(t)
	w := ts.do(t, "GET", "/api/sdui/config", admin, nil)
	if w.Code != 200 {
		t.Fatalf("config: %d %s", w.Code, w.Body)
	}
	etag := w.Header().Get("ETag")
	cfg := decode[sduistore.Config](t, w)
	if cfg.ETag != etag || cfg.Revision != 1 || len(cfg.Screens) != len(sdui.DefaultScreens()) {
		t.Fatalf("config: rev %d etag %q/%q screens %d", cfg.Revision, cfg.ETag, etag, len(cfg.Screens))
	}
	if w := ts.do(t, "GET", "/api/sdui/config", admin, nil, "If-None-Match", etag); w.Code != 304 {
		t.Errorf("config If-None-Match: %d", w.Code)
	}
	// Change the home screen's title.
	home := cfg.Screens["home"]
	home.Meta.Title = "Welcome"
	cfg.Screens["home"] = home
	body, _ := json.Marshal(cfg)
	if w := ts.do(t, "PUT", "/api/sdui/config", admin, body); w.Code != 428 {
		t.Errorf("no If-Match: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/sdui/config", admin, body, "If-Match", `W/"ui-0-stale"`); w.Code != 412 {
		t.Errorf("stale If-Match: %d", w.Code)
	}
	w = ts.do(t, "PUT", "/api/sdui/config", admin, body, "If-Match", etag)
	if w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	saved := decode[sduistore.Config](t, w)
	if saved.Revision != 2 || saved.ETag == etag || saved.UpdatedBy != "owner" || w.Header().Get("ETag") != saved.ETag {
		t.Errorf("saved: %+v", saved)
	}
	// The tag it was read with is now stale.
	if w := ts.do(t, "PUT", "/api/sdui/config", admin, body, "If-Match", etag); w.Code != 412 {
		t.Errorf("second save with the old tag: %d", w.Code)
	}
	// Invalid documents, unknown fields and bad theme sections are 400.
	bad := func(edit func(c *sduistore.Config)) []byte {
		c := saved
		c.Screens = map[string]sdui.Document{}
		for k, v := range saved.Screens {
			c.Screens[k] = v
		}
		edit(&c)
		b, _ := json.Marshal(c)
		return b
	}
	for name, b := range map[string][]byte{
		"root not a Screen": bad(func(c *sduistore.Config) { d := c.Screens["home"]; d.Root.Type = sdui.Text; c.Screens["home"] = d }),
		"name mismatch":     bad(func(c *sduistore.Config) { c.Screens["other"] = c.Screens["home"] }),
		"unknown token":     bad(func(c *sduistore.Config) { c.Theme.Tokens = map[string]string{"sparkle": "#ffffff"} }),
		"bad colour": bad(func(c *sduistore.Config) {
			c.Theme.Modules = map[string]map[string]string{"wittyplay": {"accent": "red;}"}}
		}),
		"unknown field": []byte(`{"version":1,"screens":{},"surprise":true}`),
		"not JSON":      []byte(`{`),
	} {
		if w := ts.do(t, "PUT", "/api/sdui/config", admin, b, "If-Match", saved.ETag); w.Code != 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	// The rendered screen follows the saved configuration.
	if w := ts.do(t, "GET", "/api/sdui/screens/home", admin, nil); !strings.Contains(w.Body.String(), `"Welcome"`) {
		t.Errorf("the saved title isn't served")
	}
}

func TestSDUIHistoryAndRollback(t *testing.T) {
	ts, admin, _, _ := sduiAccounts(t)
	for _, acc := range []string{"#112233", "#445566"} {
		if w := ts.do(t, "PUT", "/api/theme/modules/wittyplay", admin, map[string]any{"tokens": map[string]string{"accent": acc}}); w.Code != 200 {
			t.Fatalf("module: %d %s", w.Code, w.Body)
		}
	}
	w := ts.do(t, "GET", "/api/sdui/config/history", admin, nil)
	var h struct {
		Current   int64                `json:"current"`
		ETag      string               `json:"etag"`
		Revisions []sduistore.Revision `json:"revisions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &h); err != nil || w.Code != 200 {
		t.Fatalf("history: %d %s", w.Code, w.Body)
	}
	if h.Current != 3 || len(h.Revisions) != 3 || h.Revisions[0].Revision != 3 || !h.Revisions[0].Current {
		t.Fatalf("history: %+v", h)
	}
	if w := ts.do(t, "POST", "/api/sdui/config/rollback/2", admin, nil, "If-Match", `W/"ui-1-old"`); w.Code != 412 {
		t.Errorf("rollback with a stale If-Match: %d", w.Code)
	}
	for path, want := range map[string]int{"/api/sdui/config/rollback/99": 404, "/api/sdui/config/rollback/x": 400, "/api/sdui/config/rollback/0": 400} {
		if w := ts.do(t, "POST", path, admin, nil); w.Code != want {
			t.Errorf("%s: %d, want %d", path, w.Code, want)
		}
	}
	w = ts.do(t, "POST", "/api/sdui/config/rollback/2", admin, nil, "If-Match", h.ETag)
	if w.Code != 200 {
		t.Fatalf("rollback: %d %s", w.Code, w.Body)
	}
	if c := decode[sduistore.Config](t, w); c.Revision != 4 || c.Theme.Modules["wittyplay"]["accent"] != "#112233" {
		t.Errorf("after rollback: rev %d %v", c.Revision, c.Theme.Modules)
	}
}

func TestSDUIConfigFileIsInTheDataFolder(t *testing.T) {
	ts, admin, _, _ := sduiAccounts(t)
	ts.do(t, "GET", "/api/sdui/screens/home", admin, nil)
	if _, err := os.Stat(filepath.Join(ts.Cfg.DataDir, sduistore.FileName)); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(ts.Cfg.DataDir, sduistore.HistoryDir)); err != nil {
		t.Error(err)
	}
}
