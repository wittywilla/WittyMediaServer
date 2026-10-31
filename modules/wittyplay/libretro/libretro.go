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
	"time"
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
	envSetDiskControl    = 13
	envSetHWRender       = 14
	envGetVariable       = 15
	envSetVariables      = 16
	envGetVariableUpdate = 17
	envSetSupportNoGame  = 18
	envGetLibretroPath   = 19
	envSetFrameTime      = 21
	envGetRumble         = 23
	envGetInputCaps      = 24
	envGetLog            = 27
	envGetPerf           = 28
	envGetCoreAssetsDir  = 30
	envGetSaveDir        = 31
	envSetSystemAVInfo   = 32
	envSetSubsystemInfo  = 34
	envSetControllerInfo = 35
	envSetGeometry       = 37
	envGetUsername       = 38
	envGetLanguage       = 39
	envSetQuirks         = 44
	envGetCoreOptVersion = 52
	envGetPreferredHW    = 56
	envGetDiskVersion    = 57
	envSetDiskControlExt = 58
	envGetMaxUsers       = 61
	envSetVariable       = 70
	envGetJITCapable     = 74

	envExperimental      = 0x10000
	envSetMemoryMaps     = 36 | envExperimental
	envSetAchievements   = 42 | envExperimental
	envGetFastForwarding = 49 | envExperimental
	envGetTargetRefresh  = 50 | envExperimental
	envGetAudioVideoEn   = 47 | envExperimental
	envGetInputBitmasks  = 51 | envExperimental
	envSetHWShared       = 44 | envExperimental
	envPrivate           = 0x20000
)

const (
	deviceJoypad  = 1
	deviceAnalog  = 5
	devicePointer = 6
	idBitmask     = 256 // the whole pad as one value
	maxPadPorts   = 4
	maxPitch      = 1 << 16
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
	pointer  engine.PointerState
	gameData []byte
	loaded   bool

	hw         *hwState
	hwRefused  bool
	optionList []string

	lastProblem string // the last thing the core complained about, for a failed load
	problemLvl  int
	disk        *diskControl
	ports       int     // how many controller ports the core described
	fnFrameTime uintptr // called before each frame with the time since the last
	frameRef    int64
	lastRun     time.Time
	refused     map[uint32]bool
	dirs        map[string]*byte
}

var _ engine.Engine = (*Core)(nil)

// The one core in this process, for the C callbacks.
var (
	cur                                                                 *Core
	callbacks                                                           sync.Once
	cbEnv, cbVideo, cbSample, cbBatch, cbPoll, cbState, cbLog, cbRumble uintptr
	perfCallbacks                                                       [7]uintptr
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
	cbGetFramebuffer = purego.NewCallback(onGetFramebuffer)
	cbGetProc = purego.NewCallback(onGetProc)
	perfCallbacks = [7]uintptr{purego.NewCallback(perfTimeUsec), purego.NewCallback(perfCPUFeatures), purego.NewCallback(perfCounter),
		purego.NewCallback(perfNothing), purego.NewCallback(perfNothing), purego.NewCallback(perfNothing), purego.NewCallback(perfNothing)}
}

// Load opens a core and starts it. Call this, and everything else on Core,
// from one locked OS thread.
func Load(path string, paths Paths, options map[string]string, h engine.Handlers) (*Core, error) {
	if cur != nil {
		return nil, errors.New("libretro: a core is already loaded")
	}
	prepareGPU()
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
	// The environment goes before retro_init, as cores read it as soon as they are
	// told. Everything else goes after: some cores keep these callbacks in objects
	// that retro_init creates.
	funcs := map[string]uintptr{}
	for _, set := range callbackSetters() {
		f, err := symbol(lib, set.name)
		if err != nil {
			cur = nil
			closeLibrary(lib)
			return nil, fmt.Errorf("%s is not a libretro core: missing %s", filepath.Base(path), set.name)
		}
		funcs[set.name] = f
	}
	startCore(func(name string, cb uintptr) { purego.SyscallN(funcs[name], cb) }, func() { purego.SyscallN(c.fnInit) })
	return c, nil
}

type setter struct {
	name string
	cb   uintptr
}

func callbackSetters() []setter {
	return []setter{{"retro_set_environment", cbEnv}, {"retro_set_video_refresh", cbVideo}, {"retro_set_audio_sample", cbSample},
		{"retro_set_audio_sample_batch", cbBatch}, {"retro_set_input_poll", cbPoll}, {"retro_set_input_state", cbState}}
}

