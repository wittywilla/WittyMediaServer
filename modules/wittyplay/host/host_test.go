package host

import (
	"testing"

	"github.com/wittywilla/wittyplay/engine"
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
