//go:build !windows

package gamearchive

import (
	"errors"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// confine has nothing to add to what the sandbox package did.
func confine(int64) {}

func workerEnv() []string { return []string{"GOTRACEBACK=none"} }

func hideWindow(*exec.Cmd) {}

func isNoSpace(err error) bool { return errors.Is(err, syscall.ENOSPC) }

func freeBytes(path string) int64 {
	var st unix.Statfs_t
	if unix.Statfs(path, &st) != nil {
		return -1
	}
	return int64(uint64(st.Bavail) * uint64(st.Bsize))
}
