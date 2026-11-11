// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package parental

import (
	"context"
	"errors"
	"testing"
	"time"
)

// at is a moment in the week of Monday 2 November 2026.
func at(day time.Weekday, clock string) time.Time {
	m, ok := clockOf(clock)
	if !ok {
		panic(clock)
	}
	offset := (int(day) + 6) % 7 // Monday is 0
	return time.Date(2026, time.November, 2+offset, m/60, m%60, 0, 0, time.UTC)
}

func clockOf(s string) (int, bool) { return clock(s) }

func TestTheWeekTheTestsUse(t *testing.T) {
	if d := at(time.Monday, "12:00"); d.Weekday() != time.Monday {
		t.Fatalf("%v", d)
	}
	if d := at(time.Sunday, "12:00"); d.Weekday() != time.Sunday || d.Day() != 8 {
		t.Fatalf("%v", d)
	}
}

func TestAllowanceBySchoolDayAndWeekend(t *testing.T) {
	r := &TimeRules{SchoolDayMinutes: 60, WeekendMinutes: 180}
	for _, c := range []struct {
		day  time.Weekday
		want int
	}{{time.Monday, 3600}, {time.Friday, 3600}, {time.Saturday, 10800}, {time.Sunday, 10800}} {
		if got := r.AllowanceSec(at(c.day, "10:00")); got != c.want {
			t.Errorf("%v: %d, want %d", c.day, got, c.want)
		}
	}
	if got := (&TimeRules{WeekendMinutes: 60}).AllowanceSec(at(time.Tuesday, "10:00")); got != -1 {
		t.Errorf("a day with no allowance: %d", got)
	}
}

func TestCheck(t *testing.T) {
	rules := &TimeRules{SchoolDayMinutes: 60, WeekendMinutes: 120, From: "07:00", SchoolNightUntil: "20:00", WeekendUntil: "21:30"}
	for _, c := range []struct {
		name      string
		rules     *TimeRules
		now       time.Time
		usage     Usage
		allowed   bool
		reason    string
		remaining int
	}{
		{"no rules", nil, at(time.Monday, "03:00"), Usage{UsedSec: 99999}, true, "", -1},
		{"empty rules", &TimeRules{}, at(time.Monday, "03:00"), Usage{}, true, "", -1},
		{"plenty left", rules, at(time.Monday, "16:00"), Usage{UsedSec: 600}, true, "", 3000},
		{"allowance used up", rules, at(time.Monday, "16:00"), Usage{UsedSec: 3600}, false, ReasonAllowance, 0},
		{"over the allowance", rules, at(time.Monday, "16:00"), Usage{UsedSec: 5000}, false, ReasonAllowance, 0},
		{"extra time adds to it", rules, at(time.Monday, "16:00"), Usage{UsedSec: 3600, ExtraSec: 900}, true, "", 900},
		{"bedtime comes before the allowance runs out", rules, at(time.Monday, "19:50"), Usage{}, true, "", 600},
		{"bedtime on a school night", rules, at(time.Thursday, "20:00"), Usage{}, false, ReasonBedtime, 0},
		{"Friday is not a school night", rules, at(time.Friday, "20:30"), Usage{}, true, "", 3600},
		{"Friday's bedtime", rules, at(time.Friday, "21:30"), Usage{}, false, ReasonBedtime, 0},
		{"Sunday is a school night", rules, at(time.Sunday, "20:15"), Usage{}, false, ReasonBedtime, 0},
		{"Sunday has the weekend's allowance", rules, at(time.Sunday, "12:00"), Usage{UsedSec: 3600}, true, "", 3600},
		{"too early", rules, at(time.Tuesday, "06:59"), Usage{}, false, ReasonEarly, 0},
		{"the first minute", rules, at(time.Tuesday, "07:00"), Usage{}, true, "", 3600},
		{"a parent allowed a little longer past bedtime", rules, at(time.Monday, "20:10"), Usage{ExtraSec: 1800, ExtraUntil: at(time.Monday, "20:40")}, true, "", 1800},
		{"and that ran out", rules, at(time.Monday, "20:40"), Usage{ExtraSec: 1800, ExtraUntil: at(time.Monday, "20:40")}, false, ReasonBedtime, 0},
		{"extra past bedtime doesn't outlast the allowance", rules, at(time.Monday, "20:10"), Usage{UsedSec: 3600 + 1500, ExtraSec: 1800, ExtraUntil: at(time.Monday, "20:40")}, true, "", 300},
		{"only a bedtime: the day starts at six", &TimeRules{SchoolNightUntil: "20:00"}, at(time.Tuesday, "00:30"), Usage{}, false, ReasonEarly, 0},
		{"only a bedtime, in the day", &TimeRules{SchoolNightUntil: "20:00"}, at(time.Tuesday, "19:00"), Usage{}, true, "", 3600},
		{"only a bedtime on school nights, on a Saturday night", &TimeRules{SchoolNightUntil: "20:00"}, at(time.Saturday, "23:00"), Usage{}, true, "", 3600},
		{"only an earliest time", &TimeRules{From: "08:00"}, at(time.Monday, "23:30"), Usage{}, true, "", 1800},
	} {
		v := c.rules.Check(c.now, c.usage)
		if v.Allowed != c.allowed || v.Reason != c.reason || v.RemainingSec != c.remaining {
			t.Errorf("%s: allowed %v, reason %q, remaining %d; want %v, %q, %d", c.name, v.Allowed, v.Reason, v.RemainingSec, c.allowed, c.reason, c.remaining)
		}
		if v.Allowed != (v.Message() == "") {
			t.Errorf("%s: message %q for allowed %v", c.name, v.Message(), v.Allowed)
		}
		if v.Allowed && v.RemainingSec >= 0 && !v.Until.Equal(c.now.Add(time.Duration(v.RemainingSec)*time.Second)) {
			t.Errorf("%s: until %v isn't %d seconds after %v", c.name, v.Until, v.RemainingSec, c.now)
		}
	}
}

