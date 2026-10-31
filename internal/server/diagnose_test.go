package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Every Diagnoser route is for administrators: signed out is 401, anyone else 403.
func TestDiagnoseRoutesAreAdminOnly(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	_, adult := ts.user(t, "adult", "adult", false, "")
	_, kid := ts.user(t, "kid", "kids", false, "")
	routes := []struct{ method, path string }{
		{"GET", "/api/diagnose/targets"},
		{"GET", "/api/diagnose/browse"},
		{"POST", "/api/diagnose/disc"},
		{"POST", "/api/diagnose/storage"},
		{"GET", "/api/diagnose/jobs"},
		{"GET", "/api/diagnose/jobs/diag-x"},
		{"DELETE", "/api/diagnose/jobs/diag-x"},
		{"GET", "/api/diagnose/jobs/diag-x/report"},
	}
	for _, rt := range routes {
		if w := ts.do(t, rt.method, rt.path, "", map[string]any{}); w.Code != 401 {
			t.Errorf("%s %s signed out: %d", rt.method, rt.path, w.Code)
		}
		for _, tok := range []string{adult, kid} {
			if w := ts.do(t, rt.method, rt.path, tok, map[string]any{}); w.Code != 403 {
				t.Errorf("%s %s as someone else: %d", rt.method, rt.path, w.Code)
			}
		}
		if w := ts.do(t, rt.method, rt.path, admin, map[string]any{}); w.Code == 401 || w.Code == 403 {
			t.Errorf("%s %s as an administrator: %d", rt.method, rt.path, w.Code)
		}
	}
}

