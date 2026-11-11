// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package main

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

func watchStopEvents() {}

// noteSignal says what a signal most likely means: a service manager or a container
// stopping the server, the system shutting down, or someone pressing Ctrl+C or using kill.
// Go doesn't say which process sent it.
func noteSignal(sig os.Signal) {
	if sig == os.Interrupt {
		logging.NoteStop(logging.StopSignal, "SIGINT (Ctrl+C)", "")
		return
	}
	_, dockerErr := os.Stat("/.dockerenv")
	switch {
	case os.Getenv("INVOCATION_ID") != "":
		if systemStopping() {
			logging.NoteStop(logging.StopShutdown, "systemd: the system is shutting down", "")
			return
		}
		logging.NoteStop(logging.StopService, "systemd", "")
	case dockerErr == nil || os.Getenv("WMS_DOCKER") == "1":
		logging.NoteStop(logging.StopService, "the container was stopped", "")
	case os.Getppid() == 1 && runtime.GOOS == "darwin":
		logging.NoteStop(logging.StopService, "launchd", "")
	case os.Getppid() == 1:
		logging.NoteStop(logging.StopService, "the service manager", "")
	default:
		logging.NoteStop(logging.StopSignal, "SIGTERM", "")
	}
}

// systemStopping asks systemd whether the whole system is on its way down.
func systemStopping() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "systemctl", "is-system-running").Output()
	return strings.TrimSpace(string(out)) == "stopping"
}
