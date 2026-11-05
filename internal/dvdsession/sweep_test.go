package dvdsession

import (
	"testing"
	"time"
)

// A server with no disc open isn't woken to look for idle ones: the sweep starts with
// the first disc and stops once none are left.
func TestSweepOnlyWhileDiscsAreOpen(t *testing.T) {
	m := NewManager(nil, nil, t.TempDir(), "/api", nil)
	m.sweepEvery = 5 * time.Millisecond
	sweeping := func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.sweeping
	}
	if sweeping() {
		t.Fatal("sweeping with nothing open")
	}
	m.mu.Lock()
	m.sweepSoon()
	m.sweepSoon() // once, however often it's asked
	m.mu.Unlock()
	for deadline := time.Now().Add(5 * time.Second); sweeping(); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("still sweeping with no discs open")
		}
	}
}
