// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymodule"
)

// olderStarter writes a signed catalogue whose only module, WittyInk 0.2.0, was made for 0.6 (as
// WittyPlay 0.3.6 was when first-run setup put it on a 0.8.0-alpha server), and answers a manager
// for a 0.8.0-alpha server that reads it.
func olderStarter(t *testing.T, ts *testServer) *modmgr.Manager {
	t.Helper()
	pub, key, _ := ed25519.GenerateKey(nil)
	starter := filepath.Join(t.TempDir(), "starter")
	dir := filepath.Join(t.TempDir(), "modules")
	sig := func(b []byte) []byte { return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, b)) + "\n") }
	hash := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	const id, name, ver = "wittyink", "WittyInk", "0.2.0"
	man := wittymodule.Manifest{ID: id, Name: name, Version: ver, Repo: "wittywilla/WittyInk", Delivery: wittymodule.DeliveryWeb, Entry: "index.js",
		API: modmgr.MinAPI, MinServer: modmgr.MinModuleServer, MaxServer: "0.6.x", License: "MIT"}
	raw, _ := json.Marshal(man)
	files := map[string][]byte{wittymodule.ManifestFile: raw, "index.js": []byte("export const v = 1\n")}
	top := wittymodule.WebTopFolder(id, ver)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for n, b := range files {
		_ = tw.WriteHeader(&tar.Header{Name: top + "/" + n, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(b))})
		_, _ = tw.Write(b)
	}
	_ = tw.Close()
	_ = gz.Close()
	archive := buf.Bytes()
	archiveName, manName, sumsName := wittymodule.WebArchiveName(id, ver), wittymodule.ManifestName(id, ver), wittymodule.SumsName(id, ver)
	var list strings.Builder
	fmt.Fprintf(&list, "%s  %s\n%s  %s\n", hash(archive), archiveName, hash(raw), manName)
	for n, b := range files {
		fmt.Fprintf(&list, "%s  %s/%s\n", hash(b), top, n)
	}
	sums := []byte(list.String())
	vdir := filepath.Join(starter, id, ver)
	if err := os.MkdirAll(vdir, 0o755); err != nil {
		t.Fatal(err)
	}
	for n, b := range map[string][]byte{archiveName: archive, manName: raw, sumsName: sums, sumsName + ".sig": sig(sums)} {
		if err := os.WriteFile(filepath.Join(vdir, n), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	at := func(n string) string { return id + "/" + ver + "/" + n }
	cat := wittymodule.Catalog{Format: wittymodule.CatalogFormat, Name: "A store", Serial: 1, Generated: "2026-11-01T12:00:00Z", Modules: []wittymodule.CatalogModule{{
		ID: id, Name: name, Summary: "Does a thing.", Publisher: wittymodule.Publisher{Name: "Someone"}, Repo: "wittywilla/WittyInk",
		Versions: []wittymodule.CatalogVersion{{Version: ver, Delivery: wittymodule.DeliveryWeb, API: modmgr.MinAPI, MinServer: modmgr.MinModuleServer, MaxServer: "0.6.x", License: "MIT", Released: "2026-11-01",
			Sums: at(sumsName), Sig: at(sumsName + ".sig"), Manifest: at(manName),
			Platforms: map[string]wittymodule.CatalogFile{wittymodule.PlatformAll: {URL: at(archiveName), Size: int64(len(archive)), SHA256: hash(archive)}}}},
	}}}
	craw, _ := json.Marshal(cat)
	_ = os.WriteFile(filepath.Join(starter, wittymodule.CatalogName), craw, 0o644)
	_ = os.WriteFile(filepath.Join(starter, wittymodule.CatalogName+".sig"), sig(craw), 0o644)
	m := modmgr.New(modmgr.Config{Dir: dir, Server: "0.8.0-alpha", Starter: starter, Keys: []ed25519.PublicKey{pub}, Log: slog.New(slog.DiscardHandler)})
	t.Cleanup(m.Close)
	ts.Modules = m
	ts.ModuleStores = modmgr.NewStores(modmgr.StoresConfig{Manager: m, Dir: dir, Log: slog.New(slog.DiscardHandler)})
	return m
}

func inkStatus(m *modmgr.Manager) modmgr.Status {
	for _, st := range m.Report().Modules {
		if st.ID == "wittyink" {
			return st
		}
	}
	return modmgr.Status{}
}

// Each route that installs refuses a version made for an older server with code older-server,
// naming it, until the body says confirmOlder; then it installs, and the audit log says so.
func TestModuleRoutesRefuseOlderServerUntilConfirmed(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	m := olderStarter(t, ts)

	refused := func(method, path, body string) {
		t.Helper()
		w := ts.do(t, method, path, admin, []byte(body))
		var got struct {
			Code  string `json:"code"`
			Older []struct {
				Module, Name, Version, MadeFor, Server, Warning string
			} `json:"older"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if w.Code != 409 || got.Code != "older-server" || len(got.Older) != 1 || got.Older[0].Version != "0.2.0" || got.Older[0].MadeFor != "0.6.x" ||
			!strings.Contains(got.Older[0].Warning, "WittyInk 0.2.0 was made for WittyMediaServer 0.6; this server is 0.8.0-alpha") {
			t.Errorf("%s %s %s: %d %s", method, path, body, w.Code, w.Body)
		}
	}
	refused("POST", "/api/modulestore/install", `{"id":"wittyink","confirm":true}`)
	refused("POST", "/api/modules/wittyink/add", ``)
	refused("POST", "/api/modules/wittyink/add", `{"confirmOlder":false}`)
	refused("POST", "/api/modules/wittyink/install", ``)
	refused("POST", "/api/modules/wittyink/install", `{"version":"0.2.0","confirm":true}`)
	if w := ts.do(t, "POST", "/api/modules/wittyink/latest", admin, nil); w.Code >= 300 {
		t.Errorf("latest: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", "/api/modules/installed", admin, []byte(`{"installed":["wittyink"]}`)); w.Code != 200 {
		t.Errorf("setup's set: %d %s", w.Code, w.Body)
	}
	deadline := time.Now().Add(10 * time.Second)
	for inkStatus(m).Error == "" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if st := inkStatus(m); st.Installed || !strings.Contains(st.Error, "made for WittyMediaServer 0.6") {
		t.Fatalf("installed without confirming, or no reason: %+v", st)
	}

	// The store's own view says what it is.
	w := ts.do(t, "GET", "/api/modulestore", admin, nil)
	if !strings.Contains(w.Body.String(), `"kind":"older-server"`) || !strings.Contains(w.Body.String(), "No version of WittyInk for this server is published yet.") {
		t.Errorf("the store view: %s", w.Body)
	}

	// Confirmed, it installs.
	if w := ts.do(t, "POST", "/api/modulestore/install", admin, []byte(`{"id":"wittyink","confirm":true,"confirmOlder":true}`)); w.Code != 202 {
		t.Fatalf("confirmed: %d %s", w.Code, w.Body)
	}
	deadline = time.Now().Add(30 * time.Second)
	for !inkStatus(m).Installed && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	st := inkStatus(m)
	if !st.Installed || !st.OlderServer || st.OlderWarning == "" {
		t.Fatalf("after confirming: %+v", st)
	}
	if auditRows(t, ts, "module-older-server") != 1 {
		t.Error("the audit log should say an older-server version was installed on purpose")
	}
	w = ts.do(t, "GET", "/api/modules", admin, nil)
	if !strings.Contains(w.Body.String(), `"olderServer":true`) || !strings.Contains(w.Body.String(), `"olderWarning":"WittyInk 0.2.0 was made for WittyMediaServer 0.6`) {
		t.Errorf("the modules list: %s", w.Body)
	}
}

// /add with confirmOlder installs it too.
func TestModuleAddWithConfirmOlder(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	m := olderStarter(t, ts)
	if w := ts.do(t, "POST", "/api/modules/wittyink/add", admin, []byte(`{"confirmOlder":true}`)); w.Code != 202 {
		t.Fatalf("add: %d %s", w.Code, w.Body)
	}
	deadline := time.Now().Add(30 * time.Second)
	for !inkStatus(m).Installed && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if st := inkStatus(m); !st.Installed || !st.OlderServer {
		t.Fatalf("after: %+v", st)
	}
}
