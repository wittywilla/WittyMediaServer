package hostsandbox

import "golang.org/x/sys/windows"

// Both ways a C library finds stdout: asking now, and the number it noted at start,
// which is what the claim closed. Writing to that one may fail, and that is fine.
func printFromC(s string) {
	if h, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE); err == nil {
		_, _ = windows.Write(h, []byte(s))
	}
	_, _ = windows.Write(startupStdout, []byte(s))
}

var startupStdout, _ = windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
