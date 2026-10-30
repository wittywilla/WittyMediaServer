package library

import (
	"context"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// Lists are filtered in SQL, single items in Go; both must give one answer.
// An episode of a blocked-genre show was listed while refusing to play.
func TestItemClauseCountsShowGenres(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	st := &Store{DB: d}
	lib, err := st.CreateLibrary(ctx, "TV", "shows", []string{"/t"})
	if err != nil {
		t.Fatal(err)
	}
	sh := &Show{LibraryID: lib.ID, Path: "/t/a", Title: "A", Genres: []string{"Horror"}}
	if err := st.UpsertShow(ctx, sh); err != nil {
		t.Fatal(err)
	}
	ep := &Item{LibraryID: lib.ID, Kind: "video", Path: "/t/a/1.mkv", Title: "One", SortTitle: "One", ShowID: &sh.ID}
	if err := st.upsertItem(ctx, ep); err != nil {
		t.Fatal(err)
	}
	a := &Access{MaxAge: NoAgeLimit, BlockGenres: []string{"Horror"}}
	if a.AllowsItem(ep, sh) {
		t.Fatal("the Go check allows it")
	}
	items, _, err := st.Items(ctx, ItemQuery{LibraryID: lib.ID, Access: a})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("the list shows %d episodes of a blocked show", len(items))
	}
	if g, err := st.Genres(ctx, 0, &Access{Libraries: []int64{lib.ID + 1}}); err != nil || len(g) != 0 {
		t.Fatalf("genres of a library the account can't open: %v %v", g, err)
	}
}

func TestAgeFor(t *testing.T) {
	cases := map[string]int{"G": 0, "PG-13": 13, "pg13": 13, "US:R": 17, "TV-Y7": 7, "TV-MA": 17, "12A": 12, "FSK 16": 16, "de/12": 12, "16+": 16}
	for in, want := range cases {
		got := AgeFor(in)
		if got == nil || *got != want {
			t.Errorf("AgeFor(%q) = %v, want %d", in, got, want)
		}
	}
	for _, in := range []string{"", "NR", "Unrated", "weird"} {
		if AgeFor(in) != nil {
			t.Errorf("AgeFor(%q) should be unknown", in)
		}
	}
}

func TestAccessQueries(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	s := &Store{DB: d}
	lib, err := s.CreateLibrary(ctx, "Movies", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	add := func(title, rating string, genres ...string) *Item {
		it := &Item{LibraryID: lib.ID, Kind: "video", Path: "/m/" + title, Title: title, SortTitle: title, ContentRating: rating, Genres: genres}
		if err := s.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		return it
	}
	cars := add("Cars", "G")
	shrek := add("Shrek", "PG")
	add("Alien", "R", "Horror")
	home := add("Birthday", "")
	titles := func(a *Access) map[string]bool {
		items, _, err := s.Items(ctx, ItemQuery{Access: a})
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]bool{}
		for _, it := range items {
			m[it.Title] = true
			if !a.AllowsItem(it, nil) {
				t.Errorf("SQL and Go disagree about %s", it.Title)
			}
		}
		return m
	}
	if got := titles(nil); len(got) != 4 {
		t.Fatalf("no limits: %v", got)
	}
	kids := &Access{MaxAge: 7}
	if got := titles(kids); !got["Cars"] || got["Shrek"] || got["Alien"] || got["Birthday"] {
		t.Errorf("kids 7+: %v", got)
	}
	kids.AllowUnrated = true
	kids.AllowItems = []int64{shrek.ID}
	kids.BlockItems = []int64{cars.ID}
	if got := titles(kids); got["Cars"] || !got["Shrek"] || !got["Birthday"] || got["Alien"] {
		t.Errorf("overrides: %v", got)
	}
	adult := &Access{MaxAge: NoAgeLimit, AllowUnrated: true, BlockGenres: []string{"Horror"}}
	if got := titles(adult); got["Alien"] || !got["Shrek"] {
		t.Errorf("genre block: %v", got)
	}
	other := &Access{MaxAge: NoAgeLimit, AllowUnrated: true, Libraries: []int64{lib.ID + 1}}
	if got := titles(other); len(got) != 0 {
		t.Errorf("library limit: %v", got)
	}
	_ = home
}
