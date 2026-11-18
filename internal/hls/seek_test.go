// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hls

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// seekStream is a long generated picture to seek around in, quick to convert.
func seekStream(t *testing.T) (*Manager, *Session, *lockedBuffer) {
	t.Helper()
	m, out := capturingManager(t, realFFmpeg(t))
	s := m.Ensure(Spec{ID: "v-seek", InputArgs: []string{"-f", "lavfi", "-i", "testsrc2=s=160x90:r=10"}, Duration: 3600, SegmentSec: 2,
		AudioIndex: -1, SubtitleIndex: -1, Preset: "ultrafast"})
	return m, s, out
}

func starts(out *lockedBuffer) int { return strings.Count(out.String(), `msg="transcode started"`) }

// Two requests for far apart places at once (a player whose older request wasn't cancelled in
// time): the newer one moves the stream, and the older one is told to ask again, not that the
// stream failed, which players count as an error and the viewer sees as one.
func TestAnOlderRequestGivesWayToANewerSeek(t *testing.T) {
	_, s, _ := seekStream(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var older error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _, older = s.Segment(ctx, 300) }()
	time.Sleep(300 * time.Millisecond)
	if _, err := s.Segment(ctx, 900); err != nil {
		t.Fatalf("the newer seek: %v", err)
	}
	wg.Wait()
	if older != nil && !errors.Is(older, ErrSuperseded) {
		t.Errorf("the older request failed as if the stream had: %v", older)
	}
}

// A burst of seeks whose requests are abandoned as the next one comes (as a player does) and a
// last one that waits: the last place plays, FFmpeg isn't started for every place passed over,
// and only one FFmpeg is left running.
func TestRapidSeeksEndWithOneRunAtTheLastPlace(t *testing.T) {
	m, s, out := seekStream(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := s.Segment(ctx, 0); err != nil {
		t.Fatal(err)
	}
	before := starts(out)
	var wg sync.WaitGroup
	for i := 1; i <= 8; i++ {
		c, stop := context.WithCancel(ctx)
		wg.Add(1)
		go func(n int) { defer wg.Done(); _, _ = s.Segment(c, n) }(i * 40)
		time.Sleep(30 * time.Millisecond)
		stop()
	}
	if _, err := s.Segment(ctx, 1000); err != nil {
		t.Fatalf("the last seek: %v", err)
	}
	wg.Wait()
	if got := starts(out) - before; got > 5 {
		t.Errorf("FFmpeg started %d times for one burst of seeks", got)
	}
	running := 0
	m.mu.Lock()
	for _, o := range m.session {
		if o.Active() {
			running++
		}
	}
	m.mu.Unlock()
	if running != 1 {
		t.Errorf("%d streams converting after the burst", running)
	}
	if !s.Active() {
		t.Error("nothing converting at the last place")
	}
}

// A restart waits for the FFmpeg it stopped to be gone before starting the next, so the two
// never write in the same folder at once.
func TestARestartWaitsForTheStoppedFFmpeg(t *testing.T) {
	_, s, _ := seekStream(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := s.Segment(ctx, 0); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	first := s.job
	s.mu.Unlock()
	if _, err := s.Segment(ctx, 500); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	second := s.job
	s.mu.Unlock()
	if second == first || !first.finished() {
		t.Fatal("the first run wasn't stopped")
	}
	if !first.stopped.Load() {
		t.Error("the first run wasn't marked as stopped on purpose")
	}
}

// A request waiting on a run that something stopped (a rest, or the limit on transcodes) with
// nothing replacing it starts one again rather than failing.
func TestAWaitingRequestOutlivesAStop(t *testing.T) {
	_, s, _ := seekStream(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := s.Segment(ctx, 700); done <- err }()
	deadline := time.Now().Add(30 * time.Second)
	for !s.Active() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	s.stop()
	if err := <-done; err != nil {
		t.Errorf("the request failed when its run was stopped: %v", err)
	}
}

// A request whose viewer has gone by the time the old run is stopped starts nothing.
func TestAnAbandonedSeekStartsNothing(t *testing.T) {
	_, s, out := seekStream(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.startFor(ctx, 400, 400); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v", err)
	}
	if starts(out) != 0 || s.Active() {
		t.Error("FFmpeg was started for a request nobody waits for")
	}
}
