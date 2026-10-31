// Package diagnose is WittyDiagnoser, the disk diagnoser: it looks at a DVD or Blu-ray, or at a
// storage folder, and says in plain words what it found. It only reads. Nothing
// on a disc or a drive is changed or repaired, copy protection is only ever
// detected, and what it writes down holds no disc content.
//
// One check runs at a time, in the background, with a short queue. A check gives
// way to anyone watching something: while the server is busy it rests for as long
// as its last piece of work took.
package diagnose

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// Severity says how much a finding matters.
type Severity string

const (
	SevOK      Severity = "ok"
	SevInfo    Severity = "info"
	SevWarn    Severity = "warn"
	SevProblem Severity = "problem"
)

func (s Severity) rank() int {
	switch s {
	case SevProblem:
		return 0
	case SevWarn:
		return 1
	case SevInfo:
		return 2
	}
	return 3
}

// Finding is one thing a check found.
type Finding struct {
	Severity Severity `json:"severity"`
	Title    string   `json:"title"`
	What     string   `json:"what,omitempty"`
	Fix      string   `json:"fix,omitempty"`
	Details  []string `json:"details,omitempty"`
	// Names are file names (base names only). They come out of a report
	// downloaded with names hidden, so nothing else here may hold one.
	Names []string `json:"names,omitempty"`
	// More counts what Names and Details left out.
	More int `json:"more,omitempty"`
	// Target is the storage folder this is about, and TargetPlain its name with
	// the owner's own words taken out.
	Target      string `json:"target,omitempty"`
	TargetPlain string `json:"targetPlain,omitempty"`
}

// The states of a job.
const (
	StateQueued    = "queued"
	StateRunning   = "running"
	StateDone      = "done"
	StateCancelled = "cancelled"
)

// Kinds of job.
const (
	KindDisc    = "disc"
	KindStorage = "storage"
)

// Limits on what is kept and waited for.
const (
	maxWaiting  = 8
	maxRecent   = 8
	maxFindings = 400
)

// ErrQueueFull: too many checks are already waiting.
var ErrQueueFull = errors.New("Too many checks are already waiting. Try again when one has finished.")

// Env is what a check needs from the server.
type Env struct {
	DataDir string // reports go in <DataDir>/diagnostics
	Log     *slog.Logger
	Version string
	// Busy says someone is watching something. nil means never.
	Busy func() bool
	// Setting reads a server setting. nil means every setting has its default.
	Setting func(key, def string) string
	// ItemPaths lists the files the database knows in a library, at most limit.
	ItemPaths func(ctx context.Context, library int64, limit int) ([]string, error)
}

func (e *Env) setting(key, def string) string {
	if e.Setting == nil {
		return def
	}
	return e.Setting(key, def)
}

// Job is a check, queued, running or finished.
type Job struct {
	ID  string
	env *Env
	log *slog.Logger
	cat string
	run func(ctx context.Context, j *Job)

	cancel context.CancelFunc
	// scrub lists full paths that must never reach a finding, and what to say
	// instead.
	scrub []scrubRule

	mu       sync.Mutex
	report   Report
	phase    string
	fraction float64
	wanted   bool // cancelling was asked for
	saved    bool
}

type scrubRule struct{ path, with string }

// Report is what a check found. It's what is saved and downloaded.
type Report struct {
	ID       string           `json:"id"`
	Kind     string           `json:"kind"` // disc or storage
	State    string           `json:"state"`
	Subject  string           `json:"subject"`         // a disc's file name, or how many folders
	Label    string           `json:"label,omitempty"` // the label written on the disc
	Options  map[string]bool  `json:"options,omitempty"`
	Version  string           `json:"version,omitempty"`
	Queued   int64            `json:"queued"`             // Unix milliseconds
	Started  int64            `json:"started,omitempty"`  // Unix milliseconds
	Finished int64            `json:"finished,omitempty"` // Unix milliseconds
	Worst    Severity         `json:"worst,omitempty"`
	Counts   map[Severity]int `json:"counts"`
	Findings []Finding        `json:"findings"`
	// Dropped counts findings past the limit.
	Dropped int `json:"dropped,omitempty"`
	// Redacted: names were taken out of this copy.
	Redacted bool `json:"redacted,omitempty"`
}

