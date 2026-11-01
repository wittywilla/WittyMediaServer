package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestLog(t *testing.T, data string) {
	t.Helper()
	logs := filepath.Join(data, "logs")
	if err := os.MkdirAll(logs, 0o750); err != nil {
		t.Fatal(err)
	}
	line := `time=2026-11-01T10:00:00.000Z level=WARN cat=playback msg="could not open" file="/mnt/planted-disk/Family Films/clip.mkv" title="A Very Secret Movie Title" user=plantedaccount from=192.168.40.17 err="exit status 1"` + "\n"
	if err := os.WriteFile(filepath.Join(logs, "wms-2026-11-01.log"), []byte(line), 0o640); err != nil {
		t.Fatal(err)
	}
}

func zipText(t *testing.T, p string) string {
	t.Helper()
	zr, err := zip.OpenReader(p)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var all strings.Builder
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		all.WriteString(f.Name + "\n" + string(b) + "\n")
	}
	return all.String()
}

// With no server, and with a database that's nothing of the kind, the logs still come out.
func TestLogsZipWithABrokenDatabase(t *testing.T) {
	data := t.TempDir()
	writeTestLog(t, data)
	if err := os.WriteFile(filepath.Join(data, "wms.db"), []byte("this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(data, "wms.db"))
	out := filepath.Join(t.TempDir(), "logs.zip")
	if code := cmdLogs([]string{"zip", "-data", data, "-o", out, "-days", "14"}); code != 0 {
		t.Fatal(code)
	}
	text := zipText(t, out)
	for _, v := range []string{"planted-disk", "Family Films", "A Very Secret", "plantedaccount", "192.168.40.17"} {
		if strings.Contains(text, v) {
			t.Errorf("%q survived:\n%s", v, text)
		}
	}
	for _, want := range []string{"logs/wms-2026-11-01.log", "summary.json", "exit status 1", "SANITIZED", "level=WARN"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q is missing:\n%s", want, text)
		}
	}
	after, _ := os.ReadFile(filepath.Join(data, "wms.db"))
	if string(before) != string(after) {
		t.Error("the database was touched")
	}
	// It won't write over something already there.
	if code := cmdLogs([]string{"zip", "-data", data, "-o", out}); code == 0 {
		t.Error("it wrote over a file")
	}
}

func TestLogsZipRaw(t *testing.T) {
	data := t.TempDir()
	writeTestLog(t, data)
	out := filepath.Join(t.TempDir(), "logs.zip")
	if code := cmdLogs([]string{"zip", "-data", data, "-o", out, "-days=14", "--raw"}); code != 0 {
		t.Fatal(code)
	}
	if text := zipText(t, out); !strings.Contains(text, "plantedaccount") || !strings.Contains(text, "NOT sanitized") {
		t.Error(text)
	}
}

func TestLogsSanitizeFile(t *testing.T) {
	data := t.TempDir()
	writeTestLog(t, data)
	dir := t.TempDir()
	out := filepath.Join(dir, "clean.txt")
	if code := cmdLogs([]string{"sanitize", "-data", data, "-o", out, filepath.Join(data, "logs", "wms-2026-11-01.log")}); code != 0 {
		t.Fatal(code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "plantedaccount") || !strings.Contains(string(b), "[user-") {
		t.Error(string(b))
	}
	if code := cmdLogs([]string{"sanitize", "-data", data}); code != 2 {
		t.Error("no file should be a usage error", code)
	}
	if code := cmdLogs([]string{"nonsense"}); code != 2 {
		t.Error(code)
	}
}
