// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package tuner

import (
	"context"
	"io"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/tuner/mpegts"
)

// Stream is an open channel: Reader for raw MPEG-TS, else Input for FFmpeg.
type Stream struct {
	Reader io.ReadCloser
	Input  []string
	// Guard watches a Reader for scrambling and collects guide data. Set
	// OnEvent on it before reading to receive that data. It is the server's own: what
	// the server believes about a stream is what this saw.
	Guard *mpegts.Guard
	// Verified: an Input stream was inspected on opening and found clear.
	Verified bool
	// Recheck looks again at an Input stream that can change while it plays. It says
	// why the stream is now encrypted, or "", and how long until it's worth asking again.
	// An error is a failure to find out, which says nothing about the stream.
	Recheck func(ctx context.Context) (why string, next time.Duration, err error)
	// Done lets go of what the stream holds elsewhere: WittyTuner's tuner. It may be called more
	// than once.
	Done func()
}

// Close releases what the stream holds.
func (s *Stream) Close() error {
	var err error
	if s.Reader != nil {
		err = s.Reader.Close()
	}
	if s.Done != nil {
		s.Done()
	}
	return err
}
