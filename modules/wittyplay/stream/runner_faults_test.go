package stream

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittyplay/ipc"
)

// TestFaultyRunner isn't a test: the tests below run this program as the runner, and
// what they ask of it comes after "--".
func TestFaultyRunner(t *testing.T) {
	if flag.NArg() == 0 {
		t.Skip("run by other tests")
	}
	mode := flag.Arg(0)
	rd := ipc.NewReader(os.Stdin)
	_, first, err := rd.Next()
	if err != nil {
		os.Exit(1)
	}
	var load ipc.Load
	_ = json.Unmarshal(first, &load)
	if load.Core == "dies" {
		os.Exit(3)
	}
	if load.Core == "slow" {
		// Still loading, and listening for being told to stop.
		for {
			if c, _, err := rd.Next(); err != nil || c == ipc.CmdQuit {
				os.Exit(0)
			}
		}
	}
	// What's passed to it is what the name says, so that a test can read it.
	name := strings.Join(flag.Args(), " ")
	ready, _ := json.Marshal(ipc.Ready{Name: name, Width: 16, Height: 16, FPS: 60, SampleRate: 44100})
	n := 1
	if mode == "repeats-ready" {
		n = 3
	}
	for i := 0; i < n; i++ {
		_ = ipc.Write(os.Stdout, ipc.MsgReady, ready)
	}
	if mode == "forks" {
		forkAChild()
	}
	for {
		c, _, err := rd.Next()
		if err != nil {
			os.Exit(0)
		}
		switch c {
		case ipc.CmdState: // this core can't read a state another one made
			b, _ := json.Marshal(ipc.Event{Type: "statefail"})
			_ = ipc.Write(os.Stdout, ipc.MsgEvent, b)
		case ipc.CmdQuit:
			if mode == "saves-on-quit" {
				var id [4]byte
				binary.LittleEndian.PutUint32(id[:], 0)
				_ = ipc.Write(os.Stdout, ipc.MsgState, id[:], []byte("a fresh machine"))
			}
			os.Exit(0)
		}
	}
}

func faultyManager(t *testing.T, mode string) *Manager {
	t.Helper()
	self, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	return NewManager(Config{Runner: self, RunnerArgs: []string{"-test.run=^TestFaultyRunner$", "--", mode}, FFmpeg: "nope"})
}

func faultyOptions(t *testing.T, core string) Options {
	t.Helper()
	return Options{UserID: 1, ItemID: 7, Title: "Test", Core: core, ROM: "fake", SystemDir: t.TempDir(), SaveDir: t.TempDir()}
}

