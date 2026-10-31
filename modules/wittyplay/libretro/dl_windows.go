package libretro

import "syscall"

// A damaged or foreign DLL can make Windows put up a dialog and wait for someone
// to click it, which on a server nobody ever does. It should only be an error.
const (
	semFailCriticalErrors = 0x0001
	semNoGPFaultErrorBox  = 0x0002
	semNoOpenFileErrorBox = 0x8000
)

func openLibrary(path string) (uintptr, error) {
	_, _, _ = syscall.NewLazyDLL("kernel32.dll").NewProc("SetErrorMode").Call(semFailCriticalErrors | semNoGPFaultErrorBox | semNoOpenFileErrorBox)
	h, err := syscall.LoadLibrary(path)
	return uintptr(h), err
}

func symbol(lib uintptr, name string) (uintptr, error) {
	return syscall.GetProcAddress(syscall.Handle(lib), name)
}

func closeLibrary(lib uintptr) { _ = syscall.FreeLibrary(syscall.Handle(lib)) }
