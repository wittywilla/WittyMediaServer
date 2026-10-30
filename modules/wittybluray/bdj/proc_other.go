//go:build !windows

package bdj

import "os/exec"

func hide(c *exec.Cmd) {}
