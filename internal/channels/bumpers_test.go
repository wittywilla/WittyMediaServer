package channels

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// A loader that tells the sets of rules these tests use apart by what they name.
func bumperWorld() PoolLoader {
	shows := []Entry{}
	for i := int64(1); i <= 6; i++ {
		shows = append(shows, Entry{ID: 100 + i, Group: 1, ShowID: 1, Dur: 1500, Title: "The Show"})
	}
	for i := int64(1); i <= 6; i++ {
		shows = append(shows, Entry{ID: 200 + i, Group: 2, ShowID: 2, Dur: 1790, Title: "Long Show"})
	}
	sting := []Entry{{ID: 901, Dur: 10, Title: "Sting"}}
	ident := []Entry{{ID: 902, Dur: 20, Title: "Ident"}, {ID: 903, Dur: 30, Title: "Ident 2"}}
	ads := []Entry{{ID: 950, Dur: 60, Title: "Ad A"}, {ID: 951, Dur: 90, Title: "Ad B"}, {ID: 952, Dur: 120, Title: "Ad C"}}
	return func(r Rules, _ int64) ([]Entry, error) {
		switch {
		case len(r.Items) == 1 && r.Items[0] == 901:
			return sting, nil
		case len(r.Items) == 1 && r.Items[0] == 902:
			return ident, nil
		case len(r.Libraries) == 1 && r.Libraries[0] == 50:
			return ads, nil
		case len(r.Shows) == 1 && r.Shows[0] == 2:
			return shows[6:], nil
		case len(r.Shows) == 1 && r.Shows[0] == 1:
			return shows[:6], nil
		}
		return shows, nil
	}
}

func dayOf(t *testing.T, def *Definition) []Programme {
	t.Helper()
	if err := def.Validate(); err != nil {
		t.Fatal(err)
	}
	progs, _, err := GenerateDay(def, 7, 20000, time.UTC, State{}, bumperWorld(), map[string][]Entry{})
	if err != nil {
		t.Fatal(err)
	}
	noOverlap(t, progs)
	return progs
}

func rule(when string, item int64) BumperRule {
	return BumperRule{ID: when, Name: when, When: when, Source: Rules{Items: []int64{item}}}
}

func TestBumpersBeforeAndAfter(t *testing.T) {
	def := Definition{Mode: "simple", Order: Order{Mode: "sequential"}, Bumpers: BumperPlan{Rules: []BumperRule{rule(WhenBefore, 901), rule(WhenAfter, 902)}}}
	progs := dayOf(t, &def)
	want := []string{RolePre, "", RolePost}
	for i, r := range want {
		if progs[i].Role != r {
			t.Fatalf("programme %d has role %q, want %q: %+v", i, progs[i].Role, r, progs[i])
		}
	}
	if progs[0].Title != "Sting" || progs[1].Title != "The Show" || progs[2].Title != "Ident" {
		t.Errorf("wrong order: %s, %s, %s", progs[0].Title, progs[1].Title, progs[2].Title)
	}
	// Videos in a rule take turns.
	if progs[5].Title != "Ident 2" {
		t.Errorf("the rule's second video should come second: %+v", progs[5])
	}
}

func TestBumpersBetweenCommercials(t *testing.T) {
	def := Definition{Mode: "simple", Order: Order{Mode: "sequential"}, Filler: Filler{Between: 3, Rules: &Rules{Libraries: []int64{50}}},
		Bumpers: BumperPlan{Rules: []BumperRule{rule(WhenBetween, 901)}}}
	progs := dayOf(t, &def)
	var roles []string
	for _, p := range progs[1:6] {
		roles = append(roles, p.Role)
	}
	if want := []string{RoleAd, RoleBetween, RoleAd, RoleBetween, RoleAd}; !reflect.DeepEqual(roles, want) {
		t.Fatalf("after a programme: %v, want %v", roles, want)
	}
	// One commercial is no one to come between: not a bumper in sight.
	def.Filler.Between = 1
	for _, p := range dayOf(t, &def) {
		if p.Role == RoleBetween {
			t.Fatalf("a bumper between one commercial: %+v", p)
		}
	}
}

