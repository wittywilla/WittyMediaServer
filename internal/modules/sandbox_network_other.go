//go:build !windows

package modules

import (
	"log/slog"
	"os/exec"
)

// SandboxNetworkProgram limits a separate program that has to reach the network. Here that's left
// to the program, which gives things up for itself: namespaces would take its network away too.
func SandboxNetworkProgram(cmd *exec.Cmd, log *slog.Logger) {}