func TestWhenPlayingMayStartAgain(t *testing.T) {
	rules := &TimeRules{SchoolDayMinutes: 60, From: "07:00", SchoolNightUntil: "20:00", WeekendUntil: "21:30"}
	for _, c := range []struct {
		name  string
		now   time.Time
		usage Usage
		next  time.Time
	}{
		{"after bedtime: tomorrow morning", at(time.Monday, "21:00"), Usage{}, at(time.Tuesday, "07:00")},
		{"too early: this morning", at(time.Monday, "05:00"), Usage{}, at(time.Monday, "07:00")},
		{"allowance gone: tomorrow morning", at(time.Monday, "15:00"), Usage{UsedSec: 3600}, at(time.Tuesday, "07:00")},
	} {
		if v := rules.Check(c.now, c.usage); v.Allowed || !v.NextAt.Equal(c.next) {
			t.Errorf("%s: allowed %v, next %v, want %v", c.name, v.Allowed, v.NextAt, c.next)
		}
	}
}

func TestValid(t *testing.T) {
	for _, c := range []struct {
		rules TimeRules
		ok    bool
	}{
		{TimeRules{}, true},
		{TimeRules{SchoolDayMinutes: 90, WeekendMinutes: 1440}, true},
		{TimeRules{SchoolDayMinutes: -1}, false},
		{TimeRules{WeekendMinutes: 1441}, false},
		{TimeRules{From: "07:00", SchoolNightUntil: "20:30", WeekendUntil: "22:00"}, true},
		{TimeRules{From: "7:00"}, false},
		{TimeRules{From: "25:00"}, false},
		{TimeRules{SchoolNightUntil: "20:60"}, false},
		{TimeRules{From: "09:00", WeekendUntil: "08:00"}, false},
		{TimeRules{SchoolNightUntil: "05:00"}, false}, // before the day starts at six
		{TimeRules{From: "ab:cd"}, false},
	} {
		if err := c.rules.Valid(); (err == nil) != c.ok {
			t.Errorf("%+v: %v", c.rules, err)
		}
	}
	if !(*TimeRules)(nil).Empty() || !(&TimeRules{}).Empty() || (&TimeRules{From: "07:00"}).Empty() {
		t.Error("Empty")
	}
}

func TestAllows(t *testing.T) {
	for _, c := range []struct {
		max     int
		unrated bool
		age     int
		want    bool
	}{
		{-1, false, 18, true}, {-1, false, -1, true}, {13, false, 13, true}, {13, false, 14, false},
		{13, false, -1, false}, {13, true, -1, true}, {0, false, 0, true}, {0, true, 3, false},
	} {
		if got := Allows(c.max, c.unrated, c.age); got != c.want {
			t.Errorf("Allows(%d, %v, %d) = %v", c.max, c.unrated, c.age, got)
		}
	}
}

func TestSystemsAreInOrderAndAllowWhatTheyShould(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Systems {
		if seen[s.ID] || s.ID == "" || s.Name == "" || len(s.Kinds) == 0 {
			t.Errorf("%+v", s)
		}
		seen[s.ID] = true
		for _, k := range s.Kinds {
			if !ValidKind(k) {
				t.Errorf("%s rates %q, which isn't a kind", s.ID, k)
			}
		}
		for i := 1; i < len(s.Ratings); i++ {
			if s.Ratings[i].Age < s.Ratings[i-1].Age {
				t.Errorf("%s: %s comes after %s", s.ID, s.Ratings[i].Label, s.Ratings[i-1].Label)
			}
		}
	}
	mpa := Systems[0]
	if got := mpa.Allowed(13); len(got) != 3 || got[2] != "PG-13" {
		t.Errorf("MPA at 13: %v", got)
	}
	if got := mpa.Allowed(-1); len(got) != len(mpa.Ratings) {
		t.Errorf("MPA with no limit: %v", got)
	}
	if ValidKind("photo") || !ValidKind(KindGame) {
		t.Error("ValidKind")
	}
}

// memStore is a Store in memory.
type memStore struct {
	days   map[int]int
	loaded int
	fail   bool
	usage  Usage
}

