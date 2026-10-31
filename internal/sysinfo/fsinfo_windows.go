package sysinfo

import (
	"strings"

	"golang.org/x/sys/windows"
)

// FSInfo finds the file system and kind of drive a folder is on.
func FSInfo(path string) (FS, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return FS{Kind: "unknown"}, err
	}
	f := FS{Kind: "unknown", UNC: strings.HasPrefix(path, `\\`)}
	var root [windows.MAX_PATH + 1]uint16
	if err := windows.GetVolumePathName(p, &root[0], uint32(len(root))); err != nil {
		return f, err
	}
	f.Mount = windows.UTF16ToString(root[:])
	switch windows.GetDriveType(&root[0]) {
	case windows.DRIVE_FIXED:
		f.Kind = "fixed"
	case windows.DRIVE_REMOVABLE:
		f.Kind = "removable"
	case windows.DRIVE_REMOTE:
		f.Kind = "network"
		f.Mapped = !f.UNC
	case windows.DRIVE_CDROM:
		f.Kind = "optical"
	case windows.DRIVE_RAMDISK:
		f.Kind = "ram"
	}
	if f.UNC {
		f.Kind = "network"
	}
	var flags uint32
	var name [windows.MAX_PATH + 1]uint16
	if err := windows.GetVolumeInformation(&root[0], nil, 0, nil, nil, &flags, &name[0], uint32(len(name))); err != nil {
		return f, err
	}
	f.Type = strings.ToLower(windows.UTF16ToString(name[:]))
	f.ReadOnly = flags&windows.FILE_READ_ONLY_VOLUME != 0
	return f, nil
}
