package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/maphash"
	"image"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittyplay"
	"github.com/wittywilla/wittyplay/engine"
	"github.com/wittywilla/wittyplay/ipc"
	"github.com/wittywilla/wittyplay/stream"
)

// Outcomes, worst first. Warnings say what else was wrong with a game that ran.
const (
	OutcomeOK         = "ok"
	OutcomeLoadFailed = "load-failed"
	OutcomeCrash      = "crash"
	OutcomeHang       = "hang"
	OutcomeBlank      = "blank"
	OutcomeFrozen     = "frozen"
	OutcomeShutdown   = "shut-itself-down"
	OutcomeSkipped    = "skipped"
	OutcomeToolError  = "tool-error"
	OutcomeTooSlow    = "too-slow-to-judge" // try again on a quieter machine
)

// Result is one game on one core.
type Result struct {
	Console     string   `json:"console"`
	Core        string   `json:"core"`
	CoreName    string   `json:"coreName,omitempty"`
	CoreVersion string   `json:"coreVersion,omitempty"`
	File        string   `json:"file,omitempty"`
	Index       int      `json:"index"`
	Outcome     string   `json:"outcome"`
	Reason      string   `json:"reason,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`

	LoadMS       int64   `json:"loadMs"`
	Width        int     `json:"width,omitempty"`
	Height       int     `json:"height,omitempty"`
	Format       int     `json:"format"`
	Formats      []int   `json:"formats,omitempty"`
	Sizes        int     `json:"sizes,omitempty"` // how many picture sizes the game used
	FPS          float64 `json:"fps"`
	TargetFPS    float64 `json:"targetFps"`
	MaxGapMS     int64   `json:"maxGapMs"`
	Frames       int     `json:"frames"`
	Dupes        int     `json:"dupes"`
	Changes      int     `json:"changes"` // frames that differed from the one before
	BlankPercent float64 `json:"blankPercent"`
	MaxColors    int     `json:"maxColors"`

	SampleRate float64 `json:"sampleRate"`
	AudioRatio float64 `json:"audioRatio"` // samples received against the rate promised
	AudioPeak  int     `json:"audioPeak"`
	AudioRMS   float64 `json:"audioRms"`

	StateSize   int     `json:"stateSize"`
	StateLoaded bool    `json:"stateLoaded"`
	StateReplay float64 `json:"stateReplay"`    // frames that matched when replayed from a state, 0 to 1
	InputReacts string  `json:"inputReacts"`    // which button changed the picture; "" if none did
	Resume      string  `json:"resume"`         // ok | failed: a state restored in a new process
	ResumeMatch float64 `json:"resumeMatch"`    // as StateReplay, in the new process
	SaveRAM     int     `json:"saveRam"`        // battery save size
	SaveRAMBack string  `json:"saveRamRestore"` // ok | differs | "" when the game has none
	Discs       int     `json:"discs,omitempty"`
	DiscChange  string  `json:"discChange,omitempty"` // ok | failed: the game kept running on the next disc

	CPUPercent float64  `json:"cpuPercent"` // of one CPU thread
	PeakMB     float64  `json:"peakMb"`
	ExitCode   int      `json:"exitCode"`
	Quit       string   `json:"quit"` // clean | crash | timeout
	QuitMS     int64    `json:"quitMs"`
	Log        []string `json:"log,omitempty"`
	Stderr     string   `json:"stderr,omitempty"`
	Seconds    float64  `json:"seconds"`
	Shot       string   `json:"shot,omitempty"`
}

func (r Result) key() string { return r.Core + "\x00" + r.File }

func (r *Result) warn(w string) { r.Warnings = append(r.Warnings, w) }

var hashSeed = maphash.MakeSeed()

// proc is one running runner and what it has said so far.
type proc struct {
	c  *config
	rn *stream.Runner

	mu         sync.Mutex
	ready      *ipc.Ready
	readyCh    chan error
	readyOnce  sync.Once
	errText    string
	stderr     bytes.Buffer
	frames     int
	playFrames int // frames and time while simply playing, for the frame rate
	playTime   time.Duration
	dupes      int
	changes    int
	blank      int
	sampled    int
	maxColors  int
	lastHash   uint64
	lastFrame  time.Time
	maxGap     time.Duration
	gapFrom    time.Time
	hdr        ipc.VideoHeader
	last       []byte
	best       []byte
	bestHdr    ipc.VideoHeader
	bestColors int
	formats    map[int]bool
	sizes      map[[2]int]bool
	audioBytes int64
	audioPeak  int
	audioSum   float64
	audioMin   int16
	audioMax   int16
	logs       []string
	shutdown   bool
	stateFail  int
	lastDisc   int
	saveRAM    []byte
	autoState  []byte

	capN   int
	capped []uint64
	capCh  chan struct{}
	state  chan []byte

	trace   *os.File
	stdinMu sync.Mutex
	done    chan struct{} // the runner's output has ended
	exited  chan struct{}
	exitErr error
	started time.Time
}

