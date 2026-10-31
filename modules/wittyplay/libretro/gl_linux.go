package libretro

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
)

// EGL with no window and no display server: a server has neither. Each graphics
// card shows up as an EGL device, so the fast one can be chosen directly.

const (
	eglPlatformDevice      = 0x313F
	eglPlatformSurfaceless = 0x31DD
	eglExtensions          = 0x3055
	eglOpenGLAPI           = 0x30A2
	eglOpenGLESAPI         = 0x30A0
	eglNone                = 0x3038
	eglSurfaceType         = 0x3033
	eglPbufferBit          = 0x0001
	eglRenderableType      = 0x3040
	eglOpenGLBit           = 0x0008
	eglOpenGLES2Bit        = 0x0004
	eglOpenGLES3Bit        = 0x0040
	eglWidth               = 0x3057
	eglHeight              = 0x3056
	eglContextMajor        = 0x3098
	eglContextMinor        = 0x30FB
	eglContextProfile      = 0x30FD
	eglCoreProfileBit      = 0x1
	eglCompatProfileBit    = 0x2
	eglContextFlagsKHR     = 0x30FC
	eglDebugBitKHR         = 0x1
)

type eglFuncs struct {
	getProc, getPlatformDisplay, queryDevices, initialize, terminate, bindAPI, chooseConfig uintptr
	createContext, destroyContext, makeCurrent, createPbuffer, destroySurface, queryString  uintptr
	getDisplay                                                                              uintptr
}

type eglPlatform struct {
	f       eglFuncs
	lib, gl uintptr
	dpy     uintptr
	ctx     uintptr
	surf    uintptr
}

func newGLPlatform(req glRequest) (glPlatform, error) {
	lib, err := purego.Dlopen("libEGL.so.1", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("%w: no OpenGL library (libEGL) is installed", ErrNoGPU)
	}
	useVirtualGPU()
	var f eglFuncs
	for name, slot := range map[string]*uintptr{
		"eglGetProcAddress": &f.getProc, "eglInitialize": &f.initialize, "eglTerminate": &f.terminate, "eglBindAPI": &f.bindAPI, "eglChooseConfig": &f.chooseConfig,
		"eglCreateContext": &f.createContext, "eglDestroyContext": &f.destroyContext, "eglMakeCurrent": &f.makeCurrent, "eglCreatePbufferSurface": &f.createPbuffer,
		"eglDestroySurface": &f.destroySurface, "eglQueryString": &f.queryString, "eglGetDisplay": &f.getDisplay,
	} {
		if *slot, err = purego.Dlsym(lib, name); err != nil {
			return nil, fmt.Errorf("%w: this libEGL has no %s", ErrNoGPU, name)
		}
	}
	get := func(name string) uintptr {
		b := append([]byte(name), 0)
		return call(f.getProc, uintptr(unsafe.Pointer(&b[0])))
	}
	f.getPlatformDisplay = get("eglGetPlatformDisplayEXT")
	f.queryDevices = get("eglQueryDevicesEXT")

	var tried []string
	var fallback *eglPlatform
	try := func(dpy uintptr) *eglPlatform {
		p, err := startEGL(f, lib, dpy, req)
		if err != nil {
			tried = append(tried, err.Error())
			return nil
		}
		name := glString(call(p.proc("glGetString"), glRendererName))
		switch {
		case softwareRenderer(name):
			tried = append(tried, name)
			p.destroyContext()
			return nil
		case integratedGPU(name):
			if fallback == nil {
				fallback = p
				return nil
			}
			p.destroyContext()
			return nil
		}
		return p
	}
	if f.queryDevices != 0 && f.getPlatformDisplay != 0 {
		var devs [16]uintptr
		var n int32
		if ok := call(f.queryDevices, 16, uintptr(unsafe.Pointer(&devs[0])), uintptr(unsafe.Pointer(&n))); ok&0xff != 0 {
			for _, d := range devs[:n] {
				if dpy := call(f.getPlatformDisplay, eglPlatformDevice, d, 0); dpy != 0 {
					if p := try(dpy); p != nil {
						if fallback != nil {
							fallback.destroyContext()
						}
						return p, nil
					}
				}
			}
		}
	}
	// A stack with no device list, such as a virtual machine's.
	if f.getPlatformDisplay != 0 {
		if dpy := call(f.getPlatformDisplay, eglPlatformSurfaceless, 0, 0); dpy != 0 {
			if p := try(dpy); p != nil {
				if fallback != nil {
					fallback.destroyContext()
				}
				return p, nil
			}
		}
	}
	if fallback != nil {
		if !fallback.makeCurrent() {
			return nil, errors.New("the graphics driver would not make its context current")
		}
		return fallback, nil
	}
	return nil, fmt.Errorf("%w (tried: %s)", ErrNoGPU, strings.Join(tried, "; "))
}

