package host

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittyplay/engine"
	"github.com/wittywilla/wittyplay/ipc"
)

// counter is a machine whose whole state is a frame number, which it draws as the
// first four bytes of its picture. It makes what a control did easy to see.
type counter struct {
	h         engine.Handlers
	n         uint32
	failLoad  bool
	noStates  bool
	option    string
	cheats    map[int]string
	big       int
	slowState time.Duration
	drawn     int
	videoOff  bool
}

func (c *counter) SystemInfo() engine.SystemInfo { return engine.SystemInfo{Name: "counter"} }
func (c *counter) LoadGame(string) (engine.AVInfo, error) {
	return engine.AVInfo{BaseWidth: 2, BaseHeight: 1, FPS: 120, SampleRate: 44100}, nil
}
func (c *counter) Run() {
	if c.h.Input != nil {
		c.h.Input()
	}
	c.n++
	if !c.videoOff {
		c.drawn++
	}
	var f [8]byte
	binary.LittleEndian.PutUint32(f[:], c.n)
	c.h.Video(f[:], 2, 1, 8, engine.FormatXRGB8888)
	c.h.Audio([]int16{1, 1})
}
func (c *counter) Reset() { c.n = 0 }
func (c *counter) SaveState() []byte {
	if c.noStates {
		return nil
	}
	time.Sleep(c.slowState)
	b := make([]byte, 4+c.big)
	binary.LittleEndian.PutUint32(b, c.n)
	return b
}
func (c *counter) LoadState(b []byte) bool {
	if c.failLoad || len(b) < 4 {
		return false
	}
	c.n = binary.LittleEndian.Uint32(b)
	return true
}
func (c *counter) SaveRAM() []byte { return nil }
func (c *counter) Close()          {}
func (c *counter) Options() []engine.Option {
	return []engine.Option{{Key: "k", Label: "K", Choices: []string{"a", "b"}, Value: c.option}}
}
func (c *counter) SetOption(key, value string) bool {
	if key == "k" && (value == "a" || value == "b") {
		c.option = value
		return true
	}
	return false
}
func (c *counter) CheatReset() { c.cheats = map[int]string{} }
func (c *counter) CheatSet(i int, on bool, code string) {
	if on {
		c.cheats[i] = code
	}
}
func (c *counter) SetOutput(video, audio, ff bool) { c.videoOff = !video }

// harness runs a host on pipes, collecting what it says.
type harness struct {
	t    *testing.T
	in   *io.PipeWriter
	core *counter

	mu      sync.Mutex
	frames  []uint32
	audio   int
	events  []ipc.Event
	stopped chan struct{}
}

func newHarness(t *testing.T, core *counter) *harness {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	h := &harness{t: t, in: inW, core: core, stopped: make(chan struct{})}
	go func() {
		_ = Run(inR, outW, func(l ipc.Load, hd engine.Handlers) (engine.Engine, error) {
			core.h = hd
			return core, nil
		})
		outW.Close()
	}()
	ready := make(chan struct{})
	go func() {
		defer close(h.stopped)
		rd := ipc.NewReader(outR)
		once := sync.Once{}
		for {
			m, p, err := rd.Next()
			if err != nil {
				return
			}
			h.mu.Lock()
			switch m {
			case ipc.MsgReady:
				once.Do(func() { close(ready) })
			case ipc.MsgVideo:
				_, d, err := ipc.ParseVideo(p)
				if err == nil {
					h.frames = append(h.frames, binary.LittleEndian.Uint32(d))
				}
			case ipc.MsgAudio:
				h.audio++
			case ipc.MsgEvent:
				var e ipc.Event
				if json.Unmarshal(p, &e) == nil {
					h.events = append(h.events, e)
				}
			}
			h.mu.Unlock()
		}
	}()
	h.send(ipc.CmdLoad, ipc.Load{Core: "x"})
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("no ready message")
	}
	t.Cleanup(func() {
		_ = ipc.Write(inW, ipc.CmdQuit, []byte{0})
		select {
		case <-h.stopped:
		case <-time.After(5 * time.Second):
			t.Error("the host didn't stop")
		}
	})
	return h
}

