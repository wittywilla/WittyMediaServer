// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"
)

// readInto reads everything, with room for all of it, for some of it or for none.
func TestReadInto(t *testing.T) {
	data := bytes.Repeat([]byte("wittyinfo "), 1000)
	for _, room := range []int{0, 1, 10, len(data), len(data) + 1, 3 * len(data)} {
		for _, r := range []io.Reader{bytes.NewReader(data), iotest.OneByteReader(bytes.NewReader(data)), iotest.HalfReader(bytes.NewReader(data))} {
			got, err := readInto(make([]byte, 0, room), r)
			if err != nil || !bytes.Equal(got, data) {
				t.Errorf("room for %d: %d bytes, %v", room, len(got), err)
			}
		}
	}
	want := errors.New("the disk went away")
	if _, err := readInto(nil, iotest.ErrReader(want)); !errors.Is(err, want) {
		t.Errorf("a failing read: %v", err)
	}
}
