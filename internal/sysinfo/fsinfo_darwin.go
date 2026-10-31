package sysinfo

import (
	"bytes"
	"strings"

	"golang.org/x/sys/unix"
)

// FSInfo finds the file system and kind of drive a folder is on.
func FSInfo(path string) (FS, error) {
	f := FS{Kind: "unknown"}
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return f, err
	}
	str := func(b []byte) string {
		if i := bytes.IndexByte(b, 0); i >= 0 {
			b = b[:i]
		}
		return string(b)
	}
	f.Type = strings.ToLower(str(st.Fstypename[:]))
	f.Mount = str(st.Mntonname[:])
	f.Source = str(st.Mntfromname[:])
	f.ReadOnly = st.Flags&unix.MNT_RDONLY != 0
	switch {
	case networkTypes[f.Type] || st.Flags&unix.MNT_LOCAL == 0:
		f.Kind = "network"
	case f.Type == "cd9660" || f.Type == "udf":
		f.Kind = "optical"
	case strings.HasPrefix(f.Mount, "/Volumes/"):
		f.Kind = "removable" // an outside drive, as far as this can tell
	default:
		f.Kind = "fixed"
	}
	return f, nil
}
