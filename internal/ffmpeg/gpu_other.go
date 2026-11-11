// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !linux && !freebsd

package ffmpeg

import "strconv"

// FindGPUs finds nothing here: Windows and macOS have no device nodes to open,
// and their encoders are found by the test encode alone.
func FindGPUs() []GPUDevice { return nil }

func groupName(gid int) string { return strconv.Itoa(gid) }
