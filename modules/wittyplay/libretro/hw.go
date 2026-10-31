package libretro

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Hardware rendering: a core that draws with a graphics card asks for a context,
// draws into a framebuffer we made, and passes the marker below instead of
// pixels. The frame is then read back and goes the same way as any other.

// hwFrameValid is RETRO_HW_FRAME_BUFFER_VALID, the marker in place of pixels.
const hwFrameValid = ^uintptr(0)

// The libretro context types.
const (
	hwNone = iota
	hwOpenGL
	hwGLES2
	hwOpenGLCore
	hwGLES3
	hwGLESVersion
	hwVulkan
	hwD3D11
	hwD3D10
	hwD3D12
	hwD3D9
)

// hwRenderC is struct retro_hw_render_callback. Go lays it out as C does.
type hwRenderC struct {
	contextType           int32
	contextReset          uintptr
	getCurrentFramebuffer uintptr
	getProcAddress        uintptr
	depth, stencil        bool
	bottomLeftOrigin      bool
	versionMajor          uint32
	versionMinor          uint32
	cacheContext          bool
	contextDestroy        uintptr
	debugContext          bool
}

var (
	cbGetFramebuffer, cbGetProc uintptr
)

// hwState is what a core asked for and what was made for it.
type hwState struct {
	req     *hwRenderC // the core's own struct
	reset   uintptr
	destroy uintptr
	gl      *glOut
}

func (c *Core) setHWRender(data unsafe.Pointer) bool {
	req := (*hwRenderC)(data)
	switch req.contextType {
	case hwOpenGL, hwOpenGLCore, hwGLES2, hwGLES3, hwGLESVersion:
	default:
		c.hwRefused = true
		c.logf(2, "this runner can't offer hardware context type %d", req.contextType)
		return false
	}
	c.hw = &hwState{req: req, reset: req.contextReset, destroy: req.contextDestroy}
	req.getCurrentFramebuffer = cbGetFramebuffer
	req.getProcAddress = cbGetProc
	return true
}

// startHW makes the context once the game has loaded, which is when a core has
// said what it needs, and tells the core it can start drawing.
func (c *Core) startHW(maxW, maxH int) error {
	if c.hw == nil {
		return nil
	}
	req := c.hw.req
	gr := glRequest{major: int(req.versionMajor), minor: int(req.versionMinor), debug: req.debugContext}
	switch req.contextType {
	case hwOpenGLCore:
		gr.core = true
	case hwGLES2:
		gr.es, gr.major, gr.minor = true, 2, 0
	case hwGLES3:
		gr.es, gr.major, gr.minor = true, 3, 0
	case hwGLESVersion:
		gr.es = true
	}
	o, err := newGLOut(gr, req.depth, req.stencil, req.bottomLeftOrigin)
	if err != nil {
		return err
	}
	if err := o.resize(maxW, maxH); err != nil {
		o.destroy()
		return err
	}
	c.hw.gl = o
	c.logf(2, "drawing with %s", o.renderer)
	if c.hw.reset != 0 {
		purego.SyscallN(c.hw.reset)
	}
	return nil
}

// Renderer names the graphics card in use, "" for a software-rendered core.
func (c *Core) Renderer() string {
	if c.hw != nil && c.hw.gl != nil {
		return c.hw.gl.renderer
	}
	return ""
}

// stopCoreHW tells the core its context is going away. Unloading comes after this,
// and the context itself after that, because a core may still touch it.
func (c *Core) stopCoreHW() {
	if c.hw != nil && c.hw.gl != nil && c.hw.destroy != 0 {
		purego.SyscallN(c.hw.destroy)
		c.hw.destroy = 0
	}
}

func (c *Core) stopHW() {
	if c.hw != nil && c.hw.gl != nil {
		c.hw.gl.destroy()
		c.hw.gl = nil
	}
}

func (c *Core) logf(level int, format string, a ...any) {
	if c.h.Log != nil {
		c.h.Log(level, fmt.Sprintf(format, a...))
	}
}

func onGetFramebuffer() uintptr {
	if c := cur; c != nil && c.hw != nil && c.hw.gl != nil {
		return uintptr(c.hw.gl.fbo)
	}
	return 0
}

func onGetProc(name uintptr) uintptr {
	c := cur
	if c == nil || c.hw == nil || c.hw.gl == nil || name == 0 {
		return 0
	}
	return c.hw.gl.p.proc(cString((*byte)(ptr(name))))
}

// OptionList is the core's options as it described them, for working out which
// settings to pass it.
func (c *Core) OptionList() []string { return c.optionList }

// ProbeGPU makes an OpenGL context the way a game would and names the graphics
// card behind it, or says why there isn't one.
func ProbeGPU() (string, error) {
	o, err := newGLOut(glRequest{major: 3, minor: 3, core: true}, false, false, false)
	if err != nil {
		return "", err
	}
	defer o.destroy()
	return o.renderer, nil
}

var errCoreThread = errors.New("the system would not start a thread for the core")