func start(c *config, saveDir, coreID string) (*proc, error) {
	if err := os.MkdirAll(saveDir, 0o700); err != nil {
		return nil, err
	}
	// As a session does: a graphics driver needs more than a software-rendered game.
	var args []string
	memory := int64(c.memoryMB)
	if wittyplay.Traits(coreID).GPU {
		args = []string{"-gpu"}
		memory = max(memory, 8192)
	}
	rn, err := stream.StartRunner(c.runner, args, saveDir, memory, c.log)
	if err != nil {
		return nil, err
	}
	p := &proc{c: c, rn: rn, readyCh: make(chan error, 1), state: make(chan []byte, 1), done: make(chan struct{}), exited: make(chan struct{}),
		formats: map[int]bool{}, sizes: map[[2]int]bool{}, started: time.Now(), audioMin: 32767, audioMax: -32768}
	go func() {
		_, _ = io.Copy(limitedWriter{&p.stderr, &p.mu, 1 << 15}, rn.Stderr)
	}()
	go p.read()
	go func() {
		<-p.done
		p.exitErr = rn.Cmd.Wait()
		rn.Release()
		close(p.exited)
	}()
	return p, nil
}

type limitedWriter struct {
	b   *bytes.Buffer
	mu  *sync.Mutex
	max int
}

func (w limitedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	// Keep the end: cores can be chatty, and a crash report comes last.
	w.b.Write(p)
	if over := w.b.Len() - w.max; over > 0 {
		rest := append([]byte(nil), w.b.Bytes()[over:]...)
		w.b.Reset()
		w.b.Write(rest)
	}
	return len(p), nil
}

func (p *proc) send(t byte, parts ...[]byte) error {
	p.stdinMu.Lock()
	defer p.stdinMu.Unlock()
	return ipc.Write(p.rn.Stdin, t, parts...)
}

func (p *proc) read() {
	defer close(p.done)
	rd := ipc.NewReader(p.rn.Stdout)
	for {
		t, b, err := rd.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) && !errors.Is(err, io.ErrUnexpectedEOF) {
				p.mu.Lock()
				p.errText = "the runner sent something invalid: " + err.Error()
				p.mu.Unlock()
				_ = p.rn.Cmd.Process.Kill()
			}
			p.readyOnce.Do(func() { p.readyCh <- errors.New("exited") })
			return
		}
		switch t {
		case ipc.MsgReady:
			var r ipc.Ready
			if err := json.Unmarshal(b, &r); err != nil {
				p.readyOnce.Do(func() { p.readyCh <- err })
				continue
			}
			p.mu.Lock()
			p.ready = &r
			p.mu.Unlock()
			p.readyOnce.Do(func() { p.readyCh <- nil })
		case ipc.MsgVideo:
			hdr, data, err := ipc.ParseVideo(b)
			if err != nil {
				p.mu.Lock()
				p.errText = err.Error()
				p.mu.Unlock()
				continue
			}
			p.video(hdr, data)
		case ipc.MsgDupe:
			p.mu.Lock()
			p.dupes++
			p.tickLocked(p.lastHash)
			p.mu.Unlock()
		case ipc.MsgAudio:
			p.audio(b)
		case ipc.MsgState:
			if len(b) < 4 {
				continue
			}
			if binary.LittleEndian.Uint32(b) == 0 {
				p.mu.Lock()
				p.autoState = append([]byte(nil), b[4:]...)
				p.mu.Unlock()
				continue
			}
			select {
			case p.state <- append([]byte(nil), b[4:]...):
			default:
			}
		case ipc.MsgSaveRAM:
			p.mu.Lock()
			p.saveRAM = append(p.saveRAM[:0], b...)
			p.mu.Unlock()
		case ipc.MsgEvent:
			var e ipc.Event
			if json.Unmarshal(b, &e) != nil {
				continue
			}
			p.mu.Lock()
			switch e.Type {
			case "log", "message":
				if len(p.logs) < 12 {
					p.logs = append(p.logs, truncate(e.Text, 240))
				}
			case "trace":
				if p.trace != nil {
					fmt.Fprintln(p.trace, e.Text)
				}
			case "shutdown":
				p.shutdown = true
			case "statefail":
				p.stateFail++
			case "disc":
				p.lastDisc = e.Index
			}
			p.mu.Unlock()
		case ipc.MsgError:
			p.mu.Lock()
			p.errText = truncate(string(b), 400)
			p.mu.Unlock()
		}
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// video looks at one frame. It runs for every frame of every game, so colours
// are only counted on a sample of the pixels of some frames.
func (p *proc) video(hdr ipc.VideoHeader, data []byte) {
	h := maphash.Bytes(hashSeed, data)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.formats[hdr.Format] = true
	p.sizes[[2]int{hdr.Width, hdr.Height}] = true
	p.hdr = hdr
	p.last = append(p.last[:0], data...)
	if h != p.lastHash {
		p.changes++
	}
	if p.frames%10 == 0 {
		colors := countColors(data, engine.BytesPerPixel(hdr.Format))
		p.sampled++
		if colors <= 1 {
			p.blank++
		}
		if colors > p.maxColors {
			p.maxColors = colors
		}
		if colors > p.bestColors {
			p.bestColors, p.bestHdr = colors, hdr
			p.best = append(p.best[:0], data...)
		}
	}
	p.tickLocked(h)
}

