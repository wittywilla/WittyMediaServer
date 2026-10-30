package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

func eps(group int64, n int, dur int64) []Entry {
	var out []Entry
	for i := 1; i <= n; i++ {
		out = append(out, Entry{ID: group*1000 + int64(i), Group: group, ShowID: group, Dur: dur, Title: fmt.Sprintf("Show %d", group), Sub: fmt.Sprintf("E%d", i)})
	}
	return out
}

func fixed(entries []Entry) PoolLoader {
	return func(Rules, int64) ([]Entry, error) { return entries, nil }
}

func noOverlap(t *testing.T, progs []Programme) {
	t.Helper()
	for i := 1; i < len(progs); i++ {
		if progs[i].Start < progs[i-1].End {
			t.Fatalf("programme %d starts at %d before the previous ends at %d", i, progs[i].Start, progs[i-1].End)
		}
	}
	for i, p := range progs {
		if p.End <= p.Start {
			t.Fatalf("programme %d has no length: %+v", i, p)
		}
	}
}

func TestSameInputSameDay(t *testing.T) {
	pool := append(eps(1, 40, 1320), eps(2, 40, 2640)...)
	for _, o := range []Order{{Mode: "rotate"}, {Mode: "sequential"}, {Mode: "shuffle"}, {Mode: "shuffle", KeepEpisodeOrder: true}, {Mode: "aired"}} {
		def := &Definition{Mode: "simple", Order: o, AlignMin: 30}
		a, sa, err := GenerateDay(def, 7, 20000, time.UTC, State{}, fixed(pool), nil)
		if err != nil {
			t.Fatal(err)
		}
		b, _, _ := GenerateDay(def, 7, 20000, time.UTC, State{}, fixed(pool), nil)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("%v: same input gave a different day", o)
		}
		c, _, _ := GenerateDay(def, 7, 20001, time.UTC, sa, fixed(pool), nil)
		if len(c) == 0 || c[0].Start < a[len(a)-1].End {
			t.Fatalf("%v: next day starts before the last programme ended", o)
		}
		noOverlap(t, a)
	}
}

func TestRotateKeepsEpisodeOrder(t *testing.T) {
	pool := append(eps(1, 5, 600), eps(2, 5, 600)...)
	def := &Definition{Mode: "simple", Order: Order{Mode: "rotate"}}
	progs, _, err := GenerateDay(def, 1, 20000, time.UTC, State{}, fixed(pool), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"E1", "E1", "E2", "E2", "E3", "E3"}
	for i, w := range want {
		if progs[i].Sub != w {
			t.Fatalf("slot %d = %s, want %s", i, progs[i].Sub, w)
		}
	}
	if progs[0].ShowID == progs[1].ShowID {
		t.Fatal("shows should take turns")
	}
}

func TestSequentialCarriesOverDays(t *testing.T) {
	pool := eps(1, 500, 3600)
	def := &Definition{Mode: "simple", Order: Order{Mode: "sequential"}}
	a, st, _ := GenerateDay(def, 1, 20000, time.UTC, State{}, fixed(pool), nil)
	b, _, _ := GenerateDay(def, 1, 20001, time.UTC, st, fixed(pool), nil)
	if len(a) != 24 || a[23].Sub != "E24" || b[0].Sub != "E25" {
		t.Fatalf("days do not follow on: %d, last %s, then %s", len(a), a[len(a)-1].Sub, b[0].Sub)
	}
}

func TestShuffleNoRepeatsInACycle(t *testing.T) {
	pool := eps(1, 48, 1800)
	def := &Definition{Mode: "simple", Order: Order{Mode: "shuffle"}}
	progs, _, _ := GenerateDay(def, 3, 20000, time.UTC, State{}, fixed(pool), nil)
	seen := map[int64]bool{}
	for _, p := range progs[:48] {
		if seen[p.ItemID] {
			t.Fatal("repeated before the pool was used up")
		}
		seen[p.ItemID] = true
	}
}

