package sysinfo

import "golang.org/x/sys/unix"

func osRelease() string {
	v, err := unix.Sysctl("kern.osrelease")
	if err != nil {
		return ""
	}
	return "FreeBSD " + v
}

func cpuName() string {
	v, err := unix.Sysctl("hw.model")
	if err != nil {
		return ""
	}
	return v
}

func memory() (total, free int64) {
	v, err := unix.SysctlUint64("hw.physmem")
	if err != nil {
		return 0, 0
	}
	return int64(v >> 20), 0
}

// gpus would mean starting a program that may not be there.
func gpus() []string { return nil }