// tickLocked counts a frame, new or repeated.
func (p *proc) tickLocked(h uint64) {
	now := time.Now()
	if !p.gapFrom.IsZero() {
		if g := now.Sub(p.gapFrom); g > p.maxGap {
			p.maxGap = g
		}
	}
	p.gapFrom = now
	p.lastFrame = now
	p.frames++
	p.lastHash = h
	if p.capN > 0 {
		p.capped = append(p.capped, h)
		if len(p.capped) >= p.capN {
			p.capN = 0
			close(p.capCh)
		}
	}
}

func countColors(data []byte, bpp int) int {
	const maxCount = 256
	px := len(data) / bpp
	step := max(1, px/3000)
	seen := make(map[uint32]struct{}, 64)
	for i := 0; i < px; i += step {
		var v uint32
		if bpp == 4 {
			v = binary.LittleEndian.Uint32(data[i*4:]) & 0xffffff
		} else {
			v = uint32(binary.LittleEndian.Uint16(data[i*2:]))
		}
		seen[v] = struct{}{}
		if len(seen) >= maxCount {
			break
		}
	}
	return len(seen)
}

func (p *proc) audio(b []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.audioBytes += int64(len(b))
	for i := 0; i+1 < len(b); i += 2 {
		v := int16(binary.LittleEndian.Uint16(b[i:]))
		if v < p.audioMin {
			p.audioMin = v
		}
		if v > p.audioMax {
			p.audioMax = v
		}
		p.audioSum += float64(v) * float64(v)
	}
}

