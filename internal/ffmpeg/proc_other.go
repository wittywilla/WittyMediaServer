// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package ffmpeg

import (
	"os/exec"
	"syscall"
)

func hideWindow(cmd *exec.Cmd) {}

// lowPriority and lowPriorityStarted ask the system to give other work the processor
// first. A request about a process that has just started: a failure changes nothing
// that matters.
func lowPriority(cmd *exec.Cmd) {}

func lowPriorityStarted(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, 10)
	}
}
