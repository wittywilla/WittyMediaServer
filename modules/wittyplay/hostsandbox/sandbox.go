// Package hostsandbox is what the host does to itself before it loads a core.
// The server's part (job objects, namespaces, sandbox profiles) is in the stream
// package, and docs/GAMING.md lists what each platform gets.
package hostsandbox

import "unsafe"

// Limits are the boundaries a host sets for itself.
type Limits struct {
	SaveDir  string // the only folder it may write to
	MemoryMB int64  // memory it may use; 0 for no limit
}

// maxFileSize stops a core filling the disk from its save folder. The largest thing
// it has any reason to write is a memory card or a battery save.
const maxFileSize = 1 << 30

// addressSpace is the ceiling on address space for a memory limit in MiB. It has to
// be far above the memory itself. Address space is mostly empty reservations: a real
// core that held 30 MB had reserved 1.9 GB, between the Go runtime and one C library
// heap per thread. The limit on what is really used is the data limit.
func addressSpace(memoryMB int64) uint64 {
	n := uint64(memoryMB) << 20
	if unsafe.Sizeof(uintptr(0)) < 8 {
		return n // a 32-bit process has little to reserve from
	}
	if ceiling := n * 8; ceiling > 16<<30 {
		return ceiling
	}
	return 16 << 30
}
