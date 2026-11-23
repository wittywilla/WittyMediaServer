// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package channels

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func ip(v int) *int { return &v }

// cloneLib is a small library: Friends season 1 with a hole in it, Cheers and Seinfeld (other
// sitcoms of their era, one of them on the same network as Frasier), a drama, and two films.
func cloneLib() *CloneLibrary {
	friends := &LibShow{ID: 1, Title: "Friends", Year: 1994, Network: "NBC", Genres: []string{"Comedy", "Romance"}, Provider: "tvdb", ProviderID: "79168"}
	titles := []string{"The One Where Monica Gets a Roommate", "The One with the Sonogram at the End", "The One with the Thumb", "The One with George Stephanopoulos", "The One with the East German Laundry Detergent"}
	for i, t := range titles {
		friends.Episodes = append(friends.Episodes, LibEpisode{Item: int64(100 + i + 1), Title: t, Season: 1, Episode: i + 1, Aired: "1994-10-" + string(rune('0'+i)) + "1", Dur: 22 * 60})
	}
	friends.Episodes = append(friends.Episodes, LibEpisode{Item: 110, Title: "The One with the Candy Hearts", Season: 1, Episode: 14, Dur: 22 * 60})
	cheers := &LibShow{ID: 2, Title: "Cheers", Year: 1982, Network: "NBC", Genres: []string{"Comedy"}}
	for i := range 4 {
		cheers.Episodes = append(cheers.Episodes, LibEpisode{Item: int64(200 + i + 1), Title: "Cheers ep", Season: 1, Episode: i + 1, Dur: 24 * 60})
	}
	seinfeld := &LibShow{ID: 3, Title: "Seinfeld", Year: 1989, Network: "NBC", Genres: []string{"Comedy"}}
	seinfeld.Episodes = []LibEpisode{{Item: 301, Title: "The Seinfeld Chronicles", Season: 1, Episode: 1, Dur: 23 * 60}}
	drama := &LibShow{ID: 4, Title: "Hospital Drama", Year: 2010, Network: "ABC", Genres: []string{"Drama"}}
	drama.Episodes = []LibEpisode{{Item: 401, Title: "Pilot", Season: 1, Episode: 1, Dur: 43 * 60}}
	lib := &CloneLibrary{
		Shows: []*LibShow{friends, cheers, seinfeld, drama},
		Titles: []*LibTitle{
			{Item: 501, Title: "Die Hard", Year: 1988, Genres: []string{"Action"}, Dur: 132 * 60},
			{Item: 502, Title: "Short Film", Year: 2001, Genres: []string{"Animation"}, Dur: 9 * 60},
		},
	}
	lib.Prepare()
	return lib
}

