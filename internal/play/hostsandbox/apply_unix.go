//go:build unix && !linux

package hostsandbox

import "syscall"

// Confine has nothing to do here: what Apply sets holds for the whole process.
func Confine(l Limits) error { return nil }

// Apply sets what every Unix has: resource limits and a lower priority.
func Apply(l Limits) error {
	_ = syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{})
	if l.MemoryMB > 0 {
		data := rlimit(l.MemoryMB << 20)
		_ = syscall.Setrlimit(syscall.RLIMIT_DATA, &syscall.Rlimit{Cur: data, Max: data})
		as := rlimit(int64(addressSpace(l.MemoryMB)))
		_ = syscall.Setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{Cur: as, Max: as})
	}
	return syscall.Setpriority(syscall.PRIO_PROCESS, 0, 5)
}
