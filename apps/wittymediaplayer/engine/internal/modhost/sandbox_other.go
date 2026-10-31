//go:build !windows

package modhost

import "os/exec"

// The limits on other systems come with their releases.
func prepareSandbox(*exec.Cmd) {}

func lockDown(*exec.Cmd) func() { return func() {} }
