package hostsandbox

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// restartedEnv tells the program, once Confine has started it again, not to start it
// a second time. Whoever starts a runner sets its environment, so nobody else can set
// this, and setting it only skips the restart: Apply still limits its own thread.
const restartedEnv = "WITTYPLAY_WRITES_LIMITED"

// Confine limits writing for every thread this process will ever have. Landlock
// belongs to the thread that asks for it and what that thread starts, and Go is
// already running others. So one thread asks, then becomes this same program again:
// that leaves only it, and every later thread descends from it.
//
// Call it first, before reading stdin or moving a descriptor, as the program runs
// from the top again, and before Apply, whose filter refuses starting a program. A
// system without Landlock has nothing to carry over, and Apply reports that.
func Confine(l Limits) error {
	if os.Getenv(restartedEnv) != "" {
		return os.Unsetenv(restartedEnv)
	}
	runtime.LockOSThread() // never undone: the limit belongs to this thread until it restarts
	if unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) != nil || landlock(l.SaveDir, l.GPU) != nil {
		return nil
	}
	exe, err := os.Executable()
	if err == nil {
		if err = os.Setenv(restartedEnv, "1"); err == nil {
			err = syscall.Exec(exe, os.Args, os.Environ())
			_ = os.Unsetenv(restartedEnv)
		}
	}
	return fmt.Errorf("the write limit doesn't cover every thread: %w", err)
}

// Apply boxes the process in: resource limits, no new privileges, a syscall
// filter that refuses networking and starting programs, and Landlock limiting
// writes. Failures in the layers a kernel or container doesn't offer are returned
// together, and the layers that worked stay on.
//
// Call it from the thread that will run the core and keep to that thread: on Linux
// the priority belongs to the calling thread and what it starts, and so does Landlock
// on kernels before it could synchronise, unless Confine came first. The syscall
// filter covers every thread.
func Apply(l Limits) error {
	runtime.LockOSThread() // never undone: what is set here belongs to this thread
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

	landlockRulePathBeneath   = 1
	landlockRestrictSelfTsync = 1 << 3

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
	accessFSTruncate   = 1 << 14 // since Landlock's third version

	landlockCreateRulesetVersion = 1

	writeAccess = accessFSWriteFile | accessFSRemoveDir | accessFSRemoveFile | accessFSMakeChar | accessFSMakeDir |
		accessFSMakeReg | accessFSMakeSock | accessFSMakeFifo | accessFSMakeBlock | accessFSMakeSym
)

// truncateLimited says Landlock stops truncating outside the save folder here. The
// syscall filter makes up for it if not.
var truncateLimited bool

// gpuDevices are the only things a graphics driver opens for writing that
// Landlock would otherwise refuse. Opening them is how every driver talks to its
// card; a missing one is simply skipped.
var gpuDevices = []string{"/dev/dri", "/dev/dxg", "/dev/kfd", "/dev/nvidiactl", "/dev/nvidia0", "/dev/nvidia1", "/dev/nvidia2", "/dev/nvidia3", "/dev/nvidia-modeset", "/dev/nvidia-uvm", "/dev/nvidia-uvm-tools"}

// landlock leaves reading alone (a core may load libraries from anywhere) and
// limits writing to the save folder, and to the graphics devices for a game that
// draws on one.
func landlock(saveDir string, gpu bool) error {
	access, deviceAccess := uint64(writeAccess), uint64(accessFSWriteFile)
	// A kernel refuses a right newer than it knows of, so it's asked which version it has.
	if v, _, e := syscall.Syscall(sysLandlockCreateRuleset, 0, 0, landlockCreateRulesetVersion); e == 0 && v >= 3 {
		access |= accessFSTruncate
		deviceAccess |= accessFSTruncate
		truncateLimited = true
	}
	attr := struct{ handled uint64 }{handled: access}
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
		*(*uint64)(unsafe.Pointer(&rule[0])) = access
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
			*(*uint64)(unsafe.Pointer(&rule[0])) = deviceAccess
			*(*int32)(unsafe.Pointer(&rule[8])) = int32(d)
			_, _, e := syscall.Syscall6(sysLandlockAddRule, fd, landlockRulePathBeneath, uintptr(unsafe.Pointer(&rule[0])), 0, 0, 0)
			unix.Close(d)
			if e != 0 {
				return e
			}
		}
	}
	// Every thread, where the kernel can do that; otherwise this one and its children.
	if _, _, e := syscall.Syscall(sysLandlockRestrictSelf, fd, landlockRestrictSelfTsync, 0); e == 0 {
		return nil
	}
	if _, _, e := syscall.Syscall(sysLandlockRestrictSelf, fd, 0, 0); e != 0 {
		return e
	}
	return nil
}