// capture returns the next n frames' hashes, stopping early when the game does.
func (p *proc) capture(n int, timeout time.Duration) []uint64 {
	ch := make(chan struct{})
	p.mu.Lock()
	p.capN, p.capped, p.capCh = n, nil, ch
	p.mu.Unlock()
	select {
	case <-ch:
	case <-p.done:
	case <-time.After(timeout):
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.capN = 0
	return p.capped
}

func (p *proc) pause(on bool) error {
	v := byte(0)
	if on {
		v = 1
	}
	return p.send(ipc.CmdPause, []byte{v})
}

// quiesce pauses and waits until the runner has finished its frame. Commands
// are handled in order, so the answer to a state request is the proof.
func (p *proc) quiesce() ([]byte, error) {
	if err := p.pause(true); err != nil {
		return nil, err
	}
	return p.requestState()
}

func (p *proc) requestState() ([]byte, error) {
	select { // an answer that arrived after its request timed out
	case <-p.state:
	default:
	}
	var id [4]byte
	binary.LittleEndian.PutUint32(id[:], 1)
	if err := p.send(ipc.CmdSaveState, id[:]); err != nil {
		return nil, err
	}
	select {
	case b := <-p.state:
		return b, nil
	case <-p.done:
		return nil, errors.New("the game ended")
	case <-time.After(20 * time.Second):
		return nil, errors.New("no answer in 20 seconds")
	}
}

func (p *proc) press(buttons uint16) error {
	var m [ipc.InputSize]byte
	binary.LittleEndian.PutUint16(m[1:], buttons)
	return p.send(ipc.CmdInput, m[:])
}

// The buttons pressed to get past title screens and into the game.
var script = []uint16{
	1 << engine.ButtonStart, 1 << engine.ButtonA, 1 << engine.ButtonStart, 1 << engine.ButtonB,
	1 << engine.ButtonA, 1 << engine.ButtonRight, 1 << engine.ButtonStart, 1 << engine.ButtonDown,
	1 << engine.ButtonA, 1 << engine.ButtonUp, 1 << engine.ButtonB, 1 << engine.ButtonLeft,
}

// play runs the game for a number of frames, pressing buttons, and stops early if
// it dies or hangs. Counting frames rather than seconds means a busy machine sees
// the same stretch of the game as an idle one, only more slowly; the wall-clock
// limit is there for a game too slow to ever get there.
func (p *proc) play(frames int, limit time.Duration, step *int) (hung bool) {
	const hold, gap = 12, 66 // in frames
	deadline := time.Now().Add(limit)
	pressed := false
	p.mu.Lock()
	p.lastFrame = time.Now()
	p.gapFrom = time.Time{}
	first := p.frames
	p.mu.Unlock()
	nextPress := first
	began := time.Now()
	defer func() {
		p.mu.Lock()
		p.playFrames += p.frames - first
		p.playTime += time.Since(began)
		p.mu.Unlock()
	}()
	for time.Now().Before(deadline) {
		select {
		case <-p.done:
			return false
		case <-time.After(5 * time.Millisecond):
		}
		p.mu.Lock()
		now, quiet := p.frames, time.Since(p.lastFrame)
		p.mu.Unlock()
		if now-first >= frames {
			break
		}
		if now >= nextPress {
			if pressed {
				_ = p.press(0)
				nextPress = now + gap
			} else {
				_ = p.press(script[*step%len(script)])
				*step++
				nextPress = now + hold
			}
			pressed = !pressed
		}
		if quiet > p.c.hang {
			return true
		}
	}
	if pressed {
		_ = p.press(0)
	}
	return false
}

func match(a, b []uint64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := min(len(a), len(b))
	same := 0
	for i := 0; i < n; i++ {
		if a[i] == b[i] {
			same++
		}
	}
	return float64(same) / float64(max(len(a), len(b)))
}

// bestMatch is match with one recording slid up to n frames against the other.
func bestMatch(a, b []uint64, n int) float64 {
	best := match(a, b)
	for i := 1; i <= n; i++ {
		if i < len(a) {
			best = max(best, match(a[i:], b))
		}
		if i < len(b) {
			best = max(best, match(a, b[i:]))
		}
	}
	return best
}

const replayFrames = 30

// stateCheck proves a state restores: it saves, records what follows, goes back
// and sees whether the same pictures come again. Then it replays with buttons
// held, which shows whether input reaches the game. It returns the state and the
// pictures that followed it, for the restart check.
func (p *proc) stateCheck(r *Result, frameTime time.Duration) (state []byte, after []uint64, err error) {
	timeout := 10*time.Second + frameTime*replayFrames*4
	_ = p.press(0)
	// A frame or two for the release to reach the game, or the replay starts with
	// the buttons in a different state from the recording.
	p.capture(2, timeout)
	state, err = p.quiesce()
	if err != nil {
		return nil, nil, err
	}
	r.StateSize = len(state)
	if len(state) == 0 {
		_ = p.pause(false)
		return nil, nil, nil
	}
	_ = p.pause(false)
	after = p.capture(replayFrames, timeout)
	if _, err := p.quiesce(); err != nil {
		return state, after, err
	}
	failed := p.failedLoads()
	if err := p.send(ipc.CmdState, state); err != nil {
		return state, after, err
	}
	_ = p.pause(false)
	again := p.capture(replayFrames, timeout)
	r.StateLoaded = p.failedLoads() == failed && len(again) > 0
	r.StateReplay = match(after, again)

	// The same stretch with a button held. Anything different on screen came from it.
	for _, try := range []struct {
		name string
		b    uint16
	}{
		{"start", 1 << engine.ButtonStart}, {"a", 1 << engine.ButtonA}, {"right", 1 << engine.ButtonRight},
		{"b", 1 << engine.ButtonB}, {"down", 1 << engine.ButtonDown}, {"up", 1 << engine.ButtonUp},
	} {
		if r.StateReplay < 0.9 {
			break // the game doesn't replay exactly, so a difference would prove nothing
		}
		if _, err := p.quiesce(); err != nil {
			return state, after, err
		}
		_ = p.press(try.b)
		if err := p.send(ipc.CmdState, state); err != nil {
			return state, after, err
		}
		_ = p.pause(false)
		held := p.capture(replayFrames, timeout)
		if len(held) > 0 && match(after, held) < 0.97 {
			r.InputReacts = try.name
			break
		}
	}
	if _, err := p.quiesce(); err != nil {
		return state, after, err
	}
	_ = p.press(0)
	_ = p.send(ipc.CmdState, state)
	_ = p.pause(false)
	return state, after, nil
}

func (p *proc) discRefused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, l := range p.logs {
		if l == "That disc could not be loaded." {
			return true
		}
	}
	return false
}

