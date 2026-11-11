// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package bufpool keeps the buffers the server fills and empties all day, so
// each answer borrows one instead of asking for new memory that the garbage
// collector then has to find and free. On a small machine that work is a real
// share of what a request costs.
package bufpool

import (
	"bytes"
	"sync"
)

// keep is the largest buffer worth keeping. A rare large answer (a whole library
// as JSON) would otherwise stay in the pool at its full size for good.
const keep = 256 << 10

var buffers = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// Get borrows an empty buffer. Give it back with Put when nothing holds its bytes.
func Get() *bytes.Buffer { return buffers.Get().(*bytes.Buffer) }

// Put gives a buffer back. Nothing may use the buffer, or bytes taken from it, after.
func Put(b *bytes.Buffer) {
	if b == nil || b.Cap() > keep {
		return
	}
	b.Reset()
	buffers.Put(b)
}

// copySize is what io.Copy uses for a buffer of its own.
const copySize = 32 << 10

var copies = sync.Pool{New: func() any { b := make([]byte, copySize); return &b }}

// Copy lends buffers for copying a stream from one place to another. It is an
// httputil.BufferPool, for the proxy to a module's service.
var Copy copyPool

type copyPool struct{}

// Get borrows a 32 KB buffer.
func (copyPool) Get() []byte { return *copies.Get().(*[]byte) }

// Put gives one back.
func (copyPool) Put(b []byte) {
	if cap(b) != copySize {
		return
	}
	b = b[:copySize]
	copies.Put(&b)
}
