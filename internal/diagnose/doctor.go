// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package diagnose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// The doctor is the half of this package that changes things, and only when told. Each
// check looks at one kind of trouble with the server's own data and install and says, in
// plain words, what it found and what its fix would do. The disc and storage checks in the
// rest of the package stay read-only.
//
// It needs nothing from a running server, so it works when the server won't start. The
// command line and Settings use this same code.

// Area is the kind of trouble a check looks for.
type Area string

const (
	AreaData     Area = "data"
	AreaMetadata Area = "metadata"
	AreaModules  Area = "modules"
	AreaInstall  Area = "install"
	// AreaFeatures: peer to peer, module overrides, emulators, karaoke and channels, as a
	// running server sees them. See doctor_features.go.
	AreaFeatures Area = "features"
)

// Areas in the order they're shown and run. The database goes first: other checks need
// to know whether what they read can be believed.
var Areas = []Area{AreaData, AreaMetadata, AreaModules, AreaInstall, AreaFeatures}

// Safety says what a fix risks.
type Safety string

const (
	// SafetySafe: nothing can be lost by it.
	SafetySafe Safety = "safe"
	// SafetyCareful changes data, and a safety copy is made first.
	SafetyCareful Safety = "careful"
	// SafetyManual: only a person can do it. Steps says how.
	SafetyManual Safety = "manual"
)

// Issue is one thing a check found.
type Issue struct {
	Check    string   `json:"check"`
	Area     Area     `json:"area"`
	Key      string   `json:"key"` // the same issue gets the same key on the next run
	Severity Severity `json:"severity"`
	Title    string   `json:"title"`
	What     string   `json:"what"`
	// Fix is what the fix does; for a manual one, what is wanted of the person.
	Fix     string   `json:"fix,omitempty"`
	Safety  Safety   `json:"safety"`
	Steps   []string `json:"steps,omitempty"`
	Details []string `json:"details,omitempty"`
	Names   []string `json:"names,omitempty"`
	Count   int      `json:"count,omitempty"`
	More    int      `json:"more,omitempty"`
	// Exclusive: the fix needs the server stopped, or takes effect at its next start.
	Exclusive bool `json:"exclusive,omitempty"`
	// Input says the fix needs an answer from a person; "remap" is the new place of a folder.
	Input string `json:"input,omitempty"`
	// Options are other ways of fixing it, chosen by a person.
	Options []FixOption `json:"options,omitempty"`
	// NotChecked: the check couldn't run, and this says why. Not a problem in itself.
	NotChecked bool `json:"notChecked,omitempty"`

	data any // what the fix needs; never leaves the process
}

// FixOption is another way of fixing an issue.
type FixOption struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	What    string `json:"what"`
	Network bool   `json:"network,omitempty"`
}

// CanFix: there's something this program can do about it.
func (i *Issue) CanFix() bool { return i.Safety != SafetyManual && !i.NotChecked }

// Check is one kind of look at the server.
type Check struct {
	ID    string
	Area  Area
	Title string
	Run   func(ctx context.Context, e *DoctorEnv) []Issue
	// Fix does what the issue's Fix says. It answers with a sentence saying what it did.
	Fix func(ctx context.Context, e *DoctorEnv, i *Issue, f *FixContext) (string, error)
}

// ModuleOps is a running server's own way of changing modules, which it keeps in memory.
type ModuleOps interface {
	Rollback(id string) error
	UseBuiltin(id string) error
	Install(id string) error
}

// DoctorEnv is what the doctor needs to know. None of it needs a server.
type DoctorEnv struct {
	Data       string // the data folder
	Cache      string
	Images     string
	Transcode  string
	LogDir     string
	ConfigFile string
	Program    string // the folder of the program
	Portable   bool
	Listen     string
	FFmpeg     string
	FFprobe    string
	Version    string
	// ConfigError is what was wrong with wms.json, if the settings here are second best.
	ConfigError string
	// InServer: this is the running server itself, in Settings.
	InServer bool
	// Modules is set in a running server. Without it module state is changed on disk,
	// which is only right with no server running.
	Modules ModuleOps
	// TryPort allows a short bind on 127.0.0.1 to see if the port is free.
	TryPort bool
	Log     *slog.Logger
	// Features is set in a running server: how its newer features are doing.
	Features func(ctx context.Context) *FeatureState
	// Now is for tests.
	Now func() time.Time

	mu      sync.Mutex
	ro      *db.DB
	roErr   error
	opened  bool
	damaged string // why the database can't be believed

	featuresDone bool
	featureState *FeatureState
}

