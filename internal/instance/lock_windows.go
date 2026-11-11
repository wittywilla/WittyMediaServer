// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build windows

package instance

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// LockFileEx belongs to the handle, and Windows drops it when the process ends. The byte
// locked is far past the end of the file, so others can still read the holder's number.
func lockFile(f *os.File) error {
	ol := windows.Overlapped{OffsetHigh: 1}
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ol)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, windows.ERROR_LOCK_VIOLATION), errors.Is(err, windows.ERROR_IO_PENDING):
		return ErrBusy
	case errors.Is(err, windows.ERROR_NOT_SUPPORTED), errors.Is(err, windows.ERROR_INVALID_FUNCTION):
		return fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	return err
}

func unlockFile(f *os.File) {
	ol := windows.Overlapped{OffsetHigh: 1}
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ol)
}
