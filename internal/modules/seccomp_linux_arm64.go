package modules

import "golang.org/x/sys/unix"

const seccompArch = 0xC00000B7 // AUDIT_ARCH_AARCH64

const seccompX32 uint32 = 0 // no such thing here

// fchmodat2 has no name in this version of the package.
const sysFchmodat2 = 452

var seccompDeny = []uint32{unix.SYS_FCHMOD, unix.SYS_FCHMODAT, sysFchmodat2, unix.SYS_FCHOWN, unix.SYS_FCHOWNAT,
	unix.SYS_PTRACE, unix.SYS_PROCESS_VM_WRITEV, unix.SYS_PROCESS_VM_READV,
	unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER} // it makes sockets with no system call to see

var seccompSockets = []uint32{unix.SYS_SOCKET, unix.SYS_SOCKETPAIR}