func (p *proc) failedLoads() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stateFail
}

// finish waits for the process, up to a limit, and fills in how it ended.
func (p *proc) waitExit(d time.Duration) bool {
	select {
	case <-p.exited:
		return true
	case <-time.After(d):
		return false
	}
}

func (p *proc) kill() {
	_ = p.rn.Cmd.Process.Kill()
	<-p.exited
}

// crashReason turns how a runner ended into a sentence.
func (p *proc) crashReason() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.errText != "" {
		return p.errText
	}
	text := p.stderr.String()
	switch {
	case strings.Contains(text, "Exception 0xc0000005"), strings.Contains(text, "SIGSEGV"), strings.Contains(text, "signal arrived during"):
		return "the core crashed: it touched memory it does not own"
	case strings.Contains(text, "Exception 0xc00000fd"), strings.Contains(text, "stack overflow"):
		return "the core crashed: it ran out of stack"
	case strings.Contains(text, "Exception 0xc000001d"), strings.Contains(text, "SIGILL"):
		return "the core crashed: an instruction this processor doesn't have"
	case strings.Contains(text, "out of memory"):
		return "ran out of memory"
	case strings.Contains(text, "panic:"):
		return "the runner panicked: " + firstLine(text[strings.Index(text, "panic:"):])
	}
	if p.exitErr != nil {
		return "the runner ended: " + p.exitErr.Error()
	}
	return "the runner ended without saying why"
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return truncate(strings.TrimSpace(s), 200)
}

// crashText is the part of what a runner said that explains its end: from the
// crash report on, when there is one.
func crashText(s string) string {
	for _, mark := range []string{"Exception 0x", "SIGSEGV", "SIGILL", "SIGABRT", "fatal error:", "panic:"} {
		if i := strings.Index(s, mark); i >= 0 {
			return truncate(s[i:], 1500)
		}
	}
	if len(s) > 1500 {
		return s[len(s)-1500:]
	}
	return s
}

