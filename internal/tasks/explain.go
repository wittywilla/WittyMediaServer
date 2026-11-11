// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package tasks

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"strings"
	"syscall"
)

// Explain turns an error into a sentence a person can understand, and what they can
// do about it. It's a fallback: a job that knows better should say so with Fail.
func Explain(err error) (message, fix string) {
	switch {
	case err == nil:
		return "", ""
	case errors.Is(err, context.DeadlineExceeded), os.IsTimeout(err):
		return "It took too long and was stopped.", "Try again. If it keeps happening, the drive or service it needs may be slow or away."
	case errors.Is(err, fs.ErrPermission):
		return "The server isn't allowed to read or write something it needs.", "Check that the account running the server can get to that folder."
	case errors.Is(err, fs.ErrNotExist):
		return "Something it needed isn't there any more.", "If it's on a drive or a network share, check that it's connected, then try again."
	case errors.Is(err, syscall.ENOSPC):
		return "The disk is full.", "Free up some space, then try again."
	case errors.Is(err, syscall.EIO):
		return "A drive reported a read or write error.", "Run WittyDiagnoser's drive check from Settings, and check the drive's connection."
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "The server couldn't reach an online service.", "Check the server's internet connection, then try again."
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return "A network connection failed.", "Check the connection between the server and what it was talking to, then try again."
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "no space left"), strings.Contains(s, "not enough space"), strings.Contains(s, "disk full"):
		return "The disk is full.", "Free up some space, then try again."
	case strings.Contains(s, "database is locked"), strings.Contains(s, "database table is locked"):
		return "The database was too busy.", "Try again in a moment."
	case strings.Contains(s, "database disk image is malformed"):
		return "The server's database is damaged.", "See Settings, Reset, or restore a backup. Nothing has been changed."
	case strings.Contains(s, "connection refused"), strings.Contains(s, "no route to host"), strings.Contains(s, "connection reset"):
		return "A network connection failed.", "Check that what it was talking to is running and reachable, then try again."
	}
	return "It stopped because of an error.", "Try again. If it happens again, WittyLog has the details."
}
