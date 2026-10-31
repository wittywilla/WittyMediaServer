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
	_ = unix.Setrlimit(unix.RLIMIT_FSIZE, &unix.Rlimit{Cur: maxFileSize, Max: maxFileSize})
	if l.MemoryMB > 0 {
		data := uint64(l.MemoryMB) << 20
		if err := unix.Setrlimit(unix.RLIMIT_DATA, &unix.Rlimit{Cur: data, Max: data}); err != nil {
			errs = append(errs, err)
		}
		// Graphics drivers map far more address space than a machine has memory, so
		// with one loaded only the data limit applies.
		if !l.GPU {
			as := addressSpace(l.MemoryMB)
			if err := unix.Setrlimit(unix.RLIMIT_AS, &unix.Rlimit{Cur: as, Max: as}); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if !l.Normal {
		_ = syscall.Setpriority(syscall.PRIO_PROCESS, 0, 5)
	}
	_ = unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(unix.SIGKILL), 0, 0, 0)
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		errs = append(errs, err)
	} else {
		if err := landlock(l.SaveDir, l.GPU); err != nil {
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

// gpuDevices are the only things a graphics driver opens for writing that
// Landlock would otherwise refuse. Opening them is how every driver talks to its
// card; a missing one is simply skipped.
var gpuDevices = []string{"/dev/dri", "/dev/dxg", "/dev/kfd", "/dev/nvidiactl", "/dev/nvidia0", "/dev/nvidia1", "/dev/nvidia2", "/dev/nvidia3", "/dev/nvidia-modeset", "/dev/nvidia-uvm", "/dev/nvidia-uvm-tools"}

// landlock leaves reading alone (a core may load libraries from anywhere) and
// limits writing to the save folder, and to the graphics devices for a game that
// draws on one.
func landlock(saveDir string, gpu bool) error {
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
	if gpu {
		for _, dev := range gpuDevices {
			d, err := unix.Open(dev, unix.O_PATH|unix.O_CLOEXEC, 0)
			if err != nil {
				continue
			}
			var rule [12]byte
			*(*uint64)(unsafe.Pointer(&rule[0])) = accessFSWriteFile
			*(*int32)(unsafe.Pointer(&rule[8])) = int32(d)
			_, _, e := syscall.Syscall6(sysLandlockAddRule, fd, landlockRulePathBeneath, uintptr(unsafe.Pointer(&rule[0])), 0, 0, 0)
			unix.Close(d)
			if e != 0 {
				return e
			}
		}
	}
	if _, _, e := syscall.Syscall(sysLandlockRestrictSelf, fd, 0, 0); e != 0 {
		return e
	}
	return nil
}
