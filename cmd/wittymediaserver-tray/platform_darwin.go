//go:build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

func hideWindow(cmd *exec.Cmd) {}

func agentPath() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Library", "LaunchAgents", "com.wittywilla.wittymediaserver.tray.plist")
}

func autostartEnabled() bool { _, err := os.Stat(agentPath()); return err == nil }

func setAutostart(on bool) error {
	p := agentPath()
	if !on {
		_ = exec.Command("launchctl", "unload", p).Run()
		return os.Remove(p)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>com.wittywilla.wittymediaserver.tray</string>
  <key>ProgramArguments</key><array><string>` + exe + `</string></array>
  <key>RunAtLoad</key><true/>
</dict></plist>
`
	if err := os.WriteFile(p, []byte(plist), 0o644); err != nil {
		return err
	}
	return exec.Command("launchctl", "load", p).Run()
}

// serviceInstalled says the installer set up the background service. Only then is a server found
// by asking on this computer one this app shows, rather than a second server of its own.
func serviceInstalled() bool {
	_, err := os.Stat("/Library/LaunchDaemons/com.wittywilla.wittymediaserver.plist")
	return err == nil
}

// endReason: the desktop closed the tray app without its Quit. A shutdown also stops the
// server itself, which says so.
func endReason() string { return logging.StopTrayClosed }