// startCore is the order the libretro API promises: the environment, then
// retro_init, then the rest.
func startCore(set func(name string, cb uintptr), init func()) {
	all := callbackSetters()
	set(all[0].name, all[0].cb)
	init()
	for _, s := range all[1:] {
		set(s.name, s.cb)
	}
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
		if c.hwRefused {
			return engine.AVInfo{}, fmt.Errorf("%w: the core needs one and none could be used", ErrNoGPU)
		}
		if c.lastProblem != "" {
			return engine.AVInfo{}, errors.New("the core could not load this game: " + c.lastProblem)
		}
		return engine.AVInfo{}, errors.New("the core could not load this game")
	}
	c.loaded = true
	// Every port the core described gets a pad, up to what a player can drive.
	ports := min(c.ports, maxPadPorts)
	if ports == 0 {
		ports = 2
	}
	for p := 0; p < ports; p++ {
		purego.SyscallN(c.fnSetCtrl, uintptr(p), deviceJoypad)
	}
	var av avInfoC
	purego.SyscallN(c.fnAVInfo, uintptr(unsafe.Pointer(&av)))
	if err := c.startHW(int(av.maxW), int(av.maxH)); err != nil {
		return engine.AVInfo{}, err
	}
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

// Run advances the game one frame. A frame drawn on the graphics card arrives
// after the core returns, so its reading back doesn't hold the core up.
func (c *Core) Run() {
	if c.fnFrameTime != 0 {
		now := time.Now()
		usec := c.frameRef
		if !c.lastRun.IsZero() {
			usec = now.Sub(c.lastRun).Microseconds()
		}
		c.lastRun = now
		purego.SyscallN(c.fnFrameTime, uintptr(usec))
	}
	purego.SyscallN(c.fnRun)
	if c.hw != nil && c.hw.gl != nil {
		c.hw.gl.deliver(c.h)
	}
}

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
	// Some cores only learn their state size when asked for it, and refuse a state
	// until they have been: a new process hasn't saved one yet.
	purego.SyscallN(c.fnSerializeSize)
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
	c.stopCoreHW()
	t := time.Now()
	if c.loaded {
		purego.SyscallN(c.fnUnloadGame)
		c.loaded = false
	}
	c.tracef("retro_unload_game took %s", time.Since(t))
	t = time.Now()
	purego.SyscallN(c.fnDeinit)
	c.tracef("retro_deinit took %s", time.Since(t))
	c.stopHW()
	// The library stays mapped: the process is about to end, and a core's worker
	// threads may still be on their way out of its code.
	cur = nil
}

func (c *Core) tracef(format string, args ...any) {
	if c.h.Trace != nil {
		c.h.Trace(fmt.Sprintf(format, args...))
	}
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
	if data == hwFrameValid {
		if c.hw != nil && c.hw.gl != nil {
			c.hw.gl.startRead(int(uint32(width)), int(uint32(height)))
		}
		return 0
	}
	if c.h.Video != nil {
		w, h, s := int(uint32(width)), int(uint32(height)), int(pitch)
		if s <= 0 || w <= 0 || h <= 0 || s > maxPitch || h > maxPitch {
			return 0 // nothing a software core should send; not worth a crash
		}
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
	if c := cur; c != nil && c.h.Pointer != nil {
		c.pointer = c.h.Pointer()
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
	case devicePointer:
		if index != 0 || port != 0 {
			return 0 // one finger, for one player
		}
		switch id {
		case 0:
			v = c.pointer.X
		case 1:
			v = c.pointer.Y
		case 2:
			if c.pointer.Pressed {
				v = 1
			}
		case 3:
			if c.pointer.Pressed {
				v = 1
			}
		}
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
		}
	}
	return uintptr(uint16(v))
}

