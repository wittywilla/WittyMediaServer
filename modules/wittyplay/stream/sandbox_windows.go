package stream

import (
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// runnerEnv is the little Windows needs to load libraries.
func runnerEnv() []string {
	return []string{"SystemRoot=" + os.Getenv("SystemRoot")}
}

// prepareSandbox gives the runner a low-integrity token: it can read what the
// account can read, but can only write where a low-integrity label allows, which
// is its save folder. If that fails (older Windows, unusual accounts) the job
// object below still applies.
func prepareSandbox(cmd *exec.Cmd, saveDir string, log *slog.Logger) {
	attr := &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow | windows.BELOW_NORMAL_PRIORITY_CLASS}
	cmd.SysProcAttr = attr
	if err := labelLowIntegrity(saveDir); err != nil {
		log.Warn("sandbox: the save folder could not be labelled, so the runner keeps normal integrity", "err", err)
		return
	}
	tok, err := lowIntegrityToken()
	if err != nil {
		log.Warn("sandbox: no low-integrity token", "err", err)
		return
	}
	attr.Token = syscall.Token(tok)
}

func labelLowIntegrity(path string) error {
	sd, err := windows.SecurityDescriptorFromString("S:(ML;OICI;NW;;;LW)")
	if err != nil {
		return err
	}
	sacl, _, err := sd.SACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.LABEL_SECURITY_INFORMATION, nil, nil, nil, sacl)
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

// lockDown puts the started runner in a job: it dies with the server, can't use
// more than its memory allowance and can't start other programs. The runner
// loads nothing until told to, so nothing has run outside the job.
func lockDown(cmd *exec.Cmd, memoryMB int64, log *slog.Logger) func() {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		log.Warn("sandbox: no job object", "err", err)
		return func() {}
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY |
		windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS | windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION
	info.BasicLimitInformation.ActiveProcessLimit = 1
	info.ProcessMemoryLimit = uintptr(memoryMB) << 20
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		log.Warn("sandbox: job limits", "err", err)
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		log.Warn("sandbox: opening the runner", "err", err)
		_ = windows.CloseHandle(job)
		return func() {}
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		log.Warn("sandbox: assigning the job", "err", err)
	}
	return func() { _ = windows.CloseHandle(job) }
}

// SandboxStrength says what boxes a core in on this platform, for people deciding
// whether to allow custom cores.
func SandboxStrength() (string, bool) {
	return "a job object and a low-integrity token: no child processes, a memory limit, and writing only to its save folder", true
}

// probePriority takes a test-load out of the background. A game runs below normal so
// it never starves a film, but someone is waiting on this for a few seconds, and on a
// busy machine a low-priority process can take longer than the time limit to load.
func probePriority(cmd *exec.Cmd) {
	if cmd.SysProcAttr != nil {
		cmd.SysProcAttr.CreationFlags &^= windows.BELOW_NORMAL_PRIORITY_CLASS
	}
}
