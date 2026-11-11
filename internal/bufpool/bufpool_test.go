// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package bufpool

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestABorrowedBufferIsEmpty(t *testing.T) {
	b := Get()
	b.WriteString("an earlier answer")
	Put(b)
	for i := 0; i < 100; i++ {
		c := Get()
		if c.Len() != 0 {
			t.Fatalf("a borrowed buffer held %q", c.String())
		}
		Put(c)
	}
}

func TestALargeBufferIsNotKept(t *testing.T) {
	b := Get()
	b.Write(make([]byte, keep+1))
	Put(b) // dropped: nothing to check but that later ones are small
	for i := 0; i < 100; i++ {
		c := Get()
		if c.Cap() > keep {
			t.Fatalf("a buffer of %d bytes was kept", c.Cap())
		}
		defer Put(c)
	}
	Put(nil)
}

func TestCopyBuffers(t *testing.T) {
	b := Copy.Get()
	if len(b) != copySize {
		t.Fatalf("a copy buffer of %d bytes", len(b))
	}
	Copy.Put(b[:10]) // given back shorter, as a caller may
	if c := Copy.Get(); len(c) != copySize {
		t.Fatalf("after a short one came back: %d bytes", len(c))
	}
	Copy.Put(make([]byte, 5)) // someone else's: not kept
	if c := Copy.Get(); len(c) != copySize {
		t.Fatalf("a foreign buffer was kept: %d bytes", len(c))
	}
}

var answer = map[string]any{"items": make([]map[string]any, 50), "total": 50, "name": "a list of fifty things"}

func BenchmarkEncodeFresh(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(answer)
	}
}

func BenchmarkEncodePooled(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := Get()
		_ = json.NewEncoder(buf).Encode(answer)
		Put(buf)
	}
}