func TestAlignmentAndSlate(t *testing.T) {
	def := &Definition{Mode: "simple", Order: Order{Mode: "rotate"}, AlignMin: 30}
	progs, _, _ := GenerateDay(def, 1, 20000, time.UTC, State{}, fixed(eps(1, 60, 1320)), nil)
	for _, p := range progs {
		if p.Kind == KindShow && p.Start%1800 != 0 {
			t.Fatalf("programme off the grid: %d", p.Start%1800)
		}
	}
	if progs[1].Kind != KindSlate || progs[1].End-progs[1].Start != 480 {
		t.Fatalf("want an 8 minute slate, got %+v", progs[1])
	}
}

func TestFillerFillsGaps(t *testing.T) {
	calls := 0
	load := func(r Rules, minSec int64) ([]Entry, error) {
		calls++
		if len(r.Items) > 0 {
			return []Entry{{ID: 9, Dur: 120, Title: "Bumper"}}, nil
		}
		return eps(1, 60, 1320), nil
	}
	def := &Definition{Mode: "simple", AlignMin: 30, Filler: Filler{Rules: &Rules{Items: []int64{9}}}}
	progs, _, err := GenerateDay(def, 1, 20000, time.UTC, State{}, load, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 22-minute show, 8-minute gap: four 2-minute bumpers.
	for i := 1; i <= 4; i++ {
		if progs[i].Kind != KindFiller {
			t.Fatalf("slot %d is %s", i, progs[i].Kind)
		}
	}
	if progs[5].Kind != KindShow || progs[5].Start%1800 != 0 {
		t.Fatalf("next show not on the grid: %+v", progs[5])
	}
}

func TestBlocksAndFit(t *testing.T) {
	short := Rules{Shows: []int64{1}}
	long := Rules{Shows: []int64{2}}
	load := func(r Rules, _ int64) ([]Entry, error) {
		switch {
		case len(r.Shows) > 0 && r.Shows[0] == 1:
			return eps(1, 100, 1500), nil
		case len(r.Shows) > 0 && r.Shows[0] == 2:
			return eps(2, 100, 2700), nil
		}
		return eps(3, 100, 1200), nil
	}
	day := 20000 // a Friday
	def := &Definition{Mode: "timetable", Blocks: []Block{
		{ID: "a", Days: []int{5}, Start: "18:00", End: "18:30", Rules: &short},
		{ID: "b", Days: []int{5}, Start: "18:30", End: "19:00", Rules: &long, Fit: "cut"},
		{ID: "c", Days: []int{5}, Start: "19:00", End: "19:30", Rules: &long, Fit: "overrun"},
	}}
	progs, _, err := GenerateDay(def, 1, day, time.UTC, State{}, load, nil)
	if err != nil {
		t.Fatal(err)
	}
	noOverlap(t, progs)
	at := func(hh, mm int) int64 { return DayStart(day, time.UTC).Add(time.Duration(hh)*time.Hour + time.Duration(mm)*time.Minute).Unix() }
	var a, b, c, after *Programme
	for i := range progs {
		p := &progs[i]
		switch {
		case p.Start == at(18, 0) && p.Kind == KindShow:
			a = p
		case p.Start == at(18, 30) && p.Kind == KindShow:
			b = p
		case p.Start == at(19, 0) && p.Kind == KindShow:
			c = p
		case p.Start == at(19, 45):
			after = p
		}
	}
	if a == nil || a.ShowID != 1 {
		t.Fatalf("18:00 should play show 1: %+v", a)
	}
	if b == nil || b.ShowID != 2 || b.End != at(19, 0) {
		t.Fatalf("a cut block ends on time: %+v", b)
	}
	if c == nil || c.End != at(19, 45) {
		t.Fatalf("an overrun block finishes its programme: %+v", c)
	}
	if after == nil {
		t.Fatal("the channel's own rules should take over after the overrun")
	}
}

func TestProgrammeLongerThanBlockDoesNotStick(t *testing.T) {
	r := Rules{Shows: []int64{1}}
	def := &Definition{Mode: "rules", Rules: r, Blocks: []Block{{ID: "a", Start: "10:00", End: "10:30", Rules: &r}}}
	progs, _, err := GenerateDay(def, 1, 20000, time.UTC, State{}, fixed(eps(1, 50, 5400)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(progs) == 0 {
		t.Fatal("nothing scheduled")
	}
	noOverlap(t, progs)
}

func TestBlockPastMidnight(t *testing.T) {
	r := Rules{Shows: []int64{1}}
	def := &Definition{Mode: "rules", Blocks: []Block{{ID: "late", Start: "23:00", End: "02:00", Rules: &r}}}
	load := func(rr Rules, _ int64) ([]Entry, error) {
		if len(rr.Shows) > 0 {
			return eps(1, 100, 1800), nil
		}
		return eps(2, 100, 1800), nil
	}
	a, st, _ := GenerateDay(def, 1, 20000, time.UTC, State{}, load, nil)
	b, _, _ := GenerateDay(def, 1, 20001, time.UTC, st, load, nil)
	noOverlap(t, a)
	noOverlap(t, b)
	if b[0].ShowID != 1 || b[0].Start != DayStart(20001, time.UTC).Unix() {
		t.Fatalf("the block should carry on after midnight: %+v", b[0])
	}
	last := b[0]
	for _, p := range b {
		if p.Start == DayStart(20001, time.UTC).Add(2*time.Hour).Unix() {
			last = p
		}
	}
	if last.ShowID != 2 {
		t.Fatalf("after 02:00 the channel's rules play: %+v", last)
	}
}

func TestDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no time zone data")
	}
	// The day clocks go forward in 2026 has 23 hours.
	day := DayNumber(time.Date(2026, 3, 8, 12, 0, 0, 0, loc), loc)
	def := &Definition{Mode: "rules", Blocks: []Block{{ID: "x", Start: "20:00", End: "21:00", Rules: &Rules{Shows: []int64{1}}}}}
	progs, _, _ := GenerateDay(def, 1, day, loc, State{}, fixed(eps(1, 100, 1800)), nil)
	noOverlap(t, progs)
	want := time.Date(2026, 3, 8, 20, 0, 0, 0, loc).Unix()
	found := false
	for _, p := range progs {
		found = found || p.Start == want
	}
	if !found {
		t.Fatal("a block should start at its wall-clock time on a DST day")
	}
}

func TestEmptyPoolIsOffAir(t *testing.T) {
	def := &Definition{Mode: "simple"}
	progs, _, err := GenerateDay(def, 1, 20000, time.UTC, State{}, fixed(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(progs) != 1 || progs[0].Kind != KindSlate {
		t.Fatalf("want one slate, got %+v", progs)
	}
}

func TestValidate(t *testing.T) {
	bad := []Definition{
		{Mode: "wat"},
		{Order: Order{Mode: "wat"}},
		{AlignMin: 7},
		{Blocks: []Block{{Start: "25:00", End: "26:00"}}},
		{Blocks: []Block{{Start: "10:00", End: "11:00", Days: []int{9}}}},
		{Rules: Rules{Kinds: []string{"wat"}}},
		{Rules: Rules{YearFrom: 2000, YearTo: 1990}},
		{Rules: Rules{Watched: "wat"}},
		{Filler: Filler{Between: 50}},
	}
	for i, d := range bad {
		if d.Validate() == nil {
			t.Fatalf("definition %d should be refused", i)
		}
	}
	good := Definition{Mode: "rules", Blocks: []Block{{Start: "23:00", End: "02:00", Days: []int{1}}}, AlignMin: 30}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// With the database
// ---------------------------------------------------------------------------

func testStore(t *testing.T) (*Store, *library.Store) {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ls := &library.Store{DB: d}
	ctx := context.Background()
	lib, err := ls.CreateLibrary(ctx, "TV", "shows", []string{"/tv"})
	if err != nil {
		t.Fatal(err)
	}
	mk := func(show int64, n int, aired, rating string, dur float64) {
		for i := 1; i <= n; i++ {
			s, e := 1, i
			_, err := d.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, runtime_sec, show_id, season, episode, aired, added_at, updated_at, min_age, content_rating)
				VALUES(?, 'video', ?, ?, ?, ?, ?, ?, ?, ?, 1, 1, ?, ?)`, lib.ID, fmt.Sprintf("/tv/%d/%d.mkv", show, i), fmt.Sprintf("Ep %d", i), fmt.Sprintf("Ep %03d", i), dur, show, s, e, aired, library.AgeFor(rating), rating)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	show := func(title, network, genres, days, at string) int64 {
		sh := &library.Show{LibraryID: lib.ID, Path: "/tv/" + title, Title: title, Network: network, Genres: []string{genres}}
		if err := ls.UpsertShow(ctx, sh); err != nil {
			t.Fatal(err)
		}
		if _, err := d.ExecContext(ctx, `UPDATE shows SET air_days = ?, air_time = ? WHERE id = ?`, days, at, sh.ID); err != nil {
			t.Fatal(err)
		}
		return sh.ID
	}
	mk(show("Ninety Comedy", "Fairview", "Comedy", "Thursday", "20:00"), 12, "1994-05-01", "TV-PG", 1320)
	mk(show("Ninety Drama", "fairview", "Drama", "Thursday", "21:00"), 12, "1996-05-01", "TV-14", 2640)
	mk(show("Noughties Kids", "Sunny", "Children", "", ""), 20, "2004-05-01", "TV-Y", 660)
	// A film and a scrambled disc: the disc must never reach a channel.
	if _, err := d.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, runtime_sec, year, studio, added_at, updated_at)
		VALUES(?, 'video', '/m/film.mkv', 'Film', 'Film', 5400, 1999, 'Fairview', 1, 1)`, lib.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, runtime_sec, year, disc_json, added_at, updated_at)
		VALUES(?, 'dvd-iso', '/m/locked.iso', 'Locked', 'Locked', 5400, 1999, '{"scrambled":true}', 1, 1)`, lib.ID); err != nil {
		t.Fatal(err)
	}
	return &Store{DB: d, Loc: time.UTC}, ls
}

func TestRulesPick(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	age := 7
	cases := []struct {
		name string
		r    Rules
		want int
	}{
		{"everything playable", Rules{}, 45},
		{"network, any spelling, with its film", Rules{Networks: []string{"FAIRVIEW"}}, 25},
		{"decade", Rules{Decades: []int{2000}}, 20},
		{"years by air date", Rules{YearFrom: 1995, YearTo: 1997}, 12},
		{"genre", Rules{Genres: []string{"Comedy"}}, 12},
		{"excluded genre", Rules{ExcludeGenres: []string{"Comedy"}}, 33},
		{"rating", Rules{Ratings: []string{"TV-14"}}, 12},
		{"age limit", Rules{MaxAge: &age}, 20},
		{"kinds", Rules{Kinds: []string{"movies"}}, 1},
		{"length", Rules{MinMinutes: 80}, 1},
		{"search", Rules{Search: "Drama"}, 12},
		{"season range", Rules{SeasonFrom: 2}, 1}, // the film has no seasons
	}
	for _, c := range cases {
		st, err := s.Stats(ctx, c.r)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if st.Titles != c.want {
			t.Errorf("%s: %d titles, want %d", c.name, st.Titles, c.want)
		}
	}
	pool, err := s.LoadPool(ctx, Rules{}, 30)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range pool {
		if e.Title == "Locked" {
			t.Fatal("a scrambled disc reached a pool")
		}
	}
}

func TestWatchedRule(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	var id, user int64
	res, err := s.DB.ExecContext(ctx, `INSERT INTO users(name, password_hash, created_at) VALUES('a', '', 1)`)
	if err != nil {
		t.Fatal(err)
	}
	user, _ = res.LastInsertId()
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM items LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO progress(user_id, item_id, position_sec, duration_sec, watched, updated_at) VALUES(?,?,1,1,1,1)`, user, id); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Stats(ctx, Rules{Watched: "unwatched", WatchedBy: user})
	if st.Titles != 44 {
		t.Fatalf("unwatched: %d", st.Titles)
	}
	st, _ = s.Stats(ctx, Rules{Watched: "watched", WatchedBy: user})
	if st.Titles != 1 {
		t.Fatalf("watched: %d", st.Titles)
	}
}

func TestChannelLifecycleAndStableGuide(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	c := &Channel{Name: "Fairview", Definition: Definition{Rules: Rules{Networks: []string{"Fairview"}}, AlignMin: 30}}
	if err := s.Create(ctx, c, 1); err != nil {
		t.Fatal(err)
	}
	if c.Number != "101" {
		t.Fatalf("first number %s", c.Number)
	}
	now := time.Now()
	cur, next, err := s.Now(ctx, c, now)
	if err != nil || cur == nil || next == nil {
		t.Fatalf("now: %v %v %v", cur, next, err)
	}
	if cur.Start > now.Unix() || cur.End <= now.Unix() {
		t.Fatalf("current programme does not cover now: %+v", cur)
	}
	first, _ := s.Day(ctx, c, DayNumber(now, time.UTC))
	again, _ := s.Day(ctx, c, DayNumber(now, time.UTC))
	if !reflect.DeepEqual(first, again) {
		t.Fatal("the guide changed between two looks")
	}
	// Scheduling tomorrow carries on from today.
	tomorrow, err := s.Day(ctx, c, DayNumber(now, time.UTC)+1)
	if err != nil || len(tomorrow) == 0 || tomorrow[0].Start < first[len(first)-1].End {
		t.Fatalf("tomorrow overlaps today: %v", err)
	}
	// An edit keeps what is on now, and is kept in the history.
	c.Definition.AlignMin = 0
	if err := s.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	c2, _ := s.Get(ctx, c.ID)
	if c2.Rev != 2 {
		t.Fatalf("rev %d", c2.Rev)
	}
	cur2, _, err := s.Now(ctx, c2, now)
	if err != nil || cur2.ItemID != cur.ItemID || cur2.End != cur.End {
		t.Fatalf("what is on now should survive an edit: %+v vs %+v (%v)", cur2, cur, err)
	}
	hist, _ := s.History(ctx, c.ID)
	if len(hist) != 1 || hist[0].Definition.AlignMin != 30 {
		t.Fatalf("history: %+v", hist)
	}
	if _, err := s.Restore(ctx, c.ID, hist[0].ID); err != nil {
		t.Fatal(err)
	}
	c3, _ := s.Get(ctx, c.ID)
	if c3.Definition.AlignMin != 30 {
		t.Fatal("restore did not bring the old version back")
	}
}

func TestLongGapReplaysWithoutStoring(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	c := &Channel{Name: "Old", Definition: Definition{Rules: Rules{Genres: []string{"Comedy"}}, Order: Order{Mode: "sequential"}}}
	if err := s.Create(ctx, c, 1); err != nil {
		t.Fatal(err)
	}
	today := DayNumber(time.Now(), time.UTC)
	c.AnchorDay = today - 30
	a, err := s.Day(ctx, c, today)
	if err != nil {
		t.Fatal(err)
	}
	var stored int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM channel_days WHERE channel_id = ?`, c.ID).Scan(&stored)
	if stored != 1 {
		t.Fatalf("stored %d days, want only today", stored)
	}
	// Starting over from the anchor gives the same day.
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM channel_days`); err != nil {
		t.Fatal(err)
	}
	b, _ := s.Day(ctx, c, today)
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	if string(x) != string(y) {
		t.Fatal("replaying gave a different day")
	}
}

func TestVisibility(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	kids := &auth.User{ID: 5, Kind: "kids"}
	kidsA := library.DefaultAccess("kids")
	adult := &auth.User{ID: 6, Kind: auth.KindAdult}
	age := 7
	kidChan := &Channel{Enabled: true, Definition: Definition{Rules: Rules{Genres: []string{"Children"}, MaxAge: &age}}}
	mixed := &Channel{Enabled: true, Definition: Definition{Rules: Rules{}}}
	if !s.Visible(ctx, kidChan, kids, &kidsA) {
		t.Error("a children's channel should show for a Kids profile")
	}
	if s.Visible(ctx, mixed, kids, &kidsA) {
		t.Error("a mixed channel must not show for a Kids profile")
	}
	// The film in the test library has no rating at all.
	unrated := &Channel{Enabled: true, Definition: Definition{Rules: Rules{Kinds: []string{"movies"}}}}
	if s.Visible(ctx, unrated, kids, &kidsA) {
		t.Error("an unrated title must not reach a profile that does not allow unrated titles")
	}
	allowsUnrated := kidsA
	allowsUnrated.AllowUnrated = true
	if !s.Visible(ctx, unrated, kids, &allowsUnrated) {
		t.Error("a profile that allows unrated titles sees them")
	}
	// A block with an adult show hides the channel too.
	withBlock := &Channel{Enabled: true, Definition: Definition{Mode: "rules", Rules: kidChan.Definition.Rules, Blocks: []Block{{Start: "21:00", End: "22:00", Rules: &Rules{Ratings: []string{"TV-14"}}}}}}
	if s.Visible(ctx, withBlock, kids, &kidsA) {
		t.Error("a late-night block must hide the channel from a Kids profile")
	}
	if !s.Visible(ctx, mixed, adult, nil) {
		t.Error("an adult with no limits sees it")
	}
	adultsOnly := &Channel{Enabled: true, Visibility: Visibility{Mode: "adults"}, Definition: kidChan.Definition}
	if s.Visible(ctx, adultsOnly, kids, &kidsA) {
		t.Error("adults only")
	}
	only := &Channel{Enabled: true, Visibility: Visibility{Mode: "only", Users: []int64{6}}, Definition: kidChan.Definition}
	if s.Visible(ctx, only, kids, &kidsA) || !s.Visible(ctx, only, adult, nil) {
		t.Error("chosen profiles")
	}
	noTV := library.DefaultAccess("kids")
	noTV.LiveTV = false
	if s.Visible(ctx, kidChan, kids, &noTV) {
		t.Error("Live TV off")
	}
	off := &Channel{Enabled: false, Definition: kidChan.Definition}
	if s.Visible(ctx, off, adult, nil) {
		t.Error("a disabled channel is hidden")
	}
}

func TestSuggestAndAsAired(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	nets, err := s.Networks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 2 || nets[0].Shows != 2 || nets[0].Slots != 2 {
		t.Fatalf("networks should merge spellings: %+v", nets)
	}
	sug, err := s.Suggest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]Suggestion{}
	for _, x := range sug {
		keys[x.Key] = x
	}
	if _, ok := keys["network:"+nets[0].Name]; !ok {
		t.Fatalf("no network suggestion: %v", keys)
	}
	aired, ok := keys["network:"+nets[0].Name+":asaired"]
	if !ok {
		t.Fatal("no as-it-aired suggestion")
	}
	if aired.Definition.Mode != "timetable" || len(aired.Definition.Blocks) != 2 {
		t.Fatalf("as aired: %+v", aired.Definition)
	}
	if err := aired.Definition.Validate(); err != nil {
		t.Fatal(err)
	}
	b := aired.Definition.Blocks[0]
	if b.Start != "20:00" || b.End != "20:30" || b.Days[0] != 4 {
		t.Fatalf("slot: %+v", b)
	}
	// One-click channels work end to end.
	c := &Channel{Name: aired.Name, Definition: aired.Definition}
	if err := s.Create(ctx, c, 1); err != nil {
		t.Fatal(err)
	}
	day := DayNumber(time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC), time.UTC) // a Thursday
	c.AnchorDay = day
	progs, err := s.Day(ctx, c, day)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 5, 20, 0, 0, 0, time.UTC).Unix()
	for _, p := range progs {
		if p.Start == want && p.Kind == KindShow && p.Title == "Ninety Comedy" {
			return
		}
	}
	t.Fatalf("the show did not air in its original slot: %+v", progs)
}
