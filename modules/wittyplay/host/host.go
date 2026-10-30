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
	writeE error

	mu    sync.Mutex
	input [engine.MaxPorts]engine.PadState
	snap  [engine.MaxPorts]engine.PadState

	audio []byte
	video []byte
	stop  bool
}

func (r *runner) write(t byte, parts ...[]byte) {
	if r.writeE == nil {
		r.writeE = ipc.Write(r.out, t, parts...)
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
			if w <= 0 || h <= 0 || row > stride || len(data) < stride*h {
				return
			}
			r.video = ipc.VideoHeader{Width: w, Height: h, Format: format}.Append(r.video[:0])
			for y := 0; y < h; y++ {
				r.video = append(r.video, data[y*stride:y*stride+row]...)
			}
			r.write(ipc.MsgVideo, r.video)
		},
		Dupe: func() { r.write(ipc.MsgDupe) },
		Audio: func(s []int16) {
			for _, v := range s {
				r.audio = binary.LittleEndian.AppendUint16(r.audio, uint16(v))
			}
		},
		Input: func() *[engine.MaxPorts]engine.PadState {
			r.mu.Lock()
			r.snap = r.input
			r.mu.Unlock()
			return &r.snap
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
					r.mu.Lock()
					r.input[port] = engine.PadState{Buttons: b, LX: lx, LY: ly, RX: rx, RY: ry}
					r.mu.Unlock()
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
	eng, err := open(load, r.handlers())
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
	ready, _ := json.Marshal(ipc.Ready{Name: info.Name, Version: info.Version, Width: av.BaseWidth, Height: av.BaseHeight, Aspect: av.Aspect, FPS: av.FPS, SampleRate: av.SampleRate})
	r.write(ipc.MsgReady, ready)

	frame := time.Duration(float64(time.Second) / av.FPS)
	next := time.Now()
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
			eng.LoadState(c.p)
		case ipc.CmdSaveState:
			r.write(ipc.MsgState, c.p, eng.SaveState())
		case ipc.CmdReset:
			eng.Reset()
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

	for !r.stop {
		if r.writeE != nil {
			return r.writeE
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
	return r.writeE
}
