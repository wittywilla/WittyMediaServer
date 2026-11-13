// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

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

// Only an administrator can remove downloaded details. A film keeps its ID and goes back to its
// file name, and an NFO the server wrote goes from the folder.
func TestForgetMetadataRoute(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "Owner", "adult", true, "")
	_, other := ts.user(t, "Guest", "adult", false, "")
	root := t.TempDir()
	lib := ts.library(t, "movies", root)
	media := filepath.Join(root, "Quiet.Harbour.1987.mkv")
	if err := os.WriteFile(media, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	nfo := filepath.Join(root, "Quiet.Harbour.1987.nfo")
	if err := os.WriteFile(nfo, []byte("<!-- written by WittyMediaServer -->\n<movie><title>Matched</title></movie>"), 0o644); err != nil {
		t.Fatal(err)
	}
	it := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: media, Title: "A Matched Name", Overview: "Online.", Provider: "tmdb", ProviderID: "4"})
	path := fmt.Sprintf("/api/libraries/%d/forget-metadata", lib)

	if w := ts.do(t, "POST", path, other, map[string]any{}); w.Code != 403 {
		t.Fatalf("someone who isn't an administrator: %d", w.Code)
	}
	if w := ts.do(t, "POST", path, admin, map[string]any{"files": false, "details": false}); w.Code != 400 {
		t.Fatalf("nothing chosen: %d", w.Code)
	}
	w := ts.do(t, "POST", path, admin, map[string]any{})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var res struct {
		Removed    int     `json:"removed"`
		Titles     int     `json:"titles"`
		Rescanning []int64 `json:"rescanning"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Removed != 1 || res.Titles != 1 || len(res.Rescanning) != 0 {
		t.Errorf("answer: %s", w.Body)
	}
	if _, err := os.Stat(nfo); err == nil {
		t.Error("the server's NFO is still there")
	}
	got, err := ts.Store.Item(context.Background(), it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Quiet Harbour" || got.Overview != "" || got.Provider != "" {
		t.Errorf("not reset: %+v", got)
	}
	// Every library at once.
	if w := ts.do(t, "POST", "/api/libraries/forget-metadata", admin, map[string]any{"files": true, "details": true}); w.Code != 200 {
		t.Fatalf("every library: %d %s", w.Code, w.Body)
	}
}
