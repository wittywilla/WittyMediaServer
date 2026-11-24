// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package search

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"Amélie":                  "amelie",
		"Don't Look Up":           "dont look up",
		"Tom & Jerry":             "tom and jerry",
		"Spider-Man: No Way Home": "spider man no way home",
		"  WALL·E  ":              "wall e",
		"東京物語":                    "東京物語",
		"":                        "",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDistance(t *testing.T) {
	var rows [3][]int
	for _, c := range []struct {
		a, b string
		want int
	}{{"matrix", "matrix", 0}, {"matirx", "matrix", 1}, {"intersteller", "interstellar", 1}, {"cat", "dog", 3}, {"abc", "", 3}} {
		if got := distance([]rune(c.a), []rune(c.b), 5, &rows); got != c.want {
			t.Errorf("distance(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	if got := distance([]rune("abcdef"), []rune("uvwxyz"), 1, &rows); got != 2 {
		t.Errorf("a distance past the limit is the limit and one, got %d", got)
	}
}

func sample() *Index {
	x := New()
	x.Replace(1, []Doc{
		{Group: "films", ID: 1, Lib: 1, Title: "Interstellar", Year: 2014},
		{Group: "films", ID: 2, Lib: 1, Title: "The Matrix", Year: 1999},
		{Group: "films", ID: 3, Lib: 1, Title: "The Matrix Reloaded", Year: 2003},
		{Group: "films", ID: 4, Lib: 1, Title: "Amélie", Year: 2001},
		{Group: "films", ID: 5, Lib: 1, Title: "Spider-Man", Year: 2002},
		{Group: "films", ID: 6, Lib: 1, Title: "Seven Samurai", Alt: "七人の侍", Year: 1954},
	})
	x.Replace(2, []Doc{
		{Group: "artists", ID: 20, Lib: 2, Title: "Lady Gaga", Count: 5},
		{Group: "albums", ID: 21, Lib: 2, Title: "Chromatica", Alt: "Lady Gaga", Count: 16},
		{Group: "songs", ID: 22, Lib: 2, Title: "Rain on Me", Alt: "Lady Gaga\nChromatica"},
		{Group: "songs", ID: 23, Lib: 2, Title: "The Lady Is a Tramp", Alt: "Frank Sinatra"},
	})
	x.Replace(3, []Doc{{Group: "books", ID: 30, Lib: 3, Title: "Matilda", Year: 1988}})
	x.Replace(0, []Doc{{Group: "people", ID: 40, Title: "Keanu Reeves", Count: 50}})
	return x
}

func titles(r Result, group string) []string {
	g := r.Group(group)
	if g == nil {
		return nil
	}
	var out []string
	for _, h := range g.Hits {
		out = append(out, h.Title)
	}
	return out
}

func TestSearchFinds(t *testing.T) {
	x := sample()
	for _, c := range []struct {
		q, group, first string
	}{
		{"interstellar", "films", "Interstellar"},
		{"intersteller", "films", "Interstellar"}, // a letter wrong
		{"interstel", "films", "Interstellar"},    // still typing
		{"matirx", "films", "The Matrix"},         // two letters swapped
		{"the matrix", "films", "The Matrix"},
		{"matrix rel", "films", "The Matrix Reloaded"},
		{"amelie", "films", "Amélie"},        // without the accent
		{"spiderman", "films", "Spider-Man"}, // without the hyphen
		{"七人", "films", "Seven Samurai"},     // another title, in another script
		{"keanu", "people", "Keanu Reeves"},
		{"kenau reeves", "people", "Keanu Reeves"},
		{"gaga", "artists", "Lady Gaga"},
		{"chromatica", "albums", "Chromatica"},
	} {
		r := x.Search(Query{Text: c.q})
		got := titles(r, c.group)
		if len(got) == 0 || got[0] != c.first {
			t.Errorf("%q: %s = %v, want %q first", c.q, c.group, got, c.first)
		}
	}
}

func TestSearchRanksTitleBeforeOtherNames(t *testing.T) {
	r := sample().Search(Query{Text: "lady"})
	if got := titles(r, "songs"); len(got) < 2 || got[0] != "The Lady Is a Tramp" {
		t.Errorf("a song called Lady comes before Lady Gaga's songs: %v", got)
	}
	if r.Groups[0].Key != "artists" {
		t.Errorf("the artist called Lady Gaga is the best group, got %s", r.Groups[0].Key)
	}
}

func TestSearchFilters(t *testing.T) {
	x := sample()
	if r := x.Search(Query{Text: "matrix year:2003"}); len(titles(r, "films")) != 1 || titles(r, "films")[0] != "The Matrix Reloaded" {
		t.Errorf("year:2003 keeps the one from 2003: %v", titles(r, "films"))
	}
	if r := x.Search(Query{Text: "matrix year:1990s"}); len(titles(r, "films")) != 1 {
		t.Errorf("year:1990s keeps the one from 1999: %v", titles(r, "films"))
	}
	if r := x.Search(Query{Text: "mat", Groups: []string{"books"}}); len(r.Groups) != 1 || r.Groups[0].Key != "books" {
		t.Errorf("only books asked for: %+v", r.Groups)
	}
	if r := x.Search(Query{Text: "matrix", Library: func(lib int64) bool { return lib != 1 }}); r.Group("films") != nil {
		t.Errorf("a library the profile can't open is never searched")
	}
	if r := x.Search(Query{Text: "m"}); len(r.Groups) != 0 {
		t.Errorf("one letter finds nothing yet")
	}
}

func TestSearchNoMatch(t *testing.T) {
	if r := sample().Search(Query{Text: "zzqqxx"}); len(r.Groups) != 0 {
		t.Errorf("nothing like zzqqxx: %+v", r.Groups)
	}
	// Short words must be exact: "cat" doesn't find "car".
	x := New()
	x.Replace(1, []Doc{{Group: "films", ID: 1, Title: "Cars"}})
	if r := x.Search(Query{Text: "cat"}); len(r.Groups) != 0 {
		t.Errorf("cat isn't cars: %+v", r.Groups)
	}
}

func TestExtraDocs(t *testing.T) {
	r := sample().Search(Query{Text: "bbc one", Extra: []Doc{{Group: "channels", Key: "c1", Title: "BBC One"}, {Group: "channels", Key: "c2", Title: "ITV"}}})
	if got := titles(r, "channels"); len(got) != 1 || got[0] != "BBC One" {
		t.Errorf("channels: %v", got)
	}
}

// big is a library the size of a large home server's, from made-up titles.
func big(n int) *Index {
	words := strings.Fields(`the a of and love night day man woman last first city dark light star war world house
		blue red black white king queen dead life story home girl boy time road river moon sun fire ice
		ghost dream heart song summer winter little big lost secret return rise fall shadow storm garden
		island ocean mountain forest wild brother sister father mother child angel devil hunter killer
		journey legend empire kingdom galaxy planet robot machine family friends school party wedding
		midnight morning evening forever never always again together alone broken golden silver iron`)
	rng := rand.New(rand.NewPCG(1, 2))
	x := New()
	per := n / 10
	for lib := int64(1); lib <= 10; lib++ {
		docs := make([]Doc, per)
		group := Groups[int(lib)%len(Groups)]
		for i := range docs {
			k := 1 + rng.IntN(4)
			t := make([]string, k)
			for j := range t {
				t[j] = words[rng.IntN(len(words))]
			}
			docs[i] = Doc{Group: group, ID: int64(i), Lib: lib, Title: strings.Join(t, " ") + fmt.Sprintf(" %d", rng.IntN(100)), Alt: words[rng.IntN(len(words))] + " " + words[rng.IntN(len(words))], Year: 1950 + rng.IntN(75)}
		}
		x.Replace(lib, docs)
	}
	return x
}

var benchQueries = []string{"th", "the", "love", "midnight gar", "galxy robot", "secret garden of the moon", "silvr", "x"}

func BenchmarkSearch200k(b *testing.B) {
	x := big(200_000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x.Search(Query{Text: benchQueries[i%len(benchQueries)]})
	}
}

func BenchmarkBuild200k(b *testing.B) {
	for i := 0; i < b.N; i++ {
		big(200_000)
	}
}

// TestSearchBudget holds every search on a 200,000-title library to the budget of
// 100 ms, with room for a slow computer running the tests: the worst of a few tries.
func TestSearchBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a large index")
	}
	x := big(200_000)
	for _, q := range benchQueries {
		best := time.Hour
		for range 3 {
			r := x.Search(Query{Text: q})
			best = min(best, r.Took)
		}
		t.Logf("%-28q %v", q, best)
		if best > 100*time.Millisecond {
			t.Errorf("%q took %v, over the budget of 100 ms", q, best)
		}
	}
}
