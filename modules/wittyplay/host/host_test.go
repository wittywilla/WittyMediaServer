package host

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

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
