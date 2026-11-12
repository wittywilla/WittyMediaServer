// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"strings"
	"testing"
	"time"
)

func TestSeatCodes(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	b := &seatBook{now: func() time.Time { return now }}
	b.started("s1", 1, "Willow", "Living room TV", seatGame{})

	if _, _, ok := b.invite("s1", 2); ok {
		t.Fatal("someone other than the host made a code")
	}
	code, expires, ok := b.invite("s1", 1)
	if !ok || len(code) != seatCodeLen || strings.Trim(code, seatCodeLetters) != "" || !expires.Equal(now.Add(seatCodeLife)) {
		t.Fatalf("code %q until %v", code, expires)
	}

	// Any case, with a dash or spaces, gives the lowest free seat; joining again keeps it.
	typed := strings.ToLower(code[:2]) + "- " + code[2:]
	if j, err := b.join(typed, 2, "Ann"); err != nil || j.Session != "s1" || j.Seat != 1 || j.Host {
		t.Fatalf("%+v %v", j, err)
	}
	if j, _ := b.join(code, 3, "Sam"); j.Seat != 2 {
		t.Fatalf("second seat %d", j.Seat)
	}
	if j, _ := b.join(code, 2, "Ann"); j.Seat != 1 {
		t.Fatalf("joining again moved seats: %d", j.Seat)
	}
	if j, err := b.join(code, 1, "Willow"); err != nil || !j.Host || j.Seat != 0 {
		t.Fatalf("the host's own code: %+v %v", j, err)
	}
	if !b.holds("s1", 2) || b.holds("s1", 1) || b.holds("s2", 2) {
		t.Fatal("holds")
	}

	// Only the host frees someone else's seat; anyone can leave their own.
	if b.leave("s1", 3, 2) {
		t.Fatal("one guest freed another's seat")
	}
	if !b.leave("s1", 1, 2) || b.holds("s1", 2) {
		t.Fatal("the host couldn't free a seat")
	}
	if !b.leave("s1", 3, 3) {
		t.Fatal("someone couldn't leave")
	}

	// A seat freed is taken again from the lowest.
	if j, _ := b.join(code, 4, "Kid"); j.Seat != 1 {
		t.Fatalf("got seat %d", j.Seat)
	}

	// The host sees the code; others don't.
	if v, _ := b.view("s1", true); v.Code != code || v.Host.Device != "Living room TV" {
		t.Fatalf("host's view %+v", v)
	}
	if v, _ := b.view("s1", false); v.Code != "" {
		t.Fatal("a guest saw the code")
	}

	// A new code replaces the old one.
	code2, _, _ := b.invite("s1", 1)
	if code2 != code {
		if _, err := b.join(code, 5, "Late"); err != errSeatCode {
			t.Fatalf("the old code still works: %v", err)
		}
	}

	// It runs out after ten minutes.
	now = now.Add(seatCodeLife + time.Second)
	if _, err := b.join(code2, 6, "Later"); err != errSeatCode {
		t.Fatalf("an old code worked: %v", err)
	}

	// The game ending forgets everything.
	b.ended("s1")
	if b.holds("s1", 4) {
		t.Fatal("a seat outlived its game")
	}
	if _, ok := b.view("s1", true); ok {
		t.Fatal("a game outlived itself")
	}
}

func TestSeatCodesCantBeGuessed(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	b := &seatBook{now: func() time.Time { return now }}
	b.started("s1", 1, "Willow", "", seatGame{})
	code, _, _ := b.invite("s1", 1)
	wrong := "BBBB"
	if wrong == code {
		wrong = "CCCC"
	}
	for range seatJoinTries {
		if _, err := b.join(wrong, 9, "Guesser"); err != errSeatCode {
			t.Fatalf("got %v", err)
		}
	}
	if _, err := b.join(code, 9, "Guesser"); err != errSeatTries {
		t.Fatalf("after %d wrong codes the right one still went through: %v", seatJoinTries, err)
	}
	// Someone else is unaffected, and the window passes.
	if _, err := b.join(code, 8, "Ann"); err != nil {
		t.Fatalf("another person was locked out: %v", err)
	}
	now = now.Add(seatJoinWindow + time.Second)
	code, _, _ = b.invite("s1", 1) // the old one has run out by now too
	if _, err := b.join(code, 9, "Guesser"); err != nil {
		t.Fatalf("still locked out after the window: %v", err)
	}
}

// Every seat taken: the next person can still watch.
func TestAFullGameCanStillBeWatched(t *testing.T) {
	b := &seatBook{}
	b.started("s1", 1, "Willow", "", seatGame{})
	code, _, _ := b.invite("s1", 1)
	for u := int64(2); u < 2+seatsMax-1; u++ {
		if j, _ := b.join(code, u, "P"); j.Seat < 1 {
			t.Fatalf("player %d got no seat", u)
		}
	}
	if j, err := b.join(code, 99, "Watcher"); err != nil || j.Seat != -1 {
		t.Fatalf("%+v %v", j, err)
	}
}

// Over stream version 2, someone who joined with a code drives the seat it gave them, and a
// watcher drives none, whatever their hello asked for.
func TestJoinedSeatsOverStreamVersion2(t *testing.T) {
	b := &seatBook{}
	b.started("s1", 1, "Willow", "", seatGame{})
	code, _, _ := b.invite("s1", 1)
	j, _ := b.join(code, 2, "Pat")
	if seat, ok := b.seatOf("s1", 2); !ok || seat != j.Seat || seat != 1 {
		t.Fatalf("seatOf = %d %v, joined as %d", seat, ok, j.Seat)
	}
	if _, ok := b.seatOf("s1", 3); ok {
		t.Fatal("someone who didn't join has a seat")
	}
	if _, ok := b.seatOf("s1", 1); ok {
		t.Fatal("the host is found as a joiner")
	}
	if got := joinedSeats(1); len(got) != 1 || got[0] != 1 {
		t.Fatalf("joinedSeats(1) = %v", got)
	}
	if got := joinedSeats(-1); got == nil || len(got) != 0 {
		t.Fatalf("joinedSeats(-1) = %#v: a watcher must name no seats, not leave them out", got)
	}
}