func TestDiagnoseDiscPathsAreChecked(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	top, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(top, "a")
	outside := filepath.Join(top, "ab")
	for _, d := range []string{lib, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ts.library(t, "movies", lib)
	secret := filepath.Join(outside, "secret.iso")
	if err := os.WriteFile(secret, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "disc.iso"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sep := string(filepath.Separator)
	refused := []string{
		secret,
		outside,
		top,
		lib + sep + ".." + sep + "ab" + sep + "secret.iso",
		"relative.iso",
		"x\x00y",
	}
	if os.PathSeparator == '\\' {
		refused = append(refused, `\\?\`+secret, `\\?\`+filepath.Join(lib, "disc.iso"), `\\.\PhysicalDrive0`, filepath.Join(lib, "disc.iso")+`:stream`, `\\localhost\c$\Windows`)
	} else {
		refused = append(refused, "/etc/passwd")
	}
	for _, p := range refused {
		w := ts.do(t, "POST", "/api/diagnose/disc", admin, map[string]any{"path": p})
		if w.Code != 403 {
			t.Errorf("POST disc %q: %d %s", p, w.Code, w.Body)
		}
		if w := ts.do(t, "GET", "/api/diagnose/browse?path="+url.QueryEscape(p), admin, nil); w.Code != 403 {
			t.Errorf("browse %q: %d %s", p, w.Code, w.Body)
		}
	}
	// Browsing: only folders and disc images, and only up to the top of the place.
	if err := os.WriteFile(filepath.Join(lib, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(lib, "sub", "VIDEO_TS"), 0o755); err != nil {
		t.Fatal(err)
	}
	var roots diagnoseListing
	if w := ts.do(t, "GET", "/api/diagnose/browse", admin, nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	} else if err := json.Unmarshal(w.Body.Bytes(), &roots); err != nil {
		t.Fatal(err)
	}
	if len(roots.Roots) != 2 || roots.Roots[0].Path != lib || roots.Roots[1].Kind != "data" {
		t.Errorf("%+v", roots.Roots)
	}
	var l diagnoseListing
	if w := ts.do(t, "GET", "/api/diagnose/browse?path="+url.QueryEscape(lib), admin, nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	} else if err := json.Unmarshal(w.Body.Bytes(), &l); err != nil {
		t.Fatal(err)
	}
	if l.Parent != nil {
		t.Errorf("up out of the library: %q", *l.Parent)
	}
	names := map[string]diagnoseEntry{}
	for _, e := range l.Entries {
		names[e.Name] = e
	}
	if _, ok := names["notes.txt"]; ok || names["disc.iso"].Kind != "disc" || !names["sub"].Dir {
		t.Errorf("%+v", l.Entries)
	}
	var sub diagnoseListing
	w := ts.do(t, "GET", "/api/diagnose/browse?path="+url.QueryEscape(filepath.Join(lib, "sub")), admin, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &sub); err != nil {
		t.Fatal(err)
	}
	if sub.Parent == nil || *sub.Parent != lib || len(sub.Entries) != 1 || sub.Entries[0].Disc != "dvd" {
		t.Errorf("%+v", sub)
	}
	// Storage takes IDs only: a path is simply not one.
	for _, target := range []string{lib, secret, "lib:99:0", "data:nope", "../x"} {
		if w := ts.do(t, "POST", "/api/diagnose/storage", admin, map[string]any{"targets": []string{target}}); w.Code != 400 {
			t.Errorf("storage %q: %d %s", target, w.Code, w.Body)
		}
	}
	if w := ts.do(t, "POST", "/api/diagnose/storage", admin, map[string]any{"targets": []string{}}); w.Code != 400 {
		t.Errorf("no targets: %d", w.Code)
	}
}

func waitDiagnose(t *testing.T, ts *testServer, id, admin string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		w := ts.do(t, "GET", "/api/diagnose/jobs/"+id, admin, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		var v map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		if st := v["state"]; (st == "done" || st == "cancelled") && v["saved"] == true {
			return v
		}
		if time.Now().After(deadline) {
			t.Fatalf("still %v", v["state"])
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDiagnoseWholeStorageCheckOverTheAPI(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	top, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(top, "Private Folder")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "Private Name.mkv"), make([]byte, 2<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "Empty Private.mkv"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	id := ts.library(t, "movies", lib)

	var targets diagnoseTargets
	if w := ts.do(t, "GET", "/api/diagnose/targets", admin, nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	} else if err := json.Unmarshal(w.Body.Bytes(), &targets); err != nil {
		t.Fatal(err)
	}
	if len(targets.Libraries) != 1 || targets.Libraries[0].ID != id || len(targets.Libraries[0].Folders) != 1 {
		t.Fatalf("%+v", targets)
	}
	folder := targets.Libraries[0].Folders[0].ID
	w := ts.do(t, "POST", "/api/diagnose/storage", admin, map[string]any{"targets": []string{folder, "data:data"}, "readTest": true, "writeTest": true, "walk": true})
	if w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var job struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	v := waitDiagnose(t, ts, job.ID, admin)
	if v["state"] != "done" || len(v["findings"].([]any)) == 0 {
		t.Fatalf("%+v", v)
	}
	ents, err := os.ReadDir(lib)
	if err != nil || len(ents) != 2 {
		t.Errorf("the library changed: %v %v", ents, err)
	}
	if left, _ := filepath.Glob(filepath.Join(ts.Cfg.DataDir, ".wms-diag-*")); len(left) != 0 {
		t.Errorf("left behind: %v", left)
	}

	jw := ts.do(t, "GET", "/api/diagnose/jobs/"+job.ID+"/report?format=json", admin, nil)
	if jw.Code != 200 || !strings.Contains(jw.Body.String(), "Empty Private.mkv") || strings.Contains(jw.Body.String(), "Private Folder\\\\") {
		t.Errorf("json report: %d %s", jw.Code, jw.Body)
	}
	if !strings.Contains(jw.Header().Get("Content-Disposition"), "attachment") {
		t.Error("should download")
	}
	for _, format := range []string{"json", "txt"} {
		r := ts.do(t, "GET", "/api/diagnose/jobs/"+job.ID+"/report?redact=1&format="+format, admin, nil)
		if r.Code != 200 {
			t.Fatalf("%s: %d", format, r.Code)
		}
		body := r.Body.String()
		for _, no := range []string{"Private", top} {
			if strings.Contains(body, no) {
				t.Errorf("redacted %s holds %q:\n%s", format, no, body)
			}
		}
	}
	if txt := ts.do(t, "GET", "/api/diagnose/jobs/"+job.ID+"/report?format=txt", admin, nil); !strings.HasPrefix(txt.Body.String(), "WittyDiagnoser report") {
		t.Errorf("txt: %s", txt.Body)
	}
	lw := ts.do(t, "GET", "/api/diagnose/jobs", admin, nil)
	var list struct {
		Jobs []map[string]any `json:"jobs"`
	}
	if err := json.Unmarshal(lw.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Jobs) != 1 || list.Jobs[0]["id"] != job.ID || list.Jobs[0]["findings"] != nil {
		t.Errorf("%+v", list.Jobs)
	}
	// A report is in the support bundle, with names hidden.
	files := ts.BundleExtras()
	if len(files) != 1 || strings.Contains(string(files[0].Data), "Private") {
		t.Errorf("bundle: %d files", len(files))
	}
	if w := ts.do(t, "GET", "/api/diagnose/jobs/diag-none", admin, nil); w.Code != 404 {
		t.Errorf("no such job: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/diagnose/jobs/..%2f..%2fwms/report", admin, nil); w.Code != 404 {
		t.Errorf("a path as an ID: %d", w.Code)
	}
}

func TestDiagnoseDiscCheckOverTheAPI(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	lib, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ts.library(t, "discs", lib)
	img := filepath.Join(lib, "Not A Disc.iso")
	if err := os.WriteFile(img, make([]byte, 128<<10), 0o644); err != nil {
		t.Fatal(err)
	}
	w := ts.do(t, "POST", "/api/diagnose/disc", admin, map[string]any{"path": img, "crawl": true, "damage": true})
	if w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var job struct {
		ID      string `json:"id"`
		Subject string `json:"subject"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if job.Subject != "Not A Disc.iso" {
		t.Errorf("%+v", job)
	}
	v := waitDiagnose(t, ts, job.ID, admin)
	if v["worst"] != "problem" {
		t.Errorf("%+v", v)
	}
	if w := ts.do(t, "DELETE", "/api/diagnose/jobs/"+job.ID, admin, nil); w.Code != 200 {
		t.Errorf("cancel after it ended: %d", w.Code)
	}
}
