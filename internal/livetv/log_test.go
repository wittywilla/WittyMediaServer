package livetv

import (
	"bytes"
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// A recording that fails always leaves a record tied to its ID, including one a
// restart cut short.
func TestFailedRecordingLeavesARecord(t *testing.T) {
	m := testManager(t)
	buf := &syncBuffer{}
	m.Log = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})).With("cat", "tv")
	srv := iptvServer(t)
	_, ch := addIPTV(t, m, srv)
	ctx := context.Background()
	now := time.Now().Unix()
	r, err := m.Schedule(ctx, ch[0].ID, 1, "Show", "", now, now+60)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.DB.Exec(`UPDATE recordings SET status = 'recording' WHERE id = ?`, r.ID); err != nil {
		t.Fatal(err)
	}

	m.failInterrupted()

	var status, why string
	if err := m.DB.QueryRow(`SELECT status, error FROM recordings WHERE id = ?`, r.ID).Scan(&status, &why); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || why == "" {
		t.Fatalf("status %q, error %q", status, why)
	}
	id := strconv.FormatInt(r.ID, 10)
	out := buf.String()
	for _, want := range []string{"level=ERROR", "recording failed", "session=rec-" + id, "recording=" + id, why} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
}