// log never returns nil.
func (e *DoctorEnv) log() *slog.Logger {
	if e.Log != nil {
		return e.Log
	}
	return slog.New(slog.DiscardHandler)
}

func (e *DoctorEnv) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// readDB is the database opened for reading, or why it can't be. It's opened once a run.
func (e *DoctorEnv) readDB() (*db.DB, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.opened {
		e.ro, e.roErr = safeOpen(e.Data, true)
		e.opened = true
	}
	return e.ro, e.roErr
}

// writeDB opens the database for one fix. The caller closes it.
func (e *DoctorEnv) writeDB() (*db.DB, error) { return safeOpen(e.Data, false) }

// closeDB lets go of the reading connection: before the file moves, and at the end of a run.
func (e *DoctorEnv) closeDB() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ro != nil {
		_ = e.ro.Close()
	}
	e.ro, e.roErr, e.opened = nil, nil, false
}

func safeOpen(data string, readOnly bool) (d *db.DB, err error) {
	defer func() {
		if p := recover(); p != nil {
			d, err = nil, fmt.Errorf("the database engine stopped on this file: %v", p)
		}
	}()
	return db.OpenExisting(data, readOnly)
}

// serverPID is a server running on this data folder other than this process.
func (e *DoctorEnv) serverPID() (int, bool) {
	return logging.RunningServer(filepath.Join(e.Data, "crash"))
}

// serverUp: a server has these files open, whether that's this process or another one.
func (e *DoctorEnv) ServerUp() bool {
	if e.InServer {
		return true
	}
	_, ok := e.serverPID()
	return ok
}

// CheckResult says a check ran, and how much it found.
type CheckResult struct {
	ID    string `json:"id"`
	Area  Area   `json:"area"`
	Title string `json:"title"`
	Found int    `json:"found"`
}

// DoctorReport is one run's results, worst first.
type DoctorReport struct {
	Version       string        `json:"version"`
	At            int64         `json:"at"`
	Took          int64         `json:"tookMs"`
	Data          string        `json:"data"`
	ServerRunning bool          `json:"serverRunning"`
	Checks        []CheckResult `json:"checks"`
	Issues        []Issue       `json:"issues"`
	Problems      int           `json:"problems"`
	Warnings      int           `json:"warnings"`
	Notes         int           `json:"notes"`
	Fixable       int           `json:"fixable"`
}

// Healthy: nothing that needs anyone's attention. Notes don't count.
func (r *DoctorReport) Healthy() bool { return r.Problems == 0 && r.Warnings == 0 }

// Find looks an issue up by its key.
func (r *DoctorReport) Find(key string) *Issue {
	for i := range r.Issues {
		if r.Issues[i].Key == key {
			return &r.Issues[i]
		}
	}
	return nil
}

// AllChecks is every check, in the order they run.
func AllChecks() []Check {
	out := append([]Check{}, dataChecks()...)
	out = append(out, metadataChecks()...)
	out = append(out, moduleChecks()...)
	out = append(out, installChecks()...)
	return append(out, featureChecks()...)
}

func findCheck(id string) *Check {
	for _, c := range AllChecks() {
		if c.ID == id {
			return &c
		}
	}
	return nil
}

// Selected says which checks a list of ids and areas means. Empty means all of them.
func Selected(only []string) ([]Check, error) {
	all := AllChecks()
	if len(only) == 0 {
		return all, nil
	}
	known := map[string]bool{}
	for _, c := range all {
		known[c.ID], known[string(c.Area)] = true, true
	}
	want := map[string]bool{}
	for _, o := range only {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if !known[o] {
			return nil, fmt.Errorf("there's no check called %q", o)
		}
		want[o] = true
	}
	if len(want) == 0 {
		return all, nil
	}
	var out []Check
	for _, c := range all {
		if want[c.ID] || want[string(c.Area)] {
			out = append(out, c)
		}
	}
	return out, nil
}

