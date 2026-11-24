// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"fmt"
	"testing"
	"time"
)

func TestAttemptsKeepEachDevicesTries(t *testing.T) {
	a := newAttempts()
	now := time.Unix(1_800_000_000, 0)
	a.now = func() time.Time { return now }

	a.note(refusal("dev1", "Phone", "invite", "expired", ""))
	if got := a.list("dev1"); len(got) != 1 || got[0].Step != "signin" || got[0].Code != "expired" || got[0].Reason == "" || got[0].Name != "Phone" {
		t.Fatalf("one refusal: %+v", got)
	}
	// The same again within a minute is one line, counted.
	now = now.Add(10 * time.Second)
	a.note(refusal("dev1", "", "invite", "expired", ""))
	if got := a.list("dev1"); len(got) != 1 || got[0].Count != 2 || got[0].At != now.Unix() {
		t.Fatalf("repeat: %+v", got)
	}
	// Something else is a line of its own, and keeps the name.
	now = now.Add(time.Second)
	a.note(Attempt{Device: "dev1", Kind: "connect", OK: true, Path: "direct"})
	got := a.list("dev1")
	if len(got) != 2 || !got[0].OK || got[0].Name != "Phone" || got[1].Code != "expired" {
		t.Fatalf("newest first: %+v", got)
	}
	// A device keeps its newest dozen.
	for i := 0; i < 30; i++ {
		now = now.Add(2 * time.Minute)
		a.note(refusal("dev2", "", "connect", "not-listed", ""))
	}
	if got := a.list("dev2"); len(got) != attemptsPerDevice {
		t.Fatalf("dev2 kept %d", len(got))
	}
	if all := a.list(""); len(all) != attemptsPerDevice+2 {
		t.Fatalf("all: %d", len(all))
	}
	// The quietest device goes when too many have tried.
	for i := 0; i < attemptDevices; i++ {
		now = now.Add(time.Second)
		a.note(refusal(fmt.Sprintf("x%d", i), "", "connect", "not-listed", ""))
	}
	if got := a.list("dev1"); len(got) != 0 {
		t.Fatalf("dev1 should have been forgotten first: %+v", got)
	}
	a.forget("dev2")
	if got := a.list("dev2"); len(got) != 0 {
		t.Fatalf("forget: %+v", got)
	}
	var none *attempts
	none.note(Attempt{Device: "d"})
	if len(none.list("")) != 0 {
		t.Fatal("nil history")
	}
}

func TestEnrolledFor(t *testing.T) {
	if enrolledFor("key", EnrolRequest{Device: "dev"}, false) != "key" || enrolledFor("key", EnrolRequest{Device: "dev"}, true) != "dev" || enrolledFor("key", EnrolRequest{}, true) != "key" {
		t.Fatal("enrolledFor")
	}
}
