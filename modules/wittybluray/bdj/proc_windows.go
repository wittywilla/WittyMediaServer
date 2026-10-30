package bdj

import (
	"os/exec"
	"syscall"
)

// hide keeps a console window from appearing for the Java process.
func hide(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} }
