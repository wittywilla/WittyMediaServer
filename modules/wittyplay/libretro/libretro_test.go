package libretro

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/wittywilla/wittyplay/engine"
)

// No core is run here, so no third-party code is needed. The callbacks are called
// through the same C entry points a core uses, with the layouts libretro defines.

func cstr(s string) *byte {
	b := append([]byte(s), 0)
	return &b[0]
}

func newTestCore(t *testing.T, h engine.Handlers) *Core {
	t.Helper()
	callbacks.Do(makeCallbacks)
	c := &Core{h: h, paths: Paths{System: "sys", Save: "save"}, corePath: "core", vars: map[string]*byte{}, options: map[string]string{"opt_b": "two"}}
	cur = c
	t.Cleanup(func() { cur = nil })
	return c
}

func env(cmd uint32, data unsafe.Pointer) bool {
	ok, _, _ := purego.SyscallN(cbEnv, uintptr(cmd), uintptr(data))
	return ok&0xff != 0
}

func TestEnvironmentAnswers(t *testing.T) {
	c := newTestCore(t, engine.Handlers{})

	var yes bool
	if !env(envGetCanDupe, unsafe.Pointer(&yes)) || !yes {
		t.Error("dupe frames should be allowed")
	}
	for _, f := range []int32{engine.FormatRGB565, engine.FormatXRGB8888} {
		if !env(envSetPixelFormat, unsafe.Pointer(&f)) || c.format != int(f) {
			t.Errorf("pixel format %d not taken", f)
		}
	}
	bad := int32(7)
	if env(envSetPixelFormat, unsafe.Pointer(&bad)) {
		t.Error("an unknown pixel format should be refused")
	}
	var p *byte
	if !env(envGetSystemDir, unsafe.Pointer(&p)) || cString(p) != "sys" {
		t.Errorf("system dir = %q", cString(p))
	}
	if !env(envGetSaveDir, unsafe.Pointer(&p)) || cString(p) != "save" {
		t.Errorf("save dir = %q", cString(p))
	}
	if env(envSetHWRender, unsafe.Pointer(&yes)) {
		t.Error("hardware rendering should be refused, so cores fall back to software")
	}
	if env(9999, unsafe.Pointer(&yes)) {
		t.Error("an unknown command should be refused")
	}
	// The private flag on a command changes nothing about it.
	if !env(envGetCanDupe|envPrivate, unsafe.Pointer(&yes)) {
		t.Error("the private flag should be ignored")
	}
}

func TestCoreOptions(t *testing.T) {
	c := newTestCore(t, engine.Handlers{})
	vars := []struct{ key, value *byte }{
		{cstr("opt_a"), cstr("First; one|two|three")},
		{cstr("opt_b"), cstr("Second; one|two|three")},
		{cstr("opt_c"), cstr("Third; one|two")},
		{nil, nil},
	}
	c.options["opt_c"] = "not a choice"
	if !env(envSetVariables, unsafe.Pointer(&vars[0])) {
		t.Fatal("set variables")
	}
	get := func(key string) (string, bool) {
		v := struct{ key, value *byte }{key: cstr(key)}
		ok := env(envGetVariable, unsafe.Pointer(&v))
		return cString(v.value), ok
	}
	for key, want := range map[string]string{"opt_a": "one", "opt_b": "two", "opt_c": "one"} {
		if got, ok := get(key); !ok || got != want {
			t.Errorf("%s = %q, %v; want %q", key, got, ok, want)
		}
	}
	if _, ok := get("missing"); ok {
		t.Error("a missing option should say so")
	}
	set := struct{ key, value *byte }{cstr("opt_a"), cstr("three")}
	if !env(envSetVariable, unsafe.Pointer(&set)) {
		t.Error("setting an option")
	}
	if got, _ := get("opt_a"); got != "three" {
		t.Errorf("opt_a = %q", got)
	}
	var dirty bool
	env(envGetVariableUpdate, unsafe.Pointer(&dirty))
	if !dirty {
		t.Error("a changed option should be reported once")
	}
	env(envGetVariableUpdate, unsafe.Pointer(&dirty))
	if dirty {
		t.Error("and only once")
	}
}

