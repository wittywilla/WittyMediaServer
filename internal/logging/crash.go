package logging

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Crash reports live in their own folder inside the data folder, apart from the
// log folder, whose files come and go. Three kinds:
//
//   - fatal: the Go runtime stopped the process. Nothing in the process can catch
//     that, so the runtime is told to write its last words to a file
//     (debug.SetCrashOutput), and the next start turns that file into a report.
//   - panic: a panic that was caught and logged. The server carried on.
//   - unclean: the last run left without saying so, and without a fatal error:
//     a power cut, a killed process, the system out of memory.

const (
	maxCrashFiles = 30
	maxCrashSize  = 2 << 20
	crashKeepDays = 90
)

// Crash describes one report.
type Crash struct {
	ID      string    `json:"id"`
	Kind    string    `json:"kind"` // fatal, panic, unclean
	Time    time.Time `json:"time"`
	Summary string    `json:"summary"`
	Version string    `json:"version,omitempty"`
	Size    int64     `json:"size"`
	Seen    bool      `json:"seen"`
}

// runMarker says a server is running here. A clean shutdown removes it.
type runMarker struct {
	Version string    `json:"version"`
	Started time.Time `json:"started"`
	PID     int       `json:"pid"`
	LogDir  string    `json:"logDir,omitempty"` // the owner may have moved the logs
}

// CrashGuard is one run of the server, watched.
type CrashGuard struct {
	dir     string
	version string
	pending *os.File
	marker  string
	run     runMarker

	mu     sync.Mutex
	recent map[string]time.Time // recovered panics by where they happened
}

var activeGuard struct {
	sync.RWMutex
	g *CrashGuard
}

// StartCrashGuard turns what the last run left behind into reports and arms the
// capture for this run. logDir is read for the last lines before a crash. It
// returns the reports made now; failures are logged, and never stop the server.
func StartCrashGuard(dir, logDir, version string) (*CrashGuard, []Crash, error) {
	g := &CrashGuard{dir: dir, version: version, recent: map[string]time.Time{}}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	found := g.collect(logDir)
	g.prune()
	me := os.Getpid()
	f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("pending-%d.txt", me)), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return g, found, err
	}
	g.pending = f
	debug.SetTraceback("all") // a deadlock needs everyone's stack, not just one
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		return g, found, err
	}
	g.marker = filepath.Join(dir, fmt.Sprintf("run-%d.json", me))
	g.run = runMarker{Version: version, Started: time.Now(), PID: me, LogDir: logDir}
	if err := g.writeMarker(); err != nil {
		return g, found, err
	}
	activeGuard.Lock()
	activeGuard.g = g
	activeGuard.Unlock()
	return g, found, nil
}

func (g *CrashGuard) writeMarker() error {
	b, err := json.Marshal(g.run)
	if err != nil {
		return err
	}
	return os.WriteFile(g.marker, b, 0o640)
}

// SetLogDir notes where this run's logs are, for the report if it ends badly.
func (g *CrashGuard) SetLogDir(dir string) {
	if g == nil || g.marker == "" {
		return
	}
	g.run.LogDir = dir
	_ = g.writeMarker()
}

// Close: a clean shutdown. Say so, so the next start isn't worried.
func (g *CrashGuard) Close() {
	if g == nil {
		return
	}
	activeGuard.Lock()
	if activeGuard.g == g {
		activeGuard.g = nil
	}
	activeGuard.Unlock()
	if g.pending != nil {
		_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
		name := g.pending.Name()
		_ = g.pending.Close()
		_ = os.Remove(name)
		g.pending = nil
	}
	if g.marker != "" {
		_ = os.Remove(g.marker)
	}
}

