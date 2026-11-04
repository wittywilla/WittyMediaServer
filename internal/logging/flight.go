package logging

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"runtime/trace"
	"sync"
	"sync/atomic"
	"time"
)

// The flight recorder keeps the last minutes of everything, at the most detailed level,
// whatever the owner's levels say, in memory only. Nothing of it is written anywhere
// unless something goes wrong: a hang (see hang.go), a caught panic, or an admin
// pressing Save what just happened. Then the minutes before it are there to read, which
// a log kept at its everyday levels can't give: a song playing and its lyrics loading
// leave no line at warn.
//
// What it costs: a debug line that the levels would have thrown away is now built,
// cleaned and kept for a while. Bounded by size and age; the oldest go first.

// Limits. Variables so tests can shrink them.
var (
	flightAge     = 10 * time.Minute
	flightBytes   = 16 << 20
	flightEntries = 1 << 17
)

type flightRing struct {
	on    atomic.Bool
	mu    sync.Mutex
	buf   []*Record
	start int
	n     int
	bytes int
}

func (f *flightRing) setOn(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.on.Store(on)
	if !on {
		f.buf, f.start, f.n, f.bytes = nil, 0, 0, 0
	}
}

func (f *flightRing) add(rec *Record) {
	if rec.Level == "" {
		rec.Level = LevelName(rec.Lv)
	}
	sz := rec.size()
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.on.Load() {
		return
	}
	if f.buf == nil {
		f.buf = make([]*Record, flightEntries)
	}
	cutoff := rec.Time.Add(-flightAge)
	for f.n > 0 && (f.n == len(f.buf) || f.bytes+sz > flightBytes || f.buf[f.start].Time.Before(cutoff)) {
		f.dropOldest()
	}
	f.buf[(f.start+f.n)%len(f.buf)] = rec
	f.n++
	f.bytes += sz
}

func (f *flightRing) dropOldest() {
	old := f.buf[f.start]
	f.buf[f.start] = nil
	f.start = (f.start + 1) % len(f.buf)
	f.n--
	f.bytes -= old.size()
}

func (f *flightRing) snapshot() []*Record {
	f.mu.Lock()
	defer f.mu.Unlock()
	cutoff := time.Now().Add(-flightAge)
	out := make([]*Record, 0, f.n)
	for i := 0; i < f.n; i++ {
		if r := f.buf[(f.start+i)%len(f.buf)]; !r.Time.Before(cutoff) {
			out = append(out, r)
		}
	}
	return out
}

// SetFlightRecorder turns the flight recorder on or off. Off forgets what it held.
func (l *Logger) SetFlightRecorder(on bool) { l.flight.setOn(on) }

// FlightRecorderOn says whether the flight recorder is keeping the last minutes.
func (l *Logger) FlightRecorderOn() bool { return l.flight.on.Load() }

// WriteFlight writes what the flight recorder holds, oldest first, as lines of a log
// file, and says how many.
func (l *Logger) WriteFlight(w io.Writer) (int, error) {
	recs := l.flight.snapshot()
	var buf []byte
	for _, r := range recs {
		buf = r.appendLine(buf)
		if len(buf) > 64<<10 {
			if _, err := w.Write(buf); err != nil {
				return 0, err
			}
			buf = buf[:0]
		}
	}
	if _, err := w.Write(buf); err != nil {
		return 0, err
	}
	return len(recs), nil
}

// flightTail is the last n lines the flight recorder holds, for the top of a report.
func (l *Logger) flightTail(n int) string {
	recs := l.flight.snapshot()
	if len(recs) > n {
		recs = recs[len(recs)-n:]
	}
	var buf []byte
	for _, r := range recs {
		buf = r.appendLine(buf)
	}
	return string(bytes.TrimRight(buf, "\n"))
}

// ---------------------------------------------------------------- Go's own

// Go's execution trace, kept for the last seconds in the same way: it shows what every
// goroutine was doing and waiting for, which no log line can. One for the process.
var goTrace struct {
	sync.Mutex
	fr *trace.FlightRecorder
}

// TraceWindow is how much of the execution trace is kept, at least.
const TraceWindow = 15 * time.Second

// StartTraceRecorder starts keeping the execution trace of the last seconds. It costs a
// little of every goroutine switch, so it's a setting of its own.
func StartTraceRecorder() error {
	goTrace.Lock()
	defer goTrace.Unlock()
	if goTrace.fr != nil {
		return nil
	}
	fr := trace.NewFlightRecorder(trace.FlightRecorderConfig{MinAge: TraceWindow, MaxBytes: 8 << 20})
	if err := fr.Start(); err != nil {
		return err
	}
	goTrace.fr = fr
	return nil
}

// StopTraceRecorder stops it, and forgets what it held.
func StopTraceRecorder() {
	goTrace.Lock()
	defer goTrace.Unlock()
	if goTrace.fr != nil {
		goTrace.fr.Stop()
		goTrace.fr = nil
	}
}

// TraceRecorderOn says whether the execution trace is being kept.
func TraceRecorderOn() bool {
	goTrace.Lock()
	defer goTrace.Unlock()
	return goTrace.fr != nil
}

var errNoTrace = errors.New("the execution trace isn't being kept")

// WriteTrace writes the execution trace of the last seconds, for `go tool trace`.
func WriteTrace(w io.Writer) (int64, error) {
	goTrace.Lock()
	defer goTrace.Unlock()
	if goTrace.fr == nil {
		return 0, errNoTrace
	}
	return goTrace.fr.WriteTo(w)
}

// flightKeeps is Enabled's answer while the flight recorder is on: everything from
// debug up is worth building, because it's kept for a while even if no file gets it.
func (l *Logger) flightKeeps(lv slog.Level) bool { return lv >= slog.LevelDebug && l.flight.on.Load() }
