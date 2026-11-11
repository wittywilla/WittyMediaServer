// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/parental"
)

// The limits are applied twice: as SQL for lists and in Go for one title. This
// walks every title of a small library through both for each kind of limit,
// and fails when they disagree or when either lets through what it shouldn't.
func TestLimitsInSQLAndInGoAgree(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	s := &Store{DB: d}
	lib, err := s.CreateLibrary(ctx, "Everything", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateLibrary(ctx, "Other", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	show := &Show{LibraryID: lib.ID, Path: "/tv/show", Title: "Teen show", SortTitle: "Teen show", ContentRating: "TV-14", Genres: []string{"Drama"}}
	if err := s.UpsertShow(ctx, show); err != nil {
		t.Fatal(err)
	}
	kidShow := &Show{LibraryID: lib.ID, Path: "/tv/kid", Title: "Kid show", SortTitle: "Kid show", ContentRating: "TV-Y", Genres: []string{"Family"}}
	if err := s.UpsertShow(ctx, kidShow); err != nil {
		t.Fatal(err)
	}
	all := map[string]*Item{}
	add := func(name, kind, rating string, showID *int64, libID int64, genres ...string) *Item {
		it := &Item{LibraryID: libID, Kind: kind, Path: "/m/" + name, Title: name, SortTitle: name, ContentRating: rating, Genres: genres, ShowID: showID}
		if err := s.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		all[name] = it
		return it
	}
	add("film-g", "video", "G", nil, lib.ID, "Family")
	add("film-pg13", "video", "PG-13", nil, lib.ID, "Action")
	add("film-r", "video", "R", nil, lib.ID, "Horror")
	add("film-unrated", "video", "", nil, lib.ID)
	add("film-fsk12", "video", "FSK 12", nil, lib.ID)
	add("film-elsewhere", "video", "G", nil, other.ID, "Family")
	add("disc-r", "dvd-iso", "R", nil, lib.ID)
	add("ep-teen", "video", "", &show.ID, lib.ID)
	add("ep-teen-rated-ma", "video", "TV-MA", &show.ID, lib.ID)
	add("ep-kid", "video", "", &kidShow.ID, lib.ID)
	add("game-e", "game", "ESRB E", nil, lib.ID)
	add("game-pegi16", "game", "PEGI 16", nil, lib.ID)
	add("game-unrated", "game", "", nil, lib.ID)
	add("song-clean", "audio", "Clean", nil, lib.ID)
	add("song-explicit", "audio", "Explicit", nil, lib.ID)
	add("song-unrated", "audio", "", nil, lib.ID)
	add("book-12", "book", "12", nil, lib.ID)
	add("book-unrated", "book", "", nil, lib.ID)
	add("photo", "photo", "", nil, lib.ID)
	add("clip-tagged", "video", "G", nil, lib.ID)
	if _, err := d.ExecContext(ctx, `INSERT INTO video_meta(item_id, library_id, tags) VALUES(?, ?, 'pranks|challenge')`, all["clip-tagged"].ID, lib.ID); err != nil {
		t.Fatal(err)
	}
	// Two collections that list their titles: one of a film and a show, one of a game.
	if _, err := d.ExecContext(ctx, `INSERT INTO users(id, name, password_hash, is_admin, created_at) VALUES(1, 'owner', '', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO collections(id, owner_id, name, created_at, updated_at) VALUES(1, 1, 'Scary', 1, 1), (2, 1, 'Approved', 1, 1)`,
	} {
		if _, err := d.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO collection_items(collection_id, item_id, added_at) VALUES(1, ?, 1), (2, ?, 1), (2, ?, 1)`, all["film-g"].ID, all["game-pegi16"].ID, all["film-r"].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO collection_shows(collection_id, show_id, added_at) VALUES(1, ?, 1), (2, ?, 1)`, kidShow.ID, show.ID); err != nil {
		t.Fatal(err)
	}

	everything := Access{MaxAge: NoAgeLimit, AllowUnrated: true}
	cases := []struct {
		name string
		a    Access
		// want names every title the limits let through, and wantShows every show.
		want      string
		wantShows string
	}{
		{"no limit of any kind", everything,
			"book-12 book-unrated clip-tagged disc-r ep-kid ep-teen ep-teen-rated-ma film-elsewhere film-fsk12 film-g film-pg13 film-r film-unrated game-e game-pegi16 game-unrated photo song-clean song-explicit song-unrated",
			"Kid show, Teen show"},
		{"one age for everything", Access{MaxAge: 12},
			"book-12 clip-tagged ep-kid film-elsewhere film-fsk12 film-g game-e song-clean", "Kid show"},
		{"one age, unrated allowed", Access{MaxAge: 12, AllowUnrated: true},
			"book-12 book-unrated clip-tagged ep-kid film-elsewhere film-fsk12 film-g film-unrated game-e game-unrated photo song-clean song-unrated", "Kid show"},
		{"films to 13, the rest as it was", Access{MaxAge: 7, Kinds: map[string]KindLimit{parental.KindFilm: {MaxAge: 13}}},
			"clip-tagged ep-kid film-elsewhere film-fsk12 film-g film-pg13 game-e song-clean", "Kid show"},
		{"television to 14, films to 0", Access{MaxAge: 0, Kinds: map[string]KindLimit{parental.KindTV: {MaxAge: 14}}},
			"clip-tagged ep-kid ep-teen film-elsewhere film-g game-e song-clean", "Kid show, Teen show"},
		{"games to PEGI 16 and unrated games too", Access{MaxAge: 0, Kinds: map[string]KindLimit{parental.KindGame: {MaxAge: 16, AllowUnrated: true}}},
			"clip-tagged ep-kid film-elsewhere film-g game-e game-pegi16 game-unrated song-clean", "Kid show"},
		{"no limit on music, which is mostly unrated", Access{MaxAge: 7, Kinds: map[string]KindLimit{parental.KindMusic: {MaxAge: NoAgeLimit}}},
			"clip-tagged ep-kid film-elsewhere film-g game-e song-clean song-explicit song-unrated", "Kid show"},
		{"music without the explicit", Access{MaxAge: NoAgeLimit, AllowUnrated: true, Kinds: map[string]KindLimit{parental.KindMusic: {MaxAge: 16, AllowUnrated: true}}},
			"book-12 book-unrated clip-tagged disc-r ep-kid ep-teen ep-teen-rated-ma film-elsewhere film-fsk12 film-g film-pg13 film-r film-unrated game-e game-pegi16 game-unrated photo song-clean song-unrated",
			"Kid show, Teen show"},
		{"books to 12, unrated books blocked, the rest open", Access{MaxAge: NoAgeLimit, AllowUnrated: true, Kinds: map[string]KindLimit{parental.KindBook: {MaxAge: 12}}},
			"book-12 clip-tagged disc-r ep-kid ep-teen ep-teen-rated-ma film-elsewhere film-fsk12 film-g film-pg13 film-r film-unrated game-e game-pegi16 game-unrated photo song-clean song-explicit song-unrated",
			"Kid show, Teen show"},
		{"photos follow the general limit, not film's", Access{MaxAge: 7, AllowUnrated: true, Kinds: map[string]KindLimit{parental.KindFilm: {MaxAge: 7}}},
			"book-unrated clip-tagged ep-kid film-elsewhere film-g game-e game-unrated photo song-clean song-unrated", "Kid show"},
		{"only one library", Access{MaxAge: NoAgeLimit, AllowUnrated: true, Libraries: []int64{other.ID}}, "film-elsewhere", ""},
		{"a hidden genre", Access{MaxAge: NoAgeLimit, AllowUnrated: true, BlockGenres: []string{"Horror", "Drama"}},
			"book-12 book-unrated clip-tagged disc-r ep-kid film-elsewhere film-fsk12 film-g film-pg13 film-unrated game-e game-pegi16 game-unrated photo song-clean song-explicit song-unrated",
			"Kid show"},
		{"only some genres", Access{MaxAge: NoAgeLimit, AllowUnrated: true, OnlyGenres: []string{"Family", "Action"}},
			"ep-kid film-elsewhere film-g film-pg13", "Kid show"},
		{"a hidden tag", Access{MaxAge: NoAgeLimit, AllowUnrated: true, BlockTags: []string{"pranks"}},
			"book-12 book-unrated disc-r ep-kid ep-teen ep-teen-rated-ma film-elsewhere film-fsk12 film-g film-pg13 film-r film-unrated game-e game-pegi16 game-unrated photo song-clean song-explicit song-unrated",
			"Kid show, Teen show"},
		{"part of a tag's name is another tag", Access{MaxAge: NoAgeLimit, AllowUnrated: true, BlockTags: []string{"prank"}},
			"book-12 book-unrated clip-tagged disc-r ep-kid ep-teen ep-teen-rated-ma film-elsewhere film-fsk12 film-g film-pg13 film-r film-unrated game-e game-pegi16 game-unrated photo song-clean song-explicit song-unrated",
			"Kid show, Teen show"},
		{"a collection never allowed", Access{MaxAge: NoAgeLimit, AllowUnrated: true, BlockCollections: []int64{1}},
			"book-12 book-unrated clip-tagged disc-r ep-teen ep-teen-rated-ma film-elsewhere film-fsk12 film-pg13 film-r film-unrated game-e game-pegi16 game-unrated photo song-clean song-explicit song-unrated",
			"Teen show"},
		{"a collection always allowed, whatever the rating", Access{MaxAge: 0, AllowCollections: []int64{2}},
			"clip-tagged ep-kid ep-teen ep-teen-rated-ma film-elsewhere film-g film-r game-e game-pegi16 song-clean", "Kid show, Teen show"},
		{"never wins over always", Access{MaxAge: 0, AllowCollections: []int64{2}, BlockItems: []int64{all["film-r"].ID}, BlockShows: []int64{show.ID}},
			"clip-tagged ep-kid film-elsewhere film-g game-e game-pegi16 song-clean", "Kid show"},
		{"a title always allowed", Access{MaxAge: 0, AllowItems: []int64{all["film-r"].ID}, AllowShows: []int64{show.ID}},
			"clip-tagged ep-kid ep-teen ep-teen-rated-ma film-elsewhere film-g film-r game-e song-clean", "Kid show, Teen show"},
		{"no games at all", Access{MaxAge: NoAgeLimit, AllowUnrated: true, Off: []string{FeatureGames}},
			"book-12 book-unrated clip-tagged disc-r ep-kid ep-teen ep-teen-rated-ma film-elsewhere film-fsk12 film-g film-pg13 film-r film-unrated photo song-clean song-explicit song-unrated",
			"Kid show, Teen show"},
		{"no games, even one that was always allowed", Access{MaxAge: 0, AllowItems: []int64{all["game-pegi16"].ID}, Off: []string{FeatureGames}},
			"clip-tagged ep-kid film-elsewhere film-g song-clean", "Kid show"},
	}
	for _, c := range cases {
		a, err := s.Resolve(ctx, &c.a)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		clause, args := a.ItemClause()
		rows, err := d.QueryContext(ctx, `SELECT i.title FROM items i WHERE `+clause, args...)
		if err != nil {
			t.Fatalf("%s: %v\n%s", c.name, err, clause)
		}
		inSQL := map[string]bool{}
		for rows.Next() {
			var title string
			if err := rows.Scan(&title); err != nil {
				t.Fatal(err)
			}
			inSQL[title] = true
		}
		rows.Close()
		var got []string
		for name, it := range all {
			var sh *Show
			if it.ShowID != nil {
				if sh, err = s.ShowFacts(ctx, *it.ShowID); err != nil {
					t.Fatal(err)
				}
			}
			inGo := a.AllowsItem(it, sh)
			if inGo != inSQL[name] {
				t.Errorf("%s: SQL says %v and Go says %v about %s", c.name, inSQL[name], inGo, name)
			}
			if inGo {
				got = append(got, name)
			}
		}
		sort.Strings(got)
		if strings.Join(got, " ") != c.want {
			t.Errorf("%s:\n got  %s\n want %s", c.name, strings.Join(got, " "), c.want)
		}

		clause, args = a.ShowClause()
		rows, err = d.QueryContext(ctx, `SELECT s.title FROM shows s WHERE `+clause+` ORDER BY s.title`, args...)
		if err != nil {
			t.Fatalf("%s: %v\n%s", c.name, err, clause)
		}
		var shows []string
		for rows.Next() {
			var title string
			if err := rows.Scan(&title); err != nil {
				t.Fatal(err)
			}
			shows = append(shows, title)
		}
		rows.Close()
		if strings.Join(shows, ", ") != c.wantShows {
			t.Errorf("%s: shows %q, want %q", c.name, strings.Join(shows, ", "), c.wantShows)
		}
		for _, sh := range []*Show{show, kidShow} {
			full, err := s.ShowFacts(ctx, sh.ID)
			if err != nil {
				t.Fatal(err)
			}
			if a.AllowsShow(full) != strings.Contains(c.wantShows, sh.Title) {
				t.Errorf("%s: Go says %v about %s", c.name, a.AllowsShow(full), sh.Title)
			}
		}
	}
}

// Limits by collection or tag that nobody looked up allow no single title: a
// caller that forgets Resolve hides things, it doesn't show them.
func TestUnresolvedLimitsAllowNothing(t *testing.T) {
	it := &Item{ID: 5, Kind: "video", ContentRating: "G"}
	for _, a := range []*Access{
		{MaxAge: NoAgeLimit, AllowUnrated: true, BlockCollections: []int64{1}},
		{MaxAge: NoAgeLimit, AllowUnrated: true, AllowCollections: []int64{1}},
		{MaxAge: NoAgeLimit, AllowUnrated: true, BlockTags: []string{"x"}},
	} {
		if a.Resolved() || a.AllowsItem(it, nil) || a.AllowsShow(&Show{ID: 1, ContentRating: "TV-Y"}) {
			t.Errorf("%+v let a title through without being looked up", a)
		}
	}
	plain := &Access{MaxAge: 7}
	if !plain.Resolved() || !plain.AllowsItem(it, nil) {
		t.Error("limits with nothing to look up need no looking up")
	}
}

// Every rating a system in Settings offers means the age Settings says it does.
func TestRatingSystemsMatchTheAgesTitlesGet(t *testing.T) {
	for _, sys := range parental.Systems {
		for _, r := range sys.Ratings {
			got := AgeFor(r.Label)
			if got == nil || *got != r.Age {
				t.Errorf("%s %q: a title rated this gets age %v, and Settings says %d", sys.Name, r.Label, got, r.Age)
			}
		}
	}
	// Other countries' certificates come to an age too.
	for in, want := range map[string]int{"NL:9": 9, "Kijkwijzer 14": 14, "de/FSK 6": 6, "GB:12A": 12, "AL": 0, "PEGI 18": 18, "ESRB T": 13, "USK 12": 12, "Explicit": 17} {
		if got := AgeFor(in); got == nil || *got != want {
			t.Errorf("AgeFor(%q) = %v, want %d", in, got, want)
		}
	}
}

func TestWhatIsLimitedAndWhatFeaturesAreOn(t *testing.T) {
	adult := DefaultAccess("adult")
	if adult.Limited() {
		t.Error("an adult nobody limited is limited")
	}
	for _, f := range Features {
		if !adult.Can(f) || !(*Access)(nil).Can(f) {
			t.Errorf("an adult nobody limited can't use %s", f)
		}
	}
	kids := DefaultAccess("kids")
	kids.ForKind("kids")
	if !kids.Limited() || kids.Can(FeatureEditProfile) || !kids.Can(FeatureGames) || !kids.Can(FeatureRemote) || !kids.Can(FeatureComments) {
		t.Errorf("a new Kids profile: %+v", kids)
	}
	given := DefaultAccess("guest")
	given.On = []string{FeatureEditProfile}
	given.ForKind("guest")
	if !given.Can(FeatureEditProfile) {
		t.Errorf("a Guest given its own name and picture: %+v", given)
	}
	adult.ForKind("adult")
	if adult.Limited() {
		t.Error("ForKind limited an adult")
	}
	for _, change := range []func(a *Access){
		func(a *Access) { a.Kinds = map[string]KindLimit{"game": {MaxAge: 12}} },
		func(a *Access) { a.Off = []string{FeatureDownloads} },
		func(a *Access) { a.Time = &parental.TimeRules{WeekendMinutes: 60} },
		func(a *Access) { a.OnlyGenres = []string{"Family"} },
		func(a *Access) { a.BlockTags = []string{"x"} },
		func(a *Access) { a.BlockCollections = []int64{1} },
	} {
		a := DefaultAccess("adult")
		change(&a)
		if !a.Limited() {
			t.Errorf("%+v isn't limited", a)
		}
	}
	// A kind with no limit of its own, on an account with none, limits nothing.
	a := DefaultAccess("adult")
	a.Kinds = map[string]KindLimit{"game": {MaxAge: NoAgeLimit}}
	a.Time = &parental.TimeRules{}
	if a.Limited() {
		t.Errorf("%+v is limited", a)
	}
}

func TestMediaKind(t *testing.T) {
	for _, c := range []struct {
		kind    string
		episode bool
		want    string
	}{{"video", false, "film"}, {"video", true, "tv"}, {"dvd-iso", false, "film"}, {"bluray-folder", false, "film"}, {"game", false, "game"},
		{"audio", false, "music"}, {"cd-image", false, "music"}, {"radio", false, "music"}, {"book", false, "book"}, {"photo", false, ""}} {
		if got := MediaKind(c.kind, c.episode); got != c.want {
			t.Errorf("MediaKind(%q, %v) = %q, want %q", c.kind, c.episode, got, c.want)
		}
	}
}
