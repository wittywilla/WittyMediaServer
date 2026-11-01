package modules

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

func hideWindow(cmd *exec.Cmd) {}

// sandboxAvailable: this system can limit a helper, so a fork never runs without them.
const sandboxAvailable = true

// prepareSandbox asks for fresh namespaces: no network except a dead loopback, no sight
// of other processes to signal, and none of their shared memory. They need unprivileged
// user namespaces, which some containers and distributions refuse; the caller then starts
// the helper without them. A helper allowed a network keeps the host's.
func prepareSandbox(cmd *exec.Cmd, network bool, log *slog.Logger) bool {
	uid, gid := os.Getuid(), os.Getgid()
	flags := uintptr(syscall.CLONE_NEWUSER | syscall.CLONE_NEWIPC | syscall.CLONE_NEWPID)
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

// launchWith wraps a program's start in the server itself, which applies limits to its
// own process and then becomes the program. They survive that, and nothing in the program
// can skip them, so they hold for code this project didn't write too.
func launchWith(launcher, program string, memoryMB int64, mode int, args []string) (string, []string) {
	if launcher == "" {
		return program, args
	}
	return launcher, append([]string{SandboxFlag, strconv.FormatInt(memoryMB, 10), strconv.Itoa(mode), program}, args...)
}

// The Landlock system calls and rights, from the kernel's stable ABI.
const (
	sysLandlockCreateRuleset = 444
	sysLandlockAddRule       = 445
	sysLandlockRestrictSelf  = 446
	landlockRulePathBeneath  = 1
	landlockCreateVersion    = 1

	// Write a file, remove or make a folder or file, a device, a socket, a pipe or a link.
	landlockWrite    = 1<<1 | 1<<4 | 1<<5 | 1<<6 | 1<<7 | 1<<8 | 1<<9 | 1<<10 | 1<<11 | 1<<12
	landlockRefer    = 1 << 13 // ABI 2: move a file from one folder to another
	landlockTruncate = 1 << 14 // ABI 3: cut a file short
	landlockNet      = 1 | 2   // ABI 4: bind and connect over TCP
	landlockScoped   = 1 | 2   // ABI 6: abstract sockets, and signals, to outside this process's domain

	// Without ABI 3 a file can still be emptied, which is not what a fork is promised.
	landlockForkABI = 3
)

func landlockABI() int {
	v, _, e := syscall.Syscall(sysLandlockCreateRuleset, 0, 0, landlockCreateVersion)
	if e != 0 {
		return 0
	}
	return int(v)
}

// RunSandboxed is the server's other life as a launcher: args are memory in MiB, the mode,
// the program, and its arguments. It never returns if all goes well. A layer this kernel
// lacks is skipped for a stock helper, as with the other limits; for a fork it means not
// running. Everything here applies to one thread, and that thread becomes the program, so
// it must not move: limits on another thread would simply be lost.
func RunSandboxed(args []string) int {
	runtime.LockOSThread()
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
	strict := mode&SandboxStrict != 0
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
	if mode&(SandboxNetwork|SandboxDevices) != 0 {
		writable = append(writable, "/dev") // a tuner or a graphics card is opened for writing
	}
	if mode&SandboxSaveArg != 0 {
		for i, a := range args[3:] {
			if a == "-save" && i+1 < len(args[3:]) {
				writable = append(writable, args[3:][i+1])
			}
		}
	}
	abi := landlockABI()
	if strict && abi < landlockForkABI {
		fmt.Fprintln(os.Stderr, "this system's kernel is too old to stop a module from another publisher changing files")
		return 1
	}
	if abi > 0 {
		if err := restrict(abi, writable, mode&SandboxNetwork != 0); err != nil {
			fmt.Fprintln(os.Stderr, "write limits:", err)
			if strict {
				return 1
			}
		}
	}
	if err := denyCalls(mode&SandboxNetwork != 0); err != nil {
		fmt.Fprintln(os.Stderr, "call limits:", err)
		if strict {
			return 1
		}
	}
	err = syscall.Exec(args[2], args[2:], os.Environ())
	fmt.Fprintln(os.Stderr, "could not start:", err)
	return 1
}

// restrict leaves reading alone, since a helper reads media wherever it is, and stops this
// process and everything it becomes from changing any file outside the folders given: not
// the database, not the server's settings, not its own program. Where the kernel can, it
// also stops TCP, and signalling or reaching anything outside itself.
func restrict(abi int, writable []string, network bool) error {
	fs := uint64(landlockWrite)
	if abi >= 2 {
		fs |= landlockRefer
	}
	if abi >= landlockForkABI {
		fs |= landlockTruncate
	}
	attr := struct{ fs, net, scoped uint64 }{fs: fs}
	size := uintptr(8)
	if abi >= 4 {
		size = 16
		if !network {
			attr.net = landlockNet
		}
	}
	if abi >= 6 {
		size = 24
		attr.scoped = landlockScoped
	}
	fd, _, e := syscall.Syscall(sysLandlockCreateRuleset, uintptr(unsafe.Pointer(&attr)), size, 0)
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
		*(*uint64)(unsafe.Pointer(&rule[0])) = fs
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

// denyCalls refuses what Landlock doesn't cover: making any socket, unless this helper may
// have a network, which also keeps it from a socket file such as a container runtime's;
// changing who owns a file or who may use it; and reaching into another process. It stays
// on through the exec that follows.
func denyCalls(network bool) error {
	if seccompArch == 0 {
		return fmt.Errorf("this system has no filter for %s", runtime.GOARCH)
	}
	deny := append([]uint32(nil), seccompDeny...)
	if !network {
		deny = append(deny, seccompSockets...)
	}
	const (
		bpfLdW   = 0x00 | 0x00 | 0x20 // BPF_LD | BPF_W | BPF_ABS
		bpfJeq   = 0x05 | 0x10 | 0x00 // BPF_JMP | BPF_JEQ | BPF_K
		bpfJge   = 0x05 | 0x30 | 0x00 // BPF_JMP | BPF_JGE | BPF_K
		bpfRet   = 0x06 | 0x00        // BPF_RET | BPF_K
		retAllow = 0x7fff0000
		retEPERM = 0x00050000 | uint32(syscall.EPERM)
		retKill  = 0x80000000
		archOff  = 4
		nrOff    = 0
	)
	prog := []unix.SockFilter{
		{Code: bpfLdW, K: archOff},
		{Code: bpfJeq, Jt: 1, K: seccompArch},
		{Code: bpfRet, K: retKill},
		{Code: bpfLdW, K: nrOff},
	}
	if seccompX32 != 0 {
		prog = append(prog, unix.SockFilter{Code: bpfJge, Jf: 1, K: seccompX32}, unix.SockFilter{Code: bpfRet, K: retEPERM})
	}
	for _, nr := range deny {
		prog = append(prog, unix.SockFilter{Code: bpfJeq, Jf: 1, K: nr}, unix.SockFilter{Code: bpfRet, K: retEPERM})
	}
	prog = append(prog, unix.SockFilter{Code: bpfRet, K: retAllow})
	fprog := unix.SockFprog{Len: uint16(len(prog)), Filter: &prog[0]}
	const secModeFilter = 2
	return unix.Prctl(unix.PR_SET_SECCOMP, secModeFilter, uintptr(unsafe.Pointer(&fprog)), 0, 0)
}
