package sysinfo

// What the machine is doing right now, for the statistics. Each reading is a small
// file or a system call, and starts no programs. Counters only ever go up: someone
// who wants a speed reads twice and divides. What a system won't say comes back empty
// or with ok false, and the app says it isn't available there rather than showing a zero.

// CPUTicks is how much a processor has done since the machine started, in whatever
// unit its system counts. Only the difference between two readings means anything.
type CPUTicks struct {
	Busy, Total uint64
}

// MemoryUse is the machine's memory, in bytes.
type MemoryUse struct {
	Total, Available, SwapTotal, SwapFree uint64
}

// NetCounters is what one network interface has carried.
type NetCounters struct {
	Name             string
	RxBytes, TxBytes uint64
	RxErrs, TxErrs   uint64
	Up               bool
	SpeedMbps        int64 // 0: not known
}

// DiskCounters is what one disk has read and written. Name is the system's: sda, nvme0n1, C:.
type DiskCounters struct {
	Name                  string
	ReadBytes, WriteBytes uint64
	BusyMs                uint64 // time it had something to do; 0 where a system doesn't say
}

// Volume is somewhere files can go.
type Volume struct {
	Mount  string
	Device string // the system's name for what it's on, to match with DiskCounters
	Type   string // ntfs, ext4...
	Kind   string // fixed, removable, network...
	Total  uint64
	Free   uint64
}

// Temperature is one sensor's reading.
type Temperature struct {
	Name    string
	Celsius float64
}

// ProcessUse is this program's own share of the machine.
type ProcessUse struct {
	CPUSec  float64 // processor time used since it started
	RSS     uint64  // memory the system says it holds
	Threads int     // 0: not known
	Handles int     // open files and such; 0: not known
}
