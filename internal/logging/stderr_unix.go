//go:build unix

package logging

import (
	"os"

	"golang.org/x/sys/unix"
)

// stderrLost: closed, or /dev/null (a launchd job with no StandardErrorPath, say).
func stderrLost() bool {
	var st unix.Stat_t
	if err := unix.Fstat(2, &st); err != nil {
		return true
	}
	var null unix.Stat_t
	if err := unix.Stat(os.DevNull, &null); err != nil {
		return false
	}
	return st.Dev == null.Dev && st.Ino == null.Ino
}

// redirectStderr puts the file where the runtime writes a crash: descriptor 2.
func redirectStderr(f *os.File) error {
	return unix.Dup2(int(f.Fd()), 2)
}
