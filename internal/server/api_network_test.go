// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/config"
)

// freePort is a port nothing listens on, a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return p
}

// Settings → Network: checked, saved in the data folder's settings.ini, for administrators only,
// and never where something stronger would win.
func TestNetworkPort(t *testing.T) {
	if inDocker() {
		t.Skip("in a container the port is the published one, and can't be changed here")
	}
	t.Setenv("WMS_LISTEN", "")
	ts := newTestServer(t)
	ts.Cfg.Listen, ts.Cfg.ListenFrom = "127.0.0.1:"+strconv.Itoa(freePort(t)), config.FromDefault
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, viewer := ts.user(t, "viewer", "adult", false, "")

	if w := ts.do(t, "GET", "/api/network", viewer, nil); w.Code != 403 {
		t.Fatalf("someone who isn't an administrator: %d", w.Code)
	}
	w := ts.do(t, "GET", "/api/network", admin, nil)
	var v networkView
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &v) != nil || v.Locked != "" || v.From != config.FromDefault {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	check := func(port int) portCheck {
		t.Helper()
		w := ts.do(t, "POST", "/api/network/check", admin, map[string]int{"port": port})
		var c portCheck
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &c) != nil {
			t.Fatalf("check %d: %d %s", port, w.Code, w.Body)
		}
		return c
	}
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	for _, c := range []struct {
		port int
		why  string
	}{{0, "number from"}, {70000, "number from"}, {6000, "Browsers refuse"}, {busy.Addr().(*net.TCPAddr).Port, "Something else"}} {
		got := check(c.port)
		if got.OK || len(got.Problems) == 0 || !strings.Contains(got.Problems[0], c.why) {
			t.Errorf("port %d: %+v, want %q", c.port, got, c.why)
		}
		if w := ts.do(t, "PUT", "/api/network", admin, map[string]int{"port": c.port}); w.Code != 400 {
			t.Errorf("port %d was saved: %d", c.port, w.Code)
		}
	}
	if got := check(v.Port); !got.OK || !got.Current {
		t.Errorf("the port in use now: %+v", got)
	}

	want := freePort(t)
	if got := check(want); !got.OK {
		t.Fatalf("a free port: %+v", got)
	}
	w = ts.do(t, "PUT", "/api/network", admin, map[string]int{"port": want})
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &v) != nil || v.Next != want || v.Port == want {
		t.Fatalf("saving: %d %s", w.Code, w.Body)
	}
	if s, _ := config.ReadSettings(config.DataSettingsFile(ts.Cfg.DataDir)); s.Port != want {
		t.Errorf("settings.ini says %+v", s)
	}
	// And the next start uses it.
	if c, err := config.ParseOnly([]string{"-data", ts.Cfg.DataDir, "-config", emptyConfig(t)}); err != nil || c.Listen != ":"+strconv.Itoa(want) {
		t.Errorf("next start: %+v %v", c, err)
	}

	// Something stronger sets it: the page says so, and saves nothing.
	ts.Cfg.ListenFrom, ts.Cfg.ConfigFile = config.FromConfig, "wms.json"
	if w := ts.do(t, "PUT", "/api/network", admin, map[string]int{"port": freePort(t)}); w.Code != 409 || !strings.Contains(w.Body.String(), "wms.json") {
		t.Errorf("with listen in wms.json: %d %s", w.Code, w.Body)
	}

	if w := ts.do(t, "POST", "/api/network/restart", admin, nil); w.Code != 409 {
		t.Errorf("a server that can't restart itself: %d", w.Code)
	}
	restarted := make(chan struct{}, 1)
	ts.Restart = func() { restarted <- struct{}{} }
	if w := ts.do(t, "POST", "/api/network/restart", viewer, nil); w.Code != 403 {
		t.Errorf("someone who isn't an administrator restarted it: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/network/restart", admin, nil); w.Code != 202 {
		t.Fatalf("restart: %d %s", w.Code, w.Body)
	}
	<-restarted
}

func emptyConfig(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// The page may ask one thing of another port on its own host, so it can follow the server there,
// and a made-up Host header can't add anything else to the policy.
func TestAppPolicyFollowsThePort(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "192.168.1.20:9797"
	if got := appCSPFor(r); !strings.Contains(got, "connect-src 'self' http://192.168.1.20:*/api/health;") {
		t.Errorf("%s", got)
	}
	for _, host := range []string{"evil; script-src *", "a b", "[::1]:9797", "x'y", ""} {
		r.Host = host
		if got := appCSPFor(r); got != appCSP {
			t.Errorf("%q made the policy %s", host, got)
		}
	}
}
