// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package remote

import (
	"context"
	"io"
	"net/http"
	"time"
)

// Throttle limits what one response sends, in bits per second. Players read
// ahead, so the first seconds' worth goes out at once and only after that does
// the limit bite.
func Throttle(ctx context.Context, w http.ResponseWriter, bitsPerSec int64) http.ResponseWriter {
	if bitsPerSec <= 0 {
		return w
	}
	return &throttled{ResponseWriter: w, ctx: ctx, rate: bitsPerSec / 8, start: time.Now()}
}

const (
	throttleBurst = 2 * time.Second // of data, sent without waiting
	throttleChunk = 64 << 10
)

type throttled struct {
	http.ResponseWriter
	ctx   context.Context
	rate  int64 // bytes per second
	start time.Time
	sent  int64
}

// wait holds back until n more bytes are due, counting the head start.
func (t *throttled) wait(n int64) error {
	due := time.Duration(float64(t.sent+n)/float64(t.rate)*float64(time.Second)) - throttleBurst
	if wait := due - time.Since(t.start); wait > 0 {
		tm := time.NewTimer(wait)
		select {
		case <-tm.C:
		case <-t.ctx.Done():
			tm.Stop()
			return t.ctx.Err()
		}
	}
	return nil
}

func (t *throttled) Write(b []byte) (int, error) {
	total := 0
	for len(b) > 0 {
		n := min(len(b), throttleChunk)
		if err := t.wait(int64(n)); err != nil {
			return total, err
		}
		w, err := t.ResponseWriter.Write(b[:n])
		t.sent += int64(w)
		total += w
		if err != nil {
			return total, err
		}
		b = b[n:]
	}
	return total, nil
}

// ReadFrom sends a file a piece at a time, each when it is due. A piece of a file
// is still a file, so the system's own copy (sendfile) still does the work and
// the file's bytes never pass through this program: on a small machine that is
// most of what sending a film away from home costs.
func (t *throttled) ReadFrom(src io.Reader) (int64, error) {
	lr, limited := src.(*io.LimitedReader)
	var total int64
	for {
		piece := &io.LimitedReader{R: src, N: throttleChunk}
		if limited {
			if lr.N <= 0 {
				return total, nil
			}
			piece = &io.LimitedReader{R: lr.R, N: min(lr.N, throttleChunk)}
		}
		want := piece.N
		if err := t.wait(want); err != nil {
			return total, err
		}
		n, err := io.Copy(t.ResponseWriter, piece)
		t.sent += n
		total += n
		if limited {
			lr.N -= n
		}
		if err != nil || n < want {
			return total, err
		}
	}
}

// Unwrap is for the response controller and whatever needs the connection itself.
func (t *throttled) Unwrap() http.ResponseWriter { return t.ResponseWriter }

func (t *throttled) Flush() {
	if f, ok := t.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
