//go:build !windows

package logging

import (
	"fmt"
	"syscall"
)

func waitReason(ws syscall.WaitStatus) (string, bool) {
	if !ws.Signaled() {
		return "", false
	}
	sig := ws.Signal()
	switch sig {
	case syscall.SIGKILL:
		return "killed (SIGKILL): the server or the system ended it, perhaps because memory ran out", true
	case syscall.SIGSEGV:
		return "crashed: it read or wrote memory it doesn't own (SIGSEGV)", true
	case syscall.SIGILL:
		return "crashed: this processor doesn't have an instruction it uses (SIGILL)", true
	case syscall.SIGABRT:
		return "crashed: it stopped itself after an internal error (SIGABRT)", true
	case syscall.SIGBUS:
		return "crashed: a file it was reading went away or the memory is bad (SIGBUS)", true
	case syscall.SIGTERM:
		return "stopped (SIGTERM)", true
	}
	return fmt.Sprintf("ended by signal %d (%s)", int(sig), sig), true
}
