package libretro

import (
	"testing"
	"unsafe"

	"github.com/wittywilla/wittyplay/engine"
)

func TestReadControllerInfo(t *testing.T) {
	type kind struct {
		desc *byte
		id   uint32
	}
	type port struct {
		types *kind
		count uint32
	}
	name := func(s string) *byte { return &append([]byte(s), 0)[0] }
	first := []kind{{name("Standard"), deviceJoypad}, {name("DualShock"), 261}}
	second := []kind{{name("Standard"), deviceJoypad}}
	ports := []port{{&first[0], 2}, {&second[0], 1}, {nil, 0}}
	got := readControllerInfo(unsafe.Pointer(&ports[0]))
	if len(got) != 2 || len(got[0].Types) != 2 || got[0].Types[1].Name != "DualShock" || got[0].Types[1].ID != 261 || got[1].Port != 1 {
		t.Fatalf("%+v", got)
	}
	if readControllerInfo(nil) != nil {
		t.Error("nothing should read as nothing")
	}
}

func TestAnalogButton(t *testing.T) {
	st := &engine.PadState{Buttons: 1<<engine.ButtonL2 | 1<<engine.ButtonA, L2: 128}
	if v := analogButton(st, engine.ButtonL2); v < 16000 || v > 17000 {
		t.Errorf("half a trigger: %d", v)
	}
	if v := analogButton(st, engine.ButtonA); v != 0x7fff {
		t.Errorf("a pressed button: %d", v)
	}
	if v := analogButton(st, engine.ButtonB); v != 0 {
		t.Errorf("a button let go: %d", v)
	}
	// A client that can't tell how far: pressed is all the way.
	if v := analogButton(&engine.PadState{Buttons: 1 << engine.ButtonR2}, engine.ButtonR2); v != 0x7fff {
		t.Errorf("%d", v)
	}
	if v := analogButton(&engine.PadState{R2: 255}, engine.ButtonR2); v != 0x7fff {
		t.Errorf("a full trigger: %d", v)
	}
}