func (h *harness) send(t byte, v any) {
	h.t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := ipc.Write(h.in, t, b); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) pause(on bool) {
	b := byte(0)
	if on {
		b = 1
	}
	_ = ipc.Write(h.in, ipc.CmdPause, []byte{b})
}

// run lets the game run for a while and returns the frames shown and the audio messages.
func (h *harness) run(d time.Duration) (frames []uint32, audio int) {
	h.mu.Lock()
	h.frames, h.audio = nil, 0
	h.mu.Unlock()
	time.Sleep(d)
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]uint32(nil), h.frames...), h.audio
}

// collect waits for some frames, however busy the machine is, and returns them with how
// many lots of sound came with them.
func (h *harness) collect(t *testing.T, n int) (frames []uint32, audio int) {
	t.Helper()
	h.mu.Lock()
	h.frames, h.audio = nil, 0
	h.mu.Unlock()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		have := len(h.frames)
		h.mu.Unlock()
		if have >= n {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]uint32(nil), h.frames...), h.audio
}

func (h *harness) waitEvent(t *testing.T, typ string) *ipc.Event {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if e := h.findEvent(typ); e != nil {
			return e
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

func (h *harness) waitFrame(t *testing.T, want func(uint32) bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		found := false
		for _, f := range h.frames {
			found = found || want(f)
		}
		h.mu.Unlock()
		if found {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("that frame never came")
}

func (h *harness) findEvent(typ string) *ipc.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := len(h.events) - 1; i >= 0; i-- {
		if h.events[i].Type == typ {
			return &h.events[i]
		}
	}
	return nil
}

func ptr[T any](v T) *T { return &v }

func TestFastForwardSendsNoMoreFramesThanNormal(t *testing.T) {
	c := &counter{}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{Speed: ptr(4.0)})
	time.Sleep(30 * time.Millisecond) // what was on its way when that was said
	frames, audio := h.run(500 * time.Millisecond)
	// The game's own rate is 120 a second, so about 60 go out in this time. However
	// fast the game runs at four times that, no more than that may be sent.
	if len(frames) > 90 {
		t.Fatalf("%d frames went out in half a second: fast-forward is costing the encoder", len(frames))
	}
	if audio != 0 {
		t.Fatalf("%d lots of sound during fast-forward", audio)
	}
	frames, _ = h.collect(t, 2)
	if len(frames) < 2 {
		t.Fatal("nothing was shown")
	}
}

// Which frames are sent is decided here, with no clock running.
func TestWhichFramesAreSent(t *testing.T) {
	now := time.Now()
	frame := 10 * time.Millisecond
	c := &control{speed: 1, frameTime: frame}
	if !c.show(now) {
		t.Error("at normal speed every frame goes out")
	}
	c.speed = 4
	c.lastShown = now.Add(-2 * time.Millisecond)
	if c.show(now) {
		t.Error("going fast, a frame so soon after the last one should stay back")
	}
	c.lastShown = now.Add(-frame)
	if !c.show(now) {
		t.Error("going fast, a frame is still due at the game's own rate")
	}
	c.speed = 1
	c.skipMode, c.skipped = ipc.FrameSkipAuto, 0
	c.lag = 0
	if !c.show(now) {
		t.Error("automatic skip shouldn't skip when on time")
	}
	c.lag = 3 * frame
	if c.show(now) {
		t.Error("automatic skip should skip when behind")
	}
	c.skipped = 3
	if !c.show(now) {
		t.Error("however far behind, some frames must get out")
	}
	c.skipMode, c.skipped = 2, 0
	c.lag = 0
	if c.show(now) {
		t.Error("fixed skip should skip")
	}
	c.skipped = 2
	if !c.show(now) {
		t.Error("fixed skip should send one after skipping")
	}
	c.skipped, c.advance = 0, 1
	if !c.show(now) {
		t.Error("a frame advanced by hand always shows")
	}
}

func TestSlowMotionRunsSlower(t *testing.T) {
	c := &counter{}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{Speed: ptr(0.25)})
	time.Sleep(30 * time.Millisecond)
	frames, audio := h.run(500 * time.Millisecond)
	if len(frames) > 25 {
		t.Fatalf("%d frames in half a second at quarter speed of 120 a second", len(frames))
	}
	if audio != 0 {
		t.Fatalf("%d lots of sound during slow motion", audio)
	}
	if frames, _ = h.collect(t, 2); len(frames) < 2 {
		t.Fatal("the game stopped")
	}
}

