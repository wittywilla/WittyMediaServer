// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// putSDUIPage saves a page as the administrator.
func putSDUIPage(t *testing.T, ts *testServer, admin string, p sdui.Page) {
	t.Helper()
	etag := ts.do(t, "GET", "/api/sdui/config", admin, nil).Header().Get("ETag")
	if w := ts.do(t, "PUT", "/api/sdui/config/pages/"+p.ID, admin, p, "If-Match", etag); w.Code != 200 {
		t.Fatalf("saving %s: %d %s", p.ID, w.Code, w.Body)
	}
}

// sectionCards are the titles and subtitles of the cards of a section's first page, as the
// account gets them; nil and the status when it gets none.
func sectionCards(t *testing.T, ts *testServer, tok, page, section, query string) ([]string, int) {
	t.Helper()
	w := ts.do(t, "GET", "/api/sdui/pages/"+page+"/sections/"+section+"/items?cursor=0&formFactor=desktop"+query, tok, nil)
	if w.Code != 200 {
		return nil, w.Code
	}
	var got sdui.SectionItems
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("%s/%s: %v %s", page, section, err, w.Body)
	}
	var out []string
	for _, n := range got.Items {
		s := fmt.Sprint(n.Props["title"])
		if sub, _ := n.Props["subtitle"].(string); sub != "" {
			s += " (" + sub + ")"
		}
		out = append(out, s)
	}
	return out, 200
}

func titlesOnly(cards []string) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i], _, _ = strings.Cut(c, " (")
	}
	return out
}

type booksFixture struct {
	ts                    *testServer
	admin, adult, blocked string
	films, books          int64
	moon1, moon2, moon3   *library.Item
	lone, strip           *library.Item
}

// A books library: Moon Hollow 1 to 3 (a series by A. Writer, EPUB, 3 kept from one account),
// Lone (no series, B. Author, a PDF) and Strip (a comic in the series Strips, no author).
func newBooksFixture(t *testing.T) *booksFixture {
	t.Helper()
	ts := newTestServer(t)
	root := t.TempDir()
	f := &booksFixture{ts: ts}
	f.films = ts.library(t, "movies", filepath.Join(root, "films"))
	f.books = ts.library(t, "books", filepath.Join(root, "books"))
	n := 0
	mk := func(title, series string, track int, author, container string) *library.Item {
		n++
		p := filepath.Join(root, "books", title+"."+container)
		put(t, p, []byte("book"))
		it := &library.Item{LibraryID: f.books, Kind: "book", Path: p, Title: title, Album: series, Artist: author, AlbumArtist: author, Container: container, Poster: "cache:c.jpg",
			AddedAt: int64(1000 + n)}
		if track > 0 {
			it.Track = &track
		}
		return ts.item(t, it)
	}
	// Stored out of order, so the order the source gives is its own.
	f.moon2 = mk("Moon Hollow 2", "Moon Hollow", 2, "A. Writer", "epub")
	f.lone = mk("Lone", "", 0, "B. Author", "pdf")
	f.moon1 = mk("Moon Hollow 1", "Moon Hollow", 1, "A. Writer", "epub")
	f.strip = mk("Strip", "Strips", 1, "", "cbz")
	f.moon3 = mk("Moon Hollow 3", "Moon Hollow", 3, "A. Writer", "epub")
	_, f.admin = ts.user(t, "owner", auth.KindAdult, true, "")
	_, f.adult = ts.user(t, "alex", auth.KindAdult, false, fmt.Sprintf(`{"maxAge":-1,"allowUnrated":true,"libraries":[%d]}`, f.films))
	_, f.blocked = ts.user(t, "bo", auth.KindAdult, false, fmt.Sprintf(`{"maxAge":-1,"allowUnrated":true,"blockItems":[%d]}`, f.moon3.ID))
	return f
}

