package host

import (
	"testing"

	"github.com/wittywilla/wittyplay/engine"
)

type deviceEngine struct {
	engine.Engine
	ports []engine.PortDevices
	set   map[int]uint32
}

func (d *deviceEngine) ControllerPorts() []engine.PortDevices { return d.ports }
func (d *deviceEngine) SetDevice(port int, id uint32)         { d.set[port] = id }

func TestPreferredDeviceIsPluggedIn(t *testing.T) {
	d := &deviceEngine{set: map[int]uint32{}, ports: []engine.PortDevices{
		{Port: 0, Current: 1, Types: []engine.DeviceType{{ID: 1, Name: "Standard"}, {ID: 261, Name: "DualShock"}, {ID: 517, Name: "Analog Joystick"}}},
		{Port: 1, Current: 1, Types: []engine.DeviceType{{ID: 1, Name: "Standard"}}},
	}}
	out := plugInDevices(d, []string{"dualshock", "analog"})
	if len(out) != 2 || out[0].Current != 261 || d.set[0] != 261 {
		t.Fatalf("%+v %v", out, d.set)
	}
	if _, touched := d.set[1]; touched || out[1].Current != 1 {
		t.Errorf("a port with no such device stays as it is: %+v %v", out[1], d.set)
	}
	if out := plugInDevices(fakeNoDevices{}, []string{"dualshock"}); out != nil {
		t.Errorf("%+v", out)
	}
}

type fakeNoDevices struct{ engine.Engine }

func TestRumbleOnlySaysWhatChanged(t *testing.T) {
	var sent int
	r := &runner{out: writerFunc(func(b []byte) (int, error) { sent++; return len(b), nil })}
	r.ctl = newControl(r)
	h := r.handlers()
	for i := 0; i < 5; i++ {
		h.Rumble(0, 0, 0xffff)
	}
	if sent != 1 {
		t.Errorf("sent %d for one change", sent)
	}
	h.Rumble(0, 1, 0xffff) // the other motor
	h.Rumble(1, 0, 0xffff) // another player
	h.Rumble(0, 0, 0)      // stopped
	if sent != 4 {
		t.Errorf("sent %d, want 4", sent)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(b []byte) (int, error) { return f(b) }
