//go:build !windows && !linux

package modules

import (
	"log/slog"
	"os/exec"
)

func hideWindow(cmd *exec.Cmd) {}

// On other systems a helper has its small environment and soft memory limit.
func prepareSandbox(cmd *exec.Cmd, network bool, log *slog.Logger) bool { return false }

func lockDown(cmd *exec.Cmd, memoryMB int64, log *slog.Logger) func() { return func() {} }

func launchWith(launcher, program string, memoryMB int64, devices, strict bool, args []string) (string, []string) {
	return program, args
}

// SandboxFlag is only used on Linux.
const SandboxFlag = "__module-sandbox"

// RunSandboxed is only used on Linux.
func RunSandboxed(args []string) int { return 2 }
