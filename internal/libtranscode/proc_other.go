//go:build !linux

package libtranscode

import "os/exec"

// dieWithServer has no equivalent here. A conversion left running by a crashed server ends
// when it ends, writing to a temporary file that Recover has already removed.
func dieWithServer(*exec.Cmd) {}