func checkGame(c *config, j job) (r Result) {
	r = Result{Console: j.console.ID, Core: j.core.id, File: j.game, Index: j.index, Outcome: OutcomeOK}
	began := time.Now()
	defer func() { r.Seconds = time.Since(began).Seconds() }()

	work, err := os.MkdirTemp(c.out, "run-")
	if err != nil {
		r.Outcome, r.Reason = OutcomeToolError, err.Error()
		return r
	}
	defer removeAll(work)
	rom, cleanup, err := unpack(c, j.game, j.console, filepath.Join(work, "rom"))
	if err != nil {
		r.Outcome, r.Reason = OutcomeSkipped, err.Error()
		return r
	}
	defer cleanup()

	saveDir := filepath.Join(work, "save")
	p, err := start(c, saveDir, j.core.id)
	if err != nil {
		r.Outcome, r.Reason = OutcomeToolError, err.Error()
		return r
	}
	if c.trace {
		p.trace, _ = os.Create(filepath.Join(c.out, fmt.Sprintf("trace-%s-%d.log", j.core.id, j.index)))
		defer p.trace.Close()
	}
	load, _ := json.Marshal(ipc.Load{Core: j.core.path, ROM: rom, System: c.systemDir, Save: saveDir, Options: gameOptions(c, j), Trace: c.trace})
	t0 := time.Now()
	if err := p.send(ipc.CmdLoad, load); err != nil {
		r.Outcome, r.Reason = OutcomeToolError, err.Error()
		p.kill()
		return r
	}
	select {
	case err := <-p.readyCh:
		if err != nil {
			p.waitExit(5 * time.Second)
			r.Outcome = OutcomeLoadFailed
			r.Reason = p.crashReason()
			if strings.HasPrefix(r.Reason, "the core crashed") {
				r.Outcome = OutcomeCrash
				r.Reason = "while loading: " + r.Reason
			}
			p.fill(&r)
			return r
		}
	case <-time.After(c.loadTimeout):
		p.kill()
		r.Outcome, r.Reason = OutcomeHang, "did not finish loading in "+c.loadTimeout.String()
		p.fill(&r)
		return r
	}
	r.LoadMS = time.Since(t0).Milliseconds()
	p.mu.Lock()
	info := *p.ready
	p.mu.Unlock()
	r.CoreName, r.CoreVersion, r.TargetFPS, r.SampleRate = info.Name, info.Version, info.FPS, info.SampleRate
	frameTime := time.Duration(float64(time.Second) / info.FPS)

	frames := int(c.seconds * info.FPS)
	limit := time.Duration(c.seconds*4) * time.Second
	step := 0
	_ = p.pause(false)
	hung := p.play(frames*6/10, limit, &step)
	r.Discs = info.Discs
	if info.Discs > 1 && !hung && !p.ended() {
		// To the last disc and back, and the game must keep running.
		r.DiscChange = "ok"
		for _, d := range []int{info.Discs - 1, info.Disc} {
			_ = p.send(ipc.CmdDisc, []byte{byte(d)})
			if len(p.capture(30, 15*time.Second)) == 0 || p.discRefused() {
				r.DiscChange = "failed"
				r.warn("disc-change-failed")
				break
			}
		}
	}
	var state []byte
	var after []uint64
	var stateErr error
	if !hung && !p.ended() && !c.quick {
		state, after, stateErr = p.stateCheck(&r, frameTime)
		p.tracePicture(j, "after-state")
		if stateErr != nil && !p.ended() {
			r.warn("state-failed")
			r.Log = append(r.Log, "state check: "+stateErr.Error())
			hung = true
		}
	}
	if !hung && !p.ended() {
		hung = p.play(frames*4/10, limit, &step)
	}

	if c.shots {
		r.Shot = p.writeShot(c, j)
	}

	// Leave the way the server does, saving on the way out.
	switch {
	case hung:
		p.mu.Lock()
		frames := p.frames
		p.mu.Unlock()
		peak := peakMB(p.rn.Cmd.Process.Pid)
		p.kill()
		r.Outcome, r.Reason = OutcomeHang, fmt.Sprintf("no frame for %s after %d frames", c.hang, frames)
		r.PeakMB = peak
		r.Quit = "killed"
	case p.ended():
		p.waitExit(5 * time.Second)
		p.mu.Lock()
		down := p.shutdown
		p.mu.Unlock()
		if down {
			r.Outcome, r.Reason = OutcomeShutdown, "the game shut itself down"
		} else {
			r.Outcome, r.Reason = OutcomeCrash, p.crashReason()
		}
		r.Quit = "already gone"
	default:
		r.PeakMB = peakMB(p.rn.Cmd.Process.Pid)
		if err := p.send(ipc.CmdQuit, []byte{1}); err != nil {
			r.warn("quit-send-failed")
		}
		quitAt := time.Now()
		defer func() { r.QuitMS = time.Since(quitAt).Milliseconds() }()
		if !p.waitExit(p.c.hang) {
			p.kill()
			r.Quit = "timeout"
			r.warn("quit-hung")
		} else if p.exitErr != nil {
			r.Quit = "crash"
			r.warn("quit-crash")
			r.Log = append(r.Log, "quit: "+p.crashReason())
		} else {
			r.Quit = "clean"
		}
	}
	p.fill(&r)

	if r.Outcome == OutcomeOK {
		switch {
		case r.Frames < frames/2:
			// A game that has barely started hasn't had the chance to draw anything.
			r.Outcome, r.Reason = OutcomeTooSlow, fmt.Sprintf("only %d of %d frames ran in the time allowed", r.Frames, frames)
		case r.Frames == 0:
			r.Outcome, r.Reason = OutcomeBlank, "no frames at all"
		case r.MaxColors <= 1:
			r.Outcome, r.Reason = OutcomeBlank, "every picture was a single colour"
		case r.Changes <= 1:
			r.Outcome, r.Reason = OutcomeFrozen, "the picture never changed"
		}
	}
	if r.Outcome == OutcomeOK || r.Outcome == OutcomeBlank || r.Outcome == OutcomeFrozen {
		classify(&r, state, after, p)
		if len(state) > 0 && r.Quit == "clean" {
			resumeCheck(c, j, rom, saveDir, state, after, p.savedRAM(), &r, frameTime)
		}
	}
	return r
}

