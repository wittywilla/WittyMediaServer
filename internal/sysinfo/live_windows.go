package sysinfo

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modNtdll     = windows.NewLazySystemDLL("ntdll.dll")
	modIphlpapi  = windows.NewLazySystemDLL("iphlpapi.dll")
	modKernel32  = windows.NewLazySystemDLL("kernel32.dll")
	modPsapi     = windows.NewLazySystemDLL("psapi.dll")
	procNtQuery  = modNtdll.NewProc("NtQuerySystemInformation")
	procIfTable2 = modIphlpapi.NewProc("GetIfTable2")
	procFreeTbl  = modIphlpapi.NewProc("FreeMibTable")
	procHandles  = modKernel32.NewProc("GetProcessHandleCount")
	procMemInfo  = modPsapi.NewProc("GetProcessMemoryInfo")
)

// SystemProcessorPerformanceInformation, one per core. Kernel time includes idle time.
type procPerf struct {
	Idle, Kernel, User, DPC, Interrupt int64
	InterruptCount                     uint32
	_                                  uint32
}

// ReadCPU gives the whole machine and each core.
func ReadCPU() (all CPUTicks, cores []CPUTicks, ok bool) {
	const info = 8
	buf := make([]procPerf, 512)
	var got uint32
	r, _, _ := procNtQuery.Call(info, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))*unsafe.Sizeof(buf[0]), uintptr(unsafe.Pointer(&got)))
	if r != 0 || got == 0 {
		return all, nil, false
	}
	n := int(uintptr(got) / unsafe.Sizeof(buf[0]))
	cores = make([]CPUTicks, n)
	for i := range n {
		total := uint64(buf[i].Kernel + buf[i].User)
		t := CPUTicks{Busy: total - uint64(buf[i].Idle), Total: total}
		cores[i] = t
		all.Busy += t.Busy
		all.Total += t.Total
	}
	return all, cores, true
}

type memoryStatusFull struct {
	Length, Load                                  uint32
	TotalPhys, AvailPhys, TotalPage, AvailPage    uint64
	TotalVirtual, AvailVirtual, AvailExtendedVirt uint64
}

// ReadMemory gives the machine's memory. Windows' page file limit includes physical
// memory, so what's left over is what counts as swap.
func ReadMemory() (MemoryUse, bool) {
	var m memoryStatusFull
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); r == 0 {
		return MemoryUse{}, false
	}
	out := MemoryUse{Total: m.TotalPhys, Available: m.AvailPhys}
	if m.TotalPage > m.TotalPhys {
		out.SwapTotal = m.TotalPage - m.TotalPhys
		used := m.TotalPage - m.AvailPage
		usedPhys := m.TotalPhys - m.AvailPhys
		if used > usedPhys && used-usedPhys < out.SwapTotal {
			out.SwapFree = out.SwapTotal - (used - usedPhys)
		} else {
			out.SwapFree = out.SwapTotal
		}
	}
	return out, true
}

// MIB_IF_ROW2, as laid out on 64-bit and 32-bit Windows alike up to what's read here.
type ifRow2 struct {
	InterfaceLuid        uint64
	InterfaceIndex       uint32
	InterfaceGuid        [16]byte
	Alias                [257]uint16
	Description          [257]uint16
	PhysicalAddressLen   uint32
	PhysicalAddress      [32]byte
	PermanentAddress     [32]byte
	Mtu                  uint32
	Type                 uint32
	TunnelType           uint32
	MediaType            uint32
	PhysicalMediumType   uint32
	AccessType           uint32
	DirectionType        uint32
	InterfaceAndOperFlag uint8
	_                    [3]byte
	OperStatus           uint32
	AdminStatus          uint32
	MediaConnectState    uint32
	NetworkGuid          [16]byte
	ConnectionType       uint32
	_                    uint32
	TransmitLinkSpeed    uint64
	ReceiveLinkSpeed     uint64
	InOctets             uint64
	InUcastPkts          uint64
	InNUcastPkts         uint64
	InDiscards           uint64
	InErrors             uint64
	InUnknownProtos      uint64
	InUcastOctets        uint64
	InMulticastOctets    uint64
	InBroadcastOctets    uint64
	OutOctets            uint64
	OutUcastPkts         uint64
	OutNUcastPkts        uint64
	OutDiscards          uint64
	OutErrors            uint64
	OutUcastOctets       uint64
	OutMulticastOctets   uint64
	OutBroadcastOctets   uint64
	OutQLen              uint64
}

