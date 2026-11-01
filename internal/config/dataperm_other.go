//go:build !windows

package config

// makePrivate: the folder's mode, 0750, already says it.
func makePrivate(string) error { return nil }