func (p *proc) ended() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (p *proc) savedRAM() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.saveRAM...)
}

// fill copies what the process said into the result.
func (p *proc) fill(r *Result) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r.Frames, r.Dupes, r.Changes = p.frames, p.dupes, p.changes
	r.MaxColors = p.maxColors
	r.MaxGapMS = p.maxGap.Milliseconds()
	r.Width, r.Height, r.Format = p.hdr.Width, p.hdr.Height, p.hdr.Format
	r.Sizes = len(p.sizes)
	for f := range p.formats {
		r.Formats = append(r.Formats, f)
	}
	if p.sampled > 0 {
		r.BlankPercent = 100 * float64(p.blank) / float64(p.sampled)
	}
	if p.playTime > 0 {
		r.FPS = float64(p.playFrames) / p.playTime.Seconds()
	}
	if r.SampleRate > 0 && p.frames > 0 && r.TargetFPS > 0 {
		// Sound per frame, so a slow machine doesn't look like a sound problem.
		r.AudioRatio = float64(p.audioBytes) / 4 / (r.SampleRate * float64(p.frames) / r.TargetFPS)
	}
	if p.audioBytes > 0 {
		r.AudioPeak = max(int(p.audioMax), -int(p.audioMin))
		r.AudioRMS = rms(p.audioSum, p.audioBytes/2)
	}
	r.SaveRAM = len(p.saveRAM)
	r.Log = append(r.Log, p.logs...)
	if r.Outcome != OutcomeOK {
		r.Stderr = crashText(p.stderr.String())
	}
	if st := p.rn.Cmd.ProcessState; st != nil {
		r.ExitCode = st.ExitCode()
		if total := time.Since(p.started).Seconds(); total > 0 {
			r.CPUPercent = 100 * (st.UserTime() + st.SystemTime()).Seconds() / total
		}
	}
}

// classify adds the warnings: the game ran, but something about it looks wrong.
func classify(r *Result, state []byte, after []uint64, p *proc) {
	if r.Frames > 0 && r.TargetFPS > 0 && r.FPS < r.TargetFPS*0.9 {
		r.warn("slow")
	}
	switch {
	case r.AudioRatio == 0:
		r.warn("no-audio")
	case r.AudioPeak < 64:
		r.warn("silent")
	case r.AudioRatio < 0.8 || r.AudioRatio > 1.25:
		r.warn("audio-rate")
	}
	if p.c.quick {
		return
	}
	switch {
	case r.StateSize == 0:
		r.warn("no-states")
	case !r.StateLoaded:
		r.warn("state-load-failed")
	case r.StateReplay < 0.9:
		r.warn("state-inexact")
	}
	if r.StateReplay >= 0.9 && r.InputReacts == "" {
		r.warn("no-input-effect")
	}
}

// gameOptions are what the server starts this core with, plus any from -options.
func gameOptions(c *config, j job) map[string]string {
	out := wittyplay.StartOptions(j.console.ID, j.core.id, "", c.systemDir)
	if out == nil {
		out = map[string]string{}
	}
	for k, v := range c.options {
		out[k] = v
	}
	return out
}

