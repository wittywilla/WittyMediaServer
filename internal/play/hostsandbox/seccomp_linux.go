//go:build linux && (amd64 || arm64)

package hostsandbox

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// denied is what no emulator needs. Everything else stays allowed: a core may
// use threads, files and memory mapping in ways nobody can list in advance.
var denied = []uint32{
	unix.SYS_SOCKET, unix.SYS_SOCKETPAIR, unix.SYS_CONNECT, unix.SYS_BIND, unix.SYS_LISTEN, unix.SYS_ACCEPT, unix.SYS_ACCEPT4,
	unix.SYS_EXECVE, unix.SYS_EXECVEAT, unix.SYS_PTRACE, unix.SYS_PROCESS_VM_READV, unix.SYS_PROCESS_VM_WRITEV,
	unix.SYS_MOUNT, unix.SYS_UMOUNT2, unix.SYS_UNSHARE, unix.SYS_SETNS, unix.SYS_PIVOT_ROOT, unix.SYS_CHROOT,
	unix.SYS_KEXEC_LOAD, unix.SYS_KEXEC_FILE_LOAD, unix.SYS_INIT_MODULE, unix.SYS_FINIT_MODULE, unix.SYS_DELETE_MODULE,
	unix.SYS_BPF, unix.SYS_PERF_EVENT_OPEN, unix.SYS_KEYCTL, unix.SYS_ADD_KEY, unix.SYS_REQUEST_KEY,
	// A ring reaches the network and the rest of this list without any of these calls.
	unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER,
}

// pretended seem to work and do nothing. Landlock has no word on who owns a file or what
// its mode is, and a filter can't see a path, so these can't be limited to the save
// folder. A core that sets a mode on its own save carries on none the worse.
var pretended = append([]uint32{
	unix.SYS_FCHMOD, unix.SYS_FCHMODAT, sysFchmodat2, unix.SYS_FCHOWN, unix.SYS_FCHOWNAT,
	unix.SYS_SETXATTR, unix.SYS_LSETXATTR, unix.SYS_FSETXATTR, sysSetxattrat,
}, pretendedHere...)

// Call numbers newer than some versions of the system package.
const (
	sysFchmodat2  = 452
	sysSetxattrat = 463
)

const (
	bpfLD  = 0x00
	bpfJMP = 0x05
	bpfRET = 0x06
	bpfW   = 0x00
	bpfABS = 0x20
	bpfJEQ = 0x10
	bpfJGE = 0x30
	bpfK   = 0x00

	retAllow = 0x7fff0000
	retErrno = 0x00050000 | 1 // SECCOMP_RET_ERRNO with EPERM
	retNoop  = 0x00050000     // SECCOMP_RET_ERRNO with no error: it did nothing, and says it worked

	// x32 is the same architecture with its own numbering: this bit and then a number.
	// Comparing numbers one by one would miss every call in it, and nothing here uses it.
	x32Bit = 0x40000000
)

// seccomp installs the filter. A refused call fails with EPERM, so a core that
// tries to reach the network gets an error it can handle, not a crash.
func seccomp() error {
	arch := uint32(unix.AUDIT_ARCH_X86_64)
	if runtime.GOARCH == "arm64" {
		arch = unix.AUDIT_ARCH_AARCH64
	}
	f := []unix.SockFilter{
		// seccomp_data: nr at offset 0, arch at offset 4. Any other
		// architecture's calling convention is refused outright, because its
		// call numbers would mean something else.
		{Code: bpfLD | bpfW | bpfABS, K: 4},
		{Code: bpfJMP | bpfJEQ | bpfK, Jt: 1, Jf: 0, K: arch},
		{Code: bpfRET | bpfK, K: retErrno},
		{Code: bpfLD | bpfW | bpfABS, K: 0},
		{Code: bpfJMP | bpfJGE | bpfK, Jt: 0, Jf: 1, K: x32Bit},
		{Code: bpfRET | bpfK, K: retErrno},
	}
	refuse := denied
	if !truncateLimited {
		// Landlock on this kernel can't stop truncating by name. Through a descriptor is
		// still fine: one that writes only opens in the save folder.
		refuse = append(append([]uint32(nil), denied...), unix.SYS_TRUNCATE)
	}
	for _, nr := range refuse {
		f = append(f,
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 1, K: nr},
			unix.SockFilter{Code: bpfRET | bpfK, K: retErrno},
		)
	}
	for _, nr := range pretended {
		f = append(f,
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 1, K: nr},
			unix.SockFilter{Code: bpfRET | bpfK, K: retNoop},
		)
	}
	f = append(f, unix.SockFilter{Code: bpfRET | bpfK, K: retAllow})
	prog := unix.SockFprog{Len: uint16(len(f)), Filter: &f[0]}
	// Every thread in the process, the ones Go has already started included. A
	// filter set the older way covers only the calling thread and its children,
	// which might not be the thread a core ends up on.
	if r, _, e := unix.Syscall(unix.SYS_SECCOMP, seccompSetModeFilter, seccompFilterFlagTsync, uintptr(unsafe.Pointer(&prog))); e == 0 && r == 0 {
		return nil
	}
	// One thread is better than none, but the log must say it isn't all of them.
	if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&prog)), 0, 0); err != nil {
		return err
	}
	return errors.New("the syscall filter covers only one thread")
}

const (
	seccompSetModeFilter   = 1
	seccompFilterFlagTsync = 1
)
