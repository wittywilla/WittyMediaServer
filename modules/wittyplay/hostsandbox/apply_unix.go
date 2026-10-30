//go:build unix && !linux

package hostsandbox

import "syscall"

// Apply sets what every Unix has: resource limits and a lower priority.
func Apply(l Limits) error {
	_ = syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{})
	if l.MemoryMB > 0 {
		n := rlimit(l.MemoryMB << 20)
		_ = syscall.Setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{Cur: n, Max: n})
	}
	return syscall.Setpriority(syscall.PRIO_PROCESS, 0, 5)
}
