package wittymoonlight

import "syscall"

// dieWithParent ends the bridge when the server ends, however it ends.
func dieWithParent() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
