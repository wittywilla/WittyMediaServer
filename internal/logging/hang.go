package logging

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// A hang writes nothing: a server that has stopped answering is, from inside, only a
// lot of goroutines waiting. The watchdog asks the server, every few seconds, whether
// it still answers, and when it hasn't for a while, writes down what every goroutine is
// waiting for, with the minutes before it from the flight recorder. That is what a
// "crash" with an empty log needed.

// Check is one question the watchdog asks: nil means fine. It's given a time limit,
// and one that doesn't come back is a failure too.
type Check struct {
	Name string
	Run  func(ctx context.Context) error
	// Patience is how many times hangAfter it may fail before that's a hang: a database
	// busy with a long scan is slow, not stuck. 0 is once.
	Patience int
}

// Watchdog timings. Variables so tests can shrink them.
var (
	watchEvery   = 2 * time.Second  // how often each check is asked
	watchLimit   = 2 * time.Second  // how long one may take
	hangAfter    = 10 * time.Second // failing this long is a hang
	hangAgain    = 30 * time.Second // a second look, to tell stuck from slow
	hangGap      = 10 * time.Minute // between hangs written down
	heartbeatLag = 5 * time.Second  // the watchdog's own clock falling this far behind
)

// Evidence parts beside a report, by the end of their names.
const (
	PartFlight     = "flight.log"
	PartGoroutines = "goroutines.txt"
	PartTrace      = "trace"
)

var evidenceParts = []string{PartFlight, PartGoroutines, PartTrace}

// watchdog state, for what's shown in a report and for tests.
type watchdog struct {
	log    *slog.Logger
	checks []Check
	beat   atomic.Int64 // unix nanoseconds of the last heartbeat

	mu        sync.Mutex
	failing   map[string]time.Time // since when
	lastErr   map[string]string
	running   map[string]bool
	episode   time.Time // when this hang began; zero while all is well
	dumps     int       // written this episode
	lastDump  time.Time
	onHang    func(Crash) // tests
	lastSaved Crash
}

// StartWatchdog watches until ctx ends. The checks are the server's own: its web
// address, its database. A heartbeat of the watchdog's own is added, for a process so
// busy that nothing gets to run.
func StartWatchdog(ctx context.Context, log *slog.Logger, checks ...Check) {
	w := newWatchdog(log, checks)
	activeWatch.Store(w)
	Go("watchdog", func() { w.run(ctx) }, "cat", "server")
}

func newWatchdog(log *slog.Logger, checks []Check) *watchdog {
	if log == nil {
		log = slog.Default()
	}
	w := &watchdog{log: log, failing: map[string]time.Time{}, lastErr: map[string]string{}, running: map[string]bool{}}
	w.beat.Store(time.Now().UnixNano())
	w.checks = append(checks, Check{Name: "heartbeat", Run: w.heartbeat})
	return w
}

func (w *watchdog) heartbeat(context.Context) error {
	last := time.Unix(0, w.beat.Swap(time.Now().UnixNano()))
	if lag := time.Since(last) - watchEvery; lag > heartbeatLag {
		return fmt.Errorf("the watchdog itself ran %s late: the process is too busy, or was paused", lag.Round(time.Second))
	}
	return nil
}

func (w *watchdog) run(ctx context.Context) {
	t := time.NewTicker(watchEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.round(ctx)
		}
	}
}

// round asks every check, each on its own goroutine: one that never returns mustn't
// stop the others being asked, and isn't asked again until it does.
func (w *watchdog) round(ctx context.Context) {
	var wg sync.WaitGroup
	for _, c := range w.checks {
		w.mu.Lock()
		busy := w.running[c.Name]
		if !busy {
			w.running[c.Name] = true
		}
		w.mu.Unlock()
		if busy {
			w.note(c.Name, time.Now(), fmt.Errorf("still waiting for the last check to answer"))
			continue
		}
		asked := time.Now()
		wg.Add(1)
		done := make(chan error, 1)
		go func() {
			defer func() {
				if p := recover(); p != nil {
					done <- fmt.Errorf("the check crashed: %v", p)
				}
				w.mu.Lock()
				w.running[c.Name] = false
				w.mu.Unlock()
			}()
			cctx, cancel := context.WithTimeout(ctx, watchLimit)
			defer cancel()
			done <- c.Run(cctx)
		}()
		go func() {
			defer wg.Done()
			t := time.NewTimer(watchLimit + time.Second)
			defer t.Stop()
			select {
			case err := <-done:
				w.note(c.Name, asked, err)
			case <-t.C:
				w.note(c.Name, asked, fmt.Errorf("no answer in %s", watchLimit))
			}
		}()
	}
	wg.Wait()
	if ctx.Err() == nil {
		w.judge(time.Now())
	}
}

