package libretro

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/wittywilla/wittyplay/engine"
	"github.com/wittywilla/wittyplay/ipc"
)

// glPlatform is the system's half of an OpenGL context: a hidden window on
// Windows, EGL elsewhere.
type glPlatform interface {
	proc(name string) uintptr
	makeCurrent() bool
	destroy()
}

// glRequest is the context a core asked for.
type glRequest struct {
	major, minor int
	core, es     bool
	debug        bool
}

// OpenGL names used here, from the Khronos registry.
const (
	glFramebuffer        = 0x8D40
	glReadFramebuffer    = 0x8CA8
	glReadFramebufferBnd = 0x8CAA
	glRenderbuffer       = 0x8D41
	glColorAttachment0   = 0x8CE0
	glDepthAttachment    = 0x8D00
	glDepthStencilAttach = 0x821A
	glFramebufferDone    = 0x8CD5
	glTexture2D          = 0x0DE1
	glTextureBinding2D   = 0x8069
	glRenderbufferBnd    = 0x8CA7
	glFramebufferBnd     = 0x8CA6
	glRGBA               = 0x1908
	glRGBA8              = 0x8058
	glBGRA               = 0x80E1
	glUnsignedByte       = 0x1401
	glDepth24Stencil8    = 0x88F0
	glDepthComponent24   = 0x81A6
	glTexMinFilter       = 0x2801
	glTexMagFilter       = 0x2800
	glNearest            = 0x2600
	glPixelPackBuffer    = 0x88EB
	glPixelPackBnd       = 0x88ED
	glStreamRead         = 0x88E1
	glMapRead            = 0x0001
	glPackAlignment      = 0x0D05
	glPackRowLength      = 0x0D02
	glPackSkipRows       = 0x0D03
	glPackSkipPixels     = 0x0D04
	glSyncGPUDone        = 0x9117
	glSyncFlushCommands  = 0x0001
	glTimeoutExpired     = 0x911B
	glWaitFailed         = 0x911D
	glVendor             = 0x1F00
	glRendererName       = 0x1F01
	glVersion            = 0x1F02
)

type glFuncs struct {
	genFramebuffers, bindFramebuffer, framebufferTexture2D, framebufferRenderbuffer, checkFramebufferStatus uintptr
	deleteFramebuffers, genTextures, bindTexture, texImage2D, texParameteri, deleteTextures                 uintptr
	genRenderbuffers, bindRenderbuffer, renderbufferStorage, deleteRenderbuffers                            uintptr
	genBuffers, bindBuffer, bufferData, mapBufferRange, unmapBuffer, deleteBuffers                          uintptr
	readPixels, pixelStorei, fenceSync, clientWaitSync, deleteSync, flush, getError, getString, getIntegerv uintptr
	viewport, clear                                                                                         uintptr
}

// load fills the table, and fails rather than let a null pointer be called later.
func (f *glFuncs) load(p glPlatform) error {
	for name, slot := range map[string]*uintptr{
		"glGenFramebuffers": &f.genFramebuffers, "glBindFramebuffer": &f.bindFramebuffer, "glFramebufferTexture2D": &f.framebufferTexture2D,
		"glFramebufferRenderbuffer": &f.framebufferRenderbuffer, "glCheckFramebufferStatus": &f.checkFramebufferStatus,
		"glDeleteFramebuffers": &f.deleteFramebuffers, "glGenTextures": &f.genTextures, "glBindTexture": &f.bindTexture,
		"glTexImage2D": &f.texImage2D, "glTexParameteri": &f.texParameteri, "glDeleteTextures": &f.deleteTextures,
		"glGenRenderbuffers": &f.genRenderbuffers, "glBindRenderbuffer": &f.bindRenderbuffer, "glRenderbufferStorage": &f.renderbufferStorage,
		"glDeleteRenderbuffers": &f.deleteRenderbuffers, "glGenBuffers": &f.genBuffers, "glBindBuffer": &f.bindBuffer, "glBufferData": &f.bufferData,
		"glMapBufferRange": &f.mapBufferRange, "glUnmapBuffer": &f.unmapBuffer, "glDeleteBuffers": &f.deleteBuffers, "glReadPixels": &f.readPixels,
		"glPixelStorei": &f.pixelStorei, "glFenceSync": &f.fenceSync, "glClientWaitSync": &f.clientWaitSync, "glDeleteSync": &f.deleteSync,
		"glFlush": &f.flush, "glGetError": &f.getError, "glGetString": &f.getString, "glGetIntegerv": &f.getIntegerv, "glViewport": &f.viewport, "glClear": &f.clear,
	} {
		*slot = p.proc(name)
		if *slot == 0 {
			return fmt.Errorf("the graphics driver has no %s", name)
		}
	}
	return nil
}

func call(fn uintptr, a ...uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, a...)
	return r
}

