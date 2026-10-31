// Package testcore is a tiny game written in Go: a square you move around a
// coloured field, a tone while a button is held, and a counter kept in save
// memory. It needs no third-party core or ROM, so the whole path from library
// to browser can be tested on any machine, including CI.
package testcore

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"

	"github.com/wittywilla/wittyplay/engine"
)

const (
	Width  = 256
	Height = 224
	Rate   = 44100
	FPS    = 60
)

// Core implements engine.Engine.
type Core struct {
	h     engine.Handlers
	seed  uint32
	frame []byte
	audio []int16
	saved [16]byte // the save memory: bytes 0-3 count presses of Start

	// The machine state, everything a save state holds.
	x, y     int16
	tick     uint32
	phase    uint32
	lastPads uint16
}

var _ engine.Engine = (*Core)(nil)

// New makes a core reporting to h.
func New(h engine.Handlers) *Core { return &Core{h: h} }

func (c *Core) SystemInfo() engine.SystemInfo {
	return engine.SystemInfo{Name: "WittyPlay test core", Version: "1", Extensions: []string{"wittytest"}}
}

// LoadGame reads the "ROM". Any non-empty file works: its checksum picks the
// colours, so two test games can be told apart.
func (c *Core) LoadGame(path string) (engine.AVInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return engine.AVInfo{}, err
	}
	if len(b) == 0 {
		return engine.AVInfo{}, errors.New("the game file is empty")
	}
	c.seed = crc32.ChecksumIEEE(b)
	c.frame = make([]byte, Width*Height*4)
	c.audio = make([]int16, Rate/FPS*2)
	c.x, c.y = Width/2, Height/2
	return engine.AVInfo{BaseWidth: Width, BaseHeight: Height, MaxWidth: Width, MaxHeight: Height, Aspect: 4.0 / 3, FPS: FPS, SampleRate: Rate}, nil
}

func (c *Core) Run() {
	var pad engine.PadState
	if c.h.Input != nil {
		if in := c.h.Input(); in != nil {
			pad = in[0]
		}
	}
	const speed = 2
	if pad.Buttons&(1<<engine.ButtonLeft) != 0 && c.x >= speed {
		c.x -= speed
	}
	if pad.Buttons&(1<<engine.ButtonRight) != 0 && c.x < Width-16-speed {
		c.x += speed
	}
	if pad.Buttons&(1<<engine.ButtonUp) != 0 && c.y >= speed {
		c.y -= speed
	}
	if pad.Buttons&(1<<engine.ButtonDown) != 0 && c.y < Height-16-speed {
		c.y += speed
	}
	if pressed := pad.Buttons &^ c.lastPads; pressed&(1<<engine.ButtonStart) != 0 {
		binary.LittleEndian.PutUint32(c.saved[:], binary.LittleEndian.Uint32(c.saved[:])+1)
	}
	if pad.Buttons&(1<<engine.ButtonA) != 0 && c.h.Rumble != nil && c.lastPads&(1<<engine.ButtonA) == 0 {
		c.h.Rumble(0, 0, 0xffff)
	}
	c.lastPads = pad.Buttons
	c.tick++
	c.draw(pad)
	if c.h.Video != nil {
		c.h.Video(c.frame, Width, Height, Width*4, engine.FormatXRGB8888)
	}
	c.tone(pad)
	if c.h.Audio != nil {
		c.h.Audio(c.audio)
	}
}

func (c *Core) draw(pad engine.PadState) {
	r0, g0, b0 := byte(c.seed), byte(c.seed>>8), byte(c.seed>>16)
	f := c.frame
	for y := 0; y < Height; y++ {
		row := f[y*Width*4:]
		band := byte((y + int(c.tick)) / 8)
		for x := 0; x < Width; x++ {
			p := row[x*4 : x*4+4]
			p[0], p[1], p[2], p[3] = b0/2+band, g0/2+byte(x/8), r0/2, 0xff
		}
	}
	// One box per RetroPad button along the top, lit while held, so a test can
	// see which buttons arrived.
	for b := 0; b < 16; b++ {
		on := pad.Buttons&(1<<b) != 0
		c.rect(4+b*15, 4, 12, 8, on, 0x40)
	}
	c.rect(int(c.x), int(c.y), 16, 16, true, 0xff)
	// A cross where the pointer is pressed, so a test can see that a touch arrived
	// and at which place.
	if c.h.Pointer != nil {
		if p := c.h.Pointer(); p.Pressed {
			x, y := (int(p.X)+32767)*Width/65535, (int(p.Y)+32767)*Height/65535
			c.rect(max(x-6, 0), max(y-1, 0), 12, 3, true, 0xff)
			c.rect(max(x-1, 0), max(y-6, 0), 3, 12, true, 0xff)
		}
	}
}

func (c *Core) rect(x, y, w, h int, on bool, off byte) {
	v := off
	if on {
		v = 0xff
	}
	for j := y; j < y+h && j < Height; j++ {
		for i := x; i < x+w && i < Width; i++ {
			p := c.frame[(j*Width+i)*4:]
			p[0], p[1], p[2] = v, v, v
		}
	}
}

// tone is a square wave while any button is held, silence otherwise.
func (c *Core) tone(pad engine.PadState) {
	period := uint32(Rate / 440)
	for i := 0; i < len(c.audio); i += 2 {
		var s int16
		if pad.Buttons != 0 {
			s = 6000
			if c.phase%period < period/2 {
				s = -6000
			}
		}
		c.phase++
		c.audio[i], c.audio[i+1] = s, s
	}
}

func (c *Core) Reset() { c.x, c.y, c.tick = Width/2, Height/2, 0 }

func (c *Core) SaveState() []byte {
	b := make([]byte, 0, 32)
	b = binary.LittleEndian.AppendUint16(b, uint16(c.x))
	b = binary.LittleEndian.AppendUint16(b, uint16(c.y))
	b = binary.LittleEndian.AppendUint32(b, c.tick)
	return append(b, c.saved[:]...)
}

func (c *Core) LoadState(b []byte) bool {
	if len(b) != 8+len(c.saved) {
		return false
	}
	c.x, c.y = int16(binary.LittleEndian.Uint16(b)), int16(binary.LittleEndian.Uint16(b[2:]))
	c.tick = binary.LittleEndian.Uint32(b[4:])
	copy(c.saved[:], b[8:])
	return true
}

func (c *Core) SaveRAM() []byte { return c.saved[:] }

func (c *Core) Close() {}