// View is a job as the settings page sees it.
type View struct {
	Report
	Phase    string  `json:"phase,omitempty"`
	Fraction float64 `json:"fraction"`
	Position int     `json:"position,omitempty"` // how many are ahead of a queued job
	Saved    bool    `json:"saved"`              // a report can be downloaded
}

// Manager runs the checks, one at a time.
type Manager struct {
	env Env

	mu      sync.Mutex
	waiting []*Job
	running *Job
	recent  []*Job // finished, oldest first
	working bool   // the worker goroutine is alive
}

// NewManager makes a manager. It starts nothing until a job arrives.
func NewManager(env Env) *Manager {
	if env.Log == nil {
		env.Log = slog.Default()
	}
	return &Manager{env: env}
}

// Submit queues a job. ErrQueueFull when too many are waiting.
func (m *Manager) Submit(kind, subject string, options map[string]bool, scrub []scrubRule, run func(ctx context.Context, j *Job)) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.waiting) >= maxWaiting {
		return nil, ErrQueueFull
	}
	cat := "discs"
	if kind == KindStorage {
		cat = "storage"
	}
	id := logging.NewID("diag")
	j := &Job{ID: id, env: &m.env, cat: cat, run: run, scrub: scrub, log: m.env.Log.With("cat", cat, "session", id)}
	j.report = Report{ID: id, Kind: kind, State: StateQueued, Subject: subject, Options: options, Version: m.env.Version, Queued: time.Now().UnixMilli(), Counts: map[Severity]int{}, Findings: []Finding{}}
	m.waiting = append(m.waiting, j)
	if !m.working {
		m.working = true
		logging.Go("WittyDiagnoser", m.worker, "cat", cat)
	}
	return j, nil
}

func (m *Manager) worker() {
	for {
		m.mu.Lock()
		if len(m.waiting) == 0 {
			m.working = false
			m.mu.Unlock()
			return
		}
		j := m.waiting[0]
		m.waiting = m.waiting[1:]
		m.running = j
		m.mu.Unlock()
		m.runJob(j)
		m.mu.Lock()
		m.running = nil
		m.remember(j)
		m.mu.Unlock()
	}
}

func (m *Manager) remember(j *Job) {
	m.recent = append(m.recent, j)
	if len(m.recent) > maxRecent {
		m.recent = slices.Delete(m.recent, 0, len(m.recent)-maxRecent)
	}
}

func (m *Manager) runJob(j *Job) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	j.mu.Lock()
	j.cancel = cancel
	stopNow := j.wanted
	j.report.State = StateRunning
	j.report.Started = time.Now().UnixMilli()
	subject := j.report.Subject
	j.mu.Unlock()
	if stopNow {
		cancel()
	}
	j.log.Info("WittyDiagnoser started", "kind", j.report.Kind, "file", subject)
	j.Engine("running this check", func() { j.run(ctx, j) })
	state := StateDone
	if ctx.Err() != nil {
		state = StateCancelled
	}
	finished := time.Now().UnixMilli()
	j.mu.Lock()
	rep := j.snapshotLocked()
	rep.State, rep.Finished = state, finished
	started := j.report.Started
	counts := fmt.Sprint(j.report.Counts)
	j.mu.Unlock()
	// Saved first: whoever sees it finished can download the report at once.
	err := saveReport(m.env.DataDir, rep)
	if err != nil {
		j.log.Warn("could not save a WittyDiagnoser report", "err", err)
	}
	j.mu.Lock()
	j.report.State, j.report.Finished = state, finished
	j.phase = ""
	if state == StateDone {
		j.fraction = 1
	}
	j.saved = err == nil
	j.mu.Unlock()
	j.log.Info("WittyDiagnoser finished", "state", state, "findings", counts, "took", (time.Duration(finished-started) * time.Millisecond).Round(time.Second))
}

