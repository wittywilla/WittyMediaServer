package libretro

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
)

// A hidden window with a WGL context: Windows has no other way to make one.
// Nothing is ever shown, and no pixel reaches the window; cores draw to our
// framebuffer object and the frames are read back from there.

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	opengl32 = syscall.NewLazyDLL("opengl32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procGetDC            = user32.NewProc("GetDC")
	procReleaseDC        = user32.NewProc("ReleaseDC")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")

	procChoosePixelFormat = gdi32.NewProc("ChoosePixelFormat")
	procSetPixelFormat    = gdi32.NewProc("SetPixelFormat")

	procWglCreateContext  = opengl32.NewProc("wglCreateContext")
	procWglDeleteContext  = opengl32.NewProc("wglDeleteContext")
	procWglMakeCurrent    = opengl32.NewProc("wglMakeCurrent")
	procWglGetProcAddress = opengl32.NewProc("wglGetProcAddress")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	wglContextMajorVersion = 0x2091
	wglContextMinorVersion = 0x2092
	wglContextFlags        = 0x2094
	wglContextProfileMask  = 0x9126
	wglCoreProfile         = 0x0001
	wglCompatProfile       = 0x0002
	wglES2Profile          = 0x0004
	wglDebugBit            = 0x0001
)

type wndClassEx struct {
	size, style        uint32
	wndProc            uintptr
	clsExtra, wndExtra int32
	instance, icon     uintptr
	cursor, background uintptr
	menuName, name     *uint16
	iconSm             uintptr
}

type pixelFormatDescriptor struct {
	size, version                                 uint16
	flags                                         uint32
	pixelType, colorBits                          uint8
	redBits, redShift, greenBits, greenShift      uint8
	blueBits, blueShift, alphaBits, alphaShift    uint8
	accumBits, accumR, accumG, accumB, accumA     uint8
	depthBits, stencilBits, auxBuffers, layerType uint8
	reserved                                      uint8
	layerMask, visibleMask, damageMask            uint32
}

type wglPlatform struct {
	hwnd, hdc, hglrc uintptr
	gl               uintptr
}

var wglClassDone bool

