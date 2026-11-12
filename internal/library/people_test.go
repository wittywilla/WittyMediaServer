// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

type peopleFixture struct {
	st         *Store
	films      int64 // library
	tv         int64 // library
	film, kids *Item
	show       *Show
	ep1, ep2   *Item
}

func newPeopleFixture(t *testing.T) *peopleFixture {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	f := &peopleFixture{st: &Store{DB: d}}
	films, err := f.st.CreateLibrary(ctx, "Films", "movies", []string{"/m"})
	if err != nil {
		t.Fatal(err)
	}
	tv, err := f.st.CreateLibrary(ctx, "TV", "shows", []string{"/t"})
	if err != nil {
		t.Fatal(err)
	}
	f.films, f.tv = films.ID, tv.ID
	year := 1999
	f.film = &Item{LibraryID: films.ID, Kind: "video", Path: "/m/heist.mkv", Title: "The Heist", SortTitle: "Heist", Year: &year, ContentRating: "R", Genres: []string{"Crime"}}
	f.kids = &Item{LibraryID: films.ID, Kind: "video", Path: "/m/robot.mkv", Title: "Robot Friend", SortTitle: "Robot Friend", ContentRating: "G"}
	for _, it := range []*Item{f.film, f.kids} {
		if err := f.st.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	f.show = &Show{LibraryID: tv.ID, Path: "/t/harbour", Title: "Harbour", SortTitle: "Harbour", ContentRating: "TV-PG"}
	if err := f.st.UpsertShow(ctx, f.show); err != nil {
		t.Fatal(err)
	}
	one, two, s1 := 1, 2, 1
	f.ep1 = &Item{LibraryID: tv.ID, Kind: "video", Path: "/t/harbour/s01e01.mkv", Title: "Pilot", SortTitle: "Pilot", ShowID: &f.show.ID, Season: &s1, Episode: &one}
	f.ep2 = &Item{LibraryID: tv.ID, Kind: "video", Path: "/t/harbour/s01e02.mkv", Title: "Storm", SortTitle: "Storm", ShowID: &f.show.ID, Season: &s1, Episode: &two}
	for _, it := range []*Item{f.ep1, f.ep2} {
		if err := f.st.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func creditNames(list []Credit) string {
	var out []string
	for _, c := range list {
		s := c.Name
		if c.Character != "" {
			s += " as " + c.Character
		}
		if c.Job != "" {
			s += " (" + c.Job + ")"
		}
		out = append(out, s)
	}
	return strings.Join(out, "; ")
}

func TestPersonKeyAndSortName(t *testing.T) {
	for in, want := range map[string]string{"Zoë  Saldaña": "zoe saldana", "J.K. Simmons": "j k simmons", "  ": "", "Bong Joon-ho": "bong joon ho"} {
		if got := PersonKey(in); got != want {
			t.Errorf("PersonKey(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"Tom Hanks": "Hanks Tom", "Zendaya": "Zendaya", "Robert Downey Jr.": "Downey Jr. Robert", "Doe, Jane": "Doe, Jane"} {
		if got := PersonSortName(in); got != want {
			t.Errorf("PersonSortName(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[[2]string]string{{"Art", ""}: "art", {"", "Set Decoration"}: "art", {"Costume & Make-Up", ""}: "costume",
		{"", "Director of Photography"}: "camera", {"Visual Effects", ""}: "effects", {"", "Original Music Composer"}: "sound", {"", "Gaffer"}: "lighting",
		{"", "Screenplay"}: "writing", {"", "Director"}: "directing", {"", "Executive Producer"}: "production", {"", "Caterer"}: "crew"} {
		if got := DepartmentOf(in[0], in[1]); got != want {
			t.Errorf("DepartmentOf(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

// One person across titles and sources: by an external id first, then by name, and two people
// with the same name but different ids stay two.
func TestCreditsKeepOnePersonPerPerson(t *testing.T) {
	f := newPeopleFixture(t)
	ctx := context.Background()
	st := f.st
	if err := st.SetCredits(ctx, TargetItem, f.film.ID, CreditsNFO, []CreditIn{
		{Name: "Ana Lima", Role: RoleCast, Character: "Vera", Order: 0, Photo: "/m/.actors/Ana_Lima.jpg"},
		{Name: "Sam Reed", Role: RoleCast, Character: "Max", Order: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetCredits(ctx, TargetItem, f.kids.ID, CreditsTMDB, []CreditIn{
		{Name: "ANA LIMA", IDs: PersonIDs{TMDB: "77"}, Role: RoleCast, Character: "Voice of Bolt", Photo: "https://image.tmdb.org/t/p/w185/ana.jpg"},
		{Name: "Sam Reed", IDs: PersonIDs{TMDB: "88"}, Role: RoleCrew, Department: "Directing", Job: "Director"},
	}); err != nil {
		t.Fatal(err)
	}
	// A different Sam Reed: TMDB says another number.
	if err := st.SetCredits(ctx, TargetShow, f.show.ID, CreditsTMDB, []CreditIn{
		{Name: "Sam Reed", IDs: PersonIDs{TMDB: "99"}, Role: RoleCast, Character: "Harbour master"},
		{Name: "Ana Lima", IDs: PersonIDs{TMDB: "77", IMDB: "nm1"}, Role: RoleCast, Character: "Pilot"},
	}); err != nil {
		t.Fatal(err)
	}
	var people int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM people`).Scan(&people); err != nil {
		t.Fatal(err)
	}
	if people != 3 {
		t.Fatalf("%d people, want 3 (Ana Lima once, Sam Reed twice)", people)
	}
	var ana *Person
	var anaID int64
	_ = st.DB.QueryRowContext(ctx, `SELECT id FROM people WHERE key = 'ana lima'`).Scan(&anaID)
	ana, err := st.Person(ctx, anaID)
	if err != nil {
		t.Fatal(err)
	}
	if ana.IDs.TMDB != "77" || ana.IDs.IMDB != "nm1" || ana.Photo != "/m/.actors/Ana_Lima.jpg" {
		t.Fatalf("Ana Lima: %+v (her own photo beside the media must win over a web address)", ana)
	}
}

// Each part comes from the best source that has it; a source read again replaces only its own.
func TestCreditsAreLayeredBySource(t *testing.T) {
	f := newPeopleFixture(t)
	ctx := context.Background()
	st := f.st
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(st.SetCredits(ctx, TargetItem, f.film.ID, CreditsTMDB, []CreditIn{
		{Name: "Online Star", Role: RoleCast, Character: "Hero", Order: 0},
		{Name: "Online Director", Role: RoleCrew, Department: "Directing", Job: "Director"},
		{Name: "Dee Signer", Role: RoleCrew, Department: "Art", Job: "Set Decoration"},
		{Name: "Pat Plans", Role: RoleCrew, Department: "Art", Job: "Production Design"},
		{Name: "Wren Words", Role: RoleCrew, Department: "Writing", Job: "Story"},
		{Name: "Wren Words", Role: RoleCrew, Department: "Writing", Job: "Screenplay"},
	}))
	must(st.SetCredits(ctx, TargetItem, f.film.ID, CreditsNFO, []CreditIn{
		{Name: "Local Lead", Role: RoleCast, Character: "Hero", Order: 1},
		{Name: "Local Lead", Role: RoleCast, Character: "Hero's twin", Order: 1},
		{Name: "Second Billing", Role: RoleCast, Character: "Sidekick", Order: 0},
		{Name: "Local Director", Role: RoleCrew, Job: "Director"},
	}))
	must(st.SetCredits(ctx, TargetItem, f.film.ID, CreditsTags, []CreditIn{{Name: "Tag Composer", Role: RoleCrew, Job: "Composer"}}))
	c, err := st.ItemCredits(ctx, f.film)
	must(err)
	if got := creditNames(c.Cast); got != "Second Billing as Sidekick; Local Lead as Hero / Hero's twin" {
		t.Errorf("cast from the NFO, in billing order, one person once: %s", got)
	}
	var depts []string
	for _, g := range c.Crew {
		depts = append(depts, g.Department+": "+creditNames(g.People))
	}
	want := "directing: Local Director (Director)|writing: Wren Words (Screenplay, Story)|sound: Tag Composer (Composer)|art: Pat Plans (Production Design); Dee Signer (Set Decoration)"
	if got := strings.Join(depts, "|"); got != want {
		t.Errorf("crew:\n got %s\nwant %s", got, want)
	}
	// The online source read again with nobody: its parts go, the NFO's stay.
	must(st.SetCredits(ctx, TargetItem, f.film.ID, CreditsTMDB, nil))
	c, _ = st.ItemCredits(ctx, f.film)
	if len(c.Crew) != 2 || len(c.Cast) != 2 {
		t.Errorf("after TMDB named nobody: %d departments, %d cast", len(c.Crew), len(c.Cast))
	}
	must(st.PrunePeople(ctx))
	var left int
	_ = st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM people WHERE name = 'Online Star'`).Scan(&left)
	if left != 0 {
		t.Error("a person no title names any more is kept")
	}
	// A title that goes takes its credits with it.
	if _, err := st.DB.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, f.film.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM credits WHERE target = 'item' AND target_id = ?`, f.film.ID).Scan(&n)
	if n != 0 {
		t.Errorf("%d credits outlived their title", n)
	}
}

// An episode has its show's regulars, its own guests and crew; a season's page has the
// season's guests, most episodes first; a regular is never a guest.
func TestEpisodeAndSeasonCredits(t *testing.T) {
	f := newPeopleFixture(t)
	ctx := context.Background()
	st := f.st
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(st.SetCredits(ctx, TargetShow, f.show.ID, CreditsTMDB, []CreditIn{
		{Name: "Regular One", Role: RoleCast, Character: "Captain", Order: 0},
		{Name: "Show Creator", Role: RoleCrew, Department: "Writing", Job: "Creator"},
		{Name: "Show Director", Role: RoleCrew, Department: "Directing", Job: "Director"},
	}))
	must(st.SetCredits(ctx, TargetItem, f.ep1.ID, CreditsTMDB, []CreditIn{
		{Name: "Guest A", Role: RoleGuest, Character: "Diver"},
		{Name: "Regular One", Role: RoleGuest, Character: "Captain"},
		{Name: "Ep Director", Role: RoleCrew, Department: "Directing", Job: "Director"},
	}))
	must(st.SetCredits(ctx, TargetItem, f.ep2.ID, CreditsTMDB, []CreditIn{
		{Name: "Guest B", Role: RoleGuest, Character: "Sailor"},
		{Name: "Guest A", Role: RoleGuest, Character: "Diver"},
	}))
	c, err := st.ItemCredits(ctx, f.ep1)
	must(err)
	if creditNames(c.Cast) != "Regular One as Captain" || creditNames(c.Guests) != "Guest A as Diver" {
		t.Errorf("episode: cast %q guests %q", creditNames(c.Cast), creditNames(c.Guests))
	}
	if len(c.Crew) != 2 || creditNames(c.Crew[0].People) != "Ep Director (Director)" || creditNames(c.Crew[1].People) != "Show Creator (Creator)" {
		t.Errorf("episode crew: its own director, the show's writers: %+v", c.Crew)
	}
	c, err = st.ShowCredits(ctx, f.show.ID, 1, nil)
	must(err)
	if got := creditNames(c.Guests); got != "Guest A as Diver; Guest B as Sailor" || c.Guests[0].Episodes != 2 {
		t.Errorf("season guests: %s (%+v)", got, c.Guests)
	}
	c, _ = st.ShowCredits(ctx, f.show.ID, -1, nil)
	if len(c.Guests) != 0 {
		t.Error("the show's own page lists every guest of every season")
	}
	// A guest of an episode this account may not see isn't named on the season.
	c, _ = st.ShowCredits(ctx, f.show.ID, 1, &Access{MaxAge: NoAgeLimit, BlockItems: []int64{f.ep2.ID}})
	if got := creditNames(c.Guests); got != "Guest A as Diver" {
		t.Errorf("season guests with episode 2 hidden: %s", got)
	}
}

// A filmography names only what the account may see, and a person credited only on hidden
// titles isn't there at all, nor found by a search.
func TestFilmographyHonoursLimits(t *testing.T) {
	f := newPeopleFixture(t)
	ctx := context.Background()
	st := f.st
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := st.BackfillAges(ctx); err != nil {
		t.Fatal(err)
	}
	must(st.SetCredits(ctx, TargetItem, f.film.ID, CreditsTMDB, []CreditIn{
		{Name: "Ana Lima", Role: RoleCast, Character: "Vera"},
		{Name: "Only Grown Ups", Role: RoleCast, Character: "Boss"},
		{Name: "Ana Lima", Role: RoleCrew, Department: "Directing", Job: "Director"},
	}))
	must(st.SetCredits(ctx, TargetItem, f.kids.ID, CreditsTMDB, []CreditIn{{Name: "Ana Lima", Role: RoleCast, Character: "Bolt"}}))
	must(st.SetCredits(ctx, TargetItem, f.ep1.ID, CreditsTMDB, []CreditIn{{Name: "Ana Lima", Role: RoleGuest, Character: "Diver"}}))
	must(st.SetCredits(ctx, TargetItem, f.ep2.ID, CreditsTMDB, []CreditIn{{Name: "Ana Lima", Role: RoleGuest, Character: "Diver"}}))
	var ana, grown int64
	_ = st.DB.QueryRowContext(ctx, `SELECT id FROM people WHERE key = 'ana lima'`).Scan(&ana)
	_ = st.DB.QueryRowContext(ctx, `SELECT id FROM people WHERE key = 'only grown ups'`).Scan(&grown)

	all, err := st.Filmography(ctx, ana, nil)
	must(err)
	if len(all.Groups) != 2 || all.Groups[0].Key != FilmActing || all.Groups[1].Key != "directing" {
		t.Fatalf("groups: %+v", all.Groups)
	}
	acting := all.Groups[0].Titles
	if len(acting) != 3 {
		t.Fatalf("acting: %+v", acting)
	}
	var harbour *FilmTitle
	for i := range acting {
		if acting[i].Target == "show" {
			harbour = &acting[i]
		}
	}
	if harbour == nil || harbour.Episodes != 2 || !harbour.Guest || harbour.Character != "Diver" {
		t.Fatalf("a guest part is its show's, with the episodes counted: %+v", harbour)
	}

	kids := &Access{MaxAge: 7}
	kids.ForKind("kids")
	mine, err := st.Filmography(ctx, ana, kids)
	must(err)
	for _, g := range mine.Groups {
		for _, ti := range g.Titles {
			if ti.Title == "The Heist" {
				t.Errorf("a kids profile's filmography names an R film: %+v", ti)
			}
		}
	}
	if !st.PersonVisible(ctx, ana, kids) {
		t.Error("Ana is in a film the kids profile may see")
	}
	if st.PersonVisible(ctx, grown, kids) {
		t.Error("a person only in a hidden film exists for a kids profile")
	}
	// A limit that carries arguments of its own (a genre) must not shift the person's.
	noCrime := &Access{MaxAge: NoAgeLimit, BlockGenres: []string{"Crime"}}
	if st.PersonVisible(ctx, grown, noCrime) {
		t.Error("a person only in a blocked genre's film exists for that profile")
	}
	if fc, _ := st.Filmography(ctx, ana, noCrime); len(fc.Groups) != 1 || len(fc.Groups[0].Titles) != 2 {
		t.Errorf("filmography without crime films: %+v", fc.Groups)
	}
	found, err := st.SearchPeople(ctx, "grown", 10, kids)
	must(err)
	if len(found) != 0 {
		t.Errorf("search found someone only in hidden titles: %+v", found)
	}
	found, _ = st.SearchPeople(ctx, "lima", 10, nil)
	if len(found) != 1 || found[0].Titles != 3 {
		t.Errorf("search: %+v", found)
	}
	// The library, filtered by her: both films and the show.
	res, err := st.Browse(ctx, BrowseFilter{Person: ana, Library: f.films}, 0, nil)
	must(err)
	if res.Total != 2 {
		t.Errorf("films with her: %d", res.Total)
	}
	res, _ = st.Browse(ctx, BrowseFilter{Person: ana, Library: f.tv}, 0, nil)
	if res.Total != 1 {
		t.Errorf("shows with her in an episode: %d", res.Total)
	}
	res, _ = st.Browse(ctx, BrowseFilter{Person: ana, Library: f.films}, 0, kids)
	if res.Total != 1 {
		t.Errorf("films with her a kids profile may see: %d", res.Total)
	}
}

func TestPersonDetails(t *testing.T) {
	f := newPeopleFixture(t)
	ctx := context.Background()
	st := f.st
	if err := st.SetCredits(ctx, TargetItem, f.film.ID, CreditsTMDB, []CreditIn{{Name: "Ana Lima", IDs: PersonIDs{TMDB: "77"}, Role: RoleCast}}); err != nil {
		t.Fatal(err)
	}
	var id int64
	_ = st.DB.QueryRowContext(ctx, `SELECT id FROM people`).Scan(&id)
	need, _ := st.PeopleNeedingDetails(ctx, 10)
	if len(need) != 1 {
		t.Fatalf("needing details: %d", len(need))
	}
	if err := st.SetPersonDetails(ctx, id, PersonDetails{Birthday: "1970-02-03", Deathday: "not a date", Biography: "Born by the sea.\nActs.", Photo: "https://image.tmdb.org/t/p/h632/a.jpg", KnownFor: "acting"}); err != nil {
		t.Fatal(err)
	}
	p, _ := st.Person(ctx, id)
	if p.Birthday != "1970-02-03" || p.Deathday != "" || p.Biography != "Born by the sea.\nActs." || !p.HasPhoto || p.KnownFor != "acting" {
		t.Fatalf("%+v", p)
	}
	if need, _ := st.PeopleNeedingDetails(ctx, 10); len(need) != 0 {
		t.Error("looked up just now, and asked again")
	}
}
