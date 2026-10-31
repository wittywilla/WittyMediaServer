package libretro

import (
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/wittywilla/wittyplay/engine"
)

var (
	_ engine.Options   = (*Core)(nil)
	_ engine.Cheats    = (*Core)(nil)
	_ engine.Output    = (*Core)(nil)
	_ engine.StateInto = (*Core)(nil)
)

// optionState is what a core offered, and what it's been asked to skip.
type optionState struct {
	optionList  []engine.Option
	optionAt    map[string]int
	videoOff    bool
	audioOff    bool
	fastForward bool

	cheatReset, cheatSet uintptr
	cheatsLooked         bool
}

// noteOption keeps what a core said about an option. A core may declare its list more
// than once, and the newest wins.
func (c *Core) noteOption(key, label string, choices []string, value string) {
	if c.optionAt == nil {
		c.optionAt = map[string]int{}
	}
	o := engine.Option{Key: key, Label: label, Choices: choices, Value: value}
	if i, ok := c.optionAt[key]; ok {
		c.optionList[i] = o
		return
	}
	c.optionAt[key] = len(c.optionList)
	c.optionList = append(c.optionList, o)
}

// Options lists what the core offers.
func (c *Core) Options() []engine.Option { return c.optionList }

// SetOption changes one option while the game runs. The core reads it when it next
// asks whether anything changed. Only a value the core listed gets through, so nothing
// a person writes can name a file or reach anything else.
func (c *Core) SetOption(key, value string) bool {
	i, ok := c.optionAt[key]
	if !ok {
		return false
	}
	o := &c.optionList[i]
	for _, ch := range o.Choices {
		if ch == value {
			if o.Value != value {
				o.Value = value
				c.vars[key] = c.pin(value)
				c.varDirty = true
			}
			return true
		}
	}
	return false
}

// CheatReset removes every cheat.
func (c *Core) CheatReset() {
	if c.findCheats(); c.cheatReset != 0 {
		purego.SyscallN(c.cheatReset)
	}
}

// CheatSet gives the core one code. Not every core acts on them, and it has no way to say.
func (c *Core) CheatSet(index int, enabled bool, code string) {
	if c.findCheats(); c.cheatSet != 0 {
		on := uintptr(0)
		if enabled {
			on = 1
		}
		purego.SyscallN(c.cheatSet, uintptr(uint32(index)), on, uintptr(unsafe.Pointer(c.pin(code))))
	}
}

func (c *Core) findCheats() {
	if c.cheatsLooked {
		return
	}
	c.cheatsLooked = true
	c.cheatReset, _ = symbol(c.lib, "retro_cheat_reset")
	c.cheatSet, _ = symbol(c.lib, "retro_cheat_set")
}

// SetOutput says what's wanted from the next frame.
func (c *Core) SetOutput(video, audio, fastForward bool) {
	c.videoOff, c.audioOff, c.fastForward = !video, !audio, fastForward
}

// avEnable is the libretro answer: bit 0 video, bit 1 audio.
func (c *Core) avEnable() int32 {
	var v int32
	if !c.videoOff {
		v |= 1
	}
	if !c.audioOff {
		v |= 2
	}
	return v
}

// SaveStateInto is SaveState without a new buffer each time.
func (c *Core) SaveStateInto(buf []byte) []byte {
	n, _, _ := purego.SyscallN(c.fnSerializeSize)
	if n == 0 {
		return nil
	}
	if uintptr(cap(buf)) < n {
		buf = make([]byte, n)
	}
	buf = buf[:n]
	if ok, _, _ := purego.SyscallN(c.fnSerialize, uintptr(unsafe.Pointer(&buf[0])), n); ok&0xff == 0 {
		return nil
	}
	return buf
}
