// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyinfo

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"
)

// readSized reads everything whatever size it was told, as io.ReadAll does.
func TestReadSized(t *testing.T) {
	data := bytes.Repeat([]byte("wittyinfo "), 1000)
	for _, tc := range []struct {
		name string
		size int64
	}{{"exact", int64(len(data))}, {"header too small", 10}, {"header zero", 0}, {"header too large", int64(len(data)) * 3}} {
		for _, r := range []struct {
			kind string
			r    io.Reader
		}{{"whole", bytes.NewReader(data)}, {"a byte at a time", iotest.OneByteReader(bytes.NewReader(data))}, {"half", iotest.HalfReader(bytes.NewReader(data))}} {
			got, err := readSized(r.r, tc.size)
			if err != nil || !bytes.Equal(got, data) {
				t.Errorf("%s, %s: %d bytes, %v", tc.name, r.kind, len(got), err)
			}
		}
	}
	want := errors.New("the disk went away")
	if _, err := readSized(iotest.ErrReader(want), 100); !errors.Is(err, want) {
		t.Errorf("a failing read: %v", err)
	}
	// One allocation when the size is right.
	if n := testing.AllocsPerRun(20, func() { _, _ = readSized(bytes.NewReader(data), int64(len(data))) }); n > 2 {
		t.Errorf("%v allocations reading a member of the size its header says", n)
	}
}
