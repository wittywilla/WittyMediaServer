//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach lets the restarted server outlive this process.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
