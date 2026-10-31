package host

import (
	"encoding/json"
	"time"

	"github.com/wittywilla/wittyplay/engine"
	"github.com/wittywilla/wittyplay/ipc"
)

// control is everything about how a game runs beyond running it: speed, frame skip,
// rewind, run-ahead and frame advance, plus core options and cheats. All of it happens
// on the one thread that runs the engine.
type control struct {
	r   *runner
	eng engine.Engine

	speed     float64
	skipMode  int
	skipped   int // in a row
	lag       time.Duration
	lastShown time.Time
	advance   int

	rewind        *rewindBuffer
	rewindSeconds int
	rewindBytes   int64
	rewinding     bool
	rewindEvery   int
	rewindCount   int
	rewindEmpty   bool
	stateBuf      []byte

	runAhead  int
	aheadBuf  []byte
	aheadSlow int // frames over budget, less those under

	frameTime time.Duration

	// What the next frame sends. A runner's write looks at these, so nothing gets out
	// past them whichever way a core delivers its frames.
	dropVideo, dropAudio bool
	quiet                bool // replaying: rumble doesn't count
	noInput              [engine.MaxPorts]engine.PadState

	statsAt             time.Time
	ran, shown, skipSum int
}

func newControl(r *runner) *control {
	return &control{r: r, speed: 1, rewindEvery: 1}
}

// wrap puts the control's gates around what an engine calls.
func (c *control) wrap(h engine.Handlers) engine.Handlers {
	video, input, rumble := h.Video, h.Input, h.Rumble
	h.Video = func(data []byte, w, ht, stride, format int) {
		if !c.dropVideo { // no point copying what won't be sent
			video(data, w, ht, stride, format)
		}
	}
	h.Input = func() *[engine.MaxPorts]engine.PadState {
		if c.rewinding {
			return &c.noInput
		}
		return input()
	}
	h.Rumble = func(port, effect, strength int) {
		if !c.quiet {
			rumble(port, effect, strength)
		}
	}
	return h
}

// pace is how many times faster than normal the loop runs. Rewind is always at normal
// speed: it's watched, and it spends its memory one step at a time.
func (c *control) pace() float64 {
	if c.rewinding {
		return 1
	}
	return c.speed
}

// wantsFrame says a paused game has frames to run.
func (c *control) wantsFrame() bool { return c.advance > 0 }

// command handles a control, option or cheat message.
func (c *control) command(t byte, p []byte) {
	switch t {
	case ipc.CmdControl:
		var m ipc.Control
		if json.Unmarshal(p, &m) == nil {
			c.apply(m)
		}
	case ipc.CmdOption:
		var m ipc.OptionSet
		if o, ok := c.eng.(engine.Options); ok && json.Unmarshal(p, &m) == nil && o.SetOption(m.Key, m.Value) {
			c.sendOptions()
		}
	case ipc.CmdCheats:
		var list []ipc.Cheat
		if ch, ok := c.eng.(engine.Cheats); ok && json.Unmarshal(p, &list) == nil {
			ch.CheatReset()
			for i, x := range list {
				ch.CheatSet(i, x.Enabled, x.Code)
			}
		}
	}
}

func (c *control) apply(m ipc.Control) {
	if m.Speed != nil && *m.Speed == *m.Speed { // not NaN
		c.speed = min(max(*m.Speed, ipc.SpeedMin), ipc.SpeedMax)
	}
	if m.FrameSkip != nil {
		c.skipMode = min(max(*m.FrameSkip, ipc.FrameSkipAuto), ipc.FrameSkipMax)
		c.skipped = 0
	}
	if m.RunAhead != nil {
		c.runAhead = min(max(*m.RunAhead, 0), ipc.RunAheadMax)
	}
	if m.Advance != nil {
		c.advance = min(max(*m.Advance, 0), ipc.AdvanceMax)
	}
	if m.RewindSeconds != nil {
		c.rewindSeconds = min(max(*m.RewindSeconds, 1), 600)
	}
	if m.RewindBytes != nil {
		c.rewindBytes = min(max(*m.RewindBytes, 0), ipc.RewindBytesMax)
	}
	if m.RewindBytes != nil || m.RewindSeconds != nil {
		c.resetRewind()
	}
	if m.Rewinding != nil {
		c.rewinding = *m.Rewinding && c.rewind != nil
		c.rewindEmpty = false
	}
}

