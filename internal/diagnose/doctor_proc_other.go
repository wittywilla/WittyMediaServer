//go:build !windows

package diagnose

import "os/exec"

func hideWindow(*exec.Cmd) {}
