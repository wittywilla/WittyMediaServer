// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build windows

package ffmpeg

import (
	"os/exec"
	"syscall"
)

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// lowPriority starts the process below normal priority (BELOW_NORMAL_PRIORITY_CLASS),
// so other work has the processor first.
func lowPriority(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= 0x00004000
}

func lowPriorityStarted(cmd *exec.Cmd) {}
