package modules

import (
	"log/slog"
	"os"
	"os/exec"
	"syscall"
)

func hideWindow(cmd *exec.Cmd) {}

// prepareSandbox asks for fresh namespaces: no network except a dead loopback,
// and no sight of other processes' shared memory. They need unprivileged user
// namespaces, which some containers and distributions refuse; the caller then
// starts the helper without them.
func prepareSandbox(cmd *exec.Cmd, log *slog.Logger) bool {
	uid, gid := os.Getuid(), os.Getgid()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:                 syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET | syscall.CLONE_NEWIPC,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: uid, HostID: uid, Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: gid, HostID: gid, Size: 1}},
		GidMappingsEnableSetgroups: false,
	}
	return true
}

func lockDown(cmd *exec.Cmd, memoryMB int64, log *slog.Logger) func() { return func() {} }
