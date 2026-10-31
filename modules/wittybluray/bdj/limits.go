package bdj

import (
	"fmt"
	"runtime"
	"time"
)

// A disc's code cannot be trusted to stop. Java's own limits cover its heap; these
// are the ones it cannot see or reach (native memory, threads, processor time),
// watched from this side of the pipe, where disc code has no way to interfere.
// Past any of them the process is ended and the disc plays from its playlists, as
// it would have without Java.

// Limits is what one disc's process may use.
type Limits struct {
	Memory  uint64        // all of it, native memory included
	Threads int           // a real disc's applications need a few dozen
	Cores   float64       // processor cores in constant use, over CPUWindow
	Window  time.Duration // how long Cores must last before it counts
	Every   time.Duration
}

// DefaultLimits leave room for any real disc's menus. A menu that busy-waits on
// one core is tolerated at its low priority; many cores at once for a minute is
// not a menu.
func DefaultLimits() Limits {
	cores := 2.0
	if n := float64(runtime.NumCPU()); n < cores {
		cores = n
	}
	return Limits{Memory: 768 << 20, Threads: 512, Cores: cores * 0.9, Window: time.Minute, Every: time.Second}
}

type procUsage struct {
	memory  uint64
	cpu     time.Duration
	threads int
}

// Usage is what the process holds now and the processor time it has had, for the
// diagnostic. ok is false where that cannot be read.
func (h *Host) Usage() (memory uint64, threads int, cpu time.Duration, ok bool) {
	u, ok := usage(h.cmd.Process.Pid)
	return u.memory, u.threads, u.cpu, ok
}

// watch ends the process when it passes a limit.
func (h *Host) watch(l Limits) {
	if l.Every <= 0 {
		return
	}
	pid := h.cmd.Process.Pid
	t := time.NewTicker(l.Every)
	defer t.Stop()
	type sample struct {
		at  time.Time
		cpu time.Duration
	}
	var hist []sample
	for {
		select {
		case <-t.C:
		case <-h.stop:
			return
		}
		u, ok := usage(pid)
		if !ok {
			continue
		}
		why := ""
		switch {
		case l.Memory > 0 && u.memory > l.Memory:
			why = fmt.Sprintf("it used %d MB of memory", u.memory>>20)
		case l.Threads > 0 && u.threads > l.Threads:
			why = fmt.Sprintf("it started %d threads", u.threads)
		}
		now := time.Now()
		hist = append(hist, sample{now, u.cpu})
		for len(hist) > 2 && now.Sub(hist[1].at) >= l.Window {
			hist = hist[1:]
		}
		if d := now.Sub(hist[0].at); why == "" && l.Cores > 0 && d >= l.Window {
			if used := float64(u.cpu-hist[0].cpu) / float64(d); used > l.Cores {
				why = fmt.Sprintf("it kept %.1f processor cores busy for %v", used, d.Round(time.Second))
			}
		}
		if why != "" {
			h.logf("ending the disc's Java process: " + why)
			if h.o.OnEvent != nil {
				h.o.OnEvent(Event{Kind: "limit", Args: []string{why}})
			}
			h.Close()
			_ = h.cmd.Process.Kill()
			return
		}
	}
}
