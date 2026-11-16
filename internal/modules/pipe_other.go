// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package modules

// osPipeEnded is false here: EPIPE and the closed-pipe errors pipeEnded checks cover it.
func osPipeEnded(error) bool { return false }
