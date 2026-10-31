package sysinfo

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// DiskUsage is how much room there is on the disk holding a folder.
func DiskUsage(dir string) (Disk, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return Disk{}, err
	}
	var avail, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &total, &free); err != nil {
		return Disk{}, err
	}
	return Disk{Free: int64(avail), Total: int64(total)}, nil
}

func osRelease() string {
	v := windows.RtlGetVersion()
	name := ""
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE); err == nil {
		defer k.Close()
		name, _, _ = k.GetStringValue("ProductName")
	}
	return strings.TrimSpace(fmt.Sprintf("%s %d.%d.%d", name, v.MajorVersion, v.MinorVersion, v.BuildNumber))
}

func cpuName() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	s, _, _ := k.GetStringValue("ProcessorNameString")
	return strings.TrimSpace(s)
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var procGlobalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

func memory() (total, free int64) {
	m := memoryStatusEx{}
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); r == 0 {
		return 0, 0
	}
	return int64(m.TotalPhys >> 20), int64(m.AvailPhys >> 20)
}

// gpus reads the display adapters' names from the registry.
func gpus() []string {
	const class = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, class, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, err := k.ReadSubKeyNames(16)
	if err != nil && len(names) == 0 {
		return nil
	}
	var out []string
	for _, n := range names {
		sub, err := registry.OpenKey(registry.LOCAL_MACHINE, class+`\`+n, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		if d, _, err := sub.GetStringValue("DriverDesc"); err == nil && d != "" {
			if v, _, err := sub.GetStringValue("DriverVersion"); err == nil && v != "" {
				d += " (driver " + v + ")"
			}
			out = append(out, d)
		}
		sub.Close()
	}
	return out
}
