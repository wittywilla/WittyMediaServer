// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows && !linux && !freebsd && !darwin

package modules

import (
	"log/slog"
	"os/exec"
)

func hideWindow(cmd *exec.Cmd) {}

// sandboxAvailable: this system has no way to limit a helper here.
const sandboxAvailable = false

// launcherMayFail: there is no launcher here.
const launcherMayFail = false

// On other systems a helper has its small environment and soft memory limit.
func prepareSandbox(cmd *exec.Cmd, network bool, log *slog.Logger) bool { return false }

func lockDown(cmd *exec.Cmd, memoryMB int64, programs bool, log *slog.Logger) func() {
	return func() {}
}

func launchWith(launcher, program string, memoryMB int64, mode int, args []string) (string, []string) {
	return program, args
}

// RunSandboxed is only used on systems with a launcher.
func RunSandboxed(args []string) int { return 2 }

func ProtectServer() {}

func confinement(launcher string) SandboxInfo { return unconfined() }