// The books source is a books library's shelf, narrowed and ordered the way WittyInk's
// bookshelf is, and only ever what the account may see.
func TestSDUIBooksSource(t *testing.T) {
	f := newBooksFixture(t)
	ts := f.ts
	src := func(extra map[string]any) sdui.Source {
		s := sdui.Source{"type": "books", "library": f.books}
		for k, v := range extra {
			s[k] = v
		}
		return s
	}
	shelf := sdui.Page{ID: "shelf", Kind: "custom", Title: "Shelf", Sections: []sdui.Section{
		{ID: "all", Component: "Shelf", Source: src(nil)},
		{ID: "series", Component: "Shelf", Source: src(map[string]any{"series": "Moon Hollow"})},
		{ID: "series-back", Component: "Shelf", Source: src(map[string]any{"series": "Moon Hollow", "desc": true})},
		{ID: "none", Component: "Shelf", Source: src(map[string]any{"series": sdui.BooksNone})},
		{ID: "author", Component: "Shelf", Source: src(map[string]any{"author": "A. Writer", "sort": "series"})},
		{ID: "no-author", Component: "Shelf", Source: src(map[string]any{"author": sdui.BooksNone})},
		{ID: "pdfs", Component: "Shelf", Source: src(map[string]any{"format": "pdf"})},
		{ID: "comics", Component: "Shelf", Source: src(map[string]any{"format": "cbz"})},
		{ID: "by-author", Component: "Shelf", Source: src(map[string]any{"sort": "author"})},
		{ID: "added", Component: "Shelf", Source: src(map[string]any{"sort": "added"})},
		{ID: "films", Component: "Shelf", Source: sdui.Source{"type": "books", "library": f.films}},
	}}
	putSDUIPage(t, ts, f.admin, shelf)
	want := map[string][]string{
		"all":         {"Lone", "Moon Hollow 1", "Moon Hollow 2", "Moon Hollow 3", "Strip"},
		"series":      {"Moon Hollow 1", "Moon Hollow 2", "Moon Hollow 3"},
		"series-back": {"Moon Hollow 3", "Moon Hollow 2", "Moon Hollow 1"},
		"none":        {"Lone"},
		"author":      {"Moon Hollow 1", "Moon Hollow 2", "Moon Hollow 3"},
		"no-author":   {"Strip"},
		"pdfs":        {"Lone"},
		"comics":      {"Strip"},
		// By author, the books with none last.
		"by-author": {"Moon Hollow 1", "Moon Hollow 2", "Moon Hollow 3", "Lone", "Strip"},
		"added":     {"Moon Hollow 3", "Strip", "Moon Hollow 1", "Lone", "Moon Hollow 2"},
		"films":     nil,
	}
	for section, titles := range want {
		got, code := sectionCards(t, ts, f.admin, "shelf", section, "")
		if code != 200 || !slices.Equal(titlesOnly(got), titles) {
			t.Errorf("%s: %d %v, want %v", section, code, got, titles)
		}
	}
	// An account kept from one book never gets it; one kept from the library gets nothing.
	if got, _ := sectionCards(t, ts, f.blocked, "shelf", "series", ""); !slices.Equal(titlesOnly(got), []string{"Moon Hollow 1", "Moon Hollow 2"}) {
		t.Errorf("series for an account kept from a book: %v", got)
	}
	if got, _ := sectionCards(t, ts, f.adult, "shelf", "all", ""); len(got) != 0 {
		t.Errorf("books for an account kept from the library: %v", got)
	}

	// The built-in books library page: a row per series, the books with none last, and a
	// row per author; none of them with the book the account is kept from.
	rows := func(tok, tab, prefix string) map[string][]string {
		t.Helper()
		doc := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/library-books?library=%d&tab=%s&formFactor=desktop", f.books, tab), tok, nil))
		out := map[string][]string{}
		var walk func(n *sdui.Node)
		walk = func(n *sdui.Node) {
			if strings.HasPrefix(n.ID, prefix) && n.Type == "Shelf" {
				title := fmt.Sprint(n.Props["title"])
				cards, _ := sectionCards(t, ts, tok, "library-books", n.ID, fmt.Sprintf("&library=%d", f.books))
				out[title] = titlesOnly(cards)
			}
			for i := range n.Children {
				walk(&n.Children[i])
			}
		}
		walk(&doc.Root)
		return out
	}
	series := rows(f.blocked, "series", "in-series-")
	if len(series) != 3 || !slices.Equal(series["Moon Hollow"], []string{"Moon Hollow 1", "Moon Hollow 2"}) || !slices.Equal(series["Strips"], []string{"Strip"}) ||
		!slices.Equal(series["No series"], []string{"Lone"}) {
		t.Errorf("series rows: %v", series)
	}
	authors := rows(f.admin, "authors", "by-author-")
	if len(authors) != 3 || !slices.Equal(authors["A. Writer"], []string{"Moon Hollow 1", "Moon Hollow 2", "Moon Hollow 3"}) || !slices.Equal(authors["No author"], []string{"Strip"}) {
		t.Errorf("author rows: %v", authors)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/library-books?library=%d&tab=series", f.books), f.adult, nil); w.Code != 404 {
		t.Errorf("the books page for an account kept from the library: %d", w.Code)
	}
}

