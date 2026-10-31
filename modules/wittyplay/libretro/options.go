package libretro

import (
	"slices"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/wittywilla/wittyplay/engine"
)

var (
	_ engine.Options   = (*Core)(nil)
	_ engine.Cheats    = (*Core)(nil)
	_ engine.Output    = (*Core)(nil)
	_ engine.StateInto = (*Core)(nil)

	_ engine.OptionsChanged = (*Core)(nil)
)

// The newer ways for a core to describe its options.
const (
	envSetCoreOptions         = 53
	envSetCoreOptionsIntl     = 54
	envSetCoreOptionsDisplay  = 55
	envSetCoreOptionsV2       = 67
	envSetCoreOptionsV2Intl   = 68
	envSetCoreOptionsUpdateCB = 69

	maxCoreValues = 128  // RETRO_NUM_CORE_OPTION_VALUES_MAX, part of the structures' layout
	maxCoreDefs   = 4096 // a list with no end is a broken core, not a long one
)

type coreValue struct{ value, label *byte }

type coreDefV1 struct {
	key, desc, info *byte
	values          [maxCoreValues]coreValue
	defaultValue    *byte
}

type coreDefV2 struct {
	key, desc, descCategorized, info, infoCategorized, category *byte
	values                                                      [maxCoreValues]coreValue
	defaultValue                                                *byte
}

type coreCategory struct{ key, desc, info *byte }

type coreOptionsV2 struct {
	categories  *coreCategory
	definitions *coreDefV2
}

type coreIntl struct{ us, local unsafe.Pointer }

// optionState is what a core offered, and what it's been asked to skip.
type optionState struct {
	optionList   []engine.Option
	optionAt     map[string]int
	optionsMoved bool
	displayCB    uintptr
	videoOff     bool
	audioOff     bool
	fastForward  bool

	cheatReset, cheatSet uintptr
	cheatsLooked         bool
}

// optionEnvironment answers the commands that describe options. This frontend's language
// is English, so what a core offers for other languages isn't read.
func (c *Core) optionEnvironment(cmd uint32, data unsafe.Pointer) bool {
	switch cmd {
	case envSetCoreOptions, envSetCoreOptionsIntl:
		if data == nil {
			return true
		}
		if cmd == envSetCoreOptionsIntl {
			data = (*coreIntl)(data).us
		}
		c.readOptionsV1((*coreDefV1)(data))
	case envSetCoreOptionsV2, envSetCoreOptionsV2Intl:
		if data == nil {
			return true
		}
		if cmd == envSetCoreOptionsV2Intl {
			data = (*coreIntl)(data).us
		}
		c.readOptionsV2((*coreOptionsV2)(data))
	case envSetCoreOptionsDisplay:
		if data == nil {
			return true
		}
		d := (*struct {
			key     *byte
			visible bool
		})(data)
		if i, ok := c.optionAt[cString(d.key)]; ok && c.optionList[i].Hidden == d.visible {
			c.optionList[i].Hidden = !d.visible
			c.optionsMoved = true
		}
	case envSetCoreOptionsUpdateCB:
		if data == nil {
			return true
		}
		c.displayCB = *(*uintptr)(data)
	default:
		return false
	}
	return true
}

func (c *Core) readOptionsV1(def *coreDefV1) {
	for i := 0; def != nil && i < maxCoreDefs; i++ {
		d := (*coreDefV1)(unsafe.Add(unsafe.Pointer(def), uintptr(i)*unsafe.Sizeof(coreDefV1{})))
		if d.key == nil {
			return
		}
		c.addOption(d.key, d.desc, d.info, "", &d.values, d.defaultValue)
	}
}

func (c *Core) readOptionsV2(o *coreOptionsV2) {
	if o == nil {
		return
	}
	names := map[string]string{}
	for i := 0; o.categories != nil && i < maxCoreDefs; i++ {
		k := (*coreCategory)(unsafe.Add(unsafe.Pointer(o.categories), uintptr(i)*unsafe.Sizeof(coreCategory{})))
		if k.key == nil {
			break
		}
		names[cString(k.key)] = cString(k.desc)
	}
	for i := 0; o.definitions != nil && i < maxCoreDefs; i++ {
		d := (*coreDefV2)(unsafe.Add(unsafe.Pointer(o.definitions), uintptr(i)*unsafe.Sizeof(coreDefV2{})))
		if d.key == nil {
			return
		}
		c.addOption(d.key, d.desc, d.info, names[cString(d.category)], &d.values, d.defaultValue)
	}
}

// addOption takes one option from either format, with the same rule as the plain list:
// a value the server passed wins when it's one of the choices.
func (c *Core) addOption(key, desc, info *byte, category string, values *[maxCoreValues]coreValue, def *byte) {
	var choices, labels []string
	for i := range values {
		if values[i].value == nil {
			break
		}
		v := cString(values[i].value)
		name := cString(values[i].label)
		if name == "" {
			name = v
		}
		choices, labels = append(choices, v), append(labels, name)
	}
	if len(choices) == 0 {
		return
	}
	k := cString(key)
	dflt := cString(def)
	if !slices.Contains(choices, dflt) {
		dflt = choices[0]
	}
	val := dflt
	if want, ok := c.options[k]; ok && slices.Contains(choices, want) {
		val = want
	}
	c.vars[k] = c.pin(val)
	c.noteOption(k, cString(desc), choices, val)
	o := &c.optionList[c.optionAt[k]]
	o.Info, o.Category, o.Labels, o.Default = cString(info), category, labels, dflt
}

// OptionsChanged says the list has moved since the last time anyone looked: a core can
// hide or show an option whenever it likes, usually when another one changes.
func (c *Core) OptionsChanged() bool {
	moved := c.optionsMoved
	c.optionsMoved = false
	return moved
}

// noteOption keeps what a core said about an option. A core may declare its list more
// than once, and the newest wins.
func (c *Core) noteOption(key, label string, choices []string, value string) {
	if c.optionAt == nil {
		c.optionAt = map[string]int{}
	}
	o := engine.Option{Key: key, Label: label, Choices: choices, Value: value, Default: choices[0]}
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
				if c.displayCB != 0 { // this may change which others apply
					purego.SyscallN(c.displayCB)
				}
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
