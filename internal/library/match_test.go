// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// A made-up TMDB: two films called nearly the same, and a show.
func fakeTMDB(t *testing.T) string {
	t.Helper()
	old := *SafeClient
	*SafeClient = *http.DefaultClient
	t.Cleanup(func() { *SafeClient = old })
	film := func(id int, title, date, over string) map[string]any {
		return map[string]any{"id": id, "title": title, "release_date": date, "overview": over, "poster_path": "/p" + title[:1] + ".jpg", "vote_average": 7.5,
			"genres":                []map[string]any{{"name": "Comedy"}, {"name": "Sci-Fi"}},
			"production_companies":  []map[string]any{{"name": "Lantern Works"}, {"name": "Small Pictures"}},
			"credits":               map[string]any{"crew": []map[string]string{{"name": "Dee Rector", "job": "Director"}}},
			"belongs_to_collection": map[string]string{"name": "Harbour Collection"},
			"images":                map[string]any{"posters": []map[string]string{{"file_path": "/one.jpg"}, {"file_path": "/two.jpg"}}}}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/search/movie":
			json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{
				{"id": 1, "title": "Harbour Lights", "release_date": "1998-01-01", "overview": "The wrong one.", "vote_average": 5.0},
				{"id": 2, "title": "Harbour Lights", "release_date": "2004-06-01", "overview": "The right one.", "poster_path": "/right.jpg", "vote_average": 8.0},
			}})
		case r.URL.Path == "/movie/1":
			json.NewEncoder(w).Encode(film(1, "Harbour Lights", "1998-01-01", "The wrong one."))
		case r.URL.Path == "/movie/2":
			json.NewEncoder(w).Encode(film(2, "Harbour Lights", "2004-06-01", "The right one."))
		case r.URL.Path == "/movie/2/watch/providers", r.URL.Path == "/movie/1/watch/providers":
			json.NewEncoder(w).Encode(map[string]any{"results": map[string]any{"US": map[string]any{
				"flatrate": []map[string]any{{"provider_name": "Streamly"}}, "rent": []map[string]any{{"provider_name": "Rentals Only"}}}}})
		case r.URL.Path == "/search/tv":
			json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"id": 9, "name": "Harbour Watch", "first_air_date": "2010-01-01"}}})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func matchFixture(t *testing.T) (*Scanner, *Library, string) {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	root := filepath.Join(t.TempDir(), "films")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	st := &Store{DB: d}
	ctx := context.Background()
	lib, err := st.CreateLibrary(ctx, "Films", "movies", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	base := fakeTMDB(t)
	s := &Scanner{Store: st, ImagesDir: t.TempDir(), CacheDir: t.TempDir(), Tools: &ffmpeg.Tools{}, TMDBBase: base, TMDBKey: func() string { return "key" },
		TVMazeOn: func() bool { return false }, WikidataOn: func() bool { return false }, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return s, lib, root
}

func addFile(t *testing.T, root, name string) string {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, 6<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func oneItem(t *testing.T, s *Scanner, lib int64) *Item {
	t.Helper()
	items, _, err := s.Store.Items(context.Background(), ItemQuery{LibraryID: lib})
	if err != nil || len(items) != 1 {
		t.Fatalf("%d items: %v", len(items), err)
	}
	return items[0]
}

func title(t *testing.T, s *Scanner, target string, id int64) *Title {
	t.Helper()
	tt, err := s.Store.LoadTitle(context.Background(), target, id)
	if err != nil {
		t.Fatal(err)
	}
	return tt
}

// The wrong film, put right, and it stays right whatever happens next: a refresh, a scan,
// even the file being replaced by a better copy.
func TestCorrectedMatchSurvivesRefreshAndRescan(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	p := addFile(t, root, "Harbour Lights.mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it := oneItem(t, s, lib.ID)
	if it.ProviderID != "1" || it.Overview != "The wrong one." {
		t.Fatalf("the scan should have taken the first result: %q %q", it.ProviderID, it.Overview)
	}
	tt := title(t, s, TargetItem, it.ID)
	if _, err := s.ApplyMatch(ctx, tt, "tmdb", "2", nil, "owner"); err != nil {
		t.Fatal(err)
	}
	it = oneItem(t, s, lib.ID)
	if it.ProviderID != "2" || it.Overview != "The right one." || it.Year == nil || *it.Year != 2004 {
		t.Fatalf("not corrected: %+v", it)
	}
	if got := s.Store.TitleBrands(ctx, TargetItem, it.ID, LogoStudio); len(got) != 2 {
		t.Errorf("studios: %v", got)
	}
	if m := s.Store.TitleMeta(ctx, TargetItem, it.ID); m.Director != "Dee Rector" {
		t.Errorf("director: %q", m.Director)
	}
	// A refresh keeps the match it has.
	tt = title(t, s, TargetItem, it.ID)
	if _, err := s.RefreshTitle(ctx, tt, nil, "owner"); err != nil {
		t.Fatal(err)
	}
	if it = oneItem(t, s, lib.ID); it.ProviderID != "2" {
		t.Fatalf("a refresh changed the match to %q", it.ProviderID)
	}
	// A scan of the same file.
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	if it = oneItem(t, s, lib.ID); it.ProviderID != "2" {
		t.Fatalf("a scan changed the match to %q", it.ProviderID)
	}
	// The file itself changing is what makes a scan rebuild a title from nothing.
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(p, later, later); err != nil {
		t.Fatal(err)
	}
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	if it = oneItem(t, s, lib.ID); it.ProviderID != "2" || it.Overview != "The right one." {
		t.Fatalf("a changed file lost the match: %q %q", it.ProviderID, it.Overview)
	}
}

// A locked field is never changed, by anything, matched or not.
func TestLockedFieldsSurviveEverything(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	p := addFile(t, root, "Harbour Lights.mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it := oneItem(t, s, lib.ID)
	tt := title(t, s, TargetItem, it.ID)
	if err := tt.EditFields(map[string]string{"title": "My Own Name", "overview": "What I typed."}, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.SaveTitle(ctx, tt); err != nil {
		t.Fatal(err)
	}
	check := func(when string) {
		t.Helper()
		got := oneItem(t, s, lib.ID)
		if got.Title != "My Own Name" || got.Overview != "What I typed." {
			t.Fatalf("%s: %q, %q", when, got.Title, got.Overview)
		}
	}
	tt = title(t, s, TargetItem, it.ID)
	if _, err := s.ApplyMatch(ctx, tt, "tmdb", "2", nil, ""); err != nil {
		t.Fatal(err)
	}
	check("after a new match")
	tt = title(t, s, TargetItem, it.ID)
	if _, err := s.RefreshTitle(ctx, tt, nil, ""); err != nil {
		t.Fatal(err)
	}
	check("after a refresh")
	// An NFO beside the file says otherwise, and the file changes so it's read.
	if err := os.WriteFile(filepath.Join(root, "Harbour Lights.nfo"), []byte(`<movie><title>From An NFO</title><plot>NFO plot</plot></movie>`), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(p, later, later); err != nil {
		t.Fatal(err)
	}
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	check("after a scan with an NFO")

	// Revert goes back to what the source said, and unlocks.
	tt = title(t, s, TargetItem, it.ID)
	if !tt.Meta.Locks["title"] || tt.Meta.Baseline["title"].Value == "" {
		t.Fatalf("no baseline: %+v", tt.Meta.Baseline)
	}
	if err := tt.RevertField("title"); err != nil {
		t.Fatal(err)
	}
	if tt.Field("title") != "Harbour Lights" || tt.Meta.Locks["title"] || tt.SourceOf("title") == SourceYou {
		t.Fatalf("revert: %q %v %q", tt.Field("title"), tt.Meta.Locks["title"], tt.SourceOf("title"))
	}
}

// A preview changes nothing, and says what would happen to what.
func TestPreviewChangesNothing(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	addFile(t, root, "Harbour Lights.mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it := oneItem(t, s, lib.ID)
	tt := title(t, s, TargetItem, it.ID)
	_ = tt.EditFields(map[string]string{"tagline": "Mine."}, true)
	_ = s.Store.SaveTitle(ctx, tt)
	tt = title(t, s, TargetItem, it.ID)
	p, err := s.PreviewMatch(ctx, tt, "tmdb", "2")
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]FieldChange{}
	for _, c := range p.Changes {
		by[c.Field] = c
	}
	if c := by["overview"]; c.Current != "The wrong one." || c.Incoming != "The right one." || !c.Apply {
		t.Errorf("overview: %+v", c)
	}
	if c, ok := by["year"]; !ok || c.Incoming != "2004" {
		t.Errorf("year: %+v", c)
	}
	if got := oneItem(t, s, lib.ID); got.ProviderID != "1" {
		t.Errorf("a preview changed the match: %q", got.ProviderID)
	}
}

// Removing a match keeps what was typed, goes back to the file's name, and stops lookups.
func TestRemoveMatch(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	p := addFile(t, root, "Harbour Lights.mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it := oneItem(t, s, lib.ID)
	tt := title(t, s, TargetItem, it.ID)
	_ = tt.EditFields(map[string]string{"tagline": "Mine."}, true)
	_ = s.Store.SaveTitle(ctx, tt)
	tt = title(t, s, TargetItem, it.ID)
	if _, err := s.RemoveMatch(ctx, tt, ""); err != nil {
		t.Fatal(err)
	}
	it = oneItem(t, s, lib.ID)
	if it.Provider != "" || it.Overview != "" || it.Tagline != "Mine." || it.Title != "Harbour Lights" {
		t.Fatalf("%+v", it)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(p, later, later); err != nil {
		t.Fatal(err)
	}
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	if it = oneItem(t, s, lib.ID); it.Provider != "" || it.Overview != "" {
		t.Fatalf("a scan looked it up again: %q %q", it.Provider, it.Overview)
	}
}

func TestUndo(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	addFile(t, root, "Harbour Lights.mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it := oneItem(t, s, lib.ID)
	tt := title(t, s, TargetItem, it.ID)
	batch, err := s.ApplyMatch(ctx, tt, "tmdb", "2", nil, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.Store.Undo(ctx, batch); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	got := oneItem(t, s, lib.ID)
	if got.ProviderID != "1" || got.Overview != "The wrong one." {
		t.Fatalf("%+v", got)
	}
	if m := s.Store.TitleMeta(ctx, TargetItem, it.ID); m.MatchState != MatchAuto {
		t.Errorf("state: %q", m.MatchState)
	}
	if _, err := s.Store.Undo(ctx, batch); err != ErrAlreadyUndone {
		t.Errorf("twice: %v", err)
	}
}

func TestParseMatchRef(t *testing.T) {
	for q, want := range map[string]MatchRef{
		"tt0000123": {"imdb", "tt0000123"},
		"https://www.imdb.com/title/tt0000123/?ref_=x":  {"imdb", "tt0000123"},
		"https://www.themoviedb.org/movie/12-a-made-up": {"tmdb", "12"},
		"https://www.themoviedb.org/tv/34-a-show":       {"tmdb-tv", "34"},
		"https://www.tvmaze.com/shows/56/a-show":        {"tvmaze", "56"},
		"https://thetvdb.com/?tab=series&id=78":         {"tvdb", "78"},
		"https://anilist.co/anime/90/x":                 {"anilist", "90"},
		"https://www.wikidata.org/wiki/Q42":             {"wikidata", "Q42"},
		"Q42":                                           {"wikidata", "Q42"},
		"tmdb:12":                                       {"tmdb", "12"},
		"tvdb 78":                                       {"tvdb", "78"},
		"TVmaze: 56":                                    {"tvmaze", "56"},
	} {
		got, ok := ParseMatchRef(q)
		if !ok || got != want {
			t.Errorf("%q: got %+v %v", q, got, ok)
		}
	}
	for _, q := range []string{"A Film", "https://example.com/movie/12", "Quarter 12", "tt12", "http://localhost/admin", "tmdb:../../etc"} {
		if got, ok := ParseMatchRef(q); ok {
			t.Errorf("%q should be a search, got %+v", q, got)
		}
	}
}

func TestSourceURLOnlyBuildsKnownAddresses(t *testing.T) {
	if u := SourceURL("tmdb", "../evil"); u != "" {
		t.Errorf("%q", u)
	}
	if u := SourceURL("somewhere", "1"); u != "" {
		t.Errorf("%q", u)
	}
}

// An id in a name wins over searching, and doesn't end up in the title.
func TestIDInFileNameDecides(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	addFile(t, root, "Harbour Lights (2004) [tmdbid-2].mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it := oneItem(t, s, lib.ID)
	if it.ProviderID != "2" || it.Title != "Harbour Lights" {
		t.Fatalf("%q %q", it.ProviderID, it.Title)
	}
	if m := s.Store.TitleMeta(ctx, TargetItem, it.ID); m.MatchState != MatchManual {
		t.Errorf("state %q", m.MatchState)
	}
}

func TestIDTags(t *testing.T) {
	for name, want := range map[string]MatchRef{
		"A Film (2001) [tmdbid-123]": {"tmdb", "123"},
		"A Film {imdb-tt0000123}":    {"imdb", "tt0000123"},
		"Show [tvdb-456]":            {"tvdb", "456"},
		"Show (tvmaze 7)":            {"tvmaze", "7"},
	} {
		got, ok := IDTag(name)
		if !ok || got != want {
			t.Errorf("%q: %+v %v", name, got, ok)
		}
		if strings.Contains(stripIDTags(name), want.ID) {
			t.Errorf("%q kept its id: %q", name, stripIDTags(name))
		}
	}
	if _, ok := IDTag("A Film [imdb-notanid]"); ok {
		t.Error("that isn't an IMDb id")
	}
}

func TestRules(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	if _, err := compileRule("{nonsense}"); err == nil {
		t.Error("an unknown word should be refused")
	}
	if _, err := compileRule(strings.Repeat("a", 400)); err == nil {
		t.Error("a pattern that long should be refused")
	}
	// Whatever is typed, it's literal, and takes no time to test.
	if _, err := s.Store.SetMatchRules(ctx, lib.ID, []MatchRule{{Pattern: "(a+)+$[", Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.SetMatchRules(ctx, lib.ID, []MatchRule{{Pattern: "x", Lookup: "somewhere"}}); err == nil {
		t.Error("an unknown source should be refused")
	}
	rules, err := s.Store.SetMatchRules(ctx, lib.ID, []MatchRule{
		{Pattern: "Harbour Studio/{year} - {title}", Lookup: "none", Tags: []string{"From the box"}, Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	addFile(t, root, filepath.Join("Harbour Studio", "1987 - A Quiet Pier.mkv"))
	try, err := s.Store.TryRules(ctx, lib.ID, rules)
	if err != nil || len(try) != 0 {
		t.Fatalf("no scan yet: %+v %v", try, err)
	}
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it := oneItem(t, s, lib.ID)
	if it.Title != "A Quiet Pier" || it.Year == nil || *it.Year != 1987 {
		t.Fatalf("%q %v", it.Title, it.Year)
	}
	if it.Provider != "" {
		t.Errorf("the rule said not to look it up, and it was: %q", it.Provider)
	}
	m := s.Store.TitleMeta(ctx, TargetItem, it.ID)
	if len(m.Tags) != 1 || m.Tags[0] != "From the box" {
		t.Errorf("tags: %v", m.Tags)
	}
	try, err = s.Store.TryRules(ctx, lib.ID, rules)
	if err != nil || len(try) != 1 || try[0].Result.Title != "A Quiet Pier" {
		t.Fatalf("%+v %v", try, err)
	}
}

// What a service answers can't put a file on this machine, control characters or a
// novel into the database.
func TestHostileDetailsAreCleaned(t *testing.T) {
	d := cleanDetails(&MatchDetails{Source: "tmdb", ID: "1", Title: "<b>Bad</b>\x00 Film\n", Overview: strings.Repeat("x", 50000),
		Poster: `C:\Windows\win.ini`, Backdrop: "file:///etc/passwd", Logo: "javascript:alert(1)", Banner: "https://ok.example/b.jpg",
		Genres: make([]string, 500), Posters: []string{"/etc/passwd", "https://ok.example/p.jpg"}})
	if d.Title != "Bad Film" {
		t.Errorf("%q", d.Title)
	}
	if len(d.Overview) > 20000 {
		t.Errorf("overview %d", len(d.Overview))
	}
	if d.Poster != "" || d.Backdrop != "" || d.Logo != "" {
		t.Errorf("art: %q %q %q", d.Poster, d.Backdrop, d.Logo)
	}
	if d.Banner == "" || len(d.Posters) != 1 {
		t.Errorf("real addresses should stay: %q %v", d.Banner, d.Posters)
	}
	if len(d.Genres) != 0 {
		t.Errorf("genres %d", len(d.Genres))
	}
	r := 99.0
	if cleanDetails(&MatchDetails{Rating: &r}).Rating != nil {
		t.Error("a rating of 99 should go")
	}
}

func TestTitleAlike(t *testing.T) {
	if TitleAlike("SHREK_THE_THIRD", "Shrek the Third") != 1 {
		t.Error("the same name")
	}
	if a := TitleAlike("A Tale 2", "A Tale 3"); a > 0.56 {
		t.Errorf("sequels aren't the same film: %f", a)
	}
	if a := TitleAlike("Tale Part II", "Tale 2"); a != 1 {
		t.Errorf("II is 2: %f", a)
	}
	if a := TitleAlike("Dr Story The Treeline", "The Treeline"); a < 0.5 {
		t.Errorf("one inside the other: %f", a)
	}
	if a := TitleAlike("Harbour Lights", "Completely Different"); a > 0.3 {
		t.Errorf("%f", a)
	}
}

func TestConfidenceSaysWhy(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	addFile(t, root, "Harbour Lights (2004).mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	q, err := s.MatchQueue(ctx, lib.ID, "all", "", 10, 0)
	if err != nil || q.Total != 1 {
		t.Fatalf("%+v %v", q, err)
	}
	if q.Rows[0].Confidence < 90 || len(q.Rows[0].Reasons) == 0 {
		t.Errorf("%+v", q.Rows[0])
	}
	// Someone's file is about one thing, and the match is another.
	it := oneItem(t, s, lib.ID)
	tt := title(t, s, TargetItem, it.ID)
	tt.Item.Title = "Something Else Entirely"
	y := 1950
	tt.Item.Year = &y
	if err := s.Store.upsertItem(ctx, tt.Item); err != nil {
		t.Fatal(err)
	}
	q, err = s.MatchQueue(ctx, lib.ID, "review", "", 10, 0)
	if err != nil || q.Total != 1 {
		t.Fatalf("it should need a look: %+v %v", q, err)
	}
	if !strings.Contains(strings.Join(q.Rows[0].Reasons, " "), "Different year") {
		t.Errorf("%v", q.Rows[0].Reasons)
	}
	// Choosing puts it right, and out of the queue.
	tt = title(t, s, TargetItem, it.ID)
	if _, err := s.ApplyMatch(ctx, tt, "tmdb", "2", nil, ""); err != nil {
		t.Fatal(err)
	}
	if q, _ = s.MatchQueue(ctx, lib.ID, "review", "", 10, 0); q.Total != 0 {
		t.Errorf("still in the queue: %+v", q.Rows)
	}
}

func TestSearchScoresBestFirst(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	addFile(t, root, "Harbour Lights (2004).mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it := oneItem(t, s, lib.ID)
	res, err := s.SearchMatches(ctx, title(t, s, TargetItem, it.ID), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) < 2 || res.Results[0].ID != "2" || res.Results[0].Score <= res.Results[1].Score {
		t.Fatalf("%+v", res.Results)
	}
	// An exact id goes straight to the title, and a TMDB address is understood.
	res, err = s.SearchMatches(ctx, title(t, s, TargetItem, it.ID), "https://www.themoviedb.org/movie/1-harbour", nil)
	if err != nil || !res.Exact || len(res.Results) != 1 || res.Results[0].ID != "1" {
		t.Fatalf("%+v %v", res, err)
	}
}

// A year kept from before counts when scoring, as one in the file's name does. One the
// match itself gave doesn't.
func TestSavedYearCountsWhenScoring(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	addFile(t, root, "Harbour Lights.mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it := oneItem(t, s, lib.ID)
	score := func(source string) int {
		tl := title(t, s, TargetItem, it.ID)
		y := 2004
		tl.Item.Year = &y
		tl.Meta.Sources = map[string]string{"year": source}
		res, err := s.SearchMatches(ctx, tl, "", nil)
		if err != nil || len(res.Results) == 0 {
			t.Fatalf("%+v %v", res, err)
		}
		return res.Results[0].Score
	}
	if saved, service := score(SourceSaved), score("tmdb"); saved != 100 || service >= 90 {
		t.Errorf("kept from before %d, from a service %d", saved, service)
	}
}

// browseFixture is a library with something to sort: no scan, just rows.
func browseFixture(t *testing.T) (*Store, *Library) {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	st := &Store{DB: d}
	lib, err := st.CreateLibrary(context.Background(), "Films", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return st, lib
}

func addRow(t *testing.T, st *Store, lib *Library, title string, year, w, h int, genres []string, studio string, rating float64) *Item {
	t.Helper()
	y := year
	r := rating
	it := &Item{LibraryID: lib.ID, Kind: "video", Path: filepath.Join(lib.Paths[0], title+".mkv"), Title: title, SortTitle: title, Year: &y, Width: w, Height: h, Genres: genres, Studio: studio, Rating: &r, Poster: "x", Backdrop: "y", AudioLangs: []string{"eng"}}
	if studio != "" {
		it.studios = []string{studio}
	}
	if err := st.upsertItem(context.Background(), it); err != nil {
		t.Fatal(err)
	}
	return it
}

func browseTitles(t *testing.T, st *Store, f BrowseFilter) []string {
	t.Helper()
	res, err := st.Browse(context.Background(), f, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, r := range res.Results {
		if r.Item != nil {
			out = append(out, r.Item.Title)
		} else {
			out = append(out, r.Show.Title)
		}
	}
	return out
}

func TestBrowseFiltersAndSorts(t *testing.T) {
	st, lib := browseFixture(t)
	addRow(t, st, lib, "Alpha", 1994, 720, 480, []string{"Comedy"}, "Lantern Works", 6)
	addRow(t, st, lib, "Bravo", 2001, 1920, 1080, []string{"Sci-Fi"}, "Lantern Works", 9)
	addRow(t, st, lib, "Charlie", 2019, 3840, 2160, []string{"Science Fiction", "Comedy"}, "Other Place", 7)
	eq := func(name string, got []string, want ...string) {
		t.Helper()
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
	base := BrowseFilter{Library: lib.ID}
	f := base
	f.Sort = "title"
	eq("title", browseTitles(t, st, f), "Alpha", "Bravo", "Charlie")
	f = base
	f.Sort = "newest"
	eq("newest", browseTitles(t, st, f), "Charlie", "Bravo", "Alpha")
	f = base
	f.Sort = "oldest"
	eq("oldest", browseTitles(t, st, f), "Alpha", "Bravo", "Charlie")
	f = base
	f.Sort, f.Desc = "rating", true
	eq("rating", browseTitles(t, st, f), "Bravo", "Charlie", "Alpha")
	f = base
	f.Sort, f.Desc = "quality", true
	eq("quality", browseTitles(t, st, f), "Charlie", "Bravo", "Alpha")
	f = base
	f.Studio = "lantern works"
	eq("studio", browseTitles(t, st, f), "Alpha", "Bravo")
	f = base
	f.Resolution = "4k"
	eq("4k", browseTitles(t, st, f), "Charlie")
	f = base
	f.Resolution = "sd"
	eq("sd", browseTitles(t, st, f), "Alpha")
	f = base
	f.Decade = 1990
	eq("decade", browseTitles(t, st, f), "Alpha")
	// Two spellings of a genre are one category.
	f = base
	f.Genre = "Science Fiction"
	eq("genre", browseTitles(t, st, f), "Bravo", "Charlie")

	// A shuffle gives each title once however it's paged.
	seen := map[string]bool{}
	for off := 0; off < 3; off++ {
		f = base
		f.Sort, f.Seed, f.Limit, f.Offset = "shuffle", 42, 1, off
		for _, n := range browseTitles(t, st, f) {
			if seen[n] {
				t.Errorf("%s came up twice", n)
			}
			seen[n] = true
		}
	}
	if len(seen) != 3 {
		t.Errorf("%v", seen)
	}

	facets, err := st.LibraryFacets(context.Background(), lib.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(facets.Genres) != 2 || len(facets.Studios) != 2 || len(facets.Resolutions) != 3 || len(facets.Decades) != 3 {
		t.Errorf("%+v", facets)
	}
}

// A kid sees neither the titles nor their counts.
func TestBrowseKeepsToAccess(t *testing.T) {
	st, lib := browseFixture(t)
	a := addRow(t, st, lib, "Allowed", 2001, 1920, 1080, []string{"Comedy"}, "Lantern Works", 6)
	b := addRow(t, st, lib, "Not For Kids", 2001, 1920, 1080, []string{"Horror"}, "Dark House", 6)
	a.ContentRating, b.ContentRating = "G", "R"
	if err := st.upsertItem(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := st.upsertItem(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	kid := &Access{MaxAge: 10, BlockItems: []int64{}}
	res, err := st.Browse(context.Background(), BrowseFilter{Library: lib.ID}, 0, kid)
	if err != nil || res.Total != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	cats, err := st.Categories(context.Background(), lib.ID, kid, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cats {
		if c.Name == "Horror" {
			t.Errorf("a kid can see a category only hidden titles are in: %+v", c)
		}
	}
	brands, _ := st.Brands(context.Background(), lib.ID, LogoStudio, kid)
	for _, b := range brands {
		if b.Name == "Dark House" {
			t.Errorf("a kid can see %s", b.Name)
		}
	}
}

func TestCategoriesCanBeArranged(t *testing.T) {
	st, lib := browseFixture(t)
	ctx := context.Background()
	addRow(t, st, lib, "One", 2001, 1920, 1080, []string{"Comedy"}, "", 6)
	addRow(t, st, lib, "Two", 2001, 1920, 1080, []string{"Comedy", "Family"}, "", 9)
	addRow(t, st, lib, "Three", 2001, 1920, 1080, []string{"Kids"}, "", 8)
	names := func(all bool) []string {
		c, err := st.Categories(ctx, lib.ID, nil, all)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, x := range c {
			out = append(out, x.Name)
		}
		return out
	}
	if got := names(false); got[0] != "Comedy" || len(got) != 3 {
		t.Fatalf("%v", got)
	}
	c, _ := st.Categories(ctx, lib.ID, nil, false)
	if len(c[0].Art) == 0 || c[0].Art[0].ID == 0 {
		t.Errorf("no art from its members: %+v", c[0])
	}
	// Rename.
	funny := "Funny Ones"
	if err := st.ChangeCategories(ctx, lib.ID, []CategoryChange{{Key: "comedy", Name: &funny}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := names(false); got[0] != "Funny Ones" {
		t.Fatalf("%v", got)
	}
	// The new name still finds the titles.
	if got := browseTitles(t, st, BrowseFilter{Library: lib.ID, Genre: "Funny Ones", Sort: "title"}); len(got) != 2 {
		t.Errorf("%v", got)
	}
	// Merge.
	into := "family"
	if err := st.ChangeCategories(ctx, lib.ID, []CategoryChange{{Key: "kids", Merge: &into}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := browseTitles(t, st, BrowseFilter{Library: lib.ID, Genre: "Family", Sort: "title"}); len(got) != 2 {
		t.Errorf("merged: %v", got)
	}
	// A merge can't go round in a circle.
	back := "kids"
	if err := st.ChangeCategories(ctx, lib.ID, []CategoryChange{{Key: "family", Merge: &back}}, nil); err == nil {
		t.Error("that's a circle")
	}
	// Hide.
	hide := true
	if err := st.ChangeCategories(ctx, lib.ID, []CategoryChange{{Key: "family", Hidden: &hide}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := names(false); len(got) != 1 {
		t.Fatalf("%v", got)
	}
	if got := names(true); len(got) != 2 {
		t.Fatalf("someone arranging them can still see it: %v", got)
	}
	// Reorder.
	if err := st.ChangeCategories(ctx, lib.ID, nil, []string{"family", "comedy"}); err != nil {
		t.Fatal(err)
	}
	if got := names(true); got[0] != "Family" {
		t.Fatalf("%v", got)
	}
	if err := st.ResetCategories(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	if got := names(false); got[0] != "Comedy" || len(got) != 3 {
		t.Fatalf("%v", got)
	}
}

func TestExtrasAndVersionsLeaveLists(t *testing.T) {
	st, lib := browseFixture(t)
	ctx := context.Background()
	film := addRow(t, st, lib, "The Film", 2001, 1920, 1080, []string{"Comedy"}, "", 6)
	trailer := addRow(t, st, lib, "The Film Trailer", 2001, 1920, 1080, []string{"Comedy"}, "", 6)
	uhd := addRow(t, st, lib, "The Film 4K", 2001, 3840, 2160, []string{"Comedy"}, "", 6)
	if err := st.AttachItem(ctx, trailer.ID, LinkExtra, "trailer", "", &film.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachItem(ctx, uhd.ID, LinkVersion, "", "4K", &film.ID, nil); err != nil {
		t.Fatal(err)
	}
	items, total, err := st.Items(ctx, ItemQuery{LibraryID: lib.ID})
	if err != nil || total != 1 || items[0].ID != film.ID {
		t.Fatalf("%d %v", total, err)
	}
	extras, links, err := st.AttachedTo(ctx, TargetItem, film.ID, LinkExtra)
	if err != nil || len(extras) != 1 || links[0].ExtraType != "trailer" {
		t.Fatalf("%v %v %v", extras, links, err)
	}
	if err := st.AttachItem(ctx, film.ID, LinkExtra, "trailer", "", &film.ID, nil); err == nil {
		t.Error("a title can't be attached to itself")
	}
	if err := st.AttachItem(ctx, film.ID, LinkExtra, "trailer", "", &uhd.ID, nil); err == nil {
		t.Error("nothing can be attached to something attached")
	}
	// A split makes it a title of its own again.
	if err := st.DetachItem(ctx, uhd.ID); err != nil {
		t.Fatal(err)
	}
	if _, total, _ = st.Items(ctx, ItemQuery{LibraryID: lib.ID}); total != 2 {
		t.Errorf("%d", total)
	}
}

// Moved to another library, a file stays there through a scan of both.
func TestMoveBetweenLibraries(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	other := filepath.Join(t.TempDir(), "more")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	addFile(t, other, "placeholder.mkv")
	lib2, err := s.Store.CreateLibrary(ctx, "More films", "movies", []string{other})
	if err != nil {
		t.Fatal(err)
	}
	music, err := s.Store.CreateLibrary(ctx, "Songs", "music", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	addFile(t, root, "Harbour Lights.mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it := oneItem(t, s, lib.ID)
	tt := title(t, s, TargetItem, it.ID)
	info := s.Store.MoveOptions(ctx, tt)
	if !info.Can || len(info.Options) != 1 || info.Options[0].ID != lib2.ID {
		t.Fatalf("%+v", info)
	}
	if err := s.Store.MoveTitle(ctx, tt, music.ID); err == nil {
		t.Error("a film can't go in a music library")
	}
	if err := s.Store.MoveTitle(ctx, tt, lib2.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{lib.ID, lib2.ID, lib.ID} {
		if err := s.scan(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if items, total, _ := s.Store.Items(ctx, ItemQuery{LibraryID: lib.ID}); total != 0 {
		t.Errorf("came back: %+v", items)
	}
	got := visible(t, s, lib2.ID)
	if got["Harbour Lights.mkv"] == nil || got["Harbour Lights.mkv"].ID != it.ID {
		t.Errorf("%+v", got)
	}
	// Moving back needs no note, because that's where its folder is.
	tt = title(t, s, TargetItem, it.ID)
	if err := s.Store.MoveTitle(ctx, tt, lib.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{lib.ID, lib2.ID} {
		if err := s.scan(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if it := oneItem(t, s, lib.ID); it == nil {
		t.Error("not back")
	}
}

// Removed from a library, a title doesn't come back by itself, and its file is untouched.
func TestRemovedTitlesStayRemoved(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	addFile(t, root, "Harbour Lights.mkv")
	p := addFile(t, root, "Another.mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	got := visible(t, s, lib.ID)
	it := got["Another.mkv"]
	if err := s.Store.RemoveTitle(ctx, title(t, s, TargetItem, it.ID)); err != nil {
		t.Fatal(err)
	}
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	if visible(t, s, lib.ID)["Another.mkv"] != nil {
		t.Fatal("it came back")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("the file should still be there: %v", err)
	}
	if list, _ := s.Store.RemovedTitles(ctx, lib.ID); len(list) != 1 {
		t.Fatalf("%+v", list)
	}
	if err := s.Store.RestoreRemoved(ctx, p); err != nil {
		t.Fatal(err)
	}
	if visible(t, s, lib.ID)["Another.mkv"] == nil {
		t.Fatal("not restored")
	}
}

// Renumbered episodes keep their numbers through a scan that reads the file again.
func TestRenumberedEpisodesStay(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	if err := s.Store.UpdateLibraryType(ctx, lib.ID, "shows"); err != nil {
		t.Fatal(err)
	}
	s.TMDBKey = func() string { return "" }
	p := addFile(t, root, filepath.Join("A Show", "A Show - S01E01.mkv"))
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it := oneItem(t, s, lib.ID)
	if _, err := s.RemapEpisodes(ctx, *it.ShowID, []EpisodeNumber{{ID: it.ID, Season: ptrInt(3), Episode: ptrInt(7)}}, ""); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(p, later, later); err != nil {
		t.Fatal(err)
	}
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it = oneItem(t, s, lib.ID)
	if it.Season == nil || *it.Season != 3 || it.Episode == nil || *it.Episode != 7 {
		t.Fatalf("season %v, episode %v", it.Season, it.Episode)
	}
	// Only this show's episodes can be renumbered.
	if _, err := s.RemapEpisodes(ctx, *it.ShowID+1, []EpisodeNumber{{ID: it.ID}}, ""); err == nil {
		t.Error("that's not its show")
	}
}

func ptrInt(n int) *int { return &n }

// Listings stay off until everything is accepted, and turning them off forgets them.
func TestListingsNeedEveryAcknowledgement(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	addFile(t, root, "Harbour Lights.mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetListings(ctx, true, "US", []string{"not-streaming"}); err != ErrListingsNotAccepted {
		t.Fatalf("%v", err)
	}
	if s.Store.ListingsEnabled(ctx) {
		t.Fatal("on without every statement accepted")
	}
	s.TMDBKey = func() string { return "" }
	if err := s.SetListings(ctx, true, "US", ListingAcknowledgements); err == nil {
		t.Fatal("on without a key")
	}
	s.TMDBKey = func() string { return "key" }
	if err := s.SetListings(ctx, false, "nowhere", nil); err == nil {
		t.Fatal("a bad region")
	}
	if err := s.Store.DB.SetSetting(ctx, "listingsConsent", ListingsVersion); err != nil {
		t.Fatal(err)
	}
	if err := s.syncListings(ctx, false); err != nil {
		t.Fatal(err)
	}
	st := s.ListingStatus(ctx)
	if st.Listed != 1 || st.AsOf == 0 {
		t.Fatalf("%+v", st)
	}
	p, err := s.Store.ListingPlatforms(ctx, lib.ID, nil)
	if err != nil || len(p) != 1 || p[0].Name != "Streamly" {
		t.Fatalf("rentals don't count as being on a service: %+v %v", p, err)
	}
	res, err := s.Store.Browse(ctx, BrowseFilter{Library: lib.ID, Platform: p[0].Key}, 0, nil)
	if err != nil || res.Total != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	if err := s.SetListings(ctx, false, "", nil); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Store.ListingPlatforms(ctx, lib.ID, nil); len(p) != 0 {
		t.Errorf("off, and still %+v", p)
	}
	var n int
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM listings`).Scan(&n); err != nil || n != 0 {
		t.Errorf("%d kept: %v", n, err)
	}
}

// A studio's list of films isn't a series, and the one a film is named after comes first.
func TestPickSeries(t *testing.T) {
	for _, c := range []struct {
		names []string
		title string
		want  string
	}{
		{[]string{"Harbour Studio feature films", "Harbour Lights"}, "Harbour Lights 2", "Harbour Lights Collection"},
		{[]string{"Lantern Universe", "Moth Trilogy"}, "The Moth Returns", "Moth Trilogy"},
		{[]string{"Lantern Universe"}, "A Quiet Sequel", "Lantern Universe Collection"},
		{[]string{"list of Harbour Studio films", "Harbour Studio films"}, "Harbour Lights", ""},
		{nil, "Harbour Lights", ""},
	} {
		if got := pickSeries(c.names, c.title); got != c.want {
			t.Errorf("%v, %q: %q, not %q", c.names, c.title, got, c.want)
		}
	}
}

// A title's listings go with its match, and Check now means now.
func TestListingsFollowTheMatch(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	addFile(t, root, "Harbour Lights.mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.DB.SetSetting(ctx, "listingsConsent", ListingsVersion); err != nil {
		t.Fatal(err)
	}
	// Changing a match starts the job by itself: let that finish, so it can't get in the way.
	settle := func() {
		for {
			listingJob.Lock()
			running := listingJob.running
			listingJob.Unlock()
			if !running {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	listed := func(all bool) int {
		t.Helper()
		settle()
		if err := s.syncListings(ctx, all); err != nil {
			t.Fatal(err)
		}
		return s.ListingStatus(ctx).Listed
	}
	if n := listed(false); n != 1 {
		t.Fatalf("listed %d", n)
	}
	it := oneItem(t, s, lib.ID)
	if _, err := s.RemoveMatch(ctx, title(t, s, TargetItem, it.ID), "someone"); err != nil {
		t.Fatal(err)
	}
	settle()
	if n := s.ListingStatus(ctx).Listed; n != 0 {
		t.Errorf("the match has gone, and still listed: %d", n)
	}
	if n := listed(false); n != 0 {
		t.Errorf("no match, and listed by a guess at its name: %d", n)
	}
	if _, err := s.ApplyMatch(ctx, title(t, s, TargetItem, it.ID), "tmdb", "2", nil, "someone"); err != nil {
		t.Fatal(err)
	}
	if n := listed(false); n != 1 {
		t.Errorf("matched again, and not looked at again: %d", n)
	}
	if _, err := s.Store.DB.ExecContext(ctx, `DELETE FROM listings`); err != nil {
		t.Fatal(err)
	}
	if n := listed(false); n != 0 {
		t.Errorf("checked again too soon: %d", n)
	}
	if n := listed(true); n != 1 {
		t.Errorf("Check now didn't: %d", n)
	}
	// What was accepted stands when only the region changes.
	if err := s.SetListings(ctx, true, "GB", nil); err != nil {
		t.Fatalf("changing the region: %v", err)
	}
	settle()
	if s.Store.ListingsRegion(ctx) != "GB" || !s.Store.ListingsEnabled(ctx) {
		t.Errorf("%q", s.Store.ListingsRegion(ctx))
	}
}

// Deleting a title leaves nothing behind in what's kept about it.
func TestDeletingATitleCleansUp(t *testing.T) {
	st, lib := browseFixture(t)
	ctx := context.Background()
	it := addRow(t, st, lib, "Going", 2001, 1920, 1080, nil, "Lantern Works", 6)
	m := newTitleMeta()
	m.Director = "Someone"
	if err := st.saveTitleMeta(ctx, TargetItem, it.ID, m); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteLibrary(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"title_meta", "title_brands"} {
		var n int
		if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s: %d left: %v", table, n, err)
		}
	}
}

func TestSavedViews(t *testing.T) {
	st, lib := browseFixture(t)
	ctx := context.Background()
	var uid int64
	res, err := st.DB.ExecContext(ctx, `INSERT INTO users(name, password_hash, created_at) VALUES('a', '', 0), ('b', '', 0)`)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ = res.LastInsertId()
	v := SavedView{LibraryID: lib.ID, Name: "  Old comedies  ", Query: "genre=Comedy&decade=1990&evil=1&sort=rating", Pinned: true}
	if err := st.SaveView(ctx, uid, &v); err != nil {
		t.Fatal(err)
	}
	if v.Name != "Old comedies" || strings.Contains(v.Query, "evil") || !strings.Contains(v.Query, "genre=Comedy") {
		t.Errorf("%+v", v)
	}
	// Someone else can't change or remove it.
	stolen := v
	stolen.Name = "Mine now"
	if err := st.SaveView(ctx, uid-1, &stolen); err != ErrNotFound {
		t.Errorf("%v", err)
	}
	if err := st.DeleteView(ctx, uid-1, v.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := st.SavedViews(ctx, uid); len(list) != 1 || list[0].Name != "Old comedies" {
		t.Errorf("%+v", list)
	}
}

// UpdateLibraryType changes what a library holds.
func (s *Store) UpdateLibraryType(ctx context.Context, id int64, typ string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE libraries SET type = ? WHERE id = ?`, typ, id)
	return err
}

// Two films of one series make a collection nobody had to make, and it can be edited like anything else.
func TestSeriesMakeCollections(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	addFile(t, root, "Harbour Lights (2004).mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it := oneItem(t, s, lib.ID)
	if tt := title(t, s, TargetItem, it.ID); tt.Field("series") != "Harbour Collection" {
		t.Fatalf("series: %q", tt.Field("series"))
	}
	if got, _ := s.Store.SeriesInLibrary(ctx, lib.ID, nil); len(got) != 0 {
		t.Errorf("one film isn't a collection: %+v", got)
	}
	second := addRow(t, s.Store, lib, "Harbour Lights Again", 2006, 1920, 1080, nil, "", 8)
	tt := title(t, s, TargetItem, second.ID)
	if err := tt.EditFields(map[string]string{"series": "Harbour Collection"}, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.SaveTitle(ctx, tt); err != nil {
		t.Fatal(err)
	}
	got, err := s.Store.SeriesInLibrary(ctx, lib.ID, nil)
	if err != nil || len(got) != 1 || got[0].Name != "Harbour Collection" || got[0].Count != 2 || got[0].Key == "" {
		t.Fatalf("%+v %v", got, err)
	}
	if names := browseTitles(t, s.Store, BrowseFilter{Library: lib.ID, Series: "Harbour Collection", Sort: "title"}); len(names) != 2 {
		t.Errorf("%v", names)
	}
	// Nothing about a series goes looking for a logo.
	if g, _ := s.Store.LogoFor(ctx, BrandSeries, "Harbour Collection"); g != nil {
		t.Errorf("%+v", g)
	}
	if collectionName("A Tale") != "A Tale Collection" || collectionName("A Tale Trilogy") != "A Tale Trilogy" {
		t.Error("collectionName")
	}
}
