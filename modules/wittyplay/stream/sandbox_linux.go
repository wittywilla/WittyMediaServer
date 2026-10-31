package stream

import (
	"log/slog"
	"os"
	"os/exec"
	"syscall"
)

func hideWindow(cmd *exec.Cmd) {}

func runnerEnv() []string { return []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent"} }

// prepareSandbox asks for fresh namespaces: no network interfaces except a dead
// loopback, and no sight of other processes' shared memory. They need
// unprivileged user namespaces, which some containers and distributions
// refuse, so availability is checked rather than assumed. The host then adds
// seccomp and Landlock to itself (hostsandbox).
func prepareSandbox(cmd *exec.Cmd, saveDir string, log *slog.Logger) {
	attr := &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	cmd.SysProcAttr = attr
	if !userNamespacesAllowed() {
		log.Info("sandbox: user namespaces are unavailable here; relying on seccomp and Landlock")
		return
	}
	uid, gid := os.Getuid(), os.Getgid()
	attr.Cloneflags = syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET | syscall.CLONE_NEWIPC
	attr.UidMappings = []syscall.SysProcIDMap{{ContainerID: uid, HostID: uid, Size: 1}}
	attr.GidMappings = []syscall.SysProcIDMap{{ContainerID: gid, HostID: gid, Size: 1}}
	attr.GidMappingsEnableSetgroups = false
}

// userNamespacesAllowed tries one, because the settings that control this differ
// between kernels and a container's seccomp profile can refuse it regardless.
func userNamespacesAllowed() bool {
	probe := exec.Command("/proc/self/exe", "-h")
	probe.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWUSER, UidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}}, GidMappingsEnableSetgroups: false}
	if err := probe.Start(); err != nil {
		return false
	}
	_ = probe.Process.Kill()
	_ = probe.Wait()
	return true
}

func lockDown(cmd *exec.Cmd, memoryMB int64, log *slog.Logger) func() { return func() {} }

// SandboxStrength says what boxes a core in on this platform, for people deciding
// whether to allow custom cores.
func SandboxStrength() (string, bool) {
	return "namespaces where allowed, a seccomp filter and Landlock: no network, no new programs, and writing only to its save folder", true
}
