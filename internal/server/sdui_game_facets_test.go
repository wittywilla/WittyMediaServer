// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// A FilterBar over a games library offers the games' own genres, and the score steps once a
// game has a score; the video filters a games library can't have are left out.
func TestSDUIGameLibraryFacets(t *testing.T) {
	ts := newTestServer(t)
	u, _ := ts.user(t, "facets-admin", "adult", true, "")
	dir := t.TempDir()
	lib := ts.library(t, "games", dir)
	ts.item(t, &library.Item{LibraryID: lib, Kind: "game", Path: filepath.Join(dir, "a.nes"), Title: "A", Console: "nes"})
	ts.item(t, &library.Item{LibraryID: lib, Kind: "game", Path: filepath.Join(dir, "b.nes"), Title: "B", Console: "nes"})
	if _, err := ts.Store.DB.Exec(`UPDATE items SET genres = 'Platform' WHERE library_id = ?`, lib); err != nil {
		t.Fatal(err)
	}
	facets := func() map[string]int {
		r := httptest.NewRequest("GET", "/", nil)
		r = r.WithContext(context.WithValue(r.Context(), userKey, u))
		got, err := ts.newSDUIResolver(r, nil).Facets(strconv.FormatInt(lib, 10), []string{"genre", "minRating", "watched", "studio"})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]int{}
		for _, f := range got {
			out[f.ID] = len(f.Options)
		}
		return out
	}
	if got := facets(); got["genre"] != 1 || len(got) != 1 {
		t.Errorf("no scores yet: %v", got)
	}
	if _, err := ts.Store.DB.Exec(`UPDATE items SET rating = 8 WHERE library_id = ? AND title = 'A'`, lib); err != nil {
		t.Fatal(err)
	}
	if got := facets(); got["genre"] != 1 || got["minRating"] != 5 || len(got) != 2 {
		t.Errorf("with a score: %v", got)
	}
}