// ReadNetwork gives each real interface: Windows also lists filter drivers and tunnels
// for each adapter, each with the same counters, which would count everything several times.
func ReadNetwork() []NetCounters {
	var table *struct {
		n    uint32
		rows [1]ifRow2 // really n of them
	}
	if r, _, _ := procIfTable2.Call(uintptr(unsafe.Pointer(&table))); r != 0 || table == nil {
		return nil
	}
	defer procFreeTbl.Call(uintptr(unsafe.Pointer(table)))
	rows := unsafe.Slice(&table.rows[0], table.n)
	const (
		ifTypeLoopback = 24
		hardwareFlag   = 1 // first bit of InterfaceAndOperFlags: HardwareInterface
		filterFlag     = 2 // FilterInterface
	)
	var out []NetCounters
	for i := range rows {
		r := &rows[i]
		if r.Type == ifTypeLoopback || r.InterfaceAndOperFlag&hardwareFlag == 0 || r.InterfaceAndOperFlag&filterFlag != 0 {
			continue
		}
		out = append(out, NetCounters{
			Name: windows.UTF16ToString(r.Alias[:]), RxBytes: r.InOctets, TxBytes: r.OutOctets, RxErrs: r.InErrors, TxErrs: r.OutErrors,
			Up: r.OperStatus == 1, SpeedMbps: int64(r.ReceiveLinkSpeed / 1_000_000),
		})
	}
	return out
}

// IOCTL_DISK_PERFORMANCE's answer.
type diskPerformance struct {
	BytesRead, BytesWritten           int64
	ReadTime, WriteTime, IdleTime     int64
	ReadCount, WriteCount, QueueDepth uint32
	SplitCount                        uint32
	QueryTime                         int64
	StorageDeviceNumber               uint32
	StorageManagerName                [8]uint16
}

const ioctlDiskPerformance = 0x70020

func driveLetters() []string {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	var out []string
	for i := range 26 {
		if mask&(1<<i) != 0 {
			out = append(out, string(rune('A'+i))+":")
		}
	}
	return out
}

// ReadDiskIO gives each fixed drive letter. Asking takes no administrator rights.
func ReadDiskIO() []DiskCounters {
	var out []DiskCounters
	for _, d := range driveLetters() {
		root, _ := windows.UTF16PtrFromString(d + `\`)
		if windows.GetDriveType(root) != windows.DRIVE_FIXED {
			continue
		}
		name, _ := windows.UTF16PtrFromString(`\\.\` + d)
		h, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			continue
		}
		var p diskPerformance
		var got uint32
		err = windows.DeviceIoControl(h, ioctlDiskPerformance, nil, 0, (*byte)(unsafe.Pointer(&p)), uint32(unsafe.Sizeof(p)), &got, nil)
		windows.CloseHandle(h)
		if err != nil {
			continue
		}
		out = append(out, DiskCounters{Name: d, ReadBytes: uint64(p.BytesRead), WriteBytes: uint64(p.BytesWritten)})
	}
	return out
}

// ReadVolumes gives each drive that's there. A network drive or an empty card reader
// that doesn't answer is left out rather than waited for.
func ReadVolumes() []Volume {
	var out []Volume
	for _, d := range driveLetters() {
		root := d + `\`
		p, _ := windows.UTF16PtrFromString(root)
		kind := "fixed"
		switch windows.GetDriveType(p) {
		case windows.DRIVE_FIXED:
		case windows.DRIVE_REMOVABLE:
			kind = "removable"
		case windows.DRIVE_REMOTE:
			kind = "network"
		case windows.DRIVE_RAMDISK:
			kind = "ram"
		default:
			continue // no disc in the drive, or nothing there
		}
		var free, total uint64
		if windows.GetDiskFreeSpaceEx(p, &free, &total, nil) != nil {
			continue
		}
		fsName := make([]uint16, 64)
		_ = windows.GetVolumeInformation(p, nil, 0, nil, nil, nil, &fsName[0], uint32(len(fsName)))
		out = append(out, Volume{Mount: root, Device: d, Type: lower(windows.UTF16ToString(fsName)), Kind: kind, Total: total, Free: free})
	}
	return out
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// ReadTemperatures: Windows only tells a program with administrator rights, and then
// often wrongly, so this doesn't ask.
func ReadTemperatures() []Temperature { return nil }

type processMemoryCounters struct {
	Size                       uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// ReadProcess gives this program's own use.
func ReadProcess() (ProcessUse, bool) {
	h := windows.CurrentProcess()
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(h, &created, &exited, &kernel, &user) != nil {
		return ProcessUse{}, false
	}
	ticks := func(f windows.Filetime) float64 {
		return float64(uint64(f.HighDateTime)<<32|uint64(f.LowDateTime)) / 1e7
	}
	p := ProcessUse{CPUSec: ticks(kernel) + ticks(user)}
	var m processMemoryCounters
	m.Size = uint32(unsafe.Sizeof(m))
	if r, _, _ := procMemInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&m)), uintptr(m.Size)); r != 0 {
		p.RSS = uint64(m.WorkingSetSize)
	}
	var n uint32
	if r, _, _ := procHandles.Call(uintptr(h), uintptr(unsafe.Pointer(&n))); r != 0 {
		p.Handles = int(n)
	}
	return p, true
}
