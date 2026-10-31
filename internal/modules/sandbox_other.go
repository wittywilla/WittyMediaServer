//go:build !windows && !linux

package modules

import (
	"log/slog"
	"os/exec"
)

func hideWindow(cmd *exec.Cmd) {}

// sandboxAvailable: this system has no way to limit a helper here.
const sandboxAvailable = false

// On other systems a helper has its small environment and soft memory limit.
func prepareSandbox(cmd *exec.Cmd, network bool, log *slog.Logger) bool { return false }

func lockDown(cmd *exec.Cmd, memoryMB int64, log *slog.Logger) func() { return func() {} }

func launchWith(launcher, program string, memoryMB int64, mode int, args []string) (string, []string) {
	return program, args
}

// RunSandboxed is only used on Linux.
func RunSandboxed(args []string) int { return 2 }
