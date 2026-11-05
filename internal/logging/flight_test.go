package logging

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFlightKeepsWhatTheLevelsDrop(t *testing.T) {
	l, log := newTest(t)
	l.SetFlightRecorder(true)
	log.Debug("a song started", "cat", "playback", "item", 42)
	log.Warn("worth a file", "cat", "playback")
	var b bytes.Buffer
	if n, err := l.WriteFlight(&b); err != nil || n != 2 {
		t.Fatalf("%d records, %v\n%s", n, err, b.String())
	}
	if !strings.Contains(b.String(), "a song started") || !strings.Contains(b.String(), "level=DEBUG") {
		t.Fatalf("the debug line isn't kept:\n%s", b.String())
	}
	if got := read(t, l); strings.Contains(got, "a song started") {
		t.Fatalf("the debug line reached the file, which the levels said no to:\n%s", got)
	}
	// Off forgets, and keeps nothing more.
	l.SetFlightRecorder(false)
	log.Debug("after", "cat", "playback")
	b.Reset()
	if n, _ := l.WriteFlight(&b); n != 0 {
		t.Fatalf("kept %d after being turned off", n)
	}
}

func TestFlightIsBoundedByAgeAndSize(t *testing.T) {
	oldAge, oldBytes, oldN := flightAge, flightBytes, flightEntries
	t.Cleanup(func() { flightAge, flightBytes, flightEntries = oldAge, oldBytes, oldN })
	l, log := newTest(t)
	flightAge, flightBytes, flightEntries = 150*time.Millisecond, 4<<10, 64
	l.SetFlightRecorder(true)
	log.Debug("old", "cat", "library")
	time.Sleep(250 * time.Millisecond)
	for i := 0; i < 200; i++ {
		log.Debug("new", "cat", "library", "i", i, "pad", strings.Repeat("x", 100))
	}
	recs := l.flight.snapshot()
	if len(recs) == 0 || len(recs) > 64 {
		t.Fatalf("%d records", len(recs))
	}
	size := 0
	for _, r := range recs {
		size += r.size()
		if r.Msg == "old" {
			t.Fatal("kept a record older than the window")
		}
	}
	if size > 4<<10 {
		t.Fatalf("holds %d bytes", size)
	}
	if last := recs[len(recs)-1]; last.Attrs[0].V != "199" {
		t.Fatalf("the newest isn't last: %+v", last)
	}
}

func TestFlightRemovesSecrets(t *testing.T) {
	l, log := newTest(t)
	l.SetFlightRecorder(true)
	log.Debug("asked", "cat", "library", "url", "https://example.com/x?token=PLANTEDSECRET", "apiKey", "PLANTEDKEY")
	var b bytes.Buffer
	_, _ = l.WriteFlight(&b)
	if strings.Contains(b.String(), "PLANTED") {
		t.Fatalf("a secret is kept:\n%s", b.String())
	}
}

