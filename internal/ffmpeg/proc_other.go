// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package ffmpeg

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
