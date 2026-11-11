// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

func TestMetadataFolderRoutes(t *testing.T) {
	ts := newGalleryServer(t)
	ts.s.Cfg.DataDir = t.TempDir()
	ts.s.Cfg.ImagesDir = t.TempDir()
	dir := filepath.Join(ts.s.Cfg.DataDir, "metadata")
	ctx := context.Background()

	for _, r := range []struct{ method, path string }{
		{"GET", "/api/metadata-folder"}, {"POST", "/api/metadata-folder/export"}, {"GET", "/api/metadata-folder/preview"}, {"POST", "/api/metadata-folder/apply"},
	} {
		if w := ts.do(r.method, r.path, "", nil); w.Code != 401 {
			t.Errorf("%s %s signed out: %d", r.method, r.path, w.Code)
		}
		if w := ts.do(r.method, r.path, ts.kid, nil); w.Code != 403 {
			t.Errorf("%s %s not an administrator: %d", r.method, r.path, w.Code)
		}
	}

	lib, err := ts.s.Store.CreateLibrary(ctx, "Films", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	res, err := ts.s.DB.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, added_at, updated_at) VALUES(?, 'video', ?, 'Harbour Lights', 'Harbour Lights', 1, 1)`, lib.ID, filepath.Join(lib.Paths[0], "Harbour Lights.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	id, _ = res.LastInsertId()

	if w := ts.do("POST", "/api/metadata-folder/export", ts.admin, map[string]any{}); w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var job metaFolderView
	for i := 0; ; i++ {
		var v struct {
			Path string         `json:"path"`
			Job  metaFolderView `json:"job"`
		}
		w := ts.do("GET", "/api/metadata-folder", ts.admin, nil)
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		if v.Path != dir {
			t.Errorf("path %q", v.Path)
		}
		job = v.Job
		if !job.Running {
			break
		}
		if i > 200 {
			t.Fatal("the export never finished")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if job.Result == nil || job.Result.Written != 1 || job.Error != "" {
		t.Fatalf("%+v", job)
	}

	file := filepath.Join(dir, "Films", "Harbour Lights [item-"+strconv.FormatInt(id, 10)+"].json")
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(b), `"title": "Harbour Lights"`, `"title": "<script>alert(1)</script>Harbour Lights Again"`, 1)
	if err := os.WriteFile(file, []byte(edited), 0o640); err != nil {
		t.Fatal(err)
	}

	w := ts.do("GET", "/api/metadata-folder/preview", ts.admin, nil)
	var p library.FolderPreview
	if err := json.Unmarshal(w.Body.Bytes(), &p); w.Code != 200 || err != nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if p.Total != 1 || p.Changes[0].Fields[0].New != "Harbour Lights Again" {
		t.Fatalf("%+v", p)
	}
	if it, _ := ts.s.Store.Item(ctx, id); it.Title != "Harbour Lights" {
		t.Fatalf("a preview changed it: %q", it.Title)
	}

	w = ts.do("POST", "/api/metadata-folder/apply", ts.admin, nil)
	var r library.FolderResult
	if err := json.Unmarshal(w.Body.Bytes(), &r); w.Code != 200 || err != nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if r.Applied != 1 || r.Batch == "" {
		t.Fatalf("%+v", r)
	}
	if it, _ := ts.s.Store.Item(ctx, id); it.Title != "Harbour Lights Again" {
		t.Fatalf("not applied: %q", it.Title)
	}
	if w := ts.do("POST", "/api/meta/undo", ts.admin, map[string]any{"batch": r.Batch}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if it, _ := ts.s.Store.Item(ctx, id); it.Title != "Harbour Lights" {
		t.Fatalf("not undone: %q", it.Title)
	}

	// One thing at a time: anything else waits for the folder.
	ts.s.metaFolder.begin("export")
	if w := ts.do("GET", "/api/metadata-folder/preview", ts.admin, nil); w.Code != 409 {
		t.Errorf("busy: %d", w.Code)
	}
	if w := ts.do("POST", "/api/metadata-folder/apply", ts.admin, nil); w.Code != 409 {
		t.Errorf("busy: %d", w.Code)
	}
	ts.s.metaFolder.end()
}
