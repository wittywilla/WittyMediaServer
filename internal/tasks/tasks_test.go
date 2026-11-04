package tasks

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNilIsSafe(t *testing.T) {
	var r *Registry
	h := r.Start(Spec{Title: "x"})
	h.Progress(1, 2)
	h.Item("a", "b")
	h.Phase("p")
	h.Problem("a", "b")
	h.Begin()
	h.Finish(nil)
	if err := h.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.Live() != nil || r.Recent() != nil {
		t.Fatal("a nil registry has nothing")
	}
}

func TestLifeOfATask(t *testing.T) {
	r := New(nil)
	h := r.Queue(Spec{Type: TypeScan, Title: "Scanning Films", LibraryID: 3, Unit: "files"})
	if got := r.Live()[0]; got.State != Queued || got.Percent != -1 {
		t.Fatalf("queued: %+v", got)
	}
	h.Begin()
	h.Progress(5, 20)
	got := r.Live()[0]
	if got.State != Running || got.Percent != 25 || got.Done != 5 {
		t.Fatalf("running: %+v", got)
	}
	h.Finish(nil)
	if len(r.Live()) != 0 {
		t.Fatal("still live")
	}
	f := r.Recent()[0]
	if f.State != Done || f.Done != 20 || f.Percent != 100 || f.FinishedAt == 0 {
		t.Fatalf("finished: %+v", f)
	}
	h.Fail("too late", "", nil)
	if r.Recent()[0].State != Done {
		t.Fatal("a finished task doesn't change")
	}
}

func TestFinishWithErrors(t *testing.T) {
	r := New(nil)
	r.Start(Spec{Title: "a"}).Finish(context.Canceled)
	if s := r.Recent()[0].State; s != Cancelled {
		t.Fatal(s)
	}
	r.Start(Spec{Title: "b"}).Finish(fmt.Errorf("open: %w", errors.New("no space left on device")))
	f := r.Recent()[0]
	if f.State != Failed || !strings.Contains(f.Error, "disk is full") || f.Fix == "" || !strings.Contains(f.Detail, "no space") {
		t.Fatalf("%+v", f)
	}
}

