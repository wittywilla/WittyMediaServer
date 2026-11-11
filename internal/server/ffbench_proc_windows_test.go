// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The benchmark's view of a process on Windows: a handle kept open from the moment it is
// seen, so its processor time and peak memory can still be read once it has ended.

var benchMemInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

type benchMemCounters struct {
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

type benchProcHandle = windows.Handle

func benchListProcs() []benchProcInfo {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var out []benchProcInfo
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		name := strings.ToLower(strings.TrimSuffix(windows.UTF16ToString(e.ExeFile[:]), ".exe"))
		out = append(out, benchProcInfo{pid: int(e.ProcessID), ppid: int(e.ParentProcessID), name: name})
	}
	return out
}

func benchReadProc(st *benchProcStat) {
	if st.h == 0 {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(st.pid))
		if err != nil {
			return
		}
		st.h = h
	}
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(st.h, &created, &exited, &kernel, &user) == nil {
		ticks := func(f windows.Filetime) float64 {
			return float64(uint64(f.HighDateTime)<<32|uint64(f.LowDateTime)) / 1e7
		}
		st.cpu = ticks(kernel) + ticks(user)
	}
	var m benchMemCounters
	m.Size = uint32(unsafe.Sizeof(m))
	if r, _, _ := benchMemInfo.Call(uintptr(st.h), uintptr(unsafe.Pointer(&m)), uintptr(m.Size)); r != 0 {
		st.peak = max(st.peak, uint64(m.PeakWorkingSetSize))
	}
}

func benchReleaseProc(st *benchProcStat) {
	if st.h != 0 {
		windows.CloseHandle(st.h)
		st.h = 0
	}
}
