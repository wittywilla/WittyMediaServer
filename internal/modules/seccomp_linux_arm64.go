// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import "golang.org/x/sys/unix"

const seccompArch = 0xC00000B7 // AUDIT_ARCH_AARCH64

const seccompX32 uint32 = 0 // no such thing here

// These have no names in this version of the package.
const (
	sysFchmodat2     = 452
	sysSetxattrat    = 463
	sysRemovexattrat = 466
)

var seccompDeny = []uint32{unix.SYS_FCHMOD, unix.SYS_FCHMODAT, sysFchmodat2, unix.SYS_FCHOWN, unix.SYS_FCHOWNAT,
	unix.SYS_SETXATTR, unix.SYS_LSETXATTR, unix.SYS_FSETXATTR, unix.SYS_REMOVEXATTR, unix.SYS_LREMOVEXATTR, unix.SYS_FREMOVEXATTR, sysSetxattrat, sysRemovexattrat, // a file's access list is one of these
	unix.SYS_PTRACE, unix.SYS_PROCESS_VM_WRITEV, unix.SYS_PROCESS_VM_READV,
	unix.SYS_BPF, unix.SYS_PERF_EVENT_OPEN, unix.SYS_USERFAULTFD, unix.SYS_KEYCTL, unix.SYS_ADD_KEY, unix.SYS_REQUEST_KEY, unix.SYS_SETNS, // nothing here needs the kernel's more exposed corners
	unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER} // it makes sockets with no system call to see

var seccompSockets = []uint32{unix.SYS_SOCKET, unix.SYS_SOCKETPAIR}
