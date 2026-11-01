//go:build linux

package libtranscode

import (
	"os/exec"
	"syscall"
)

// dieWithServer: a conversion left running by a server that crashed would use the machine
// for hours on a file nobody will look at. Recover deals with what it left behind.
func dieWithServer(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
}
