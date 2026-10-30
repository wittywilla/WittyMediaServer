package logging

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, l *Logger) string {
	t.Helper()
	l.Flush()
	var b strings.Builder
	for _, f := range l.Files() {
		data, _ := os.ReadFile(filepath.Join(l.Dir(), f.Name))
		b.Write(data)
	}
	return b.String()
}

func TestCategoriesAndRedaction(t *testing.T) {
	l, err := New(t.TempDir(), slog.LevelInfo, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	log := slog.New(l.Handler())

	log.Info("plain info")
	log.Debug("plain debug")
	log.Info("dvd step", "cat", "dvd")
	log.With("cat", "ffmpeg").Debug("ffmpeg start", "args", "-i x.mkv")
	log.Warn("dvd warning", "cat", "dvd")
	out := read(t, l)
	for _, want := range []string{"plain info", "dvd warning"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, not := range []string{"plain debug", "dvd step", "ffmpeg start"} {
		if strings.Contains(out, not) {
			t.Errorf("%q logged although its category is off", not)
		}
	}

	l.SetCategories([]string{"dvd", "ffmpeg", "bogus"})
	if got := strings.Join(l.Enabled(), ","); got != "dvd,ffmpeg" {
		t.Errorf("enabled = %s", got)
	}
	log.Info("dvd step 2", "cat", "dvd")
	log.With("cat", "ffmpeg").Debug("ffmpeg start 2")
	log.Debug("plain debug 2")
	log.Info("request", "url", "/api/hls/x?token=abc123&x=1", "password", "hunter2", "auth", "Bearer eyJabc.def")
	out = read(t, l)
	for _, want := range []string{"dvd step 2", "ffmpeg start 2", "token=[hidden]"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	for _, not := range []string{"plain debug 2", "abc123", "hunter2", "eyJabc"} {
		if strings.Contains(out, not) {
			t.Errorf("%q should not be in the log", not)
		}
	}
}

// An HTTP client's error quotes the address it asked, API key and all, and it
// reaches the log as an error value rather than text.
func TestErrorValuesAreRedacted(t *testing.T) {
	l, err := New(t.TempDir(), slog.LevelInfo, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	slog.New(l.Handler()).Warn("lookup failed", "err", errors.New(`Get "https://api.example.com/3/search?api_key=abc123&query=x": timeout`))
	if got := read(t, l); strings.Contains(got, "abc123") || !strings.Contains(got, "timeout") {
		t.Fatalf("log: %s", got)
	}
}

func TestLogNames(t *testing.T) {
	for name, ok := range map[string]bool{"wms-2026-10-29.log": true, "wms-2026-10-29-2.log": true, "../wms-x.log": false, `a\wms-x.log`: false, "other.log": false} {
		if IsLogName(name) != ok {
			t.Errorf("%s: want %v", name, ok)
		}
	}
}
