// Package host is the program that runs one game: it reads commands on one
// stream, writes frames on another and paces the engine at its own frame rate.
// cmd/wittyplay-host wraps it in a process; tests run it directly.
package host

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"time"

	"github.com/wittywilla/wittyplay/engine"
	"github.com/wittywilla/wittyplay/ipc"
)

// Open makes the engine a Load asks for.
type Open func(l ipc.Load, h engine.Handlers) (engine.Engine, error)

const saveRAMEvery = 30 * time.Second

// Run plays until told to quit or until in closes. Cores may keep thread-local
// state, so it locks a thread for itself.
func Run(in io.Reader, out io.Writer, open Open) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	r := &runner{out: out}
	err := r.run(in, open)
	if err != nil && !errors.Is(err, io.EOF) {
		_ = ipc.Write(out, ipc.MsgError, []byte(err.Error()))
	}
	return err
}

type runner struct {
	out    io.Writer
	outMu  sync.Mutex // the frame sender writes too, and messages must not interleave
	writeE error
	frames frameSender

	pads pads

	pointMu sync.Mutex
	point   engine.PointerState

	audio []byte
	video []byte
	stop  bool
	fps   float64 // a new frame rate the game has asked for
	muted bool    // these frames are only to get a core ready, and nobody should see them
}

func (r *runner) write(t byte, parts ...[]byte) {
	r.outMu.Lock()
	defer r.outMu.Unlock()
	if r.writeE == nil {
		r.writeE = ipc.Write(r.out, t, parts...)
	}
}

func (r *runner) writeErr() error {
	r.outMu.Lock()
	defer r.outMu.Unlock()
	return r.writeE
}

// frameSender sends the frames a graphics card draws. They run to several megabytes,
// and a game mustn't wait while one crosses the pipe, or wait at all if the server
// has fallen behind: the newest frame replaces one still waiting to go, the same
// as on the server's side, so no queue can build up and add delay.
type frameSender struct {
	mu      sync.Mutex
	wake    *sync.Cond
	next    []byte // ready to go
	spare   []byte
	waiting bool
	running bool
	closed  bool
}

// offer takes a frame built by fill, which is given a buffer to reuse.
func (f *frameSender) offer(r *runner, fill func(buf []byte) []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return
	}
	f.next = fill(f.next[:0])
	f.waiting = true
	if !f.running {
		f.running = true
		f.wake = sync.NewCond(&f.mu)
		go f.loop(r)
	}
	f.wake.Signal()
}

func (f *frameSender) loop(r *runner) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for {
		for !f.waiting && !f.closed {
			f.wake.Wait()
		}
		if f.closed {
			return
		}
		frame := f.next
		f.next, f.spare = f.spare, nil
		f.waiting = false
		f.mu.Unlock()
		r.write(ipc.MsgVideo, frame)
		f.mu.Lock()
		f.spare = frame
	}
}

func (f *frameSender) stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	if f.wake != nil {
		f.wake.Signal()
	}
}

func (r *runner) event(e ipc.Event) {
	b, _ := json.Marshal(e)
	r.write(ipc.MsgEvent, b)
}

func (r *runner) handlers() engine.Handlers {
	return engine.Handlers{
		Video: func(data []byte, w, h, stride, format int) {
			row := w * engine.BytesPerPixel(format)
			if r.muted || w <= 0 || h <= 0 || row > stride || len(data) < stride*h {
				return
			}
			r.video = ipc.VideoHeader{Width: w, Height: h, Format: format}.Append(r.video[:0])
			for y := 0; y < h; y++ {
				r.video = append(r.video, data[y*stride:y*stride+row]...)
			}
			r.write(ipc.MsgVideo, r.video)
		},
		VideoFlipped: func(data []byte, w, h, stride, format int) {
			row := w * engine.BytesPerPixel(format)
			if r.muted || w <= 0 || h <= 0 || row > stride || len(data) < stride*h {
				return
			}
			r.frames.offer(r, func(buf []byte) []byte {
				buf = ipc.VideoHeader{Width: w, Height: h, Format: format}.Append(buf)
				for y := h - 1; y >= 0; y-- {
					buf = append(buf, data[y*stride:y*stride+row]...)
				}
				return buf
			})
		},
		Dupe: func() {
			if !r.muted {
				r.write(ipc.MsgDupe)
			}
		},
		Audio: func(s []int16) {
			for _, v := range s {
				r.audio = binary.LittleEndian.AppendUint16(r.audio, uint16(v))
			}
		},
		Input: r.pads.poll,
		Pointer: func() engine.PointerState {
			r.pointMu.Lock()
			defer r.pointMu.Unlock()
			return r.point
		},
		Rumble: func(port, effect, strength int) {
			r.event(ipc.Event{Type: "rumble", Port: port, Effect: effect, Strength: strength})
		},
		Message: func(text string) { r.event(ipc.Event{Type: "message", Text: text}) },
		Log: func(level int, text string) {
			if level >= 2 {
				r.event(ipc.Event{Type: "log", Level: level, Text: text})
			}
		},
		Geometry: func(w, h int, aspect float64) {
			r.event(ipc.Event{Type: "geometry", Width: w, Height: h, Aspect: aspect})
		},
		Rotation: func(n int) { r.event(ipc.Event{Type: "rotation", Turns: n}) },
		AVInfo: func(a engine.AVInfo) {
			r.fps = a.FPS
			r.event(ipc.Event{Type: "avinfo", Width: a.BaseWidth, Height: a.BaseHeight, Aspect: a.Aspect, FPS: a.FPS, Rate: a.SampleRate})
		},
		Shutdown: func() { r.event(ipc.Event{Type: "shutdown"}); r.stop = true },
	}
}

