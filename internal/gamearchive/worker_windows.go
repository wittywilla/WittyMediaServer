package gamearchive

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// confine does on Windows what the sandbox package does elsewhere: a job with a memory
// limit that allows no child processes, and a lower priority. The job ends with us.
func confine(memoryMB int64) {
	self := windows.CurrentProcess()
	_ = windows.SetPriorityClass(self, windows.BELOW_NORMAL_PRIORITY_CLASS)
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY | windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
	info.BasicLimitInformation.ActiveProcessLimit = 1
	info.ProcessMemoryLimit = uintptr(memoryMB) << 20
	_, _ = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	_ = windows.AssignProcessToJobObject(job, self)
}

func workerEnv() []string {
	return []string{"GOTRACEBACK=none", "SystemRoot=" + os.Getenv("SystemRoot")}
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

func isNoSpace(err error) bool {
	return errors.Is(err, windows.ERROR_DISK_FULL) || errors.Is(err, windows.ERROR_HANDLE_DISK_FULL)
}

func freeBytes(path string) int64 {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return -1
	}
	var avail, total, all uint64
	if windows.GetDiskFreeSpaceEx(p, &avail, &total, &all) != nil {
		return -1
	}
	return int64(avail)
}
