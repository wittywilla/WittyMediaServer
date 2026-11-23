// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A picture beside the media that has gone is replaced by the next best one beside it, or
// forgotten so the next scan makes one; a drive that is away changes nothing.
func TestStaleArtIsLookedForAgain(t *testing.T) {
	root := t.TempDir()
	s := testScanner(t, root)
	ctx := context.Background()
	lib, err := s.Store.CreateLibrary(ctx, "Films", "movies", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	write := func(p string) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	add := func(it *Item) *Item {
		t.Helper()
		it.LibraryID, it.Kind, it.SortTitle = lib.ID, "video", it.Title
		if err := s.Store.UpdateItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		got, err := s.Store.itemByPath(ctx, lib.ID, it.Path)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	// Renamed: Film-poster.jpg is now Film-poster.jpg.bak, and folder.jpg is beside it.
	a := filepath.Join(root, "A (2001)")
	renamed := add(&Item{Title: "A", Path: write(filepath.Join(a, "A.mkv")), Poster: filepath.Join(a, "A-poster.jpg"), Backdrop: filepath.Join(a, "A-fanart.jpg")})
	write(filepath.Join(a, "folder.jpg"))
	write(filepath.Join(a, "backdrop.jpg"))
	// Gone, with nothing beside it.
	b := filepath.Join(root, "B (2002)")
	gone := add(&Item{Title: "B", Path: write(filepath.Join(b, "B.mkv")), Poster: filepath.Join(b, "B-poster.jpg")})
	// The media's drive is away: the picture is missing, and so is the film.
	away := add(&Item{Title: "C", Path: filepath.Join(root, "Unplugged", "C.mkv"), Poster: filepath.Join(root, "Unplugged", "C-poster.jpg")})
	// A picture that is there, and one in the cache, stay as they are.
	d := filepath.Join(root, "D (2004)")
	there := add(&Item{Title: "D", Path: write(filepath.Join(d, "D.mkv")), Poster: write(filepath.Join(d, "poster.png")), Backdrop: "cache:abc.jpg"})

	if got := s.Store.StaleArt(ctx, renamed, false); got != filepath.Join(a, "folder.jpg") || renamed.Poster != got {
		t.Errorf("renamed poster: %q (item %q)", got, renamed.Poster)
	}
	if got := s.Store.StaleArt(ctx, renamed, true); got != filepath.Join(a, "backdrop.jpg") {
		t.Errorf("renamed backdrop: %q", got)
	}
	if again, _ := s.Store.itemByPath(ctx, lib.ID, renamed.Path); again.Poster != filepath.Join(a, "folder.jpg") || again.Backdrop != filepath.Join(a, "backdrop.jpg") {
		t.Errorf("not saved: %q %q", again.Poster, again.Backdrop)
	}
	if got := s.Store.StaleArt(ctx, gone, false); got != "" || gone.Poster != "" {
		t.Errorf("gone with nothing beside it should be forgotten for the next scan: %q", gone.Poster)
	}
	if got := s.Store.StaleArt(ctx, away, false); got != "" || away.Poster == "" {
		t.Errorf("a drive that's away must keep what's stored: %q", away.Poster)
	}
	if s.Store.StaleArt(ctx, there, false) != "" || s.Store.StaleArt(ctx, there, true) != "" || there.Poster == "" || there.Backdrop != "cache:abc.jpg" {
		t.Errorf("what's there changed: %q %q", there.Poster, there.Backdrop)
	}

	// After a scan, without anyone opening the page.
	write(filepath.Join(b, "cover.webp"))
	if err := s.Store.setPoster(ctx, gone.ID, filepath.Join(b, "B-poster.jpg")); err != nil {
		t.Fatal(err)
	}
	if n, err := s.Store.repointStaleArt(ctx, lib.ID); err != nil || n != 1 {
		t.Fatalf("scan pass: %d %v", n, err)
	}
	if again, _ := s.Store.itemByPath(ctx, lib.ID, gone.Path); again.Poster != filepath.Join(b, "cover.webp") {
		t.Errorf("scan pass didn't re-point: %q", again.Poster)
	}
}

// An episode takes its own picture, not the season's poster in the same folder.
func TestStaleEpisodeArtIsItsOwn(t *testing.T) {
	dir := t.TempDir()
	s := testScanner(t, dir)
	ep := filepath.Join(dir, "Show S01E01.mkv")
	for _, n := range []string{ep, filepath.Join(dir, "poster.jpg"), filepath.Join(dir, "Show S01E01-thumb.jpg")} {
		if err := os.WriteFile(n, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	show := int64(1)
	it := &Item{ID: 999, Path: ep, ShowID: &show, Poster: filepath.Join(dir, "old-thumb.jpg")}
	if got := s.Store.StaleArt(context.Background(), it, false); got != filepath.Join(dir, "Show S01E01-thumb.jpg") {
		t.Errorf("episode: %q", got)
	}
}
