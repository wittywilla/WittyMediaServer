// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sysinfo

import (
	"encoding/binary"

	"golang.org/x/sys/unix"
)

// ReadCPU isn't available: macOS gives per-core times only through a call that needs cgo.
func ReadCPU() (all CPUTicks, cores []CPUTicks, ok bool) { return all, nil, false }

// ReadMemory gives the machine's memory. How much of it is free takes a call that needs
// cgo, so Available is what the page counts say and may be low: macOS uses spare memory.
func ReadMemory() (MemoryUse, bool) {
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil || total == 0 {
		return MemoryUse{}, false
	}
	m := MemoryUse{Total: total}
	if free, err := unix.SysctlUint32("vm.page_free_count"); err == nil {
		m.Available = uint64(free) * uint64(unix.Getpagesize())
		if pur, err := unix.SysctlUint32("vm.page_purgeable_count"); err == nil {
			m.Available += uint64(pur) * uint64(unix.Getpagesize())
		}
	}
	// xsw_usage: total, available, used as uint64s, then a page size and a flag.
	if b, err := unix.SysctlRaw("vm.swapusage"); err == nil && len(b) >= 16 {
		m.SwapTotal = binary.LittleEndian.Uint64(b[0:8])
		m.SwapFree = binary.LittleEndian.Uint64(b[8:16])
	}
	return m, true
}

func fsType(s *unix.Statfs_t) string   { return cString(int8sToBytes(s.Fstypename[:])) }
func fsMount(s *unix.Statfs_t) string  { return cString(int8sToBytes(s.Mntonname[:])) }
func fsSource(s *unix.Statfs_t) string { return cString(int8sToBytes(s.Mntfromname[:])) }

func int8sToBytes(in []byte) []byte { return in }

func maxRSSBytes(n int64) uint64 { return uint64(n) }