// newGLPlatform makes a context of the kind a core asked for, current on this
// thread. A context the driver can't make is an error, not a quiet step down.
func newGLPlatform(req glRequest) (glPlatform, error) {
	preferDiscreteGPU()
	if err := opengl32.Load(); err != nil {
		return nil, fmt.Errorf("this server has no OpenGL: %w", err)
	}
	inst, _, _ := procGetModuleHandleW.Call(0)
	name, _ := syscall.UTF16PtrFromString("WittyPlayGL")
	if !wglClassDone {
		cls := wndClassEx{wndProc: procDefWindowProcW.Addr(), instance: inst, name: name, style: 0x20 /* CS_OWNDC */}
		cls.size = uint32(unsafe.Sizeof(cls))
		if a, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&cls))); a == 0 {
			return nil, fmt.Errorf("registering a window class: %w", err)
		}
		wglClassDone = true
	}
	// Never shown: WS_VISIBLE is not set and nothing calls ShowWindow.
	p := &wglPlatform{}
	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(name)), 0x02000000|0x04000000 /* WS_CLIPCHILDREN|WS_CLIPSIBLINGS */, 0, 0, 16, 16, 0, 0, inst, 0)
	if hwnd == 0 {
		return nil, fmt.Errorf("creating a hidden window: %w", err)
	}
	p.hwnd = hwnd
	p.hdc, _, _ = procGetDC.Call(hwnd)
	pfd := pixelFormatDescriptor{version: 1, flags: 0x4 | 0x20 | 0x1 /* DRAW_TO_WINDOW|SUPPORT_OPENGL|DOUBLEBUFFER */, colorBits: 32, depthBits: 24, stencilBits: 8}
	pfd.size = uint16(unsafe.Sizeof(pfd))
	pf, _, _ := procChoosePixelFormat.Call(p.hdc, uintptr(unsafe.Pointer(&pfd)))
	if pf == 0 {
		p.destroy()
		return nil, errors.New("this server has no OpenGL pixel format")
	}
	if ok, _, err := procSetPixelFormat.Call(p.hdc, pf, uintptr(unsafe.Pointer(&pfd))); ok == 0 {
		p.destroy()
		return nil, fmt.Errorf("setting the pixel format: %w", err)
	}
	// A legacy context first: it is the only way to reach the modern entry point.
	legacy, _, err := procWglCreateContext.Call(p.hdc)
	if legacy == 0 {
		p.destroy()
		return nil, fmt.Errorf("creating an OpenGL context: %w", err)
	}
	procWglMakeCurrent.Call(p.hdc, legacy)
	createAttribs := p.proc("wglCreateContextAttribsARB")
	var attribs []int32
	if req.major > 0 {
		attribs = append(attribs, wglContextMajorVersion, int32(req.major), wglContextMinorVersion, int32(req.minor))
	}
	switch {
	case req.es:
		attribs = append(attribs, wglContextProfileMask, wglES2Profile)
	case req.core:
		attribs = append(attribs, wglContextProfileMask, wglCoreProfile)
	case req.major >= 3:
		attribs = append(attribs, wglContextProfileMask, wglCompatProfile)
	}
	if req.debug {
		attribs = append(attribs, wglContextFlags, wglDebugBit)
	}
	attribs = append(attribs, 0)
	if createAttribs != 0 && (len(attribs) > 1 || req.es || req.core) {
		ctx, _, _ := purego.SyscallN(createAttribs, p.hdc, 0, uintptr(unsafe.Pointer(&attribs[0])))
		if ctx == 0 {
			procWglMakeCurrent.Call(0, 0)
			procWglDeleteContext.Call(legacy)
			p.destroy()
			return nil, fmt.Errorf("the graphics driver can't make an OpenGL %d.%d context", req.major, req.minor)
		}
		procWglMakeCurrent.Call(p.hdc, ctx)
		procWglDeleteContext.Call(legacy)
		p.hglrc = ctx
	} else {
		p.hglrc = legacy
	}
	return p, nil
}

// preferDiscreteGPU asks a two-GPU laptop for the fast one. The NVIDIA driver's
// hybrid-graphics layer reads this variable when OpenGL starts, for this process
// only, so no system setting changes. Other drivers ignore it.
func preferDiscreteGPU() {
	if os.Getenv("SHIM_MCCOMPAT") == "" {
		_ = os.Setenv("SHIM_MCCOMPAT", "0x800000001")
	}
}

func (p *wglPlatform) proc(name string) uintptr {
	b := append([]byte(name), 0)
	a, _, _ := procWglGetProcAddress.Call(uintptr(unsafe.Pointer(&b[0])))
	// Some drivers answer 1, 2, 3 or -1 for "no"; OpenGL 1.1 lives in the DLL.
	if a <= 3 || a == ^uintptr(0) {
		if p.gl == 0 {
			p.gl = opengl32.Handle()
		}
		a, _ = syscall.GetProcAddress(syscall.Handle(p.gl), name)
	}
	return a
}

func (p *wglPlatform) makeCurrent() bool {
	ok, _, _ := procWglMakeCurrent.Call(p.hdc, p.hglrc)
	return ok != 0
}

func (p *wglPlatform) destroy() {
	if p.hglrc != 0 {
		procWglMakeCurrent.Call(0, 0)
		procWglDeleteContext.Call(p.hglrc)
		p.hglrc = 0
	}
	if p.hwnd != 0 {
		if p.hdc != 0 {
			procReleaseDC.Call(p.hwnd, p.hdc)
		}
		procDestroyWindow.Call(p.hwnd)
		p.hwnd, p.hdc = 0, 0
	}
}