// GET /music/albums and /music/artists answer a books library with its series and authors
// (WittyMediaServer's own Books page asks them for its Series tab), within the account's
// limits, where they used to answer nothing.
func TestMusicAlbumsAnswerABooksLibrary(t *testing.T) {
	f := newBooksFixture(t)
	ts := f.ts
	var albums []library.Album
	w := ts.do(t, "GET", fmt.Sprintf("/api/music/albums?library=%d&sort=title", f.books), f.blocked, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &albums); err != nil || w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got := map[string]int{}
	for _, a := range albums {
		got[a.Title] = a.Tracks
	}
	if got["Moon Hollow"] != 2 || got["Strips"] != 1 || got[""] != 1 {
		t.Errorf("series as albums: %+v", albums)
	}
	var artists []library.Artist
	_ = json.Unmarshal(ts.do(t, "GET", fmt.Sprintf("/api/music/artists?library=%d", f.books), f.admin, nil).Body.Bytes(), &artists)
	names := []string{}
	for _, a := range artists {
		names = append(names, a.Name)
	}
	if !slices.Equal(names, []string{"", "A. Writer", "B. Author"}) {
		t.Errorf("authors as artists: %v", names)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/music/albums?library=%d", f.books), f.adult, nil); w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("series for an account kept from the library: %d %s", w.Code, w.Body)
	}
}

// Statistics: a profile sees its own record; everyone's only an administrator; nothing names
// or counts a title the profile may not open; the period counts back from today.
func TestSDUIStatisticsSource(t *testing.T) {
	f := newSDUIFixture(t)
	ts := f.ts
	admin, one, kid := ts.userID(t, f.admin), ts.userID(t, f.oneLibrary), ts.userID(t, f.kid)
	today := library.DayNumber(time.Now())
	old := library.DayNumber(time.Now().AddDate(0, 0, -100))
	for _, row := range []struct {
		user int64
		item *library.Item
		day  int
		secs float64
	}{
		{admin, f.secretOne, today, 20000},
		{one, f.family, today, 3000},
		{one, f.grownUp, today, 1200},
		{one, f.grownUp, old, 90000},     // long ago: only all time counts it
		{one, f.secretOne, today, 50000}, // in a library it may no longer open
		{kid, f.family, today, 600},
		{kid, f.grownUp, today, 7000}, // rated beyond it
	} {
		if _, err := ts.DB.Exec(`INSERT INTO play_log(user_id, item_id, day, seconds) VALUES(?, ?, ?, ?)`, row.user, row.item.ID, row.day, row.secs); err != nil {
			t.Fatal(err)
		}
	}
	putSDUIPage(t, ts, f.admin, sdui.Page{ID: "stats", Kind: "custom", Title: "Statistics", Sections: []sdui.Section{
		{ID: "month", Component: "Shelf", Source: sdui.Source{"type": "statistics"}},
		{ID: "ever", Component: "Shelf", Source: sdui.Source{"type": "statistics", "period": "all"}},
		{ID: "everyone", Component: "Shelf", Source: sdui.Source{"type": "statistics", "who": "everyone"}},
		{ID: "libraries", Component: "Shelf", Source: sdui.Source{"type": "statistics", "stat": "libraries", "period": "all"}},
	}})
	cases := []struct {
		tok, section string
		want         []string
	}{
		{f.oneLibrary, "month", []string{"Family Film (50 min)", "Grown-up Film (20 min)"}},
		{f.oneLibrary, "ever", []string{"Grown-up Film (25 h 20 min)", "Family Film (50 min)"}},
		{f.oneLibrary, "everyone", nil},
		{f.oneLibrary, "libraries", []string{"movies (26 h 10 min)"}},
		{f.kid, "month", []string{"Family Film (10 min)"}},
		{f.kid, "everyone", nil},
		{f.admin, "month", []string{"Secret Film (5 h 33 min)"}},
		{f.admin, "everyone", []string{"Secret Film (19 h 26 min · 2 people)", "Grown-up Film (2 h 16 min · 2 people)", "Family Film (1 h · 2 people)"}},
	}
	for _, c := range cases {
		got, code := sectionCards(t, ts, c.tok, "stats", c.section, "")
		if code != 200 || !slices.Equal(got, c.want) {
			t.Errorf("%s for %s: %d %v, want %v", c.section, c.tok[:6], code, got, c.want)
		}
	}
	// Shows: the episodes of one show together, the show's own card.
	show := ts.library(t, "shows", t.TempDir())
	if _, err := ts.DB.Exec(`INSERT INTO shows(id, library_id, path, title, sort_title, added_at, updated_at) VALUES(55, ?, '/s', 'A Show', 'a show', 1, 1)`, show); err != nil {
		t.Fatal(err)
	}
	sh := int64(55)
	for i, secs := range []float64{1800, 2400} {
		s, e := 1, i+1
		ep := ts.item(t, &library.Item{LibraryID: show, Kind: "video", Path: filepath.Join(t.TempDir(), "e.mkv"), Title: fmt.Sprint("Episode ", e), ShowID: &sh, Season: &s, Episode: &e})
		if _, err := ts.DB.Exec(`INSERT INTO play_log(user_id, item_id, day, seconds) VALUES(?, ?, ?, ?)`, admin, ep.ID, today, secs); err != nil {
			t.Fatal(err)
		}
	}
	putSDUIPage(t, ts, f.admin, sdui.Page{ID: "shows", Kind: "custom", Title: "Shows", Sections: []sdui.Section{
		{ID: "shows", Component: "Shelf", Source: sdui.Source{"type": "statistics", "stat": "shows"}}}})
	if got, _ := sectionCards(t, ts, f.admin, "shows", "shows", ""); !slices.Equal(got, []string{"A Show (1 h 10 min)"}) {
		t.Errorf("shows: %v", got)
	}
}

