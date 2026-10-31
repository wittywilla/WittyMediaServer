package host

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wittywilla/wittyplay/engine"
	"github.com/wittywilla/wittyplay/ipc"
	"github.com/wittywilla/wittyplay/testcore"
)

// Leaving a resumed game before its first frame must not save over the state it was
// about to restore.
func TestLeavingBeforeTheFirstFrameKeepsTheState(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- Run(inR, outW, func(_ ipc.Load, h engine.Handlers) (engine.Engine, error) { return testcore.New(h), nil })
		outW.Close()
	}()
	rom := filepath.Join(t.TempDir(), "game")
	if err := os.WriteFile(rom, []byte("a test game"), 0o644); err != nil {
		t.Fatal(err)
	}
	load, _ := json.Marshal(ipc.Load{Core: ipc.TestCore, ROM: rom})
	saved := []byte("the state someone's progress is in")
	go func() {
		_ = ipc.Write(inW, ipc.CmdLoad, load)
		_ = ipc.Write(inW, ipc.CmdState, saved)
		_ = ipc.Write(inW, ipc.CmdQuit, []byte{1})
	}()
	rd := ipc.NewReader(outR)
	for {
		typ, p, err := rd.Next()
		if err != nil {
			t.Fatalf("no state came back: %v", err)
		}
		if typ == ipc.MsgError {
			t.Fatalf("the runner failed: %s", p)
		}
		if typ == ipc.MsgState {
			if !bytes.Equal(p[4:], saved) {
				t.Fatalf("saved %d bytes of something else", len(p)-4)
			}
			break
		}
	}
	go io.Copy(io.Discard, outR)
	inW.Close()
	<-done
}

func TestShortTapReachesTheGame(t *testing.T) {
	var p pads
	const a = 1 << engine.ButtonA
	// Pressed and released between two frames, as a quick tap can be.
	p.set(0, engine.PadState{Buttons: a})
	p.set(0, engine.PadState{})
	if got := p.poll()[0].Buttons; got&a == 0 {
		t.Fatal("a tap between frames was lost")
	}
	if got := p.poll()[0].Buttons; got&a == 0 {
		t.Fatal("a core looking twice in one frame should see the same press")
	}
	p.frameDone()
	if got := p.poll()[0].Buttons; got&a != 0 {
		t.Fatal("the tap should last one frame, not two")
	}
	p.frameDone()

	// A held button stays held, and other players are their own.
	p.set(1, engine.PadState{Buttons: a, LX: 7})
	p.frameDone()
	st := p.poll()
	if st[1].Buttons&a == 0 || st[1].LX != 7 || st[0].Buttons != 0 {
		t.Fatalf("held state: %+v", st[:2])
	}
	p.frameDone()
	p.set(1, engine.PadState{})
	if got := p.poll()[1].Buttons; got != 0 {
		t.Fatal("a released button should read released")
	}
}

// A server that has stopped reading can't hold the game up on a frame from a
// graphics card, and what it gets when it comes back is the newest one.
func TestLargeFramesNeverWaitOnTheServer(t *testing.T) {
	pr, pw := io.Pipe()
	r := &runner{out: pw}
	r.ctl = newControl(r)
	h := r.handlers()
	frame := func(v byte) []byte { return bytes.Repeat([]byte{v, v, v, v}, 4) }
	done := make(chan struct{})
	go func() {
		defer close(done)
		for v := byte(1); v <= 5; v++ {
			h.VideoFlipped(frame(v), 2, 2, 8, engine.FormatXRGB8888)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the game waited for a server that wasn't reading")
	}
	// Whatever was already on its way, then the newest, which always arrives.
	rd := ipc.NewReader(pr)
	for seen := 0; ; seen++ {
		if seen > 2 {
			t.Fatal("frames queued up instead of replacing each other")
		}
		typ, p, err := rd.Next()
		if err != nil || typ != ipc.MsgVideo {
			t.Fatalf("%d %v", typ, err)
		}
		_, data, err := ipc.ParseVideo(p)
		if err != nil {
			t.Fatal(err)
		}
		if data[0] == 5 {
			break
		}
	}
	r.frames.stop()
	// Other messages still go through, in one piece.
	go r.write(ipc.MsgDupe)
	if typ, _, err := rd.Next(); err != nil || typ != ipc.MsgDupe {
		t.Errorf("%d %v", typ, err)
	}
}
