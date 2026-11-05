//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/wittywilla/wittymediaserver/internal/desktop"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// autostartIn says a hive has it starting at sign-in. For everyone, the installer writes it in
// HKLM, and only Windows' own Settings can turn that off for one person.
func autostartIn(hive registry.Key) bool {
	k, err := registry.OpenKey(hive, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue("WittyMediaServer")
	return err == nil
}

func autostartEnabled() bool {
	return autostartIn(registry.CURRENT_USER) || autostartIn(registry.LOCAL_MACHINE)
}

func setAutostart(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if autostartIn(registry.LOCAL_MACHINE) {
			// Windows Settings, Apps, Startup has the switch for this.
			_ = desktop.OpenURL("ms-settings:startupapps")
		}
		return k.DeleteValue("WittyMediaServer")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue("WittyMediaServer", `"`+exe+`"`)
}

var procGetSystemMetrics = windows.NewLazySystemDLL("user32.dll").NewProc("GetSystemMetrics")

// endReason: Windows closes the tray app when it shuts down or signs out, and when an
// installer's Restart Manager needs its files. The window message that would say which
// isn't passed on by the tray library, so a shutdown is told apart by asking Windows.
func endReason() string {
	const smShuttingDown = 0x2000
	if r, _, _ := procGetSystemMetrics.Call(smShuttingDown); r != 0 {
		return logging.StopShutdown
	}
	return logging.StopTrayClosed
}