func (c *control) resetRewind() {
	c.rewinding = false
	if c.rewindBytes <= 0 {
		c.rewind = nil
		return
	}
	c.rewind = newRewindBuffer(c.rewindBytes, c.maxSteps())
	c.rewindEvery, c.rewindCount = 1, 0
}

func (c *control) maxSteps() int {
	secs := c.rewindSeconds
	if secs <= 0 {
		secs = 20
	}
	fps := 60.0
	if c.frameTime > 0 {
		fps = float64(time.Second) / float64(c.frameTime)
	}
	return max(int(float64(secs)*fps), 1)
}

// stateChanged: loading a state or resetting makes what rewind holds a different
// timeline, so it starts over from here.
func (c *control) stateChanged() {
	if c.rewind != nil {
		c.resetRewind()
	}
}

func (c *control) sendOptions() {
	if o, ok := c.eng.(engine.Options); ok {
		list := o.Options()
		out := make([]ipc.Option, 0, len(list))
		for _, x := range list {
			out = append(out, ipc.Option{Key: x.Key, Label: x.Label, Choices: x.Choices, Value: x.Value})
		}
		c.r.event(ipc.Event{Type: "options", Options: out})
	}
}

// show decides whether this frame's picture is sent. Going faster than normal, no more
// are sent than the game's own frame rate: the viewer can't use more, and the encoder
// and the pipe would pay for them. Frame skip leaves out more, on request or when the
// machine has fallen behind.
func (c *control) show(now time.Time) bool {
	if c.advance > 0 {
		return true
	}
	if c.speed > 1 && now.Sub(c.lastShown) < c.frameTime*9/10 {
		return false
	}
	switch {
	case c.skipMode > 0:
		if c.skipped < c.skipMode {
			return false
		}
	case c.skipMode == ipc.FrameSkipAuto:
		if c.lag > c.frameTime && c.skipped < 3 {
			return false
		}
	}
	return true
}

// runFrame stands in for the engine's Run: one step of the game, in whichever mode is on.
func (c *control) runFrame() {
	now := time.Now()
	if c.rewinding {
		c.stepBack()
		c.count(true, now)
		return
	}
	show := c.show(now)
	normal := c.speed == 1
	c.setOutput(show, normal)
	if c.runAhead > 0 && normal && c.advance == 0 {
		c.runAheadFrame(show)
	} else {
		c.eng.Run()
	}
	c.captureRewind()
	if c.dropAudio {
		c.r.audio = c.r.audio[:0]
	}
	c.dropVideo, c.dropAudio = false, false
	if c.advance > 0 {
		c.advance--
	}
	c.count(show, now)
}

func (c *control) setOutput(video, audio bool) {
	c.dropVideo, c.dropAudio = !video, !audio
	if o, ok := c.eng.(engine.Output); ok {
		o.SetOutput(video, audio, c.speed > 1)
	}
}

func (c *control) count(show bool, now time.Time) {
	c.ran++
	if show {
		c.shown++
		c.skipped = 0
		c.lastShown = now
	} else {
		c.skipped++
		c.skipSum++
	}
	if now.Sub(c.statsAt) >= time.Second {
		if !c.statsAt.IsZero() {
			st := &ipc.RunStats{Ran: c.ran, Shown: c.shown, Skipped: c.skipSum}
			if c.rewind != nil {
				st.RewindBytes, st.RewindFrames = c.rewind.held, c.rewind.len()
			}
			c.r.event(ipc.Event{Type: "stats", Stats: st})
		}
		c.statsAt, c.ran, c.shown, c.skipSum = now, 0, 0, 0
	}
}

