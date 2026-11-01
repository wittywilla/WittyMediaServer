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

func sandboxStarted(cmd *exec.Cmd) {}

func killRunner(cmd *exec.Cmd) { _ = cmd.Process.Kill() }

// SandboxStrength says what boxes a core in on this platform, for people deciding
// whether to allow custom cores. Resource limits do not stop a program doing harm.
func SandboxStrength() (string, bool) { return "resource limits only", false }

// probePriority does nothing here: the host's own priority is only a little lowered.
func probePriority(cmd *exec.Cmd) {}

// pathTooLong has nothing to say here: only Windows limits these paths.
func pathTooLong(folder, file string) string { return "" }