type command struct {
	t byte
	p []byte
}

func (r *runner) run(in io.Reader, open Open) error {
	cmds := make(chan command, 64)
	readErr := make(chan error, 1)
	go func() {
		rd := ipc.NewReader(in)
		for {
			t, p, err := rd.Next()
			if err != nil {
				readErr <- err
				close(cmds)
				return
			}
			if t == ipc.CmdInput {
				// Input skips the queue: the newest state is all that matters.
				if port, b, lx, ly, rx, ry, ok := ipc.ParseInput(p); ok && port < engine.MaxPorts {
					r.pads.set(port, engine.PadState{Buttons: b, LX: lx, LY: ly, RX: rx, RY: ry})
				}
				continue
			}
			if t == ipc.CmdPointer {
				if x, y, pressed, ok := ipc.ParsePointer(p); ok {
					r.pointMu.Lock()
					r.point = engine.PointerState{X: x, Y: y, Pressed: pressed}
					r.pointMu.Unlock()
				}
				continue
			}
			cmds <- command{t, append([]byte(nil), p...)}
		}
	}()

	// The first command says what to play. No core is loaded before this, so
	// the server has had time to box this process in.
	first, ok := <-cmds
	if !ok {
		return <-readErr
	}
	if first.t != ipc.CmdLoad {
		return fmt.Errorf("expected a load command, got %d", first.t)
	}
	var load ipc.Load
	if err := json.Unmarshal(first.p, &load); err != nil {
		return err
	}
	h := r.handlers()
	if load.Trace {
		h.Trace = func(text string) { r.event(ipc.Event{Type: "trace", Text: text}) }
	}
	eng, err := open(load, h)
	if err != nil {
		return err
	}
	defer eng.Close()
	av, err := eng.LoadGame(load.ROM)
	if err != nil {
		return err
	}
	if av.FPS <= 0 || av.FPS > 240 {
		av.FPS = 60
	}
	info := eng.SystemInfo()
	renderer := ""
	if hw, ok := eng.(interface{ Renderer() string }); ok {
		renderer = hw.Renderer()
	}
	discs, _ := eng.(engine.Discs)
	disc, discCount := 0, 0
	if discs != nil {
		disc, discCount = discs.Disc()
	}
	ready, _ := json.Marshal(ipc.Ready{Name: info.Name, Version: info.Version, Width: av.BaseWidth, Height: av.BaseHeight, Aspect: av.Aspect, FPS: av.FPS, SampleRate: av.SampleRate, Disc: disc, Discs: discCount, Renderer: renderer})
	r.write(ipc.MsgReady, ready)

	frame := time.Duration(float64(time.Second) / av.FPS)
	next := time.Now()
	// A state can't always be restored before a core has run. Some cores say so, some
	// refuse, and some accept it and then start the game from the beginning on the
	// first frame anyway. So a state that arrives before the first frame waits for a
	// couple of frames, which nobody sees.
	const earlyStateFrames = 2
	var settle int
	var heldState []byte
	framesRun := 0
	paused := true // until the server has a viewer
	var lastSave []byte
	lastSaveAt := time.Now()

	handle := func(c command) (quit bool) {
		switch c.t {
		case ipc.CmdSaveRAM:
			if ram := eng.SaveRAM(); ram != nil {
				copy(ram, c.p)
				lastSave = append(lastSave[:0], ram...)
			}
		case ipc.CmdState:
			switch {
			case framesRun < earlyStateFrames:
				heldState, settle = c.p, earlyStateFrames-framesRun
				r.muted = true
			case !eng.LoadState(c.p):
				r.event(ipc.Event{Type: "statefail"})
			}
		case ipc.CmdSaveState:
			r.write(ipc.MsgState, c.p, eng.SaveState())
		case ipc.CmdReset:
			eng.Reset()
		case ipc.CmdDisc:
			if discs != nil && len(c.p) > 0 {
				if !discs.ChangeDisc(int(c.p[0])) {
					r.event(ipc.Event{Type: "message", Text: "That disc could not be loaded."})
				}
				i, n := discs.Disc()
				r.event(ipc.Event{Type: "disc", Index: i, Count: n})
			}
		case ipc.CmdPause:
			paused = len(c.p) > 0 && c.p[0] != 0
			next = time.Now()
		case ipc.CmdQuit:
			if len(c.p) > 0 && c.p[0] != 0 {
				var id [4]byte
				r.write(ipc.MsgState, id[:], eng.SaveState())
			}
			if ram := eng.SaveRAM(); ram != nil {
				r.write(ipc.MsgSaveRAM, ram)
			}
			return true
		}
		return false
	}

	defer r.frames.stop()
	for !r.stop {
		if err := r.writeErr(); err != nil {
			return err
		}
		if paused {
			c, ok := <-cmds
			if !ok {
				return <-readErr
			}
			if handle(c) {
				return nil
			}
			continue
		}
	drain:
		for {
			select {
			case c, ok := <-cmds:
				if !ok {
					return <-readErr
				}
				if handle(c) {
					return nil
				}
			default:
				break drain
			}
		}
		if paused {
			continue
		}
		eng.Run()
		framesRun++
		r.pads.frameDone()
		if settle > 0 {
			if settle--; settle == 0 {
				if !eng.LoadState(heldState) {
					r.event(ipc.Event{Type: "statefail"})
				}
				heldState = nil
				r.muted = false
				r.audio = r.audio[:0]
			}
		}
		if r.fps > 0 && r.fps <= 240 {
			frame = time.Duration(float64(time.Second) / r.fps)
			r.fps = 0
		}
		if len(r.audio) > 0 {
			r.write(ipc.MsgAudio, r.audio)
			r.audio = r.audio[:0]
		}
		if time.Since(lastSaveAt) > saveRAMEvery {
			lastSaveAt = time.Now()
			if ram := eng.SaveRAM(); ram != nil && !bytes.Equal(ram, lastSave) {
				lastSave = append(lastSave[:0], ram...)
				r.write(ipc.MsgSaveRAM, ram)
			}
		}
		next = next.Add(frame)
		now := time.Now()
		if d := next.Sub(now); d > 0 {
			time.Sleep(d)
		} else if d < -5*frame {
			next = now // too far behind to catch up: the machine is overloaded
		}
	}
	// The game asked to shut down, so nothing should be lost.
	if ram := eng.SaveRAM(); ram != nil {
		r.write(ipc.MsgSaveRAM, ram)
	}
	return r.writeErr()
}

// pads holds the players' input. A press and its release can arrive together, in a
// short tap or a bunched-up packet, and a game only looks once a frame. So a button
// pressed since the last frame reads as held for one frame, however briefly it was.
type pads struct {
	mu      sync.Mutex
	state   [engine.MaxPorts]engine.PadState
	pending [engine.MaxPorts]uint16 // pressed since the last frame looked
	active  [engine.MaxPorts]uint16 // what this frame is keeping pressed
	snap    [engine.MaxPorts]engine.PadState
}

func (p *pads) set(port int, st engine.PadState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending[port] |= st.Buttons &^ p.state[port].Buttons
	p.state[port] = st
}

// poll is what a core sees. It may look several times in one frame and sees the
// same presses each time.
func (p *pads) poll() *[engine.MaxPorts]engine.PadState {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.state {
		p.active[i] |= p.pending[i]
		p.pending[i] = 0
		p.snap[i] = p.state[i]
		p.snap[i].Buttons |= p.active[i]
	}
	return &p.snap
}

func (p *pads) frameDone() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active = [engine.MaxPorts]uint16{}
}
