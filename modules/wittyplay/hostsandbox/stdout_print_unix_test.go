//go:build unix

package hostsandbox

import "syscall"

func printFromC(s string) { _, _ = syscall.Write(1, []byte(s)) }
