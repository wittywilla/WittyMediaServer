// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"path/filepath"
	"testing"
)

// One folder's album is one album (#355): a compilation whose songs have their own artists,
// and an album whose tags differ only in spacing or case.
func TestOneFolderIsOneAlbum(t *testing.T) {
	root := t.TempDir()
	s := testScanner(t, root)
	ctx := context.Background()
	lib, err := s.Store.CreateLibrary(ctx, "Music", "music", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	add := func(dir, name, artist, albumArtist, album string) {
		t.Helper()
		p := filepath.Join(root, dir, name+".flac")
		if err := s.Store.UpdateItem(ctx, &Item{LibraryID: lib.ID, Kind: "audio", Path: p, Title: name, SortTitle: name, Artist: artist, AlbumArtist: albumArtist, Album: album}); err != nil {
			t.Fatal(err)
		}
	}
	// No album artist in the tags: the scan used each song's artist.
	ae := filepath.Join("MonoCulture", "The Animal Economy")
	add(ae, "1", "MonoCulture", "MonoCulture", "The Animal Economy")
	add(ae, "2", "Teague Cullen", "Teague Cullen", "The Animal Economy")
	add(ae, "3", "Winterstand", "Winterstand", "The Animal Economy")
	// A compilation in a folder named after no one of its artists.
	add(filepath.Join("Compilations", "Summer Hits"), "1", "A", "A", "Summer Hits")
	add(filepath.Join("Compilations", "Summer Hits"), "2", "B", "B", "Summer Hits")
	// Spacing.
	add(filepath.Join("monophonic", "2.5 beats"), "1", "monophonic", "monophonic", "2.5 beats")
	add(filepath.Join("monophonic", "2.5 beats"), "2", "monophonic", "monophonic", "2.5 beats")
	add(filepath.Join("monophonic", "2.5 beats"), "3", "monophonic", "monophonic", "2.5beats")
	// Two discs in their own folders are one album.
	add(filepath.Join("Band", "Long", "CD1"), "1", "Band", "Band", "Long")
	add(filepath.Join("Band", "Long", "CD2"), "1", "Band", "Band", "long")
	// Two albums of the same name in different folders stay two.
	add(filepath.Join("X", "Greatest Hits"), "1", "X", "X", "Greatest Hits")
	add(filepath.Join("Y", "Greatest Hits"), "1", "Y", "Y", "Greatest Hits")

	if err := s.Store.unifyFolderAlbums(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	albums, err := s.Store.AlbumsFiltered(ctx, lib.ID, AlbumFilter{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, a := range albums {
		got[a.Artist+" / "+a.Title] = a.Tracks
	}
	want := map[string]int{"MonoCulture / The Animal Economy": 3, "Various Artists / Summer Hits": 2, "monophonic / 2.5 beats": 3,
		"Band / Long": 2, "X / Greatest Hits": 1, "Y / Greatest Hits": 1}
	if len(got) != len(want) {
		t.Fatalf("albums %v, want %v", got, want)
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("%s: %d songs, want %d (all: %v)", k, got[k], n, got)
		}
	}
	// Each song keeps its own artist.
	tracks, err := s.Store.MusicTracks(ctx, MusicQuery{LibraryID: lib.ID, Artist: "Teague Cullen"}, nil)
	if err != nil || len(tracks) != 1 {
		t.Fatalf("Teague Cullen's songs: %d %v", len(tracks), err)
	}
	// Running it again changes nothing.
	if err := s.Store.unifyFolderAlbums(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.Store.AlbumsFiltered(ctx, lib.ID, AlbumFilter{}, nil); len(again) != len(albums) {
		t.Errorf("a second run: %d albums, then %d", len(albums), len(again))
	}
}
