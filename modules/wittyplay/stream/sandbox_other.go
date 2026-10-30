//go:build !windows && !linux && !darwin

package stream

import (
	"log/slog"
	"os/exec"
)

func hideWindow(cmd *exec.Cmd) {}

func runnerEnv() []string { return []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent"} }

// prepareSandbox has nothing to add here: the host sets its own resource limits
// and priority, and docs/GAMING.md says plainly that that's all it gets.
func prepareSandbox(cmd *exec.Cmd, saveDir string, log *slog.Logger) {
	log.Info("sandbox: this platform has resource limits only")
}

func lockDown(cmd *exec.Cmd, memoryMB int64, log *slog.Logger) func() { return func() {} }
