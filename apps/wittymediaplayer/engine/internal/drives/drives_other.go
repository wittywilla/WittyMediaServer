//go:build !windows

package drives

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Copyright types a DVD drive reports for its disc.
const (
	ProtectionNone    = 0
	ProtectionCSS     = 1
	ProtectionUnknown = -1
)

// Protection: only Windows asks the drive for now; the sample reads decide here.
func Protection(string) int { return ProtectionUnknown }

// parseMounts picks out optical drives from the system's list of mounts.
func parseMounts(r io.Reader) []Drive {
	out := []Drive{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 {
			continue
		}
		base := filepath.Base(f[0])
		if !(strings.HasPrefix(base, "sr") || strings.HasPrefix(base, "cdrom")) || !strings.HasPrefix(f[0], "/dev/") {
			continue
		}
		mount := strings.ReplaceAll(f[1], `\040`, " ")
		out = append(out, Drive{Path: mount, Name: f[0], Label: filepath.Base(mount), Media: true})
	}
	return out
}

// List finds mounted optical drives. The rest of an unmounted drive's story, and
// other systems' lists, come with the releases for those systems.
func List() ([]Drive, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return []Drive{}, nil
	}
	defer f.Close()
	return parseMounts(f), nil
}

// IsDrive says a path is a mounted optical drive.
func IsDrive(p string) bool {
	list, _ := List()
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	for _, d := range list {
		if abs == d.Path || strings.HasPrefix(abs, d.Path+"/") {
			return true
		}
	}
	return false
}

// HasMedia is true for anything mounted.
func HasMedia(p string) bool { return IsDrive(p) }

// RawDisc reads a whole disc.
type RawDisc struct{}

// OpenRaw is not built on this system yet.
func OpenRaw(string) (*RawDisc, error) { return nil, ErrNoRawAccess }

// Size is the disc's length in bytes.
func (*RawDisc) Size() int64 { return 0 }

// ReadAt reads whole sectors.
func (*RawDisc) ReadAt([]byte, int64) (int, error) { return 0, ErrNoRawAccess }

// Close releases the drive.
func (*RawDisc) Close() error { return nil }
