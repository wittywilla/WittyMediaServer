// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package syncplay

import (
	"math"
	"testing"
)

func TestClockOffset(t *testing.T) {
	for _, c := range []struct {
		name           string
		t0, t1, t2, t3 float64
		offset, rtt    float64
	}{
		// The server's clock is 5000 ms ahead; 40 ms each way; it took 10 ms to answer.
		{"even path", 1000, 6040, 6050, 1090, 5000, 80},
		// The client's clock is ahead of the server's.
		{"client ahead", 9000, 2030, 2030, 9060, -7000, 60},
		// Same clocks, nothing on the wire.
		{"no delay", 500, 500, 500, 500, 0, 0},
		// 100 ms out and 20 back: the sum can't know, and is out by half the difference.
		{"uneven path", 0, 100, 100, 120, 40, 120},
	} {
		o, r := ClockOffset(c.t0, c.t1, c.t2, c.t3)
		if o != c.offset || r != c.rtt {
			t.Errorf("%s: offset %v rtt %v, want %v and %v", c.name, o, r, c.offset, c.rtt)
		}
	}
}

func TestBestOffsetTrustsTheQuickestPing(t *testing.T) {
	// True offset 5000. The slow ping was held up on the way out and misleads by 200.
	samples := []Sample{
		{T0: 0, T1: 5450, T2: 5450, T3: 500},     // rtt 500, says 5200
		{T0: 1000, T1: 6020, T2: 6021, T3: 1041}, // rtt 40, says 5000
		{T0: 2000, T1: 7100, T2: 7100, T3: 2150}, // rtt 150, says 5025
		{T0: 3000, T1: 8000, T2: 8000, T3: 2990}, // came back before it left: ignored
		{T0: math.NaN(), T1: 1, T2: 1, T3: 1},
	}
	o, r, ok := BestOffset(samples)
	if !ok || o != 5000 || r != 40 {
		t.Fatalf("offset %v rtt %v ok %v", o, r, ok)
	}
	if _, _, ok := BestOffset(nil); ok {
		t.Fatal("an offset from no pings")
	}
	if _, _, ok := BestOffset(samples[3:]); ok {
		t.Fatal("an offset from pings that make no sense")
	}
}

// The same cases as web/src/player/sync.test.mjs: both must agree.
func TestCorrect(t *testing.T) {
	for _, c := range []struct {
		behind  float64
		nudging bool
		want    Step
	}{
		{0, false, Step{Rate: 1}},
		{0.1, false, Step{Rate: 1}},
		{-0.1, false, Step{Rate: 1}},
		{0.2, false, Step{Rate: 1.03}},
		{-0.2, false, Step{Rate: 0.97}},
		// Once nudging it carries on past "close enough to leave alone", to "settled".
		{0.1, true, Step{Rate: 1.03}},
		{-0.05, true, Step{Rate: 0.97}},
		{0.03, true, Step{Rate: 1}},
		{2.9, false, Step{Rate: 1.03}},
		{3, false, Step{Rate: 1, SeekBy: 3}},
		{-45, true, Step{Rate: 1, SeekBy: -45}},
	} {
		if got := Correct(c.behind, c.nudging); got != c.want {
			t.Errorf("behind %v nudging %v: %+v, want %+v", c.behind, c.nudging, got, c.want)
		}
	}
}

func TestPositionAt(t *testing.T) {
	s := State{PositionSec: 60, AtMs: 10_000, Rate: 1}
	for ms, want := range map[int64]float64{9_000: 60, 10_000: 60, 10_500: 60.5, 70_000: 120} {
		if got := s.PositionAt(ms); got != want {
			t.Errorf("at %d: %v, want %v", ms, got, want)
		}
	}
	s.Rate = 1.5
	if got := s.PositionAt(12_000); got != 63 {
		t.Error(got)
	}
	s.Paused = true
	if got := s.PositionAt(99_000); got != 60 {
		t.Error(got)
	}
	s.Paused, s.Waiting = false, true
	if got := s.PositionAt(99_000); got != 60 {
		t.Error(got)
	}
}
