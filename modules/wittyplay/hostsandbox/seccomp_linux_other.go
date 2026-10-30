//go:build linux && !amd64 && !arm64

package hostsandbox

// On 32-bit Linux the network goes through one multiplexed call, and the filter
// isn't worth the risk untested. These hosts still get the other layers.
func seccomp() error { return nil }
