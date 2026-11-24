// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hls

import (
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// The lead is counted in the player's time: at 2× the stream rests twice as far ahead, and a
// resting stream starts again twice as far ahead, so the player never comes nearer than the
// same number of seconds of waiting, whatever its rate.
func TestLeadGrowsWithTheRate(t *testing.T) {
	m := testManager(t, 4)
	s := m.Ensure(Spec{ID: "v-lead", Duration: 3600, SegmentSec: 4, AudioIndex: -1, SubtitleIndex: -1})
	if s.Rate() != 1 || s.wakeAhead() != wakeSegs || s.restAhead() != maxAheadSegs {
		t.Fatalf("at 1×: rate %v, wake %d, rest %d", s.Rate(), s.wakeAhead(), s.restAhead())
	}
	s.SetRate(2)
	if s.wakeAhead() != 2*wakeSegs || s.restAhead()-s.wakeAhead() != maxAheadSegs-wakeSegs {
		t.Fatalf("at 2×: wake %d, rest %d", s.wakeAhead(), s.restAhead())
	}
	s.SetRate(0.5) // slower keeps what 1× keeps
	if s.wakeAhead() != wakeSegs {
		t.Fatalf("at 0.5×: wake %d", s.wakeAhead())
	}
	s.SetRate(0) // a paused player says nothing about its rate
	s.SetRate(-3)
	if s.Rate() != 0.5 {
		t.Fatalf("0 and less are ignored: %v", s.Rate())
	}
	s.SetRate(400)
	if s.Rate() != maxRate {
		t.Fatalf("clamped: %v", s.Rate())
	}
	// A new stream starts at the rate its player asked with.
	if s2 := m.Ensure(Spec{ID: "v-lead2", Duration: 60, SegmentSec: 4, Rate: 3}); s2.Rate() != 3 || s2.wakeAhead() != 3*wakeSegs {
		t.Fatalf("asked at 3×: %v, wake %d", s2.Rate(), s2.wakeAhead())
	}
}

// The rungs of a ladder share a player, and so its rate.
func TestRateReachesTheLadder(t *testing.T) {
	m := testManager(t, 4)
	a := m.Ensure(Spec{ID: "v-r-720", Group: "g", Duration: 60, SegmentSec: 2})
	b := m.Ensure(Spec{ID: "v-r-480", Group: "g", Duration: 60, SegmentSec: 2})
	other := m.Ensure(Spec{ID: "v-x", Duration: 60, SegmentSec: 2})
	a.SetRate(1.5)
	if b.Rate() != 1.5 || other.Rate() != 1 {
		t.Fatalf("rung %v, other stream %v", b.Rate(), other.Rate())
	}
}

// A remux reads ahead at four times real time, or twice the player's rate when that is more.
func TestRemuxReadsAheadOfAFastPlayer(t *testing.T) {
	tools := &ffmpeg.Tools{Encoders: map[string]bool{"libx264": true}}
	s := &Session{Spec: Spec{InputArgs: []string{"-i", "in.mkv"}, Duration: 60, SegmentSec: 4, CopyVideo: true, Growing: true, SubtitleIndex: -1}, Dir: t.TempDir(), tools: tools, m: &Manager{tools: tools}}
	hasBurst = false
	readrateOnce.Do(func() {})
	line := func() string { args, _, _, _ := s.commandLine(0); return strings.Join(args, " ") }
	if !strings.Contains(line(), "-readrate 4 ") {
		t.Fatalf("1×: %s", line())
	}
	s.rate.Store(0)
	s.setRate(3)
	if !strings.Contains(line(), "-readrate 6 ") {
		t.Fatalf("3×: %s", line())
	}
}

// A job's speed is film seconds made per second, over the last window, and starts counting once
// FFmpeg has settled.
func TestPaceMeasuresSpeed(t *testing.T) {
	t0 := time.Now()
	var p pace
	p.started = t0
	if got := p.note(t0.Add(2*time.Second), 3, 4); got != 0 {
		t.Fatalf("before it settled: %v", got)
	}
	// From settling on, one segment of 4 s every 5 s: 0.8×.
	last := int64(3)
	var got float64
	for at := paceSettle; at <= paceSettle+30*time.Second; at += 250 * time.Millisecond {
		if (at-paceSettle)%(5*time.Second) == 0 {
			last++
		}
		got = p.note(t0.Add(at), last, 4)
	}
	if got < 0.7 || got > 0.9 {
		t.Fatalf("one 4 s segment every 5 s: %v, want about 0.8", got)
	}
	// Nothing at all for a whole window: slower than one segment a window.
	var q pace
	q.started = t0
	q.note(t0.Add(paceSettle), 0, 4)
	if got := q.note(t0.Add(paceSettle+paceWindow+time.Second), 0, 4); got == 0 || got > 4/paceWindow.Seconds() {
		t.Fatalf("stuck: %v", got)
	}
}

// A job slower than the player, with the player close behind, is told about once and then, after
// half a window more, restarted with a faster libx264 preset. Not twice, and not for a job that
// rests far ahead.
func TestFallingBehindGoesFaster(t *testing.T) {
	m := testManager(t, 4)
	m.tools = &ffmpeg.Tools{Encoders: map[string]bool{"libx264": true}}
	s := m.Ensure(Spec{ID: "v-slow", Duration: 600, SegmentSec: 4, AudioIndex: -1, SubtitleIndex: -1})
	s.SetRate(2)
	j := &job{startSeg: 0, done: make(chan struct{})}
	t0 := time.Now().Add(-time.Hour)
	j.pace.started = t0
	s.lastAsked.Store(10)
	j.lastSeg.Store(11)
	j.pace.note(t0.Add(paceSettle), 11, 4)
	j.lastSeg.Store(14) // 3 segments (12 s) in 20 s: 0.6×, at 2×
	now := t0.Add(paceSettle + 20*time.Second)
	s.checkPace(j, now)
	if j.slowSince.IsZero() || s.paceStep.Load() != 0 {
		t.Fatalf("the first look only notes it: since %v, step %d", j.slowSince, s.paceStep.Load())
	}
	s.checkPace(j, now.Add(paceWindow/2+time.Second))
	if s.paceStep.Load() != 1 {
		t.Fatal("still behind half a window later: no faster setting")
	}
	if got := s.preset(); got != "ultrafast" {
		t.Fatalf("veryfast two steps faster: %q", got)
	}
	if p := s.Pace(); !p.Fastest || p.Rate != 2 {
		t.Fatalf("%+v", p)
	}
	s.checkPace(j, now.Add(paceWindow*2))
	if s.paceStep.Load() != 1 {
		t.Fatal("only one faster setting")
	}

	// Far ahead: however slow, nothing to do.
	s2 := m.Ensure(Spec{ID: "v-ahead", Duration: 600, SegmentSec: 4, AudioIndex: -1, SubtitleIndex: -1})
	j2 := &job{done: make(chan struct{})}
	j2.pace.started = t0
	s2.lastAsked.Store(0)
	j2.lastSeg.Store(40)
	j2.pace.note(t0.Add(paceSettle), 39, 4)
	s2.checkPace(j2, now)
	s2.checkPace(j2, now.Add(paceWindow))
	if !j2.slowSince.IsZero() || s2.paceStep.Load() != 0 {
		t.Fatal("a stream far ahead of its player is not behind")
	}
}

func TestFasterPreset(t *testing.T) {
	for in, want := range map[string]string{"": "ultrafast", "veryfast": "ultrafast", "medium": "faster", "superfast": "ultrafast", "ultrafast": "ultrafast", "odd": "odd"} {
		if got := fasterPreset(in, 2); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// On a small machine the processor converts 4K to 1080p at most: at full size it was far slower
// than the film and outgrew a 1 GB container. A roomy machine, a copy, a picture that is already
// small enough and a size the viewer chose below 1080 are left as they are.
func TestSmallMachineConvertsTo1080(t *testing.T) {
	m := testManager(t, 4)
	m.tools = &ffmpeg.Tools{Encoders: map[string]bool{"libx264": true}}
	defer func(f func() bool) { smallMachine = f }(smallMachine)
	smallMachine = func() bool { return true }
	sp := Spec{Height: 2160, Duration: 60, SegmentSec: 4}
	if !m.CapForMachine(&sp) || sp.MaxHeight != 1080 {
		t.Fatalf("4K on a small machine: %d", sp.MaxHeight)
	}
	for name, sp := range map[string]Spec{
		"copy":     {Height: 2160, CopyVideo: true},
		"1080p":    {Height: 1080},
		"wide 4K":  {Height: 1600, MaxHeight: 720},
		"no video": {Height: 2160, NoVideo: true},
	} {
		if m.CapForMachine(&sp) {
			t.Errorf("%s: capped", name)
		}
	}
	smallMachine = func() bool { return false }
	sp = Spec{Height: 2160}
	if m.CapForMachine(&sp) || sp.MaxHeight != 0 {
		t.Fatal("a roomy machine keeps 4K")
	}
}

// On a small machine a fast player, and every stream once one here needed it, start with the faster
// setting: going faster half-way stalled the player while the new FFmpeg started.
func TestSmallMachineStartsFaster(t *testing.T) {
	m := testManager(t, 4)
	m.tools = &ffmpeg.Tools{Encoders: map[string]bool{"libx264": true}}
	defer func(f func() bool) { smallMachine = f }(smallMachine)
	smallMachine = func() bool { return true }
	s := m.Ensure(Spec{ID: "v-a", Duration: 60, SegmentSec: 4})
	if s.preset() != "" || s.Pace().Fastest {
		t.Fatalf("1×: %q", s.preset())
	}
	s.SetRate(2)
	if s.preset() != "ultrafast" || !s.Pace().Fastest {
		t.Fatalf("2× on a small machine: %q", s.preset())
	}
	b := m.Ensure(Spec{ID: "v-b", Duration: 60, SegmentSec: 4})
	if b.preset() != "" {
		t.Fatal("another stream at 1× is left alone until one needed it")
	}
	m.neededFaster.Store(true)
	if b.preset() != "ultrafast" {
		t.Fatal("once one stream needed it, the next starts with it")
	}
	smallMachine = func() bool { return false }
	if s.preset() != "" || b.preset() != "" {
		t.Fatal("a roomy machine goes faster only when a stream falls behind")
	}
}
