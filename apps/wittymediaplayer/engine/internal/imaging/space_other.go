//go:build !windows

package imaging

// freeSpace isn't checked here yet: a full disk still fails the write plainly.
func freeSpace(string) (uint64, bool) { return 0, false }
