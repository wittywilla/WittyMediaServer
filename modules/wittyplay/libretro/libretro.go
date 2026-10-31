// Package libretro is a small libretro frontend: it loads a core (a shared
// library the user installed), runs it one frame at a time and hands back raw
// video and audio. It uses no cgo, so one Go toolchain builds it everywhere.
//
// This package is only linked into wittyplay-host, the separate sandboxed
// process that runs a game. The server never loads a core itself.
package libretro

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/wittywilla/wittyplay/engine"
)

// Environment commands this frontend answers (from the libretro API).
const (
	envSetRotation       = 1
	envGetCanDupe        = 3
	envSetMessage        = 6
	envShutdown          = 7
	envSetPerformance    = 8
	envGetSystemDir      = 9
	envSetPixelFormat    = 10
	envSetInputDescs     = 11
	envSetHWRender       = 14
	envGetVariable       = 15
	envSetVariables      = 16
	envGetVariableUpdate = 17
	envGetLibretroPath   = 19
	envGetRumble         = 23
	envGetInputCaps      = 24
	envGetLog            = 27
	envGetCoreAssetsDir  = 30
	envGetSaveDir        = 31
	envSetSystemAVInfo   = 32
	envSetSubsystemInfo  = 34
	envSetControllerInfo = 35
	envSetGeometry       = 37
	envGetUsername       = 38
	envGetLanguage       = 39
	envGetCoreOptVersion = 52
	envGetMaxUsers       = 61
	envSetVariable       = 70

	envExperimental      = 0x10000
	envGetFastForwarding = 49 | envExperimental
	envGetTargetRefresh  = 50 | envExperimental
	envGetAudioVideoEn   = 47 | envExperimental
	envGetInputBitmasks  = 51 | envExperimental
	envPrivate           = 0x20000
)

const (
	deviceJoypad = 1
	deviceAnalog = 5
	idBitmask    = 256 // the whole pad as one value

	indexAnalogButton = 2 // an analog device's index for pressure on a button
)

// Paths are the folders a core is told about.
type Paths struct {
	System string // BIOS files
	Save   string // the core's own save files
}

// Core is a loaded libretro core. A process holds at most one, because the
// libretro API is one set of global callbacks per library.
type Core struct {
	lib      uintptr
	h        engine.Handlers
	paths    Paths
	corePath string

	fnInit, fnDeinit, fnRun, fnReset, fnLoadGame, fnUnloadGame uintptr
	fnSerializeSize, fnSerialize, fnUnserialize                uintptr
	fnMemData, fnMemSize, fnAVInfo, fnSysInfo, fnSetCtrl       uintptr
	fnAPIVersion                                               uintptr

	format   int
	vars     map[string]*byte
	varDirty bool
	options  map[string]string
	pinned   [][]byte
	input    *[engine.MaxPorts]engine.PadState
	devices  []engine.PortDevices // from SET_CONTROLLER_INFO
	gameData []byte
	loaded   bool
}

var _ engine.Engine = (*Core)(nil)

// The one core in this process, for the C callbacks.
var (
	cur                                                                 *Core
	callbacks                                                           sync.Once
	cbEnv, cbVideo, cbSample, cbBatch, cbPoll, cbState, cbLog, cbRumble uintptr
)

func makeCallbacks() {
	cbEnv = purego.NewCallback(onEnvironment)
	cbVideo = purego.NewCallback(onVideo)
	cbSample = purego.NewCallback(onAudioSample)
	cbBatch = purego.NewCallback(onAudioBatch)
	cbPoll = purego.NewCallback(onInputPoll)
	cbState = purego.NewCallback(onInputState)
	cbLog = purego.NewCallback(onLog)
	cbRumble = purego.NewCallback(onRumble)
}

