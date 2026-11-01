package hostsandbox

import "golang.org/x/sys/windows"

// Confine has nothing to do here: the server sets the limits from outside.
func Confine(l Limits) error { return nil }

// Apply does little on Windows: the server starts the host in a job object, with a
// low-integrity token, at below-normal priority. A demanding game asks to go back to
// normal priority, which a process may always do for itself.
func Apply(l Limits) error {
	if l.Normal {
		return windows.SetPriorityClass(windows.CurrentProcess(), windows.NORMAL_PRIORITY_CLASS)
	}
	return nil
}
