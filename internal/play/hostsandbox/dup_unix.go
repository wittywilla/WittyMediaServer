//go:build unix && !darwin

package hostsandbox

import "golang.org/x/sys/unix"

func dupOnto(from, to int) error { return unix.Dup3(from, to, 0) }
