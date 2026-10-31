package ffmpeg

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func captureDefault(t *testing.T) *lockedBuffer {
	t.Helper()
	var out lockedBuffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &out
}

func TestRunTiesAFailureToTheSession(t *testing.T) {
	out := captureDefault(t)
	ctx := logging.WithSession(context.Background(), "v-abc")
	if _, _, err := Run(ctx, "wms-no-such-program", []string{"-i", "x"}, "track", 3); err == nil {
		t.Fatal("a program that isn't there should fail")
	}
	got := out.String()
	for _, want := range []string{"level=WARN", "cat=transcode", "session=v-abc", "track=3", "not found"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in: %s", want, got)
		}
	}
}

// A viewer leaving stops the job: that is no failure.
func TestRunStoppedByItsContextIsQuiet(t *testing.T) {
	out := captureDefault(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := Run(ctx, "wms-no-such-program", nil); err == nil {
		t.Fatal("should fail")
	}
	if got := out.String(); !strings.Contains(got, "level=DEBUG") || strings.Contains(got, "level=WARN") {
		t.Errorf("not a debug line: %s", got)
	}
}

func TestRedactArgs(t *testing.T) {
	got := RedactArgs([]string{"-headers", "X-Secret: abc\r\n", "-i", "http://me:pw@tuner/1", "-y"})
	if strings.Contains(got, "abc") || strings.Contains(got, "pw") {
		t.Errorf("a secret is in the log: %s", got)
	}
	if !strings.Contains(got, "http://(hidden)@tuner/1") || !strings.HasSuffix(got, " -y") {
		t.Errorf("too much is gone: %s", got)
	}
}
