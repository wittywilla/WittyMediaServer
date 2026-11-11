// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package pace fits the server to the machine it runs on, so a small one (a
// Raspberry Pi 3, an old Celeron mini PC) stays responsive. It does three things:
//
//   - says how small the machine is (Machine), from its processors and memory;
//   - tells Go's memory manager how much the machine can spare (Tune), so the
//     server's own memory stays well inside it;
//   - holds background work back while someone is watching (Yield), and says how
//     much of it may run at once (Slots).
//
// Work that nobody is waiting for marks its context with Background. Everything
// else is left alone: a person pressing play never waits here.
package pace

import (
	"context"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

// Class is how much machine there is.
type Class int

const (
	// Tiny: a Raspberry Pi 3 or the like. Up to 1.5 GB of memory, or one or two processors
	// with up to 2 GB.
	Tiny Class = iota
	// Small: an old Celeron or Atom mini PC, a Pi 4 with 2 or 4 GB. Up to four processors
	// or up to 4 GB.
	Small
	// Roomy: anything larger. Nothing is held back.
	Roomy
)

func (c Class) String() string {
	switch c {
	case Tiny:
		return "tiny"
	case Small:
		return "small"
	}
	return "roomy"
}

// Classify says what class a machine with these processors and this much memory is.
// Memory of 0 is "not known", and then the processors alone decide.
func Classify(cores int, memoryMB int64) Class {
	known := memoryMB > 0
	switch {
	case known && memoryMB <= 1536, known && memoryMB <= 2048 && cores <= 2, cores <= 1:
		return Tiny
	case known && memoryMB <= 4096, cores <= 4:
		return Small
	}
	return Roomy
}

var (
	machineOnce sync.Once
	machine     Class
	memoryMB    int64
	forced      atomic.Int32 // a class + 1 set by tests, 0 for none
)

// Machine is this machine's class, worked out once. WMS_MACHINE=tiny, small or roomy
// overrides it, for a machine this reads wrongly and for trying the small settings
// on a large one.
func Machine() Class {
	if f := forced.Load(); f != 0 {
		return Class(f - 1)
	}
	machineOnce.Do(func() {
		memoryMB = machineMemoryMB(sysinfo.MemoryMB(), containerLimit())
		machine = Classify(runtime.NumCPU(), memoryMB)
		switch strings.ToLower(strings.TrimSpace(os.Getenv("WMS_MACHINE"))) {
		case "tiny":
			machine = Tiny
		case "small":
			machine = Small
		case "roomy", "large":
			machine = Roomy
		}
	})
	return machine
}

// machineMemoryMB is the smaller of the machine's memory and the limit its container
// was given: a NAS with 8 GB that gives this container 1 GB is a 1 GB machine.
func machineMemoryMB(totalMB, limitBytes int64) int64 {
	if limitMB := limitBytes >> 20; limitMB > 0 && (totalMB == 0 || limitMB < totalMB) {
		return limitMB
	}
	return totalMB
}

// containerLimit reads the memory limit Linux control groups put on this process
// (Docker's --memory, a systemd MemoryMax), 0 for none or anywhere else.
func containerLimit() int64 {
	for _, p := range []string{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		// "max", or version 1's way of saying none: a number near the largest there is.
		if err != nil || n <= 0 || n >= 1<<60 {
			return 0
		}
		return n
	}
	return 0
}

// Force sets the class, for tests, and returns what puts it back.
func Force(c Class) (restore func()) {
	was := forced.Swap(int32(c) + 1)
	return func() { forced.Store(was) }
}

// Kind is a sort of work that is limited to so many at once.
type Kind int

const (
	// Probes: ffprobe reading a file's details.
	Probes Kind = iota
	// Thumbs: FFmpeg making a small picture.
	Thumbs
)

// Slots is how many of a kind may run at once on this machine. Each is a process
// of its own, with tens of megabytes of memory and a processor to itself for as
// long as it runs.
func Slots(k Kind) int { return slots(k, Machine(), runtime.NumCPU()) }

func slots(k Kind, c Class, cores int) int {
	switch k {
	case Probes:
		switch c {
		case Tiny:
			return 2
		case Small:
			return max(2, min(cores, 4))
		}
		return max(4, cores)
	case Thumbs:
		switch c {
		case Tiny:
			return 1
		case Small:
			return max(1, min(cores/2, 2))
		}
		return max(1, min(cores/2, 3))
	}
	return 1
}

// Tuning is what Tune did, for the log.
type Tuning struct {
	Class       Class
	Cores       int
	MemoryMB    int64
	MemoryLimit int64 // bytes given to Go as its soft limit, 0 when left alone
	GCPercent   int   // 0 when left alone
}

// Tune tells Go's memory manager how much this machine can spare. Without it Go
// lets the heap double before collecting, whatever the machine holds: on 1 GB,
// beside FFmpeg, that is what gets a server killed for memory. With a soft limit
// it collects harder as it nears the limit instead. Whoever set GOMEMLIMIT or
// GOGC themselves keeps what they set.
func Tune() Tuning {
	c := Machine()
	t := Tuning{Class: c, Cores: runtime.NumCPU(), MemoryMB: memoryMB}
	limit, percent := tuning(c, memoryMB)
	if limit > 0 && os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(limit)
		t.MemoryLimit = limit
	}
	if percent > 0 && os.Getenv("GOGC") == "" {
		debug.SetGCPercent(percent)
		t.GCPercent = percent
	}
	return t
}

