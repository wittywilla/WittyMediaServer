// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

func TestGamesHome(t *testing.T) {
	g := newGameServer(t)
	id := g.aGame(t, "first.wittytest")
	it, err := g.s.Store.Item(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if w := g.do("GET", "/api/games/home", g.other, nil); w.Code != 400 {
		t.Errorf("no library: %d", w.Code)
	}
	if w := g.do("GET", fmt.Sprintf("/api/games/home?library=%d", it.LibraryID), "", nil); w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}
	if w := g.do("PUT", fmt.Sprintf("/api/games/%d/meta", id), g.other, map[string]any{"favourite": true}); w.Code != 200 {
		t.Fatalf("favourite: %d %s", w.Code, w.Body)
	}
	type home struct {
		Recent       []*library.Item `json:"recent"`
		Favourites   []*library.Item `json:"favourites"`
		FavouriteIDs []int64         `json:"favouriteIds"`
		Continue     []struct {
			Item *library.Item `json:"item"`
		} `json:"continue"`
		Shelves []struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"shelves"`
	}
	read := func(tok string) home {
		t.Helper()
		var h home
		w := g.do("GET", fmt.Sprintf("/api/games/home?library=%d", it.LibraryID), tok, nil)
		if w.Code != 200 {
			t.Fatalf("home: %d %s", w.Code, w.Body)
		}
		if err := json.Unmarshal(w.Body.Bytes(), &h); err != nil {
			t.Fatal(err)
		}
		return h
	}
	mine := read(g.other)
	if len(mine.Recent) != 1 || mine.Recent[0].ID != id {
		t.Errorf("recent: %+v", mine.Recent)
	}
	if len(mine.Favourites) != 1 || len(mine.FavouriteIDs) != 1 {
		t.Errorf("their own favourite didn't come back: %+v", mine)
	}
	if theirs := read(g.kid); len(theirs.Favourites) != 0 || len(theirs.FavouriteIDs) != 0 {
		t.Errorf("another person saw someone's favourites: %+v", theirs)
	}
	// The path of a game is an administrator's to see, here as everywhere.
	if w := g.do("GET", fmt.Sprintf("/api/games/home?library=%d", it.LibraryID), g.other, nil); strings.Contains(w.Body.String(), `"path":"`+it.Path[:2]) {
		t.Error("the home screen showed a file's path")
	}
}

func TestGameSortsKeepUnknownYearsLast(t *testing.T) {
	g := newGameServer(t)
	a := g.aGame(t, "a.wittytest")
	it, err := g.s.Store.Item(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	// Three more in the same library, one of them with no year.
	for i, year := range []any{1990, 2001, nil} {
		_, err := g.s.DB.ExecContext(t.Context(), `INSERT INTO items(library_id, kind, title, sort_title, path, year, console, added_at, updated_at, container, size_bytes, missing)
			SELECT library_id, kind, ?, ?, ?, ?, console, added_at, updated_at, container, size_bytes, 0 FROM items WHERE id = ?`, fmt.Sprintf("g%d", i), fmt.Sprintf("g%d", i), fmt.Sprintf("/x/g%d", i), year, a)
		if err != nil {
			t.Fatal(err)
		}
	}
	titles := func(sort string) []string {
		t.Helper()
		var r struct {
			Items []*library.Item `json:"items"`
		}
		w := g.do("GET", fmt.Sprintf("/api/items?library=%d&kind=game&sort=%s", it.LibraryID, sort), g.other, nil)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", sort, w.Code, w.Body)
		}
		jsonBody(t, w, &r)
		out := []string{}
		for _, x := range r.Items {
			if strings.HasPrefix(x.Title, "g") {
				out = append(out, x.Title)
			}
		}
		return out
	}
	if got := strings.Join(titles("newest"), " "); got != "g1 g0 g2" {
		t.Errorf("newest first: %s", got)
	}
	if got := strings.Join(titles("oldest"), " "); got != "g0 g1 g2" {
		t.Errorf("oldest first: %s", got)
	}
	// Whoever's played most comes first under popular, whoever played it.
	if _, err := g.s.DB.ExecContext(t.Context(), `INSERT INTO wittyplay_game_meta(user_id, item_id, plays) SELECT id, (SELECT id FROM items WHERE title = 'g2'), 3 FROM users LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if got := titles("popular"); len(got) == 0 || got[0] != "g2" {
		t.Errorf("popular: %v", got)
	}
}

func TestGameCheck(t *testing.T) {
	g := newGameServer(t)
	id := g.aGame(t, "first.wittytest")
	var c struct {
		Ready bool `json:"ready"`
	}
	w := g.do("GET", fmt.Sprintf("/api/games/%d/check", id), g.other, nil)
	if w.Code != 200 {
		t.Fatalf("check: %d %s", w.Code, w.Body)
	}
	jsonBody(t, w, &c)
	if !c.Ready {
		t.Errorf("the test console should always be ready: %s", w.Body)
	}
	if w := g.do("GET", "/api/games/999999/check", g.other, nil); w.Code != 404 {
		t.Errorf("a game that isn't there: %d", w.Code)
	}
}
