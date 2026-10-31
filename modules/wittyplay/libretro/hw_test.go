package libretro

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/wittywilla/wittyplay/engine"
)

// These need a graphics card, and skip without one. A frame goes through the same
// framebuffer, pixel buffer and fence a core's does.
func TestHardwareFrameComesBack(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	o, err := newGLOut(glRequest{major: 3, minor: 3, core: true}, true, true, true)
	if errors.Is(err, ErrNoGPU) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer o.destroy()
	t.Log("drawing with", o.renderer)
	if err := o.resize(16, 8); err != nil {
		t.Fatal(err)
	}
	fbo := o.fbo
	if err := o.resize(32, 8); err != nil {
		t.Fatal(err)
	}
	if o.fbo != fbo {
		t.Fatal("a larger frame should keep the framebuffer's name")
	}

	// Red in the bottom row as OpenGL counts rows, blue in the rest.
	sub := o.p.proc("glTexSubImage2D")
	if sub == 0 {
		t.Fatal("no glTexSubImage2D")
	}
	pixels := make([]byte, 32*8*4)
	for i := 0; i < len(pixels); i += 4 {
		if i < 32*4 {
			pixels[i], pixels[i+3] = 0xff, 0xff
		} else {
			pixels[i+2], pixels[i+3] = 0xff, 0xff
		}
	}
	call(o.f.bindTexture, glTexture2D, uintptr(o.color))
	call(sub, glTexture2D, 0, 0, 0, 32, 8, glRGBA, glUnsignedByte, uintptr(unsafe.Pointer(&pixels[0])))

	o.startRead(16, 8)
	var got []byte
	var w, h, stride, format int
	o.deliver(engine.Handlers{VideoFlipped: func(d []byte, fw, fh, s, f int) {
		got, w, h, stride, format = append([]byte(nil), d...), fw, fh, s, f
	}})
	if got == nil {
		t.Fatal("no frame came back")
	}
	if w != 16 || h != 8 || stride != 64 || format != engine.FormatXRGB8888 {
		t.Fatalf("frame %dx%d stride %d format %d", w, h, stride, format)
	}
	// BGRA in memory, first row first: the bottom of the picture, in red.
	if got[0] != 0 || got[1] != 0 || got[2] != 0xff {
		t.Errorf("first row = %v, want red", got[:4])
	}
	if p := got[stride*7:]; p[0] != 0xff || p[2] != 0 {
		t.Errorf("last row = %v, want blue", p[:4])
	}
	if e := call(o.f.getError); e != 0 {
		t.Errorf("OpenGL reported error %#x", e)
	}
	// A size the framebuffer doesn't have is ignored, not read out of bounds.
	o.startRead(4096, 4096)
	if o.fence != 0 {
		t.Error("an impossible read should be refused")
	}
}

func TestProbeGPU(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	name, err := ProbeGPU()
	if errors.Is(err, ErrNoGPU) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if name == "" || softwareRenderer(name) {
		t.Errorf("probe named %q", name)
	}
}

func TestSoftwareRendererSpotted(t *testing.T) {
	for name, want := range map[string]bool{"GDI Generic": true, "llvmpipe (LLVM 19.1, 256 bits)": true, "softpipe": true, "": true,
		"NVIDIA GeForce RTX 3070 Laptop GPU/PCIe/SSE2": false, "Intel(R) Iris(R) Xe Graphics": false, "D3D12 (NVIDIA GeForce RTX 3070 Laptop GPU)": false} {
		if got := softwareRenderer(name); got != want {
			t.Errorf("softwareRenderer(%q) = %v", name, got)
		}
	}
}

func TestPointerReachesCore(t *testing.T) {
	c := newTestCore(t, engine.Handlers{Pointer: func() engine.PointerState { return engine.PointerState{X: 1000, Y: -2000, Pressed: true} }})
	var pads [engine.MaxPorts]engine.PadState
	c.h.Input = func() *[engine.MaxPorts]engine.PadState { return &pads }
	purego.SyscallN(cbPoll)
	state := func(port, device, index, id uintptr) int16 {
		v, _, _ := purego.SyscallN(cbState, port, device, index, id)
		return int16(v)
	}
	if got := state(0, devicePointer, 0, 0); got != 1000 {
		t.Errorf("X = %d", got)
	}
	if got := state(0, devicePointer, 0, 1); got != -2000 {
		t.Errorf("Y = %d", got)
	}
	if state(0, devicePointer, 0, 2) != 1 {
		t.Error("should be pressed")
	}
	if state(0, devicePointer, 0, 3) != 1 {
		t.Error("one finger should count as one")
	}
	if state(1, devicePointer, 0, 2) != 0 || state(0, devicePointer, 1, 2) != 0 {
		t.Error("only the first finger of the first player exists")
	}
}

// A real core and game, on the graphics card, when someone has set them up:
//
//	WITTYPLAY_TEST_GPU_CORE=path/to/core WITTYPLAY_TEST_GPU_ROM=path/to/game go test ./libretro
func TestRealGPUCore(t *testing.T) {
	core, rom := os.Getenv("WITTYPLAY_TEST_GPU_CORE"), os.Getenv("WITTYPLAY_TEST_GPU_ROM")
	if core == "" || rom == "" {
		t.Skip("set WITTYPLAY_TEST_GPU_CORE and WITTYPLAY_TEST_GPU_ROM to run this")
	}
	if err := OnCoreThread(func() { playRealGPUCore(t, core, rom) }); err != nil {
		t.Fatal(err)
	}
}

// This runs on the core's own thread, so it reports with Error and returns.
func playRealGPUCore(t *testing.T, core, rom string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var pads [engine.MaxPorts]engine.PadState
	frames, lit := 0, 0
	video := func(d []byte, w, h, stride, format int) {
		frames++
	rows:
		for y := 0; y < h; y += max(h/32, 1) {
			for x := 0; x < w; x += max(w/32, 1) {
				if p := d[y*stride+x*4:]; p[0] > 16 || p[1] > 16 || p[2] > 16 {
					lit++
					break rows
				}
			}
		}
	}
	dir := t.TempDir()
	c, err := Load(core, Paths{System: filepath.Join(dir, "system"), Save: dir}, nil, engine.Handlers{
		Video: video, VideoFlipped: video, Input: func() *[engine.MaxPorts]engine.PadState { return &pads },
	})
	if err != nil {
		t.Error(err)
		return
	}
	if _, err := c.LoadGame(rom); err != nil {
		if errors.Is(err, ErrNoGPU) {
			t.Log("no graphics card here:", err)
		} else {
			t.Error(err)
		}
		return
	}
	for i := 0; i < 3000 && lit == 0; i++ { // consoles start on a black screen
		c.Run()
	}
	t.Logf("%d frames, %d with something on them, drawn by %q", frames, lit, c.Renderer())
	if frames == 0 || lit == 0 {
		t.Errorf("%d frames, %d not blank", frames, lit)
	}
}
