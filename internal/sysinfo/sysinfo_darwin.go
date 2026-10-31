package sysinfo

import "golang.org/x/sys/unix"

func osRelease() string {
	v, err := unix.Sysctl("kern.osproductversion")
	if err != nil {
		return ""
	}
	return "macOS " + v
}

func cpuName() string {
	v, err := unix.Sysctl("machdep.cpu.brand_string")
	if err != nil {
		return ""
	}
	return v
}

func memory() (total, free int64) {
	v, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0, 0
	}
	return int64(v >> 20), 0
}

// gpus on a Mac would mean starting a program that takes seconds; the processor
// says as much for Apple's own chips.
func gpus() []string { return nil }
