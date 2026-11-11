// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package hls

import (
	"io"
	"os"
)

// readShared reads a file FFmpeg keeps replacing. Elsewhere than Windows, a file
// being read can always be renamed over.
func readShared(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}
