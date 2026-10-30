package hls

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

func testManager(t *testing.T, max int) *Manager {
	t.Helper()
	return &Manager{tools: &ffmpeg.Tools{}, root: t.TempDir(), log: slog.New(slog.NewTextHandler(io.Discard, nil)), session: map[string]*Session{}, max: max}
}

// running gives a session a job that looks like a live FFmpeg.
func running(s *Session, used time.Time) {
	s.job = &job{cmd: &exec.Cmd{}, done: make(chan struct{})}
	s.lastUsed = used
}

func TestCleanRootOnlyTakesItsOwnFolders(t *testing.T) {
	root := t.TempDir()
	mine := []string{"v-0123abcd-a0-s-1-h0-c2-aac-abc123", "dvd-0123-m-VMGM-0-1-1-a0", "bd-0123-p00001-a0-g1"}
	for _, d := range mine {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Someone may point the transcode folder at a folder that holds other things.
	theirs := []string{"Movies", "video-project"}
	for _, d := range theirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(root, "v-notes.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cleanRoot(root)
	for _, d := range mine {
		if _, err := os.Stat(filepath.Join(root, d)); err == nil {
			t.Errorf("%s was left behind", d)
		}
	}
	for _, d := range theirs {
		if _, err := os.Stat(filepath.Join(root, d)); err != nil {
			t.Errorf("%s was deleted", d)
		}
	}
	if _, err := os.Stat(file); err != nil {
		t.Error("a file of someone else's was deleted")
	}
}

func TestSessionIDsMatchCleanup(t *testing.T) {
	m := testManager(t, 4)
	s := m.Ensure(Spec{ID: "v-" + Key("/a/b.mkv") + "-a0-s-1-h0-c2-aac-abc123", Duration: 10})
	if !sessionDirRe.MatchString(filepath.Base(s.Dir)) {
		t.Fatalf("cleanRoot would not recognise %s", s.Dir)
	}
}

func TestTailBufferKeepsTheEnd(t *testing.T) {
	var b tailBuffer
	for i := 0; i < 2000; i++ {
		_, _ = b.Write([]byte(strings.Repeat("x", 1000)))
	}
	_, _ = b.Write([]byte("the last words"))
	if !strings.HasSuffix(b.String(), "the last words") {
		t.Error("the end is gone")
	}
	if len(b.b) > 2*stderrTail+1024 {
		t.Errorf("kept %d bytes", len(b.b))
	}
}

func TestLimitOnRunningTranscodes(t *testing.T) {
	m := testManager(t, 1)
	watching := m.Ensure(Spec{ID: "v-watching", Duration: 10})
	waiting := m.Ensure(Spec{ID: "v-waiting", Duration: 10})
	running(watching, time.Now())
	if err := m.makeRoom(waiting); !errors.Is(err, ErrBusy) {
		t.Fatalf("over the limit with everyone watching: %v", err)
	}
	if watching.job == nil {
		t.Fatal("a stream in use was stopped")
	}
	// The stream was left alone for a while (paused): it makes way.
	watching.lastUsed = time.Now().Add(-2 * yieldAfter)
	if err := m.makeRoom(waiting); err != nil {
		t.Fatal(err)
	}
	if watching.job != nil {
		t.Fatal("the idle stream is still running")
	}
	// Its own job doesn't count against a session.
	running(waiting, time.Now())
	if err := m.makeRoom(waiting); err != nil {
		t.Fatal(err)
	}
}

func TestLeaveKeepsAStreamOthersWatch(t *testing.T) {
	m := testManager(t, 4)
	s := m.Ensure(Spec{ID: "v-shared", Duration: 10})
	s.See("phone")
	s.See("tv")
	m.Leave("v-shared", "phone")
	if m.Get("v-shared") == nil {
		t.Fatal("one viewer leaving ended the stream for the other")
	}
	m.Leave("v-shared", "tv")
	if m.Get("v-shared") != nil {
		t.Fatal("the last viewer left and the stream goes on")
	}
}

// A film paused for a long time plays on: its segments are cleaned up, but
// the stream still exists for the viewer's next request.
func TestSweepKeepsIdleSessionsPlayable(t *testing.T) {
	m := testManager(t, 4)
	s := m.Ensure(Spec{ID: "v-paused", Duration: 10})
	seg := s.segPath(0)
	if err := os.WriteFile(seg, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	running(s, time.Now().Add(-pauseAfter-time.Second))
	m.sweepOnce()
	if s.job != nil {
		t.Fatal("not paused")
	}
	if !fileReady(seg) {
		t.Fatal("segments of a paused stream are gone")
	}
	s.lastUsed = time.Now().Add(-cleanAfter - time.Second)
	m.sweepOnce()
	if fileReady(seg) {
		t.Fatal("the segments of an idle stream stay on disk")
	}
	if m.Get("v-paused") == nil {
		t.Fatal("the stream is gone")
	}
	s.lastUsed = time.Now().Add(-forgetAfter - time.Second)
	m.sweepOnce()
	if m.Get("v-paused") != nil {
		t.Fatal("never forgotten")
	}
}

func TestPruneKeepsWhatIsNear(t *testing.T) {
	m := testManager(t, 4)
	s := m.Ensure(Spec{ID: "v-long", Duration: 4000})
	for _, n := range []int{0, 10, 349, 350, 500} {
		if err := os.WriteFile(s.segPath(n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s.prune(500)
	for n, want := range map[int]bool{0: false, 10: false, 349: false, 350: true, 500: true} {
		if fileReady(s.segPath(n)) != want {
			t.Errorf("segment %d: want present=%v", n, want)
		}
	}
}
