//go:build windows

package modhost

import (
	"os/exec"
	"syscall"
	"unsafe"
)

// Straight system calls: the usual Go package for these brings in network code, and this
// program has none.
var (
	advapi32                = syscall.NewLazyDLL("advapi32.dll")
	procDuplicateTokenEx    = advapi32.NewProc("DuplicateTokenEx")
	procCreateWellKnownSid  = advapi32.NewProc("CreateWellKnownSid")
	procGetLengthSid        = advapi32.NewProc("GetLengthSid")
	procSetTokenInformation = advapi32.NewProc("SetTokenInformation")
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW    = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJob   = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJob  = kernel32.NewProc("AssignProcessToJobObject")
)

const (
	createNoWindow      = 0x08000000
	belowNormalPriority = 0x00004000

	tokenDuplicate        = 0x0002
	tokenAssignPrimary    = 0x0001
	tokenQuery            = 0x0008
	tokenAdjustDefault    = 0x0080
	tokenAllAccess        = 0x000F01FF
	securityImpersonation = 2
	tokenPrimary          = 1
	tokenIntegrityLevel   = 25
	winLowLabelSid        = 66
	seGroupIntegrity      = 0x00000020

	jobLimitActiveProcess  = 0x00000008
	jobLimitProcessMemory  = 0x00000100
	jobLimitDieOnException = 0x00000400
	jobLimitKillOnClose    = 0x00002000
	jobExtendedLimitInfo   = 9
	processSetQuota        = 0x0100
	processTerminate       = 0x0001
)

// helperMemoryMB is more than a Blu-ray's menus need.
const helperMemoryMB = 2048

// prepareSandbox gives a helper a low-integrity token: it can read what the account can
// read, which it needs for discs, and can't write to anything of the account's. If that
// fails (older Windows, unusual accounts) the job in lockDown still applies.
func prepareSandbox(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow | belowNormalPriority}
	if tok, ok := lowIntegrityToken(); ok {
		cmd.SysProcAttr.Token = tok
	}
}

type sidAndAttributes struct {
	sid        uintptr
	attributes uint32
}

func lowIntegrityToken() (syscall.Token, bool) {
	self, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, false
	}
	var own syscall.Token
	if err := syscall.OpenProcessToken(self, tokenDuplicate|tokenQuery|tokenAssignPrimary|tokenAdjustDefault, &own); err != nil {
		return 0, false
	}
	defer own.Close()
	var tok syscall.Token
	if r, _, _ := procDuplicateTokenEx.Call(uintptr(own), tokenAllAccess, 0, securityImpersonation, tokenPrimary, uintptr(unsafe.Pointer(&tok))); r == 0 {
		return 0, false
	}
	sid := make([]byte, 68)
	size := uint32(len(sid))
	if r, _, _ := procCreateWellKnownSid.Call(winLowLabelSid, 0, uintptr(unsafe.Pointer(&sid[0])), uintptr(unsafe.Pointer(&size))); r == 0 {
		tok.Close()
		return 0, false
	}
	label := sidAndAttributes{sid: uintptr(unsafe.Pointer(&sid[0])), attributes: seGroupIntegrity}
	n, _, _ := procGetLengthSid.Call(uintptr(unsafe.Pointer(&sid[0])))
	if r, _, _ := procSetTokenInformation.Call(uintptr(tok), tokenIntegrityLevel, uintptr(unsafe.Pointer(&label)), unsafe.Sizeof(label)+n); r == 0 {
		tok.Close()
		return 0, false
	}
	return tok, true
}

// The system's JOBOBJECT_EXTENDED_LIMIT_INFORMATION.
type jobLimits struct {
	perProcessTime, perJobTime                               int64
	limitFlags                                               uint32
	minWorkingSet, maxWorkingSet                             uintptr
	activeProcessLimit                                       uint32
	affinity                                                 uintptr
	priorityClass, schedulingClass                           uint32
	io                                                       [6]uint64
	processMemoryLimit, jobMemoryLimit, peakProcess, peakJob uintptr
}

// lockDown puts a started helper in a job: it dies with the player, has a memory cap and
// can't start other programs.
func lockDown(cmd *exec.Cmd) func() {
	if t := cmd.SysProcAttr.Token; t != 0 {
		_ = t.Close()
		cmd.SysProcAttr.Token = 0
	}
	job, _, _ := procCreateJobObjectW.Call(0, 0)
	if job == 0 {
		return func() {}
	}
	info := jobLimits{limitFlags: jobLimitKillOnClose | jobLimitProcessMemory | jobLimitActiveProcess | jobLimitDieOnException, activeProcessLimit: 1, processMemoryLimit: helperMemoryMB << 20}
	procSetInformationJob.Call(job, jobExtendedLimitInfo, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
	h, err := syscall.OpenProcess(processSetQuota|processTerminate, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = syscall.CloseHandle(syscall.Handle(job))
		return func() {}
	}
	defer syscall.CloseHandle(h)
	procAssignProcessToJob.Call(job, uintptr(h))
	return func() { _ = syscall.CloseHandle(syscall.Handle(job)) }
}