// useVirtualGPU points the graphics library at the host's card under WSL, where the
// only way to it is /dev/dxg and nothing else says so. The library reads these
// through C, so that's how they're set, for this process only.
func useVirtualGPU() {
	if _, err := os.Stat("/dev/dxg"); err != nil {
		return
	}
	libc, err := purego.Dlopen("libc.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return
	}
	setenv, err := purego.Dlsym(libc, "setenv")
	if err != nil {
		return
	}
	for _, kv := range [][2]string{{"GALLIUM_DRIVER", "d3d12"}, {"MESA_D3D12_DEFAULT_ADAPTER_NAME", "NVIDIA"}} {
		k, v := append([]byte(kv[0]), 0), append([]byte(kv[1]), 0)
		call(setenv, uintptr(unsafe.Pointer(&k[0])), uintptr(unsafe.Pointer(&v[0])), 0) // an existing value stays
	}
}

// integratedGPU spots a card that shares the machine's memory, to prefer any
// other. It is still far better than none.
func integratedGPU(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "intel") && !strings.Contains(n, " arc")
}

func startEGL(f eglFuncs, lib, dpy uintptr, req glRequest) (*eglPlatform, error) {
	var major, minor int32
	if call(f.initialize, dpy, uintptr(unsafe.Pointer(&major)), uintptr(unsafe.Pointer(&minor)))&0xff == 0 {
		return nil, errors.New("eglInitialize failed")
	}
	api, renderable := uintptr(eglOpenGLAPI), int32(eglOpenGLBit)
	if req.es {
		api, renderable = eglOpenGLESAPI, eglOpenGLES2Bit
		if req.major >= 3 {
			renderable = eglOpenGLES3Bit
		}
	}
	if call(f.bindAPI, api)&0xff == 0 {
		return nil, errors.New("this driver has no OpenGL")
	}
	cfgAttr := []int32{eglSurfaceType, eglPbufferBit, eglRenderableType, renderable, eglNone}
	var cfg uintptr
	var num int32
	if call(f.chooseConfig, dpy, uintptr(unsafe.Pointer(&cfgAttr[0])), uintptr(unsafe.Pointer(&cfg)), 1, uintptr(unsafe.Pointer(&num)))&0xff == 0 || num == 0 {
		return nil, errors.New("this driver has no suitable configuration")
	}
	var attr []int32
	if req.major > 0 {
		attr = append(attr, eglContextMajor, int32(req.major), eglContextMinor, int32(req.minor))
	}
	if !req.es {
		switch {
		case req.core:
			attr = append(attr, eglContextProfile, eglCoreProfileBit)
		case req.major >= 3:
			attr = append(attr, eglContextProfile, eglCompatProfileBit)
		}
	}
	if req.debug {
		attr = append(attr, eglContextFlagsKHR, eglDebugBitKHR)
	}
	attr = append(attr, eglNone)
	ctx := call(f.createContext, dpy, cfg, 0, uintptr(unsafe.Pointer(&attr[0])))
	if ctx == 0 {
		return nil, fmt.Errorf("this driver can't make an OpenGL %d.%d context", req.major, req.minor)
	}
	pb := []int32{eglWidth, 16, eglHeight, 16, eglNone}
	surf := call(f.createPbuffer, dpy, cfg, uintptr(unsafe.Pointer(&pb[0])))
	p := &eglPlatform{f: f, lib: lib, dpy: dpy, ctx: ctx, surf: surf}
	if !p.makeCurrent() {
		p.destroyContext()
		return nil, errors.New("the driver would not make its context current")
	}
	return p, nil
}

func (p *eglPlatform) proc(name string) uintptr {
	b := append([]byte(name), 0)
	if a := call(p.f.getProc, uintptr(unsafe.Pointer(&b[0]))); a != 0 {
		return a
	}
	if p.gl == 0 {
		for _, l := range []string{"libOpenGL.so.0", "libGL.so.1", "libGLESv2.so.2"} {
			if h, err := purego.Dlopen(l, purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
				p.gl = h
				break
			}
		}
	}
	if p.gl != 0 {
		a, _ := purego.Dlsym(p.gl, name)
		return a
	}
	return 0
}

func (p *eglPlatform) makeCurrent() bool {
	return call(p.f.makeCurrent, p.dpy, p.surf, p.surf, p.ctx)&0xff != 0
}

func (p *eglPlatform) destroyContext() {
	call(p.f.makeCurrent, p.dpy, 0, 0, 0)
	if p.surf != 0 {
		call(p.f.destroySurface, p.dpy, p.surf)
		p.surf = 0
	}
	call(p.f.destroyContext, p.dpy, p.ctx)
	p.ctx = 0
}

func (p *eglPlatform) destroy() {
	if p.ctx != 0 {
		p.destroyContext()
	}
	call(p.f.terminate, p.dpy)
}
