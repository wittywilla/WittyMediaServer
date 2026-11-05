package logging

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func unzip(t *testing.T, b []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(r)
		r.Close()
		out[f.Name] = string(data)
	}
	return out
}

// Issue #73: a bundle made after a reset said nothing about crashes, held nothing from
// before the reset, and gave an empty date.
func TestBundleSaysWhatItHolds(t *testing.T) {
	l, log := newTest(t)
	log.Warn("today's line", "cat", "server")
	l.Flush()
	guard := OpenCrashReports(filepath.Join(t.TempDir(), "crash"))

	// What a reset set aside: its logs and a crash report.
	data := t.TempDir()
	copyName := "before-reset-20261102-165543"
	oldLogs := filepath.Join(data, copyName, "logs")
	oldCrash := filepath.Join(data, copyName, "crash")
	for _, d := range []string{oldLogs, oldCrash} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	day := time.Now().Add(-2 * time.Hour)
	line := (&Record{Time: day, Lv: slog.LevelWarn, Cat: "server", Msg: "from before the reset"}).appendLine(nil)
	oldFile := filepath.Join(oldLogs, "wms-"+day.Format("2006-01-02")+".log")
	if err := os.WriteFile(oldFile, line, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(oldFile, day, day)
	ancient := filepath.Join(oldLogs, "wms-2020-01-01.log")
	_ = os.WriteFile(ancient, line, 0o644)
	old := time.Date(2020, 1, 1, 12, 0, 0, 0, time.Local)
	_ = os.Chtimes(ancient, old, old)
	if err := os.WriteFile(filepath.Join(oldCrash, "crash-20261102T120000.000Z-panic.txt"), []byte("WittyMediaServer crash report\nKind: panic\n\n\n\n\nA panic before the reset\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	write := func(o BundleOptions) map[string]string {
		var b bytes.Buffer
		if err := l.WriteBundle(&b, guard, o); err != nil {
			t.Fatal(err)
		}
		return unzip(t, b.Bytes())
	}

	// No reports at all: that's said, not left to wonder about.
	files := write(BundleOptions{})
	if !strings.Contains(files["crashes/none.txt"], "No crash reports") {
		t.Fatalf("no note that there are no crash reports: %v", keys(files))
	}
	if !strings.Contains(files["README.txt"], "No crash reports") || !strings.Contains(files["README.txt"], "Log files last written between") {
		t.Fatalf("the readme doesn't say what it holds:\n%s", files["README.txt"])
	}

	// With what a reset set aside: in, under the copy's name, and named in the readme.
	files = write(BundleOptions{Earlier: []Earlier{
		{Name: copyName, Logs: oldLogs, Crashes: oldCrash},
		{Name: "../escape", Logs: oldLogs},
	}})
	got := files["logs/"+copyName+"/"+filepath.Base(oldFile)]
	if !strings.Contains(got, "from before the reset") {
		t.Fatalf("the logs from before the reset aren't in: %v", keys(files))
	}
	if _, ok := files["logs/"+copyName+"/wms-2020-01-01.log"]; ok {
		t.Fatal("a log older than the days asked for went in")
	}
	if !strings.Contains(files["crashes/"+copyName+"/crash-20261102T120000.000Z-panic.txt"], "A panic before the reset") {
		t.Fatalf("the crash report from before the reset isn't in: %v", keys(files))
	}
	if _, ok := files["crashes/none.txt"]; ok {
		t.Fatal("says there are none, with one from before the reset")
	}
	readme := files["README.txt"]
	for _, want := range []string{"From before a reset, in " + copyName + ": 1 log files and 1 crash reports", "1 older log files weren't included", "1 crash reports."} {
		if !strings.Contains(readme, want) {
			t.Errorf("the readme doesn't say %q:\n%s", want, readme)
		}
	}
	for name := range files {
		if strings.Contains(name, "..") || strings.Contains(name, "escape") {
			t.Fatalf("a copy name made its way into the zip: %s", name)
		}
	}
	// The preview lists the same.
	p := l.Preview(guard, BundleOptions{Earlier: []Earlier{{Name: copyName, Logs: oldLogs, Crashes: oldCrash}}})
	var listed bool
	for _, part := range p.Parts {
		if part.Name == "logs/"+copyName+"/"+filepath.Base(oldFile) {
			listed = true
		}
	}
	if !listed {
		t.Fatalf("the preview doesn't list it: %+v", p.Parts)
	}
}

// The date a problem with the log began is only there when there's a problem.
func TestHealthHasNoEmptyDate(t *testing.T) {
	b, err := json.Marshal(Health{Writing: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "since") || strings.Contains(string(b), "0001-01-01") {
		t.Fatalf("%s", b)
	}
	b, _ = json.Marshal(Health{Error: "full", Since: time.Now()})
	if !strings.Contains(string(b), `"since":"`) {
		t.Fatalf("%s", b)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