// RunDoctor runs checks and returns what they found. One check crashing is reported as
// a finding, and the rest carry on. progress may be nil.
func RunDoctor(ctx context.Context, e *DoctorEnv, checks []Check, progress func(phase string, fraction float64)) *DoctorReport {
	started := time.Now()
	e.closeDB()
	defer e.closeDB()
	e.damaged = ""
	rep := &DoctorReport{Version: e.Version, At: e.now().UnixMilli(), Data: e.Data, ServerRunning: e.ServerUp(), Issues: []Issue{}, Checks: []CheckResult{}}
	for n, c := range checks {
		if err := ctx.Err(); err != nil {
			break
		}
		if progress != nil {
			progress(c.Title, float64(n)/float64(len(checks)))
		}
		found := runOne(ctx, e, c)
		rep.Checks = append(rep.Checks, CheckResult{ID: c.ID, Area: c.Area, Title: c.Title, Found: len(found)})
		rep.Issues = append(rep.Issues, found...)
	}
	sort.SliceStable(rep.Issues, func(i, j int) bool {
		a, b := rep.Issues[i], rep.Issues[j]
		if a.Severity.rank() != b.Severity.rank() {
			return a.Severity.rank() < b.Severity.rank()
		}
		return areaRank(a.Area) < areaRank(b.Area)
	})
	for i := range rep.Issues {
		switch rep.Issues[i].Severity {
		case SevProblem:
			rep.Problems++
		case SevWarn:
			rep.Warnings++
		case SevInfo:
			rep.Notes++
		}
		if rep.Issues[i].CanFix() {
			rep.Fixable++
		}
	}
	rep.Took = time.Since(started).Milliseconds()
	if progress != nil {
		progress("Finished", 1)
	}
	return rep
}

func areaRank(a Area) int {
	for i, x := range Areas {
		if x == a {
			return i
		}
	}
	return len(Areas)
}

func runOne(ctx context.Context, e *DoctorEnv, c Check) (out []Issue) {
	defer func() {
		if p := recover(); p != nil {
			e.log().Error("a repair check crashed", "cat", "server", "check", c.ID, "err", p, "stack", string(debug.Stack()))
			out = []Issue{{Severity: SevWarn, Title: "This check stopped unexpectedly: " + c.Title, What: "The check itself failed, which says nothing about your server. Nothing was changed. It's worth reporting.", Safety: SafetyManual, NotChecked: true, Key: c.ID + ":crashed"}}
		}
		for i := range out {
			out[i].Check, out[i].Area = c.ID, c.Area
			if out[i].Key == "" {
				out[i].Key = c.ID
			}
			if out[i].Safety == "" {
				out[i].Safety = SafetyManual
			}
			out[i].Names, out[i].More = clipNames(out[i].Names, out[i].More)
			out[i].Details, out[i].More = clipMore(out[i].Details, out[i].More)
		}
	}()
	return c.Run(ctx, e)
}

const doctorListMax = 40

func clipNames(l []string, more int) ([]string, int) { return clipMore(l, more) }

func clipMore(l []string, more int) ([]string, int) {
	if len(l) > doctorListMax {
		return l[:doctorListMax], more + len(l) - doctorListMax
	}
	return l, more
}

// notChecked is what a check says when it can't look.
func notChecked(key, title, why string) []Issue {
	return []Issue{{Key: key, Severity: SevInfo, Title: "Not checked: " + title, What: why, Safety: SafetyManual, NotChecked: true}}
}

// ErrNeedsStopped: this fix can't run while a server has the files open.
var ErrNeedsStopped = errors.New("this fix needs the server stopped")

// FixResult says how one fix went.
type FixResult struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	OK      bool   `json:"ok"`
	Done    string `json:"done,omitempty"`
	Error   string `json:"error,omitempty"`
	Copy    string `json:"copy,omitempty"` // where the safety copy is
	Restart bool   `json:"restart,omitempty"`
}

