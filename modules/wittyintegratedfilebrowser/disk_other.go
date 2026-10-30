//go:build !windows

package wittyintegratedfilebrowser

import "golang.org/x/sys/unix"

// diskInfo: free and total bytes of the file system holding path (0 when
// unknown). Volumes have no separate label here.
func diskInfo(path string) (label string, free, total uint64) {
	var st unix.Statfs_t
	if unix.Statfs(path, &st) != nil {
		return "", 0, 0
	}
	return "", uint64(st.Bavail) * uint64(st.Bsize), uint64(st.Blocks) * uint64(st.Bsize)
}