func TestCloneMatchingRules(t *testing.T) {
	slots := []CloneSlot{
		{Title: "Friends", Season: ip(1), Episode: ip(3), Dur: 1800},                          // 0: the episode itself
		{Title: "FRIENDS", Sub: "The One With The Sonogram At The End", Dur: 1800},            // 1: by its title
		{Title: "Friends", Season: ip(1), Episode: ip(9), Dur: 1800},                          // 2: not there: the closest
		{Title: "Friends", Dur: 1800},                                                         // 3: no episode given
		{Title: "Frasier", Genres: []string{"Comedy"}, Network: "NBC", Year: 1993, Dur: 1800}, // 4: not there: a show like it
		{Title: "Die Hard", Year: 1988, Movie: true, Dur: 7200},                               // 5: a film
		{Gap: true, Dur: 14 * 60},                                                             // 6: nothing in the guide
		{Title: "Friends", TVDB: "79168", Season: ip(1), Episode: ip(1), Dur: 1800},           // 7: by metadata id
		{Title: "Unknown Quiz", Genres: []string{"Game Show"}, Dur: 1800},                     // 8: nothing like it
		{Title: "Friends", Aired: "1994-10-41", Dur: 1800},                                    // 9: by air date (episode 5)
	}
	cloneLib().Match(slots, CloneOptions{})
	want := []struct {
		rule string
		item int64
	}{
		{RuleExact, 103}, {RuleTitle, 102}, {RuleNear, 105}, {RuleSeries, 0}, {RuleSimilar, 0},
		{RuleExact, 501}, {RuleFiller, 502}, {RuleExact, 101}, {RuleFiller, 0}, {RuleAired, 105},
	}
	for i, w := range want {
		s := slots[i]
		if s.Rule != w.rule || (w.item != 0 && s.Item != w.item) || s.Item == 0 {
			t.Errorf("slot %d (%s): got %s item %d (%s), want %s item %d", i, s.Title, s.Rule, s.Item, s.Why, w.rule, w.item)
		}
		if s.Confidence <= 0 || s.Confidence > 100 {
			t.Errorf("slot %d: confidence %d", i, s.Confidence)
		}
	}
	// The closest to episode 9 that isn't already playing is the next one after 5, not 14.
	if slots[2].Why == "" || !strings.Contains(slots[2].Why, "same season") {
		t.Errorf("the closest episode should say so: %q", slots[2].Why)
	}
	// A show like Frasier: a sitcom on NBC from its years.
	if s := slots[4]; s.Item/100 != 2 && s.Item/100 != 3 {
		t.Errorf("Frasier's stand-in should be Cheers or Seinfeld, got %d (%s)", s.Item, s.Why)
	}
	// Confidence falls as the rules get looser.
	if !(slots[0].Confidence > slots[2].Confidence && slots[2].Confidence > slots[4].Confidence && slots[4].Confidence > slots[6].Confidence) {
		t.Errorf("confidence should fall: exact %d, near %d, similar %d, filler %d", slots[0].Confidence, slots[2].Confidence, slots[4].Confidence, slots[6].Confidence)
	}
}

func TestCloneStandInsDontRepeat(t *testing.T) {
	var slots []CloneSlot
	for range 4 {
		slots = append(slots, CloneSlot{Title: "Friends", Dur: 1800})
	}
	cloneLib().Match(slots, CloneOptions{})
	seen := map[int64]bool{}
	for _, s := range slots {
		if seen[s.Item] {
			t.Fatalf("an episode plays twice while others are free: %+v", slots)
		}
		seen[s.Item] = true
	}
}

func TestCloneOptionsSwitchRulesOff(t *testing.T) {
	slots := []CloneSlot{
		{Title: "Friends", Season: ip(1), Episode: ip(9), Dur: 1800},
		{Title: "Frasier", Genres: []string{"Comedy"}, Network: "NBC", Dur: 1800},
		{Gap: true, Dur: 600},
	}
	cloneLib().Match(slots, CloneOptions{NoNear: true})
	if slots[0].Rule != RuleSeries {
		t.Errorf("without the closest episode, any episode: got %s", slots[0].Rule)
	}
	slots[0].Rule, slots[0].Item = "", 0
	cloneLib().Match(slots, CloneOptions{NoNear: true, NoSeries: true, NoSimilar: true, NoFiller: true})
	for i, s := range slots {
		if s.Rule != RuleNone || s.Item != 0 {
			t.Errorf("slot %d: with every stand-in off nothing plays, got %s %d", i, s.Rule, s.Item)
		}
	}
}

func TestClonePinnedSlotsStay(t *testing.T) {
	slots := []CloneSlot{{Title: "Friends", Season: ip(1), Episode: ip(3), Dur: 1800, Item: 401, Rule: RuleManual, Confidence: 100, Pinned: true}}
	cloneLib().Match(slots, CloneOptions{})
	if slots[0].Item != 401 || slots[0].Rule != RuleManual {
		t.Fatalf("a slot chosen by hand was changed: %+v", slots[0])
	}
}

func TestCloneCandidates(t *testing.T) {
	c := cloneLib().Candidates(CloneSlot{Title: "Friends", Season: ip(1), Episode: ip(3), Dur: 1800}, CloneOptions{}, 8)
	if len(c) < 4 || c[0].Item != 103 || c[0].Rule != RuleExact {
		t.Fatalf("candidates: %+v", c)
	}
	seen := map[int64]bool{}
	for _, x := range c {
		if seen[x.Item] {
			t.Fatalf("a candidate twice: %+v", c)
		}
		seen[x.Item] = true
	}
}

