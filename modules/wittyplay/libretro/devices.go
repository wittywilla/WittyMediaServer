package libretro

import (
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/wittywilla/wittyplay/engine"
)

// Kinds of controller a core offers per port, from SET_CONTROLLER_INFO. A game's
// DualShock, say, is one of these, and is what turns its sticks and rumble on.

// Bounds on what's read of a core's list. It's the core's own memory, in a process that
// can only hurt itself, and these are well above any real core.
const (
	maxDevicePorts = engine.MaxPorts
	maxDeviceTypes = 32
)

// readControllerInfo reads retro_controller_info: one {types, count} per port, ended by
// a null, each type being {description, id}.
func readControllerInfo(data unsafe.Pointer) []engine.PortDevices {
	type port struct {
		types unsafe.Pointer
		count uint32
	}
	type kind struct {
		desc *byte
		id   uint32
	}
	if data == nil {
		return nil
	}
	var out []engine.PortDevices
	for n := 0; n < maxDevicePorts; n++ {
		p := (*port)(unsafe.Add(data, uintptr(n)*unsafe.Sizeof(port{})))
		if p.types == nil {
			break
		}
		d := engine.PortDevices{Port: n, Current: deviceJoypad}
		for i := 0; i < int(p.count) && i < maxDeviceTypes; i++ {
			k := (*kind)(unsafe.Add(p.types, uintptr(i)*unsafe.Sizeof(kind{})))
			d.Types = append(d.Types, engine.DeviceType{ID: k.id, Name: cString(k.desc)})
		}
		out = append(out, d)
	}
	return out
}

// ControllerPorts is what the core offers.
func (c *Core) ControllerPorts() []engine.PortDevices { return c.devices }

// SetDevice plugs a kind of controller into a port. The list holds what the core
// offered, and anything else is ignored.
func (c *Core) SetDevice(port int, id uint32) {
	for i := range c.devices {
		if c.devices[i].Port != port {
			continue
		}
		for _, t := range c.devices[i].Types {
			if t.ID == id {
				c.devices[i].Current = id
				purego.SyscallN(c.fnSetCtrl, uintptr(port), uintptr(id))
				return
			}
		}
	}
}

// analogButton is how hard a button is pressed, for cores that ask. Only triggers can
// tell, when the client said; anything else is all or nothing.
func analogButton(st *engine.PadState, id uintptr) int16 {
	var v uint8
	switch id {
	case engine.ButtonL2:
		v = st.L2
	case engine.ButtonR2:
		v = st.R2
	}
	if v != 0 {
		return int16(uint32(v) * 0x7fff / 255)
	}
	if st.Buttons&(1<<id) != 0 {
		return 0x7fff
	}
	return 0
}
