package host

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/wittywilla/wittyplay/engine"
	"github.com/wittywilla/wittyplay/ipc"
)

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
