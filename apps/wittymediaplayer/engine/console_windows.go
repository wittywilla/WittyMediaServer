//go:build windows

package main

import (
	"os"
	"syscall"
)

// attachConsole answers in the terminal this was started from. It's built with no
// console of its own, so that the window can start it without one flashing up.
func attachConsole() {
	if _, err := os.Stdout.Stat(); err == nil {
		return // already going somewhere: a file, or a pipe someone is reading
	}
	const attachParent = ^uintptr(0)
	r, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole").Call(attachParent)
	if r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = f, f
	}
}
