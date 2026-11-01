package diagnose

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps a console from flashing up on someone's desktop.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
