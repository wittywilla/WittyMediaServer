// Package sysinfo asks the operating system what a support report needs to know:
// the processor, memory, graphics cards, and how much room a disk has. It needs
// no administrator rights, changes nothing and never fails loudly: what can't be
// found out is left empty.
package sysinfo

import (
	"os"
	"runtime"
)

// Info describes the machine.
type Info struct {
	OS        string   `json:"os"`      // "linux", "windows", "darwin"
	Arch      string   `json:"arch"`    // "amd64", "arm64"
	Release   string   `json:"release"` // the system's own name for its version
	CPU       string   `json:"cpu,omitempty"`
	Cores     int      `json:"cores"`
	MemoryMB  int64    `json:"memoryMB,omitempty"`
	FreeMB    int64    `json:"freeMemoryMB,omitempty"`
	GPUs      []string `json:"gpus,omitempty"`
	Container bool     `json:"container,omitempty"`
}

// Read finds out what it can. It may read a few small files or the registry, but
// starts no programs.
func Read() Info {
	i := Info{OS: runtime.GOOS, Arch: runtime.GOARCH, Cores: runtime.NumCPU()}
	i.Release = osRelease()
	i.CPU = cpuName()
	i.MemoryMB, i.FreeMB = memory()
	i.GPUs = gpus()
	i.Container = inContainer()
	return i
}

func inContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	_, err := os.Stat("/run/.containerenv")
	return err == nil
}

// Disk is how much room a disk has.
type Disk struct {
	Free  int64 `json:"free"` // what this program may use
	Total int64 `json:"total"`
}

// FreeBytes is the room on the disk holding a folder; -1 when unknown.
func FreeBytes(dir string) int64 {
	d, err := DiskUsage(dir)
	if err != nil {
		return -1
	}
	return d.Free
}
