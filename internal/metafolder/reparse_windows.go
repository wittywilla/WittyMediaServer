package metafolder

import (
	"os"
	"syscall"
)

// isReparsePoint: a junction, a symbolic link, or a placeholder for something elsewhere.
func isReparsePoint(fi os.FileInfo) bool {
	d, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	return ok && d.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
