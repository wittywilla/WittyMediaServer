package modules

import "golang.org/x/sys/unix"

const seccompArch = 0xC000003E // AUDIT_ARCH_X86_64

// The kernel's x32 numbers are the same calls with this bit set, and the same architecture
// in the filter's eyes. Nothing here uses them, so all of them are refused.
const seccompX32 uint32 = 0x40000000

// fchmodat2 has no name in this version of the package.
const sysFchmodat2 = 452

var seccompDeny = []uint32{unix.SYS_CHMOD, unix.SYS_FCHMOD, unix.SYS_FCHMODAT, sysFchmodat2, unix.SYS_CHOWN, unix.SYS_FCHOWN, unix.SYS_LCHOWN, unix.SYS_FCHOWNAT,
	unix.SYS_PTRACE, unix.SYS_PROCESS_VM_WRITEV, unix.SYS_PROCESS_VM_READV,
	unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER} // it makes sockets with no system call to see

var seccompSockets = []uint32{unix.SYS_SOCKET, unix.SYS_SOCKETPAIR}
