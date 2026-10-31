package modules

import (
	"log/slog"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow | windows.BELOW_NORMAL_PRIORITY_CLASS}
}

// prepareSandbox gives a helper a low-integrity token: it can read what the
// account can read, and can't write to anything of the account's. If that fails
// (older Windows, unusual accounts) the job in lockDown still applies.
func prepareSandbox(cmd *exec.Cmd, network bool, log *slog.Logger) bool {
	hideWindow(cmd)
	tok, err := lowIntegrityToken()
	if err != nil {
		log.Info("modules: no low-integrity token", "err", err)
		return false
	}
	cmd.SysProcAttr.Token = syscall.Token(tok)
	return true
}

func lowIntegrityToken() (windows.Token, error) {
	var self windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ASSIGN_PRIMARY|windows.TOKEN_ADJUST_DEFAULT, &self); err != nil {
		return 0, err
	}
	defer self.Close()
	var tok windows.Token
	if err := windows.DuplicateTokenEx(self, windows.TOKEN_ALL_ACCESS, nil, windows.SecurityImpersonation, windows.TokenPrimary, &tok); err != nil {
		return 0, err
	}
	sid, err := windows.CreateWellKnownSid(windows.WinLowLabelSid)
	if err != nil {
		tok.Close()
		return 0, err
	}
	label := windows.Tokenmandatorylabel{Label: windows.SIDAndAttributes{Sid: sid, Attributes: windows.SE_GROUP_INTEGRITY}}
	size := uint32(unsafe.Sizeof(label)) + windows.GetLengthSid(sid)
	if err := windows.SetTokenInformation(tok, windows.TokenIntegrityLevel, (*byte)(unsafe.Pointer(&label)), size); err != nil {
		tok.Close()
		return 0, err
	}
	return tok, nil
}

// lockDown puts a started helper in a job: it dies with the server, has a memory
// cap and can't start other programs. It does nothing until it is called, so
// nothing has run outside the job.
func lockDown(cmd *exec.Cmd, memoryMB int64, log *slog.Logger) func() {
	if t := cmd.SysProcAttr.Token; t != 0 {
		_ = windows.Token(t).Close()
		cmd.SysProcAttr.Token = 0
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		log.Warn("modules: no job object", "err", err)
		return func() {}
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY |
		windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS | windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION
	info.BasicLimitInformation.ActiveProcessLimit = 1
	info.ProcessMemoryLimit = uintptr(memoryMB) << 20
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		log.Warn("modules: job limits", "err", err)
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		log.Warn("modules: opening the helper", "err", err)
		_ = windows.CloseHandle(job)
		return func() {}
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		log.Warn("modules: assigning the job", "err", err)
	}
	return func() { _ = windows.CloseHandle(job) }
}