func TestTimeWatched(t *testing.T) {
	for sec, want := range map[float64]string{0: "Under a minute", 59: "Under a minute", 60: "1 min", 3600: "1 h", 3660: "1 h 1 min", 90000: "25 h"} {
		if got := timeWatched(sec); got != want {
			t.Errorf("%v seconds: %q, want %q", sec, got, want)
		}
	}
}

// Liked: what the profile liked, most recent first, never what it may not see now.
func TestSDUILikedSource(t *testing.T) {
	f := newSDUIFixture(t)
	ts := f.ts
	for _, r := range []struct {
		tok  string
		item *library.Item
		at   int64
	}{
		{f.oneLibrary, f.family, 1}, {f.oneLibrary, f.grownUp, 2}, {f.oneLibrary, f.secretOne, 3},
		{f.kid, f.grownUp, 1}, {f.kid, f.family, 2},
	} {
		if _, err := ts.DB.Exec(`INSERT INTO item_reactions(user_id, item_id, value, created_at) VALUES(?, ?, 1, ?)`, ts.userID(t, r.tok), r.item.ID, r.at); err != nil {
			t.Fatal(err)
		}
	}
	// A dislike is no like.
	if _, err := ts.DB.Exec(`INSERT INTO item_reactions(user_id, item_id, value, created_at) VALUES(?, ?, -1, 9)`, ts.userID(t, f.admin), f.family.ID); err != nil {
		t.Fatal(err)
	}
	putSDUIPage(t, ts, f.admin, sdui.Page{ID: "likes", Kind: "custom", Title: "Likes", Sections: []sdui.Section{
		{ID: "liked", Component: "Shelf", Source: sdui.Source{"type": "liked"}},
		{ID: "read", Component: "Shelf", Source: sdui.Source{"type": "liked", "media": "read"}}}})
	for _, c := range []struct {
		tok, section string
		want         []string
	}{
		{f.oneLibrary, "liked", []string{"Grown-up Film", "Family Film"}},
		{f.oneLibrary, "read", nil},
		{f.kid, "liked", []string{"Family Film"}},
		{f.admin, "liked", nil},
	} {
		if got, _ := sectionCards(t, ts, c.tok, "likes", c.section, ""); !slices.Equal(titlesOnly(got), c.want) {
			t.Errorf("%s for %s: %v, want %v", c.section, c.tok[:6], got, c.want)
		}
	}
}

// Top Shelf takes at most two rows from the modules' offered sources, in order, skipping
// any that fail or are empty, titled with the source's title unless it needs a page.
func TestTopShelfRowsFromModules(t *testing.T) {
	card := []sdui.Item{{ID: "1", Kind: "game", Title: "Go", Route: "/page/x"}}
	answers := map[string]sdui.SourceResult{"x.empty": {}, "x.picks": {Items: card}, "x.each": {Items: card}, "x.more": {Items: card}}
	var asked []string
	rows := topShelfRowsFrom([]sdui.SourceSpec{
		{Type: "x.broken", Label: "Broken"}, {Type: "x.empty", Label: "Empty"}, {Type: "x.picks", Label: "Picks", Title: "Tonight's picks"},
		{Type: "x.each", Label: "Each", Title: "{each}"}, {Type: "x.more", Label: "More"},
	}, func(src sdui.Source) (sdui.SourceResult, error) {
		asked = append(asked, src.Type())
		if src.Type() == "x.broken" {
			return sdui.SourceResult{}, errors.New("not running")
		}
		return answers[src.Type()], nil
	})
	if len(rows) != 2 || rows[0].Title != "Tonight's picks" || rows[1].Title != "Each" {
		t.Errorf("rows %+v", rows)
	}
	if !slices.Equal(asked, []string{"x.broken", "x.empty", "x.picks", "x.each"}) {
		t.Errorf("asked %v", asked)
	}
}