func TestCallbacksCarryArguments(t *testing.T) {
	var gotW, gotH, gotStride, gotFormat, dupes, port, strength int
	var pixels []byte
	var audio []int16
	inputs := [engine.MaxPorts]engine.PadState{}
	inputs[1] = engine.PadState{Buttons: 1<<engine.ButtonA | 1<<engine.ButtonUp, LX: -123, RY: 456}
	c := newTestCore(t, engine.Handlers{
		Video: func(d []byte, w, h, s, f int) {
			pixels, gotW, gotH, gotStride, gotFormat = append([]byte(nil), d...), w, h, s, f
		},
		Dupe:   func() { dupes++ },
		Audio:  func(s []int16) { audio = append(audio, s...) },
		Input:  func() *[engine.MaxPorts]engine.PadState { return &inputs },
		Rumble: func(p, _, s int) { port, strength = p, s },
	})
	c.format = engine.FormatRGB565

	frame := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	purego.SyscallN(cbVideo, uintptr(unsafe.Pointer(&frame[0])), 1, 2, 4)
	if gotW != 1 || gotH != 2 || gotStride != 4 || gotFormat != engine.FormatRGB565 || len(pixels) != 8 || pixels[7] != 8 {
		t.Errorf("video: %d %d %d %d %v", gotW, gotH, gotStride, gotFormat, pixels)
	}
	purego.SyscallN(cbVideo, 0, 1, 2, 4)
	if dupes != 1 {
		t.Error("a null frame means the previous one again")
	}

	samples := []int16{-1000, 1000, 5, -5}
	if n, _, _ := purego.SyscallN(cbBatch, uintptr(unsafe.Pointer(&samples[0])), 2); n != 2 {
		t.Errorf("batch returned %d", n)
	}
	minus := int16(-7)
	purego.SyscallN(cbSample, uintptr(uint16(minus)), 9)
	if len(audio) != 6 || audio[0] != -1000 || audio[3] != -5 || audio[4] != -7 || audio[5] != 9 {
		t.Errorf("audio: %v", audio)
	}

	purego.SyscallN(cbPoll)
	state := func(port, device, index, id uintptr) int16 {
		r, _, _ := purego.SyscallN(cbState, port, device, index, id)
		return int16(uint16(r))
	}
	if state(1, deviceJoypad, 0, engine.ButtonA) != 1 || state(1, deviceJoypad, 0, engine.ButtonB) != 0 {
		t.Error("joypad buttons")
	}
	if uint16(state(1, deviceJoypad, 0, idBitmask)) != inputs[1].Buttons {
		t.Error("the whole pad at once")
	}
	if state(1, deviceAnalog, 0, 0) != -123 || state(1, deviceAnalog, 1, 1) != 456 {
		t.Error("analog sticks")
	}
	if state(0, deviceJoypad, 0, engine.ButtonA) != 0 {
		t.Error("another player's buttons")
	}
	if state(engine.MaxPorts+5, deviceJoypad, 0, engine.ButtonA) != 0 {
		t.Error("a player out of range must read as nothing")
	}

	purego.SyscallN(cbRumble, 1, 0, 0xffff)
	if port != 1 || strength != 0xffff {
		t.Errorf("rumble: %d %d", port, strength)
	}
}

func TestAVInfoLayout(t *testing.T) {
	// The layout of retro_system_av_info: four unsigned, a float, then two doubles
	// aligned as C aligns them on this platform.
	var av avInfoC
	const want64 = 24
	if unsafe.Sizeof(uintptr(0)) == 8 && unsafe.Offsetof(av.fps) != want64 {
		t.Fatalf("fps is at offset %d", unsafe.Offsetof(av.fps))
	}
	av.baseW, av.fps, av.rate = 320, 60.5, 44100
	if got := av.convert(); got.BaseWidth != 320 || got.FPS != 60.5 || got.SampleRate != 44100 {
		t.Fatalf("%+v", got)
	}
}

func TestLoadRefusesWhatIsNotACore(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing"+libraryExtForTest()), Paths{}, nil, engine.Handlers{}); err == nil {
		t.Fatal("a missing file should fail")
	}
	bad := filepath.Join(t.TempDir(), "junk"+libraryExtForTest())
	if err := os.WriteFile(bad, []byte("not a library"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad, Paths{}, nil, engine.Handlers{}); err == nil {
		t.Fatal("a file that isn't a library should fail")
	}
	if runtime.GOOS == "windows" {
		// A real library that isn't a core is told apart too.
		if _, err := Load(`C:\Windows\System32\version.dll`, Paths{}, nil, engine.Handlers{}); err == nil {
			t.Fatal("a library that isn't a core should fail")
		}
	}
	if cur != nil {
		t.Fatal("a failed load must leave nothing behind")
	}
}

func libraryExtForTest() string {
	if runtime.GOOS == "windows" {
		return ".dll"
	}
	return ".so"
}

func TestAskingForPerfInterfaceGivesWorkingCalls(t *testing.T) {
	newTestCore(t, engine.Handlers{})
	var perf [7]uintptr
	if !env(envGetPerf, unsafe.Pointer(&perf)) {
		t.Fatal("perf interface refused")
	}
	for i, f := range perf {
		if f == 0 {
			t.Fatalf("call %d is empty; some cores call them without checking", i)
		}
	}
	a, _, _ := purego.SyscallN(perf[0])
	time.Sleep(2 * time.Millisecond)
	b, _, _ := purego.SyscallN(perf[0])
	if b <= a {
		t.Errorf("the clock didn't advance: %d, %d", a, b)
	}
	var counter [5]uintptr
	purego.SyscallN(perf[3], uintptr(unsafe.Pointer(&counter)))
}
