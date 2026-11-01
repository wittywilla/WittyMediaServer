//go:build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
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
