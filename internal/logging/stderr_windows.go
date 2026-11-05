//go:build windows

package logging

import (
	"os"

	"golang.org/x/sys/windows"
)

// stderrLost: a service has no standard error at all, and a program started with it
// pointing at NUL loses it too.
func stderrLost() bool {
	h, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	if err != nil || h == 0 || h == windows.InvalidHandle {
		return true
	}
	t, err := windows.GetFileType(h)
	if err != nil || t == windows.FILE_TYPE_UNKNOWN {
		return true
	}
	if t == windows.FILE_TYPE_CHAR {
		var mode uint32
		return windows.GetConsoleMode(h, &mode) != nil // a character device that isn't a console: NUL
	}
	return false
}

// redirectStderr: the runtime looks the handle up each time it writes a crash.
func redirectStderr(f *os.File) error {
	return windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd()))
}
