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
	if len(frames) == 0 {
		t.Fatal("nothing was shown")
	}
	// The game's own rate is 120 a second. At four times that it runs some 240 frames
	// in this time, and only about 60 go out.
	if len(frames) > 90 {
		t.Fatalf("%d frames went out in half a second: fast-forward is costing the encoder", len(frames))
	}
	ran := frames[len(frames)-1] - frames[0]
	if ran < 120 {
		t.Fatalf("the game only got through %d frames: it didn't speed up", ran)
	}
	if audio != 0 {
		t.Fatalf("%d lots of sound during fast-forward", audio)
	}
}

func TestSlowMotionRunsSlower(t *testing.T) {
	c := &counter{}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{Speed: ptr(0.25)})
	time.Sleep(30 * time.Millisecond)
	frames, audio := h.run(500 * time.Millisecond)
	if len(frames) > 25 || len(frames) < 5 {
		t.Fatalf("%d frames in half a second at quarter speed of 120 a second", len(frames))
	}
	if audio != 0 {
		t.Fatalf("%d lots of sound during slow motion", audio)
	}
}

func TestASpeedIsHeldToItsLimits(t *testing.T) {
	c := &counter{}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{Speed: ptr(1e12)})
	h.send(ipc.CmdControl, ipc.Control{Speed: ptr(-5.0)})
	frames, _ := h.run(300 * time.Millisecond)
	if len(frames) == 0 {
		// The lowest speed is a tenth, so a frame comes every 83ms.
		t.Fatal("a nonsense speed stopped the game")
	}
	if len(frames) > 10 {
		t.Fatalf("%d frames: a negative speed wasn't held to the slowest", len(frames))
	}
}

func TestFixedFrameSkipSendsOneInSoMany(t *testing.T) {
	c := &counter{}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{FrameSkip: ptr(2)})
	time.Sleep(30 * time.Millisecond)
	frames, audio := h.run(400 * time.Millisecond)
	if len(frames) < 5 {
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
	if c.drawn >= int(c.n) {
		t.Fatalf("the core drew %d of %d frames: it wasn't told it could skip", c.drawn, c.n)
	}
}

func TestAutomaticFrameSkipOnlyWhenBehind(t *testing.T) {
	c := &counter{}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{FrameSkip: ptr(ipc.FrameSkipAuto)})
	time.Sleep(30 * time.Millisecond)
	frames, _ := h.run(300 * time.Millisecond)
	skips := 0
	for i := 1; i < len(frames); i++ {
		if frames[i]-frames[i-1] > 1 {
			skips++
		}
	}
	if skips > len(frames)/3 {
		t.Fatalf("%d skips in %d frames on a machine with nothing to do", skips, len(frames))
	}
}

func TestRewindGoesBack(t *testing.T) {
	c := &counter{}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{RewindBytes: ptr(int64(1 << 20)), RewindSeconds: ptr(30)})
	frames, _ := h.run(300 * time.Millisecond)
	if len(frames) < 10 {
		t.Fatalf("only %d frames", len(frames))
	}
	h.send(ipc.CmdControl, ipc.Control{Rewinding: ptr(true)})
	time.Sleep(20 * time.Millisecond)
	back, audio := h.run(100 * time.Millisecond)
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
	on, _ := h.run(100 * time.Millisecond)
	for i := 1; i < len(on); i++ {
		if on[i] != on[i-1]+1 {
			t.Fatalf("frames went %d then %d after rewinding", on[i-1], on[i])
		}
	}
}

// Rewinding on a core with no states, or with states too big, turns itself off and
// says why. The game carries on.
func TestRewindGivesUpWhenItCant(t *testing.T) {
	t.Run("no states", func(t *testing.T) {
		c := &counter{noStates: true}
		h := newHarness(t, c)
		h.pause(false)
		h.send(ipc.CmdControl, ipc.Control{RewindBytes: ptr(int64(1 << 20))})
		frames, _ := h.run(100 * time.Millisecond)
		if len(frames) == 0 {
			t.Fatal("the game stopped")
		}
		if h.findEvent("message") == nil {
			t.Fatal("nobody was told")
		}
	})
	t.Run("too big", func(t *testing.T) {
		c := &counter{big: 8 << 20}
		h := newHarness(t, c)
		h.pause(false)
		h.send(ipc.CmdControl, ipc.Control{RewindBytes: ptr(int64(1 << 20))})
		frames, _ := h.run(100 * time.Millisecond)
		if len(frames) == 0 {
			t.Fatal("the game stopped")
		}
		if e := h.findEvent("message"); e == nil {
			t.Fatal("nobody was told")
		}
	})
}

// A state that takes too long to save slows the capture down, not the game.
func TestRewindBacksOffWhenStatesAreSlow(t *testing.T) {
	c := &counter{slowState: 3 * time.Millisecond} // a quarter of a frame at 120 a second
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{RewindBytes: ptr(int64(1 << 20))})
	time.Sleep(200 * time.Millisecond)
	frames, _ := h.run(300 * time.Millisecond)
	if len(frames) < 25 {
		t.Fatalf("%d frames in 0.3 s of 120 a second: rewind took the frame rate with it", len(frames))
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
	frames, audio := h.run(200 * time.Millisecond)
	if len(frames) < 5 {
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

// A machine that can't keep up turns run-ahead off, and says so.
func TestRunAheadTurnsItselfOffWhenTooSlow(t *testing.T) {
	c := &counter{slowState: 20 * time.Millisecond}
	h := newHarness(t, c)
	h.pause(false)
	h.send(ipc.CmdControl, ipc.Control{RunAhead: ptr(2)})
	frames, _ := h.run(1500 * time.Millisecond)
	if len(frames) == 0 {
		t.Fatal("the game stopped")
	}
	if h.findEvent("message") == nil {
		t.Fatal("nobody was told")
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
	frames, _ := h.run(100 * time.Millisecond)
	if len(frames) == 0 {
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