// note records a check's answer; since is when it was asked, which is when the trouble
// began if it didn't answer.
func (w *watchdog) note(name string, since time.Time, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err == nil {
		delete(w.failing, name)
		delete(w.lastErr, name)
		return
	}
	if _, ok := w.failing[name]; !ok {
		w.failing[name] = since
	}
	w.lastErr[name] = err.Error()
}

// judge decides whether what's failing is a hang, and writes it down: once when it's
// been long enough, and once more a little later, to tell stuck from slow.
func (w *watchdog) judge(now time.Time) {
	w.mu.Lock()
	var worst string
	var since time.Time
	for name, t := range w.failing {
		if worst == "" || t.Before(since) {
			worst, since = name, t
		}
	}
	// The check that has been failing longest for its patience decides.
	failingFor, over := time.Duration(0), false
	for _, c := range w.checks {
		t, ok := w.failing[c.Name]
		if !ok {
			continue
		}
		d := now.Sub(t)
		if d > failingFor {
			failingFor = d
		}
		if d >= hangAfter*time.Duration(max(c.Patience, 1)) {
			over = true
		}
	}
	if worst == "" {
		if !w.episode.IsZero() {
			took := now.Sub(w.episode)
			w.episode, w.dumps = time.Time{}, 0
			w.mu.Unlock()
			w.log.Warn("the server is answering again", "cat", "server", "after", took.Round(time.Second).String())
			return
		}
		w.mu.Unlock()
		return
	}
	what := w.describeLocked()
	dump := 0
	switch {
	case !over && w.episode.IsZero():
	case w.episode.IsZero():
		w.episode = since
		if w.lastDump.IsZero() || now.Sub(w.lastDump) >= hangGap {
			dump = 1
		}
	case w.dumps == 1 && now.Sub(w.lastDump) >= hangAgain:
		dump = 2
	}
	if dump > 0 {
		w.dumps, w.lastDump = dump, now
	}
	w.mu.Unlock()
	if dump == 0 {
		return
	}
	blocked := ""
	stacks := goroutineGroups()
	if g := busiestGroup(stacks); g != "" {
		blocked = g
	}
	msg := "the server stopped answering"
	if dump == 2 {
		msg = "the server is still not answering"
	}
	w.log.Error(msg, "cat", "server", "for", failingFor.Round(time.Second).String(), "failing", what, "blocked", blocked)
	summary := fmt.Sprintf("The server stopped answering for %s: %s.", failingFor.Round(time.Second), what)
	if dump == 2 {
		summary = fmt.Sprintf("The server was still not answering after %s: %s.", failingFor.Round(time.Second), what)
	}
	if blocked != "" {
		summary += " Most waiting: " + blocked + "."
	}
	c, err := SaveEvidence("hang", summary, w.details(stacks))
	if err != nil {
		w.log.Warn("what the server was doing couldn't be written down", "cat", "server", "err", err)
		return
	}
	w.mu.Lock()
	w.lastSaved = c
	f := w.onHang
	w.mu.Unlock()
	if f != nil {
		f(c)
	}
}

func (w *watchdog) describeLocked() string {
	names := make([]string, 0, len(w.failing))
	for n := range w.failing {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, n+" ("+w.lastErr[n]+")")
	}
	return strings.Join(parts, ", ")
}

func (w *watchdog) details(stacks string) string {
	var b strings.Builder
	w.mu.Lock()
	b.WriteString("What was asked, and how it went:\n")
	for _, c := range w.checks {
		if since, ok := w.failing[c.Name]; ok {
			fmt.Fprintf(&b, "  %s: failing for %s: %s\n", c.Name, time.Since(since).Round(time.Second), w.lastErr[c.Name])
		} else {
			fmt.Fprintf(&b, "  %s: fine\n", c.Name)
		}
	}
	w.mu.Unlock()
	b.WriteString("\n")
	b.WriteString(InFlight())
	b.WriteString("\n--- goroutines, grouped by where they are ---\n")
	b.WriteString(stacks)
	return b.String()
}

// ---------------------------------------------------------------- evidence

