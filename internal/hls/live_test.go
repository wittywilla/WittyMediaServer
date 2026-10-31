package hls

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

func liveTestManager(t *testing.T) *Manager {
	t.Helper()
	// Not t.TempDir: sessions let go of their files a moment after stopping.
	root, err := os.MkdirTemp("", "wms-live")
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{tools: &ffmpeg.Tools{Encoders: map[string]bool{"libx264": true}}, root: root, log: slog.Default(), session: map[string]*Session{}, max: 2}
	t.Cleanup(func() {
		m.StopAllLive()
		time.AfterFunc(time.Second, func() { _ = os.RemoveAll(root) })
	})
	return m
}

// fakeRun stands in for FFmpeg: it adds segments as if it had produced them.
func fakeRun(segments int) func(context.Context, *LiveSession, *LiveInput, int, bool) error {
	return func(ctx context.Context, s *LiveSession, in *LiveInput, run int, burst bool) error {
		for i := range segments {
			name := "r" + string(rune('a'+run)) + "-" + string(rune('a'+i)) + ".ts"
			_ = os.WriteFile(filepath.Join(s.Dir, name), []byte("x"), 0o644)
			s.addSegment(name, 4)
		}
		<-ctx.Done()
		return nil
	}
}

func startFake(t *testing.T, m *Manager, spec LiveSpec, run func(context.Context, *LiveSession, *LiveInput, int, bool) error) *LiveSession {
	t.Helper()
	m.live.sessions = map[string]*LiveSession{}
	s := m.newLive(spec)
	s.runner = run
	m.live.sessions[spec.ID] = s
	go s.loop()
	return s
}

func TestLivePlaylistSlidesAndKeepsSequence(t *testing.T) {
	m := liveTestManager(t)
	s := m.newLive(LiveSpec{ID: "c", WindowSec: 10})
	for i := range 6 {
		s.addSegment("s"+string(rune('0'+i))+".ts", 4)
	}
	p := string(s.Playlist(func(f string) string { return "seg/" + f }))
	if !strings.Contains(p, "#EXT-X-MEDIA-SEQUENCE:3") {
		t.Fatalf("the window should have moved on:\n%s", p)
	}
	if strings.Contains(p, "s0.ts") || !strings.Contains(p, "seg/s5.ts") {
		t.Fatalf("wrong segments:\n%s", p)
	}
	if strings.Contains(p, "ENDLIST") {
		t.Fatal("a live playlist never ends")
	}
	if _, ok := s.SegmentPath("s0.ts"); ok {
		t.Fatal("a segment out of the window should not be served")
	}
	if _, ok := s.SegmentPath("../../secret"); ok {
		t.Fatal("only known segment names may be served")
	}
}

func TestLiveDiscontinuityBetweenInputs(t *testing.T) {
	m := liveTestManager(t)
	s := m.newLive(LiveSpec{ID: "c", WindowSec: 100})
	s.addSegment("a.ts", 4)
	s.nextDisc = true
	s.addSegment("b.ts", 4)
	p := string(s.Playlist(func(f string) string { return f }))
	if !strings.Contains(p, "#EXT-X-DISCONTINUITY\nb.ts") && !strings.Contains(p, "#EXT-X-DISCONTINUITY\n#EXTINF:4.000,\nb.ts") {
		t.Fatalf("no discontinuity before the new input:\n%s", p)
	}
	if strings.Contains(p, "#EXT-X-DISCONTINUITY\n#EXTINF:4.000,\na.ts") {
		t.Fatalf("no discontinuity before the first segment:\n%s", p)
	}
}

func TestLiveDiscontinuitySequenceCounts(t *testing.T) {
	m := liveTestManager(t)
	s := m.newLive(LiveSpec{ID: "c", WindowSec: 8})
	s.addSegment("a.ts", 4)
	s.nextDisc = true
	s.addSegment("b.ts", 4)
	s.addSegment("c.ts", 4)
	s.addSegment("d.ts", 4)
	s.addSegment("e.ts", 4)
	p := string(s.Playlist(func(f string) string { return f }))
	if !strings.Contains(p, "#EXT-X-DISCONTINUITY-SEQUENCE:1") {
		t.Fatalf("a discontinuity left the window:\n%s", p)
	}
}

func TestLiveSharedAndLimit(t *testing.T) {
	m := liveTestManager(t)
	spec := LiveSpec{ID: "a", Feed: func(ctx context.Context, at time.Time) (*LiveInput, error) { return &LiveInput{AudioIndex: 0}, nil }}
	a := startFake(t, m, spec, fakeRun(3))
	// The same channel: the same session.
	b, err := m.EnsureLive(spec)
	if err != nil || a != b {
		t.Fatalf("viewers should share a session: %v", err)
	}
	spec2 := spec
	spec2.ID = "b"
	if _, err := m.EnsureLive(spec2); err != nil {
		t.Fatal(err)
	}
	spec3 := spec
	spec3.ID = "c"
	if _, err := m.EnsureLive(spec3); !errors.Is(err, ErrLiveBusy) {
		t.Fatalf("over the limit: %v", err)
	}
}

