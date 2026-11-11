// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sysinfo

import (
	"encoding/binary"
	"strings"

	"golang.org/x/sys/unix"
)

// ReadCPU reads kern.cp_times: five counters per core (user, nice, system, interrupt, idle).
func ReadCPU() (all CPUTicks, cores []CPUTicks, ok bool) {
	b, err := unix.SysctlRaw("kern.cp_times")
	if err != nil || len(b) < 40 {
		return all, nil, false
	}
	const word = 8 // a long, on the 64-bit systems this runs on
	for off := 0; off+5*word <= len(b); off += 5 * word {
		var t CPUTicks
		for i := range 5 {
			v := binary.LittleEndian.Uint64(b[off+i*word:])
			t.Total += v
			if i != 4 {
				t.Busy += v
			}
		}
		cores = append(cores, t)
		all.Busy += t.Busy
		all.Total += t.Total
	}
	return all, cores, true
}

// ReadMemory reads the page counts.
func ReadMemory() (MemoryUse, bool) {
	total, err := unix.SysctlUint64("hw.physmem")
	if err != nil || total == 0 {
		return MemoryUse{}, false
	}
	m := MemoryUse{Total: total}
	page := uint64(unix.Getpagesize())
	for _, name := range []string{"vm.stats.vm.v_free_count", "vm.stats.vm.v_inactive_count"} {
		if n, err := unix.SysctlUint32(name); err == nil {
			m.Available += uint64(n) * page
		}
	}
	return m, true
}

func fsType(s *unix.Statfs_t) string   { return strings.TrimRight(string(s.Fstypename[:]), "\x00") }
func fsMount(s *unix.Statfs_t) string  { return strings.TrimRight(string(s.Mntonname[:]), "\x00") }
func fsSource(s *unix.Statfs_t) string { return strings.TrimRight(string(s.Mntfromname[:]), "\x00") }

func maxRSSBytes(n int64) uint64 { return uint64(n) * 1024 }
