package sysinfo

import (
	"path/filepath"
	"strings"
)

// FS describes the file system a folder is on, as far as the operating system
// says without administrator rights. Whatever it won't say stays empty.
type FS struct {
	Type     string `json:"type,omitempty"` // lower case: ntfs, exfat, fat32, refs, ext4, xfs, nfs, cifs, apfs...
	Kind     string `json:"kind"`           // fixed, removable, sdcard, network, optical, ram or unknown
	ReadOnly bool   `json:"readOnly,omitempty"`
	Mount    string `json:"mount,omitempty"`  // the drive's root or the mount point
	Source   string `json:"source,omitempty"` // Linux and macOS: the device or share it comes from
	// Mapped: a drive letter for a network share. Services don't see those.
	Mapped bool `json:"mapped,omitempty"`
	// UNC: named as \\server\share.
	UNC bool `json:"unc,omitempty"`
}

// Network says the file system is on another machine (or a share of the host,
// from a virtual machine or a container): locks and speed work differently.
func (f FS) Network() bool { return f.Kind == "network" }

// networkTypes are file system names that mean somewhere else.
var networkTypes = map[string]bool{
	"nfs": true, "nfs4": true, "cifs": true, "smb": true, "smbfs": true, "smb3": true, "9p": true, "afs": true,
	"ceph": true, "glusterfs": true, "virtiofs": true, "davfs": true, "webdav": true, "afpfs": true, "fuse.sshfs": true,
	"fuse.rclone": true, "fuse.gvfsd-fuse": true, "fuse.smbnetfs": true, "fuse.s3fs": true,
}

// MountLine is one line of /proc/self/mountinfo.
type MountLine struct {
	Mount, Type, Source string
	ReadOnly            bool
}

// FindMount picks the mount that holds a path from the text of
// /proc/self/mountinfo: the longest mount point that is the path or a folder
// above it. The last line wins among equals: a later mount hides an earlier one.
func FindMount(mountinfo, path string) (MountLine, bool) {
	path = filepath.ToSlash(filepath.Clean(path))
	var best MountLine
	found := false
	for _, line := range strings.Split(mountinfo, "\n") {
		left, right, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		l, r := strings.Fields(left), strings.Fields(right)
		if len(l) < 6 || len(r) < 2 {
			continue
		}
		mount := unescapeMount(l[4])
		if !underMount(mount, path) || (found && len(mount) < len(best.Mount)) {
			continue
		}
		best = MountLine{Mount: mount, Type: strings.ToLower(r[0]), Source: unescapeMount(r[1])}
		for _, o := range strings.Split(l[5], ",") {
			if o == "ro" {
				best.ReadOnly = true
			}
		}
		if len(r) >= 3 {
			for _, o := range strings.Split(r[2], ",") {
				if o == "ro" {
					best.ReadOnly = true
				}
			}
		}
		found = true
	}
	return best, found
}

func underMount(mount, path string) bool {
	if mount == "/" || mount == path {
		return true
	}
	return strings.HasPrefix(path, strings.TrimSuffix(mount, "/")+"/")
}

// unescapeMount undoes the octal escapes mountinfo uses for spaces and the like.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			n := 0
			ok := true
			for _, c := range []byte(s[i+1 : i+4]) {
				if c < '0' || c > '7' {
					ok = false
					break
				}
				n = n*8 + int(c-'0')
			}
			if ok {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
