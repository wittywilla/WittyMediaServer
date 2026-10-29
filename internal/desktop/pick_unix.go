//go:build !windows && !darwin

package desktop

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// PickFolder uses the desktop's own dialog through zenity (GNOME and most
// others) or kdialog (KDE), whichever is installed.
func PickFolder(ctx context.Context, title string) (string, error) {
	if headless() {
		return "", ErrUnavailable
	}
	home, _ := os.UserHomeDir()
	var cmd *exec.Cmd
	switch {
	case has("zenity"):
		cmd = exec.CommandContext(ctx, "zenity", "--file-selection", "--directory", "--title="+title)
	case has("kdialog"):
		cmd = exec.CommandContext(ctx, "kdialog", "--title", title, "--getexistingdirectory", home)
	default:
		return "", ErrUnavailable
	}
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 { // both tools: 1 = cancelled
			return "", ErrCancelled
		}
		return "", ErrUnavailable
	}
	return strings.TrimSpace(string(out)), nil
}

func has(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}

func headless() bool {
	return os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
}
