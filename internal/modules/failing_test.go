// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

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
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", MinAPI, "slow-hello"))
	m := w.manager()
	defer m.Close()
	check(t, m)
	installWait(t, m)
	usesHelper(t, m, "0.2.0")
	if s := one(t, m); s.LastFail != "" || s.Notice != "" {
		t.Fatalf("a slow start counted as failing: %+v", s)
	}
}

// A helper too slow to say hello even on its second go: the install says it ran out of time, not
// the read error the server's own closing of the pipe causes, and the log says the server stopped
// it (on Windows a killed helper reads as "exit code 1") and that it wrote nothing to stderr.
func TestAHelperStoppedForNoHelloSaysSo(t *testing.T) {
	old := helloTimeout
	helloTimeout = 200 * time.Millisecond // the helper takes 1.5 s: too long twice
	defer func() { helloTimeout = old }()
	logs := &syncBuffer{}
	w := newWorld(t)
	w.logs = logs
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", MinAPI, "slow-hello"))
	m := w.manager()
	defer m.Close()
	check(t, m)
	s := installWait(t, m)
	if !strings.Contains(s.Error, "still starting when its time ran out (400ms), with no hello") || strings.Contains(s.Error, "closed") {
		t.Fatalf("the install's error: %q", s.Error)
	}
	said := logs.String()
	for _, want := range []string{`why="stopped by the server: no hello within 200ms"`, `why="stopped by the server: no hello within 400ms"`, `stderr="(it wrote nothing)"`} {
		if !strings.Contains(said, want) {
			t.Errorf("the log has no %s:\n%s", want, said)
		}
	}
}

func TestFailuresTogetherCountOnce(t *testing.T) {
	old := failSpacing
	failSpacing = time.Hour
	defer func() { failSpacing = old }()
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", MinAPI, "crash"))
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
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", MinAPI, "crash"))
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

// A rest ends when its time is up. Time passes here by moving the rest's end into the past, not
// by waiting out a short rest: on a busy machine (CI's race detector, with every package at once)
// more than 50 ms went by between the third failure starting the rest and the first look at it,
// and the test saw no rest at all.
func TestARestEnds(t *testing.T) {
	oldRest := restFor
	restFor = time.Hour
	defer func() { restFor = oldRest }()
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", MinAPI, "crash"))
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
	m.mu.Lock()
	m.mods["wittyphotos"].restUntil = time.Now().Add(-time.Second) // the hour is up
	m.mu.Unlock()
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
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", MinAPI))
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

// Issue #294: the start-up repair lifts only what slow starts and crashes did.
func TestTheStartUpRepairLiftsOnlySlowStartsAndCrashes(t *testing.T) {
	m := New(Config{Dir: t.TempDir(), Server: "0.6.2-alpha", Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	defer m.Close()
	for _, c := range []struct {
		why    string
		lifted bool
	}{
		{keptFailing + " (the helper didn't say hello: read |0: file already closed)", true},
		{keptFailing + " (it stopped without answering)", true},
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
