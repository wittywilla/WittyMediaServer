package bdj

import (
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

// hide keeps a console window from appearing for the Java process. It keeps normal
// priority: Windows gives a lower one almost nothing while anything else is busy,
// so the menu would stall whenever FFmpeg transcodes, which is exactly when a menu
// with moving video runs. The limits in limits.go deal with a disc that spins.
func hide(c *exec.Cmd) {
	const noWindow = 0x08000000
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: noWindow}
}

func javaCommand(path string, args ...string) *exec.Cmd { return exec.Command(path, args...) }

var memoryInfo = syscall.NewLazyDLL("kernel32.dll").NewProc("K32GetProcessMemoryInfo")

type processMemory struct {
	size, faults                 uint32
	peakWorking, working         uintptr
	peakPagedPool, pagedPool     uintptr
	peakNonPaged, nonPaged       uintptr
	pagefile, peakPagefile, priv uintptr
}

// usage is what a process holds and has used.
func usage(pid int) (u procUsage, ok bool) {
	const queryLimited = 0x1000
	h, err := syscall.OpenProcess(queryLimited, false, uint32(pid))
	if err != nil {
		return u, false
	}
	defer syscall.CloseHandle(h)
	var m processMemory
	m.size = uint32(unsafe.Sizeof(m))
	if r, _, _ := memoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&m)), uintptr(m.size)); r != 0 {
		// Committed memory, not just what is in RAM: a disc must not get round
		// the limit by having its allocations paged out.
		u.memory = uint64(m.working)
		if uint64(m.pagefile) > u.memory {
			u.memory = uint64(m.pagefile)
		}
	}
	var created, exited, kernel, user syscall.Filetime
	if syscall.GetProcessTimes(h, &created, &exited, &kernel, &user) == nil {
		u.cpu = filetime(kernel) + filetime(user)
	}
	u.threads = threadCount(uint32(pid))
	return u, true
}

func filetime(f syscall.Filetime) time.Duration {
	return time.Duration(uint64(f.HighDateTime)<<32|uint64(f.LowDateTime)) * 100
}

func threadCount(pid uint32) int {
	s, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer syscall.CloseHandle(s)
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = syscall.Process32First(s, &e); err == nil; err = syscall.Process32Next(s, &e) {
		if e.ProcessID == pid {
			return int(e.Threads)
		}
	}
	return 0
}