func TestBumperTiedToAShow(t *testing.T) {
	r := rule(WhenBefore, 901)
	r.Only = &Rules{Shows: []int64{2}}
	def := Definition{Mode: "simple", Order: Order{Mode: "sequential"}, Bumpers: BumperPlan{Rules: []BumperRule{r}}}
	var seen int
	progs := dayOf(t, &def)
	for i, p := range progs {
		if p.Kind != KindFiller {
			continue
		}
		seen++
		if progs[i+1].Title != "Long Show" {
			t.Fatalf("a bumper tied to one show came before %q", progs[i+1].Title)
		}
	}
	if seen == 0 {
		t.Fatal("the tied bumper never played")
	}
}

// The heart of it: with trimming on, the next programme starts on the grid and
// what gave way says so. A 25-minute show with 5 minutes of room keeps its ads;
// a show of 29:50 cuts them right down.
func TestCutCommercialsShortKeepsTheTimetable(t *testing.T) {
	plan := BumperPlan{Rules: []BumperRule{func() BumperRule { r := rule(WhenBefore, 901); r.Mandatory = true; return r }()},
		Trim: Trim{On: true, Ads: TrimCut, MinKeepSec: 5}}
	def := Definition{Mode: "simple", Order: Order{Mode: "sequential"}, AlignMin: 30, Filler: Filler{Between: 3, Rules: &Rules{Libraries: []int64{50}}}, Bumpers: plan}
	progs := dayOf(t, &def)
	dayStart := DayStart(20000, time.UTC).Unix()
	var trimmed, shows int
	for _, p := range progs {
		if p.Role == RolePre {
			if (p.Start-dayStart)%1800 != 0 {
				t.Fatalf("a slot did not start on the half hour: %+v", p)
			}
			shows++
		}
		if p.Trimmed > 0 {
			trimmed++
			if !p.Mandatory && p.Role != RoleAd && p.Role != RolePad {
				t.Errorf("only commercials may be cut here: %+v", p)
			}
		}
		if p.Mandatory && p.End-p.Start != 10 {
			t.Errorf("a mandatory bumper was shortened: %+v", p)
		}
	}
	if shows < 40 {
		t.Fatalf("only %d slots in a day", shows)
	}
	// Exactly on the grid, every one, with nothing out of place between them.
	for i := 1; i < len(progs); i++ {
		if progs[i].Start != progs[i-1].End {
			t.Fatalf("a gap in the schedule: %+v then %+v", progs[i-1], progs[i])
		}
	}
	if trimmed == 0 {
		t.Error("nothing was cut, though three commercials can't fit after a show of 29:50")
	}
}

// Without trimming the commercials all play, and the show after them is late.
func TestWithoutTrimmingEverythingPlays(t *testing.T) {
	def := Definition{Mode: "simple", Order: Order{Mode: "sequential"}, AlignMin: 30, Filler: Filler{Between: 3, Rules: &Rules{Libraries: []int64{50}}}}
	for _, p := range dayOf(t, &def) {
		if p.Trimmed != 0 {
			t.Fatalf("something was cut with trimming off: %+v", p)
		}
	}
}

func TestDropRatherThanCut(t *testing.T) {
	def := Definition{Mode: "simple", Order: Order{Mode: "sequential"}, AlignMin: 30, Filler: Filler{Between: 3, Rules: &Rules{Libraries: []int64{50}}},
		Bumpers: BumperPlan{Trim: Trim{On: true, Ads: TrimDrop}}}
	for _, p := range dayOf(t, &def) {
		if p.Trimmed != 0 {
			t.Fatalf("whole commercials only, but one was cut: %+v", p)
		}
	}
}

