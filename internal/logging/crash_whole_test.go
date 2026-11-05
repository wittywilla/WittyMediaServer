package logging

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A report is never seen half written. Written in place, List could find one empty for a moment,
// which is how TestRecoverAndLoop once read a report with nothing in it.
func TestReportsAreNeverSeenHalfWritten(t *testing.T) {
	g := OpenCrashReports(t.TempDir())
	var stop atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		when := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
		for i := 0; !stop.Load() && i < 2000; i++ {
			if _, err := g.writeReport("panic", when.Add(time.Duration(i)*time.Millisecond), "test", "a summary", strings.Repeat("goroutine 1 [running]\n", 2000), ""); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	defer func() { stop.Store(true); <-done }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, c := range g.List() {
			body, err := g.Read(c.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), "goroutine") || c.Summary != "a summary" {
				t.Fatalf("%s seen half written: %d bytes, summary %q", c.ID, len(body), c.Summary)
			}
		}
	}
}
