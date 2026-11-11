// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

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

// FreeBSD has real boxes, but neither fits an ordinary helper run by an ordinary account: a jail
// takes root to make, and Capsicum takes away opening any file by its name, which is how a helper
// reads your media. So what this does is apply every limit an ordinary account can set, which do
// hold for code this project didn't write, and a jail as well when this server runs as root. It
// says plainly that this isn't a sandbox, and a fork isn't let in.

func hideWindow(cmd *exec.Cmd) {}

// sandboxAvailable: this system can't box in a fork, whatever else it does.
const sandboxAvailable = false

// launcherMayFail: every limit here has been seen working, so a failure means something.
const launcherMayFail = false

func prepareSandbox(cmd *exec.Cmd, network bool, log *slog.Logger) bool { return false }

func lockDown(cmd *exec.Cmd, memoryMB int64, programs bool, log *slog.Logger) func() {
	return func() {}
}

func launchWith(launcher, program string, memoryMB int64, mode int, args []string) (string, []string) {
	if launcher == "" {
		return program, args
	}
	return launcher, append([]string{SandboxFlag, strconv.FormatInt(memoryMB, 10), strconv.Itoa(mode), program}, args...)
}

// Resource numbers and process controls from FreeBSD's headers, which the system package leaves out.
const (
	rlimitNProc  = 7
	rlimitSBSize = 9
	rlimitNPTS   = 11

	sysProcctl        = 544
	sysJailSet        = 507
	procTraceCtl      = 7
	procTraceDisable  = 2
	procNoNewPrivsCtl = 19
	procNoNewPrivsOn  = 1
	pPid              = 0

	jailCreate     = 1
	jailAttach     = 4
	jailSysDisable = 0
	jailSysInherit = 2
)

func procctl(cmd int, arg int) error {
	_, _, e := unix.Syscall6(sysProcctl, pPid, uintptr(os.Getpid()), uintptr(cmd), uintptr(unsafe.Pointer(&arg)), 0, 0)
	if e != 0 {
		return e
	}
	return nil
}

// ProtectServer keeps a helper from attaching to the server as a debugger, which an account's own
// programs may otherwise do, and which would get round every limit below.
func ProtectServer() {
	if err := procctl(procTraceCtl, procTraceDisable); err != nil {
		slog.Warn("modules: a helper could debug this server", "cat", "modules", "err", err)
	}
}

// jailParam builds the pairs of name and value jail_set takes.
type jailParams []syscall.Iovec

func (j *jailParams) text(s string) {
	b := append([]byte(s), 0)
	*j = append(*j, syscall.Iovec{Base: &b[0], Len: uint64(len(b))})
}

func (j *jailParams) value(name string, v int32) {
	j.text(name)
	p := new(int32)
	*p = v
	*j = append(*j, syscall.Iovec{Base: (*byte)(unsafe.Pointer(p)), Len: 4})
}

// enterJail puts this process in a jail of its own, over the whole file system: it sees no other
// programs, reaches nothing outside unless it may have a network, and can't change the kernel or
// who owns what. It takes root. It ends with the last program in it.
func enterJail(network bool) error {
	addresses := int32(jailSysDisable)
	if network {
		addresses = jailSysInherit
	}
	var j jailParams
	j.text("path")
	j.text("/")
	j.text("name")
	j.text("wittymodule-" + strconv.Itoa(os.Getpid()))
	j.value("ip4", addresses)
	j.value("ip6", addresses)
	j.value("children.max", 0)
	_, _, e := unix.Syscall(sysJailSet, uintptr(unsafe.Pointer(&j[0])), uintptr(len(j)), jailCreate|jailAttach)
	runtime.KeepAlive(j)
	if e != 0 {
		return e
	}
	return nil
}

// RunSandboxed is the server's other life as a launcher: args are memory in MiB, the mode, the
// program, and its arguments. It never returns if all goes well.
func RunSandboxed(args []string) int {
	runtime.LockOSThread()
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "this is how the server starts a module's helper; there is nothing to do by hand")
		return 2
	}
	mem, err := strconv.ParseInt(args[0], 10, 64)
	mode, merr := strconv.Atoi(args[1])
	if err != nil || merr != nil {
		fmt.Fprintln(os.Stderr, "unreadable limits")
		return 2
	}
	if mode&SandboxStrict != 0 {
		fmt.Fprintln(os.Stderr, "this system can't box in a module from another publisher")
		return 1
	}
	limit := func(what int, n int64) {
		if err := unix.Setrlimit(what, &unix.Rlimit{Cur: n, Max: n}); err != nil {
			fmt.Fprintln(os.Stderr, "limit", what, err)
		}
	}
	if err := procctl(procNoNewPrivsCtl, procNoNewPrivsOn); err != nil {
		fmt.Fprintln(os.Stderr, "no new privileges:", err)
	}
	limit(unix.RLIMIT_CORE, 0)
	if mem > 0 {
		limit(unix.RLIMIT_AS, mem<<20)
	}
	limit(rlimitNPTS, 0)
	if mode&SandboxPrograms == 0 {
		limit(rlimitNProc, 0) // threads don't count, so a Go program runs
	}
	if mode&SandboxNetwork == 0 {
		limit(rlimitSBSize, 0) // no room for any socket's buffers, so no new socket
	}
	if mode&(SandboxWriteData|SandboxSaveArg) == 0 {
		limit(unix.RLIMIT_FSIZE, 0) // nothing can be written to a file, whichever file
	}
	if os.Geteuid() == 0 {
		if err := enterJail(mode&SandboxNetwork != 0); err != nil {
			fmt.Fprintln(os.Stderr, "jail:", err)
		}
	}
	err = syscall.Exec(args[2], args[2:], os.Environ())
	fmt.Fprintln(os.Stderr, "could not start:", err)
	return 1
}

// confinement says what a helper gets here.
func confinement(launcher string) SandboxInfo {
	if launcher == "" {
		return unconfined()
	}
	if os.Geteuid() == 0 {
		return SandboxInfo{Level: SandboxLimited, Title: "Modules run in a jail here, but can still change files",
			Words: "This server runs as root, so each module's helper goes in a FreeBSD jail: it can't see programs outside it and has no network unless it needs one. Being root, it can still change any file and take off its other limits. Run this server as an ordinary account if you can: limits hold better there than root's jail does. Modules from other publishers are refused here."}
	}
	return SandboxInfo{Level: SandboxLimited, Title: "Modules run with limits here, not in a sandbox",
		Words: "A module's helper here can't start other programs, open a network connection or write into a file, unless it needs to. FreeBSD offers no more than that to an ordinary account. It can still read anything this server's account can read, delete files this account owns, and stop this account's other programs. That's why modules from other publishers are refused here."}
}