func TestKeepNeverGivesWay(t *testing.T) {
	def := Definition{Mode: "simple", Order: Order{Mode: "sequential"}, AlignMin: 30, Filler: Filler{Between: 3, Rules: &Rules{Libraries: []int64{50}}},
		Bumpers: BumperPlan{Trim: Trim{On: true, Ads: TrimKeep}}}
	ads := 0
	for _, p := range dayOf(t, &def) {
		if p.Role == RoleAd {
			ads++
			if p.Trimmed != 0 {
				t.Fatalf("commercials are to play in full: %+v", p)
			}
		}
	}
	if ads == 0 {
		t.Fatal("no commercials played at all")
	}
}

// A programme a little over its slot may lose its tail, when the owner allows
// it, rather than leave a gap of nearly a whole slot.
func TestTailCanBeCut(t *testing.T) {
	over := func(Rules, int64) ([]Entry, error) {
		return []Entry{{ID: 1, Dur: 1860, Title: "Just Over"}, {ID: 2, Dur: 1860, Title: "Just Over 2"}}, nil
	}
	def := Definition{Mode: "simple", Order: Order{Mode: "sequential"}, AlignMin: 30, Bumpers: BumperPlan{Trim: Trim{On: true, TailSec: 120}}}
	progs, _, err := GenerateDay(&def, 1, 20000, time.UTC, State{}, over, nil)
	if err != nil {
		t.Fatal(err)
	}
	noOverlap(t, progs)
	if progs[0].End-progs[0].Start != 1800 || progs[0].Trimmed != 60 {
		t.Errorf("the first should run 30 minutes, losing 60 seconds: %+v", progs[0])
	}
	if progs[1].Start != progs[0].End || progs[1].Title != "Just Over 2" {
		t.Errorf("the next should start right on time: %+v", progs[1])
	}
}

func TestBreaksInsideAProgrammeAreOneInTheGuide(t *testing.T) {
	r := rule(WhenEvery, 901)
	r.EveryMin = 10
	def := Definition{Mode: "simple", Order: Order{Mode: "sequential"}, Bumpers: BumperPlan{Rules: []BumperRule{r}}}
	progs := dayOf(t, &def)
	if progs[1].Role != RoleBreak || progs[2].Offset != 600 {
		t.Fatalf("expected a break at 10 minutes: %+v, %+v", progs[1], progs[2])
	}
	var played int64
	for _, p := range progs[:6] {
		if p.Kind == KindShow && p.ItemID == progs[0].ItemID {
			played += p.End - p.Start
		}
	}
	if played != 1500 {
		t.Errorf("the parts of a 25-minute show add up to %d seconds", played)
	}
	cells := GuideCells(progs)
	if cells[0].Title != "The Show" || cells[0].ItemID != progs[0].ItemID || cells[1].ItemID == cells[0].ItemID {
		t.Fatalf("the guide should show one programme: %+v / %+v", cells[0], cells[1])
	}
	if cells[0].End != cells[1].Start {
		t.Errorf("the guide has a hole: %+v / %+v", cells[0], cells[1])
	}
}

func TestLeadInStartsTheSlotInTheGuide(t *testing.T) {
	def := Definition{Mode: "simple", Order: Order{Mode: "sequential"}, AlignMin: 30, Bumpers: BumperPlan{Rules: []BumperRule{rule(WhenBefore, 901)}}}
	progs := dayOf(t, &def)
	cells := GuideCells(progs)
	if cells[0].Start != progs[0].Start || cells[0].Title != "The Show" {
		t.Fatalf("the slot should start with its bumper: %+v", cells[0])
	}
	for i := 1; i < len(cells); i++ {
		if cells[i].Start != cells[i-1].End {
			t.Fatalf("a hole in the guide at %d: %+v / %+v", i, cells[i-1], cells[i])
		}
	}
}

func TestBumperScheduleIsTheSameEveryTime(t *testing.T) {
	r := rule(WhenBefore, 902)
	def := Definition{Mode: "simple", Order: Order{Mode: "shuffle"}, AlignMin: 30, Filler: Filler{Between: 2, Rules: &Rules{Libraries: []int64{50}}},
		Bumpers: BumperPlan{Rules: []BumperRule{r}, Trim: Trim{On: true}}}
	if a, b := dayOf(t, &def), dayOf(t, &def); !reflect.DeepEqual(a, b) {
		t.Fatal("the same definition gave two different days")
	}
}