func stopsWithin(s *Session, d time.Duration) bool {
	done := make(chan struct{})
	go func() { s.Stop(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

func TestSayingReadyTwiceCantHoldUpStopping(t *testing.T) {
	m := faultyManager(t, "repeats-ready")
	defer m.StopAll() // before the temporary folders go
	s, err := m.Start(faultyOptions(t, "fake"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if !stopsWithin(s, 10*time.Second) {
		t.Fatalf("Stop hung, holding %d sessions", m.Count())
	}
}

// A child keeps the runner's pipes open after the runner is gone.
func TestAChildCantHoldUpStopping(t *testing.T) {
	was := pipesGrace
	pipesGrace = time.Second
	defer func() { pipesGrace = was }()
	m := faultyManager(t, "forks")
	defer m.StopAll() // before the temporary folders go
	s, err := m.Start(faultyOptions(t, "fake"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if !stopsWithin(s, 30*time.Second) {
		t.Fatalf("Stop hung, holding %d sessions", m.Count())
	}
}

// A runner that has stopped reading its commands, with the pipe to it full.
func TestARunnerNotReadingCantHoldUpStopping(t *testing.T) {
	m := faultyManager(t, "plain")
	defer m.StopAll() // before the temporary folders go
	s, err := m.Start(faultyOptions(t, "fake"))
	if err != nil {
		t.Fatal(err)
	}
	s.stdinMu.Lock() // as a command blocked on that pipe would
	defer s.stdinMu.Unlock()
	s.stopping.Store(true)
	killed := make(chan struct{})
	go func() {
		s.Stop()
		close(killed)
	}()
	time.Sleep(300 * time.Millisecond)
	s.kill() // what Stop's own limit does, sooner
	select {
	case <-killed:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop hung behind the command")
	}
}

func TestLeavingWhileLoadingDoesntStartTheNextCore(t *testing.T) {
	m := faultyManager(t, "plain")
	defer m.StopAll() // before the temporary folders go
	a := faultyOptions(t, "slow")
	a.Fallbacks = []CoreChoice{{Core: "fast"}}
	cleaned := make(chan struct{}, 1)
	a.Cleanup = func() { cleaned <- struct{}{} }
	first := make(chan error, 1)
	go func() {
		_, err := m.Start(a)
		first <- err
	}()
	time.Sleep(500 * time.Millisecond)
	b := faultyOptions(t, "fast")
	b.ItemID = 8
	if _, err := m.Start(b); err != nil {
		t.Fatal(err)
	}
	if err := <-first; !errors.Is(err, ErrReplaced) {
		t.Errorf("the game left behind: %v", err)
	}
	if n := m.Count(); n != 1 {
		t.Errorf("%d games for one account", n)
	}
	select {
	case <-cleaned:
	case <-time.After(5 * time.Second):
		t.Error("what the game held back wasn't let go")
	}
}

func TestANextCoreBringsItsOwnSettings(t *testing.T) {
	m := faultyManager(t, "plain")
	defer m.StopAll() // before the temporary folders go
	o := faultyOptions(t, "dies")
	o.CoreID = "first"
	o.RunnerArgs = []string{"-first-core"}
	o.Fallbacks = []CoreChoice{{Core: "fast", CoreID: "second", RunnerArgs: []string{"-second-core"}}}
	s, err := m.Start(o)
	if err != nil {
		t.Fatal(err)
	}
	if s.CoreID() != "second" {
		t.Errorf("core %q", s.CoreID())
	}
	if got := s.Info().Core; !strings.Contains(got, "-second-core") || strings.Contains(got, "-first-core") {
		t.Errorf("the runner was given %q", got)
	}
}

func TestTwoStartsAtOnceEndCleanly(t *testing.T) {
	m := faultyManager(t, "plain")
	defer m.StopAll() // before the temporary folders go
	var wg sync.WaitGroup
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = m.Start(faultyOptions(t, "fast"))
		}()
		time.Sleep(25 * time.Millisecond)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		t.Fatal("some are still waiting")
	}
}

// A session stopped when no runner could be started has nothing to wait for.
func TestStoppingWithNoRunner(t *testing.T) {
	m := NewManager(Config{Runner: filepath.Join(t.TempDir(), "no-such-program"), FFmpeg: "nope"})
	o := faultyOptions(t, "fake")
	s := &Session{m: m, opt: o, cfg: m.config(), log: m.log, done: make(chan struct{}), spawned: make(chan struct{}), stderr: &tailWriter{max: 100}}
	if err := s.start(); err == nil {
		t.Fatal("it started")
	}
	if !stopsWithin(s, 5*time.Second) {
		t.Fatal("Stop hung")
	}
}

func TestAResumeThatFailsKeepsTheSave(t *testing.T) {
	m := faultyManager(t, "saves-on-quit")
	defer m.StopAll() // before the temporary folders go
	o := faultyOptions(t, "fake")
	o.Resume = true
	if err := os.WriteFile(filepath.Join(o.SaveDir, "auto.state"), []byte("hours of progress"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := m.Start(o)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for s.resumed.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.Info().Resumed {
		t.Error("it says it resumed")
	}
	s.Stop()
	prev, err := os.ReadFile(filepath.Join(o.SaveDir, "autoprev.state"))
	if err != nil || string(prev) != "hours of progress" {
		t.Errorf("the save the core couldn't load: %q, %v", prev, err)
	}
	if auto, _ := os.ReadFile(filepath.Join(o.SaveDir, "auto.state")); string(auto) != "a fresh machine" {
		t.Errorf("auto.state: %q", auto)
	}
}

func TestLoadingIsntBeingLeftAlone(t *testing.T) {
	s := &Session{viewers: map[*viewer]bool{}, noViewerSince: time.Now().Add(-time.Hour), lastInput: time.Now()}
	if s.idleTooLong() {
		t.Error("a game still loading was stopped for having nobody watching")
	}
	s.loaded.Store(true)
	if !s.idleTooLong() {
		t.Error("a loaded game nobody is watching should stop")
	}
}

// Nobody has anything to watch until it's loaded, however long that took.
func TestTheWaitForAViewerStartsWhenItsLoaded(t *testing.T) {
	m := faultyManager(t, "plain")
	defer m.StopAll() // before the temporary folders go
	s, err := m.Start(faultyOptions(t, "fake"))
	if err != nil {
		t.Fatal(err)
	}
	s.viewMu.Lock()
	since := time.Since(s.noViewerSince)
	s.viewMu.Unlock()
	if !s.loaded.Load() || since > 5*time.Second {
		t.Errorf("loaded %v, waiting %v", s.loaded.Load(), since)
	}
}

func TestHoldingRewindIsntBeingStuck(t *testing.T) {
	s := &Session{}
	s.lastFrame.Store(time.Now().Add(-2 * stuckLimit).UnixNano())
	if !s.stuck() {
		t.Error("no frames for this long is stuck")
	}
	s.rewinding.Store(true)
	if s.stuck() {
		t.Error("held at the start of the rewind buffer, the game would be killed without saving")
	}
}

func TestAnEncoderThatDiesIsReplacedAndThenAvoided(t *testing.T) {
	m := NewManager(Config{Runner: "nope", FFmpeg: "nope"})
	s := &Session{m: m, log: m.log}
	for i := 1; i <= encoderLives; i++ {
		e := &videoEncoder{name: "libx264"}
		s.h264 = e
		s.encoderLost(e)
		if s.h264 != nil {
			t.Fatal("the dead one is still in use, so the picture would stay frozen")
		}
		m.mu.Lock()
		bad := m.badEnc["libx264"]
		m.mu.Unlock()
		if bad != (i == encoderLives) {
			t.Errorf("after %d: marked bad %v", i, bad)
		}
	}
}
