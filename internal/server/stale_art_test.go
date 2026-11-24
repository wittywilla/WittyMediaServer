// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// A film whose poster file was renamed shows the picture beside it, not a broken one.
func TestARenamedPosterIsNoBrokenPicture(t *testing.T) {
	h, admin, _, s := tvServer(t)
	ctx := context.Background()
	dir := t.TempDir()
	lib, err := s.Store.CreateLibrary(ctx, "Films", "movies", []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	film := filepath.Join(dir, "Film (2001)", "Film.mkv")
	if err := os.MkdirAll(filepath.Dir(film), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(film, []byte("not really a film"), 0o644); err != nil {
		t.Fatal(err)
	}
	var pic bytes.Buffer
	_ = png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 2, 3)))
	if err := os.WriteFile(filepath.Join(dir, "Film (2001)", "folder.png"), pic.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	it := &library.Item{LibraryID: lib.ID, Kind: "video", Path: film, Title: "Film", SortTitle: "Film", Poster: filepath.Join(dir, "Film (2001)", "Film-poster.jpg")}
	if err := s.Store.UpdateItem(ctx, it); err != nil {
		t.Fatal(err)
	}
	items, _, err := s.Store.Items(ctx, library.ItemQuery{LibraryID: lib.ID})
	if err != nil || len(items) != 1 {
		t.Fatalf("%d %v", len(items), err)
	}
	w := tvCall(h, "GET", "/api/items/"+itoa(items[0].ID)+"/poster", admin, "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("poster: %d %s %s", w.Code, w.Header().Get("Content-Type"), w.Body)
	}
	if again, _ := s.Store.Item(ctx, items[0].ID); again == nil || filepath.Base(again.Poster) != "folder.png" {
		t.Errorf("the item still points at the old file: %+v", again)
	}
}
