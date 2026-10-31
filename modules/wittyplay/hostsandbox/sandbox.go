// Package hostsandbox is what the host does to itself before it loads a core.
// The server's part (job objects, namespaces, sandbox profiles) is in the stream
// package, and docs/GAMING.md lists what each platform gets.
package hostsandbox

// Limits are the boundaries a host sets for itself.
type Limits struct {
	SaveDir  string // the only folder it may write to
	MemoryMB int64  // address space limit; 0 for none
	GPU      bool   // a graphics driver will load: see docs/GAMING.md for what that opens
}
