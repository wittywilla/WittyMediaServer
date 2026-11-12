// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
	"github.com/wittywilla/wittymodule"
)

// unsignedRelease is a release archive of a module this server doesn't know, with no signature.
func unsignedRelease(t *testing.T) (name string, archive []byte) {
	t.Helper()
	man := wittymodule.Manifest{ID: "wittynew", Name: "Witty New", Version: "0.1.0", Repo: "someone/WittyNew", Delivery: wittymodule.DeliveryWeb,
		Entry: "index.js", API: modmgr.SDUIAPI, MinServer: "0.4.0", MaxServer: "0.5.x", Permissions: []string{wittymodule.PermissionNetwork}}
	raw, _ := json.Marshal(man)
	ui := `{"format":"wittymodule-sdui","formatVersion":1,"module":"wittynew","catalog":2,"theme":{"scope":"wittynew"},
"pages":[{"id":"wittynew","kind":"custom","title":"New things","sections":[{"id":"recent","component":"Shelf","source":{"type":"recentlyAdded"}}]}],
"navigation":[{"id":"wittynew","type":"page","page":"wittynew"}]}`
	top := man.Top(runtime.GOOS, runtime.GOARCH)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for n, b := range map[string][]byte{wittymodule.ManifestFile: raw, "index.js": []byte("export default 1"), sdui.ModuleFile: []byte(ui)} {
		if err := tw.WriteHeader(&tar.Header{Name: top + "/" + n, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(b))}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write(b)
	}
	_ = tw.Close()
	_ = gz.Close()
	return top + ".tar.gz", buf.Bytes()
}

