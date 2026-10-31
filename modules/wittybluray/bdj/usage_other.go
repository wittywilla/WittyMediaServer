//go:build !windows && !linux

package bdj

func usage(pid int) (procUsage, bool) { return usagePS(pid) }
