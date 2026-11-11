// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows && !darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

func hideWindow(cmd *exec.Cmd) {}

func desktopPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		h, _ := os.UserHomeDir()
		base = filepath.Join(h, ".config")
	}
	return filepath.Join(base, "autostart", "wittymediaserver.desktop")
}

func autostartEnabled() bool { _, err := os.Stat(desktopPath()); return err == nil }

func setAutostart(on bool) error {
	p := desktopPath()
	if !on {
		return os.Remove(p)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	entry := "[Desktop Entry]\nType=Application\nName=WittyMediaServer\nComment=Media server tray launcher\nExec=" + exe + "\nIcon=wittymediaserver\nTerminal=false\nCategories=AudioVideo;\nX-GNOME-Autostart-enabled=true\n"
	return os.WriteFile(p, []byte(entry), 0o644)
}

// serviceInstalled says a package or install.sh set up the background service. Only then is a
// server found by asking on this computer one this app shows, rather than a second server of its own.
func serviceInstalled() bool {
	for _, p := range []string{"/etc/systemd/system/multi-user.target.wants/wittymediaserver.service", "/usr/local/etc/rc.d/wittymediaserver"} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// endReason: the desktop closed the tray app without its Quit. A shutdown also stops the
// server itself, which says so.
func endReason() string { return logging.StopTrayClosed }