// SaveEvidence writes a report of the given kind ("hang", "saved") with everything the
// server can say about this moment: every goroutine's stack, the flight recorder's last
// minutes and the execution trace, each as a file beside the report.
func SaveEvidence(kind, summary, details string) (Crash, error) {
	activeGuard.RLock()
	g := activeGuard.g
	activeGuard.RUnlock()
	if g == nil {
		return Crash{}, fmt.Errorf("crash reports can't be kept")
	}
	l := active.Load()
	if details == "" {
		details = InFlight() + "\n--- goroutines, grouped by where they are ---\n" + goroutineGroups()
	}
	last := ""
	if l != nil {
		last = l.flightTail(80)
	}
	now := time.Now()
	c, err := g.writeReport(kind, now, g.version, summary, Redact(details), last)
	if err != nil {
		return c, err
	}
	base := filepath.Join(g.dir, c.ID)
	if l != nil && l.FlightRecorderOn() {
		if f, err := os.OpenFile(base+"."+PartFlight, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640); err == nil {
			bw := bufio.NewWriterSize(f, 64<<10)
			_, _ = l.WriteFlight(bw)
			_ = bw.Flush()
			_ = f.Close()
		}
	}
	if f, err := os.OpenFile(base+"."+PartGoroutines, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640); err == nil {
		_, _ = f.WriteString(Redact(allStacks()))
		_ = f.Close()
	}
	if TraceRecorderOn() {
		if f, err := os.OpenFile(base+"."+PartTrace, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640); err == nil {
			_, terr := WriteTrace(f)
			_ = f.Close()
			if terr != nil {
				_ = os.Remove(base + "." + PartTrace)
			}
		}
	}
	c.Parts = g.parts(c.ID)
	g.prune()
	if l != nil {
		l.Flush()
	}
	return c, nil
}

// allStacks is every goroutine's stack, as a panic would print it, up to a limit.
func allStacks() string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) || len(buf) >= 16<<20 {
			return string(buf[:n])
		}
		buf = make([]byte, 2*len(buf))
	}
}

// goroutineGroups is every goroutine, with those waiting in the same place counted
// together: "300 @ ..." is what a pile-up looks like.
func goroutineGroups() string {
	var b bytes.Buffer
	if p := pprof.Lookup("goroutine"); p != nil {
		_ = p.WriteTo(&b, 1)
	}
	if b.Len() > maxCrashSize/2 {
		b.Truncate(maxCrashSize / 2)
		b.WriteString("\n…[cut]\n")
	}
	return b.String()
}

// busiestGroup reads goroutineGroups for the place the most goroutines are waiting in
// the server's own code, as "57 in server.(*Server).askOnline".
func busiestGroup(groups string) string {
	best, bestN := "", 0
	for _, block := range strings.Split(groups, "\n\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) < 2 {
			continue
		}
		head, _, ok := strings.Cut(lines[0], " @ ")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(head))
		if err != nil || n <= bestN {
			continue
		}
		for _, line := range lines[1:] {
			f := strings.Fields(line) // "#", address, function+offset, file:line
			if len(f) < 3 || !strings.Contains(f[2], "wittywilla/wittymediaserver/") || strings.Contains(f[2], "/internal/logging.") {
				continue
			}
			fn := f[2]
			if i := strings.LastIndex(fn, "+0x"); i > 0 {
				fn = fn[:i]
			}
			if i := strings.LastIndex(fn, "/"); i >= 0 {
				fn = fn[i+1:]
			}
			best, bestN = fmt.Sprintf("%d in %s", n, fn), n
			break
		}
	}
	return best
}

// The watchdog that's running, for NoteStopping.
var activeWatch atomic.Pointer[watchdog]

// NoteStopping is called as the server begins to stop. A stop while it isn't answering,
// or soon after, is very likely someone giving up on it: what it was doing is kept, as
// the watchdog may not have had the chance.
func NoteStopping() {
	w := activeWatch.Load()
	if w == nil {
		return
	}
	w.mu.Lock()
	episode, last := w.episode, w.lastDump
	what := w.describeLocked()
	failing := len(w.failing) > 0
	w.mu.Unlock()
	var summary string
	switch {
	case !episode.IsZero():
		summary = fmt.Sprintf("The server was asked to stop while it wasn't answering, for %s: %s.", time.Since(episode).Round(time.Second), what)
	case failing:
		summary = fmt.Sprintf("The server was asked to stop while it was slow to answer: %s.", what)
	case !last.IsZero() && time.Since(last) < flightAge:
		summary = fmt.Sprintf("The server was asked to stop %s after it had stopped answering.", time.Since(last).Round(time.Second))
	default:
		return
	}
	if !last.IsZero() && time.Since(last) < hangAgain {
		return // just written down
	}
	if _, err := SaveEvidence("hang", summary, ""); err != nil {
		w.log.Warn("what the server was doing couldn't be written down", "cat", "server", "err", err)
	}
}