func TestPauseAndResume(t *testing.T) {
	r := New(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := r.Start(Spec{Title: "a", Pausable: true, Cancel: cancel})
	if !r.Live()[0].CanPause || r.Live()[0].CanResume {
		t.Fatal("should offer pause")
	}
	if err := r.Pause(h.ID()); err != nil {
		t.Fatal(err)
	}
	if !h.PauseWanted() || r.Live()[0].CanPause || !r.Live()[0].CanResume {
		t.Fatal("should offer resume")
	}
	done := make(chan error)
	go func() { done <- h.Wait(ctx) }()
	time.Sleep(20 * time.Millisecond)
	if r.Live()[0].State != Paused {
		t.Fatal("job at Wait: paused")
	}
	select {
	case <-done:
		t.Fatal("Wait should hold")
	default:
	}
	if err := r.Resume(h.ID()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if r.Live()[0].State != Running {
		t.Fatal("running again")
	}
	if err := r.Pause(h.ID()); err != nil {
		t.Fatal(err)
	}
	go func() { done <- h.Wait(ctx) }()
	time.Sleep(20 * time.Millisecond)
	if err := r.Cancel(h.ID()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("cancelling a paused task lets it go", err)
	}
	h.Finish(ctx.Err())
	if r.Recent()[0].State != Cancelled {
		t.Fatal("cancelled")
	}
	if err := r.Resume(h.ID()); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestControlsARefusedWithoutSupport(t *testing.T) {
	r := New(nil)
	h := r.Start(Spec{Title: "a"})
	if r.Pause(h.ID()) != ErrCant || r.Cancel(h.ID()) != ErrCant {
		t.Fatal("no support, no control")
	}
	v := r.Live()[0]
	if v.CanPause || v.CanCancel || v.CanResume {
		t.Fatalf("%+v", v)
	}
}

func TestRetry(t *testing.T) {
	r := New(nil)
	var got string
	r.OnRetry(TypeScan, func(old Task, by By) error { got = old.RetryKey + by.Name; return nil })
	h := r.Start(Spec{Type: TypeScan, Title: "a", RetryKey: "7"})
	h.Finish(errors.New("oh"))
	f := r.Recent()[0]
	if !f.CanRetry || !r.CanRetry(f) {
		t.Fatal("should offer a retry")
	}
	if err := r.Retry(f, By{Name: "w"}); err != nil || got != "7w" {
		t.Fatal(err, got)
	}
	h2 := r.Start(Spec{Type: TypeScan, Title: "b"})
	h2.Finish(errors.New("oh"))
	if r.Recent()[0].CanRetry {
		t.Fatal("no key, no retry")
	}
	h3 := r.Start(Spec{Type: TypeScan, Title: "c", RetryKey: "8"})
	h3.Finish(nil)
	if r.Recent()[0].CanRetry || r.Retry(r.Recent()[0], By{}) == nil {
		t.Fatal("what worked isn't retried")
	}
}

func TestProblemsAreBounded(t *testing.T) {
	r := New(nil)
	h := r.Start(Spec{Title: "a"})
	for i := range 500 {
		h.Problem(fmt.Sprint("f", i), strings.Repeat("x", 2000))
	}
	h.Finish(nil)
	f := r.Recent()[0]
	if f.ProblemCount != 500 || len(f.Problems) != maxProblems || len(f.Problems[0].Message) > maxText+4 {
		t.Fatalf("%d %d %d", f.ProblemCount, len(f.Problems), len(f.Problems[0].Message))
	}
}

func TestRecentIsBounded(t *testing.T) {
	r := New(nil)
	for range recentFinished * 3 {
		r.Start(Spec{Title: "a"}).Finish(nil)
	}
	if len(r.Recent()) != recentFinished {
		t.Fatal(len(r.Recent()))
	}
}

func TestQuietAndNoHistory(t *testing.T) {
	r := New(nil)
	r.Start(Spec{Title: "idle", Quiet: true}).Finish(nil)
	r.Start(Spec{Title: "t", NoHistory: true}).Finish(nil)
	if len(r.Recent()) != 0 {
		t.Fatal("nothing to keep")
	}
	h := r.Start(Spec{Title: "worked", Quiet: true})
	h.Add(1)
	h.Finish(nil)
	if len(r.Recent()) != 1 {
		t.Fatal("did something: keep it")
	}
}

func TestViewerSeesOnlyWhatTheyMay(t *testing.T) {
	all := Task{ID: "a", Title: "Scanning", LibraryID: 2, Item: "secret.mkv", Path: "/media/secret.mkv", By: "Someone", ByID: 9, Error: "open /media/secret.mkv", Detail: "x", Fix: "y",
		Problems: []Problem{{Item: "secret.mkv", Message: "m"}}, ProblemCount: 1, CanCancel: true, CanRetry: true, Result: "z"}
	admin := Viewer{Admin: true}
	if got, ok := admin.For(all); !ok || got.Path == "" {
		t.Fatal("admin sees all")
	}
	yes := func(int64) bool { return true }
	u := Viewer{UserID: 1, Library: yes}
	got, ok := u.For(all)
	if !ok {
		t.Fatal("may see a library they have")
	}
	if got.Item != "" || got.Path != "" || got.By != "" || got.ByID != 0 || got.Detail != "" || got.Fix != "" || got.Problems != nil || got.Result != "" {
		t.Fatalf("leak: %+v", got)
	}
	if strings.Contains(got.Error, "secret") || got.CanCancel || got.CanRetry {
		t.Fatalf("leak: %+v", got)
	}
	if got.ProblemCount != 1 {
		t.Fatal("the count is fine")
	}
	if _, ok := (Viewer{UserID: 1, Library: func(int64) bool { return false }}).For(all); ok {
		t.Fatal("a library they can't see")
	}
	if _, ok := (Viewer{UserID: 1}).For(all); ok {
		t.Fatal("no library rule: none")
	}
	priv := Task{ID: "b", Title: "Converting", Private: true, ByID: 9, By: "Someone"}
	if _, ok := u.For(priv); ok {
		t.Fatal("someone else's")
	}
	if got, ok := (Viewer{UserID: 9}).For(priv); !ok || got.By != "Someone" {
		t.Fatal("their own")
	}
	if _, ok := (Viewer{UserID: 9}).For(Task{AdminOnly: true, ByID: 9}); ok {
		t.Fatal("admins only")
	}
}

func TestSummary(t *testing.T) {
	s := Summarize([]Task{{State: Running, Percent: 50}, {State: Running, Percent: -1}, {State: Paused, Percent: 100}, {State: Queued, Percent: -1}, {State: Done, Percent: 100}})
	if s.Active != 3 || s.Queued != 1 || s.Paused != 1 || s.Percent != 75 {
		t.Fatalf("%+v", s)
	}
	if Summarize(nil).Percent != -1 {
		t.Fatal("nothing to say")
	}
}

func TestSubscriptionCoalescesAndStops(t *testing.T) {
	r := New(nil)
	before := runtime.NumGoroutine()
	s := r.Subscribe()
	h := r.Start(Spec{Title: "a"})
	for i := range 1000 {
		h.Progress(int64(i), 1000)
	}
	select {
	case <-s.C():
	case <-time.After(2 * time.Second):
		t.Fatal("no update")
	}
	u := s.Take()
	if len(u.Tasks) != 1 {
		t.Fatalf("%+v", u)
	}
	h.Finish(nil)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-s.C():
			u := s.Take()
			if len(u.Tasks) == 1 && u.Tasks[0].State == Done {
				s.Close()
				goto closed
			}
		case <-deadline:
			t.Fatal("finish never arrived")
		}
	}
closed:
	// With nobody watching, nothing keeps running.
	for i := 0; i < 100; i++ {
		if runtime.NumGoroutine() <= before {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("flusher still running: %d goroutines, was %d", runtime.NumGoroutine(), before)
}

func TestNoHistoryTaskIsGoneForWatchers(t *testing.T) {
	r := New(nil)
	s := r.Subscribe()
	defer s.Close()
	h := r.Start(Spec{Title: "a", NoHistory: true})
	time.Sleep(10 * time.Millisecond)
	h.Finish(nil)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-s.C():
			if u := s.Take(); len(u.Gone) == 1 && u.Gone[0] == h.ID() {
				return
			}
		case <-deadline:
			t.Fatal("never told to forget it")
		}
	}
}

func TestConcurrentUse(t *testing.T) {
	r := New(nil)
	s := r.Subscribe()
	defer s.Close()
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := r.Start(Spec{Title: "a", Pausable: true})
			for j := range 200 {
				h.Progress(int64(j), 200)
				h.Problem("x", "y")
				_ = r.Pause(h.ID())
				_ = r.Resume(h.ID())
				_ = r.Live()
			}
			h.Finish(nil)
		}()
	}
	wg.Wait()
}