func TestTitleComparisons(t *testing.T) {
	if normTitle("The Office (US)") != "office us" || normTitle("Law & Order: SVU") != "law and order svu" || normTitle("Grey’s Anatomy") != "greys anatomy" {
		t.Errorf("normTitle: %q %q %q", normTitle("The Office (US)"), normTitle("Law & Order: SVU"), normTitle("Grey’s Anatomy"))
	}
	if stripYear("doctor who 2005") != "doctor who" || stripYear("1923") != "1923" {
		t.Error("stripYear")
	}
	if similarity("the one with the thumb", "the one with the thumbs") < 0.9 || similarity("pilot", "finale") > 0.3 {
		t.Error("similarity")
	}
}

func TestXMLTVImport(t *testing.T) {
	w := Window{From: time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC), Days: 1}
	f, err := os.Open("testdata/clone/station.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, err := XMLTVStations(f, w)
	if err != nil || len(st) != 2 || st[0].Name != "KWIT 4" || st[0].Count != 5 {
		t.Fatalf("stations: %+v %v", st, err)
	}
	f.Seek(0, 0)
	slots, err := XMLTVSlots(f, "kwit.test", w)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range slots {
		got = append(got, s.Wall[11:]+" "+s.Title)
	}
	// The one-minute ident joins Frasier; the quarter hour after it is a gap to fill.
	want := []string{"18:00 Friends", "18:30 Friends", "19:00 Frasier", "19:31 ", "19:45 Die Hard"}
	if !slices.Equal(got, want) {
		t.Fatalf("slots %q, want %q", got, want)
	}
	if s := slots[0]; *s.Season != 1 || *s.Episode != 3 || s.Sub != "The One with the Thumb" || s.Dur != 1800 {
		t.Errorf("first slot: %+v", s)
	}
	if !slots[3].Gap || slots[2].Dur != 31*60 || !slots[4].Movie || slots[4].Year != 1988 {
		t.Errorf("gap, ident and film: %+v", slots[2:])
	}
}

func TestTVmazeImport(t *testing.T) {
	body, err := os.ReadFile("testdata/clone/tvmaze.json")
	if err != nil {
		t.Fatal(err)
	}
	day, err := ParseTVmaze(body)
	if err != nil {
		t.Fatal(err)
	}
	st := TVmazeStations(day)
	if len(st) != 2 || st[0].Name != "CBS" || st[1].Name != "NBC" || st[1].Count != 2 || st[1].TZ != "America/New_York" {
		t.Fatalf("stations: %+v", st)
	}
	slots := TVmazeSlots("1", day)
	if len(slots) != 2 {
		t.Fatalf("slots: %+v", slots)
	}
	s := slots[0]
	if s.Wall != "2026-11-23 20:00" || s.Title != "Friends" || s.TVDB != "79168" || s.IMDb != "tt0108778" || s.Year != 1994 || s.Network != "NBC" || s.Dur != 1800 {
		t.Errorf("first: %+v", s)
	}
	if slots[1].Dur != 1800 {
		t.Errorf("a missing runtime uses the show's: %+v", slots[1])
	}
	if _, err := ParseTVmaze([]byte("<html>")); err == nil {
		t.Error("not a schedule")
	}
}

