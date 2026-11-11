// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package libtranscode

import (
	"os/exec"
	"syscall"
)

// lowPriority asks the system to give other work the processor first. It's a request
// about a process that has just started, so a failure changes nothing that matters.
func lowPriority(cmd *exec.Cmd) {}

func afterStart(cmd *exec.Cmd, low bool) {
	if low && cmd.Process != nil {
		_ = syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, 15)
	}
}

// suspend and resume hold a running FFmpeg still, so it gives up the processor at once
// and can carry on exactly where it was.
func suspend(cmd *exec.Cmd) bool {
	return cmd.Process != nil && syscall.Kill(cmd.Process.Pid, syscall.SIGSTOP) == nil
}

func resume(cmd *exec.Cmd) bool {
	return cmd.Process != nil && syscall.Kill(cmd.Process.Pid, syscall.SIGCONT) == nil
}
