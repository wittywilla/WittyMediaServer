//go:build windows

package libtranscode

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// lowPriority starts FFmpeg below normal priority, so streaming goes first.
func lowPriority(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.BELOW_NORMAL_PRIORITY_CLASS
}

func afterStart(*exec.Cmd, bool) {}

var (
	ntdll            = windows.NewLazySystemDLL("ntdll.dll")
	ntSuspendProcess = ntdll.NewProc("NtSuspendProcess")
	ntResumeProcess  = ntdll.NewProc("NtResumeProcess")
)

// suspend and resume hold a running FFmpeg still, so it gives up the processor at once
// and can carry on exactly where it was.
func suspend(cmd *exec.Cmd) bool { return procCall(cmd, ntSuspendProcess) }

func resume(cmd *exec.Cmd) bool { return procCall(cmd, ntResumeProcess) }

func procCall(cmd *exec.Cmd, p *windows.LazyProc) bool {
	if cmd.Process == nil || p.Find() != nil {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME, false, uint32(cmd.Process.Pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	r, _, _ := p.Call(uintptr(h))
	return r == 0
}
