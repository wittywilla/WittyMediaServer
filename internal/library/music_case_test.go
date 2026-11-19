// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"path/filepath"
	"testing"
)

// "Lady GaGa" on one album and "Lady Gaga" on another are one artist, and her page and
// albums find both.
func TestArtistsDifferingOnlyInCaseAreOne(t *testing.T) {
	root := t.TempDir()
	s := testScanner(t, root)
	ctx := context.Background()
	lib, err := s.Store.CreateLibrary(ctx, "Music", "music", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	for i, x := range []struct{ artist, album string }{{"Lady GaGa", "The Fame"}, {"Lady Gaga", "Joanne"}, {"Lady Gaga", "Joanne"}, {"Adele", "21"}} {
		p := filepath.Join(root, x.album, string(rune('a'+i))+".flac")
		if err := s.Store.UpdateItem(ctx, &Item{LibraryID: lib.ID, Kind: "audio", Path: p, Title: p, SortTitle: p, AlbumArtist: x.artist, Album: x.album}); err != nil {
			t.Fatal(err)
		}
	}
	artists, err := s.Store.Artists(ctx, lib.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(artists) != 2 {
		t.Fatalf("artists %+v, want Adele and one Lady Gaga", artists)
	}
	for _, a := range artists {
		if a.Name != "Adele" && (a.Albums != 2 || a.Tracks != 3) {
			t.Errorf("%+v", a)
		}
	}
	albums, err := s.Store.AlbumsFiltered(ctx, lib.ID, AlbumFilter{Artist: "Lady GaGa"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 2 {
		t.Errorf("her albums: %+v", albums)
	}
	tracks, err := s.Store.MusicTracks(ctx, MusicQuery{LibraryID: lib.ID, Artist: "lady gaga"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 3 {
		t.Errorf("her songs: %d", len(tracks))
	}
}
