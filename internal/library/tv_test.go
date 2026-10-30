package library

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseEpisode(t *testing.T) {
	root := filepath.FromSlash("/media/tv")
	cases := []struct {
		path              string
		show              string
		season, ep, epEnd int
		title             string
	}{
		{"Codename Kids Next Door/Season 2/Codename - Kids Next Door - S02E01 - Operation - C.A.T.S WEBDL-1080p.mkv", "Codename Kids Next Door", 2, 1, 0, "Operation - C.A.T.S"},
		{"Codename Kids Next Door/Season 2/Codename - Kids Next Door - S02E15-E16 - Operation - T.H.E.-S.H.O.G.U.N + Operation - C.O.L.L.E.G.E WEBDL-1080p.mkv", "Codename Kids Next Door", 2, 15, 16, "Operation - T.H.E.-S.H.O.G.U.N + Operation - C.O.L.L.E.G.E"},
		{"The Office (2005)/The.Office.S03E10.A.Benihana.Christmas.720p.WEB-DL.mkv", "The Office", 3, 10, 0, "A Benihana Christmas"},
		{"Lost/2x05 - ...And Found.avi", "Lost", 2, 5, 0, "And Found"},
		{"Firefly/Season 1/05 - Safe.mkv", "Firefly", 1, 5, 0, "Safe"},
		{"Firefly/Specials/Episode 2 - Here's How It Was.mkv", "Firefly", 0, 2, 0, "Here's How It Was"},
		{"Breaking.Bad.S01E01E02.Pilot.mkv", "Breaking Bad", 1, 1, 2, "Pilot"},
		{"Show/Season 01/Show.s01e03.mkv", "Show", 1, 3, 0, ""},
		// Anime-style absolute numbering
		{"[SubsPlease] Frieren - 012 [1080p].mkv", "Frieren", 1, 12, 0, ""},
		{"Cowboy Bebop/Cowboy Bebop - 05v2.mkv", "Cowboy Bebop", 1, 5, 0, ""},
		{"[Group] One Piece/One Piece - 1071 - The Gear 5.mkv", "One Piece", 1, 1071, 0, "The Gear 5"},
	}
	for _, c := range cases {
		ref, ok := ParseEpisode(root, filepath.Join(root, filepath.FromSlash(c.path)))
		if !ok {
			t.Errorf("%s: not recognised", c.path)
			continue
		}
		if ref.Show != c.show || ref.Season != c.season || ref.Episode != c.ep || ref.EpisodeEnd != c.epEnd || ref.Title != c.title {
			t.Errorf("%s:\n got  %q S%dE%d-%d %q\n want %q S%dE%d-%d %q", c.path, ref.Show, ref.Season, ref.Episode, ref.EpisodeEnd, ref.Title, c.show, c.season, c.ep, c.epEnd, c.title)
		}
	}
	if _, ok := ParseEpisode(root, filepath.Join(root, "Movies", "Shrek (2001).mkv")); ok {
		t.Error("a movie was parsed as an episode")
	}
	if ref, ok := ParseEpisode(root, filepath.Join(root, "Blade Runner 2049.mkv")); ok && ref.Episode == 2049 {
		t.Error("a year was parsed as an episode number")
	}
}

func TestTVDBClient(t *testing.T) {
	logins := 0
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v4/login", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]string
		_ = json.NewDecoder(r.Body).Decode(&b)
		if b["apikey"] != "k" {
			w.WriteHeader(401)
			return
		}
		logins++
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"token": "tok"}})
	})
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(401)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("GET /v4/search", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "Codename Kids Next Door" || r.URL.Query().Get("type") != "series" {
			t.Errorf("search query %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"data":[{"tvdb_id":"76163","name":"Codename: Kids Next Door","year":"2002","image_url":"https://artworks.thetvdb.com/p.jpg"}]}`))
	}))
	mux.HandleFunc("GET /v4/series/76163/extended", auth(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":76163,"name":"Codename: Kids Next Door","year":"2002","firstAired":"2002-12-06","overview":null,
			"image":"https://artworks.thetvdb.com/poster.jpg","score":12345,"averageRuntime":22,
			"status":{"name":"Ended"},"originalNetwork":{"name":"Cartoon Network"},
			"genres":[{"name":"Animation"},{"name":"Comedy"}],
			"contentRatings":[{"name":"TV-Y7","country":"usa"}],
			"artworks":[{"type":3,"image":"https://artworks.thetvdb.com/bg.jpg","score":10,"language":"eng"},
			            {"type":7,"image":"https://artworks.thetvdb.com/s2.jpg","seasonId":555,"score":5,"language":"eng"}],
			"seasons":[{"id":555,"number":2,"type":{"type":"official"},"image":""}]}}`))
	}))
	mux.HandleFunc("GET /v4/series/76163/translations/eng", auth(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"name":"Codename: Kids Next Door","overview":"Five kids fight adult tyranny."}}`))
	}))
	mux.HandleFunc("GET /v4/series/76163/episodes/default/eng", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "0" {
			_, _ = w.Write([]byte(`{"data":{"episodes":[{"id":1,"seasonNumber":2,"number":1,"name":"Operation: C.A.T.S.","overview":"Cats!","aired":"2003-06-06","runtime":11,"image":"https://artworks.thetvdb.com/e1.jpg"}]},"links":{"next":"x?page=1"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"episodes":[{"id":2,"seasonNumber":2,"number":2,"name":"Operation: P.O.P.","aired":"2003-06-13"}]},"links":{"next":null}}`))
	}))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &TVDB{Key: "k", BaseURL: srv.URL + "/v4", Client: srv.Client()}
	ctx := context.Background()
	s, err := c.FindSeries(ctx, "Codename Kids Next Door", nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "76163" || s.Title != "Codename: Kids Next Door" || s.Overview != "Five kids fight adult tyranny." ||
		s.Network != "Cartoon Network" || s.Status != "Ended" || s.ContentRating != "TV-Y7" || len(s.Genres) != 2 ||
		s.Backdrop != "https://artworks.thetvdb.com/bg.jpg" || s.Poster != "https://artworks.thetvdb.com/poster.jpg" ||
		s.SeasonPosters[2] != "https://artworks.thetvdb.com/s2.jpg" || s.Year == nil || *s.Year != 2002 {
		t.Fatalf("series %+v", s)
	}
	eps, err := c.Episodes(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 2 || eps[[2]int{2, 1}].Title != "Operation: C.A.T.S." || eps[[2]int{2, 2}].Aired != "2003-06-13" {
		t.Fatalf("episodes %+v", eps)
	}
	if logins != 1 {
		t.Fatalf("logged in %d times; the token should be reused", logins)
	}
}

func TestCommercialsKeepSmallFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Soda Ad.mp4"), []byte("tiny"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := discoverClips(dir)
	if err != nil || len(c) != 1 {
		t.Fatalf("a small commercial should be found: %v %+v", err, c)
	}
	c, err = discover(dir)
	if err != nil || len(c) != 0 {
		t.Fatalf("elsewhere, tiny videos are samples: %v %+v", err, c)
	}
}
