package modules

import "golang.org/x/sys/unix"

const seccompArch = 0xC00000B7 // AUDIT_ARCH_AARCH64

// fchmodat2 has no name in this version of the package.
const sysFchmodat2 = 452

var seccompDeny = []uint32{unix.SYS_FCHMOD, unix.SYS_FCHMODAT, sysFchmodat2, unix.SYS_FCHOWN, unix.SYS_FCHOWNAT,
	unix.SYS_PTRACE, unix.SYS_PROCESS_VM_WRITEV, unix.SYS_PROCESS_VM_READV}

var seccompSockets = []uint32{unix.SYS_SOCKET, unix.SYS_SOCKETPAIR}