// A hang the way #17 looked: the web requests all stuck behind one lock held over a
// network call. The watchdog must notice, and write down who was waiting where.
func TestWatchdogWritesDownAHang(t *testing.T) {
	saved := []time.Duration{watchEvery, watchLimit, hangAfter, hangAgain, hangGap}
	t.Cleanup(func() {
		watchEvery, watchLimit, hangAfter, hangAgain, hangGap = saved[0], saved[1], saved[2], saved[3], saved[4]
	})
	watchEvery, watchLimit, hangAfter, hangAgain, hangGap = 20*time.Millisecond, 60*time.Millisecond, 300*time.Millisecond, 400*time.Millisecond, time.Hour

	l, log := newTest(t)
	l.SetFlightRecorder(true)
	slog.SetDefault(log)
	t.Cleanup(func() { slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil))) })
	guard, _, err := StartCrashGuard(t.TempDir(), l.Dir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(guard.Close)

	var shared sync.Mutex
	release := make(chan struct{})
	stuck := make(chan struct{})
	go stuckLookup(&shared, stuck, release)
	<-stuck
	log.Debug("lyrics asked for", "cat", "library", "item", 7)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newWatchdog(log, []Check{{Name: "web", Run: func(ctx context.Context) error {
		shared.Lock() // what every request needed
		shared.Unlock()
		return nil
	}}})
	got := make(chan Crash, 4)
	w.onHang = func(c Crash) { got <- c }
	go w.run(ctx)

	var c Crash
	select {
	case c = <-got:
	case <-time.After(20 * time.Second):
		t.Fatal("no hang was written down")
	}
	if c.Kind != "hang" || !strings.Contains(c.Summary, "web") {
		t.Fatalf("%+v", c)
	}
	report, err := guard.Read(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"stopped answering", "goroutines, grouped", "stuckLookup", "lyrics asked for"} {
		if !bytes.Contains(report, []byte(want)) {
			t.Errorf("the report doesn't say %q:\n%s", want, report)
		}
	}
	for _, part := range []string{PartFlight, PartGoroutines} {
		p, err := guard.PartPath(c.ID, part)
		if err != nil {
			t.Fatalf("no %s: %v", part, err)
		}
		b, _ := os.ReadFile(p)
		if part == PartFlight && !bytes.Contains(b, []byte("lyrics asked for")) {
			t.Errorf("the flight recorder doesn't have the minutes before:\n%s", b)
		}
		if part == PartGoroutines && !bytes.Contains(b, []byte("stuckLookup")) {
			t.Errorf("the stacks don't show who held it:\n%.2000s", b)
		}
	}
	// A second look, a little later, tells stuck from slow.
	select {
	case c2 := <-got:
		if !strings.Contains(c2.Summary, "still not answering") {
			t.Fatalf("%+v", c2)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("no second look")
	}
	close(release)
	deadline := time.Now().Add(20 * time.Second)
	for !strings.Contains(read(t, l), "answering again") {
		if time.Now().After(deadline) {
			t.Fatalf("never said it was answering again:\n%s", read(t, l))
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Listed with its parts, and deleted with them.
	var listed *Crash
	for _, x := range guard.List() {
		if x.ID == c.ID {
			listed = &x
		}
	}
	if listed == nil || len(listed.Parts) < 2 {
		t.Fatalf("listed as %+v", listed)
	}
	if err := guard.Delete(c.ID); err != nil {
		t.Fatal(err)
	}
	if left, _ := filepath.Glob(filepath.Join(guard.Dir(), c.ID+"*")); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

// stuckLookup holds what everyone needs while it waits on something slow: the shape of
// the fault #17 was most likely to be.
func stuckLookup(mu *sync.Mutex, stuck, release chan struct{}) {
	mu.Lock()
	defer mu.Unlock()
	close(stuck)
	<-release
}

func TestStoppingWhileHungIsWrittenDown(t *testing.T) {
	l, log := newTest(t)
	l.SetFlightRecorder(true)
	guard, _, err := StartCrashGuard(t.TempDir(), l.Dir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(guard.Close)
	w := newWatchdog(log, nil)
	activeWatch.Store(w)
	t.Cleanup(func() { activeWatch.Store(nil) })
	NoteStopping()
	if n := len(guard.List()); n != 0 {
		t.Fatalf("a stop with all well wrote %d reports", n)
	}
	w.note("web", time.Now(), context.DeadlineExceeded)
	w.mu.Lock()
	w.episode = time.Now().Add(-time.Minute)
	w.mu.Unlock()
	NoteStopping()
	list := guard.List()
	if len(list) != 1 || list[0].Kind != "hang" || !strings.Contains(list[0].Summary, "asked to stop while it wasn't answering") {
		t.Fatalf("%+v", list)
	}
}

func TestSavedOnRequest(t *testing.T) {
	l, log := newTest(t)
	l.SetFlightRecorder(true)
	if err := StartTraceRecorder(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(StopTraceRecorder)
	guard, _, err := StartCrashGuard(t.TempDir(), l.Dir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(guard.Close)
	log.Debug("something odd", "cat", "playback")
	time.Sleep(50 * time.Millisecond)
	c, err := SaveEvidence("saved", "Saved from Settings by owner.", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != "saved" || len(c.Parts) != 3 {
		t.Fatalf("%+v", c)
	}
	p, _ := guard.PartPath(c.ID, PartTrace)
	if st, err := os.Stat(p); err != nil || st.Size() == 0 {
		t.Fatalf("no execution trace: %v", err)
	}
	if _, err := guard.PartPath(c.ID, "../../etc/passwd"); err == nil {
		t.Fatal("a part outside the folder")
	}
	// It goes in the support bundle, sanitized like the rest.
	var zipped bytes.Buffer
	if err := l.WriteBundle(&zipped, guard, BundleOptions{Sanitize: true}); err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{".flight.log", ".goroutines.txt", ".trace"} {
		if !bytes.Contains(zipped.Bytes(), []byte(c.ID+part)) {
			t.Errorf("the bundle has no %s", part)
		}
	}
}

func TestCallsAreTimedAndListedWhileWaiting(t *testing.T) {
	l, log := newTest(t)
	l.SetFlightRecorder(true)
	slog.SetDefault(log)
	t.Cleanup(func() { slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil))) })
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			<-release
		}
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()
	c := &http.Client{Transport: Calls(nil, "library", "lyrics")}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if res, err := c.Get(srv.URL + "/slow?apikey=PLANTEDKEY"); err == nil {
			_, _ = io.ReadAll(res.Body)
			res.Body.Close()
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(InFlight(), "lyrics: GET") {
		if time.Now().After(deadline) {
			t.Fatalf("not listed while waiting: %s", InFlight())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if strings.Contains(InFlight(), "PLANTED") {
		t.Fatal("the query is listed")
	}
	close(release)
	<-done
	if strings.Contains(InFlight(), "lyrics: GET") {
		t.Fatalf("still listed: %s", InFlight())
	}
	var b bytes.Buffer
	_, _ = l.WriteFlight(&b)
	if !strings.Contains(b.String(), "outgoing call") || !strings.Contains(b.String(), "/slow") || !strings.Contains(b.String(), "bytes=5") || strings.Contains(b.String(), "PLANTED") {
		t.Fatalf("%s", b.String())
	}
}

func TestBusiestGroupNamesOurOwnCode(t *testing.T) {
	groups := "goroutine profile: total 60\n" +
		"3 @ 0x1 0x2\n#\t0x1\truntime.gopark+0x1\t/go/src/runtime/proc.go:1\n\n" +
		"57 @ 0x1 0x2\n#\t0x1\tsync.(*Mutex).Lock+0x1\t/go/src/sync/mutex.go:1\n#\t0x2\tgithub.com/wittywilla/wittymediaserver/internal/server.(*Server).askOnline+0x99\t/src/api_lyrics.go:300\n"
	if got := busiestGroup(groups); got != "57 in server.(*Server).askOnline" {
		t.Fatalf("%q", got)
	}
}

// What the flight recorder costs a debug line that no file wants.
func BenchmarkQuietCategoryFlight(b *testing.B) {
	l, log := benchLogger(b)
	l.SetFlightRecorder(true)
	sub := log.With("cat", "discs", "session", "abc")
	b.ReportAllocs()
	for b.Loop() {
		sub.Debug("step", "cell", 5, "pgc", 3)
	}
}