// ApplyFixes runs the fixes of the given issues, in order: database first, because
// other fixes need it sound. A safety copy is made before the first one. If it can't be
// made, nothing runs. option is another way of fixing, for one issue.
func ApplyFixes(ctx context.Context, e *DoctorEnv, issues []*Issue, option string) []FixResult {
	e.closeDB()
	defer e.closeDB()
	out := []FixResult{}
	fix := &FixContext{env: e}
	ordered := append([]*Issue{}, issues...)
	sort.SliceStable(ordered, func(a, b int) bool { return areaRank(ordered[a].Area) < areaRank(ordered[b].Area) })
	for _, i := range ordered {
		r := FixResult{Key: i.Key, Title: i.Title}
		func() {
			defer func() {
				if p := recover(); p != nil {
					e.log().Error("a repair crashed", "cat", "server", "check", i.Check, "err", p, "stack", string(debug.Stack()))
					r.Error = "the fix stopped unexpectedly, and nothing more was done"
				}
			}()
			c := findCheck(i.Check)
			switch {
			case c == nil || c.Fix == nil || !i.CanFix():
				r.Error = "this one has no fix to run: " + strings.Join(i.Steps, " ")
				return
			case i.Exclusive && !e.InServer && e.ServerUp():
				pid, _ := e.serverPID()
				r.Error = fmt.Sprintf("this fix can't run while the server is running (process %d). Stop the server and run this again, or use Settings, Repair, which does it at the next start.", pid)
				return
			}
			if err := fix.ensureCopy(ctx); err != nil {
				r.Error = "no safety copy could be made, so nothing was changed: " + err.Error()
				return
			}
			fix.Option, fix.restart = option, false
			done, err := c.Fix(ctx, e, i, fix)
			r.Copy = fix.copyDir
			r.Restart = fix.restart
			if err != nil {
				r.Error = err.Error()
				return
			}
			r.OK, r.Done = true, done
			e.log().Warn("a repair was made", "cat", "server", "check", i.Check, "title", i.Title)
		}()
		out = append(out, r)
	}
	return out
}

// Text writes a report for a person at a terminal.
func (r *DoctorReport) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "WittyMediaServer doctor, version %s\nData folder: %s\n", r.Version, r.Data)
	if r.ServerRunning {
		b.WriteString("A server is running on this folder.\n")
	}
	if len(r.Issues) == 0 {
		fmt.Fprintf(&b, "\nEverything looks fine: %d checks found nothing.\n", len(r.Checks))
		return b.String()
	}
	for _, a := range Areas {
		first := true
		for _, i := range r.Issues {
			if i.Area != a {
				continue
			}
			if first {
				fmt.Fprintf(&b, "\n%s\n", areaTitle(a))
				first = false
			}
			writeIssue(&b, &i)
		}
	}
	b.WriteString("\n" + r.Summary() + "\n")
	return b.String()
}

func writeIssue(b *strings.Builder, i *Issue) {
	fmt.Fprintf(b, "\n  [%s] %s\n", WordFor(i.Severity), i.Title)
	if i.What != "" {
		fmt.Fprintf(b, "    Found: %s\n", i.What)
	}
	if i.Fix != "" {
		fmt.Fprintf(b, "    Fix (%s): %s\n", SafetyWord(i.Safety), i.Fix)
	}
	for n, s := range i.Steps {
		fmt.Fprintf(b, "    %d. %s\n", n+1, s)
	}
	for _, d := range append(append([]string{}, i.Details...), i.Names...) {
		fmt.Fprintf(b, "      - %s\n", d)
	}
	if i.More > 0 {
		fmt.Fprintf(b, "      ...and %d more\n", i.More)
	}
}

// Summary is the last line.
func (r *DoctorReport) Summary() string {
	if r.Healthy() {
		if r.Notes > 0 {
			return fmt.Sprintf("Nothing needs fixing. %s to read above. %d checks ran.", countOf(r.Notes, "note", "notes"), len(r.Checks))
		}
		return fmt.Sprintf("Nothing needs fixing. %d checks ran.", len(r.Checks))
	}
	return fmt.Sprintf("%s and %s. %d can be fixed by this program, and the rest need you.", countOf(r.Problems, "problem", "problems"), countOf(r.Warnings, "warning", "warnings"), r.Fixable)
}

func countOf(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func areaTitle(a Area) string {
	switch a {
	case AreaData:
		return "THE DATABASE AND DATA FOLDER"
	case AreaMetadata:
		return "METADATA"
	case AreaModules:
		return "MODULES"
	}
	return "THE INSTALL"
}

// WordFor is the same word the page shows for a severity.
func WordFor(s Severity) string {
	switch s {
	case SevProblem:
		return "Problem"
	case SevWarn:
		return "Warning"
	case SevInfo:
		return "Note"
	}
	return "Fine"
}

// SafetyWord is the same word the page shows for a fix.
func SafetyWord(s Safety) string {
	switch s {
	case SafetySafe:
		return "safe"
	case SafetyCareful:
		return "backed up first"
	}
	return "you'll need to do this yourself"
}

// JSON writes a report for a program.
func (r *DoctorReport) JSON() ([]byte, error) { return json.MarshalIndent(r, "", "  ") }
