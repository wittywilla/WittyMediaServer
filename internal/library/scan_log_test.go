package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// sessionCapture keeps what a scan logs, with the session its context carries.
type sessionCapture struct {
	mu   sync.Mutex
	recs []capturedRecord
}

type capturedRecord struct {
	lv      slog.Level
	msg     string
	session string
	text    string
}

func (c *sessionCapture) Enabled(context.Context, slog.Level) bool { return true }
func (c *sessionCapture) WithAttrs([]slog.Attr) slog.Handler       { return c }
func (c *sessionCapture) WithGroup(string) slog.Handler            { return c }
func (c *sessionCapture) Handle(ctx context.Context, r slog.Record) error {
	var b strings.Builder
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, "%s=%v ", a.Key, a.Value)
		return true
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recs = append(c.recs, capturedRecord{r.Level, r.Message, logging.SessionFrom(ctx), b.String()})
	return nil
}

func (c *sessionCapture) all() []capturedRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]capturedRecord(nil), c.recs...)
}

func TestScanLeavesARecord(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	root := filepath.Join(t.TempDir(), "books")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Private Title.pdf"), []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}
	away := filepath.Join(t.TempDir(), "away")
	st := &Store{DB: d}
	lib, err := st.CreateLibrary(context.Background(), "Books", "books", []string{root, away})
	if err != nil {
		t.Fatal(err)
	}
	c := &sessionCapture{}
	s := NewScanner(st, &ffmpeg.Tools{Encoders: map[string]bool{}}, t.TempDir(), func() string { return "" }, slog.New(c))

	if !s.Start(lib.ID) {
		t.Fatal("did not start")
	}
	for end := time.Now().Add(20 * time.Second); s.Snapshot(lib.ID) != nil; {
		if time.Now().After(end) {
			t.Fatal("the scan did not end")
		}
		time.Sleep(10 * time.Millisecond)
	}

	var session string
	var started, finished, offline bool
	for _, r := range c.all() {
		if r.session == "" || !strings.HasPrefix(r.session, "scan-") {
			t.Errorf("%q has session %q", r.msg, r.session)
		}
		if session != "" && r.session != session {
			t.Errorf("two sessions in one scan: %q and %q", session, r.session)
		}
		session = r.session
		if r.lv >= slog.LevelInfo && r.lv < slog.LevelWarn && strings.Contains(r.text, "Private Title") {
			t.Errorf("a file name at Info: %q %s", r.msg, r.text)
		}
		switch r.msg {
		case "scan started":
			started = true
		case "scan finished":
			finished = true
			if !strings.Contains(r.text, "errors=0") || !strings.Contains(r.text, "offline=1") {
				t.Errorf("summary: %s", r.text)
			}
		case "library folder is empty or can't be reached; keeping its items":
			offline = r.lv == slog.LevelWarn
		}
	}
	if !started || !finished || !offline {
		t.Fatalf("started %v, finished %v, away folder noted %v: %v", started, finished, offline, c.all())
	}
}

// A file or a lookup failing doesn't stop a scan: during one it is counted for the
// summary, and outside one it is a warning.
func TestMinorFailures(t *testing.T) {
	c := &sessionCapture{}
	s := &Scanner{Log: slog.New(c)}
	boom := errors.New("network is down")

	s.minorFailure(context.Background(), "metadata lookup failed", boom)
	if r := c.all(); len(r) != 1 || r[0].lv != slog.LevelWarn {
		t.Fatalf("outside a scan: %v", r)
	}

	c = &sessionCapture{}
	s.Log = slog.New(c)
	tally := &scanTally{what: map[string]*tallied{}}
	ctx := context.WithValue(context.Background(), tallyKey{}, tally)
	for i := 0; i < 3; i++ {
		s.minorFailure(ctx, "metadata lookup failed", boom, "title", "x")
	}
	for _, r := range c.all() {
		if r.lv != slog.LevelDebug {
			t.Fatalf("per file: %v", r)
		}
	}
	if n := tally.report(ctx, s.Log); n != 3 {
		t.Fatalf("counted %d", n)
	}
	recs := c.all()
	last := recs[len(recs)-1]
	if last.lv != slog.LevelWarn || !strings.Contains(last.text, "count=3") || !strings.Contains(last.text, boom.Error()) {
		t.Fatalf("summary: %+v", last)
	}

	// Stopping a scan isn't a failure of anything.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	s.minorFailure(cancelled, "metadata lookup failed", context.Canceled)
	if tally.what["metadata lookup failed"].n != 3 {
		t.Fatal("a cancelled lookup was counted")
	}
}

func TestUnreadableFolderIsReported(t *testing.T) {
	var bad unreadable
	if err := bad.with(nil); err != nil {
		t.Fatal(err)
	}
	bad.note(errors.New("access denied"))
	bad.note(errors.New("second"))
	err := bad.with(nil)
	if err == nil || !strings.Contains(err.Error(), "2 folders") || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("got %v", err)
	}
}