// tuning: the soft limit is a share of the machine's memory, leaving the rest to
// FFmpeg, the system's file cache (which is what makes playing a file smooth) and
// everything else that runs there. A roomy machine gets no limit.
func tuning(c Class, memMB int64) (limit int64, gcPercent int) {
	const mb = 1 << 20
	switch c {
	case Tiny:
		limit, gcPercent = 192*mb, 50
		if memMB > 0 {
			limit = min(max(memMB*mb/4, 128*mb), 384*mb)
		}
	case Small:
		limit, gcPercent = 512*mb, 75
		if memMB > 0 {
			limit = min(max(memMB*mb/4, 256*mb), 1024*mb)
		}
	}
	return limit, gcPercent
}

type backgroundKey struct{}

// Background marks a context as work nobody is waiting for: a scan, a refresh, a
// tidy-up. Such work yields to people (Yield) and may be started at a lower priority.
func Background(ctx context.Context) context.Context {
	if IsBackground(ctx) {
		return ctx
	}
	return context.WithValue(ctx, backgroundKey{}, true)
}

// IsBackground says whether a context was marked with Background.
func IsBackground(ctx context.Context) bool {
	b, _ := ctx.Value(backgroundKey{}).(bool)
	return b
}

// busy says whether someone is being served something that needs the processor
// right now: a stream being converted.
var busy atomic.Pointer[func() bool]

// SetBusy gives the question Yield asks. The server sets it once at start.
func SetBusy(f func() bool) { busy.Store(&f) }

// Busy says whether the machine is serving someone work that background jobs
// should give way to.
func Busy() bool {
	f := busy.Load()
	return f != nil && (*f)()
}

// How often Yield looks again, and the longest it holds one piece of work back.
// A variable, for the tests.
var (
	yieldPoll = 500 * time.Millisecond
	yieldMax  = map[Class]time.Duration{Tiny: 20 * time.Second, Small: 5 * time.Second}
)

// yields counts the pieces of background work held back, and for how long, for
// anyone asking why a scan is slow (Stats).
var yields, yieldNanos atomic.Int64

// Yield is called by background work before each piece of it: a file to probe, a
// picture to fetch. On a small machine it waits while someone's stream is being
// converted, so the scan doesn't take the processor a film needs. It never waits
// for ever: after a while one piece goes through, so a scan started during a long
// film still finishes, slowly. Foreground work and roomy machines return at once.
// The only error is the context's own.
func Yield(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !IsBackground(ctx) {
		return nil
	}
	limit, held := yieldMax[Machine()]
	if !held || !Busy() {
		return nil
	}
	start := time.Now()
	defer func() {
		yields.Add(1)
		yieldNanos.Add(int64(time.Since(start)))
	}()
	t := time.NewTicker(yieldPoll)
	defer t.Stop()
	for time.Since(start) < limit {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
		if !Busy() {
			return nil
		}
	}
	return nil
}

// Stats is how often background work has been held back since the server started,
// and for how long in all.
func Stats() (times int64, waited time.Duration) {
	return yields.Load(), time.Duration(yieldNanos.Load())
}
