// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gamemeta

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// A game's names in its own script and in English, from each source's answer. The names are
// made up: メガゲーム ("Mega Geemu" written in Latin letters, sold in English as Mega Game).

func decode[T any](t *testing.T, raw string) *T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	return &v
}

func TestNonLatin(t *testing.T) {
	for s, want := range map[string]bool{"メガゲーム": true, "Mega Game 2": false, "Pokémon": false, "Мега": true, "メガ Game": true, "": false, "1-2-3!": false, "메가": true, "超级游戏": true} {
		if NonLatin(s) != want {
			t.Errorf("NonLatin(%q) = %v", s, !want)
		}
	}
}

func TestScreenScraperTitles(t *testing.T) {
	cases := []struct {
		name, raw, native, english string
	}{
		{"Japanese first", `{"noms":[{"region":"ss","text":"Mega Geemu"},{"region":"jp","text":"メガゲーム"},{"region":"us","text":"Mega Game"}],
			"dates":[{"region":"jp","text":"1990-01-02"},{"region":"us","text":"1991-05-01"}]}`, "メガゲーム", "Mega Game"},
		// Released in America first: the Japanese name is a translation, not the game's own.
		{"American first", `{"noms":[{"region":"jp","text":"メガゲーム"},{"region":"us","text":"Mega Game"}],
			"dates":[{"region":"us","text":"1990-01-02"},{"region":"jp","text":"1991-05-01"}]}`, "", "Mega Game"},
		{"no dates", `{"noms":[{"region":"wor","text":"Mega Geemu"},{"region":"jp","text":"メガゲーム"}]}`, "メガゲーム", "Mega Geemu"},
		{"Japan only, written in Latin letters", `{"noms":[{"region":"jp","text":"Mega Geemu"}],"dates":[{"region":"jp","text":"1990"}]}`, "", ""},
		{"Europe's name when America has none", `{"noms":[{"region":"eu","text":"Mega Game EU"},{"region":"kr","text":"메가 게임"}],"dates":[{"region":"kr","text":"1999"}]}`, "메가 게임", "Mega Game EU"},
	}
	for _, c := range cases {
		g := decode[ssGame](t, c.raw)
		in := (&ScreenScraper{}).info(g)
		if in.Native != c.native || in.English != c.english {
			t.Errorf("%s: native %q english %q, want %q %q", c.name, in.Native, in.English, c.native, c.english)
		}
	}
}

func TestIGDBTitles(t *testing.T) {
	g := decode[igdbGame](t, `{"id":1,"name":"Mega Game","alternative_names":[
		{"name":"Mega Geemu","comment":"Japanese title - romanization"},
		{"name":"メガゲーム・トランスレーテッド","comment":"Japanese title - translated"},
		{"name":"メガゲーム","comment":"Japanese title - original"}]}`)
	in := (&IGDB{}).info(g)
	if in.Native != "メガゲーム" || in.English != "Mega Game" || in.Title != "Mega Game" {
		t.Errorf("%+v", in)
	}
	// An original name in Latin letters isn't a name in another script.
	g = decode[igdbGame](t, `{"id":2,"name":"Mega Game","alternative_names":[{"name":"Mega Spiel","comment":"Other Title - Original"}]}`)
	if in := (&IGDB{}).info(g); in.Native != "" {
		t.Errorf("native %q", in.Native)
	}
	// What's asked for includes the names.
	if !strings.Contains(igdbFields, "alternative_names.name,alternative_names.comment") {
		t.Error("the alternative names aren't asked for")
	}
}

func TestTGDBTitles(t *testing.T) {
	g := decode[tgdbGame](t, `{"id":3,"game_title":"Mega Geemu","alternates":["Mega Game","メガゲーム"]}`)
	in := (&TGDB{}).info(context.Background(), g, &tgdbAnswer{})
	if in.Native != "メガゲーム" || in.English != "" {
		t.Errorf("%+v", in)
	}
}

func TestRAWGTitles(t *testing.T) {
	g := decode[rawgGame](t, `{"id":4,"name":"Mega Game","alternative_names":["Mega Geemu","メガゲーム"]}`)
	in := (&RAWG{}).info(g)
	if in.Native != "メガゲーム" || in.English != "Mega Game" {
		t.Errorf("%+v", in)
	}
}

func TestWikidataTitles(t *testing.T) {
	e := decode[wdEntityData](t, `{"id":"Q1","labels":{"en":{"value":"Mega Game"}},"claims":{
		"P1476":[{"rank":"normal","mainsnak":{"datavalue":{"value":{"text":"メガゲーム","language":"ja"}}}}]}}`)
	in := (&WikidataSource{}).info(e)
	if in.Native != "メガゲーム" || in.English != "Mega Game" {
		t.Errorf("%+v", in)
	}
	// A title in the game's own Latin letters isn't one in another script.
	e = decode[wdEntityData](t, `{"id":"Q2","labels":{"en":{"value":"Mega Game"}},"claims":{
		"P1476":[{"rank":"normal","mainsnak":{"datavalue":{"value":{"text":"Mega Game","language":"en"}}}}]}}`)
	if in := (&WikidataSource{}).info(e); in.Native != "" {
		t.Errorf("native %q", in.Native)
	}
}

func TestMergeTakesOtherNamesFromSureSources(t *testing.T) {
	m := Merge([]Answer{
		{Source: "libretro", Info: &Info{ID: "a", Title: "Mega Geemu (Japan)", Method: MethodChecksum, Confidence: 100}},
		{Source: "unsure", Info: &Info{ID: "b", Title: "Other", Native: "ちがう", English: "Wrong", Method: MethodName, Confidence: AcceptConfidence}},
		{Source: "igdb", Info: &Info{ID: "c", Title: "Mega Game", Native: "メガゲーム", English: "Mega Game", Method: MethodName, Confidence: RenameConfidence}},
	})
	if m.Title != "Mega Geemu (Japan)" || m.Native != "メガゲーム" || m.English != "Mega Game" {
		t.Errorf("%+v", m)
	}
}
