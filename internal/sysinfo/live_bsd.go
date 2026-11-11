// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build darwin || freebsd

package sysinfo

import (
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// macOS and FreeBSD: what a system call gives without cgo or a program to run. The
// statistics say which readings aren't there rather than showing a zero. Traffic per
// interface and disk speeds would need more than that, so they aren't offered yet.

// ReadNetwork names the interfaces, with no counters.
func ReadNetwork() []NetCounters {
	list, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []NetCounters
	for _, i := range list {
		if i.Flags&net.FlagLoopback != 0 {
			continue
		}
		out = append(out, NetCounters{Name: i.Name, Up: i.Flags&net.FlagUp != 0})
	}
	return out
}

// ReadDiskIO isn't available here.
func ReadDiskIO() []DiskCounters { return nil }

// ReadTemperatures isn't available here.
func ReadTemperatures() []Temperature { return nil }

func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// ReadVolumes gives each mounted file system with space of its own.
func ReadVolumes() []Volume {
	n, err := unix.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil || n == 0 {
		return nil
	}
	list := make([]unix.Statfs_t, n)
	if n, err = unix.Getfsstat(list, unix.MNT_NOWAIT); err != nil {
		return nil
	}
	skip := map[string]bool{"devfs": true, "autofs": true, "tmpfs": true, "fdescfs": true, "procfs": true, "linprocfs": true, "linsysfs": true, "nullfs": true, "map": true}
	var out []Volume
	for _, s := range list[:n] {
		t := fsType(&s)
		if skip[t] || s.Blocks == 0 {
			continue
		}
		kind := "fixed"
		if networkTypes[t] {
			kind = "network"
		}
		bs := uint64(s.Bsize)
		out = append(out, Volume{Mount: fsMount(&s), Device: fsSource(&s), Type: t, Kind: kind, Total: uint64(s.Blocks) * bs, Free: uint64(s.Bavail) * bs})
	}
	return out
}

// ReadProcess gives this program's own use. RSS is in bytes on macOS and in KB on FreeBSD:
// see maxRSSBytes.
func ReadProcess() (ProcessUse, bool) {
	var ru syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &ru) != nil {
		return ProcessUse{}, false
	}
	return ProcessUse{
		CPUSec: float64(ru.Utime.Sec) + float64(ru.Utime.Usec)/1e6 + float64(ru.Stime.Sec) + float64(ru.Stime.Usec)/1e6,
		RSS:    maxRSSBytes(ru.Maxrss),
	}, true
}
