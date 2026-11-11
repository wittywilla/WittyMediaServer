// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build windows

package desktop

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procCreateDesktopW   = user32.NewProc("CreateDesktopW")
	procCloseDesktop     = user32.NewProc("CloseDesktop")
	procSetThreadDesktop = user32.NewProc("SetThreadDesktop")
	procGetThreadDesktop = user32.NewProc("GetThreadDesktop")
)

// The task dialog really opens and answers, on a desktop of its own that nobody sees:
// nothing appears on the screen of whoever runs the tests. If that desktop can't be had,
// the test is skipped rather than show anything.
func TestTaskDialogOnHiddenDesktop(t *testing.T) {
	skip := make(chan string, 1)
	go func() {
		defer close(skip)
		runtime.LockOSThread() // this thread ends with the goroutine, desktop and all
		name, _ := windows.UTF16PtrFromString("wms-dialog-test")
		const desktopAll = 0x01FF
		d, _, err := procCreateDesktopW.Call(uintptr(unsafe.Pointer(name)), 0, 0, 0, desktopAll, 0)
		if d == 0 {
			skip <- "no desktop of its own: " + err.Error()
			return
		}
		defer procCloseDesktop.Call(d)
		if r, _, err := procSetThreadDesktop.Call(d); r == 0 {
			skip <- "can't move to it: " + err.Error()
			return
		}
		if cur, _, _ := procGetThreadDesktop.Call(uintptr(windows.GetCurrentThreadId())); cur != d {
			skip <- "not on the hidden desktop"
			return
		}
		for _, click := range []int{0, 1, 2, 0} {
			testClick = click
			got, err := taskDialog("WittyMediaServer is already running", "It's at http://localhost:9797.", []string{"Open WittyMediaServer", "Start another server…", "Exit"})
			if err != nil {
				t.Errorf("task dialog: %v", err)
				return
			}
			if got != click {
				t.Errorf("clicked %d, got %d", click, got)
			}
		}
		testClick = -1
	}()
	if why := <-skip; why != "" {
		t.Skip(why)
	}
}