func TestBumperValidation(t *testing.T) {
	bad := []BumperPlan{
		{Rules: []BumperRule{{When: "sometimes"}}},
		{Rules: []BumperRule{{When: WhenEvery, EveryMin: 1}}},
		{Rules: []BumperRule{{When: WhenBefore, Count: 99}}},
		{Trim: Trim{Ads: "shred"}},
		{Trim: Trim{TailSec: 99999}},
	}
	for i, p := range bad {
		def := Definition{Bumpers: p}
		if def.Validate() == nil {
			t.Errorf("plan %d should be refused", i)
		}
	}
	for _, c := range []DiscClip{{Item: 0}, {Item: 1, Start: 10, End: 11}, {Item: 1, Start: -1}} {
		def := Definition{Rules: Rules{Clips: []DiscClip{c}}}
		if def.Validate() == nil {
			t.Errorf("clip %+v should be refused", c)
		}
	}
	for _, b := range []Block{{Start: "00:00", End: "01:00", FromDate: "12-01"}, {Start: "00:00", End: "01:00", FromDate: "13-01", ToDate: "12-31"}} {
		def := Definition{Blocks: []Block{b}}
		if def.Validate() == nil {
			t.Errorf("block %+v should be refused", b)
		}
	}
}

func TestOldDefinitionsAreUntouched(t *testing.T) {
	const old = `{"mode":"rules","rules":{"genres":["Comedy"]},"order":{"mode":"rotate"},"blocks":null,"filler":{"between":2},"alignMin":30}`
	var def Definition
	if err := json.Unmarshal([]byte(old), &def); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != old {
		t.Fatalf("saving an old channel would change it, and so rebuild its schedule:\n%s\n%s", old, b)
	}
}

