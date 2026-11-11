// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package main

import (
	"os"
	"runtime"
)

// startedFromDesktop: a launcher on a Linux or BSD desktop, with no terminal and no
// service manager. On a Mac, the menu-bar app is how a person starts the server, and it
// asks itself.
func startedFromDesktop() bool {
	if runtime.GOOS == "darwin" || os.Getenv("INVOCATION_ID") != "" {
		return false
	}
	if st, err := os.Stdin.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
		if null, err := os.Stat(os.DevNull); err != nil || !os.SameFile(st, null) {
			return false // a terminal: it gets the line
		}
	}
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}
