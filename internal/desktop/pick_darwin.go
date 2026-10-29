//go:build darwin

package desktop

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// PickFolder shows Finder's folder chooser via AppleScript.
func PickFolder(ctx context.Context, title string) (string, error) {
	if headless() {
		return "", ErrUnavailable
	}
	prompt := strings.NewReplacer(`\`, ``, `"`, `'`).Replace(title)
	out, err := exec.CommandContext(ctx, "osascript", "-e", "activate", "-e", `POSIX path of (choose folder with prompt "`+prompt+`")`).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && strings.Contains(string(ee.Stderr), "-128") { // user cancelled
			return "", ErrCancelled
		}
		return "", ErrUnavailable
	}
	p := strings.TrimSpace(string(out))
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p, nil
}

// headless: no window server when logged in over SSH only.
func headless() bool {
	return os.Getenv("SSH_CONNECTION") != "" && os.Getenv("TERM_PROGRAM") == ""
}
