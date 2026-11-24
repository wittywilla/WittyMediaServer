// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hls

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A shifted session (one viewer's start over) asks its feed for the moment Shift ago, and its
// program date-times say that moment, so the player knows where in the programme it is.
func TestShiftedLiveSessionPlaysBehindTheClock(t *testing.T) {
	m := liveTestManager(t)
	asked := make(chan time.Time, 4)
	shift := 40 * time.Minute
	s := startFake(t, m, LiveSpec{ID: "lib-1-so-u1-abc", Shift: shift, Feed: func(_ context.Context, at time.Time) (*LiveInput, error) {
		asked <- at
		return &LiveInput{InputArgs: SlateInput(), Slate: true, Duration: 60, AudioIndex: -1}, nil
	}}, fakeRun(3))
	var at time.Time
	select {
	case at = <-asked:
	case <-time.After(5 * time.Second):
		t.Fatal("the feed was never asked")
	}
	if d := time.Since(at) - shift; d < -2*time.Second || d > 2*time.Second {
		t.Fatalf("asked for %v ago, want %v", time.Since(at), shift)
	}
	deadline := time.Now().Add(5 * time.Second)
	for s.WindowSeconds() < 12 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	p := string(s.Playlist(func(f string) string { return f }))
	want := at.UTC().Format("2006-01-02T15:04")
	if !strings.Contains(p, "#EXT-X-PROGRAM-DATE-TIME:"+want) {
		t.Fatalf("the date-times should be the schedule's (%s…):\n%s", want, p)
	}
	// Hold-back is measured against the shifted clock too: a stream that is only on time for
	// its own clock isn't "ahead", so players start three segments back as usual.
	if !strings.Contains(p, "HOLD-BACK=12.000") {
		t.Fatalf("hold-back should be the usual three segments:\n%s", p)
	}
}

func TestUnshiftedClockIsTheWallClock(t *testing.T) {
	s := &LiveSession{}
	if d := time.Since(s.clock()); d < 0 || d > time.Second {
		t.Fatalf("clock off by %v", d)
	}
}
