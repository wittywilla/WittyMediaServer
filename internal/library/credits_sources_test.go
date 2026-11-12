// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// Every source's people, from the shapes each really sends to the credits a page shows. The
// files in testdata/credits are written by hand in those shapes; the people in them are made up.

func creditsFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "credits", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// creditsLibrary is a library of one type with no online source unless a test gives one.
func creditsLibrary(t *testing.T, typ string) (*Scanner, *Library, string) {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	root := filepath.Join(t.TempDir(), typ)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	st := &Store{DB: d}
	lib, err := st.CreateLibrary(context.Background(), "Lib", typ, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	s := &Scanner{Store: st, ImagesDir: t.TempDir(), CacheDir: t.TempDir(), Tools: &ffmpeg.Tools{}, TMDBKey: func() string { return "" },
		TVMazeOn: func() bool { return false }, WikidataOn: func() bool { return false }, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return s, lib, root
}

// allowLocalCatalogues lets the catalogue clients reach a test server on this machine.
func allowLocalCatalogues(t *testing.T) {
	old := *SafeClient
	*SafeClient = *http.DefaultClient
	t.Cleanup(func() { *SafeClient = old })
}

func writeFile(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

type creditView struct{ Name, Character, Job, Source string }

func view(list []Credit) []creditView {
	out := []creditView{}
	for _, c := range list {
		out = append(out, creditView{c.Name, c.Character, c.Job, c.Source})
	}
	return out
}

func crewOf(c *Credits, dept string) []creditView {
	for _, g := range c.Crew {
		if g.Department == dept {
			return view(g.People)
		}
	}
	return []creditView{}
}

func sameView(t *testing.T, what string, got, want []creditView) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n got  %+v\n want %+v", what, got, want)
	}
}

func countRows(t *testing.T, s *Scanner, q string, args ...any) int {
	t.Helper()
	var n int
	if err := s.Store.DB.QueryRowContext(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func personNamed(t *testing.T, s *Scanner, name string) *Person {
	t.Helper()
	var id int64
	if err := s.Store.DB.QueryRowContext(context.Background(), `SELECT id FROM people WHERE name = ?`, name).Scan(&id); err != nil {
		t.Fatalf("%s isn't a person: %v", name, err)
	}
	p, err := s.Store.Person(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func touch(t *testing.T, p string) {
	t.Helper()
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(p, later, later); err != nil {
		t.Fatal(err)
	}
}

// A Kodi movie.nfo: actors with and without <order>, two roles, a <thumb>, Kodi's "A / B"
// directors, writers in <credits>, and pictures in the film's .actors folder. When the NFO goes,
// so do its people.
func TestNFOCreditsKodiMovie(t *testing.T) {
	s, lib, root := creditsLibrary(t, "movies")
	ctx := context.Background()
	dir := filepath.Join(root, "Lantern Bay (2011)")
	media := addFile(t, dir, "Lantern Bay (2011).mkv")
	writeFile(t, filepath.Join(dir, "movie.nfo"), creditsFixture(t, "kodi-movie.nfo"))
	writeFile(t, filepath.Join(dir, ".actors", "Mei_Okafor.jpg"), testJPEG)
	writeFile(t, filepath.Join(dir, ".actors", "Tomas_Reyes.png"), pngHead)
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it := oneItem(t, s, lib.ID)
	c, err := s.Store.ItemCredits(ctx, it)
	if err != nil {
		t.Fatal(err)
	}
	sameView(t, "cast", view(c.Cast), []creditView{{"Ana Lima", "Keeper", "", "nfo"}, {"Tomas Reyes", "Ferryman / Narrator", "", "nfo"}, {"Mei Okafor", "Mayor", "", "nfo"}})
	sameView(t, "directing", crewOf(c, "directing"), []creditView{{"Dee Rector", "", "Director", "nfo"}, {"Sam Second", "", "Director", "nfo"}})
	sameView(t, "writing", crewOf(c, "writing"), []creditView{{"Dee Rector", "", "Writer", "nfo"}, {"Wren Scribe", "", "Writer", "nfo"}})
	sameView(t, "production", crewOf(c, "production"), []creditView{{"Pat Money", "", "Producer", "nfo"}})
	if ana := personNamed(t, s, "Ana Lima"); ana.Photo != "https://image.tmdb.org/t/p/original/ana.jpg" || ana.IDs.TMDB != "101" {
		t.Errorf("Ana Lima: photo %q, tmdb %q", ana.Photo, ana.IDs.TMDB)
	}
	if mei := personNamed(t, s, "Mei Okafor"); mei.Photo != filepath.Join(dir, ".actors", "Mei_Okafor.jpg") {
		t.Errorf("Mei Okafor's picture from .actors: %q", mei.Photo)
	}
	if tom := personNamed(t, s, "Tomas Reyes"); tom.Photo != filepath.Join(dir, ".actors", "Tomas_Reyes.png") {
		t.Errorf("Tomas Reyes's picture from .actors: %q", tom.Photo)
	}

	// The NFO is gone: it names nobody now, and people no title names are forgotten.
	if err := os.Remove(filepath.Join(dir, "movie.nfo")); err != nil {
		t.Fatal(err)
	}
	touch(t, media)
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	if c, _ = s.Store.ItemCredits(ctx, oneItem(t, s, lib.ID)); !c.Empty() {
		t.Errorf("credits of an NFO that's gone: %+v", c)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM people`); n != 0 {
		t.Errorf("%d people kept that no title names", n)
	}
}

// Jellyfin's tvshow.nfo (<type>Actor</type>, <sortorder>, the composer as an <actor>) and a Kodi
// episode NFO that lists a regular beside its guest stars: the regular stays a regular, the
// guests are the episode's, and the episode's own crew is put before the show's.
func TestNFOCreditsShowAndEpisode(t *testing.T) {
	s, lib, root := creditsLibrary(t, "shows")
	ctx := context.Background()
	show := filepath.Join(root, "Harbour Watch")
	writeFile(t, filepath.Join(show, "tvshow.nfo"), creditsFixture(t, "jellyfin-tvshow.nfo"))
	addFile(t, filepath.Join(show, "Season 1"), "Harbour Watch S01E01.mkv")
	writeFile(t, filepath.Join(show, "Season 1", "Harbour Watch S01E01.nfo"), creditsFixture(t, "kodi-episode.nfo"))
	writeFile(t, filepath.Join(show, ".actors", "Gus_Visitor.webp"), pngHead)
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	var showID int64
	if err := s.Store.DB.QueryRow(`SELECT id FROM shows`).Scan(&showID); err != nil {
		t.Fatal(err)
	}
	sc, err := s.Store.ShowCredits(ctx, showID, -1, nil)
	if err != nil {
		t.Fatal(err)
	}
	sameView(t, "show cast", view(sc.Cast), []creditView{{"Ana Lima", "Harbourmaster", "", "nfo"}, {"Jon Reef", "Diver", "", "nfo"}})
	sameView(t, "show directing", crewOf(sc, "directing"), []creditView{{"Rafa Helm", "", "Director", "nfo"}})
	sameView(t, "show sound", crewOf(sc, "sound"), []creditView{{"Lou Notes", "", "Composer", "nfo"}})

	it := oneItem(t, s, lib.ID)
	ec, err := s.Store.ItemCredits(ctx, it)
	if err != nil {
		t.Fatal(err)
	}
	sameView(t, "episode cast", view(ec.Cast), view(sc.Cast))
	sameView(t, "episode guests", view(ec.Guests), []creditView{{"Gus Visitor", "Smuggler", "", "nfo"}, {"Nell Passing", "Tourist", "", "nfo"}})
	sameView(t, "episode directing", crewOf(ec, "directing"), []creditView{{"Bea Stern", "", "Director", "nfo"}})
	sameView(t, "episode sound (the show's)", crewOf(ec, "sound"), []creditView{{"Lou Notes", "", "Composer", "nfo"}})
	if gus := personNamed(t, s, "Gus Visitor"); gus.Photo != filepath.Join(show, ".actors", "Gus_Visitor.webp") {
		t.Errorf("a guest's picture from the show's .actors: %q", gus.Photo)
	}
	season, err := s.Store.ShowCredits(ctx, showID, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	sameView(t, "season 1 guests", view(season.Guests), []creditView{{"Gus Visitor", "Smuggler", "", "nfo"}, {"Nell Passing", "Tourist", "", "nfo"}})
}

// fakeTMDBCredits answers as TMDB does for the film, show and season in testdata/credits, and
// can be made to fail as if the network were down.
type fakeTMDBCredits struct {
	srv  *httptest.Server
	down atomic.Bool
	hits sync.Map // path -> *atomic.Int32
}

func newFakeTMDBCredits(t *testing.T) *fakeTMDBCredits {
	allowLocalCatalogues(t)
	f := &fakeTMDBCredits{}
	movie, show, season := creditsFixture(t, "tmdb-movie.json"), creditsFixture(t, "tmdb-show.json"), creditsFixture(t, "tmdb-season.json")
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := f.hits.LoadOrStore(r.URL.Path, new(atomic.Int32))
		n.(*atomic.Int32).Add(1)
		if f.down.Load() {
			http.Error(w, "down", 503)
			return
		}
		switch r.URL.Path {
		case "/search/movie":
			_, _ = w.Write([]byte(`{"results":[{"id":7,"title":"Lantern Bay","release_date":"2011-03-04"}]}`))
		case "/movie/7":
			if !strings.Contains(r.URL.Query().Get("append_to_response"), "credits") {
				t.Errorf("a film's details without its credits: %s", r.URL.RawQuery)
			}
			_, _ = w.Write(movie)
		case "/tv/9":
			if q := r.URL.Query().Get("append_to_response"); q != "" && !strings.Contains(q, "aggregate_credits") {
				t.Errorf("a show's details without its credits: %s", r.URL.RawQuery)
			}
			_, _ = w.Write(show)
		case "/tv/9/season/1":
			_, _ = w.Write(season)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTMDBCredits) count(path string) int32 {
	if n, ok := f.hits.Load(path); ok {
		return n.(*atomic.Int32).Load()
	}
	return 0
}

// A film TMDB finds brings its whole cast and crew, every department in its place; a later scan
// that can't reach TMDB keeps them.
func TestTMDBFilmCreditsAndOfflineRescan(t *testing.T) {
	s, lib, root := creditsLibrary(t, "movies")
	fake := newFakeTMDBCredits(t)
	s.TMDBBase, s.TMDBKey = fake.srv.URL, func() string { return "key" }
	ctx := context.Background()
	media := addFile(t, root, "Lantern Bay (2011).mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	check := func(when string) {
		t.Helper()
		it := oneItem(t, s, lib.ID)
		c, err := s.Store.ItemCredits(ctx, it)
		if err != nil {
			t.Fatal(err)
		}
		sameView(t, when+": cast", view(c.Cast), []creditView{{"Ana Lima", "Keeper", "", "tmdb"}, {"Tomas Reyes", "Ferryman", "", "tmdb"}, {"Mei Okafor", "Mayor", "", "tmdb"}})
		sameView(t, when+": directing", crewOf(c, "directing"), []creditView{{"Dee Rector", "", "Director", "tmdb"}})
		sameView(t, when+": writing", crewOf(c, "writing"), []creditView{{"Dee Rector", "", "Screenplay", "tmdb"}, {"Wren Scribe", "", "Novel", "tmdb"}})
		sameView(t, when+": art", crewOf(c, "art"), []creditView{{"Ida Plans", "", "Production Design", "tmdb"}, {"Sol Decor", "", "Set Decoration", "tmdb"}})
		sameView(t, when+": costume", crewOf(c, "costume"), []creditView{{"Ines Seam", "", "Costume Design", "tmdb"}, {"Hal Powder", "", "Makeup Artist", "tmdb"}})
		sameView(t, when+": camera", crewOf(c, "camera"), []creditView{{"Bo Lens", "", "Director of Photography", "tmdb"}})
		sameView(t, when+": sound", crewOf(c, "sound"), []creditView{{"Pia Score", "", "Original Music Composer", "tmdb"}})
		sameView(t, when+": effects", crewOf(c, "effects"), []creditView{{"Vic Pixel", "", "Visual Effects Supervisor", "tmdb"}})
		if !c.Cast[0].HasPhoto || c.Cast[1].HasPhoto {
			t.Errorf("%s: pictures: %+v", when, c.Cast)
		}
		if m := s.Store.TitleMeta(ctx, TargetItem, it.ID); m.Director != "Dee Rector" {
			t.Errorf("%s: the Director field: %q", when, m.Director)
		}
	}
	check("first scan")
	if ana := personNamed(t, s, "Ana Lima"); ana.IDs.TMDB != "101" || ana.Photo != "https://image.tmdb.org/t/p/w185/ana.jpg" {
		t.Errorf("Ana Lima: %+v", ana)
	}
	if n := fake.count("/movie/7/credits"); n != 0 {
		t.Errorf("credits read with the film were asked for again %d times", n)
	}
	// The file changes while TMDB can't be reached: what TMDB said stays.
	fake.down.Store(true)
	touch(t, media)
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	check("offline rescan")
}

// A show matched to TMDB by hand: its regulars across seasons, its own crew and creators, and
// each episode's guest stars and crew from the season answer the episodes come in. Taking the
// match away takes TMDB's people with it.
func TestTMDBShowMatchCredits(t *testing.T) {
	s, lib, root := creditsLibrary(t, "shows")
	fake := newFakeTMDBCredits(t)
	s.TMDBBase, s.TMDBKey = fake.srv.URL, func() string { return "key" }
	ctx := context.Background()
	addFile(t, filepath.Join(root, "Harbour Watch", "Season 1"), "Harbour Watch S01E01.mkv")
	addFile(t, filepath.Join(root, "Harbour Watch", "Season 1"), "Harbour Watch S01E02.mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	var showID int64
	if err := s.Store.DB.QueryRow(`SELECT id FROM shows`).Scan(&showID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyMatch(ctx, title(t, s, TargetShow, showID), "tmdb-tv", "9", nil, "owner"); err != nil {
		t.Fatal(err)
	}
	sc, err := s.Store.ShowCredits(ctx, showID, -1, nil)
	if err != nil {
		t.Fatal(err)
	}
	sameView(t, "show cast", view(sc.Cast), []creditView{{"Ana Lima", "Harbourmaster", "", "tmdb"}, {"Jon Reef", "Diver / Diver's Twin", "", "tmdb"}})
	sameView(t, "show writing", crewOf(sc, "writing"), []creditView{{"Cleo Quill", "", "Creator", "tmdb"}})
	sameView(t, "show production", crewOf(sc, "production"), []creditView{{"Max Funds", "", "Executive Producer", "tmdb"}})
	sameView(t, "show directing (an episode's director isn't the show's)", crewOf(sc, "directing"), []creditView{})

	eps, err := s.Store.Episodes(ctx, showID, nil)
	if err != nil || len(eps) != 2 {
		t.Fatalf("%d episodes: %v", len(eps), err)
	}
	for _, ep := range eps {
		full, err := s.Store.Item(ctx, ep.ID)
		if err != nil {
			t.Fatal(err)
		}
		c, err := s.Store.ItemCredits(ctx, full)
		if err != nil {
			t.Fatal(err)
		}
		switch *full.Episode {
		case 1:
			sameView(t, "episode 1 guests (not the regular)", view(c.Guests), []creditView{{"Gus Visitor", "Smuggler", "", "tmdb"}})
			sameView(t, "episode 1 directing", crewOf(c, "directing"), []creditView{{"Bea Stern", "", "Director", "tmdb"}})
			sameView(t, "episode 1 writing", crewOf(c, "writing"), []creditView{{"Cleo Quill", "", "Teleplay", "tmdb"}})
		case 2:
			sameView(t, "episode 2 guests", view(c.Guests), []creditView{})
			sameView(t, "episode 2 writing (the show's)", crewOf(c, "writing"), []creditView{{"Cleo Quill", "", "Creator", "tmdb"}})
		}
	}
	if _, err := s.RemoveMatch(ctx, title(t, s, TargetShow, showID), "owner"); err != nil {
		t.Fatal(err)
	}
	if sc, _ = s.Store.ShowCredits(ctx, showID, -1, nil); !sc.Empty() {
		t.Errorf("a show's TMDB people outlived its match: %+v", sc)
	}
}

// A film matched by hand gets the match's people; taking the match away takes them.
func TestMatchedFilmCreditsFollowTheMatch(t *testing.T) {
	s, lib, root := matchFixture(t)
	ctx := context.Background()
	addFile(t, root, "Harbour Lights.mkv")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	it := oneItem(t, s, lib.ID)
	if _, err := s.ApplyMatch(ctx, title(t, s, TargetItem, it.ID), "tmdb", "2", nil, "owner"); err != nil {
		t.Fatal(err)
	}
	c, err := s.Store.ItemCredits(ctx, oneItem(t, s, lib.ID))
	if err != nil {
		t.Fatal(err)
	}
	sameView(t, "directing", crewOf(c, "directing"), []creditView{{"Dee Rector", "", "Director", "tmdb"}})
	if _, err := s.RemoveMatch(ctx, title(t, s, TargetItem, it.ID), "owner"); err != nil {
		t.Fatal(err)
	}
	if c, _ = s.Store.ItemCredits(ctx, oneItem(t, s, lib.ID)); !c.Empty() {
		t.Errorf("a film's TMDB people outlived its match: %+v", c)
	}
}

// TheTVDB names everyone as a "character": actors and their parts, the crew by people type, and
// guest stars and writers of one episode by its id.
func TestTVDBCharactersCredits(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v4/login", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"token":"tok"}}`))
	})
	mux.HandleFunc("GET /v4/series/81189/extended", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("short") != "false" {
			t.Errorf("characters come only with short=false: %s", r.URL.RawQuery)
		}
		_, _ = w.Write(creditsFixture(t, "tvdb-series.json"))
	})
	mux.HandleFunc("GET /v4/series/81189/translations/eng", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ctx := context.Background()
	m, err := (&TVDB{Key: "credits-key", BaseURL: srv.URL + "/v4", Client: srv.Client()}).Series(ctx, "81189")
	if err != nil {
		t.Fatal(err)
	}
	if !m.CreditsRead || len(m.EpisodeCredits["5551"]) != 2 {
		t.Fatalf("people of the series: read %v, episodes %+v", m.CreditsRead, m.EpisodeCredits)
	}

	s, lib, root := creditsLibrary(t, "shows")
	sh := &Show{LibraryID: lib.ID, Path: filepath.Join(root, "Harbour Watch"), Title: "Harbour Watch"}
	applySeriesMeta(sh, "tvdb", m, false, false)
	if err := s.Store.UpsertShow(ctx, sh); err != nil {
		t.Fatal(err)
	}
	sc, err := s.Store.ShowCredits(ctx, sh.ID, -1, nil)
	if err != nil {
		t.Fatal(err)
	}
	sameView(t, "cast", view(sc.Cast), []creditView{{"Ana Lima", "Harbourmaster", "", "tvdb"}, {"Jon Reef", "Diver", "", "tvdb"}})
	sameView(t, "directing", crewOf(sc, "directing"), []creditView{{"Rafa Helm", "", "Director", "tvdb"}})
	sameView(t, "writing", crewOf(sc, "writing"), []creditView{{"Cleo Quill", "", "Creator", "tvdb"}})
	sameView(t, "production", crewOf(sc, "production"), []creditView{{"Max Funds", "", "Executive Producer", "tvdb"}})
	if ana := personNamed(t, s, "Ana Lima"); ana.IDs.TVDB != "7001" || ana.Photo != "https://artworks.thetvdb.com/banners/person/7001/ana.jpg" {
		t.Errorf("Ana Lima: %+v", ana)
	}

	// The episodes: TheTVDB's own episode ids say whose guest stars are whose.
	eps := map[[2]int]EpisodeMeta{{1, 1}: {ID: "5551", Season: 1, Number: 1, Title: "Low Tide"}, {1, 2}: {ID: "5552", Season: 1, Number: 2, Title: "High Water"}}
	withEpisodeCredits(eps, m.EpisodeCredits)
	for n, want := range map[int][]creditView{1: {{"Gus Visitor", "Smuggler", "", "tvdb"}}, 2: {}} {
		season, number := 1, n
		it := &Item{LibraryID: lib.ID, Kind: "video", Path: filepath.Join(sh.Path, fmt.Sprintf("e%d.mkv", n)), ShowID: &sh.ID, Season: &season, Episode: &number}
		applyEpisodeMeta(it, "tvdb", eps, false)
		if err := s.Store.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		c, err := s.Store.ItemCredits(ctx, it)
		if err != nil {
			t.Fatal(err)
		}
		sameView(t, fmt.Sprintf("guests of episode %d", n), view(c.Guests), want)
		if !s.Store.CreditsRead(ctx, TargetItem, it.ID, CreditsTVDB) {
			t.Errorf("episode %d: TheTVDB's people weren't recorded as read", n)
		}
		if n == 1 {
			sameView(t, "episode 1 writing", crewOf(c, "writing"), []creditView{{"Wren Scribe", "", "Writer", "tvdb"}})
		}
	}
}

// TVmaze's cast and crew come in the same answer as the show (embed[]=cast&embed[]=crew).
func TestTVmazeEmbeddedCredits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shows/3":
			if q := r.URL.Query()["embed[]"]; !reflect.DeepEqual(q, []string{"cast", "crew"}) {
				t.Errorf("asked to embed %v", q)
			}
			_, _ = w.Write(creditsFixture(t, "tvmaze-show.json"))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	m, err := (&TVMaze{BaseURL: srv.URL, Client: srv.Client()}).Series(ctx, "3")
	if err != nil {
		t.Fatal(err)
	}
	s, lib, root := creditsLibrary(t, "shows")
	sh := &Show{LibraryID: lib.ID, Path: filepath.Join(root, "Harbour Watch"), Title: "Harbour Watch"}
	applySeriesMeta(sh, "tvmaze", m, false, false)
	if err := s.Store.UpsertShow(ctx, sh); err != nil {
		t.Fatal(err)
	}
	sc, err := s.Store.ShowCredits(ctx, sh.ID, -1, nil)
	if err != nil {
		t.Fatal(err)
	}
	sameView(t, "cast", view(sc.Cast), []creditView{{"Ana Lima", "Harbourmaster", "", "tvmaze"}, {"Jon Reef", "Diver", "", "tvmaze"}})
	sameView(t, "writing", crewOf(sc, "writing"), []creditView{{"Cleo Quill", "", "Creator", "tvmaze"}})
	sameView(t, "production", crewOf(sc, "production"), []creditView{{"Max Funds", "", "Executive Producer", "tvmaze"}})
	sameView(t, "camera", crewOf(sc, "camera"), []creditView{{"Bo Lens", "", "Director of Photography", "tvmaze"}})
	if ana := personNamed(t, s, "Ana Lima"); ana.IDs.TVMaze != "11" || ana.Photo != "https://static.tvmaze.com/ana-m.jpg" {
		t.Errorf("Ana Lima: %+v", ana)
	}
}

// A file's own tags: Matroska's names, MP4's ©dir; an artist is never a director.
func TestCreditsFromTags(t *testing.T) {
	type c struct{ Name, Role, Department, Job string }
	for _, tc := range []struct {
		name string
		tags map[string]string
		want []c
	}{
		{"matroska", map[string]string{
			"DIRECTOR": "Dee Rector", "WRITTEN_BY": "Wren Scribe, Dee Rector", "SCREENPLAY_BY": "Ora Draft", "PRODUCER": "Pat Money",
			"COMPOSER": "Pia Score", "ACTOR": "Ana Lima; Tomas Reyes", "DIRECTOR_OF_PHOTOGRAPHY": "Bo Lens", "EDITED_BY": "Cut Short",
			"PRODUCTION_DESIGNER": "Ida Plans", "COSTUME_DESIGNER": "Ines Seam", "ENCODER": "libebml", "ARTIST": "Some Band"},
			[]c{{"Ana Lima", RoleCast, "", ""}, {"Tomas Reyes", RoleCast, "", ""}, {"Pia Score", RoleCrew, "sound", "Composer"},
				{"Ines Seam", RoleCrew, "costume", "Costume Design"}, {"Dee Rector", RoleCrew, "directing", "Director"},
				{"Bo Lens", RoleCrew, "camera", "Director of Photography"}, {"Cut Short", RoleCrew, "editing", "Editor"},
				{"Pat Money", RoleCrew, "production", "Producer"}, {"Ida Plans", RoleCrew, "art", "Production Design"},
				{"Ora Draft", RoleCrew, "writing", "Screenplay"}, {"Wren Scribe", RoleCrew, "writing", "Writer"}, {"Dee Rector", RoleCrew, "writing", "Writer"}}},
		{"mp4", map[string]string{"©dir": "Dee Rector", "artist": "Some Band", "title": "Lantern Bay"}, []c{{"Dee Rector", RoleCrew, "directing", "Director"}}},
		{"lower case", map[string]string{"director": "Dee Rector / Sam Second"}, []c{{"Dee Rector", RoleCrew, "directing", "Director"}, {"Sam Second", RoleCrew, "directing", "Director"}}},
		{"nobody", map[string]string{"title": "Lantern Bay"}, []c{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := []c{}
			for _, in := range creditsFromTags(tc.tags) {
				cc, ok := cleanCredit(in)
				if !ok {
					t.Fatalf("not a credit: %+v", in)
				}
				got = append(got, c{cc.Name, cc.Role, cc.Department, cc.Job})
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("\n got  %+v\n want %+v", got, tc.want)
			}
		})
	}
}

// NFOs the server writes name the people another media server should see, and reading the file
// back names the same people: nobody is doubled.
func TestCreditsNFORoundTrip(t *testing.T) {
	root := t.TempDir()
	s := testScanner(t, root)
	ctx := context.Background()
	libID := int64(1)
	var movie tmdbMovie
	var extra struct {
		Credits tmdbCredits `json:"credits"`
	}
	raw := creditsFixture(t, "tmdb-movie.json")
	if json.Unmarshal(raw, &movie) != nil || json.Unmarshal(raw, &extra) != nil {
		t.Fatal("fixture")
	}

	// A film.
	media := filepath.Join(root, "Lantern Bay (2011)", "Lantern Bay (2011).mkv")
	writeFile(t, media, []byte("x"))
	it := &Item{LibraryID: libID, Kind: "video", Path: media, Title: "Lantern Bay", Provider: "tmdb", ProviderID: "7"}
	it.credits.set(CreditsTMDB, extra.Credits.list(RoleCast))
	if err := s.SaveItemMetadata(ctx, it); err != nil { // before it was ever saved: what was read is written
		t.Fatal(err)
	}
	if err := s.Store.upsertItem(ctx, it); err != nil {
		t.Fatal(err)
	}
	nfo := filepath.Join(filepath.Dir(media), "Lantern Bay (2011).nfo")
	b, err := os.ReadFile(nfo)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<name>Ana Lima</name>", "<role>Keeper</role>", "<thumb>https://image.tmdb.org/t/p/w185/ana.jpg</thumb>", "<tmdbid>101</tmdbid>",
		"<director>Dee Rector</director>", "<credits>Dee Rector</credits>", "<credits>Wren Scribe</credits>", "<producer>Pat Money</producer>",
		"<productiondesigner>Ida Plans</productiondesigner>", "<setdecorator>Sol Decor</setdecorator>", "<costumedesigner>Ines Seam</costumedesigner>",
		"<cinematographer>Bo Lens</cinematographer>", "<composer>Pia Score</composer>"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the NFO has no %s:\n%s", want, b)
		}
	}
	if strings.Contains(string(b), "<thumb></thumb>") || strings.Count(string(b), "<actor>") != 3 {
		t.Errorf("actors:\n%s", b)
	}
	people := countRows(t, s, `SELECT COUNT(*) FROM people`)
	before, _ := s.Store.ItemCredits(ctx, it)
	var back Item
	if !applyNFO(&back, nfo) {
		t.Fatal("the NFO doesn't read back")
	}
	if err := s.Store.SetCredits(ctx, TargetItem, it.ID, CreditsNFO, back.credits[CreditsNFO]); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM people`); n != people {
		t.Errorf("reading its own NFO made %d people of %d", n, people)
	}
	after, _ := s.Store.ItemCredits(ctx, it)
	if len(after.Cast) != len(before.Cast) || after.Cast[0].Source != "nfo" || after.Cast[0].Name != "Ana Lima" || after.Cast[2].Name != "Mei Okafor" {
		t.Errorf("cast read back: %+v", after.Cast)
	}

	// A show and an episode: tvshow.nfo names the regulars, the episode its regulars and guests.
	var sd struct {
		AggregateCredits tmdbCredits   `json:"aggregate_credits"`
		Credits          tmdbCredits   `json:"credits"`
		CreatedBy        []tmdbCreator `json:"created_by"`
	}
	var season struct {
		Episodes []struct {
			GuestStars []tmdbPersonCredit `json:"guest_stars"`
			Crew       []tmdbPersonCredit `json:"crew"`
		} `json:"episodes"`
	}
	if json.Unmarshal(creditsFixture(t, "tmdb-show.json"), &sd) != nil || json.Unmarshal(creditsFixture(t, "tmdb-season.json"), &season) != nil {
		t.Fatal("fixture")
	}
	sh := &Show{LibraryID: libID, Path: filepath.Join(root, "Harbour Watch"), Title: "Harbour Watch"}
	if err := os.MkdirAll(sh.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	sh.credits.set(CreditsTMDB, tmdbShowCredits(sd.AggregateCredits, sd.Credits, sd.CreatedBy))
	if err := s.Store.UpsertShow(ctx, sh); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveShowMetadata(ctx, sh); err != nil {
		t.Fatal(err)
	}
	var showBack Show
	if !applyShowNFO(&showBack, filepath.Join(sh.Path, "tvshow.nfo")) || len(showBack.credits[CreditsNFO]) == 0 {
		t.Fatalf("tvshow.nfo read back no people: %+v", showBack.credits)
	}
	epFile := filepath.Join(sh.Path, "Season 1", "Harbour Watch S01E01.mkv")
	writeFile(t, epFile, []byte("x"))
	one := 1
	ep := &Item{LibraryID: libID, Kind: "video", Path: epFile, Title: "Low Tide", ShowID: &sh.ID, Season: &one, Episode: &one}
	ep.credits.set(CreditsTMDB, (&tmdbCredits{GuestStars: season.Episodes[0].GuestStars, Crew: season.Episodes[0].Crew}).list(RoleGuest))
	if err := s.Store.upsertItem(ctx, ep); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveItemMetadata(ctx, ep); err != nil {
		t.Fatal(err)
	}
	eb, err := os.ReadFile(filepath.Join(filepath.Dir(epFile), "Harbour Watch S01E01.nfo"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<name>Ana Lima</name>", "<type>Actor</type>", "<name>Gus Visitor</name>", "<type>GuestStar</type>", "<director>Bea Stern</director>"} {
		if !strings.Contains(string(eb), want) {
			t.Errorf("the episode NFO has no %s:\n%s", want, eb)
		}
	}
	if strings.Count(string(eb), "<name>Ana Lima</name>") != 1 || strings.Contains(string(eb), "Max Funds") {
		t.Errorf("a regular twice, or the show's crew as the episode's:\n%s", eb)
	}
	people = countRows(t, s, `SELECT COUNT(*) FROM people`)
	var epBack Item
	if !applyEpisodeNFO(&epBack, filepath.Join(filepath.Dir(epFile), "Harbour Watch S01E01.nfo")) {
		t.Fatal("the episode NFO doesn't read back")
	}
	if err := s.Store.SetCredits(ctx, TargetItem, ep.ID, CreditsNFO, epBack.credits[CreditsNFO]); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.SetCredits(ctx, TargetShow, sh.ID, CreditsNFO, showBack.credits[CreditsNFO]); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM people`); n != people {
		t.Errorf("reading its own NFOs made %d people of %d", n, people)
	}
	c, _ := s.Store.ItemCredits(ctx, ep)
	sameView(t, "episode guests read back", view(c.Guests), []creditView{{"Gus Visitor", "Smuggler", "", "nfo"}})
	sameView(t, "episode cast read back", view(c.Cast), []creditView{{"Ana Lima", "Harbourmaster", "", "nfo"}, {"Jon Reef", "Diver / Diver's Twin", "", "nfo"}})
}

