// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package tdarr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// These run against a stand-in that answers as the API reference says. They can't say
// that a real Tdarr answers that way: TestLive does, when pointed at one.

func standIn(t *testing.T, key string) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var sent []map[string]any
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key != "" && r.Header.Get("x-api-key") != key {
			w.WriteHeader(401)
			return
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v2/status":
			w.Write([]byte(`{"status":"good","isProduction":true,"os":"linux","version":"2.62.01","uptime":12}`))
		case "GET /api/v2/get-nodes":
			w.Write([]byte(`{"abc":{"_id":"abc","nodeName":"Spare PC","nodePaused":false,"workers":{"w1":{"_id":"w1","file":"/media/a.mkv","percentage":42.5,"mode":"transcode","fps":30,"ETA":"10m"},"w2":{"_id":"w2"}}},"bad":"nonsense"}`))
		case "POST /api/v2/cruddb":
			var body map[string]map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["data"]["collection"] != "LibrarySettingsJSONDB" || body["data"]["mode"] != "getAll" {
				w.WriteHeader(400)
				return
			}
			w.Write([]byte(`[{"_id":"lib1","name":"Films","folder":"/media"},{"_id":"lib2"}]`))
		case "POST /api/v2/scan-files":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			sent = append(sent, body)
			w.Write([]byte(`"OK"`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(s.Close)
	return s, &sent
}

func TestClient(t *testing.T) {
	s, sent := standIn(t, "tapi_secret")
	ctx := context.Background()
	c, err := New(s.URL, "tapi_secret")
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.GetStatus(ctx)
	if err != nil || st.Version != "2.62.01" {
		t.Fatalf("%+v %v", st, err)
	}
	nodes, err := c.GetNodes(ctx)
	if err != nil || len(nodes) != 1 || nodes[0].Name != "Spare PC" || len(nodes[0].Workers) != 2 {
		t.Fatalf("%+v %v", nodes, err)
	}
	if w := nodes[0].Workers[0]; w.File != "/media/a.mkv" || w.Percent != 42.5 {
		t.Fatalf("%+v", w)
	}
	libs, err := c.GetLibraries(ctx)
	if err != nil || len(libs) != 2 || libs[0].Name != "Films" || libs[1].Name != "lib2" {
		t.Fatalf("%+v %v", libs, err)
	}
	if err := c.SendFiles(ctx, "lib1", []string{"/media/a.mkv"}); err != nil {
		t.Fatal(err)
	}
	if err := c.ScanLibrary(ctx, "lib1", "/media"); err != nil {
		t.Fatal(err)
	}
	cfg := (*sent)[0]["data"].(map[string]any)["scanConfig"].(map[string]any)
	if cfg["dbID"] != "lib1" || cfg["mode"] != "scanFolderWatcher" {
		t.Fatalf("%+v", cfg)
	}
}

func TestProblems(t *testing.T) {
	ctx := context.Background()
	s, _ := standIn(t, "tapi_secret")
	c, _ := New(s.URL, "wrong")
	if _, err := c.GetStatus(ctx); !errors.Is(err, ErrKey) {
		t.Fatalf("a wrong key: %v", err)
	}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"hello":"world"}`)) }))
	defer other.Close()
	c, _ = New(other.URL, "")
	if _, err := c.GetStatus(ctx); !errors.Is(err, ErrNotTdarr) {
		t.Fatalf("something else: %v", err)
	}
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`<html>`)) }))
	defer web.Close()
	c, _ = New(web.URL, "")
	if _, err := c.GetStatus(ctx); !errors.Is(err, ErrNotTdarr) {
		t.Fatalf("a web page: %v", err)
	}
	c, _ = New("http://127.0.0.1:1", "")
	if _, err := c.GetStatus(ctx); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("nothing there: %v", err)
	}
}

func TestRedirectsAreNotFollowedWithTheKey(t *testing.T) {
	got := ""
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.Header.Get("x-api-key") }))
	defer other.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, 302) }))
	defer first.Close()
	c, _ := New(first.URL, "tapi_secret")
	if _, err := c.GetStatus(context.Background()); err == nil {
		t.Fatal("a redirect counted as an answer")
	}
	if got != "" {
		t.Fatal("the key went to another address")
	}
}

func TestCleanAddress(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.5:8266":        "http://192.168.1.5:8266",
		" http://tdarr.local/ ":   "http://tdarr.local",
		"https://x.example/tdarr": "https://x.example/tdarr",
	} {
		if got, err := CleanAddress(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "ftp://x", "file:///etc/passwd", "http://user:pw@x", "http://x?a=1", "http://x#y", strings.Repeat("a", 600)} {
		if _, err := CleanAddress(bad); err == nil {
			t.Errorf("%q allowed", bad)
		}
	}
}

func TestMetadataAddressIsRefused(t *testing.T) {
	c, err := New("http://169.254.169.254", "tapi_secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetStatus(context.Background()); !errors.Is(err, ErrUnreachable) {
		t.Fatal(err)
	}
}

func TestTranslate(t *testing.T) {
	cases := []struct{ path, from, to, want string }{
		{"/srv/films/a.mkv", "/srv/films", "/media", "/media/a.mkv"},
		{"/srv/films/x/a.mkv", "/srv/films/", "/media/", "/media/x/a.mkv"},
		{`D:\Films\a.mkv`, `D:\Films`, "/media", "/media/a.mkv"},
		{"/srv/films/a.mkv", "", "", "/srv/films/a.mkv"},
	}
	for _, c := range cases {
		if got, ok := Translate(c.path, c.from, c.to); !ok || got != c.want {
			t.Errorf("%+v: %q %v", c, got, ok)
		}
	}
	if _, ok := Translate("/srv/filmsmore/a.mkv", "/srv/films", "/media"); ok {
		t.Error("a folder with a longer name counted as inside")
	}
}

// TestLive runs against a real Tdarr server when told where one is, for checking this
// client after installing it: WMS_TDARR_URL=http://localhost:8266 WMS_TDARR_KEY=tapi_...
func TestLive(t *testing.T) {
	address := os.Getenv("WMS_TDARR_URL")
	if address == "" {
		t.Skip("set WMS_TDARR_URL to try this against a real Tdarr")
	}
	c, err := New(address, os.Getenv("WMS_TDARR_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st, err := c.GetStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("status: %+v", st)
	nodes, err := c.GetNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("nodes: %+v", nodes)
	libs, err := c.GetLibraries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("libraries: %+v", libs)
}
