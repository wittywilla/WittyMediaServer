package hls

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// max is what "maximum concurrent transcodes" promises. Without it, anyone
// signed in could ask for hundreds of variants of a title, each with its own
// FFmpeg process, and take the machine down.
func TestTranscodeLimit(t *testing.T) {
	sleeper := helperCommand(t)
	m := &Manager{tools: &ffmpeg.Tools{FFmpeg: sleeper}, root: t.TempDir(), log: slog.New(slog.NewTextHandler(io.Discard, nil)), session: map[string]*Session{}, max: 2, running: map[*job]*Session{}}
	defer m.StopAll()
	start := func(id string) (*Session, error) {
		s := m.Ensure(Spec{ID: id, Duration: 100, AudioIndex: -1, SubtitleIndex: -1})
		s.mu.Lock()
		defer s.mu.Unlock()
		_, err := s.startLocked(0)
		return s, err
	}
	a, err := start("a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := start("b"); err != nil {
		t.Fatal(err)
	}
	if _, err := start("c"); !errors.Is(err, ErrBusy) {
		t.Fatalf("third transcode: %v, want ErrBusy", err)
	}
	// One nobody watches any more makes way.
	a.used.Store(time.Now().Add(-time.Minute).UnixNano())
	if _, err := start("c"); err != nil {
		t.Fatalf("after a stream went idle: %v", err)
	}
	m.jobMu.Lock()
	n := len(m.running)
	m.jobMu.Unlock()
	if n > 2 {
		t.Fatalf("%d processes running", n)
	}
}

func TestSessionsAreBounded(t *testing.T) {
	m := &Manager{tools: &ffmpeg.Tools{}, root: t.TempDir(), log: slog.New(slog.NewTextHandler(io.Discard, nil)), session: map[string]*Session{}, max: 2, running: map[*job]*Session{}}
	for i := 0; i < maxSessions+50; i++ {
		m.Ensure(Spec{ID: fmt.Sprint("s", i), Duration: 10})
	}
	if len(m.session) > maxSessions {
		t.Fatalf("%d sessions kept", len(m.session))
	}
}

// helperCommand is a program that runs for a while whatever its arguments, to
// stand in for FFmpeg: this test binary, told to wait by an environment variable.
func helperCommand(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WMS_HLS_TEST_SLEEP", "1")
	return exe
}

func TestMain(m *testing.M) {
	if os.Getenv("WMS_HLS_TEST_SLEEP") == "1" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	os.Exit(m.Run())
}