// The pass after a scan: titles matched before credits existed get their people a batch at a
// time, nothing is asked without a key, and it stops with the scan.
func TestCreditsBackfillIsBounded(t *testing.T) {
	allowLocalCatalogues(t)
	var credits, people atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/movie/") && strings.HasSuffix(r.URL.Path, "/credits"):
			credits.Add(1)
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/movie/"), "/credits")
			_, _ = w.Write([]byte(`{"cast":[{"id":1` + id + `,"name":"Actor ` + id + `","character":"Someone","order":0}],"crew":[]}`))
		case strings.HasPrefix(r.URL.Path, "/person/"):
			people.Add(1)
			_, _ = w.Write([]byte(`{"name":"x","biography":"Born somewhere.","birthday":"1970-01-01"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	s, lib, root := creditsLibrary(t, "movies")
	s.TMDBBase = srv.URL
	ctx := context.Background()
	const films = 30
	for i := 1; i <= films; i++ {
		name := fmt.Sprintf("Film %02d", i)
		it := &Item{LibraryID: lib.ID, Kind: "video", Path: filepath.Join(root, name+".mkv"), Title: name, Provider: "tmdb", ProviderID: strconv.Itoa(100 + i)}
		if err := s.Store.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	read := func(src string) int {
		return countRows(t, s, `SELECT COUNT(*) FROM credits_read WHERE source = ?`, src)
	}

	// Without a key nothing is asked; the NFOs (there are none) are read all the same.
	s.fillCredits(ctx, lib)
	if credits.Load() != 0 || people.Load() != 0 || read(CreditsTMDB) != 0 {
		t.Fatalf("asked TMDB without a key: %d credits, %d people", credits.Load(), people.Load())
	}
	if n := read(CreditsNFO); n != films {
		t.Errorf("NFOs read for %d of %d films", n, films)
	}

	// Stopped: nothing.
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	s.TMDBKey = func() string { return "key" }
	s.fillCredits(stopped, lib)
	if credits.Load() != 0 {
		t.Fatalf("a stopped pass asked %d times", credits.Load())
	}

	// A batch, then the rest; each film asked once.
	s.fillCredits(ctx, lib)
	if n := read(CreditsTMDB); n != creditsOnlineBatch || int(credits.Load()) != creditsOnlineBatch {
		t.Errorf("first pass: %d read, %d asked; want %d", n, credits.Load(), creditsOnlineBatch)
	}
	if n := people.Load(); int(n) != creditsPeopleBatch {
		t.Errorf("first pass looked up %d people; want %d", n, creditsPeopleBatch)
	}
	s.fillCredits(ctx, lib)
	s.fillCredits(ctx, lib)
	if n := read(CreditsTMDB); n != films || credits.Load() != films {
		t.Errorf("after three passes: %d read, %d asked; want %d", n, credits.Load(), films)
	}
	var withCast int
	if err := s.Store.DB.QueryRow(`SELECT COUNT(DISTINCT target_id) FROM credits WHERE source = 'tmdb' AND role = 'cast'`).Scan(&withCast); err != nil || withCast != films {
		t.Errorf("%d films have a cast: %v", withCast, err)
	}
}
