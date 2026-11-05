package channels

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
)

// A reset changes what is on now, which an edit never does.
// Both tests compare a day's schedule before and after; across midnight that's two days.
func sameDay(t *testing.T) {
	t.Helper()
	if now := time.Now().UTC(); now.Hour() == 23 && now.Minute() >= 58 {
		t.Skip("too near midnight to compare one day's schedule")
	}
}

func TestResetStartsTheNewScheduleNow(t *testing.T) {
	sameDay(t)
	s, _ := testStore(t)
	ctx := context.Background()
	c := &Channel{Name: "Sunny", Definition: Definition{Rules: Rules{Networks: []string{"Sunny"}}}}
	if err := s.Create(ctx, c, 1); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	before, _, err := s.Now(ctx, c, now)
	if err != nil || before.Title != "Noughties Kids" {
		t.Fatalf("on now before: %+v %v", before, err)
	}
	c.Definition.Rules = Rules{Networks: []string{"Fairview"}}
	if err := s.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	c, _ = s.Get(ctx, c.ID)
	if cur, _, _ := s.Now(ctx, c, now); cur.Title != "Noughties Kids" {
		t.Fatalf("an edit should keep what is on now, got %q", cur.Title)
	}
	seed := c.Seed
	if _, err := s.Reset(ctx, c.ID, false); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Get(ctx, c.ID)
	if after.Rev != c.Rev+1 || after.Seed != seed {
		t.Fatalf("rev %d→%d, seed changed %v", c.Rev, after.Rev, after.Seed != seed)
	}
	cur, next, err := s.Now(ctx, after, time.Now())
	if err != nil || cur.Kind == KindSlate || cur.Title == "Noughties Kids" || cur.Start > now.Unix() || cur.End <= now.Unix() {
		t.Fatalf("on now after a reset: %+v %v", cur, err)
	}
	if next == nil || next.Title == "Noughties Kids" {
		t.Fatalf("next after a reset: %+v", next)
	}
	day, err := s.Day(ctx, after, DayNumber(now, time.UTC))
	if err != nil || len(day) == 0 {
		t.Fatal(err)
	}
	noOverlap(t, day)
	for _, p := range day {
		if p.Title == "Noughties Kids" {
			t.Fatalf("today still plays what the old settings chose: %+v", p)
		}
	}
	// Starting over picks a new shuffle.
	if _, err := s.Reset(ctx, c.ID, true); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.Get(ctx, c.ID); again.Seed == seed {
		t.Fatal("starting over should pick a new shuffle")
	}
	if _, err := s.Reset(ctx, 9999, false); err == nil {
		t.Fatal("no such channel should be an error")
	}
}

// Reminders stay with a programme that still airs, follow it to a new time, or
// go when it doesn't air any more.
func TestResetFollowsReminders(t *testing.T) {
	sameDay(t)
	s, _ := testStore(t)
	ctx := context.Background()
	users := &auth.Service{DB: s.DB}
	u1, err := users.CreateUser(ctx, "one", "a long enough password", true)
	if err != nil {
		t.Fatal(err)
	}
	u2, err := users.CreateUser(ctx, "two", "another long password", false)
	if err != nil {
		t.Fatal(err)
	}
	c := &Channel{Name: "Sunny", Definition: Definition{Rules: Rules{Networks: []string{"Sunny"}}}}
	if err := s.Create(ctx, c, u1.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	sched, err := s.Window(ctx, c, now, now.Add(12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var future []Programme
	for _, p := range GuideCells(sched) {
		if p.Start > now.Unix()+600 && p.Kind == KindShow {
			future = append(future, p)
		}
	}
	if len(future) < 3 {
		t.Fatalf("not enough to set reminders on: %d", len(future))
	}
	key := "lib-" + strconv.FormatInt(c.ID, 10)
	stays := future[1]
	// The same episode, remembered at a time it no longer has.
	moves := Reminder{Channel: key, Title: future[2].Title, Sub: future[2].Sub, Start: future[2].Start + 61, End: future[2].End + 61}
	add := func(u int64, r Reminder) *Reminder {
		t.Helper()
		if err := s.AddReminder(ctx, u, &r); err != nil {
			t.Fatal(err)
		}
		return &r
	}
	add(u1.ID, Reminder{Channel: key, Title: stays.Title, Sub: stays.Sub, Start: stays.Start, End: stays.End})
	// Someone else's reminder for the same programme is theirs, and stays too.
	add(u2.ID, Reminder{Channel: key, Title: stays.Title, Sub: stays.Sub, Start: stays.Start, End: stays.End})
	moved := add(u1.ID, moves)
	add(u2.ID, Reminder{Channel: key, Title: "Gone Show", Sub: "Pilot", Start: future[0].Start, End: future[0].End})
	// Another channel's reminder isn't touched.
	add(u2.ID, Reminder{Channel: "lib-999", Title: "Elsewhere", Start: future[0].Start, End: future[0].End})

	res, err := s.Reset(ctx, c.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kept != 2 || res.Moved != 1 || res.Dropped != 1 || !slices.Equal(res.DroppedTitles, []string{"Gone Show"}) {
		t.Fatalf("result %+v", res)
	}
	mine, _ := s.Reminders(ctx, u1.ID, now.Unix())
	theirs, _ := s.Reminders(ctx, u2.ID, now.Unix())
	if len(mine) != 2 || len(theirs) != 2 {
		t.Fatalf("left: mine %+v, theirs %+v", mine, theirs)
	}
	after, _ := s.Get(ctx, c.ID)
	sched, _ = s.Window(ctx, after, now, now.Add(reminderLook))
	cells := GuideCells(sched)
	for _, r := range mine {
		if r.ID != moved.ID {
			continue
		}
		if r.Start == moves.Start {
			t.Fatal("the reminder did not move")
		}
		if !slices.ContainsFunc(cells, func(p Programme) bool {
			return p.Start == r.Start && p.End == r.End && p.Title == moves.Title && p.Sub == moves.Sub
		}) {
			t.Fatalf("moved to %d, where that programme isn't", r.Start)
		}
	}
	if !slices.ContainsFunc(theirs, func(r Reminder) bool { return r.Channel == "lib-999" }) {
		t.Fatal("another channel's reminder went")
	}
}
