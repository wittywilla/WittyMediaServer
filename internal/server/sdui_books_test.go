// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"slices"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// A book a reader can open is read from its page: the item offers "play" (the apps open the
// reader for it), with its place as the point to continue from. One no reader opens offers
// nothing to play, and neither do apps that have no readers (Apple and Android today).
func TestSDUIBookIsReadable(t *testing.T) {
	rv := &sduiResolver{platform: "web"}
	book := rv.item(&library.Item{ID: 7, Kind: "book", Title: "The Lantern Road", Reader: "epub"})
	if book.PlayID != "7" || !slices.Contains(book.Can, "play") || slices.Contains(book.Can, "playOn") {
		t.Fatalf("a readable book: playId %q can %v", book.PlayID, book.Can)
	}
	started := rv.item(&library.Item{ID: 7, Kind: "book", Reader: "epub", Progress: &library.Progress{PositionSec: 2.4, DurationSec: 5}})
	if started.ResumeSec != 2.4 {
		t.Fatalf("a book part read continues from its place, got %v", started.ResumeSec)
	}
	done := rv.item(&library.Item{ID: 7, Kind: "book", Reader: "epub", Progress: &library.Progress{PositionSec: 5, DurationSec: 5, Watched: true}})
	if done.ResumeSec != 0 || !done.Watched {
		t.Fatalf("a finished book is read again from the start: %+v", done)
	}
	djvu := rv.item(&library.Item{ID: 8, Kind: "book", Title: "No reader"})
	if djvu.PlayID != "" || slices.Contains(djvu.Can, "play") {
		t.Fatalf("a book no reader opens offers to play: %+v", djvu)
	}
	tv := (&sduiResolver{platform: "tvos"}).item(&library.Item{ID: 7, Kind: "book", Reader: "epub"})
	if tv.PlayID != "" || slices.Contains(tv.Can, "play") {
		t.Fatalf("an app without readers is offered Read: %+v", tv)
	}
}