func TestASpeedIsHeldToItsLimits(t *testing.T) {
	c := &counter{}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{Speed: ptr(1e12)})
	h.send(ipc.CmdControl, ipc.Control{Speed: ptr(-5.0)})
	time.Sleep(30 * time.Millisecond)
	frames, _ := h.run(300 * time.Millisecond)
	if len(frames) > 10 {
		t.Fatalf("%d frames: a negative speed wasn't held to the slowest", len(frames))
	}
	// The slowest is a tenth, so a frame comes every 83ms.
	if frames, _ = h.collect(t, 1); len(frames) == 0 {
		t.Fatal("a nonsense speed stopped the game")
	}
}

func TestFixedFrameSkipSendsOneInSoMany(t *testing.T) {
	c := &counter{}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{FrameSkip: ptr(2)})
	time.Sleep(30 * time.Millisecond)
	frames, audio := h.collect(t, 6)
	if len(frames) < 6 {
		t.Fatalf("only %d frames", len(frames))
	}
	for i := 1; i < len(frames); i++ {
		if d := frames[i] - frames[i-1]; d != 3 {
			t.Fatalf("frames %d and %d: %d apart, want 3", frames[i-1], frames[i], d)
		}
	}
	if audio == 0 {
		t.Fatal("frame skip must keep the sound")
	}
}

func TestAutomaticFrameSkipKeepsAGameGoing(t *testing.T) {
	c := &counter{}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{FrameSkip: ptr(ipc.FrameSkipAuto)})
	time.Sleep(30 * time.Millisecond)
	if frames, _ := h.collect(t, 5); len(frames) < 5 {
		t.Fatalf("only %d frames", len(frames))
	}
}

func TestRewindGoesBack(t *testing.T) {
	c := &counter{}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{RewindBytes: ptr(int64(1 << 20)), RewindSeconds: ptr(30)})
	if frames, _ := h.collect(t, 12); len(frames) < 12 {
		t.Fatalf("only %d frames", len(frames))
	}
	h.send(ipc.CmdControl, ipc.Control{Rewinding: ptr(true)})
	time.Sleep(20 * time.Millisecond)
	back, audio := h.collect(t, 3)
	if len(back) < 3 {
		t.Fatalf("only %d frames while rewinding", len(back))
	}
	for i := 1; i < len(back); i++ {
		if back[i] >= back[i-1] {
			t.Fatalf("frames went %d then %d while rewinding", back[i-1], back[i])
		}
	}
	if audio != 0 {
		t.Fatalf("%d lots of sound while rewinding", audio)
	}
	h.send(ipc.CmdControl, ipc.Control{Rewinding: ptr(false)})
	time.Sleep(20 * time.Millisecond)
	on, _ := h.collect(t, 4)
	for i := 1; i < len(on); i++ {
		if on[i] != on[i-1]+1 {
			t.Fatalf("frames went %d then %d after rewinding", on[i-1], on[i])
		}
	}
}

// Rewinding on a core with no states, or with states too big, turns itself off and
// says why. The game carries on.
func TestRewindGivesUpWhenItCant(t *testing.T) {
	for name, c := range map[string]*counter{"no states": {noStates: true}, "too big": {big: 8 << 20}} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, c)
			h.pause(false)
			h.send(ipc.CmdControl, ipc.Control{RewindBytes: ptr(int64(1 << 20))})
			if frames, _ := h.collect(t, 3); len(frames) < 3 {
				t.Fatal("the game stopped")
			}
			if h.waitEvent(t, "message") == nil {
				t.Fatal("nobody was told")
			}
		})
	}
}