// Load opens a core and starts it. Call this, and everything else on Core,
// from one locked OS thread.
func Load(path string, paths Paths, options map[string]string, h engine.Handlers) (*Core, error) {
	if cur != nil {
		return nil, errors.New("libretro: a core is already loaded")
	}
	lib, err := openLibrary(path)
	if err != nil {
		return nil, fmt.Errorf("opening core: %w", err)
	}
	c := &Core{lib: lib, h: h, paths: paths, corePath: path, vars: map[string]*byte{}, options: options, format: engine.Format0RGB1555}
	for name, p := range map[string]*uintptr{
		"retro_init": &c.fnInit, "retro_deinit": &c.fnDeinit, "retro_run": &c.fnRun, "retro_reset": &c.fnReset,
		"retro_load_game": &c.fnLoadGame, "retro_unload_game": &c.fnUnloadGame,
		"retro_serialize_size": &c.fnSerializeSize, "retro_serialize": &c.fnSerialize, "retro_unserialize": &c.fnUnserialize,
		"retro_get_memory_data": &c.fnMemData, "retro_get_memory_size": &c.fnMemSize,
		"retro_get_system_av_info": &c.fnAVInfo, "retro_get_system_info": &c.fnSysInfo,
		"retro_set_controller_port_device": &c.fnSetCtrl, "retro_api_version": &c.fnAPIVersion,
	} {
		f, err := symbol(lib, name)
		if err != nil {
			closeLibrary(lib)
			return nil, fmt.Errorf("%s is not a libretro core: missing %s", filepath.Base(path), name)
		}
		*p = f
	}
	if v, _, _ := purego.SyscallN(c.fnAPIVersion); v != 1 {
		closeLibrary(lib)
		return nil, fmt.Errorf("the core speaks libretro API version %d, not 1", v)
	}
	callbacks.Do(makeCallbacks)
	cur = c
	// The environment goes first: cores read it as soon as they are told.
	for _, set := range []struct {
		name string
		cb   uintptr
	}{
		{"retro_set_environment", cbEnv}, {"retro_set_video_refresh", cbVideo}, {"retro_set_audio_sample", cbSample},
		{"retro_set_audio_sample_batch", cbBatch}, {"retro_set_input_poll", cbPoll}, {"retro_set_input_state", cbState},
	} {
		f, err := symbol(lib, set.name)
		if err != nil {
			cur = nil
			closeLibrary(lib)
			return nil, fmt.Errorf("%s is not a libretro core: missing %s", filepath.Base(path), set.name)
		}
		purego.SyscallN(f, set.cb)
	}
	purego.SyscallN(c.fnInit)
	return c, nil
}

// SystemInfo asks the core about itself.
func (c *Core) SystemInfo() engine.SystemInfo {
	var si struct {
		name, version, exts     *byte
		needFullPath, blockExtr bool
	}
	purego.SyscallN(c.fnSysInfo, uintptr(unsafe.Pointer(&si)))
	out := engine.SystemInfo{Name: cString(si.name), Version: cString(si.version), NeedFullPath: si.needFullPath, BlockExtract: si.blockExtr}
	for _, e := range strings.Split(cString(si.exts), "|") {
		if e != "" {
			out.Extensions = append(out.Extensions, strings.ToLower(e))
		}
	}
	return out
}

// LoadGame gives the core a game. The file is read here only when the core
// wants it in memory.
func (c *Core) LoadGame(path string) (engine.AVInfo, error) {
	info := struct {
		path *byte
		data unsafe.Pointer
		size uintptr
		meta *byte
	}{path: c.pin(path)}
	if !c.SystemInfo().NeedFullPath {
		data, err := os.ReadFile(path)
		if err != nil {
			return engine.AVInfo{}, err
		}
		if len(data) == 0 {
			return engine.AVInfo{}, errors.New("the game file is empty")
		}
		c.gameData = data // cores may keep pointing at it
		info.data, info.size = unsafe.Pointer(&data[0]), uintptr(len(data))
	}
	if ok, _, _ := purego.SyscallN(c.fnLoadGame, uintptr(unsafe.Pointer(&info))); ok&0xff == 0 {
		return engine.AVInfo{}, errors.New("the core could not load this game")
	}
	c.loaded = true
	for p := 0; p < 2; p++ {
		purego.SyscallN(c.fnSetCtrl, uintptr(p), deviceJoypad)
	}
	var av avInfoC
	purego.SyscallN(c.fnAVInfo, uintptr(unsafe.Pointer(&av)))
	return av.convert(), nil
}

// avInfoC is retro_system_av_info. timingPad stands in for the padding C puts
// before a double on the 32-bit platforms where Go aligns it differently.
type avInfoC struct {
	baseW, baseH, maxW, maxH uint32
	aspect                   float32
	_                        [timingPad]byte
	fps, rate                float64
}

func (av *avInfoC) convert() engine.AVInfo {
	return engine.AVInfo{BaseWidth: int(av.baseW), BaseHeight: int(av.baseH), MaxWidth: int(av.maxW), MaxHeight: int(av.maxH), Aspect: float64(av.aspect), FPS: av.fps, SampleRate: av.rate}
}

// Run advances the game one frame.
func (c *Core) Run() { purego.SyscallN(c.fnRun) }

