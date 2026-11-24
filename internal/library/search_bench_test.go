// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/search"
)

// TestSearchBeforeAfter times the search page's old way (a LIKE query per library, and
// one for people) against the index, on the same database. It runs only when asked:
//
//	WMS_SEARCH_BENCH=150000 go test ./internal/library -run TestSearchBeforeAfter -v
//	    a made-up database of that many titles
//	WMS_SEARCH_BENCH_DB=D:\copy-of-a-data-folder go test ...
//	    a copy of a real server's data folder (never the server's own: it is migrated)
func TestSearchBeforeAfter(t *testing.T) {
	dir, rows := os.Getenv("WMS_SEARCH_BENCH_DB"), os.Getenv("WMS_SEARCH_BENCH")
	if dir == "" && rows == "" {
		t.Skip("set WMS_SEARCH_BENCH or WMS_SEARCH_BENCH_DB")
	}
	ctx := context.Background()
	if dir == "" {
		dir = t.TempDir()
	}
	d, err := db.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := &Store{DB: d}
	if rows != "" {
		var n int
		fmt.Sscan(rows, &n)
		fillSearchBench(t, s, n)
	}
	libs, err := s.Libraries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	access := &Access{MaxAge: -1, AllowUnrated: true}

	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	start := time.Now()
	idx := search.New()
	docs := 0
	for i := range libs {
		list, err := s.SearchDocs(ctx, &libs[i])
		if err != nil {
			t.Fatal(err)
		}
		idx.Replace(libs[i].ID, list)
		docs += len(list)
	}
	people, err := s.PeopleDocs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	idx.Replace(0, people)
	docs += len(people)
	took := time.Since(start)
	runtime.GC()
	runtime.ReadMemStats(&m1)
	t.Logf("index: %d libraries, %d things to find, built in %v, %.1f MB held", len(libs), docs, took.Round(time.Millisecond), float64(int64(m1.HeapAlloc)-int64(m0.HeapAlloc))/(1<<20))

	old := func(q string) (int, error) {
		found := 0
		for _, l := range libs {
			switch l.Type {
			case "shows", "anime":
				sh, err := s.Shows(ctx, ShowQuery{LibraryID: l.ID, Sort: "title", Limit: 25, Access: access, Search: q})
				if err != nil {
					return 0, err
				}
				found += len(sh)
			case "music", "audiobooks", "podcasts":
				al, err := s.AlbumsFiltered(ctx, l.ID, AlbumFilter{Search: q, Sort: "title", Limit: 25}, access)
				if err != nil {
					return 0, err
				}
				found += len(al)
			default:
				it, _, err := s.Items(ctx, ItemQuery{LibraryID: l.ID, Sort: "title", Limit: 25, Access: access, NoTotal: true, Brief: true, Search: q})
				if err != nil {
					return 0, err
				}
				found += len(it)
			}
		}
		p, err := s.SearchPeople(ctx, q, 25, access)
		return found + len(p), err
	}
	cards := func(r search.Result) error { // reading the cards of what was found, as the page does
		for _, g := range r.Groups {
			var ids []int64
			for _, h := range g.Hits {
				ids = append(ids, h.ID)
			}
			if g.Key == "shows" {
				if _, err := s.Shows(ctx, ShowQuery{IDs: ids, Access: access, Limit: len(ids)}); err != nil {
					return err
				}
			} else if g.Key != "people" && g.Key != "channels" {
				if _, _, err := s.Items(ctx, ItemQuery{IDs: ids, Access: access, Limit: len(ids), NoTotal: true, Brief: true}); err != nil {
					return err
				}
			}
		}
		return nil
	}
	queries := []string{"th", "love", "star wars", "beatles", "intersteller", "the dark night", "matirx", "zzqx"}
	t.Logf("%-18s %12s %8s %12s %12s %8s", "search", "before", "found", "index", "with cards", "found")
	var worstOld, worstNew time.Duration
	for _, q := range queries {
		best := func(f func() error) time.Duration {
			b := time.Hour
			for range 3 {
				t0 := time.Now()
				if err := f(); err != nil {
					t.Fatal(err)
				}
				b = min(b, time.Since(t0))
			}
			return b
		}
		var nOld int
		tOld := best(func() (err error) { nOld, err = old(q); return })
		var r search.Result
		tIdx := best(func() error { r = idx.Search(search.Query{Text: q, PerGroup: 24}); return nil })
		tAll := best(func() error { r = idx.Search(search.Query{Text: q, PerGroup: 24}); return cards(r) })
		nNew := 0
		for _, g := range r.Groups {
			nNew += len(g.Hits)
		}
		t.Logf("%-18q %12v %8d %12v %12v %8d", q, tOld.Round(time.Microsecond), nOld, tIdx.Round(time.Microsecond), tAll.Round(time.Microsecond), nNew)
		worstOld, worstNew = max(worstOld, tOld), max(worstNew, tAll)
		if os.Getenv("WMS_SEARCH_BENCH_SHOW") != "" {
			for _, g := range r.Groups {
				var top []string
				for _, h := range g.Hits[:min(5, len(g.Hits))] {
					top = append(top, fmt.Sprintf("%s (%d)", h.Title, h.Score))
				}
				t.Logf("    %s: %s", g.Key, strings.Join(top, ", "))
			}
		}
	}
	t.Logf("slowest: before %v, now %v (budget 100ms)", worstOld.Round(time.Microsecond), worstNew.Round(time.Microsecond))
	if worstNew > 100*time.Millisecond {
		t.Errorf("a search took %v, over the budget of 100 ms", worstNew)
	}
}

