// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procGetConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")
	procGetConsoleWindow      = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	procIsWindowVisible       = windows.NewLazySystemDLL("user32.dll").NewProc("IsWindowVisible")
)

// startedFromDesktop: a double-click gives the program a console window of its own, which
// no other process shares. Started from a terminal, the terminal's shell shares it; started
// by another program with a hidden console (a script, a scheduled task), nobody sees it.
func startedFromDesktop() bool {
	var pids [4]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if n != 1 {
		return false
	}
	w, _, _ := procGetConsoleWindow.Call()
	if w == 0 {
		return false
	}
	visible, _, _ := procIsWindowVisible.Call(w)
	return visible != 0
}