// Cancel stops a job. ok is false when there's no such job.
func (m *Manager) Cancel(id string) (*Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, j := range m.waiting {
		if j.ID == id {
			m.waiting = slices.Delete(m.waiting, i, i+1)
			j.mu.Lock()
			j.report.State = StateCancelled
			j.report.Finished = time.Now().UnixMilli()
			j.mu.Unlock()
			m.remember(j)
			return j, true
		}
	}
	if j := m.running; j != nil && j.ID == id {
		j.mu.Lock()
		j.wanted = true
		if j.cancel != nil {
			j.cancel()
		}
		j.mu.Unlock()
		return j, true
	}
	for _, j := range m.recent {
		if j.ID == id {
			return j, true
		}
	}
	return nil, false
}

// Get finds a job that's still in memory.
func (m *Manager) Get(id string) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.findLocked(id)
}

func (m *Manager) findLocked(id string) *Job {
	if j := m.running; j != nil && j.ID == id {
		return j
	}
	for _, j := range m.waiting {
		if j.ID == id {
			return j
		}
	}
	for _, j := range m.recent {
		if j.ID == id {
			return j
		}
	}
	return nil
}

// JobView describes a job by ID, from memory or from its saved report.
func (m *Manager) JobView(id string) (*View, bool) {
	m.mu.Lock()
	j := m.findLocked(id)
	pos := 0
	for i, w := range m.waiting {
		if w.ID == id {
			pos = i + 1
			if m.running != nil {
				pos++
			}
		}
	}
	m.mu.Unlock()
	if j != nil {
		v := j.View()
		v.Position = pos
		return v, true
	}
	r, err := loadReport(m.env.DataDir, id)
	if err != nil {
		return nil, false
	}
	return &View{Report: *r, Fraction: 1, Saved: true}, true
}

// Jobs lists what's running and waiting, then the saved reports, newest first,
// without their findings.
func (m *Manager) Jobs() []*View {
	m.mu.Lock()
	live := []*Job{}
	if m.running != nil {
		live = append(live, m.running)
	}
	live = append(live, m.waiting...)
	for i := len(m.recent) - 1; i >= 0; i-- {
		// Saved ones are read back below. A job cancelled while it waited has no
		// report, and is listed from memory.
		if !m.recent[i].isSaved() {
			live = append(live, m.recent[i])
		}
	}
	first := 0
	if m.running != nil {
		first = 1
	}
	m.mu.Unlock()
	out := make([]*View, 0, len(live)+keepReports)
	for i, j := range live {
		v := j.View()
		if j.isQueued() && i >= first {
			v.Position = i - first + 1
			if first == 1 {
				v.Position++
			}
		}
		v.Findings = nil
		out = append(out, v)
	}
	for _, r := range listReports(m.env.DataDir) {
		// A job just finishing can be running and saved for a moment.
		if !slices.ContainsFunc(out, func(v *View) bool { return v.ID == r.ID }) {
			out = append(out, &View{Report: r, Fraction: 1, Saved: true})
		}
	}
	return out
}

func (j *Job) isSaved() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.saved
}

func (j *Job) isQueued() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.report.State == StateQueued
}

// View is a copy of the job as it stands, worst findings first.
func (j *Job) View() *View {
	j.mu.Lock()
	defer j.mu.Unlock()
	return &View{Report: *j.snapshotLocked(), Phase: j.phase, Fraction: j.fraction, Saved: j.saved}
}

func (j *Job) snapshotLocked() *Report {
	r := j.report
	r.Findings = append([]Finding{}, j.report.Findings...)
	sortFindings(r.Findings)
	r.Counts = make(map[Severity]int, len(j.report.Counts))
	for k, v := range j.report.Counts {
		r.Counts[k] = v
	}
	return &r
}

func sortFindings(f []Finding) {
	slices.SortStableFunc(f, func(a, b Finding) int { return a.Severity.rank() - b.Severity.rank() })
}

