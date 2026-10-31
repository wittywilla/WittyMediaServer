package modules

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

func hideWindow(cmd *exec.Cmd) {}

// prepareSandbox asks for fresh namespaces: no network except a dead loopback,
// and no sight of other processes' shared memory. They need unprivileged user
// namespaces, which some containers and distributions refuse; the caller then
// starts the helper without them. A helper allowed a network keeps the host's.
func prepareSandbox(cmd *exec.Cmd, network bool, log *slog.Logger) bool {
	uid, gid := os.Getuid(), os.Getgid()
	flags := uintptr(syscall.CLONE_NEWUSER | syscall.CLONE_NEWIPC)
	if !network {
		flags |= syscall.CLONE_NEWNET
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:                 flags,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: uid, HostID: uid, Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: gid, HostID: gid, Size: 1}},
		GidMappingsEnableSetgroups: false,
		Pdeathsig:                  syscall.SIGKILL,
	}
	return true
}

func lockDown(cmd *exec.Cmd, memoryMB int64, log *slog.Logger) func() { return func() {} }

// SandboxFlag is what the server starts itself with to box a helper in before it runs.
const SandboxFlag = "__module-sandbox"

// launchWith wraps a helper's start in the server itself, which applies limits to its
// own process and then becomes the helper. They survive that, and nothing in the helper
// can skip them, so they hold for code this project didn't write too.
func launchWith(launcher, program string, memoryMB int64, devices, strict bool, args []string) (string, []string) {
	if launcher == "" {
		return program, args
	}
	mode := 0
	if devices {
		mode |= sandboxDevices
	}
	if strict {
		mode |= sandboxStrict
	}
	return launcher, append([]string{SandboxFlag, strconv.FormatInt(memoryMB, 10), strconv.Itoa(mode), program}, args...)
}

// What the launcher is asked to allow and to insist on.
const (
	sandboxDevices = 1 << iota // a tuner opens its device for writing
	sandboxStrict              // a fork: if a limit can't be applied, it doesn't run
)

// The Landlock system calls and write access rights, from the kernel's stable ABI.
const (
	sysLandlockCreateRuleset = 444
	sysLandlockAddRule       = 445
	sysLandlockRestrictSelf  = 446
	landlockRulePathBeneath  = 1
	// Write a file, remove or make a folder or file, a device, a socket, a pipe or a link.
	landlockWrite = 1<<1 | 1<<4 | 1<<5 | 1<<6 | 1<<7 | 1<<8 | 1<<9 | 1<<10 | 1<<11 | 1<<12
)

// RunSandboxed is the server's other life as a launcher: args are memory in MiB, the mode,
// the program, and its arguments. It never returns if all goes well. A layer this kernel
// lacks is skipped for a stock helper, as with the other limits; for a fork it means not
// running. No new privileges is expected everywhere.
func RunSandboxed(args []string) int {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "this is how the server starts a module's helper; there is nothing to do by hand")
		return 2
	}
	mem, err := strconv.ParseUint(args[0], 10, 32)
	mode, merr := strconv.Atoi(args[1])
	if err != nil || merr != nil {
		fmt.Fprintln(os.Stderr, "unreadable limits")
		return 2
	}
	strict := mode&sandboxStrict != 0
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		fmt.Fprintln(os.Stderr, "no new privileges:", err)
		return 1
	}
	_ = unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{})
	if mem > 0 {
		n := mem << 20
		if err := unix.Setrlimit(unix.RLIMIT_AS, &unix.Rlimit{Cur: n, Max: n}); err != nil {
			fmt.Fprintln(os.Stderr, "memory limit:", err)
			if strict {
				return 1
			}
		}
	}
	var writable []string
	if mode&sandboxDevices != 0 {
		writable = append(writable, "/dev")
	}
	if err := denyWrites(writable); err != nil {
		if strict {
			fmt.Fprintln(os.Stderr, "this system won't stop a module from another publisher changing files:", err)
			return 1
		}
		if !errors.Is(err, syscall.ENOSYS) && !errors.Is(err, syscall.EOPNOTSUPP) {
			fmt.Fprintln(os.Stderr, "write limits:", err)
		}
	}
	err = syscall.Exec(args[2], args[2:], os.Environ())
	fmt.Fprintln(os.Stderr, "could not start:", err)
	return 1
}

// denyWrites leaves reading alone, since a helper reads media wherever it is, and stops
// this process and everything it becomes from changing any file outside the folders
// given: not the database, not the server's settings, not its own program.
func denyWrites(writable []string) error {
	attr := struct{ handled uint64 }{handled: landlockWrite}
	fd, _, e := syscall.Syscall(sysLandlockCreateRuleset, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if e != 0 {
		return e
	}
	defer syscall.Close(int(fd))
	for _, p := range writable {
		dir, err := unix.Open(p, unix.O_PATH|unix.O_CLOEXEC, 0)
		if err != nil {
			continue
		}
		// struct landlock_path_beneath_attr is packed: u64 then s32.
		var rule [12]byte
		*(*uint64)(unsafe.Pointer(&rule[0])) = landlockWrite
		*(*int32)(unsafe.Pointer(&rule[8])) = int32(dir)
		_, _, e := syscall.Syscall6(sysLandlockAddRule, fd, landlockRulePathBeneath, uintptr(unsafe.Pointer(&rule[0])), 0, 0, 0)
		unix.Close(dir)
		if e != 0 {
			return e
		}
	}
	if _, _, e := syscall.Syscall(sysLandlockRestrictSelf, fd, 0, 0); e != 0 {
		return e
	}
	return nil
}

// rawLandlockVersion asks the kernel which Landlock it has, for the tests.
func rawLandlockVersion() (uintptr, uintptr, syscall.Errno) {
	return syscall.Syscall(sysLandlockCreateRuleset, 0, 0, 1)
}
