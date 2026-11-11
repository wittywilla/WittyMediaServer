// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build windows

package hls

import (
	"io"
	"os"
	"syscall"
)

// readShared reads a file FFmpeg keeps replacing, opened so that it can go on
// replacing it meanwhile. Opened the usual way on Windows, a playlist being read
// can't be renamed over, and FFmpeg's update of it is lost.
func readShared(path string, limit int64) ([]byte, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}
