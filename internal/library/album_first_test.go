// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"testing"
)

// An album's first track is the one Play starts with: disc first, then track
// number, whatever order the files were found in, and with no cover at all.
func TestAlbumFirstItem(t *testing.T) {
	s, _ := galleryStore(t, "movies")
	ctx := context.Background()
	music, _ := s.CreateLibrary(ctx, "Music", "music", []string{t.TempDir()})
	n := func(v int) *int { return &v }
	tracks := []*Item{
		{Path: "/m/a/2-1", Title: "Two one", DiscNo: n(2), Track: n(1)},
		{Path: "/m/a/1-3", Title: "One three", DiscNo: n(1), Track: n(3)},
		{Path: "/m/a/1-2", Title: "One two", DiscNo: n(1), Track: n(2)},
		{Path: "/m/b/x", Title: "Other album", Track: n(1)},
	}
	for i, it := range tracks {
		it.LibraryID, it.Kind, it.Artist, it.SortTitle = music.ID, "audio", "Band", it.Title
		it.Album = "A"
		if i == 3 {
			it.Album = "B"
		}
		if err := s.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	albums, err := s.AlbumsFiltered(ctx, music.ID, AlbumFilter{Sort: "title"}, nil)
	if err != nil || len(albums) != 2 {
		t.Fatalf("albums %v %v", albums, err)
	}
	if albums[0].Title != "A" || albums[0].FirstItem != tracks[2].ID {
		t.Errorf("album A starts with %d, want %d (disc 1 track 2)", albums[0].FirstItem, tracks[2].ID)
	}
	if albums[1].FirstItem != tracks[3].ID || albums[0].CoverItem != 0 {
		t.Errorf("album B starts with %d, want %d; covers %d", albums[1].FirstItem, tracks[3].ID, albums[0].CoverItem)
	}
}
