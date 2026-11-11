// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package metrics

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestSeriesCountMatches(t *testing.T) {
	if len(SeriesNames) != seriesCount {
		t.Fatalf("%d names, room for %d", len(SeriesNames), seriesCount)
	}
}

func TestWatchStartsAndStops(t *testing.T) {
	c := New(nil, nil)
	before := runtime.NumGoroutine()
	r1 := c.Watch()
	r2 := c.Watch()
	if c.Watchers() != 2 {
		t.Fatal(c.Watchers())
	}
	_, ch := c.Latest()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("no sample while watched")
	}
	s, _ := c.Latest()
	if s.Time == 0 || s.Process == nil {
		t.Fatalf("%+v", s)
	}
	r1()
	r1() // twice is once
	if c.Watchers() != 1 {
		t.Fatal(c.Watchers())
	}
	r2()
	for i := 0; i < 100 && runtime.NumGoroutine() > before; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Fatalf("still sampling: %d goroutines, was %d", n, before)
	}
}

func TestReaderWorksOutSpeeds(t *testing.T) {
	var r reader
	started := time.Now()
	first := r.read(started, true)
	if first.Process == nil {
		t.Fatal("no process section")
	}
	if first.Process.CPUPercent != 0 {
		t.Error("the first reading has nothing to compare with")
	}
	if len(first.Volumes) == 0 {
		t.Error("a full reading has volumes")
	}
	time.Sleep(50 * time.Millisecond)
	for end := time.Now().Add(20 * time.Millisecond); time.Now().Before(end); { // a little work to see
		_ = make([]byte, 1024)
	}
	s := r.read(started, false)
	if len(s.Volumes) != 0 {
		t.Error("a quick reading leaves the disks alone")
	}
	if s.CPU != nil && (s.CPU.Percent < 0 || s.CPU.Percent > 100) {
		t.Errorf("cpu %v", s.CPU.Percent)
	}
}

func TestCounterGoingBackIsNotAHugeSpeed(t *testing.T) {
	if delta(5, 10) != 0 {
		t.Fatal("a reset counter")
	}
}

func TestRingKeepsTheLast(t *testing.T) {
	r := newRing(3)
	for i := int64(1); i <= 5; i++ {
		r.add(point{t: i})
	}
	got := r.since(0)
	if len(got) != 3 || got[0].t != 3 || got[2].t != 5 {
		t.Fatalf("%+v", got)
	}
	if got := r.since(5); len(got) != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestHourSummary(t *testing.T) {
	var h hourTally
	base := int64(3600 * 1000)
	p := func(at int64, v float32) point {
		x := point{t: at}
		x.v[0] = v
		return x
	}
	if h.add(p(base+60, 10)) != nil || h.add(p(base+120, 30)) != nil {
		t.Fatal("the hour isn't over")
	}
	done := h.add(p(base+3600, 5))
	if len(done) != len(SeriesNames) || done[0].Name != SeriesNames[0] || done[0].Avg != 20 || done[0].Peak != 30 || done[0].Hour != base {
		t.Fatalf("%+v", done)
	}
}

type memStore struct{ rows []HourRow }

func (m *memStore) SaveHour(_ context.Context, h int64, n string, a, p float64) error {
	m.rows = append(m.rows, HourRow{h, n, a, p})
	return nil
}
func (m *memStore) Hours(_ context.Context, from, to int64, names []string) ([]HourRow, error) {
	return m.rows, nil
}
func (m *memStore) Prune(context.Context, int64) error { return nil }

func TestReadChoosesTheRightSource(t *testing.T) {
	st := &memStore{rows: []HourRow{{Hour: 7200, Name: CPU, Avg: 12, Peak: 40}, {Hour: 10800, Name: CPU, Avg: 14, Peak: 50}}}
	c := New(st, func() Counts { return Counts{Streams: 2} })
	now := time.Now().Unix()
	for i := int64(0); i < 1000; i++ {
		p := point{t: now - 1000 + i}
		p.v[0] = float32(i % 10)
		c.low.add(p)
	}
	h, err := c.Read(context.Background(), 30*24*time.Hour, []string{CPU})
	if err != nil || h.Step != 3600 || len(h.Times) != 2 || h.Series[CPU][1] != 14 || h.Peak[CPU][1] != 50 {
		t.Fatalf("%+v %v", h, err)
	}
	h, err = c.Read(context.Background(), 24*time.Hour, []string{CPU})
	if err != nil || len(h.Times) > maxPoints+1 || len(h.Times) < 100 || h.Peak == nil {
		t.Fatalf("%d points, %v", len(h.Times), err)
	}
	if h.Peak[CPU][0] < 4 {
		t.Errorf("peak %v", h.Peak[CPU][0])
	}
	// Ten minutes with nobody having watched falls back to the minute samples.
	h, err = c.Read(context.Background(), 10*time.Minute, []string{CPU, "nonsense"})
	if err != nil || h.Step%60 != 0 || len(h.Times) == 0 {
		t.Fatalf("%+v %v", h, err)
	}
}

func BenchmarkAMinutesSample(b *testing.B) {
	var r reader
	started := time.Now()
	for b.Loop() {
		pointOf(r.read(started, false), Counts{})
	}
}
