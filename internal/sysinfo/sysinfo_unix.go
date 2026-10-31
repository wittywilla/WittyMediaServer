//go:build !windows

package sysinfo

import "golang.org/x/sys/unix"

// DiskUsage is how much room there is on the disk holding a folder.
func DiskUsage(dir string) (Disk, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return Disk{}, err
	}
	bs := int64(st.Bsize)
	return Disk{Free: int64(st.Bavail) * bs, Total: int64(st.Blocks) * bs}, nil
}
