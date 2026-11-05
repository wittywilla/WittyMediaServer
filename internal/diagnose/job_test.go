package diagnose

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// A job is read while it runs: the settings page asks for it every second, the
// list of checks shows it, and its report can be downloaded part-way. Each is a
// copy taken under the job's lock and encoded after, so the copy must share nothing
// the job goes on to change. The lines here go to every finding, as one drive's
// health lines go to every folder on it. Run with -race.
func TestDiagnoseJobIsReadWhileItAddsFindings(t *testing.T) {
	const private = "/srv/Private Media"
	lines := []string{"Model: the drive under " + private, "Status: fine"}
	names := []string{"Private Name.mkv"}
	m := NewManager(Env{DataDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	enough := make(chan struct{})
	j, err := m.Submit(KindStorage, "1 folder", nil, []scrubRule{{path: private, with: "this folder"}}, func(ctx context.Context, j *Job) {
		for i := 0; ; i++ {
			j.Add(Finding{Severity: SevOK, Title: "The drive is fine", Details: lines, Names: names})
			j.Step("adding", float64(i%maxFindings)/maxFindings)
			select {
			case <-enough:
				return
			default:
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	encode := func(what string, v any) {
		t.Helper()
		if _, err := json.Marshal(v); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	// Until the report is saved, which is the last the job writes to its folder.
	deadline := time.Now().Add(2 * time.Minute)
	for reads := 1; ; reads++ {
		v, ok := m.JobView(j.ID)
		if !ok {
			t.Fatal("the job went missing")
		}
		encode("the job", v)
		encode("the list", m.Jobs())
		if r, ok := m.ReportFor(j.ID); ok {
			encode("the report", r)
			_ = r.Redact().Text()
		}
		if reads == 50 {
			close(enough)
		}
		if v.Saved {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("still %s after %d reads", v.State, reads)
		}
	}

	if lines[0] != "Model: the drive under "+private || names[0] != "Private Name.mkv" {
		t.Errorf("the caller's lists were changed: %q %q", lines, names)
	}
	v, _ := m.JobView(j.ID)
	if v.State != StateDone || len(v.Findings) < 2 {
		t.Fatalf("%s, %d findings", v.State, len(v.Findings))
	}
	for _, f := range v.Findings {
		if len(f.Details) != 2 || f.Details[0] != "Model: the drive under this folder" || strings.Contains(strings.Join(f.Details, " "), private) {
			t.Fatalf("details: %q", f.Details)
		}
	}
}