// fillSearchBench makes libraries of made-up titles: films, shows with episodes, music
// with albums and artists, books and photos, n titles in all.
func fillSearchBench(t *testing.T, s *Store, n int) {
	ctx := context.Background()
	words := strings.Fields(`the a of and love night day man woman last first city dark light star war world house
		blue red black white king queen dead life story home girl boy time road river moon sun fire ice
		ghost dream heart song summer winter little big lost secret return rise fall shadow storm garden
		island ocean mountain forest wild brother sister father mother child angel devil hunter killer
		journey legend empire kingdom galaxy planet robot machine family friends school party wedding
		midnight morning evening forever never always again together alone broken golden silver iron
		interstellar matrix beatles knight wars`)
	rng := rand.New(rand.NewPCG(7, 9))
	title := func(k int) string {
		t := make([]string, 1+rng.IntN(k))
		for i := range t {
			t[i] = words[rng.IntN(len(words))]
		}
		s := strings.Join(t, " ")
		return strings.ToUpper(s[:1]) + s[1:]
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := db.Now()
	lib := func(typ string) int64 {
		r, err := tx.ExecContext(ctx, `INSERT INTO libraries(name, type, created_at) VALUES (?, ?, ?)`, typ, typ, now)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := r.LastInsertId()
		return id
	}
	item := func(lib int64, kind, ttl string, show any, artist, album string) {
		_, err := tx.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, year, show_id, artist, album, album_artist, added_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, lib, kind, fmt.Sprintf("/m/%d/%d", lib, rng.Int64()), ttl, strings.ToLower(ttl), 1950+rng.IntN(75), show, artist, album, artist, now, now)
		if err != nil {
			t.Fatal(err)
		}
	}
	films, shows, music, books, photos := lib("movies"), lib("shows"), lib("music"), lib("books"), lib("photos")
	for range n * 15 / 100 {
		item(films, "video", title(4), nil, "", "")
	}
	for range n / 100 {
		r, err := tx.ExecContext(ctx, `INSERT INTO shows(library_id, path, title, sort_title, added_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, shows, fmt.Sprintf("/s/%d", rng.Int64()), title(3), "x", now, now)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := r.LastInsertId()
		for range 25 {
			item(shows, "video", title(4), id, "", "")
		}
	}
	artists := make([]string, max(1, n/200))
	for i := range artists {
		artists[i] = title(2)
	}
	for range n * 35 / 100 / 12 {
		a, al := artists[rng.IntN(len(artists))], title(3)
		for range 12 {
			item(music, "audio", title(5), nil, a, al)
		}
	}
	for range n / 10 {
		item(books, "book", title(5), nil, "", "")
	}
	for range n * 15 / 100 {
		item(photos, "photo", fmt.Sprintf("IMG_%05d", rng.IntN(99999)), nil, "", "")
	}
	for range n / 20 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO people(name, sort_name, key) VALUES (?, ?, ?)`, title(2), "x", "x"); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	_ = slices.Clip(artists)
}