func TestTidyGuide(t *testing.T) {
	in := []CloneSlot{
		{Start: 1000, Dur: 1800, Wall: "2026-11-23 10:00", Title: "B"},
		{Start: 0, Dur: 1200, Wall: "2026-11-23 09:43", Title: "A"},   // overlaps B: stops at B
		{Start: 2800, Dur: 600, Wall: "2026-11-23 10:30", Title: "C"}, // 0 gap
		{Start: 3500, Dur: 900, Wall: "2026-11-23 10:41", Title: "D"}, // 100 s gap: C takes it
		{Start: 5400, Dur: 900, Wall: "2026-11-23 11:13", Title: "E"}, // 1000 s gap: a slot of its own
	}
	out := Tidy(in)
	var titles []string
	for _, s := range out {
		titles = append(titles, s.Title)
	}
	if !slices.Equal(titles, []string{"A", "B", "C", "D", "", "E"}) {
		t.Fatalf("got %q", titles)
	}
	if out[0].Dur != 1000 || out[2].Dur != 700 || !out[4].Gap || out[4].Dur != 1000 || out[4].Wall != "2026-11-23 10:56" {
		t.Fatalf("got %+v", out)
	}
}

func cloneDef(repeat string, slots ...CloneSlot) *Definition {
	return &Definition{Mode: "clone", Filler: Filler{Commercials: []CommercialPick{{Library: 60}}},
		Clone: &Clone{Source: CloneSource{Kind: "xmltv", Channel: "x"}, From: "2026-11-23", Days: 1, Options: CloneOptions{Repeat: repeat}, Slots: slots}}
}

func TestCompileClone(t *testing.T) {
	mon := CloneSlot{Wall: "2026-11-23 20:00", Dur: 1800, Title: "Friends", Item: 103, Rule: RuleExact}
	late := CloneSlot{Wall: "2026-11-23 23:30", Dur: 3600, Title: "Late", Item: 401, Rule: RuleSimilar}
	tue := CloneSlot{Wall: "2026-11-24 20:00", Dur: 1800, Title: "Tuesday", Item: 0, Rule: RuleNone}

	d := cloneDef(RepeatAuto, mon, late)
	d.CompileClone(time.UTC)
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(d.Blocks) != 2 || d.Blocks[0].Start != "20:00" || d.Blocks[0].End != "20:30" || len(d.Blocks[0].Days) != 0 || !d.Blocks[0].Once || d.Blocks[0].Fit != "cut" {
		t.Fatalf("one day plays every day: %+v", d.Blocks)
	}
	if b := d.Blocks[1]; b.End != "00:30" || !b.StandIn || b.Rules.Items[0] != 401 {
		t.Fatalf("past midnight, a stand-in: %+v", b)
	}

	d = cloneDef(RepeatAuto, mon, tue)
	d.Clone.Days = 2
	d.CompileClone(time.UTC)
	if len(d.Blocks) != 2 || d.Blocks[0].Days[0] != 1 || d.Blocks[1].Days[0] != 2 || d.Blocks[1].Rules != nil {
		t.Fatalf("days on their weekdays; nothing matched plays the channel's rules: %+v", d.Blocks)
	}

	d = cloneDef(RepeatOnce, mon)
	d.CompileClone(time.UTC)
	if b := d.Blocks[0]; b.FromDate != "11-23" || b.ToDate != "11-23" {
		t.Fatalf("only on the date: %+v", b)
	}

	d = cloneDef(RepeatDaily, mon, tue)
	d.Clone.Days = 2
	d.Clone.Options.ShiftMin = -90
	d.CompileClone(time.UTC)
	if len(d.Blocks) != 1 || d.Blocks[0].Start != "18:30" {
		t.Fatalf("daily plays the first day, shifted: %+v", d.Blocks)
	}

	// Real time: a station in New York at 20:00 plays at 01:00 here (UTC).
	d = cloneDef(RepeatDaily, CloneSlot{Wall: "2026-11-23 20:00", Start: time.Date(2026, 11, 24, 1, 0, 0, 0, time.UTC).Unix(), Dur: 1800, Item: 1, Rule: RuleExact})
	d.Clone.Options.RealTime = true
	d.CompileClone(time.UTC)
	if d.Blocks[0].Start != "01:00" {
		t.Fatalf("real time: %+v", d.Blocks)
	}
}

func TestCloneValidate(t *testing.T) {
	d := cloneDef("", CloneSlot{Wall: "2026-11-23 20:00", Dur: 1800})
	d.Clone.Days = 9
	if d.Validate() == nil {
		t.Error("too many days")
	}
	d = cloneDef("", CloneSlot{Wall: "8pm", Dur: 1800})
	if d.Validate() == nil {
		t.Error("bad time")
	}
	d = &Definition{Mode: "clone"}
	if d.Validate() == nil {
		t.Error("clone mode needs its guide")
	}
}

