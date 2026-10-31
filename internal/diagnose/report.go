package diagnose

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// Reports are JSON files in <data>/diagnostics. They hold structure, counts and
// base names of files, and never a full path or a byte of what's on a disc.

const (
	keepReports   = 20
	maxReportSize = 8 << 20
)

func reportDir(dataDir string) string { return filepath.Join(dataDir, "diagnostics") }

// validReportID accepts only what NewID("diag") makes, so an ID from a request
// can't name a file elsewhere.
func validReportID(id string) bool {
	return strings.HasPrefix(id, "diag-") && logging.ValidSession(id) == id && !strings.Contains(id, "..")
}

func saveReport(dataDir string, r *Report) error {
	dir := reportDir(dataDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".new-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, r.ID+".json")); err != nil {
		return err
	}
	pruneReports(dir)
	return nil
}

type reportFile struct {
	id  string
	mod time.Time
}

// reportFiles lists the saved reports, newest first.
func reportFiles(dir string) []reportFile {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []reportFile
	for _, e := range ents {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !validReportID(id) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, reportFile{id: id, mod: info.ModTime()})
	}
	slices.SortStableFunc(out, func(a, b reportFile) int { return b.mod.Compare(a.mod) })
	return out
}

func pruneReports(dir string) {
	files := reportFiles(dir)
	for i := keepReports; i < len(files); i++ {
		_ = os.Remove(filepath.Join(dir, files[i].id+".json")) // tried again after the next report
	}
}

func loadReport(dataDir, id string) (*Report, error) {
	if !validReportID(id) {
		return nil, errors.New("no such report")
	}
	f, err := os.Open(filepath.Join(reportDir(dataDir), id+".json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var r Report
	if err := json.NewDecoder(io.LimitReader(f, maxReportSize)).Decode(&r); err != nil {
		return nil, err
	}
	if r.Findings == nil {
		r.Findings = []Finding{}
	}
	return &r, nil
}

// listReports reads the saved reports, newest first, leaving out their findings.
func listReports(dataDir string) []Report {
	files := reportFiles(reportDir(dataDir))
	out := make([]Report, 0, len(files))
	for _, f := range files {
		r, err := loadReport(dataDir, f.id)
		if err != nil {
			continue // a damaged report isn't worth more than not listing it
		}
		r.Findings = nil
		out = append(out, *r)
	}
	return out
}

// ReportFor finds a finished job's report by ID: a job still running gives what
// it has so far.
func (m *Manager) ReportFor(id string) (*Report, bool) {
	if j := m.Get(id); j != nil {
		j.mu.Lock()
		defer j.mu.Unlock()
		return j.snapshotLocked(), true
	}
	r, err := loadReport(m.env.DataDir, id)
	if err != nil {
		return nil, false
	}
	return r, true
}

// Redact gives a copy with the disc's name, its label and every file name taken
// out, to share with someone.
func (r *Report) Redact() *Report {
	c := *r
	c.Redacted = true
	c.Label = ""
	if c.Kind == KindDisc {
		c.Subject = "a disc"
	}
	c.Findings = make([]Finding, len(r.Findings))
	for i, f := range r.Findings {
		if n := len(f.Names); n > 0 {
			f.More += n
			f.Names = nil
		}
		if f.TargetPlain != "" {
			f.Target = f.TargetPlain
		}
		f.TargetPlain = ""
		c.Findings[i] = f
	}
	return &c
}

var severityWords = map[Severity]string{SevProblem: "PROBLEM", SevWarn: "WARNING", SevInfo: "NOTE", SevOK: "OK"}

// Text is the report as plain text, to read or paste.
func (r *Report) Text() string {
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	line("WittyDiagnoser report %s", r.ID)
	if r.Version != "" {
		line("Server version: %s", r.Version)
	}
	what := "A disc"
	if r.Kind == KindStorage {
		what = "Storage"
	}
	line("Checked: %s: %s", what, r.Subject)
	if r.Label != "" {
		line("Label on the disc: %s", r.Label)
	}
	if len(r.Options) > 0 {
		var on []string
		for k, v := range r.Options {
			if v {
				on = append(on, k)
			}
		}
		slices.Sort(on)
		if len(on) > 0 {
			line("Options: %s", strings.Join(on, ", "))
		}
	}
	if r.Started > 0 {
		line("Started: %s", time.UnixMilli(r.Started).Format(time.RFC1123))
	}
	if r.Finished > 0 && r.Started > 0 {
		line("Took: %s", (time.Duration(r.Finished-r.Started) * time.Millisecond).Round(time.Second))
	}
	switch r.State {
	case StateCancelled:
		line("Stopped before it finished, so this is only part of the picture.")
	case StateRunning, StateQueued:
		line("Still running: this is only part of the picture.")
	}
	if r.Redacted {
		line("Names of discs and files were hidden in this copy.")
	}
	line("This check only reads. Nothing was changed.")
	line("")
	line("%s, %s, %s, %d fine", plural(r.Counts[SevProblem], "problem", "problems"), plural(r.Counts[SevWarn], "warning", "warnings"), plural(r.Counts[SevInfo], "note", "notes"), r.Counts[SevOK])
	fs := append([]Finding{}, r.Findings...)
	sortFindings(fs)
	for _, f := range fs {
		line("")
		if f.Target != "" {
			line("[%s] %s: %s", severityWords[f.Severity], f.Target, f.Title)
		} else {
			line("[%s] %s", severityWords[f.Severity], f.Title)
		}
		if f.What != "" {
			line("  %s", f.What)
		}
		if f.Fix != "" {
			line("  What to do: %s", f.Fix)
		}
		for _, d := range f.Details {
			line("    %s", d)
		}
		for _, n := range f.Names {
			line("    %s", n)
		}
		if f.More > 0 {
			line("    ...and %d more", f.More)
		}
	}
	if r.Dropped > 0 {
		line("")
		line("%d more findings were left out of this report.", r.Dropped)
	}
	return b.String()
}

// BundleFiles gives the newest few reports for a support bundle, with names
// hidden: a bundle gets passed around.
func (m *Manager) BundleFiles(n int) []logging.BundleFile {
	var out []logging.BundleFile
	for _, f := range reportFiles(reportDir(m.env.DataDir)) {
		if len(out) >= n {
			break
		}
		r, err := loadReport(m.env.DataDir, f.id)
		if err != nil {
			continue
		}
		b, err := json.MarshalIndent(r.Redact(), "", "  ")
		if err != nil {
			continue
		}
		out = append(out, logging.BundleFile{Name: "diagnostics/" + f.id + ".json", Description: "A WittyDiagnoser report, with names of discs and files hidden", Data: b})
	}
	return out
}
