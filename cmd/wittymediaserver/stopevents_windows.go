// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

var procSetConsoleCtrlHandler = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetConsoleCtrlHandler")

// watchStopEvents notes which console event is behind a stop: Go turns Ctrl+C into an
// interrupt, and a closed window, a sign-out and a shutdown all into SIGTERM. This handler
// is asked first and passes each event on, so Go still stops the server as before.
func watchStopEvents() {
	h := windows.NewCallback(func(event uint32) uintptr {
		switch event {
		case windows.CTRL_C_EVENT:
			logging.NoteStop(logging.StopSignal, "Ctrl+C", "")
		case windows.CTRL_BREAK_EVENT:
			logging.NoteStop(logging.StopSignal, "Ctrl+Break", "")
		case windows.CTRL_CLOSE_EVENT:
			logging.NoteStop(logging.StopConsoleClosed, "", "")
		case windows.CTRL_LOGOFF_EVENT:
			if !asService { // a service hears every account sign out, and carries on
				logging.NoteStop(logging.StopLogoff, "", "")
			}
		case windows.CTRL_SHUTDOWN_EVENT:
			logging.NoteStop(logging.StopShutdown, "", "")
		}
		return 0 // not handled here: on to Go's own handler
	})
	_, _, _ = procSetConsoleCtrlHandler.Call(h, 1)
}

// noteSignal is for a stop no console event explained.
func noteSignal(sig os.Signal) {
	if sig == os.Interrupt {
		logging.NoteStop(logging.StopSignal, "Ctrl+C", "")
		return
	}
	logging.NoteStop(logging.StopSignal, "SIGTERM", "")
}