// collect turns files left by earlier runs into reports.
func (g *CrashGuard) collect(logDir string) []Crash {
	var found []Crash
	entries, _ := os.ReadDir(g.dir)
	fatalPIDs := map[int]bool{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "pending-") || !strings.HasSuffix(name, ".txt") {
			continue
		}
		pid, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "pending-"), ".txt"))
		p := filepath.Join(g.dir, name)
		if pid != os.Getpid() && pidAlive(pid) {
			continue // another server using this folder (it won't last: the port is taken)
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Size() > 0 {
			m := g.readMarker(pid)
			dir := logDir
			if m.LogDir != "" {
				dir = m.LogDir
			}
			if c, err := g.reportFromFatal(p, info, dir, m.Version); err == nil {
				found = append(found, c)
				fatalPIDs[pid] = true
			}
		}
		_ = os.Remove(p)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "run-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		pid, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "run-"), ".json"))
		// This run hasn't written its marker yet, so one with this process number is from
		// an earlier run: in a container, a server gets the same number every time.
		if pid != os.Getpid() && pidAlive(pid) {
			continue
		}
		p := filepath.Join(g.dir, name)
		if !fatalPIDs[pid] {
			if m := g.readMarker(pid); m.PID != 0 {
				dir := logDir
				if m.LogDir != "" {
					dir = m.LogDir
				}
				if c, err := g.reportFromUnclean(m, dir); err == nil {
					found = append(found, c)
				}
			}
		}
		_ = os.Remove(p)
	}
	return found
}

func (g *CrashGuard) readMarker(pid int) runMarker {
	var m runMarker
	if b, err := os.ReadFile(filepath.Join(g.dir, fmt.Sprintf("run-%d.json", pid))); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

var fatalFirstLine = regexp.MustCompile(`(?m)^(panic: .*|fatal error: .*|runtime: .*|unexpected fault.*|SIG[A-Z]+: .*|Exception 0x.*)$`)

func (g *CrashGuard) reportFromFatal(p string, info os.FileInfo, logDir, version string) (Crash, error) {
	b, err := readBounded(p, maxCrashSize)
	if err != nil {
		return Crash{}, err
	}
	text := Redact(string(b))
	summary := "The server stopped because of an internal error."
	if m := fatalFirstLine.FindString(text); m != "" {
		summary = clean(strings.TrimSpace(m), 200)
	} else if fn := crashedIn(text); fn != "" {
		// The runtime keeps its "fatal error:" line for the console; the file starts at
		// the stack. Where it happened is what's left to go on.
		summary = "The server stopped because of an internal error in " + clean(fn, 160) + "."
	}
	return g.writeReport("fatal", info.ModTime(), version, summary, text, lastLogLines(logDir, 60))
}

func (g *CrashGuard) reportFromUnclean(m runMarker, logDir string) (Crash, error) {
	last := lastLogLines(logDir, 60)
	when := m.Started
	if files := listLogFiles(logDir); len(files) > 0 && files[0].Modified.After(when) {
		when = files[0].Modified
	}
	return g.writeReport("unclean", when, m.Version,
		"The server stopped without shutting down: a power cut, the computer restarting, the process being ended or the system running out of memory.",
		fmt.Sprintf("Started: %s\nProcess: %d\nNo error was recorded by the server itself.\n", m.Started.Format(time.RFC3339), m.PID), last)
}

func (g *CrashGuard) writeReport(kind string, when time.Time, version, summary, body, lastLines string) (Crash, error) {
	id := fmt.Sprintf("crash-%s-%s", when.UTC().Format("20060102T150405.000Z"), kind)
	var b strings.Builder
	fmt.Fprintf(&b, "WittyMediaServer crash report\nKind: %s\nTime: %s\nVersion: %s\nThis run: %s %s/%s\n\n%s\n\n", kind, when.Format(time.RFC3339), version, runtime.Version(), runtime.GOOS, runtime.GOARCH, summary)
	if body != "" {
		b.WriteString("--- details ---\n")
		b.WriteString(body)
		b.WriteString("\n")
	}
	if lastLines != "" {
		b.WriteString("\n--- last lines of the log ---\n")
		b.WriteString(lastLines)
		b.WriteString("\n")
	}
	p := filepath.Join(g.dir, id+".txt")
	if err := os.WriteFile(p, []byte(b.String()), 0o640); err != nil {
		return Crash{}, err
	}
	return Crash{ID: id, Kind: kind, Time: when, Summary: summary, Version: version, Size: int64(b.Len())}, nil
}

// readBounded reads a file, keeping its start and end if it is too long.
func readBounded(p string, limit int64) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() <= limit {
		b := make([]byte, st.Size())
		n, err := readFull(f, b)
		return b[:n], err
	}
	half := limit / 2
	head := make([]byte, half)
	n, err := readFull(f, head)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(st.Size()-half, 0); err != nil {
		return nil, err
	}
	tail := make([]byte, half)
	m, err := readFull(f, tail)
	if err != nil {
		return nil, err
	}
	out := append(head[:n], []byte(fmt.Sprintf("\n…[%d bytes cut]…\n", st.Size()-2*half))...)
	return append(out, tail[:m]...), nil
}

