//go:build !windows

package metafolder

import "os"

func isReparsePoint(fi os.FileInfo) bool { return fi.Mode()&os.ModeSymlink != 0 }