// A state that takes too long to save slows the capture down, not the game. This drives
// the capture directly, so that how busy the machine is doesn't change the answer.
func TestRewindBacksOffWhenStatesAreSlow(t *testing.T) {
	core := &counter{slowState: 30 * time.Millisecond}
	c := &control{eng: core, frameTime: 100 * time.Millisecond, rewindEvery: 1, rewind: newRewindBuffer(1<<20, 1000)}
	c.r = &runner{out: io.Discard, ctl: c}
	for i := 0; i < 60; i++ {
		core.n++
		c.captureRewind()
	}
	if c.rewind == nil {
		t.Fatal("it gave up before it had tried taking states less often")
	}
	if c.rewindEvery < 2 {
		t.Fatalf("still taking a state every %d frames", c.rewindEvery)
	}
	// And once it's cheap again, it goes back to keeping more.
	core.slowState = 0
	for i := 0; i < 400; i++ {
		core.n++
		c.captureRewind()
	}
	if c.rewindEvery != 1 {
		t.Fatalf("still taking a state every %d frames", c.rewindEvery)
	}
	// A state that takes longer than a frame, even that rarely, is more than it can afford.
	core.slowState = 2 * time.Millisecond
	c.frameTime = time.Millisecond
	for i := 0; i < 200 && c.rewind != nil; i++ {
		core.n++
		c.captureRewind()
	}
	if c.rewind != nil {
		t.Fatal("it should have given up")
	}
}

func TestFrameAdvanceRunsExactlyThatMany(t *testing.T) {
	c := &counter{}
	h := newHarness(t, c)
	h.send(ipc.CmdControl, ipc.Control{Advance: ptr(3)})
	h.waitFrame(t, func(f uint32) bool { return f == 3 })
	time.Sleep(50 * time.Millisecond)
	h.mu.Lock()
	n := len(h.frames)
	h.mu.Unlock()
	if n != 3 {
		t.Fatalf("%d frames for three advances", n)
	}
	// One more, and pausing again is the same as not having moved.
	h.send(ipc.CmdControl, ipc.Control{Advance: ptr(1)})
	h.waitFrame(t, func(f uint32) bool { return f == 4 })
}

func TestRunAheadShowsTheFutureAndKeepsThePresent(t *testing.T) {
	c := &counter{}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{RunAhead: ptr(2)})
	time.Sleep(30 * time.Millisecond)
	frames, audio := h.collect(t, 6)
	if len(frames) < 6 {
		t.Fatalf("only %d frames", len(frames))
	}
	// The picture is from two frames on, but the machine itself only advanced one a time.
	for i := 1; i < len(frames); i++ {
		if frames[i] != frames[i-1]+1 {
			t.Fatalf("frames went %d then %d", frames[i-1], frames[i])
		}
	}
	if audio == 0 || audio > len(frames)+2 {
		t.Fatalf("%d lots of sound for %d frames: the extra frames' sound got out", audio, len(frames))
	}
}

// A machine that can't keep up turns run-ahead off, and says so. A busy moment doesn't.
func TestRunAheadTurnsItselfOffWhenTooSlow(t *testing.T) {
	c := &counter{slowState: 20 * time.Millisecond}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{RunAhead: ptr(2)})
	if h.waitEvent(t, "message") == nil {
		t.Fatal("nobody was told")
	}
	c.slowState = 0
	if frames, _ := h.collect(t, 2); len(frames) < 2 {
		t.Fatal("the game stopped")
	}
}