// resumeCheck starts a new process and restores the way a session does: battery
// save, then state, both before the first frame.
func resumeCheck(c *config, j job, rom, saveDir string, state []byte, after []uint64, ram []byte, r *Result, frameTime time.Duration) {
	r.Resume = "failed"
	p, err := start(c, saveDir, j.core.id)
	if err != nil {
		return
	}
	load, _ := json.Marshal(ipc.Load{Core: j.core.path, ROM: rom, System: c.systemDir, Save: saveDir, Options: gameOptions(c, j), Trace: c.trace})
	if err := p.send(ipc.CmdLoad, load); err != nil {
		p.kill()
		return
	}
	select {
	case err := <-p.readyCh:
		if err != nil {
			p.waitExit(5 * time.Second)
			r.Log = append(r.Log, "resume: "+p.crashReason())
			r.warn("resume-failed")
			return
		}
	case <-time.After(c.loadTimeout):
		p.kill()
		r.warn("resume-failed")
		return
	}
	if len(ram) > 0 {
		_ = p.send(ipc.CmdSaveRAM, ram)
	}
	_ = p.send(ipc.CmdState, state)
	_ = p.pause(false)
	again := p.capture(replayFrames, 10*time.Second+frameTime*replayFrames*4)
	// A new process may start a frame or two either side of where the state was
	// taken, so the best line-up of the two recordings is the fair one.
	r.ResumeMatch = bestMatch(after, again, 5)
	stateOK := p.failedLoads() == 0
	p.tracePicture(j, "after-resume")
	more := p.capture(60, 10*time.Second)
	gone := p.ended()
	if !gone {
		_ = p.send(ipc.CmdQuit, []byte{0})
		if !p.waitExit(15 * time.Second) {
			p.kill()
		}
	} else {
		p.waitExit(5 * time.Second)
	}
	switch {
	case gone:
		r.warn("resume-crash")
		r.Log = append(r.Log, "resume: "+p.crashReason())
		return
	case !stateOK:
		r.warn("resume-state-refused")
		return
	case len(again) == 0 || len(more) == 0:
		r.warn("resume-no-frames")
		return
	}
	r.Resume = "ok"
	if r.StateReplay >= 0.9 && r.ResumeMatch < 0.7 {
		r.warn("resume-inexact")
	}
	if len(ram) > 0 {
		back := p.savedRAM()
		r.SaveRAMBack = "ok"
		if !bytes.Equal(back, ram) {
			r.SaveRAMBack = "differs"
		}
		if !bytes.Equal(back[:min(len(back), 16)], ram[:min(len(ram), 16)]) && len(back) != len(ram) {
			r.warn("save-ram-size-changed")
		}
	}
}

// writeShot saves the last picture, or the most colourful one when the last is
// blank, as a file in the report folder.
func (p *proc) writeShot(c *config, j job) string {
	p.mu.Lock()
	hdr, data := p.hdr, append([]byte(nil), p.last...)
	if len(data) == 0 || countColors(data, engine.BytesPerPixel(hdr.Format)) <= 1 {
		hdr, data = p.bestHdr, append([]byte(nil), p.best...)
	}
	p.mu.Unlock()
	if len(data) == 0 {
		return ""
	}
	dir := filepath.Join(c.out, "shots", j.core.id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	path := filepath.Join(dir, strconv.Itoa(j.index)+".png")
	if err := writePNG(path, hdr, data); err != nil {
		return ""
	}
	return path
}

// tracePicture saves the current picture when tracing, so a state that restored
// the wrong thing can be seen.
func (p *proc) tracePicture(j job, name string) {
	if !p.c.trace {
		return
	}
	p.mu.Lock()
	hdr, data := p.hdr, append([]byte(nil), p.last...)
	p.mu.Unlock()
	if len(data) > 0 {
		_ = writePNG(filepath.Join(p.c.out, fmt.Sprintf("trace-%s-%d-%s.png", j.core.id, j.index, name)), hdr, data)
	}
}

func toImage(hdr ipc.VideoHeader, data []byte) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, hdr.Width, hdr.Height))
	bpp := engine.BytesPerPixel(hdr.Format)
	for y := 0; y < hdr.Height; y++ {
		row := data[y*hdr.Width*bpp:]
		out := img.Pix[y*img.Stride:]
		for x := 0; x < hdr.Width; x++ {
			var r, g, b byte
			switch hdr.Format {
			case engine.FormatXRGB8888:
				v := binary.LittleEndian.Uint32(row[x*4:])
				r, g, b = byte(v>>16), byte(v>>8), byte(v)
			case engine.FormatRGB565:
				v := binary.LittleEndian.Uint16(row[x*2:])
				r, g, b = byte(v>>11)<<3, byte(v>>5)<<2, byte(v)<<3
			default:
				v := binary.LittleEndian.Uint16(row[x*2:])
				r, g, b = byte(v>>10)<<3, byte(v>>5)<<3, byte(v)<<3
			}
			out[x*4], out[x*4+1], out[x*4+2], out[x*4+3] = r, g, b, 255
		}
	}
	return img
}
