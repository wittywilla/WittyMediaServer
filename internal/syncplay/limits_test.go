// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package syncplay

import (
	"errors"
	"testing"
)

// A member whose screen time is over sits out: the room doesn't wait for them, they can't
// steer it and aren't told what it plays. They keep the seat, and are back with more time.
func TestAMemberWhoseTimeIsUpSitsOut(t *testing.T) {
	w := newWorld(t, nil)
	host, kid := person(1), person(2)
	room := w.create(host, 10)
	everyone := PolicyEveryone
	if err := w.e.Set(room.ID, room.You, 1, Settings{Policy: &everyone}); err != nil {
		t.Fatal(err)
	}
	seat, err := w.e.Join(kid, room.Code)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.e.Report(room.ID, room.You, 1, ReportReady, room.State.Rev); err != nil {
		t.Fatal(err)
	}
	// Playing is asked for; the child's player never says it is ready.
	v, err := w.e.Do(room.ID, room.You, 1, Command{Type: CmdPlay})
	if err != nil || !v.State.Waiting {
		t.Fatalf("the room should be waiting for the child: %+v %v", v.State, err)
	}
	w.e.SetTimeUp(2, true)
	v, _ = w.e.View(room.ID, room.You, 1)
	if v.State.Waiting || v.State.Paused {
		t.Fatalf("the room still waits for a member whose time is up: %+v", v.State)
	}
	state := ""
	for _, m := range v.Members {
		if m.UserID == 2 {
			state = m.State
		}
	}
	if state != MemberTimeUp {
		t.Errorf("the others see the member as %q", state)
	}
	mine, _ := w.e.View(seat.ID, seat.You, 2)
	if !mine.TimeUp || mine.CanControl || mine.State.ItemID != 0 {
		t.Errorf("what the member is told: %+v", mine)
	}
	if _, err := w.e.Do(seat.ID, seat.You, 2, Command{Type: CmdPause}); !errors.Is(err, ErrTimeUp) {
		t.Errorf("a member whose time is up paused the room: %v", err)
	}
	// Saying it again changes nothing, and tells nobody.
	rev := v.State.Rev
	w.e.SetTimeUp(2, true)
	if again, _ := w.e.View(room.ID, room.You, 1); again.State.Rev != rev {
		t.Error("the room changed when nothing did")
	}
	w.e.SetTimeUp(2, false)
	mine, _ = w.e.View(seat.ID, seat.You, 2)
	if mine.TimeUp || !mine.CanControl || mine.State.ItemID != 10 {
		t.Errorf("with more time: %+v", mine)
	}
	// Back in a room that is playing: it catches up without stopping the others.
	if v, _ := w.e.View(room.ID, room.You, 1); v.State.Waiting {
		t.Error("a member coming back stopped the room")
	}
	if got := w.e.Seated(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("who is seated: %v", got)
	}
}

// Recheck asks again what a member may see; DropSeats and Uninvite take seats and
// invitations away.
func TestRecheckDropSeatsAndUninvite(t *testing.T) {
	w := newWorld(t, nil)
	host, guest := person(1), User{ID: 2, Name: "p2", Seat: "phone:a"}
	room := w.create(host, 10, 11)
	seat, err := w.e.Join(guest, room.Code)
	if err != nil {
		t.Fatal(err)
	}
	// The guest's limits change: what is playing is no longer theirs to see.
	w.hide(2, 10)
	w.e.Recheck(2)
	mine, _ := w.e.View(seat.ID, seat.You, 2)
	if !mine.Blocked || mine.State.ItemID != 0 || mine.Queue[0] != 0 || mine.Queue[1] != 11 {
		t.Fatalf("after the limits changed: %+v", mine)
	}
	w.e.Recheck(99) // nobody's seat: nothing happens
	// A second seat of the same account on another sign-in; only the first sign-in ends.
	tv, err := w.e.Join(User{ID: 2, Name: "p2", Seat: "tv:b"}, room.ID)
	if err == nil {
		t.Fatalf("joined by the room's id without an invitation: %+v", tv)
	}
	if err := w.e.Invite(room.ID, room.You, 1, []int64{2, 3}); err != nil {
		t.Fatal(err)
	}
	w.mu.Lock()
	delete(w.hidden, 2)
	w.mu.Unlock()
	tv, err = w.e.Join(User{ID: 2, Name: "p2", Seat: "tv:b"}, room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n := w.e.DropSeats(func(u User) bool { return u.ID == 2 && u.Seat == "phone:a" }); n != 1 {
		t.Fatalf("%d seats dropped", n)
	}
	if _, err := w.e.View(seat.ID, seat.You, 2); !errors.Is(err, ErrNotMember) {
		t.Errorf("the dropped seat: %v", err)
	}
	if _, err := w.e.View(tv.ID, tv.You, 2); err != nil {
		t.Errorf("the seat of the sign-in that didn't end: %v", err)
	}
	if got := w.e.RoomsFor(User{ID: 3}); len(got) != 1 || !got[0].Invited {
		t.Fatalf("the invitation: %+v", got)
	}
	w.e.Uninvite(3)
	if got := w.e.RoomsFor(User{ID: 3}); len(got) != 0 {
		t.Errorf("after the invitation was taken back: %+v", got)
	}
}
