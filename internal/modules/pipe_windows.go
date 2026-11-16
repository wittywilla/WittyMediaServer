// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"errors"
	"syscall"
)

// Windows' own words for a pipe whose other end has gone: ERROR_BROKEN_PIPE (109) and
// ERROR_NO_DATA (232, "The pipe is being closed"), which writing to a helper that has left gives.
const (
	errorBrokenPipe syscall.Errno = 109
	errorNoData     syscall.Errno = 232
)

func osPipeEnded(err error) bool {
	var en syscall.Errno
	return errors.As(err, &en) && (en == errorBrokenPipe || en == errorNoData)
}