func TestLiveWaitReadyAndStopCleansUp(t *testing.T) {
	m := liveTestManager(t)
	var stopped atomic.Int32
	s := startFake(t, m, LiveSpec{ID: "x", OnStop: func() { stopped.Add(1) },
		Feed: func(ctx context.Context, at time.Time) (*LiveInput, error) { return &LiveInput{AudioIndex: 0}, nil }}, fakeRun(3))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	m.StopLive("x")
	if stopped.Load() != 1 {
		t.Fatal("OnStop should run once")
	}
	if m.GetLive("x") != nil {
		t.Fatal("still listed")
	}
	for i := 0; i < 40; i++ {
		if _, err := os.Stat(s.Dir); os.IsNotExist(err) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("segments were not deleted")
}

func TestLiveFatalErrorEndsSession(t *testing.T) {
	m := liveTestManager(t)
	refused := errors.New("encrypted")
	s := startFake(t, m, LiveSpec{ID: "x", Feed: func(ctx context.Context, at time.Time) (*LiveInput, error) { return &LiveInput{AudioIndex: 0}, nil }},
		func(ctx context.Context, s *LiveSession, in *LiveInput, run int, burst bool) error {
			return &FatalError{refused}
		})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.WaitReady(ctx); !errors.Is(err, refused) {
		t.Fatalf("want the reason, got %v", err)
	}
}

func TestLiveFeedErrorIsReported(t *testing.T) {
	m := liveTestManager(t)
	bad := errors.New("no such file")
	s := startFake(t, m, LiveSpec{ID: "x", Feed: func(ctx context.Context, at time.Time) (*LiveInput, error) { return nil, bad }}, fakeRun(1))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.WaitReady(ctx); !errors.Is(err, bad) {
		t.Fatalf("got %v", err)
	}
}

func TestLiveLeaveStopsAChannelNobodyWatches(t *testing.T) {
	m := liveTestManager(t)
	s := startFake(t, m, LiveSpec{ID: "x", Feed: func(ctx context.Context, at time.Time) (*LiveInput, error) { return &LiveInput{AudioIndex: 0}, nil }}, fakeRun(3))
	s.Viewer("a")
	s.Viewer("b")
	if !s.watched() {
		t.Fatal("two viewers are watching")
	}
	m.LeaveLive("x", "a")
	time.Sleep(leaveGrace + 300*time.Millisecond)
	if m.GetLive("x") == nil {
		t.Fatal("someone else is still watching")
	}
	m.LeaveLive("x", "b")
	// The same person's next player can take over within the grace.
	s.Viewer("c")
	time.Sleep(leaveGrace + 300*time.Millisecond)
	if m.GetLive("x") == nil {
		t.Fatal("a new viewer arrived in time")
	}
	m.LeaveLive("x", "c")
	time.Sleep(leaveGrace + 300*time.Millisecond)
	if m.GetLive("x") != nil {
		t.Fatal("the last viewer left: the channel should stop")
	}
	// A player that gave no name stays until it goes quiet.
	s2 := startFake(t, m, LiveSpec{ID: "y", Feed: func(ctx context.Context, at time.Time) (*LiveInput, error) { return &LiveInput{AudioIndex: 0}, nil }}, fakeRun(3))
	s2.Viewer("")
	s2.Viewer("n")
	m.LeaveLive("y", "n")
	time.Sleep(leaveGrace + 300*time.Millisecond)
	if m.GetLive("y") == nil {
		t.Fatal("an unnamed player is still watching")
	}
}

func TestLiveArgs(t *testing.T) {
	m := liveTestManager(t)
	s := m.newLive(LiveSpec{ID: "x", MaxHeight: 720})
	hasBurst = false
	readrateOnce.Do(func() {})
	file := strings.Join(s.liveArgs(&LiveInput{InputArgs: ffmpeg.FileInput("/m/a.mkv"), Seek: 90, Duration: 30, AudioIndex: 0}, 1, true), " ")
	for _, want := range []string{"-re", "-ss 90.000", "-t 30.000", "-c:a aac", "-ar 48000", "scale=-2:'min(ih,720)'", "-f hls"} {
		if !strings.Contains(file, want) {
			t.Errorf("file run lacks %q:\n%s", want, file)
		}
	}
	live := s.liveArgs(&LiveInput{InputArgs: []string{"-f", "mpegts", "-i", "pipe:0"}, Stdin: os.Stdin, AudioIndex: 0, Realtime: true, CopyVideo: true}, 2, false)
	joined := strings.Join(live, " ")
	if strings.Contains(joined, "-re") || strings.Contains(joined, "-nostdin") {
		t.Errorf("a tuner is already real time and must read standard input:\n%s", joined)
	}
	if !strings.Contains(joined, "-c:v copy") {
		t.Errorf("copy mode: %s", joined)
	}
	slate := strings.Join(s.liveArgs(&LiveInput{InputArgs: SlateInput(), Slate: true, Duration: 10, AudioIndex: -1}, 3, false), " ")
	if !strings.Contains(slate, "anullsrc") || !strings.Contains(slate, "-map 1:a:0") {
		t.Errorf("a slate has silence:\n%s", slate)
	}
}

func TestRedactArgs(t *testing.T) {
	got := redactArgs([]string{"-headers", "Authorization: Basic abc", "-i", "http://me:secret@host/x"})
	if strings.Contains(got, "secret") || strings.Contains(got, "abc") {
		t.Fatalf("logged a login: %s", got)
	}
}

// Opening a growing stream must not hold the manager's lock while it starts:
// starting asks the manager for room, and every other stream waits on that lock.
func TestEnsureGrowingDoesNotBlockTheManager(t *testing.T) {
	m := liveTestManager(t)
	done := make(chan *Session, 1)
	go func() { done <- m.Ensure(Spec{ID: "grow", Growing: true, CopyVideo: true, Duration: 20}) }()
	select {
	case s := <-done:
		if m.Get("grow") != s {
			t.Error("the stream was not kept")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Ensure never returned: the manager's lock is held for good")
	}
}

// Growing streams have their own limit, so a busy server still opens one.
func TestGrowingStreamsSkipTheTranscodeLimit(t *testing.T) {
	m := liveTestManager(t)
	m.max = 0
	if err := m.makeRoom(&Session{Spec: Spec{ID: "grow", Growing: true}, m: m}); err != nil {
		t.Errorf("makeRoom: %v", err)
	}
}
