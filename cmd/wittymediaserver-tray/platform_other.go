//go:build !windows && !darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
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
