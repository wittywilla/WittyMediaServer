package server

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

func uploadOne(t *testing.T, ts *testServer, lib int64, tok, name string, size int) int {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	head := []byte("\xff\xd8\xff\xe0")
	if _, err := fw.Write(append(head, make([]byte, size)...)); err != nil {
		t.Fatal(err)
	}
	_ = mw.Close()
	r := httptest.NewRequest("POST", fmt.Sprintf("/api/libraries/%d/upload", lib), &b)
	r.Host = "127.0.0.1:9797"
	r.RemoteAddr = "127.0.0.1:1"
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	return w.Code
}

func TestUploadsAddUpToALimit(t *testing.T) {
	ts := newTestServer(t)
	root := t.TempDir()
	lib := ts.library(t, "photos", root)
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, adult := ts.user(t, "bob", "adult", false, "")
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"uploadAccountMb": "1"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if got := uploadOne(t, ts, lib, adult, "one.jpg", 700<<10); got != 200 {
		t.Fatalf("the first: %d", got)
	}
	if got := uploadOne(t, ts, lib, adult, "two.jpg", 700<<10); got != 413 {
		t.Errorf("past the total: %d", got)
	}
	var names []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, d.Name())
		}
		return nil
	})
	if len(names) != 1 || names[0] != "one.jpg" {
		t.Errorf("left behind: %v", names)
	}
	// An administrator has no total, only the disk's floor.
	if got := uploadOne(t, ts, lib, admin, "three.jpg", 700<<10); got != 200 {
		t.Errorf("an administrator: %d", got)
	}
	if got := uploadOne(t, ts, lib, admin, "four.jpg", 700<<10); got != 200 {
		t.Errorf("an administrator, again: %d", got)
	}
	old := uploadFreeFloor
	uploadFreeFloor = 1 << 62
	defer func() { uploadFreeFloor = old }()
	if got := uploadOne(t, ts, lib, admin, "five.jpg", 10); got != 507 {
		t.Errorf("a disk that's full: %d", got)
	}
}

func TestCommentsCollectionsAndAlbumsPerAccount(t *testing.T) {
	oc, oa, om := maxCommentsPerAccount, maxCollectionsPerAccount, maxAlbumsPerAccount
	maxCommentsPerAccount, maxCollectionsPerAccount, maxAlbumsPerAccount = 2, 2, 2
	defer func() { maxCommentsPerAccount, maxCollectionsPerAccount, maxAlbumsPerAccount = oc, oa, om }()
	ts := newTestServer(t)
	root := t.TempDir()
	movies := ts.library(t, "movies", root)
	photos := ts.library(t, "photos", root)
	it := ts.item(t, &library.Item{LibraryID: movies, Kind: "video", Path: filepath.Join(root, "a.mkv"), Title: "A"})
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, adult := ts.user(t, "bob", "adult", false, "")
	for _, c := range []struct {
		name, path string
		body       map[string]any
	}{
		{"comment", fmt.Sprintf("/api/items/%d/comments", it.ID), map[string]any{"body": "hello"}},
		{"collection", "/api/collections", map[string]any{"name": "Mine"}},
		{"album", "/api/photos/albums", map[string]any{"libraryId": photos, "name": "Mine"}},
	} {
		for i := range 2 {
			if w := ts.do(t, "POST", c.path, adult, c.body); w.Code >= 300 {
				t.Fatalf("%s %d: %d %s", c.name, i, w.Code, w.Body)
			}
		}
		if w := ts.do(t, "POST", c.path, adult, c.body); w.Code != 409 {
			t.Errorf("%s past the limit: %d %s", c.name, w.Code, w.Body)
		}
		if w := ts.do(t, "POST", c.path, admin, c.body); w.Code >= 300 {
			t.Errorf("%s by an administrator: %d %s", c.name, w.Code, w.Body)
		}
	}
}
