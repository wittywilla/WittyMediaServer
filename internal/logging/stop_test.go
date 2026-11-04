package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNoteStopFirstWins(t *testing.T) {
	resetStop()
	t.Cleanup(resetStop)
	if !NoteStopIfNothingElse(StopTrayGone, "pipe closed") {
		t.Fatal("a guess with nothing before it is kept")
	}
	if !NoteStop(StopShutdown, "", "") {
		t.Fatal("an exact reason replaces a guess")
	}
	if NoteStop(StopSignal, "SIGTERM", "") || NoteStopIfNothingElse(StopTrayGone, "") {
		t.Fatal("what came first stays")
	}
	if s := StopNoted(); s.Reason != StopShutdown || s.Words() != "the computer shut down or restarted" || s.At.IsZero() {
		t.Fatalf("%+v %q", s, s.Words())
	}
	resetStop()
	NoteStop(StopFatal, "the database couldn't be opened: locked", "")
	if w := StopNoted().Words(); !strings.Contains(w, "database couldn't be opened") {
		t.Fatal(w)
	}
}

// Every reason on the list has words for people.
func TestEveryStopHasWords(t *testing.T) {
	for _, r := range []string{StopTrayQuit, StopTrayStop, StopTrayRestart, StopTrayClosed, StopTrayGone, StopService, StopShutdown,
		StopLogoff, StopInstaller, StopUpdate, StopRestartSetting, StopSignal, StopConsoleClosed, StopAPI, StopFatal} {
		if stopWords[r] == "" {
			t.Errorf("%s has no words", r)
		}
	}
	if (Stop{}).Words() != "no reason was given" {
		t.Error("an empty reason")
	}
}

func TestUptime(t *testing.T) {
	for d, want := range map[time.Duration]string{
		12 * time.Second:               "12s",
		5 * time.Minute:                "5m",
		2*time.Hour + 38*time.Minute:   "2h38m",
		4*24*time.Hour + 3*time.Hour:   "4d3h",
		47*time.Hour + 59*time.Minute:  "47h59m",
		100*24*time.Hour + time.Minute: "100d0h",
	} {
		if got := Uptime(d); got != want {
			t.Errorf("%v: %s, want %s", d, got, want)
		}
	}
}

// A clean stop is recorded with its reason, and the next start reads it.
func TestCleanStopRecorded(t *testing.T) {
	resetStop()
	t.Cleanup(resetStop)
	dir := t.TempDir()
	crashDir, logDir := filepath.Join(dir, "crash"), filepath.Join(dir, "logs")
	g, found, err := StartCrashGuard(crashDir, logDir, "one")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := g.Previous(); ok || len(found) != 0 {
		t.Fatalf("a first start has no previous run: %+v", found)
	}
	NoteStop(StopTrayQuit, "said by the tray app", "")
	// The marker says it's stopping, in case the stop never finishes.
	var m runMarker
	b, _ := os.ReadFile(filepath.Join(crashDir, fmt.Sprintf("run-%d.json", os.Getpid())))
	if err := json.Unmarshal(b, &m); err != nil || m.Stopping == nil || m.Stopping.Reason != StopTrayQuit {
		t.Fatalf("marker: %s %v", b, err)
	}
	g.Close()
	resetStop()

	g, found, err = StartCrashGuard(crashDir, logDir, "two")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	prev, ok := g.Previous()
	if !ok || !prev.Clean || prev.Stop.Reason != StopTrayQuit || prev.Version != "one" || len(found) != 0 {
		t.Fatalf("previous: %+v %v, reports %+v", prev, ok, found)
	}
	if s := prev.Summary(); !strings.HasPrefix(s, "stopped ") || !strings.Contains(s, "(tray-quit), ran ") {
		t.Fatal(s)
	}
	if runs := Runs(crashDir); len(runs) != 1 || runs[0].Stop.Reason != StopTrayQuit {
		t.Fatalf("runs: %+v", runs)
	}
}

// A stop that began and never finished (hung, then ended by force) says what it was doing.
func TestStopThatDidntFinish(t *testing.T) {
	resetStop()
	t.Cleanup(resetStop)
	dir := t.TempDir()
	crashDir := filepath.Join(dir, "crash")
	if err := os.MkdirAll(crashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const pid = 999999 // not running
	st := &Stop{Reason: StopService, Detail: "systemd", At: time.Now().Add(-time.Minute)}
	b, _ := json.Marshal(runMarker{Version: "last", Started: time.Now().Add(-3 * time.Hour), PID: pid, LogDir: dir, Stopping: st})
	if err := os.WriteFile(filepath.Join(crashDir, fmt.Sprintf("run-%d.json", pid)), b, 0o640); err != nil {
		t.Fatal(err)
	}
	// What it wrote to standard error, where nothing else kept it.
	if err := os.WriteFile(filepath.Join(crashDir, stderrName(pid)), []byte("goroutine 1 [semacquire]:\nmain.stuck()\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	g, found, err := StartCrashGuard(crashDir, dir, "next")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if len(found) != 1 || found[0].Kind != "unclean" || !strings.Contains(found[0].Summary, "began to stop (service-stop: systemd)") {
		t.Fatalf("reports: %+v", found)
	}
	body, err := g.Read(found[0].ID)
	if err != nil || !strings.Contains(string(body), "main.stuck()") || !strings.Contains(string(body), "the service was stopped") {
		t.Fatalf("report: %s %v", body, err)
	}
	if _, err := os.Stat(filepath.Join(crashDir, stderrName(pid))); !os.IsNotExist(err) {
		t.Fatal("the earlier run's stderr file stayed after it went into the report")
	}
	prev, ok := g.Previous()
	if !ok || prev.Clean || prev.Report != found[0].ID || prev.Stop.Reason != StopService {
		t.Fatalf("previous: %+v", prev)
	}
	if s := prev.Summary(); !strings.Contains(s, "was stopping at") || !strings.Contains(s, "(service-stop) but didn't finish") {
		t.Fatal(s)
	}
}

// A run that just vanished.
func TestPreviousUnclean(t *testing.T) {
	resetStop()
	t.Cleanup(resetStop)
	dir := t.TempDir()
	crashDir := filepath.Join(dir, "crash")
	_ = os.MkdirAll(crashDir, 0o755)
	const pid = 999998
	b, _ := json.Marshal(runMarker{Version: "last", Started: time.Now().Add(-time.Hour), PID: pid, LogDir: dir})
	_ = os.WriteFile(filepath.Join(crashDir, fmt.Sprintf("run-%d.json", pid)), b, 0o640)
	g, _, err := StartCrashGuard(crashDir, dir, "next")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	prev, ok := g.Previous()
	if !ok || prev.Clean || prev.Report == "" || !strings.HasPrefix(prev.Summary(), "ended without stopping cleanly") {
		t.Fatalf("%+v %q", prev, prev.Summary())
	}
}

func TestRunsKeepsTheLastTwenty(t *testing.T) {
	g := &CrashGuard{dir: t.TempDir()}
	for i := range 25 {
		g.record(LastRun{PID: i, Started: time.Now(), Clean: true})
	}
	runs := Runs(g.dir)
	if len(runs) != keepRuns || runs[0].PID != 24 || runs[keepRuns-1].PID != 5 {
		t.Fatalf("%d runs, newest %d", len(runs), runs[0].PID)
	}
	if last, ok := readLastRun(g.dir); !ok || last.PID != 24 {
		t.Fatalf("%+v", last)
	}
}