type memStore struct {
	mu    sync.Mutex
	begun []Task
	ended []Task
}

func (m *memStore) Begin(t Task) { m.mu.Lock(); defer m.mu.Unlock(); m.begun = append(m.begun, t) }
func (m *memStore) End(t Task)   { m.mu.Lock(); defer m.mu.Unlock(); m.ended = append(m.ended, t) }

func TestStoreSeesStartAndEnd(t *testing.T) {
	m := &memStore{}
	r := New(m)
	r.Start(Spec{Title: "a"}).Finish(nil)
	r.Start(Spec{Title: "b", NoHistory: true}).Finish(nil)
	r.Start(Spec{Title: "c", Quiet: true}).Finish(nil)
	r.Settle()
	if len(m.begun) != 1 || len(m.ended) != 1 || m.ended[0].State != Done {
		t.Fatalf("%+v %+v", m.begun, m.ended)
	}
	// A Quiet task that did something is kept, though it was never begun in the store.
	h := r.Start(Spec{Title: "d", Quiet: true})
	h.Add(3)
	h.Finish(nil)
	r.Settle()
	if len(m.ended) != 2 {
		t.Fatal("kept")
	}
}

func TestReporterStaysInItsLane(t *testing.T) {
	r := New(nil)
	a, b := r.ForSource("moda"), r.ForSource("modb")
	a.Take(Report{Key: "k", Title: "Doing a thing", State: Running, Done: 1, Total: 4})
	b.Take(Report{Key: "k", Title: "B's", State: Running, Done: 3, Total: 4})
	l := r.Live()
	if len(l) != 2 || l[0].Done != 1 || l[1].Done != 3 || l[0].Source != "moda" || !l[0].AdminOnly || l[0].Key != "moda/k" {
		t.Fatalf("%+v", l)
	}
	b.Take(Report{Key: "k", Title: "B's", State: Done})
	if l := r.Live(); len(l) != 1 || l[0].Source != "moda" {
		t.Fatal("one module can't end another's")
	}
	for i := range 100 {
		a.Take(Report{Key: fmt.Sprint("n", i), Title: "x", State: Running})
	}
	if n := len(r.Live()); n != maxPerSource {
		t.Fatal("bounded per module", n)
	}
	a.CloseAll(true)
	if len(r.Live()) != 0 || r.Recent()[0].State != Failed {
		t.Fatal("a failed module's tasks fail")
	}
	idle := r.ForSource("idle")
	idle.Take(Report{Key: "k", Title: "x", State: Running})
	idle.CloseAll(false)
	if len(r.Live()) != 0 || r.Recent()[0].State != Cancelled {
		t.Fatal("a stopped module's tasks don't count as done")
	}
	a.Take(Report{Key: "late", Title: "x", State: Running})
	if len(r.Live()) != 0 {
		t.Fatal("nothing after the process has gone")
	}
	c := r.ForSource("modc")
	c.Take(Report{Key: "new", Title: "x", State: Done})
	if len(r.Live()) != 0 {
		t.Fatal("nothing to end, nothing made")
	}
}