func multipartBody(t *testing.T, fields map[string][]byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for n, b := range fields {
		fw, err := mw.CreateFormFile(n, n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write(b)
	}
	_ = mw.Close()
	return buf.Bytes(), mw.FormDataContentType()
}

// Installing from a file, every step an administrator takes and every refusal on the way:
// only an administrator sends one, the go-ahead names everything bypassed, the password is asked
// again, and each override is written down and can be taken back.
func TestInstallingAModuleFromAFile(t *testing.T) {
	t.Setenv(modmgr.LockForks, "")
	if why := modmgr.ForkRefusal(); why != "" {
		t.Skip(why)
	}
	ts := newTestServer(t)
	ts.Modules = modmgr.New(modmgr.Config{Dir: filepath.Join(t.TempDir(), "modules"), Server: "0.4.0", Builtins: modmgr.ServerManifests(), Known: true,
		UIChecks: UIChecks(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	t.Cleanup(ts.Modules.Close)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "adult", "adult", false, "")
	name, archive := unsignedRelease(t)

	body, ct := multipartBody(t, map[string][]byte{"archive": archive})
	if w := ts.do(t, "POST", "/api/module-uploads", adult, body, "Content-Type", ct); w.Code != 403 {
		t.Fatalf("an adult uploaded: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/module-uploads", admin, []byte("not a form"), "Content-Type", "text/plain"); w.Code != 400 {
		t.Errorf("not a form: %d %s", w.Code, w.Body)
	}
	// Not a release archive: answered, nothing kept.
	junk, jct := multipartBody(t, map[string][]byte{"archive": []byte("hello")})
	w := ts.do(t, "POST", "/api/module-uploads", admin, junk, "Content-Type", jct)
	var refused modmgr.UploadInfo
	_ = json.Unmarshal(w.Body.Bytes(), &refused)
	if w.Code != 200 || refused.Refusal == "" || refused.Token != "" {
		t.Errorf("junk: %d %s", w.Code, w.Body)
	}

	// The archive's own name says what it is.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("archive", name)
	_, _ = fw.Write(archive)
	_ = mw.Close()
	w = ts.do(t, "POST", "/api/module-uploads", admin, buf.Bytes(), "Content-Type", mw.FormDataContentType())
	var info modmgr.UploadInfo
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil || w.Code != 200 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	want := []string{modmgr.OverrideLocalFile, modmgr.OverrideCustom, modmgr.OverrideUnsigned}
	if info.Token == "" || info.Refusal != "" || info.NeedsOverride == nil || !slices.Equal(info.NeedsOverride.Kinds, want) {
		t.Fatalf("staged: %+v", info)
	}
	route := "/api/module-uploads/" + info.Token + "/install"
	full := map[string]any{"accept": want, "typed": "wittynew"}

	if w := ts.do(t, "POST", route, adult, map[string]any{"override": full, "password": "correct-horse-1"}); w.Code != 403 {
		t.Errorf("an adult installed: %d", w.Code)
	}
	if w := ts.do(t, "POST", route, admin, map[string]any{"password": "correct-horse-1"}); w.Code != 400 {
		t.Errorf("no go-ahead at all: %d %s", w.Code, w.Body)
	}
	w = ts.do(t, "POST", route, admin, map[string]any{"override": full})
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 403 || out["code"] != "password-needed" {
		t.Errorf("no password: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", route, admin, map[string]any{"override": full, "password": "wrong"}); w.Code != 403 {
		t.Errorf("wrong password: %d %s", w.Code, w.Body)
	}
	for what, o := range map[string]map[string]any{
		"some of it":       {"accept": want[:2], "typed": "wittynew"},
		"without the name": {"accept": want},
	} {
		w := ts.do(t, "POST", route, admin, map[string]any{"override": o, "password": "correct-horse-1"})
		out = map[string]any{}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		if w.Code != 400 || out["code"] != "bad-override" || out["needsOverride"] == nil {
			t.Errorf("%s: %d %s", what, w.Code, w.Body)
		}
	}
	if auditRows(t, ts, "module-override") != 0 {
		t.Error("nothing was installed, so nothing should be written down")
	}
	if w := ts.do(t, "POST", route, admin, map[string]any{"override": full, "password": "correct-horse-1"}); w.Code != 202 {
		t.Fatalf("the full go-ahead: %d %s", w.Code, w.Body)
	}
	var st modmgr.Status
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		for _, s := range ts.Modules.Report().Modules {
			if s.ID == "wittynew" {
				st = s
			}
		}
		if st.Version == "0.1.0" && st.Phase == modmgr.PhaseIdle {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never installed: %+v", st)
		}
	}
	if st.Override == nil || !slices.Equal(st.Override.Kinds, want) || st.Override.By != "owner" {
		t.Fatalf("the override: %+v", st.Override)
	}
	if auditRows(t, ts, "module-override") != 1 || auditRows(t, ts, "module-upload") != 2 {
		t.Errorf("written down: %d overrides, %d uploads", auditRows(t, ts, "module-override"), auditRows(t, ts, "module-upload"))
	}
	// The token is used up.
	if w := ts.do(t, "DELETE", "/api/module-uploads/"+info.Token, admin, nil); w.Code != 404 {
		t.Errorf("a used token: %d", w.Code)
	}

	// The audit log, for administrators.
	if w := ts.do(t, "GET", "/api/modules/audit", adult, nil); w.Code != 403 {
		t.Errorf("an adult read the audit log: %d", w.Code)
	}
	w = ts.do(t, "GET", "/api/modules/audit?limit=5", admin, nil)
	var log []ModuleAudit
	if err := json.Unmarshal(w.Body.Bytes(), &log); err != nil || w.Code != 200 || len(log) == 0 {
		t.Fatalf("audit: %d %s", w.Code, w.Body)
	}
	if log[0].Action != "module-override" || log[0].Module != "wittynew" || log[0].User != "owner" {
		t.Errorf("newest first, naming the module: %+v", log[0])
	}

	// Taking it back asks for the password too, and takes off a module this server didn't know.
	if w := ts.do(t, "DELETE", "/api/modules/wittynew/override", adult, map[string]any{"password": "correct-horse-1"}); w.Code != 403 {
		t.Errorf("an adult took it back: %d", w.Code)
	}
	if w := ts.do(t, "DELETE", "/api/modules/wittynew/override", admin, map[string]any{}); w.Code != 403 {
		t.Errorf("no password: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "DELETE", "/api/modules/wittynew/override", admin, map[string]any{"password": "correct-horse-1"}); w.Code != 200 {
		t.Fatalf("remove override: %d %s", w.Code, w.Body)
	}
	for _, s := range ts.Modules.Report().Modules {
		if s.ID == "wittynew" {
			t.Errorf("still here: %+v", s)
		}
	}
	if auditRows(t, ts, "module-override-removed") != 1 {
		t.Error("taking it back should be written down")
	}
	if w := ts.do(t, "DELETE", "/api/modules/wittynew/override", admin, map[string]any{"password": "correct-horse-1"}); w.Code != 404 {
		t.Errorf("a module that's gone: %d %s", w.Code, w.Body)
	}
}

// Another administrator can't use, or throw away, someone's upload, and a discarded one is gone.
func TestAnUploadIsItsAdministrators(t *testing.T) {
	ts := newTestServer(t)
	ts.Modules = modmgr.New(modmgr.Config{Dir: filepath.Join(t.TempDir(), "modules"), Server: "0.4.0", Builtins: modmgr.ServerManifests(), Known: true,
		UIChecks: UIChecks(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	t.Cleanup(ts.Modules.Close)
	_, owner := ts.user(t, "owner", "adult", true, "")
	_, other := ts.user(t, "other", "adult", true, "")
	name, archive := unsignedRelease(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("archive", name)
	_, _ = fw.Write(archive)
	_ = mw.Close()
	w := ts.do(t, "POST", "/api/module-uploads", owner, buf.Bytes(), "Content-Type", mw.FormDataContentType())
	var info modmgr.UploadInfo
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil || w.Code != 200 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	if info.Token == "" {
		t.Skip(info.Refusal) // a system where someone else's code can't run at all
	}
	if w := ts.do(t, "DELETE", "/api/module-uploads/"+info.Token, other, nil); w.Code != 404 {
		t.Errorf("another administrator discarded it: %d", w.Code)
	}
	o := map[string]any{"accept": info.NeedsOverride.Kinds, "typed": "wittynew"}
	if w := ts.do(t, "POST", "/api/module-uploads/"+info.Token+"/install", other, map[string]any{"override": o, "password": "correct-horse-1"}); w.Code != 404 {
		t.Errorf("another administrator installed it: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "DELETE", "/api/module-uploads/"+info.Token, owner, nil); w.Code != 204 {
		t.Fatalf("discard: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/module-uploads/"+info.Token+"/install", owner, map[string]any{"override": o, "password": "correct-horse-1"}); w.Code != 404 {
		t.Errorf("a discarded upload: %d %s", w.Code, w.Body)
	}
	// Nothing held for a module that has nothing held.
	if w := ts.do(t, "DELETE", "/api/modules/wittyphotos/pending", owner, nil); w.Code == 200 || w.Code == 500 {
		t.Errorf("nothing pending: %d %s", w.Code, w.Body)
	}
}