// A cloned evening plays each programme at its slot's start, puts commercials in breaks inside
// it and after it, and the next slot starts exactly on time. A programme longer than its slot is
// cut to fit; era matching picks commercials from the programme's years first.
func TestClonedDayKeepsToTheClock(t *testing.T) {
	day := 20000
	start := DayStart(day, time.UTC)
	wall := func(h, m int) string {
		return start.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute).Format("2006-01-02 15:04")
	}
	d := cloneDef(RepeatDaily,
		CloneSlot{Wall: wall(20, 0), Dur: 1800, Title: "Show 1", Item: 1001, Rule: RuleExact},
		CloneSlot{Wall: wall(20, 30), Dur: 1800, Title: "Show 2", Item: 2001, Rule: RuleSimilar},
		CloneSlot{Wall: wall(21, 0), Dur: 1800, Title: "Long", Item: 3001, Rule: RuleExact},
	)
	d.Clone.From = start.Format("2006-01-02")
	d.CompileClone(time.UTC)
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	shows := map[int64]Entry{
		1001: {ID: 1001, Dur: 22*60 + 13, Title: "Show 1", Year: 1994, Breaks: []int64{60, 900}},
		2001: {ID: 2001, Dur: 23 * 60, Title: "Show 2", Year: 1989},
		3001: {ID: 3001, Dur: 40 * 60, Title: "Long"},
	}
	var ads []Entry
	for i := range 30 {
		y := 1994
		if i%2 == 1 {
			y = 2015
		}
		ads = append(ads, Entry{ID: 700 + int64(i), Dur: []int64{15, 30, 30, 60}[i%4], Title: "Ad", Year: y})
	}
	load := func(r Rules, _ int64) ([]Entry, error) {
		if len(r.Libraries) == 1 && r.Libraries[0] == 60 {
			return ads, nil
		}
		if len(r.Items) == 1 {
			return []Entry{shows[r.Items[0]]}, nil
		}
		return []Entry{{ID: 9001, Dur: 25 * 60, Title: "Outside the guide"}}, nil
	}
	progs, _, err := GenerateDay(d, 1, day, time.UTC, State{}, load, nil)
	if err != nil {
		t.Fatal(err)
	}
	noOverlap(t, progs)
	at := func(h, m int) int64 {
		return start.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute).Unix()
	}
	firstAt := map[int64]int64{}
	played := map[int64]int64{}
	breaks := map[int64]int{}
	var era, offEra int
	for i, p := range progs {
		if p.Kind == KindShow {
			if _, ok := firstAt[p.ItemID]; !ok {
				firstAt[p.ItemID] = p.Start
			}
			played[p.ItemID] += p.End - p.Start
		}
		if p.Role == RoleBreak {
			breaks[progs[i-1].ItemID]++
		}
		if p.Kind == KindFiller && p.Start >= at(20, 0) && p.Start < at(20, 30) {
			if p.Year == 1994 {
				era++
			} else {
				offEra++
			}
		}
	}
	for id, h := range map[int64][2]int{1001: {20, 0}, 2001: {20, 30}, 3001: {21, 0}} {
		if firstAt[id] != at(h[0], h[1]) {
			t.Errorf("item %d starts at %d, want %d", id, firstAt[id], at(h[0], h[1]))
		}
	}
	if played[1001] != 22*60+13 || played[3001] != 30*60 {
		t.Errorf("played: %v (the long one cut to its slot)", played)
	}
	if breaks[1001] == 0 {
		t.Errorf("no commercial breaks inside the programme: %+v", progs)
	}
	if era == 0 || offEra > era {
		t.Errorf("commercials from the programme's era first: %d in it, %d not", era, offEra)
	}
	// Slots are filled to their ends: Show 1 plus its commercials take exactly 30 minutes.
	var used int64
	for _, p := range progs {
		if p.Start >= at(20, 0) && p.End <= at(20, 30) {
			used += p.End - p.Start
		}
	}
	if used < 1800-minSlate {
		t.Errorf("the 20:00 slot has %d seconds in it", used)
	}
	for _, p := range progs {
		if p.ItemID == 2001 && p.Kind == KindShow && p.Start >= at(20, 30) && p.Start < at(21, 0) && p.For != "Show 2" {
			t.Errorf("a stand-in says what it stands in for: %+v", p)
		}
	}
}

