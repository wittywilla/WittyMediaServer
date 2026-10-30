package server

import "golang.org/x/sys/windows"

// diskInfo: a drive's label and its free and total bytes (0 when unknown).
func diskInfo(root string) (label string, free, total uint64) {
	p, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return "", 0, 0
	}
	var avail, tot, all uint64
	if windows.GetDiskFreeSpaceEx(p, &avail, &tot, &all) == nil {
		free, total = avail, tot
	}
	buf := make([]uint16, 261)
	if windows.GetVolumeInformation(p, &buf[0], uint32(len(buf)), nil, nil, nil, nil, 0) == nil {
		label = windows.UTF16ToString(buf)
	}
	return label, free, total
}
