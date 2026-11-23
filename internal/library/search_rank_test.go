// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db/dbtest"
)

// A search lists what matches by how well it matches: a title starting with the word, a title
// holding it, an artist's or album's name, and only then a plot that mentions it (#553: "lady"
// listed a book and two games before Lady Gaga, and found no song by its artist at all).
func TestSearchRanksTitlesAndArtistsBeforePlots(t *testing.T) {
	d, err := dbtest.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	s := &Store{DB: d}
	lib, err := s.CreateLibrary(ctx, "Everything", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	add := func(it *Item) *Item {
		it.LibraryID, it.Path, it.SortTitle = lib.ID, "/m/"+it.Title, it.Title
		if err := s.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		return it
	}
	plot := add(&Item{Kind: "book", Title: "A Sense of Things", Overview: "A young lady and her sister."})
	game := add(&Item{Kind: "game", Title: "Ape Kong", Overview: "Save the lady from the ape."})
	song := add(&Item{Kind: "audio", Title: "Bad Romance", Artist: "Lady Gaga", Album: "The Fame Monster"})
	inside := add(&Item{Kind: "video", Title: "The Lady Vanishes"})
	first := add(&Item{Kind: "video", Title: "Lady Bird"})
	add(&Item{Kind: "video", Title: "Nothing to do with it"})

	got, total, err := s.Items(ctx, ItemQuery{Search: "lady"})
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{first.ID, inside.ID, song.ID, plot.ID, game.ID} // each tier by title
	if total != len(want) || len(got) != len(want) {
		t.Fatalf("found %d (total %d), want %d", len(got), total, len(want))
	}
	for i, it := range got {
		if it.ID != want[i] {
			t.Fatalf("place %d is %q; order %v", i, it.Title, titles(got))
		}
	}
	// An order asked for is kept.
	got, _, err = s.Items(ctx, ItemQuery{Search: "lady", Sort: "title"})
	if err != nil || len(got) != 5 || got[0].ID != plot.ID {
		t.Fatalf("by title: %v %v", titles(got), err)
	}
}

func titles(v []*Item) []string {
	out := make([]string, len(v))
	for i, it := range v {
		out[i] = it.Title
	}
	return out
}
