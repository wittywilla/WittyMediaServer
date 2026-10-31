package libretro

import (
	"unsafe"

	"github.com/ebitengine/purego"
)

// OnCoreThread runs fn on a thread the Go runtime didn't create, and returns when
// it has finished.
//
// Several cores run the game on stacks of their own (coroutines), and call back
// from there. The runtime insists that a callback on one of its own threads is on
// that thread's stack, and dies if it isn't. On a thread it didn't create, it
// takes the stack it finds. That's why a core lives on one of those.
func OnCoreThread(fn func()) error {
	libc, err := purego.Dlopen("libc.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	create, err := purego.Dlsym(libc, "pthread_create")
	if err != nil {
		return err
	}
	attrInit, err := purego.Dlsym(libc, "pthread_attr_init")
	if err != nil {
		return err
	}
	attrStack, err := purego.Dlsym(libc, "pthread_attr_setstacksize")
	if err != nil {
		return err
	}
	done := make(chan struct{})
	entry := purego.NewCallback(func(uintptr) uintptr {
		defer close(done)
		fn()
		return 0
	})
	// Emulators and graphics drivers use a good deal more stack than Go code does.
	var attr [64]byte
	purego.SyscallN(attrInit, uintptr(unsafe.Pointer(&attr[0])))
	purego.SyscallN(attrStack, uintptr(unsafe.Pointer(&attr[0])), coreThreadStack)
	var thread uintptr
	if r, _, _ := purego.SyscallN(create, uintptr(unsafe.Pointer(&thread)), uintptr(unsafe.Pointer(&attr[0])), entry, 0); r != 0 {
		return errCoreThread
	}
	<-done
	return nil
}

const coreThreadStack = 16 << 20