// glOut is the framebuffer a core draws to, and how each finished frame comes
// back. Reading back goes through a pixel buffer with a fence, so the game
// thread never waits for more than the drawing it has just asked for.
type glOut struct {
	p      glPlatform
	f      glFuncs
	flip   bool
	depth  bool
	stenc  bool
	fbo    uint32
	color  uint32
	ds     uint32
	pbo    uint32
	w, h   int // allocated size of the framebuffer
	pboLen int

	fence        uintptr
	readW, readH int
	renderer     string
}

const maxFramebuffer = ipc.MaxDimension

func newGLOut(req glRequest, depth, stencil, flip bool) (*glOut, error) {
	prepareGPU()
	p, err := newGLPlatform(req)
	if err != nil {
		return nil, err
	}
	o := &glOut{p: p, flip: flip, depth: depth, stenc: stencil}
	if err := o.f.load(p); err != nil {
		p.destroy()
		return nil, err
	}
	o.renderer = glString(call(o.f.getString, glRendererName))
	if softwareRenderer(o.renderer) {
		name := o.renderer
		p.destroy()
		return nil, fmt.Errorf("%w (OpenGL here is only %s)", ErrNoGPU, name)
	}
	return o, nil
}

// ErrNoGPU is why a core that needs a graphics card can't run here.
var ErrNoGPU = errors.New("this server has no usable graphics card for this game")

// softwareRenderer spots OpenGL drawn by the CPU: it works, far too slowly to
// play on, and would hold a low-end server at full load.
func softwareRenderer(name string) bool {
	n := strings.ToLower(name)
	for _, s := range []string{"gdi generic", "llvmpipe", "softpipe", "swrast", "software rasterizer", "swiftshader", "basic render driver"} {
		if strings.Contains(n, s) {
			return true
		}
	}
	return name == ""
}

func glString(p uintptr) string {
	if p == 0 {
		return ""
	}
	return cString((*byte)(ptr(p)))
}

// resize makes the framebuffer at least this big. The names stay the same, so a
// core holding them keeps working.
func (o *glOut) resize(w, h int) error {
	// Cores may claim a far larger maximum than they ever draw. Anything over this
	// couldn't reach the server anyway.
	w, h = min(max(w, 1), maxFramebuffer), min(max(h, 1), maxFramebuffer)
	if w <= o.w && h <= o.h && o.fbo != 0 {
		return nil
	}
	if w < o.w {
		w = o.w
	}
	if h < o.h {
		h = o.h
	}
	f := &o.f
	var oldFB, oldTex, oldRB int32
	call(f.getIntegerv, glFramebufferBnd, uintptr(unsafe.Pointer(&oldFB)))
	call(f.getIntegerv, glTextureBinding2D, uintptr(unsafe.Pointer(&oldTex)))
	call(f.getIntegerv, glRenderbufferBnd, uintptr(unsafe.Pointer(&oldRB)))
	if o.fbo == 0 {
		call(f.genFramebuffers, 1, uintptr(unsafe.Pointer(&o.fbo)))
		call(f.genTextures, 1, uintptr(unsafe.Pointer(&o.color)))
		if o.depth || o.stenc {
			call(f.genRenderbuffers, 1, uintptr(unsafe.Pointer(&o.ds)))
		}
		call(f.genBuffers, 1, uintptr(unsafe.Pointer(&o.pbo)))
	}
	call(f.bindTexture, glTexture2D, uintptr(o.color))
	call(f.texParameteri, glTexture2D, glTexMinFilter, glNearest)
	call(f.texParameteri, glTexture2D, glTexMagFilter, glNearest)
	call(f.texImage2D, glTexture2D, 0, glRGBA8, uintptr(w), uintptr(h), 0, glRGBA, glUnsignedByte, 0)
	call(f.bindFramebuffer, glFramebuffer, uintptr(o.fbo))
	call(f.framebufferTexture2D, glFramebuffer, glColorAttachment0, glTexture2D, uintptr(o.color), 0)
	if o.ds != 0 {
		call(f.bindRenderbuffer, glRenderbuffer, uintptr(o.ds))
		if o.stenc {
			call(f.renderbufferStorage, glRenderbuffer, glDepth24Stencil8, uintptr(w), uintptr(h))
			call(f.framebufferRenderbuffer, glFramebuffer, glDepthStencilAttach, glRenderbuffer, uintptr(o.ds))
		} else {
			call(f.renderbufferStorage, glRenderbuffer, glDepthComponent24, uintptr(w), uintptr(h))
			call(f.framebufferRenderbuffer, glFramebuffer, glDepthAttachment, glRenderbuffer, uintptr(o.ds))
		}
	}
	status := uint32(call(f.checkFramebufferStatus, glFramebuffer))
	call(f.clear, 0x00004000) // GL_COLOR_BUFFER_BIT: the first frame shows black, not old memory
	call(f.bindTexture, glTexture2D, uintptr(uint32(oldTex)))
	call(f.bindRenderbuffer, glRenderbuffer, uintptr(uint32(oldRB)))
	call(f.bindFramebuffer, glFramebuffer, uintptr(uint32(oldFB)))
	if status != glFramebufferDone {
		return fmt.Errorf("the graphics driver refused a %dx%d framebuffer (status %#x)", w, h, status)
	}
	o.w, o.h = w, h
	return nil
}

