package modules

import (
	"log/slog"
	"os/exec"
)

// SandboxNetworkProgram limits a separate program that has to reach the network: low integrity,
// so it can't write to anything of this account's, and no window. Sockets still work.
func SandboxNetworkProgram(cmd *exec.Cmd, log *slog.Logger) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	prepareSandbox(cmd, true, log)
}
