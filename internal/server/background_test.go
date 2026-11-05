package server

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// What a request starts on its own is ended and waited for by Close. Before, a test's server
// could still be using its database as the test's folder was removed, and on a busy machine
// that wrote a new database into the folder ("TempDir RemoveAll cleanup: directory not empty").
func TestCloseEndsAndWaitsForBackgroundWork(t *testing.T) {
	s := &Server{}
	started := make(chan struct{})
	var ended atomic.Bool
	if !s.background("test work", func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond) // still finishing when Close is asked
		ended.Store(true)
	}) {
		t.Fatal("refused before Close")
	}
	<-started
	s.Close()
	if !ended.Load() {
		t.Error("Close returned before the work ended")
	}
	if s.background("late work", func(context.Context) { t.Error("ran after Close") }) {
		t.Error("started after Close")
	}
	s.Close() // a second time is harmless
}

// Activity finished just before the server stops is in the history after Close: it's written on a
// goroutine of the registry's, which Close waits for.
func TestCloseSavesFinishedActivity(t *testing.T) {
	ts := newTestServer(t)
	for range 20 {
		ts.Tasks.Start(tasks.Spec{Title: "A job that just ended"}).Finish(nil)
	}
	ts.Close()
	var n int
	if err := ts.DB.QueryRow(`SELECT COUNT(*) FROM task_history WHERE title = 'A job that just ended'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 20 {
		t.Errorf("%d of 20 in the history", n)
	}
}

// The games service's own work goes through it: a sweep asked for after Close never starts.
func TestGamesWorkStopsWithTheServer(t *testing.T) {
	ts := newTestServer(t)
	ts.Close()
	ts.sweepBIOS(true)
	g := ts.games()
	g.bios.mu.Lock()
	sweeping := g.bios.sweeping
	g.bios.mu.Unlock()
	if sweeping {
		t.Error("a BIOS sweep is marked running after the server closed, so none would run again")
	}
	if p := ts.gpuStatus("/a/runner"); p.Checking {
		t.Errorf("the card check started after Close: %+v", p)
	}
}