// startRead asks for a frame to be copied into the pixel buffer, after whatever
// the core has queued. It doesn't wait for anything.
func (o *glOut) startRead(w, h int) {
	if w <= 0 || h <= 0 || w > o.w || h > o.h {
		return
	}
	f := &o.f
	o.finishFence()
	var oldRead, oldPack int32
	var align, row, skipR, skipP int32
	call(f.getIntegerv, glReadFramebufferBnd, uintptr(unsafe.Pointer(&oldRead)))
	call(f.getIntegerv, glPixelPackBnd, uintptr(unsafe.Pointer(&oldPack)))
	call(f.getIntegerv, glPackAlignment, uintptr(unsafe.Pointer(&align)))
	call(f.getIntegerv, glPackRowLength, uintptr(unsafe.Pointer(&row)))
	call(f.getIntegerv, glPackSkipRows, uintptr(unsafe.Pointer(&skipR)))
	call(f.getIntegerv, glPackSkipPixels, uintptr(unsafe.Pointer(&skipP)))

	call(f.bindFramebuffer, glReadFramebuffer, uintptr(o.fbo))
	call(f.bindBuffer, glPixelPackBuffer, uintptr(o.pbo))
	if n := w * h * 4; n > o.pboLen {
		call(f.bufferData, glPixelPackBuffer, uintptr(n), 0, glStreamRead)
		o.pboLen = n
	}
	call(f.pixelStorei, glPackAlignment, 4)
	call(f.pixelStorei, glPackRowLength, 0)
	call(f.pixelStorei, glPackSkipRows, 0)
	call(f.pixelStorei, glPackSkipPixels, 0)
	call(f.readPixels, 0, 0, uintptr(w), uintptr(h), glBGRA, glUnsignedByte, 0)
	o.fence = call(f.fenceSync, glSyncGPUDone, 0)
	call(f.flush)
	o.readW, o.readH = w, h

	call(f.pixelStorei, glPackAlignment, uintptr(align))
	call(f.pixelStorei, glPackRowLength, uintptr(row))
	call(f.pixelStorei, glPackSkipRows, uintptr(skipR))
	call(f.pixelStorei, glPackSkipPixels, uintptr(skipP))
	call(f.bindBuffer, glPixelPackBuffer, uintptr(uint32(oldPack)))
	call(f.bindFramebuffer, glReadFramebuffer, uintptr(uint32(oldRead)))
}

func (o *glOut) finishFence() {
	if o.fence != 0 {
		call(o.f.deleteSync, o.fence)
		o.fence = 0
	}
}

// deliver waits for the frame asked for by startRead and hands it over. The wait
// is only for the GPU to finish this frame's own drawing.
func (o *glOut) deliver(h engine.Handlers) {
	if o.fence == 0 || o.readW == 0 {
		return
	}
	f := &o.f
	for {
		r := uint32(call(f.clientWaitSync, o.fence, glSyncFlushCommands, 50_000_000))
		if r != glTimeoutExpired {
			if r == glWaitFailed {
				o.finishFence()
				return
			}
			break
		}
	}
	o.finishFence()
	var oldPack int32
	call(f.getIntegerv, glPixelPackBnd, uintptr(unsafe.Pointer(&oldPack)))
	call(f.bindBuffer, glPixelPackBuffer, uintptr(o.pbo))
	w, ht := o.readW, o.readH
	n := w * ht * 4
	if p := call(f.mapBufferRange, glPixelPackBuffer, 0, uintptr(n), glMapRead); p != 0 {
		data := unsafe.Slice((*byte)(ptr(p)), n)
		if o.flip && h.VideoFlipped != nil {
			h.VideoFlipped(data, w, ht, w*4, engine.FormatXRGB8888)
		} else if h.Video != nil {
			h.Video(data, w, ht, w*4, engine.FormatXRGB8888)
		}
		call(f.unmapBuffer, glPixelPackBuffer)
	}
	call(f.bindBuffer, glPixelPackBuffer, uintptr(uint32(oldPack)))
	o.readW, o.readH = 0, 0
}

func (o *glOut) destroy() {
	f := &o.f
	if o.p == nil {
		return
	}
	o.finishFence()
	if o.fbo != 0 {
		call(f.deleteFramebuffers, 1, uintptr(unsafe.Pointer(&o.fbo)))
		call(f.deleteTextures, 1, uintptr(unsafe.Pointer(&o.color)))
		if o.ds != 0 {
			call(f.deleteRenderbuffers, 1, uintptr(unsafe.Pointer(&o.ds)))
		}
		call(f.deleteBuffers, 1, uintptr(unsafe.Pointer(&o.pbo)))
	}
	o.p.destroy()
	o.p = nil
}