func readFull(f *os.File, b []byte) (int, error) {
	n := 0
	for n < len(b) {
		m, err := f.Read(b[n:])
		n += m
		if err != nil {
			if errors.Is(err, os.ErrClosed) {
				return n, err
			}
			break // EOF
		}
	}
	return n, nil
}

// lastLogLines is the end of the newest log file, redacted again, as it may be
// from an older version of the server.
func lastLogLines(logDir string, n int) string {
	files := listLogFiles(logDir)
	if len(files) == 0 {
		return ""
	}
	f, err := os.Open(filepath.Join(logDir, files[0].Name))
	if err != nil {
		return ""
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > 256<<10 {
		_, _ = f.Seek(st.Size()-256<<10, 0)
	}
	var keep []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 256<<10)
	for sc.Scan() {
		keep = append(keep, Redact(sc.Text()))
		if len(keep) > n {
			keep = keep[1:]
		}
	}
	return strings.Join(keep, "\n")
}

// prune keeps the newest reports, for a while.
func (g *CrashGuard) prune() {
	list := g.List()
	cutoff := time.Now().AddDate(0, 0, -crashKeepDays)
	for i, c := range list {
		if i >= maxCrashFiles || c.Time.Before(cutoff) {
			_ = os.Remove(filepath.Join(g.dir, c.ID+".txt"))
			_ = os.Remove(filepath.Join(g.dir, c.ID+".seen"))
		}
	}
}

var crashName = regexp.MustCompile(`^crash-[0-9TZ.]+-(fatal|panic|unclean)$`)

// IsCrashID accepts only our own report names (never a path).
func IsCrashID(id string) bool { return crashName.MatchString(id) }

// List is every report, newest first.
func (g *CrashGuard) List() []Crash {
	if g == nil {
		return nil
	}
	entries, err := os.ReadDir(g.dir)
	if err != nil {
		return nil
	}
	var out []Crash
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".txt")
		if !ok || !IsCrashID(id) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		c := Crash{ID: id, Kind: id[strings.LastIndexByte(id, '-')+1:], Time: info.ModTime(), Size: info.Size()}
		if t, err := time.Parse("20060102T150405.000Z", strings.TrimPrefix(id[:strings.LastIndexByte(id, '-')], "crash-")); err == nil {
			c.Time = t
		}
		c.Summary, c.Version = readCrashHead(filepath.Join(g.dir, e.Name()))
		if _, err := os.Stat(filepath.Join(g.dir, id+".seen")); err == nil {
			c.Seen = true
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out
}

// readCrashHead reads the report's first few lines: version and summary.
func readCrashHead(p string) (summary, version string) {
	f, err := os.Open(p)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for i := 0; i < 8 && sc.Scan(); i++ {
		line := sc.Text()
		if v, ok := strings.CutPrefix(line, "Version: "); ok {
			version = v
		}
		if i == 6 {
			summary = clean(line, 300)
		}
	}
	return summary, version
}

// Read returns one report's text.
func (g *CrashGuard) Read(id string) ([]byte, error) {
	if g == nil || !IsCrashID(id) {
		return nil, os.ErrNotExist
	}
	return readBounded(filepath.Join(g.dir, id+".txt"), maxCrashSize*2)
}

// MarkSeen: the owner has seen this one, so it stops being pointed out.
func (g *CrashGuard) MarkSeen(id string) error {
	if g == nil || !IsCrashID(id) {
		return os.ErrNotExist
	}
	if _, err := os.Stat(filepath.Join(g.dir, id+".txt")); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(g.dir, id+".seen"), nil, 0o640)
}

// Delete removes one report.
func (g *CrashGuard) Delete(id string) error {
	if g == nil || !IsCrashID(id) {
		return os.ErrNotExist
	}
	_ = os.Remove(filepath.Join(g.dir, id+".seen"))
	return os.Remove(filepath.Join(g.dir, id+".txt"))
}

// Dir is the folder holding the reports.
func (g *CrashGuard) Dir() string {
	if g == nil {
		return ""
	}
	return g.dir
}