func TestBreakPoints(t *testing.T) {
	if got := breakPoints(1320, 2, nil); !slices.Equal(got, []int64{440, 880}) {
		t.Errorf("even: %v", got)
	}
	// A natural break nearby wins; one too far away doesn't.
	if got := breakPoints(1320, 2, []int64{400, 1300}); !slices.Equal(got, []int64{400, 880}) {
		t.Errorf("natural: %v", got)
	}
	if breakPoints(200, 3, nil) != nil {
		t.Error("too short for breaks")
	}
}

// An hour-long premiere listed as two episodes at the same time plays them one after the other.
func TestTidyTwoAtOnce(t *testing.T) {
	out := Tidy([]CloneSlot{
		{Start: 0, Dur: 1800, Wall: "2005-02-21 20:30", Title: "Avatar", Episode: ip(1)},
		{Start: 0, Dur: 1800, Wall: "2005-02-21 20:30", Title: "Avatar", Episode: ip(2)},
		{Start: 3600, Dur: 1800, Wall: "2005-02-21 21:30", Title: "Next"},
	})
	if len(out) != 3 || out[1].Start != 1800 || out[1].Wall != "2005-02-21 21:00" || *out[1].Episode != 2 || out[2].Start != 3600 {
		t.Fatalf("got %+v", out)
	}
}

// A stand-in far shorter than its slot plays its series on to the slot's end; filler takes turns
// between series.
func TestCloneLongSlotsAndFillerTurns(t *testing.T) {
	d := cloneDef(RepeatDaily,
		CloneSlot{Wall: "2026-11-23 19:45", Dur: 7200, Title: "A Film", Item: 201, ItemDur: 24 * 60, Show: 2, Rule: RuleSimilar},
		CloneSlot{Wall: "2026-11-23 21:45", Dur: 7200, Title: "Exact But Short", Item: 101, ItemDur: 22 * 60, Show: 1, Rule: RuleExact},
	)
	d.CompileClone(time.UTC)
	if b := d.Blocks[0]; b.Once || b.Fit != "fill" || len(b.Rules.Shows) != 1 || b.Rules.Shows[0] != 2 {
		t.Fatalf("a short stand-in in a long slot: %+v", b)
	}
	if b := d.Blocks[1]; !b.Once || b.Rules.Items[0] != 101 {
		t.Fatalf("the programme itself plays once, however short: %+v", b)
	}
	var slots []CloneSlot
	for range 4 {
		slots = append(slots, CloneSlot{Gap: true, Dur: 23 * 60})
	}
	cloneLib().Match(slots, CloneOptions{})
	shows := map[int64]bool{}
	for _, s := range slots {
		shows[s.Show] = true
	}
	if len(shows) < 2 {
		t.Fatalf("filler always from one series: %+v", slots)
	}
}

// A series whose episodes are all longer than the slots still doesn't repeat one while others are
// free.
func TestCloneNoRepeatWhenNothingFits(t *testing.T) {
	slots := []CloneSlot{
		{Title: "Like Cheers", Genres: []string{"Comedy"}, Network: "NBC", Year: 1983, Dur: 15 * 60},
		{Title: "Like Cheers", Genres: []string{"Comedy"}, Network: "NBC", Year: 1983, Dur: 15 * 60},
	}
	cloneLib().Match(slots, CloneOptions{NoFiller: true})
	if slots[0].Item == 0 || slots[0].Item == slots[1].Item {
		t.Fatalf("the same stand-in twice: %+v", slots)
	}
}
