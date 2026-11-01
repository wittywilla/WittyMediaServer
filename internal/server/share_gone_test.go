package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// shareLink makes a link and returns its secret.
func shareLink(t *testing.T, ts *testServer, tok, kind string, id int64) string {
	t.Helper()
	w := ts.do(t, "POST", "/api/shares", tok, map[string]any{"kind": kind, "id": id, "allowDownload": true})
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	return out.Token
}

func photoItem(t *testing.T, ts *testServer, lib int64, dir, name string) int64 {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("PRIVATE-PICTURE-"+name), 0o644); err != nil {
		t.Fatal(err)
	}
	return ts.item(t, &library.Item{LibraryID: lib, Kind: "photo", Path: p, Title: name}).ID
}

// Numbers come back when the newest row goes, so a link used to open whatever got its target's
// number next: removing a library and adding it again, for example after moving a folder, handed
// an old link someone's new private pictures.
func TestOldLinkDoesntOpenWhatGetsItsNumber(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()
	_, tok := ts.user(t, "owner", "adult", true, "")
	dir := t.TempDir()
	lib := ts.library(t, "photos", dir)
	id := photoItem(t, ts, lib, dir, "shared.jpg")
	secret := shareLink(t, ts, tok, "item", id)
	if err := ts.Store.DeleteLibrary(ctx, lib); err != nil {
		t.Fatal(err)
	}
	lib2 := ts.library(t, "photos", dir)
	id2 := photoItem(t, ts, lib2, dir, "never-shared.jpg")
	if id2 != id {
		t.Skipf("the number didn't come back: %d %d", id, id2)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/share/%s", secret), "", nil); w.Code != 404 {
		t.Errorf("the link opened the new item: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/share/%s/original/%d", secret, id2), "", nil); w.Code != 404 {
		t.Errorf("the link gave its file: %d %s", w.Code, w.Body)
	}
}

// The same for an album: another person's private album can get the number of a deleted one.
func TestOldLinkDoesntOpenAnotherAlbum(t *testing.T) {
	ts := newTestServer(t)
	_, first := ts.user(t, "first", "adult", false, "")
	_, second := ts.user(t, "second", "adult", false, "")
	dir := t.TempDir()
	lib := ts.library(t, "photos", dir)
	item := photoItem(t, ts, lib, dir, "private.jpg")
	album := func(tok string) int64 {
		w := ts.do(t, "POST", "/api/photos/albums", tok, map[string]any{"libraryId": lib, "name": "Album"})
		var a struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil || a.ID == 0 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		return a.ID
	}
	a1 := album(first)
	secret := shareLink(t, ts, first, "album", a1)
	if w := ts.do(t, "DELETE", fmt.Sprintf("/api/photos/albums/%d", a1), first, nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	a2 := album(second)
	if a2 != a1 {
		t.Skipf("the number didn't come back: %d %d", a1, a2)
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/photos/albums/%d/items", a2), second, map[string]any{"ids": []int64{item}}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/share/%s/original/%d", secret, item), "", nil); w.Code != 404 {
		t.Errorf("the old link read a picture from another person's album: %d %s", w.Code, w.Body)
	}
}
