package hostsandbox

import (
	"os"

	"golang.org/x/sys/windows"
)

// ClaimStdout moves the protocol pipe somewhere a core can't write to by accident.
// A C library may already have noted stdout's handle number, so that number is
// closed and, where the system hands it straight back, reopened on NUL.
func ClaimStdout() (*os.File, error) {
	old := windows.Handle(os.Stdout.Fd())
	self := windows.CurrentProcess()
	var private windows.Handle
	if err := windows.DuplicateHandle(self, old, self, &private, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}
	if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(os.Stderr.Fd())); err != nil {
		return nil, err
	}
	if err := windows.CloseHandle(old); err != nil {
		return nil, err
	}
	nul, err := windows.UTF16PtrFromString("NUL")
	if err != nil {
		return nil, err
	}
	var extra []windows.Handle
	for i := 0; i < 8; i++ {
		h, err := windows.CreateFile(nul, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil || h == old {
			break
		}
		extra = append(extra, h)
	}
	for _, h := range extra {
		_ = windows.CloseHandle(h)
	}
	return os.NewFile(uintptr(private), "ipc"), nil
}
