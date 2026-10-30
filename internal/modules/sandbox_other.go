//go:build !windows && !linux

package modules

import (
	"log/slog"
	"os/exec"
)

func hideWindow(cmd *exec.Cmd) {}

// On other systems a helper has its small environment and soft memory limit.
func prepareSandbox(cmd *exec.Cmd, log *slog.Logger) bool { return false }

func lockDown(cmd *exec.Cmd, memoryMB int64, log *slog.Logger) func() { return func() {} }
