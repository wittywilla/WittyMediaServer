//go:build !linux

package wittymoonlight

import "syscall"

// dieWithParent: other systems have no simple way to do this, so a server that crashes can
// leave the bridge running on loopback until the machine restarts. docs/GAME-RIGS.md says so.
func dieWithParent() *syscall.SysProcAttr { return nil }
