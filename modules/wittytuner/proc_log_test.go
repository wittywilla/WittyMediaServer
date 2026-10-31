package wittytuner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestProcReaderSaysHowItEnded(t *testing.T) {
	run := func(ctx context.Context, readToEnd bool) string {
		var out bytes.Buffer
		ctx, cancel := context.WithCancel(ctx)
		stderr := &stderrTail{}
		_, _ = stderr.Write([]byte("Connection refused\n"))
		p := &procReader{ReadCloser: io.NopCloser(strings.NewReader("data")), cancel: cancel, wait: func() error { return errors.New("exit status 1") },
			ctx: ctx, log: slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})), what: "ffmpeg", stderr: stderr, started: time.Now()}
		if readToEnd {
			_, _ = io.Copy(io.Discard, p)
		}
		p.Close()
		p.Close() // a second close doesn't wait or log again
		return out.String()
	}
	got := run(context.Background(), true)
	if !strings.Contains(got, "level=WARN") || !strings.Contains(got, "Connection refused") || !strings.Contains(got, "exit status 1") {
		t.Errorf("ended by itself: %s", got)
	}
	if strings.Count(got, "process ended") != 1 {
		t.Errorf("logged twice: %s", got)
	}
	if got := run(context.Background(), false); strings.Contains(got, "level=WARN") {
		t.Errorf("stopped on purpose: %s", got)
	}
	gone, cancel := context.WithCancel(context.Background())
	cancel()
	if got := run(gone, true); strings.Contains(got, "level=WARN") {
		t.Errorf("the caller left: %s", got)
	}
}

func TestStderrTailIsBounded(t *testing.T) {
	var s stderrTail
	for i := 0; i < 100; i++ {
		_, _ = s.Write(bytes.Repeat([]byte("x"), 4<<10))
		if len(s.b) > 2*stderrKeep {
			t.Fatalf("holding %d", len(s.b))
		}
	}
	_, _ = s.Write([]byte("the end"))
	if got := s.String(); len(got) > stderrKeep || !strings.HasSuffix(got, "the end") {
		t.Fatalf("%d bytes, ending %q", len(got), got[len(got)-10:])
	}
	_, _ = s.Write(bytes.Repeat([]byte("y"), 3*stderrKeep))
	if got := s.String(); len(got) != stderrKeep {
		t.Fatalf("after one huge write: %d", len(got))
	}
}
