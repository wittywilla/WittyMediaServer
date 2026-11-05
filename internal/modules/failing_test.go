package modules

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymodule"
)

// Issue #265: on a busy machine a helper crashed once, the calls waiting on it restarted it,
// the restarts took longer than the 45 seconds a helper has to start, and within a minute the
// only version there was had been taken out of use for good, a restart included. Discs stopped
// until someone installed the module again.

func TestASlowStartGetsASecondGo(t *testing.T) {
	old := helloTimeout
	helloTimeout = time.Second // the helper takes 1.5 s: too long once, in time with twice as long
	defer func() { helloTimeout = old }()
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", wittymodule.API, "slow-hello"))
	m := w.manager()
	defer m.Close()
	check(t, m)
	installWait(t, m)
	usesHelper(t, m, "0.2.0")
	if s := one(t, m); s.LastFail != "" || s.Notice != "" {
		t.Fatalf("a slow start counted as failing: %+v", s)
	}
}

func TestFailuresTogetherCountOnce(t *testing.T) {
	old := failSpacing
	failSpacing = time.Hour
	defer func() { failSpacing = old }()
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", wittymodule.API, "crash"))
	m := w.manager()
	defer m.Close()
	check(t, m)
	installWait(t, m)
	for range failLimit * 2 {
		(PhotoEngine{m}).Dimensions(filepath.Join(t.TempDir(), "missing.jpg"))
	}
	s := one(t, m)
	if s.Version != "0.2.0" || s.Notice != "" || s.LastFail == "" {
		t.Fatalf("one incident took it out of use, or wasn't noted: %+v", s)
	}
	// Something wrong it did, rather than a crash, counts every time.
	for i := range failLimit {
		if err := m.ReportFailure("wittyphotos", "0.2.0", "a disc it described can't be real "+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	if s := one(t, m); !strings.Contains(s.Notice, "kept failing") {
		t.Fatalf("misbehaving wasn't counted each time: %+v", s)
	}
}

func TestTheOnlyVersionRestsRatherThanGoing(t *testing.T) {
	oldRest := restFor
	restFor = time.Hour
	defer func() { restFor = oldRest }()
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", wittymodule.API, "crash"))
	m := w.manager()
	check(t, m)
	installWait(t, m)
	for range failLimit {
		(PhotoEngine{m}).Dimensions(filepath.Join(t.TempDir(), "missing.jpg"))
	}
	s := one(t, m)
	if s.Version != "0.2.0" || !strings.Contains(s.Notice, "resting until") {
		t.Fatalf("%+v", s)
	}
	// A feature with no way of its own is told why at once, rather than waiting.
	a := m.Attach("wittyphotos", time.Minute)
	if a == nil {
		t.Fatal("no helper to attach to")
	}
	start := time.Now()
	_, err := a.Call("dimensions", map[string]string{"path": "x"}, nil)
	a.Close()
	var resting *ErrResting
	if !errors.As(err, &resting) || time.Since(start) > 5*time.Second {
		t.Fatalf("%v after %s", err, time.Since(start))
	}
	if !strings.Contains(err.Error(), "WittyPhotos") || !strings.Contains(err.Error(), "resting until") {
		t.Fatalf("doesn't say what or until when: %v", err)
	}
	m.Close()

	// It isn't out of use for good: a restart tries it again.
	m2 := w.manager()
	defer m2.Close()
	if s := one(t, m2); s.Version != "0.2.0" || s.Notice != "" {
		t.Fatalf("after a restart: %+v", s)
	}
	if m2.Resting("wittyphotos") != nil {
		t.Fatal("still resting after a restart")
	}
}

func TestARestEnds(t *testing.T) {
	oldRest := restFor
	restFor = 50 * time.Millisecond
	defer func() { restFor = oldRest }()
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", wittymodule.API, "crash"))
	m := w.manager()
	defer m.Close()
	check(t, m)
	installWait(t, m)
	for range failLimit {
		(PhotoEngine{m}).Dimensions(filepath.Join(t.TempDir(), "missing.jpg"))
	}
	if m.Resting("wittyphotos") == nil {
		t.Fatal("not resting")
	}
	time.Sleep(100 * time.Millisecond)
	if err := m.Resting("wittyphotos"); err != nil {
		t.Fatalf("still resting: %v", err)
	}
	if s := one(t, m); s.Notice != "" {
		t.Fatalf("%+v", s)
	}
}

// What an earlier run did to an only version, before this was fixed, is undone at the next start.
func TestAnEarlierRunsGivingUpIsUndone(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", wittymodule.API))
	m := w.manager()
	check(t, m)
	installWait(t, m)
	m.mu.Lock()
	e := m.mods["wittyphotos"]
	e.state.Blocked = map[string]string{"0.2.0": keptFailing + " (the helper didn't say hello)"}
	if err := m.saveState(); err != nil {
		t.Fatal(err)
	}
	m.mu.Unlock()
	m.Close()

	m2 := w.manager()
	defer m2.Close()
	s := one(t, m2)
	if s.Version != "0.2.0" || s.Notice != "" {
		t.Fatalf("%+v", s)
	}
	usesHelper(t, m2, "0.2.0")
	var kinds []string
	for _, ev := range s.Events {
		kinds = append(kinds, ev.Kind)
	}
	if !strings.Contains(strings.Join(kinds, " "), "another-chance") {
		t.Fatalf("no record of it: %v", kinds)
	}

	// Changed files are another matter: those stay out of use.
	m2.mu.Lock()
	e = m2.mods["wittyphotos"]
	e.state.Blocked = map[string]string{"0.2.0": "Its files have changed since they were installed."}
	if err := m2.saveState(); err != nil {
		t.Fatal(err)
	}
	m2.mu.Unlock()
	m2.Close()
	m3 := w.manager()
	defer m3.Close()
	if s := one(t, m3); s.Version == "0.2.0" && s.Notice == "" {
		t.Fatalf("a version with changed files was let back in: %+v", s)
	}
}

// Issue #294: the start-up repair lifts only what slow starts and crashes did. A game runner
// that stopped before the game was ready is the Mac sandbox fix's to decide on
// (liftNestedSandboxBlocks), so it's left as it was here, on every system.
func TestTheStartUpRepairLeavesTheMacFixItsOwn(t *testing.T) {
	m := New(Config{Dir: t.TempDir(), Server: "0.6.2-alpha", Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	defer m.Close()
	for _, c := range []struct {
		why    string
		lifted bool
	}{
		{keptFailing + " (the helper didn't say hello: read |0: file already closed)", true},
		{keptFailing + " (it stopped without answering)", true},
		{runnerStopped + ": a sandbox refused it)", false},
		{"Its files have changed since they were installed.", false},
	} {
		e := &entry{builtin: &wittymodule.Manifest{ID: "wittyplay", Name: "WittyPlay", Version: "0.1.0"}, state: moduleState{Active: "0.3.4", Blocked: map[string]string{"0.3.4": c.why}}}
		m.mu.Lock()
		m.giveAnotherChance("wittyplay", e)
		m.mu.Unlock()
		if lifted := e.state.Blocked["0.3.4"] == ""; lifted != c.lifted {
			t.Errorf("%q: lifted %v, want %v", c.why, lifted, c.lifted)
		}
	}
}