// recordPanic writes a report for a panic that was caught, unless the same place
// has just done so: a loop of panics must not fill the folder.
func (g *CrashGuard) recordPanic(where string, value any, stack []byte) {
	sig := where + "|" + firstFrame(stack)
	now := time.Now()
	g.mu.Lock()
	last, seen := g.recent[sig]
	if !seen || now.Sub(last) > 10*time.Minute {
		g.recent[sig] = now
	}
	if len(g.recent) > 256 {
		g.recent = map[string]time.Time{sig: now}
	}
	g.mu.Unlock()
	if seen && now.Sub(last) <= 10*time.Minute {
		return
	}
	summary := clean(Redact(fmt.Sprintf("A panic in %s was caught; the server carried on: %v", where, value)), 300)
	body := Redact(string(stack))
	_, _ = g.writeReport("panic", now, g.version, summary, body, "")
	g.prune()
}

// crashedIn finds the first function in a crash's stack that isn't the runtime's
// own machinery.
func crashedIn(text string) string {
	first := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "goroutine ") || !strings.HasSuffix(line, ")") && !strings.Contains(line, "(") {
			continue
		}
		if strings.HasPrefix(line, "runtime.") || strings.HasPrefix(line, "internal/") || strings.HasPrefix(line, "panic(") || strings.HasPrefix(line, "fatal") || strings.Contains(line, ".fatal(") || strings.Contains(line, ".go:") {
			continue
		}
		i := strings.LastIndexByte(line, '(')
		if i <= 0 {
			continue
		}
		fn := line[:i]
		if first == "" {
			first = fn
		}
		// The first of our own, rather than the standard library that noticed.
		if pkg, _, _ := strings.Cut(fn, "/"); strings.Contains(pkg, ".") && !strings.HasPrefix(fn, "sync.") {
			return fn
		}
	}
	return first
}

func firstFrame(stack []byte) string {
	// Past the goroutine line, the panic machinery and the recover itself.
	lines := strings.Split(string(stack), "\n")
	for i := 1; i+1 < len(lines); i += 2 {
		fn := lines[i]
		if strings.HasPrefix(fn, "panic(") || strings.Contains(fn, "runtime/debug.Stack") || strings.Contains(fn, "internal/logging.") {
			continue
		}
		return fn
	}
	return ""
}

// ---------------------------------------------------------------- recovering

// Recover is for a defer at the top of a goroutine: a panic becomes a record
// with its stack and a crash report, and that goroutine ends instead of the
// whole server. attrs are added to the record ("session", id, "cat", "games").
//
//	defer logging.Recover("hls sweep", "cat", "transcode")
func Recover(where string, attrs ...any) {
	if p := recover(); p != nil {
		ReportPanic(where, p, attrs...)
	}
}

// ReportPanic records a panic already recovered by the caller.
func ReportPanic(where string, value any, attrs ...any) {
	if value == http.ErrAbortHandler { //nolint:errorlint // the standard library's sentinel: nothing is wrong
		return
	}
	stack := debug.Stack()
	args := append([]any{"where", where, "panic", fmt.Sprint(value), "stack", string(stack)}, attrs...)
	slog.Default().Log(context.Background(), LevelCrash, "a part of the server crashed and was recovered", args...)
	activeGuard.RLock()
	g := activeGuard.g
	activeGuard.RUnlock()
	if g != nil {
		g.recordPanic(where, value, stack)
	}
	if l := active.Load(); l != nil {
		l.Flush()
	}
}

// Go runs fn on a goroutine of its own, recovering any panic.
func Go(where string, fn func(), attrs ...any) {
	go func() {
		defer Recover(where, attrs...)
		fn()
	}()
}

// Loop is Go for something that must keep running, such as a sweeper: after a
// panic it starts again, a little later each time.
func Loop(where string, fn func(), attrs ...any) {
	go func() {
		wait := 5 * time.Second
		for {
			if !runOnce(where, fn, attrs) {
				return
			}
			time.Sleep(wait)
			if wait < 5*time.Minute {
				wait *= 2
			}
		}
	}()
}

func runOnce(where string, fn func(), attrs []any) (panicked bool) {
	defer func() {
		if p := recover(); p != nil {
			panicked = true
			ReportPanic(where, p, attrs...)
		}
	}()
	fn()
	return false
}
