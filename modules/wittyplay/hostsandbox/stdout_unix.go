//go:build unix

package hostsandbox

import (
	"os"
	"syscall"
)

// ClaimStdout moves the protocol pipe to a descriptor of its own and points stdout
// at stderr. Cores print to stdout, and one printf on the pipe would end the game.
func ClaimStdout() (*os.File, error) {
	syscall.ForkLock.RLock()
	fd, err := syscall.Dup(1)
	if err == nil {
		syscall.CloseOnExec(fd)
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return nil, err
	}
	if err := dupOnto(2, 1); err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "ipc"), nil
}
