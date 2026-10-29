// Package desktop talks to the desktop the server runs on (when it has one):
// opening the web app in the default browser and showing the system's own
// folder picker during setup.
package desktop

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
)

var (
	// ErrCancelled means the viewer closed the folder picker without choosing.
	ErrCancelled = errors.New("cancelled")
	// ErrUnavailable means there is no desktop to show a dialog on (service,
	// container, SSH session, no picker tool installed).
	ErrUnavailable = errors.New("no desktop folder picker available")
)

// OpenURL opens u in the default browser.
func OpenURL(u string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		return exec.Command("open", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}

// Headless reports whether this process has no interactive desktop: running
// as a service, in a container, over SSH without a display, or told so with
// WMS_NO_BROWSER=1.
func Headless() bool {
	if os.Getenv("WMS_NO_BROWSER") == "1" {
		return true
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if os.Getenv("container") != "" || os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}
	return headless()
}