// Reset is the console's reset button.
func (c *Core) Reset() { purego.SyscallN(c.fnReset) }

// SaveState returns a snapshot of the machine, or nil when the core has none.
func (c *Core) SaveState() []byte {
	n, _, _ := purego.SyscallN(c.fnSerializeSize)
	if n == 0 {
		return nil
	}
	buf := make([]byte, n)
	if ok, _, _ := purego.SyscallN(c.fnSerialize, uintptr(unsafe.Pointer(&buf[0])), n); ok&0xff == 0 {
		return nil
	}
	return buf
}

// LoadState restores a snapshot from SaveState.
func (c *Core) LoadState(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	ok, _, _ := purego.SyscallN(c.fnUnserialize, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	return ok&0xff != 0
}

// SaveRAM is the game's battery-backed memory, nil when it has none.
func (c *Core) SaveRAM() []byte {
	p, _, _ := purego.SyscallN(c.fnMemData, 0)
	n, _, _ := purego.SyscallN(c.fnMemSize, 0)
	if p == 0 || n == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(ptr(p)), n)
}

// Close unloads the game and the core.
func (c *Core) Close() {
	if c.loaded {
		purego.SyscallN(c.fnUnloadGame)
		c.loaded = false
	}
	purego.SyscallN(c.fnDeinit)
	closeLibrary(c.lib)
	cur = nil
}

// pin makes a NUL-terminated copy the core may point at for as long as the
// process lives. Go's collector does not move heap memory.
func (c *Core) pin(s string) *byte {
	b := append([]byte(s), 0)
	c.pinned = append(c.pinned, b)
	return &b[0]
}

func cString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for n < 1<<20 && *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// ptr turns an address from C into a pointer without tripping vet: these
// addresses are not Go memory, so the collector has no interest in them.
func ptr(p uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&p)) }

// ---- calls from the core; every argument is passed as a machine word ----

func onVideo(data, width, height, pitch uintptr) uintptr {
	c := cur
	if c == nil {
		return 0
	}
	if data == 0 {
		if c.h.Dupe != nil {
			c.h.Dupe()
		}
		return 0
	}
	if c.h.Video != nil {
		w, h, s := int(uint32(width)), int(uint32(height)), int(pitch)
		c.h.Video(unsafe.Slice((*byte)(ptr(data)), s*h), w, h, s, c.format)
	}
	return 0
}

func onAudioSample(left, right uintptr) uintptr {
	if c := cur; c != nil && c.h.Audio != nil {
		s := [2]int16{int16(left), int16(right)}
		c.h.Audio(s[:])
	}
	return 0
}

func onAudioBatch(data, frames uintptr) uintptr {
	if c := cur; c != nil && c.h.Audio != nil && frames > 0 {
		c.h.Audio(unsafe.Slice((*int16)(ptr(data)), int(frames)*2))
	}
	return frames
}

func onInputPoll() uintptr {
	if c := cur; c != nil && c.h.Input != nil {
		c.input = c.h.Input()
	}
	return 0
}

func onInputState(port, device, index, id uintptr) uintptr {
	c := cur
	port, device, index, id = uintptr(uint32(port)), uintptr(uint32(device)), uintptr(uint32(index)), uintptr(uint32(id))
	if c == nil || c.input == nil || port >= engine.MaxPorts {
		return 0
	}
	st := &c.input[port]
	var v int16
	switch device & 0xff {
	case deviceJoypad:
		if id == idBitmask {
			v = int16(st.Buttons)
		} else if id < 16 && st.Buttons&(1<<id) != 0 {
			v = 1
		}
	case deviceAnalog:
		switch {
		case index == 0 && id == 0:
			v = st.LX
		case index == 0 && id == 1:
			v = st.LY
		case index == 1 && id == 0:
			v = st.RX
		case index == 1 && id == 1:
			v = st.RY
		case index == indexAnalogButton && id < 16:
			v = analogButton(st, id)
		}
	}
	return uintptr(uint16(v))
}

func onLog(level, format uintptr) uintptr {
	if c := cur; c != nil && c.h.Log != nil {
		// This C function is variadic. Its other arguments stay unread: the
		// format string is enough to say what happened.
		c.h.Log(int(uint32(level)), strings.TrimRight(cString((*byte)(ptr(format))), "\r\n"))
	}
	return 0
}

func onRumble(port, effect, strength uintptr) uintptr {
	if c := cur; c != nil && c.h.Rumble != nil {
		c.h.Rumble(int(uint32(port)), int(uint32(effect)), int(uint16(strength)))
	}
	return 1
}

