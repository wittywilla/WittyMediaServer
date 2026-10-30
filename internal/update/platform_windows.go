//go:build windows

package update

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// UninstallKey is the per-user "Installed apps" entry the Windows setup
// writes.
const UninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\WittyMediaServer`

// noteInstalled updates the version shown in Installed apps when this copy
// was installed by the Windows setup.
func noteInstalled(dir, ver string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, UninstallKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	loc, _, err := k.GetStringValue("InstallLocation")
	if err != nil || !strings.EqualFold(filepath.Clean(realPath(loc)), filepath.Clean(dir)) {
		return
	}
	_ = k.SetStringValue("DisplayVersion", ver)
}
