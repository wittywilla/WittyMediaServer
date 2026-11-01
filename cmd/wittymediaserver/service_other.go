//go:build !windows

package main

// runAsService is for Windows, where the service manager starts programs its own way.
func runAsService([]string) (int, bool) { return 0, false }
