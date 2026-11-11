// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build unix

package instance

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// flock belongs to the open file, and the kernel drops it when the process ends.
func lockFile(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, unix.EWOULDBLOCK):
		return ErrBusy
	case errors.Is(err, unix.ENOLCK), errors.Is(err, unix.EOPNOTSUPP), errors.Is(err, unix.ENOTSUP), errors.Is(err, unix.ENOSYS):
		return fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	return err
}

func unlockFile(f *os.File) { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }
