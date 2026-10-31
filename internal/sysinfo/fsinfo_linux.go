package sysinfo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// FSInfo finds the file system and kind of drive a folder is on, from the
// mount table and /sys.
func FSInfo(path string) (FS, error) {
	f := FS{Kind: "unknown"}
	b, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return f, err
	}
	m, ok := FindMount(string(b), path)
	if !ok {
		return f, errors.New("not in the mount table")
	}
	f.Type, f.Mount, f.Source, f.ReadOnly = m.Type, m.Mount, m.Source, m.ReadOnly
	switch dev := BlockDevice(m.Source); {
	case networkTypes[m.Type]:
		f.Kind = "network"
	case m.Type == "tmpfs" || m.Type == "ramfs":
		f.Kind = "ram"
	case dev != "":
		f.Kind = "fixed"
		if strings.HasPrefix(dev, "mmcblk") {
			f.Kind = "sdcard"
		} else if b, err := os.ReadFile(filepath.Join("/sys/block", dev, "removable")); err == nil && strings.TrimSpace(string(b)) == "1" {
			f.Kind = "removable"
		}
		if strings.HasPrefix(dev, "sr") {
			f.Kind = "optical"
		}
	}
	return f, nil
}

// BlockDevice names the whole disk under /sys/block behind a mount's source
// (/dev/sda1 is sda; /dev/mapper/x is what the mapping sits on). "" when the
// source is not a local disk.
func BlockDevice(source string) string {
	if !strings.HasPrefix(source, "/dev/") {
		return ""
	}
	real, err := filepath.EvalSymlinks(source)
	if err != nil {
		real = source
	}
	name := filepath.Base(real)
	for range 4 {
		if _, err := os.Stat(filepath.Join("/sys/block", name)); err == nil {
			// A mapped device: follow it down to the disk it uses.
			slaves, _ := os.ReadDir(filepath.Join("/sys/block", name, "slaves"))
			if len(slaves) == 0 {
				return name
			}
			name = slaves[0].Name()
			continue
		}
		// A partition: its folder in /sys/class/block sits inside the disk's.
		link, err := filepath.EvalSymlinks(filepath.Join("/sys/class/block", name))
		if err != nil {
			return ""
		}
		name = filepath.Base(filepath.Dir(link))
	}
	return ""
}
