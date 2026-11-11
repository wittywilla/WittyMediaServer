// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package pace

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	for _, c := range []struct {
		name     string
		cores    int
		memoryMB int64
		want     Class
	}{
		{"Raspberry Pi 3 (4 cores, 1 GB)", 4, 926, Tiny},
		{"Pi Zero 2 (4 cores, 512 MB)", 4, 427, Tiny},
		{"an old dual-core Celeron with 2 GB", 2, 1900, Tiny},
		{"a Celeron J1900 mini PC (4 cores, 4 GB)", 4, 3840, Small},
		{"Raspberry Pi 4 with 8 GB", 4, 7800, Small},
		{"a dual-core with 8 GB", 2, 7900, Small},
		{"a NAS that gives its container 1 GB", 8, 1024, Tiny},
		{"an N100 with 16 GB", 4, 15900, Small},
		{"a desktop (8 cores, 16 GB)", 8, 16000, Roomy},
		{"memory unknown, 4 cores", 4, 0, Small},
		{"memory unknown, 12 cores", 12, 0, Roomy},
		{"memory unknown, 1 core", 1, 0, Tiny},
	} {
		if got := Classify(c.cores, c.memoryMB); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAContainersLimitCounts(t *testing.T) {
	if got := machineMemoryMB(8192, 1<<30); got != 1024 {
		t.Errorf("8 GB with a 1 GB limit: %d MB", got)
	}
	if got := machineMemoryMB(2048, 0); got != 2048 {
		t.Errorf("no limit: %d MB", got)
	}
	if got := machineMemoryMB(2048, 64<<30); got != 2048 {
		t.Errorf("a limit above the machine: %d MB", got)
	}
	if got := machineMemoryMB(0, 512<<20); got != 512 {
		t.Errorf("memory unknown, with a limit: %d MB", got)
	}
}

func TestSlotsFitTheMachine(t *testing.T) {
	for _, c := range []struct {
		class       Class
		cores       int
		probes, thb int
	}{
		{Tiny, 4, 2, 1},
		{Tiny, 1, 2, 1},
		{Small, 2, 2, 1},
		{Small, 4, 4, 2},
		{Roomy, 8, 8, 3},
		{Roomy, 16, 16, 3},
	} {
		if p, th := slots(Probes, c.class, c.cores), slots(Thumbs, c.class, c.cores); p != c.probes || th != c.thb {
			t.Errorf("%v with %d cores: %d probes and %d thumbnails, want %d and %d", c.class, c.cores, p, th, c.probes, c.thb)
		}
	}
}

func TestTuningLeavesRoomForTheRest(t *testing.T) {
	const mb = 1 << 20
	for _, c := range []struct {
		class   Class
		memMB   int64
		limitMB int64
		gc      int
	}{
		{Tiny, 926, 231, 50}, // a Pi 3: a quarter
		{Tiny, 427, 128, 50}, // never less than the server needs to work at all
		{Tiny, 0, 192, 50},   // unknown
		{Small, 3840, 960, 75},
		{Small, 15900, 1024, 75}, // four slow cores with plenty of memory: capped
		{Roomy, 32000, 0, 0},
	} {
		limit, gc := tuning(c.class, c.memMB)
		if limit/mb != c.limitMB || gc != c.gc {
			t.Errorf("%v with %d MB: limit %d MB and GC %d, want %d and %d", c.class, c.memMB, limit/mb, gc, c.limitMB, c.gc)
		}
	}
}

func quick(t *testing.T, limit time.Duration) {
	t.Helper()
	wasPoll, wasMax := yieldPoll, yieldMax
	yieldPoll = 2 * time.Millisecond
	yieldMax = map[Class]time.Duration{Tiny: limit, Small: limit}
	t.Cleanup(func() { yieldPoll, yieldMax = wasPoll, wasMax; SetBusy(func() bool { return false }) })
}

func TestYieldWaitsForAStreamOnASmallMachine(t *testing.T) {
	quick(t, 5*time.Second)
	defer Force(Tiny)()
	var streaming atomic.Bool
	streaming.Store(true)
	SetBusy(streaming.Load)
	time.AfterFunc(60*time.Millisecond, func() { streaming.Store(false) })

	start := time.Now()
	if err := Yield(Background(context.Background())); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took < 50*time.Millisecond || took > 2*time.Second {
		t.Errorf("a scan waited %v for a stream that ended after 60 ms", took)
	}
	if n, waited := Stats(); n == 0 || waited <= 0 {
		t.Errorf("nothing was counted: %d, %v", n, waited)
	}
}

func TestYieldNeverHoldsAPersonUp(t *testing.T) {
	quick(t, 5*time.Second)
	defer Force(Tiny)()
	SetBusy(func() bool { return true })
	start := time.Now()
	if err := Yield(context.Background()); err != nil { // not marked as background
		t.Fatal(err)
	}
	if took := time.Since(start); took > 50*time.Millisecond {
		t.Errorf("someone's own request waited %v", took)
	}
}

func TestYieldLetsWorkTrickleThrough(t *testing.T) {
	quick(t, 40*time.Millisecond)
	defer Force(Small)()
	SetBusy(func() bool { return true }) // a film that never ends
	start := time.Now()
	if err := Yield(Background(context.Background())); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took < 30*time.Millisecond || took > 2*time.Second {
		t.Errorf("held for %v, with a limit of 40 ms", took)
	}
}

func TestYieldOnARoomyMachineDoesNothing(t *testing.T) {
	quick(t, 5*time.Second)
	defer Force(Roomy)()
	SetBusy(func() bool { return true })
	start := time.Now()
	if err := Yield(Background(context.Background())); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 50*time.Millisecond {
		t.Errorf("a roomy machine held a scan for %v", took)
	}
}

func TestYieldStopsWithItsContext(t *testing.T) {
	quick(t, 5*time.Second)
	defer Force(Tiny)()
	SetBusy(func() bool { return true })
	ctx, cancel := context.WithCancel(Background(context.Background()))
	time.AfterFunc(20*time.Millisecond, cancel)
	if err := Yield(ctx); err != context.Canceled {
		t.Errorf("a cancelled scan: %v", err)
	}
	if err := Yield(ctx); err != context.Canceled {
		t.Errorf("already cancelled: %v", err)
	}
}

func TestBackgroundMarksOnce(t *testing.T) {
	ctx := Background(context.Background())
	if !IsBackground(ctx) || IsBackground(context.Background()) {
		t.Error("the mark is wrong")
	}
	if Background(ctx) != ctx {
		t.Error("marked twice")
	}
}
