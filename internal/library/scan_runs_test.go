// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
	"github.com/wittywilla/wittymodule"
)

func tasksBy() tasks.By { return tasks.By{} }

func TestParseScanMode(t *testing.T) {
	for in, want := range map[string]ScanMode{"": ScanChanges, "changes": ScanChanges, "full": ScanFull, " full ": ScanFull} {
		if got, ok := ParseScanMode(in); !ok || got != want {
			t.Errorf("%q: %q %v", in, got, ok)
		}
	}
	for _, bad := range []string{"deep", "FULL", "extensive"} {
		if _, ok := ParseScanMode(bad); ok {
			t.Errorf("%q taken", bad)
		}
	}
}

// waitScan waits for a library's scan to end.
func waitScan(t *testing.T, s *Scanner, id int64) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for s.Running(id) {
		if time.Now().After(deadline) {
			t.Fatal("the scan didn't end")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// games writes n games and has the fake module list each as an item.
func (f *hookFixture) games(t *testing.T, n int) []string {
	t.Helper()
	var paths []string
	for i := range n {
		paths = append(paths, f.write(t, fmt.Sprintf("NES/game%03d.nes", i)))
	}
	f.mod.done = func(files []wittymodule.ScanFile) wittymodule.ScanIdentified {
		var out []wittymodule.ScanItem
		for _, file := range files {
			out = append(out, wittymodule.ScanItem{Files: []string{file.Path}, Kind: "game", Title: file.Path, Size: file.Size, MTime: file.MTime})
		}
		return wittymodule.ScanIdentified{Items: out}
	}
	return paths
}

func (f *hookFixture) described() (ids map[int64]int, refresh []bool) {
	f.mod.mu.Lock()
	defer f.mod.mu.Unlock()
	ids = map[int64]int{}
	for _, d := range f.mod.describe {
		refresh = append(refresh, d.Refresh)
		for _, it := range d.Items {
			ids[it.ID]++
		}
	}
	return ids, refresh
}

// A scan for changes describes only what's new; a Full rescan describes every item again, as
// looked up from the start, and is forgotten once it's done.
func TestFullRescanDescribesEveryItem(t *testing.T) {
	f := newHookFixture(t, 1)
	f.games(t, 120)
	if !f.s.StartBy(f.lib.ID, ScanChanges, tasksBy()) {
		t.Fatal("not started")
	}
	waitScan(t, f.s, f.lib.ID)
	ids, refresh := f.described()
	if len(ids) != 120 || len(refresh) != 3 || refresh[0] {
		t.Fatalf("first scan described %d items in %d calls (refresh %v)", len(ids), len(refresh), refresh)
	}
	f.mod.describe = nil
	f.s.StartBy(f.lib.ID, ScanChanges, tasksBy())
	waitScan(t, f.s, f.lib.ID)
	if ids, _ := f.described(); len(ids) != 0 {
		t.Fatalf("a scan for changes described %d unchanged items", len(ids))
	}
	f.s.StartBy(f.lib.ID, ScanFull, tasksBy())
	waitScan(t, f.s, f.lib.ID)
	ids, refresh = f.described()
	if len(ids) != 120 {
		t.Fatalf("a full rescan described %d of 120 items", len(ids))
	}
	for _, r := range refresh {
		if !r {
			t.Fatal("a full rescan's describe isn't from the start")
		}
	}
	if runs, _ := f.s.Store.ScanRuns(f.ctx); len(runs) != 0 {
		t.Errorf("finished runs kept: %+v", runs)
	}
	if left, _ := f.s.Store.toDescribe(f.ctx, f.lib.ID); len(left) != 0 {
		t.Errorf("%d items left to describe", len(left))
	}
}

// A Full rescan that a restart cuts short carries on at the next start: still a full one, and
// without describing again what it already had.
func TestFullRescanCarriesOnAfterARestart(t *testing.T) {
	f := newHookFixture(t, 1)
	f.games(t, 120)
	f.s.StartBy(f.lib.ID, ScanChanges, tasksBy())
	waitScan(t, f.s, f.lib.ID)
	f.mod.describe = nil

	// The server stops during the second batch of the full rescan's describe: the module's
	// service goes with it, and its call fails.
	f.mod.failDescribe = func(n int) error {
		if n == 2 {
			f.s.Stopping()
			return errors.New(`Post "http://127.0.0.1:65215/call/scan.describe": EOF`)
		}
		return nil
	}
	f.s.StartBy(f.lib.ID, ScanFull, tasksBy())
	waitScan(t, f.s, f.lib.ID)
	runs, _ := f.s.Store.ScanRuns(f.ctx)
	if len(runs) != 1 || runs[0].Mode != ScanFull || runs[0].LibraryID != f.lib.ID {
		t.Fatalf("the cut-short run wasn't kept: %+v", runs)
	}
	if lib, _ := f.s.Store.Library(f.ctx, f.lib.ID); lib.ScanMode != ScanFull || lib.ScanSince != runs[0].Since {
		t.Errorf("library says %q since %d", lib.ScanMode, lib.ScanSince)
	}
	f.mod.mu.Lock()
	firstBatch := f.mod.describe[0].Items
	f.mod.mu.Unlock()

	// The next start: a new scanner over the same database.
	s2 := NewScanner(f.s.Store, &ffmpeg.Tools{Encoders: map[string]bool{}}, t.TempDir(), func() string { return "" }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s2.ImagesDir, s2.slot, s2.Modules = f.s.ImagesDir, nil, f.mod
	f.mod.failDescribe = nil
	f.mod.mu.Lock()
	f.mod.describe = nil
	f.mod.mu.Unlock()
	if n := s2.ResumeScans(f.ctx); n != 1 {
		t.Fatalf("%d scans carried on", n)
	}
	if p := s2.Snapshot(f.lib.ID); p != nil && (p.Mode != ScanFull || !p.Resumed) {
		t.Errorf("progress %+v", p)
	}
	waitScan(t, s2, f.lib.ID)
	second, refresh := f.described()
	if len(second) != 120-50 {
		t.Errorf("carried on with %d items to describe, not the 70 left", len(second))
	}
	for _, it := range firstBatch {
		if second[it.ID] > 0 {
			t.Errorf("item %d, described before the restart, described again", it.ID)
		}
	}
	for _, r := range refresh {
		if !r {
			t.Error("the carried-on run isn't a full one")
		}
	}
	if runs, _ := s2.Store.ScanRuns(f.ctx); len(runs) != 0 {
		t.Errorf("finished run kept: %+v", runs)
	}
}

// A scan whose module stops answering is stopped as stuck, saying what it was waiting for; one
// whose module keeps saying how far it's got runs as long as it takes.
func TestStuckScanStopsAndReportsKeepOneGoing(t *testing.T) {
	was, wasCheck := StuckAfter, stuckCheck
	StuckAfter, stuckCheck = 300*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { StuckAfter, stuckCheck = was, wasCheck })

	f := newHookFixture(t, 1)
	f.games(t, 3)
	var cause error
	stuck := make(chan struct{})
	blockDone := func(report bool) func(ctx context.Context, a wittymodule.ScanIdentifyArgs) {
		return func(ctx context.Context, a wittymodule.ScanIdentifyArgs) {
			if !a.Done {
				return
			}
			until := time.After(4 * StuckAfter)
			for {
				select {
				case <-ctx.Done():
					cause = context.Cause(ctx)
					close(stuck)
					return
				case <-until:
					return
				case <-time.After(StuckAfter / 4):
					if report && !f.s.ModuleReport("fake", f.lib.ID, 1, 3) {
						t.Error("a report while the module's call is under way wasn't taken")
					}
				}
			}
		}
	}
	f.mod.during = blockDone(false)
	f.s.StartBy(f.lib.ID, ScanChanges, tasksBy())
	select {
	case <-stuck:
	case <-time.After(10 * time.Second):
		t.Fatal("a scan nothing moved in wasn't stopped")
	}
	waitScan(t, f.s, f.lib.ID)
	if !errors.Is(cause, ErrScanStuck) {
		t.Fatalf("stopped with %v", cause)
	}
	if runs, _ := f.s.Store.ScanRuns(f.ctx); len(runs) != 0 {
		t.Errorf("a stuck run is kept to carry on: %+v", runs)
	}

	// Reports from the module keep it going past StuckAfter, to the end.
	f.games(t, 3)
	f.mod.during = blockDone(true)
	started := time.Now()
	f.s.StartBy(f.lib.ID, ScanChanges, tasksBy())
	waitScan(t, f.s, f.lib.ID)
	if took := time.Since(started); took < 3*StuckAfter {
		t.Fatalf("ended after %v", took)
	}
	if lib, _ := f.s.Store.Library(f.ctx, f.lib.ID); lib.ItemCount != 3 {
		t.Errorf("%d items after a long scan", lib.ItemCount)
	}
	if f.s.ModuleReport("fake", f.lib.ID, 1, 3) {
		t.Error("a report with no scan running was taken")
	}
}

// A Full rescan reads again every file it hasn't read since it began; a scan for changes only
// what changed.
func TestFullRescanRereadsWhatItHasNotRead(t *testing.T) {
	since := int64(1000)
	known := &knownFile{mtime: 5, size: 10, updated: 900}
	c := candidate{mtime: 5, size: 10, known: known}
	if c.unchanged() == nil || !c.same(&Item{MTime: 5, SizeBytes: 10, UpdatedAt: 900}, 5) {
		t.Fatal("an unchanged file is read again by a scan for changes")
	}
	c.reread = since
	if c.unchanged() != nil || c.same(&Item{MTime: 5, SizeBytes: 10, UpdatedAt: 900}, 5) {
		t.Fatal("a full rescan doesn't read again a file it hasn't read")
	}
	known.updated = 1000
	if c.unchanged() == nil || !c.same(&Item{MTime: 5, SizeBytes: 10, UpdatedAt: 1001}, 5) {
		t.Fatal("a full rescan reads again what it has read already")
	}
	c.mtime = 6
	if c.unchanged() != nil || c.same(&Item{MTime: 5, SizeBytes: 10, UpdatedAt: 1001}, 6) {
		t.Fatal("a changed file isn't read again")
	}
}
