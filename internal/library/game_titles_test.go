// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
)

// A game's other names are kept beside its title, which keeps its meaning and its sort, and
// searches find the game by any of them. A title typed by hand drops them.
func TestGameOtherNames(t *testing.T) {
	src := &fakeSource{id: "fake", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Title: "Mega Geemu (Japan)", Native: "メガゲーム", English: "Mega Game", Method: gamemeta.MethodChecksum, Confidence: 100}, nil
	}}
	f := newMetaFixture(t, src)
	it := f.scanFile(t, "Mega Geemu (Japan).sfc")
	f.work(t)
	got := f.item(t, it.ID)
	if got.Title != "Mega Geemu" || got.SortTitle != SortTitle("Mega Geemu") {
		t.Errorf("the title changed meaning: %q %q", got.Title, got.SortTitle)
	}
	if got.Titles == nil || *got.Titles != (GameTitles{Native: "メガゲーム", English: "Mega Game", Romanized: "Mega Geemu"}) {
		t.Fatalf("%+v", got.Titles)
	}
	for _, q := range []string{"メガ", "Mega Game", "Geemu"} {
		list, _, err := f.store.Items(f.ctx, ItemQuery{LibraryID: f.lib.ID, Search: q})
		if err != nil || len(list) != 1 || list[0].Titles == nil {
			t.Errorf("searching %q: %v, %d found", q, err, len(list))
		}
	}
	// A search for the names' keys finds nothing: only the names are searched.
	if list, _, _ := f.store.Items(f.ctx, ItemQuery{LibraryID: f.lib.ID, Search: "native"}); len(list) != 0 {
		t.Errorf("found by a key: %d", len(list))
	}
	it = f.item(t, it.ID)
	it.Title = "Typed By Hand"
	if err := f.store.upsertItem(f.ctx, it); err != nil {
		t.Fatal(err)
	}
	if err := f.store.MarkGameEdited(f.ctx, it.ID, "title"); err != nil {
		t.Fatal(err)
	}
	if got := f.item(t, it.ID); got.Titles != nil {
		t.Errorf("a typed title kept the source's other names: %+v", got.Titles)
	}
	if err := f.g.RefetchAll(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	if got := f.item(t, it.ID); got.Titles != nil || got.Title != "Typed By Hand" {
		t.Errorf("%q %+v", got.Title, got.Titles)
	}
}

func TestGameTitlesForDropsRepeats(t *testing.T) {
	if s := gameTitlesFor("", "Mega Game (USA)", "mega game"); s != "" {
		t.Errorf("%q", s)
	}
	if s := gameTitlesFor("メガゲーム", "Mega Game", "Mega Game"); s != `{"native":"メガゲーム"}` {
		t.Errorf("%q", s)
	}
	if readGameTitles(`{}`, "x") != nil || readGameTitles(`nope`, "x") != nil || readGameTitles("", "x") != nil {
		t.Error("nothing known should be nil")
	}
}
