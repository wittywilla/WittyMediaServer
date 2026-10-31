//go:build windows

package imaging

import (
	"syscall"
	"unsafe"
)

var procGetDiskFreeSpaceExW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

func freeSpace(dir string) (uint64, bool) {
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, false
	}
	var free uint64
	r, _, _ := procGetDiskFreeSpaceExW.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&free)), 0, 0)
	return free, r != 0
}
