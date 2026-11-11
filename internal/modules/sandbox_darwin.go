// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// A Mac's own tool for boxing in a program is sandbox-exec. Apple has marked it deprecated, and it
// still ships and still works. It's the same one game runners use here. It's checked before it's
// relied on, as nothing else says whether a later macOS still takes this profile, and if it won't
// the page says that helpers aren't confined. On a real Mac (macOS 27, Apple silicon) helpers start
// in it, and a program in it can't start a sandbox of its own, so WittyPlay's helper goes without
// it: see helperLauncher.

const sandboxExec = "/usr/bin/sandbox-exec"

func hideWindow(cmd *exec.Cmd) {}

// sandboxAvailable: this hasn't been seen on a real Mac, so a fork isn't let in on its word yet.
const sandboxAvailable = false

// launcherMayFail: this has never been run, so a helper that won't start this way is tried again
// without it, and the page says so, rather than every module being lost to a mistake here.
const launcherMayFail = true

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

func ProtectServer() {}

// RunSandboxed is the server's other life as a launcher: args are memory in MiB, the mode, the
// program, and its arguments. Limits on memory aren't enforced by this system, so GOMEMLIMIT is
// what a helper has for those. It never returns if all goes well.
func RunSandboxed(args []string) int {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "this is how the server starts a module's helper; there is nothing to do by hand")
		return 2
	}
	mode, err := strconv.Atoi(args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "unreadable limits")
		return 2
	}
	if mode&SandboxStrict != 0 {
		fmt.Fprintln(os.Stderr, "this system can't box in a module from another publisher")
		return 1
	}
	_ = unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{})
	profile := helperProfile(mode, realPath(args[2]), writableFolders(mode, args[3:], os.Getenv))
	argv := append([]string{sandboxExec, "-p", profile}, args[2:]...)
	err = syscall.Exec(sandboxExec, argv, os.Environ())
	fmt.Fprintln(os.Stderr, "could not start:", err)
	return 1
}

var (
	profileOnce sync.Once
	profileOK   bool
)

// profileWorks tries the profile on a program that does nothing.
func profileWorks() bool {
	profileOnce.Do(func() {
		if _, err := os.Stat(sandboxExec); err != nil {
			return
		}
		const tryIt = "/usr/bin/true"
		profileOK = exec.Command(sandboxExec, "-p", helperProfile(0, tryIt, nil), tryIt).Run() == nil
	})
	return profileOK
}

// confinement says what a helper gets here.
func confinement(launcher string) SandboxInfo {
	if launcher == "" || !profileWorks() || launcherFailed.Load() {
		return unconfined()
	}
	return SandboxInfo{Level: SandboxLimited, Title: "Modules run in Apple's sandbox here",
		Words: "A module's helper can't change files outside its own folder, can't reach the network or start other programs unless it needs to, and can't signal programs outside its own sandbox. It can still read anything this server's account can read. WittyPlay's helper runs outside it, since a Mac can't start one sandbox inside another, and each game it runs gets a stricter sandbox of its own. Modules from other publishers are refused on a Mac for now."}
}
