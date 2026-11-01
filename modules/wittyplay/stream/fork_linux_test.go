//go:build linux && amd64

package stream

import (
	"syscall"
	"unsafe"
)

// forkAChild does what a taken-over core can under the syscall filter: the child
// inherits the pipes, and only a raw loop can run in it. It leaves its group, so that
// only the pipes being closed can free the session.
func forkAChild() {
	pid, _, _ := syscall.RawSyscall(syscall.SYS_FORK, 0, 0, 0)
	if pid == 0 {
		_, _, _ = syscall.RawSyscall(syscall.SYS_SETSID, 0, 0, 0)
		ts := syscall.Timespec{Sec: 30}
		_, _, _ = syscall.RawSyscall(syscall.SYS_NANOSLEEP, uintptr(unsafe.Pointer(&ts)), 0, 0)
		syscall.RawSyscall(syscall.SYS_EXIT_GROUP, 0, 0, 0)
	}
}