func TestReporterKeepsTheLatestAndNeverHoldsBackAFinish(t *testing.T) {
	r := New(nil)
	m := r.ForSource("m")
	m.Take(Report{Key: "k", Title: "x", State: Running, Done: 1, Total: 100})
	m.Take(Report{Key: "k", Title: "x", State: Running, Done: 2, Total: 100})
	for i := int64(3); i <= 50; i++ {
		m.Take(Report{Key: "k", Title: "x", State: Running, Done: i, Total: 100})
	}
	deadline := time.Now().Add(3 * time.Second)
	for r.Live()[0].Done != 50 {
		if time.Now().After(deadline) {
			t.Fatalf("the latest report never arrived: %d", r.Live()[0].Done)
		}
		time.Sleep(10 * time.Millisecond)
	}
	m.Take(Report{Key: "k", Title: "x", State: Running, Done: 60, Total: 100})
	m.Take(Report{Key: "k", Title: "x", State: Failed, Done: 70, Total: 100})
	f := r.Recent()[0]
	if f.State != Failed || f.Done != 70 || f.Error == "" {
		t.Fatalf("%+v", f)
	}
}

func BenchmarkProgressNobodyWatching(b *testing.B) {
	r := New(nil)
	h := r.Start(Spec{Title: "a"})
	for i := 0; i < b.N; i++ {
		h.Progress(int64(i), int64(b.N))
	}
}

// A job with named steps shows one bar across them, each step counting its own work, rather
// than a bar that starts again at every phase.
func TestStepsMakeOneBar(t *testing.T) {
	r := New(nil)
	h := r.Start(Spec{Title: "Scanning Films", Phase: "Waiting for its turn"})
	h.Steps(Step{Name: "Finding files", Share: 1}, Step{Name: "Looking at each file", Share: 3})
	if got := r.Live()[0]; got.Step != 0 || got.Percent != -1 || len(got.Steps) != 2 {
		t.Fatalf("not on a step yet: %+v", got)
	}
	h.Phase("Finding files")
	h.Progress(5, 10)
	if got := r.Live()[0]; got.Step != 1 || got.Percent != 12.5 {
		t.Fatalf("half of the first quarter: %+v", got)
	}
	h.Phase("Looking at each file")
	if got := r.Live()[0]; got.Step != 2 || got.Done != 0 || got.Total != 0 || got.Percent != 25 {
		t.Fatalf("the second step starts where the first ended, and counts from nothing: %+v", got)
	}
	h.Progress(2, 4)
	if got := r.Live()[0]; got.Percent != 62.5 {
		t.Fatalf("half of the rest: %+v", got)
	}
	h.Phase("Waiting for the disk")
	if got := r.Live()[0]; got.Step != 2 || got.Percent != 62.5 || got.Phase != "Waiting for the disk" {
		t.Fatalf("a phase that isn't a step leaves the bar where it was: %+v", got)
	}
	h.Finish(nil)
	if got := r.Recent()[0]; got.Percent != 100 {
		t.Fatalf("done is all of it: %+v", got)
	}
}