// Step says which part of the check is under way, and how far along it all is.
func (j *Job) Step(phase string, fraction float64) {
	j.mu.Lock()
	j.phase = phase
	j.fraction = fraction
	j.mu.Unlock()
	j.log.Debug("WittyDiagnoser step", "phase", phase)
}

// Progress moves the bar without changing the words.
func (j *Job) Progress(fraction float64) {
	j.mu.Lock()
	if fraction > j.fraction { // it never goes backward
		j.fraction = min(fraction, 1)
	}
	j.mu.Unlock()
}

// SetLabel records what's written on the disc.
func (j *Job) SetLabel(label string) {
	j.mu.Lock()
	j.report.Label = label
	j.mu.Unlock()
}

// Add records a finding. Full paths are taken out here, whatever made them.
func (j *Job) Add(f Finding) {
	f.Title = j.scrubText(f.Title)
	f.What = j.scrubText(f.What)
	f.Fix = j.scrubText(f.Fix)
	for i := range f.Details {
		f.Details[i] = j.scrubText(f.Details[i])
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.report.Findings) >= maxFindings {
		j.report.Dropped++
		return
	}
	j.report.Findings = append(j.report.Findings, f)
	j.report.Counts[f.Severity]++
	if j.report.Worst == "" || f.Severity.rank() < j.report.Worst.rank() {
		j.report.Worst = f.Severity
	}
}

func (j *Job) scrubText(s string) string {
	for _, r := range j.scrub {
		s = replaceFold(s, r.path, r.with)
		if alt := filepath.ToSlash(r.path); alt != r.path {
			s = replaceFold(s, alt, r.with)
		}
	}
	return s
}

// replaceFold is ReplaceAll ignoring case (a path on Windows comes back in
// whatever case something chose).
func replaceFold(s, old, with string) string {
	if old == "" {
		return s
	}
	low, lowOld := strings.ToLower(s), strings.ToLower(old)
	if len(low) != len(s) || len(lowOld) != len(old) { // lower-casing changed the bytes: exact only
		return strings.ReplaceAll(s, old, with)
	}
	var b strings.Builder
	for {
		i := strings.Index(low, lowOld)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		b.WriteString(with)
		s, low = s[i+len(old):], low[i+len(old):]
	}
}

// Engine runs a part of a check that uses a disc engine or walks a disc's data.
// A crash there is a result, not a disaster: it's recorded with a stack and turned
// into a finding, and the rest of the check carries on. ok is false after a crash.
func (j *Job) Engine(what string, fn func()) (ok bool) {
	defer func() {
		if p := recover(); p != nil {
			logging.ReportPanic("WittyDiagnoser", p, "cat", j.cat, "session", j.ID, "step", what)
			f := Finding{
				Severity: SevProblem,
				Title:    "The engine crashed on this disc",
				What:     "Something went wrong inside the server while " + what + ". That's a bug in the server, and is itself a useful result: this disc would probably cause trouble in the player too.",
				Fix:      "The rest of the check carried on. Please send this report (it holds no disc content) with a bug report, along with the crash report in Settings, WittyLog.",
				Details:  []string{fmt.Sprint(p)},
			}
			if j.cat == "storage" {
				f.Title = "This check crashed"
				f.What = "Something went wrong inside the server while " + what + ". That's a bug in the server, not in your drive."
				f.Fix = "Please send this report with a bug report, along with the crash report in Settings, WittyLog."
			}
			j.Add(f)
			ok = false
		}
	}()
	fn()
	return true
}

// Rest gives way to anyone watching something: when the server is busy, it waits
// for as long as the last piece of work took, so a check never takes more than
// half of what the disk can do. False means the job was cancelled.
func (j *Job) Rest(ctx context.Context, took time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	if j.env.Busy == nil || took <= 0 || !j.env.Busy() {
		return true
	}
	if took > 5*time.Second {
		took = 5 * time.Second
	}
	t := time.NewTimer(took)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// clip keeps the first max of a list, and says how many were left out.
func clip(list []string, total, max int) ([]string, int) {
	if len(list) > max {
		list = list[:max]
	}
	return list, total - len(list)
}
