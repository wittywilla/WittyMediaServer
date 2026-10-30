package hostsandbox

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Apply boxes the process in: resource limits, no new privileges, a syscall
// filter that refuses networking and starting programs, and Landlock limiting
// writes. Failures in the layers a kernel or container doesn't offer are returned
// together, and the layers that worked stay on.
func Apply(l Limits) error {
	var errs []error
	_ = unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{})
	if l.MemoryMB > 0 {
		n := uint64(l.MemoryMB) << 20
		if err := unix.Setrlimit(unix.RLIMIT_AS, &unix.Rlimit{Cur: n, Max: n}); err != nil {
			errs = append(errs, err)
		}
	}
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, 0, 5)
	_ = unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(unix.SIGKILL), 0, 0, 0)
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		errs = append(errs, err)
	} else {
		if err := landlock(l.SaveDir); err != nil {
			errs = append(errs, err)
		}
		if err := seccomp(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// The Landlock system calls and access rights, from the kernel's stable ABI.
const (
	sysLandlockCreateRuleset = 444
	sysLandlockAddRule       = 445
	sysLandlockRestrictSelf  = 446

	landlockRulePathBeneath = 1

	accessFSWriteFile  = 1 << 1
	accessFSRemoveDir  = 1 << 4
	accessFSRemoveFile = 1 << 5
	accessFSMakeChar   = 1 << 6
	accessFSMakeDir    = 1 << 7
	accessFSMakeReg    = 1 << 8
	accessFSMakeSock   = 1 << 9
	accessFSMakeFifo   = 1 << 10
	accessFSMakeBlock  = 1 << 11
	accessFSMakeSym    = 1 << 12

	writeAccess = accessFSWriteFile | accessFSRemoveDir | accessFSRemoveFile | accessFSMakeChar | accessFSMakeDir |
		accessFSMakeReg | accessFSMakeSock | accessFSMakeFifo | accessFSMakeBlock | accessFSMakeSym
)

// landlock leaves reading alone (a core may load libraries from anywhere) and
// limits writing to the save folder.
func landlock(saveDir string) error {
	attr := struct{ handled uint64 }{handled: writeAccess}
	fd, _, e := syscall.Syscall(sysLandlockCreateRuleset, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if e != 0 {
		return e
	}
	defer syscall.Close(int(fd))
	if saveDir != "" {
		dir, err := unix.Open(saveDir, unix.O_PATH|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		defer unix.Close(dir)
		// struct landlock_path_beneath_attr is packed: u64 then s32.
		var rule [12]byte
		*(*uint64)(unsafe.Pointer(&rule[0])) = writeAccess
		*(*int32)(unsafe.Pointer(&rule[8])) = int32(dir)
		if _, _, e := syscall.Syscall6(sysLandlockAddRule, fd, landlockRulePathBeneath, uintptr(unsafe.Pointer(&rule[0])), 0, 0, 0); e != 0 {
			return e
		}
	}
	if _, _, e := syscall.Syscall(sysLandlockRestrictSelf, fd, 0, 0); e != 0 {
		return e
	}
	return nil
}