func (m *memStore) Load(_ context.Context, user int64, day int) (Usage, error) {
	m.loaded++
	u := m.usage
	if user == 1 {
		u.UsedSec = m.days[day]
	}
	return u, nil
}

func (m *memStore) Add(_ context.Context, user int64, day int, seconds int) error {
	if user != 1 {
		return nil
	}
	if m.fail {
		return errors.New("the disk is full")
	}
	if m.days == nil {
		m.days = map[int]int{}
	}
	m.days[day] += seconds
	return nil
}

func TestTrackerCountsStretchesNotPauses(t *testing.T) {
	now := at(time.Monday, "16:00")
	st := &memStore{}
	tr := &Tracker{Store: st, Now: func() time.Time { return now }}
	ctx := context.Background()
	step := func(d time.Duration) { now = now.Add(d); tr.Tick(ctx, 1) }

	tr.Tick(ctx, 1) // the first request starts a stretch and counts nothing
	if u := tr.Usage(ctx, 1); u.UsedSec != 0 {
		t.Fatalf("at the start: %d", u.UsedSec)
	}
	for i := 0; i < 6; i++ {
		step(10 * time.Second)
	}
	if u := tr.Usage(ctx, 1); u.UsedSec != 60 {
		t.Errorf("a minute of reports: %d", u.UsedSec)
	}
	step(30 * time.Minute) // paused: a silence longer than the gap counts nothing
	if u := tr.Usage(ctx, 1); u.UsedSec != 60 {
		t.Errorf("after a pause: %d", u.UsedSec)
	}
	step(Gap) // exactly the gap still counts
	if u := tr.Usage(ctx, 1); u.UsedSec != 60+int(Gap/time.Second) {
		t.Errorf("after a gap: %d", u.UsedSec)
	}
	// Two devices at once count once: ticks in between add only the time that passed.
	now = now.Add(5 * time.Second)
	tr.Tick(ctx, 1)
	tr.Tick(ctx, 1)
	if u := tr.Usage(ctx, 1); u.UsedSec != 155 {
		t.Errorf("two devices: %d", u.UsedSec)
	}
	tr.Flush(ctx)
	if got := st.days[DayNumber(now)]; got != 155 {
		t.Errorf("written down: %d", got)
	}
	if u := tr.Usage(ctx, 2); u.UsedSec != 0 {
		t.Errorf("another profile: %d", u.UsedSec)
	}
}

func TestTrackerStartsAgainEachDay(t *testing.T) {
	now := at(time.Monday, "23:59")
	st := &memStore{}
	tr := &Tracker{Store: st, Now: func() time.Time { return now }}
	ctx := context.Background()
	tr.Tick(ctx, 1)
	now = now.Add(30 * time.Second)
	tr.Tick(ctx, 1)
	now = now.Add(60 * time.Second) // past midnight
	tr.Tick(ctx, 1)
	if got := st.days[DayNumber(at(time.Monday, "12:00"))]; got != 30 {
		t.Errorf("Monday: %d", got)
	}
	if u := tr.Usage(ctx, 1); u.UsedSec > 60 {
		t.Errorf("Tuesday began with %d", u.UsedSec)
	}
}

func TestTrackerKeepsWhatItCouldNotWrite(t *testing.T) {
	now := at(time.Monday, "16:00")
	st := &memStore{fail: true}
	tr := &Tracker{Store: st, Now: func() time.Time { return now }}
	ctx := context.Background()
	tr.Tick(ctx, 1)
	now = now.Add(20 * time.Second)
	tr.Tick(ctx, 1)
	if u := tr.Usage(ctx, 1); u.UsedSec != 20 {
		t.Fatalf("held: %d", u.UsedSec)
	}
	st.fail = false
	now = now.Add(10 * time.Second)
	tr.Tick(ctx, 1)
	if got := st.days[DayNumber(now)]; got != 30 {
		t.Errorf("written once the disk had room: %d", got)
	}
}

func TestTrackerForgetReadsExtraTimeAgain(t *testing.T) {
	now := at(time.Monday, "16:00")
	st := &memStore{}
	tr := &Tracker{Store: st, Now: func() time.Time { return now }}
	ctx := context.Background()
	if u := tr.Usage(ctx, 1); u.ExtraSec != 0 {
		t.Fatal(u)
	}
	st.usage.ExtraSec = 900
	if u := tr.Usage(ctx, 1); u.ExtraSec != 0 {
		t.Errorf("read again without being told: %+v", u)
	}
	tr.Forget(ctx, 1)
	if u := tr.Usage(ctx, 1); u.ExtraSec != 900 {
		t.Errorf("after Forget: %+v", u)
	}
}

func TestTrackerIsBounded(t *testing.T) {
	now := at(time.Monday, "16:00")
	tr := &Tracker{Store: &memStore{}, Now: func() time.Time { return now }}
	ctx := context.Background()
	for i := int64(1); i <= maxTracked*3; i++ {
		tr.Tick(ctx, i)
		now = now.Add(time.Second)
	}
	if len(tr.m) > maxTracked {
		t.Errorf("%d profiles remembered", len(tr.m))
	}
}
