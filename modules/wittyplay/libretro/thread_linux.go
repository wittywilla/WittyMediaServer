package libretro

import (
	"unsafe"

	"github.com/ebitengine/purego"
)

// coreThreadStack is generous: emulators keep big things on the stack, and a
// container's default for a new thread can be as little as 2 MB.
const coreThreadStack = 16 << 20

// RunOnCoreThread runs fn on a thread created by C rather than by Go, and returns
// when it has finished. Everything that touches a core belongs inside it.
//
// Some cores (bsnes, through its coroutine library) switch to stacks of their own
// and then call back. Go only copes with that on a thread it didn't create: on one
// of its own it stops the whole program with "morestack on g0".
func RunOnCoreThread(fn func()) {
	create, err := purego.Dlsym(purego.RTLD_DEFAULT, "pthread_create")
	if err != nil {
		fn()
		return
	}
	done := make(chan struct{})
	entry := purego.NewCallback(func(uintptr) uintptr {
		defer close(done)
		fn()
		return 0
	})
	var thread uintptr
	var attr [128]byte // pthread_attr_t, whose size differs by platform but is below this
	attrPtr := uintptr(0)
	if init, err := purego.Dlsym(purego.RTLD_DEFAULT, "pthread_attr_init"); err == nil {
		if set, err := purego.Dlsym(purego.RTLD_DEFAULT, "pthread_attr_setstacksize"); err == nil {
			if r, _, _ := purego.SyscallN(init, uintptr(unsafe.Pointer(&attr))); r == 0 {
				if r, _, _ := purego.SyscallN(set, uintptr(unsafe.Pointer(&attr)), coreThreadStack); r == 0 {
					attrPtr = uintptr(unsafe.Pointer(&attr))
				}
			}
		}
	}
	if r, _, _ := purego.SyscallN(create, uintptr(unsafe.Pointer(&thread)), attrPtr, entry, 0); r != 0 {
		fn()
		return
	}
	<-done
}
