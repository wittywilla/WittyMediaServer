//go:build !windows

package config

import (
	"os"
	"syscall"
)

// extraLinks says a file has another name too: a hard link someone made to a file of their choosing.
func extraLinks(st os.FileInfo) bool {
	s, ok := st.Sys().(*syscall.Stat_t)
	return ok && s.Nlink > 1
}