// saveState takes a state with as little garbage as the engine allows.
func (c *control) saveState(buf *[]byte) []byte {
	if s, ok := c.eng.(engine.StateInto); ok {
		out := s.SaveStateInto(*buf)
		if out != nil {
			*buf = out
		}
		return out
	}
	return c.eng.SaveState()
}

// captureRewind keeps a state now and then. How often depends on what it costs: with a
// slow machine or a big state it backs off, so that rewind never takes the frame rate
// with it, and when even that's too much, it gives up and says so.
func (c *control) captureRewind() {
	if c.rewind == nil {
		return
	}
	c.rewindCount++
	if c.rewindCount < c.rewindEvery {
		return
	}
	c.rewindCount = 0
	start := time.Now()
	state := c.saveState(&c.stateBuf)
	if len(state) == 0 {
		c.rewind = nil
		c.r.event(ipc.Event{Type: "message", Text: "This core can’t rewind: it has no save states."})
		return
	}
	if !c.rewind.push(state) {
		c.rewind = nil
		c.r.event(ipc.Event{Type: "message", Text: "This game’s saved state is too large to rewind with the memory allowed."})
		return
	}
	cost := time.Since(start)
	switch {
	case cost > c.frameTime && c.rewindEvery >= 16:
		c.rewind = nil
		c.r.event(ipc.Event{Type: "message", Text: "Rewind is off: saving this game’s state takes too long on this server."})
	case cost > c.frameTime/4 && c.rewindEvery < 16:
		c.rewindEvery *= 2
	case cost < c.frameTime/16 && c.rewindEvery > 1:
		c.rewindEvery /= 2
	}
}

// stepBack restores the previous state and runs one frame of it for the picture, with
// no sound and nobody's buttons.
func (c *control) stepBack() {
	state, ok := c.rewind.pop()
	if !ok {
		if !c.rewindEmpty {
			c.rewindEmpty = true
			c.r.event(ipc.Event{Type: "message", Text: "That’s as far back as rewind goes."})
		}
		return
	}
	c.eng.LoadState(state)
	c.quiet = true
	c.setOutput(true, false)
	c.eng.Run()
	c.r.audio = c.r.audio[:0]
	c.quiet = false
	c.dropVideo, c.dropAudio = false, false
}

// runAheadFrame shows the future: it runs the frame that counts with its sound, saves
// the state, runs on a little with what's held now, sends only that last picture, and
// goes back. The game reacts that many frames sooner. It costs that many extra frames
// and a save and a load each time, so it turns itself off when it doesn't fit.
func (c *control) runAheadFrame(show bool) {
	start := time.Now()
	c.setOutput(false, true)
	c.eng.Run()
	state := c.saveState(&c.aheadBuf)
	if len(state) == 0 {
		c.runAhead = 0
		return
	}
	// The real frame's sound goes out now, while the frames ahead make none.
	if len(c.r.audio) > 0 {
		c.r.write(ipc.MsgAudio, c.r.audio)
		c.r.audio = c.r.audio[:0]
	}
	c.quiet = true
	for i := 0; i < c.runAhead; i++ {
		c.setOutput(show && i == c.runAhead-1, false)
		c.eng.Run()
	}
	if !c.eng.LoadState(state) {
		c.runAhead = 0
		c.r.event(ipc.Event{Type: "message", Text: "Run-ahead is off: this core can’t go back to a state that quickly."})
	}
	c.quiet = false
	// One slow frame says nothing: another program ran for a moment. Half a second of
	// them says it doesn't fit.
	switch {
	case c.frameTime > 0 && time.Since(start) > c.frameTime*8/10:
		c.aheadSlow++
	case c.aheadSlow > 0:
		c.aheadSlow--
	}
	if c.aheadSlow > 30 {
		c.runAhead, c.aheadSlow = 0, 0
		c.r.event(ipc.Event{Type: "message", Text: "Run-ahead is off: this server can’t keep up with it on this game."})
	}
}
