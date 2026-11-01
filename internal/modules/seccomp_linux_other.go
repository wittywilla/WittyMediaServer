//go:build linux && !amd64 && !arm64

package modules

// The 32-bit systems make sockets through one shared call with its arguments in memory,
// which a filter can't read, so they have no filter. A fork doesn't run on them.
const seccompArch = 0

const seccompX32 uint32 = 0

var seccompDeny, seccompSockets []uint32