func TestHolidayBlockTakesOver(t *testing.T) {
	holiday := Rules{Shows: []int64{2}}
	def := Definition{Mode: "rules", Order: Order{Mode: "sequential"}, Blocks: []Block{
		{ID: "evening", Start: "18:00", End: "22:00", Rules: &Rules{Shows: []int64{1}}},
		{ID: "xmas", Start: "12:00", End: "20:00", Rules: &holiday, FromDate: "12-20", ToDate: "12-26"},
		{ID: "nye", Start: "00:00", End: "23:00", Rules: &holiday, FromDate: "12-31", ToDate: "01-01", Marathon: true},
	}}
	day := DayNumber(time.Date(2030, 12, 24, 12, 0, 0, 0, time.UTC), time.UTC)
	progs, _, err := GenerateDay(&def, 1, day, time.UTC, State{}, bumperWorld(), nil)
	if err != nil {
		t.Fatal(err)
	}
	noOverlap(t, progs)
	noon := time.Date(2030, 12, 24, 12, 30, 0, 0, time.UTC).Unix()
	evening := time.Date(2030, 12, 24, 20, 30, 0, 0, time.UTC).Unix()
	titleAt := func(u int64) string {
		for _, p := range progs {
			if p.Start <= u && u < p.End {
				return p.Title
			}
		}
		return ""
	}
	if got := titleAt(noon); got != "Long Show" {
		t.Errorf("in season the holiday block plays at noon, got %q", got)
	}
	if got := titleAt(evening); got != "The Show" {
		t.Errorf("after the season's hours the evening block plays, got %q", got)
	}
	// Out of season it isn't there at all.
	day = DayNumber(time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC), time.UTC)
	progs, _, err = GenerateDay(&def, 1, day, time.UTC, State{}, bumperWorld(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range progs {
		if p.Title == "Long Show" && p.Start >= time.Date(2030, 6, 1, 18, 0, 0, 0, time.UTC).Unix() && p.Start < time.Date(2030, 6, 1, 22, 0, 0, 0, time.UTC).Unix() {
			t.Fatalf("the holiday block played in June: %+v", p)
		}
	}
	// Running over New Year: both ends count, and a marathon has no waiting.
	day = DayNumber(time.Date(2031, 1, 1, 12, 0, 0, 0, time.UTC), time.UTC)
	def.AlignMin = 30
	progs, _, err = GenerateDay(&def, 1, day, time.UTC, State{}, bumperWorld(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 5; i++ {
		if progs[i].Start != progs[i-1].End || progs[i].Kind != KindShow {
			t.Fatalf("a marathon should run straight on: %+v / %+v", progs[i-1], progs[i])
		}
	}
}

// Scenes from discs, against a real database.
func TestClipsFromDiscs(t *testing.T) {
	s, ls := testStore(t)
	ctx := context.Background()
	lib, err := ls.CreateLibrary(ctx, "Discs", "discs", []string{"/d"})
	if err != nil {
		t.Fatal(err)
	}
	disc := func(path string, scrambled bool) int64 {
		t.Helper()
		info := library.DiscInfo{MainTitle: 2, Scrambled: scrambled, Titles: []library.DiscTitle{
			{Number: 1, DurationSec: 120, Chapters: []float64{0}},
			{Number: 2, DurationSec: 5400, Chapters: []float64{0, 600, 1200, 1800}},
		}}
		raw, _ := json.Marshal(info)
		res, err := s.DB.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, runtime_sec, disc_json, added_at, updated_at) VALUES(?, 'dvd-iso', ?, ?, ?, 5400, ?, 1, 1)`,
			lib.ID, path, "Made Up Film", "Made Up Film", string(raw))
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	film := disc("/d/a.iso", false)
	locked := disc("/d/b.iso", true)

	r := Rules{Clips: []DiscClip{
		{ID: "main", Item: film},
		{ID: "s1", Item: film, Title: 2, Start: 600, End: 1200, FromChapter: 2, ToChapter: 2},
		{ID: "s2", Item: film, Title: 1, Label: "A trailer"},
		{ID: "bad", Item: film, Title: 9},
		{ID: "past", Item: film, Title: 2, Start: 5000, End: 99999},
		{ID: "lock", Item: locked},
	}}
	got, err := s.LoadPool(ctx, r, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("want four clips, without the missing title or the encrypted disc: %+v", got)
	}
	if got[0].Dur != 5400 || got[0].Clip.Title != 2 || got[0].Sub != "" {
		t.Errorf("main feature: %+v %+v", got[0], got[0].Clip)
	}
	if got[1].Dur != 600 || got[1].Clip.Start != 600 || got[1].Sub != "Chapter 2" {
		t.Errorf("scene: %+v %+v", got[1], got[1].Clip)
	}
	if got[2].Dur != 120 || got[2].Sub != "A trailer" {
		t.Errorf("whole short title: %+v", got[2])
	}
	if got[3].Dur != 400 {
		t.Errorf("an end past the title should be held to it: %+v", got[3])
	}
	ids := map[int64]bool{}
	for _, e := range got {
		if e.ID >= 0 || ids[e.ID] {
			t.Errorf("each clip needs an id of its own: %d", e.ID)
		}
		ids[e.ID] = true
		if e.itemID() != film {
			t.Errorf("a clip's item should be its disc: %+v", e)
		}
	}
	// A channel of nothing but clips plays nothing else, with the scene's offset.
	def := Definition{Mode: "simple", Rules: Rules{Clips: r.Clips[1:2]}, Order: Order{Mode: "sequential"}}
	progs, _, err := GenerateDay(&def, 1, 20000, time.UTC, State{}, s.loader(ctx), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range progs {
		if p.ItemID != film || p.Clip == nil || p.Clip.Start != 600 || p.End-p.Start != 600 {
			t.Fatalf("expected only the scene: %+v", p)
		}
	}
	// Clips only: stats count clips, and the check on who may see it looks at discs.
	st, err := s.Stats(ctx, Rules{Clips: r.Clips[1:2]})
	if err != nil || st.Titles != 1 || st.Seconds != 600 {
		t.Fatalf("stats: %+v %v", st, err)
	}
	// Saving a scene from an encrypted disc is refused outright.
	c := &Channel{Name: "Locked", Definition: Definition{Rules: Rules{Clips: []DiscClip{{ID: "x", Item: locked}}}}}
	if err := s.Create(ctx, c, 1); err != ErrEncryptedClip {
		t.Fatalf("a scene from an encrypted disc must be refused: %v", err)
	}
	c = &Channel{Name: "Fine", Categories: []string{" Movies ", "movies", "Late"}, Look: Look{Accent: "#336699"}, Definition: Definition{Rules: Rules{Clips: []DiscClip{{ID: "x", Item: film}}}}}
	if err := s.Create(ctx, c, 1); err != nil {
		t.Fatal(err)
	}
	back, err := s.Get(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.Categories, []string{"Movies", "Late"}) || back.Look.Accent != "#336699" {
		t.Errorf("categories and colour should be kept: %+v %+v", back.Categories, back.Look)
	}
	if err := (&Channel{Name: "x", Look: Look{Accent: "red;"}}).Check(); err == nil {
		t.Error("a colour that isn't one should be refused")
	}
}

func TestBumperLibraryAndViewerLists(t *testing.T) {
	s, ls := testStore(t)
	ctx := context.Background()
	lib, err := ls.CreateLibrary(ctx, "Ads", library.CommercialsType, []string{"/ads"})
	if err != nil {
		t.Fatal(err)
	}
	add := func(path string, libID int64) int64 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, runtime_sec, added_at, updated_at) VALUES(?, 'video', ?, ?, ?, 15, 1, 1)`, libID, path, path, path)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	inLib := add("/ads/a.mp4", lib.ID)
	other, _ := ls.CreateLibrary(ctx, "Clips", "videos", []string{"/c"})
	chosen := add("/c/x.mp4", other.ID)
	if err := s.AddBumper(ctx, chosen, false); err != nil {
		t.Fatal(err)
	}
	list, err := s.Bumpers(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %+v %v", list, err)
	}
	if !list[0].Chosen || list[0].ItemID != chosen || list[1].Chosen || list[1].Uploaded {
		t.Errorf("wrong flags or order: %+v", list)
	}
	if _, err := s.RemoveBumper(ctx, inLib); err != ErrBundledBumper {
		t.Errorf("one from a commercials library is managed there: %v", err)
	}
	if up, err := s.RemoveBumper(ctx, chosen); up || err != nil {
		t.Errorf("remove chosen: %v %v", up, err)
	}

	// Favourites and reminders belong to one account.
	for _, name := range []string{"one", "two"} {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO users(name, password_hash, created_at) VALUES(?, '', 1)`, name); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetFavourite(ctx, 1, "lib-1", true); err != nil {
		t.Fatal(err)
	}
	if f, _ := s.Favourites(ctx, 1); !f["lib-1"] {
		t.Error("favourite not kept")
	}
	if f, _ := s.Favourites(ctx, 2); len(f) != 0 {
		t.Error("favourites leaked to another account")
	}
	if err := s.SetFavourite(ctx, 1, "lib-1", false); err != nil {
		t.Fatal(err)
	}
	if f, _ := s.Favourites(ctx, 1); len(f) != 0 {
		t.Error("favourite not removed")
	}
	r := &Reminder{Channel: "lib-1", Title: "Show", Start: 1000, End: 2000}
	if err := s.AddReminder(ctx, 1, r); err != nil || r.ID == 0 {
		t.Fatalf("reminder: %+v %v", r, err)
	}
	if list, _ := s.Reminders(ctx, 2, 0); len(list) != 0 {
		t.Error("reminders leaked to another account")
	}
	if err := s.RemoveReminder(ctx, 2, r.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.Reminders(ctx, 1, 0); len(list) != 1 {
		t.Error("another account removed this one's reminder")
	}
	if list, _ := s.Reminders(ctx, 1, 3000); len(list) != 0 {
		t.Error("a reminder for what has ended should be forgotten")
	}
}