// onLog receives a variadic C call. Its arguments are read as machine words,
// which is what they are on the platforms canReadVarargs allows.
func onLog(level, format, a0, a1, a2, a3, a4, a5 uintptr) uintptr {
	c := cur
	if c == nil {
		return 0
	}
	lvl := int(uint32(level))
	if c.h.Trace == nil && (c.h.Log == nil || lvl < 2) {
		return 0
	}
	text := strings.TrimRight(formatLog(cString((*byte)(ptr(format))), [6]uintptr{a0, a1, a2, a3, a4, a5}), "\r\n")
	if c.h.Trace != nil {
		c.h.Trace(fmt.Sprintf("log %d: %s", lvl, text))
	}
	if lvl >= 2 && lvl >= c.problemLvl && text != "" {
		// One line: some cores log whole documents.
		first, _, _ := strings.Cut(text, "\n")
		c.lastProblem, c.problemLvl = strings.TrimSpace(first), lvl
	}
	if c.h.Log != nil {
		c.h.Log(lvl, text)
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
	n := uint32(cmd) &^ envPrivate
	if c.environment(n, ptr(data)) {
		return 1
	}
	if c.h.Trace != nil && !c.refused[n] {
		if c.refused == nil {
			c.refused = map[uint32]bool{}
		}
		c.refused[n] = true
		c.h.Trace(fmt.Sprintf("refused environment call %d", n&^envExperimental))
	}
	return 0
}

// dir is a folder name a core may keep pointing at. Cores may ask every frame, so
// it is pinned once.
func (c *Core) dir(s string) *byte {
	if p, ok := c.dirs[s]; ok {
		return p
	}
	p := c.pin(s)
	if c.dirs == nil {
		c.dirs = map[string]*byte{}
	}
	c.dirs[s] = p
	return p
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
		*(**byte)(data) = c.dir(c.paths.System)
	case envGetSaveDir, envGetCoreAssetsDir:
		*(**byte)(data) = c.dir(c.paths.Save)
	case envGetLibretroPath:
		*(**byte)(data) = c.dir(c.corePath)
	case envGetUsername:
		*(**byte)(data) = c.dir("Player")
	case envGetLanguage:
		*(*uint32)(data) = 0 // English
	case envGetLog:
		*(*uintptr)(data) = cbLog
	case envGetRumble:
		*(*uintptr)(data) = cbRumble
	case envGetPerf:
		// Some cores call these without checking the answer, so they all work.
		*(*[7]uintptr)(data) = perfCallbacks
	case envSetFrameTime:
		ft := (*struct {
			callback  uintptr
			reference int64
		})(data)
		c.fnFrameTime, c.frameRef = ft.callback, ft.reference
	case envGetDiskVersion:
		*(*uint32)(data) = 1
	case envSetDiskControl, envSetDiskControlExt:
		// The extended form only adds to the end of the same list.
		c.disk = &diskControl{}
		*c.disk = *(*diskControl)(data)
	case envSetControllerInfo:
		c.ports = countControllerPorts(data)
	case envGetInputBitmasks:
	case envGetInputCaps:
		*(*uint64)(data) = 1<<deviceJoypad | 1<<deviceAnalog | 1<<devicePointer
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
		if c.hw != nil && c.hw.gl != nil {
			_ = c.hw.gl.resize(int(g.maxW), int(g.maxH))
		}
	case envSetSystemAVInfo:
		if c.h.AVInfo != nil {
			c.h.AVInfo((*avInfoC)(data).convert())
		}
		if c.hw != nil && c.hw.gl != nil {
			_ = c.hw.gl.resize(int((*avInfoC)(data).maxW), int((*avInfoC)(data).maxH))
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
	case envSetPerformance, envSetInputDescs, envSetSubsystemInfo, envSetSupportNoGame, envSetMemoryMaps, envSetAchievements, envSetQuirks:
		// Noted, and needed by nothing here.
	case envGetJITCapable:
		*(*bool)(data) = true
	case envGetAudioVideoEn:
		*(*int32)(data) = 3 // both on
	case envGetFastForwarding:
		*(*bool)(data) = false
	case envGetTargetRefresh:
		*(*float32)(data) = 60
	case envSetHWRender:
		return c.setHWRender(data)
	case envSetHWShared:
	case envGetPreferredHW:
		*(*uint32)(data) = hwOpenGLCore
	default:
		return false
	}
	return true
}

// diskControl is the start of retro_disk_control_callback.
type diskControl struct {
	setEject, getEject, getIndex, setIndex, getCount, replace, add uintptr
}

// Disc says which disc is in and how many the game has.
func (c *Core) Disc() (index, count int) {
	if c.disk == nil || c.disk.getCount == 0 || c.disk.getIndex == 0 {
		return 0, 0
	}
	n, _, _ := purego.SyscallN(c.disk.getCount)
	i, _, _ := purego.SyscallN(c.disk.getIndex)
	return int(uint32(i)), int(uint32(n))
}

// ChangeDisc opens the drive, swaps the disc and closes it, as a person would.
func (c *Core) ChangeDisc(index int) bool {
	if c.disk == nil || c.disk.setEject == 0 || c.disk.setIndex == 0 {
		return false
	}
	if _, n := c.Disc(); index < 0 || index >= n {
		return false
	}
	if ok, _, _ := purego.SyscallN(c.disk.setEject, 1); ok&0xff == 0 {
		return false
	}
	ok, _, _ := purego.SyscallN(c.disk.setIndex, uintptr(index))
	if closed, _, _ := purego.SyscallN(c.disk.setEject, 0); closed&0xff == 0 {
		return false
	}
	return ok&0xff != 0
}

// countControllerPorts reads retro_controller_info: one {types, count} per port,
// ending with a null entry.
func countControllerPorts(data unsafe.Pointer) int {
	type entry struct {
		types uintptr
		count uint32
	}
	n := 0
	for n < 16 && (*entry)(unsafe.Add(data, uintptr(n)*unsafe.Sizeof(entry{}))).types != 0 {
		n++
	}
	return n
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
		c.optionList = append(c.optionList, key+" = "+cString(v.value))
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
		c.tracef("option %s = %s (%s)", key, val, list)
	}
}

func (c *Core) getVariable(data unsafe.Pointer) bool {
	v := (*struct{ key, value *byte })(data)
	val, ok := c.vars[cString(v.key)]
	v.value = val
	if !ok {
		c.tracef("asked for an option it never declared: %s", cString(v.key))
	}
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