func onEnvironment(cmd, data uintptr) uintptr {
	c := cur
	if c == nil {
		return 0
	}
	if c.environment(uint32(cmd)&^envPrivate, ptr(data)) {
		return 1
	}
	return 0
}

func (c *Core) environment(cmd uint32, data unsafe.Pointer) bool {
	switch cmd {
	case envGetCanDupe:
		*(*bool)(data) = true
	case envSetPixelFormat:
		f := *(*int32)(data)
		if f < 0 || f > 2 {
			return false
		}
		c.format = int(f)
	case envGetSystemDir:
		*(**byte)(data) = c.pin(c.paths.System)
	case envGetSaveDir, envGetCoreAssetsDir:
		*(**byte)(data) = c.pin(c.paths.Save)
	case envGetLibretroPath:
		*(**byte)(data) = c.pin(c.corePath)
	case envGetUsername:
		*(**byte)(data) = c.pin("Player")
	case envGetLanguage:
		*(*uint32)(data) = 0 // English
	case envGetLog:
		*(*uintptr)(data) = cbLog
	case envGetRumble:
		*(*uintptr)(data) = cbRumble
	case envGetInputBitmasks:
	case envGetInputCaps:
		*(*uint64)(data) = 1<<deviceJoypad | 1<<deviceAnalog
	case envGetMaxUsers:
		*(*uint32)(data) = engine.MaxPorts
	case envGetCoreOptVersion:
		// Version 0: the plain "label; a|b|c" option lists below, which every
		// core falls back to.
		*(*uint32)(data) = 0
	case envSetVariables:
		c.setVariables(data)
	case envGetVariable:
		return c.getVariable(data)
	case envGetVariableUpdate:
		*(*bool)(data) = c.varDirty
		c.varDirty = false
	case envSetVariable:
		return c.setVariable(data)
	case envSetGeometry:
		g := (*avInfoC)(data) // the geometry is the start of the same layout
		if c.h.Geometry != nil {
			c.h.Geometry(int(g.baseW), int(g.baseH), float64(g.aspect))
		}
	case envSetSystemAVInfo:
		if c.h.AVInfo != nil {
			c.h.AVInfo((*avInfoC)(data).convert())
		}
	case envSetRotation:
		if c.h.Rotation != nil {
			c.h.Rotation(int(*(*uint32)(data)))
		}
	case envSetMessage:
		if c.h.Message != nil {
			c.h.Message(cString(*(**byte)(data)))
		}
	case envShutdown:
		if c.h.Shutdown != nil {
			c.h.Shutdown()
		}
	case envSetControllerInfo:
		c.devices = readControllerInfo(data)
	case envSetPerformance, envSetInputDescs, envSetSubsystemInfo:
		// Noted, and needed by nothing here.
	case envGetAudioVideoEn:
		*(*int32)(data) = 3 // both on
	case envGetFastForwarding:
		*(*bool)(data) = false
	case envGetTargetRefresh:
		*(*float32)(data) = 60
	case envSetHWRender:
		return false // software-rendered cores only here; see docs/GAMING.md
	default:
		return false
	}
	return true
}

// setVariables reads the core's option list. The first choice is the default,
// and a value the server passed wins when it is one of the choices.
func (c *Core) setVariables(data unsafe.Pointer) {
	type variable struct{ key, value *byte }
	for i := 0; ; i++ {
		v := (*variable)(unsafe.Add(data, uintptr(i)*unsafe.Sizeof(variable{})))
		if v.key == nil {
			return
		}
		key := cString(v.key)
		_, list, _ := strings.Cut(cString(v.value), "; ")
		choices := strings.Split(list, "|")
		val := choices[0]
		if want, ok := c.options[key]; ok {
			for _, ch := range choices {
				if ch == want {
					val = want
				}
			}
		}
		c.vars[key] = c.pin(val)
	}
}

func (c *Core) getVariable(data unsafe.Pointer) bool {
	v := (*struct{ key, value *byte })(data)
	val, ok := c.vars[cString(v.key)]
	v.value = val
	return ok
}

func (c *Core) setVariable(data unsafe.Pointer) bool {
	if data == nil {
		return true // the call that asks whether this command exists
	}
	v := (*struct{ key, value *byte })(data)
	if _, ok := c.vars[cString(v.key)]; ok && v.value != nil {
		c.vars[cString(v.key)] = c.pin(cString(v.value))
		c.varDirty = true
		return true
	}
	return false
}
