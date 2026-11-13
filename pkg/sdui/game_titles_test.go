// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "testing"

// A game card carries the game's other names, so an app can show the one its profile
// prefers. The title stays the title. Only games carry them, and only names that are known.
func TestGameCardsCarryTheirOtherNames(t *testing.T) {
	r := newRenderer(&Page{}, &RenderContext{})
	game := &Item{ID: "7", Kind: "game", Title: "Mega Geemu", Titles: &Titles{Native: "メガゲーム", English: "Mega Game", Romanized: "Mega Geemu"}}
	n := r.card("row", "game", "m", game, Source{})
	titles, ok := n.Props["titles"].(map[string]any)
	if !ok || titles["native"] != "メガゲーム" || titles["english"] != "Mega Game" || titles["romanized"] != "Mega Geemu" || n.Props["title"] != "Mega Geemu" {
		t.Errorf("%#v", n.Props)
	}
	game = &Item{ID: "8", Kind: "game", Title: "Only Native", Titles: &Titles{Native: "ネイティブ", Romanized: "Only Native"}}
	n = r.card("row", "game", "m", game, Source{})
	if titles, _ := n.Props["titles"].(map[string]any); len(titles) != 2 || titles["english"] != nil {
		t.Errorf("%#v", n.Props["titles"])
	}
	film := &Item{ID: "9", Kind: "movie", Title: "A Film", Titles: &Titles{Native: "映画"}}
	if n := r.card("row", "poster", "m", film, Source{}); n.Props["titles"] != nil {
		t.Errorf("a film card has titles: %#v", n.Props)
	}
	if n := r.card("row", "game", "m", &Item{ID: "10", Kind: "game", Title: "Plain"}, Source{}); n.Props["titles"] != nil {
		t.Errorf("%#v", n.Props)
	}
}