func TestABusyMomentLeavesRunAheadOn(t *testing.T) {
	core := &counter{h: engine.Handlers{Video: func([]byte, int, int, int, int) {}, Audio: func([]int16) {}}}
	c := &control{eng: core, frameTime: 10 * time.Millisecond, runAhead: 1, speed: 1}
	c.r = &runner{out: io.Discard, ctl: c}
	// A second of lateness, then on time for longer: not what turns it off.
	c.aheadSince = time.Now().Add(-2 * time.Second)
	c.aheadLate, c.aheadOnTime = 60, 100
	c.runAheadFrame(true)
	if c.runAhead != 1 {
		t.Fatal("a busy moment turned it off")
	}
	// Two seconds of nothing else does.
	c.aheadSince = time.Now().Add(-2 * time.Second)
	c.aheadLate, c.aheadOnTime = 30, 2
	c.runAheadFrame(true)
	if c.runAhead != 0 {
		t.Fatal("it should have given up")
	}
}

func TestOptionsAreListedAndOnlyOfferedValuesTakeEffect(t *testing.T) {
	c := &counter{option: "a"}
	h := newHarness(t, c)
	e := h.findEvent("options")
	if e == nil || len(e.Options) != 1 || e.Options[0].Key != "k" {
		t.Fatalf("the options weren't listed: %+v", e)
	}
	h.pause(false)
	h.send(ipc.CmdOption, ipc.OptionSet{Key: "k", Value: "../../etc/passwd"})
	h.send(ipc.CmdOption, ipc.OptionSet{Key: "other", Value: "a"})
	h.send(ipc.CmdOption, ipc.OptionSet{Key: "k", Value: "b"})
	time.Sleep(50 * time.Millisecond)
	if c.option != "b" {
		t.Fatalf("option is %q", c.option)
	}
}

func TestCheatsReplaceTheList(t *testing.T) {
	c := &counter{cheats: map[int]string{}}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdCheats, []ipc.Cheat{{Code: "one", Enabled: true}, {Code: "two", Enabled: false}})
	h.send(ipc.CmdCheats, []ipc.Cheat{{Code: "three", Enabled: true}})
	time.Sleep(50 * time.Millisecond)
	if len(c.cheats) != 1 || c.cheats[0] != "three" {
		t.Fatalf("cheats are %v", c.cheats)
	}
}

// A control that makes no sense changes nothing, and the game keeps running.
func TestBadControlsChangeNothing(t *testing.T) {
	c := &counter{}
	h := newHarness(t, c)
	h.pause(false)
	for _, bad := range []string{`{`, `"text"`, `{"speed":"fast"}`, `[]`, `{"frameSkip":99999}`, `{"rewindBytes":-1}`, `{"runAhead":100000}`} {
		if err := ipc.Write(h.in, ipc.CmdControl, []byte(bad)); err != nil {
			t.Fatal(err)
		}
		if err := ipc.Write(h.in, ipc.CmdOption, []byte(bad)); err != nil {
			t.Fatal(err)
		}
		if err := ipc.Write(h.in, ipc.CmdCheats, []byte(bad)); err != nil {
			t.Fatal(err)
		}
	}
	if frames, _ := h.collect(t, 2); len(frames) < 2 {
		t.Fatal("the game stopped")
	}
}

func TestProbeSaysWhatsOfferedWithoutAGame(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- Run(inR, outW, func(l ipc.Load, hd engine.Handlers) (engine.Engine, error) {
			return &counter{h: hd, option: "a"}, nil
		})
		outW.Close()
	}()
	b, _ := json.Marshal(ipc.Load{Core: "x", Probe: true})
	if err := ipc.Write(inW, ipc.CmdLoad, b); err != nil {
		t.Fatal(err)
	}
	rd := ipc.NewReader(outR)
	var offered []ipc.Option
	for {
		m, p, err := rd.Next()
		if err != nil {
			break
		}
		if m == ipc.MsgEvent {
			var e ipc.Event
			if json.Unmarshal(p, &e) == nil && e.Type == "options" {
				offered = e.Options
			}
		}
	}
	if len(offered) != 1 {
		t.Fatalf("got %v", offered)
	}
	<-done
}
