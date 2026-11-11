// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package tuner

import (
	"context"
	"io"
	"net/http"
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
}

// Close releases what the stream holds.
func (s *Stream) Close() error {
	if s.Reader != nil {
		return s.Reader.Close()
	}
	return nil
}

// Source is one configured tuner.
type Source interface {
	Info() Info
	// Channels is what the source knows now, without tuning anything.
	Channels(ctx context.Context) ([]Channel, error)
	// Scan looks for channels. It may take minutes, and is cancelled by ctx.
	Scan(ctx context.Context, o ScanOptions, progress func(Progress)) ([]Channel, error)
	// Open starts a channel. ErrBusy when no tuner is free, *EncryptedError
	// when the channel is encrypted.
	Open(ctx context.Context, ch Channel) (*Stream, error)
	Close() error
}

// Guided is a source with headers to use when fetching its guide.
type Guided interface{ GuideHeaders() http.Header }

// Config is how a tuner is set up.
type Config struct {
	Name    string
	Options map[string]string
}

// Env is what a helper needs told about this server.
type Env struct {
	FFmpeg string
	// AllowCommands: the owner has allowed the custom command driver.
	AllowCommands bool
}

// HelperCall is what a helper is asked: which tuner, since it keeps nothing, and what for.
type HelperCall struct {
	Driver  string            `json:"driver,omitempty"`
	Name    string            `json:"name,omitempty"`
	Options map[string]string `json:"options,omitempty"`
	Channel *Channel          `json:"channel,omitempty"` // "open" and "recheck"
	Source  string            `json:"source,omitempty"`  // "scan": ScanOptions.Source
	// What it can't find out for itself.
	FFmpeg        string `json:"ffmpeg,omitempty"`
	AllowCommands bool   `json:"allowCommands,omitempty"`
}

// HelperDoc is one document of an answer. Progress may come any number of times; any
// other document ends the answer.
type HelperDoc struct {
	Progress     *Progress   `json:"progress,omitempty"`
	Channels     []Channel   `json:"channels,omitempty"`
	Info         *Info       `json:"info,omitempty"`
	GuideHeaders http.Header `json:"guideHeaders,omitempty"`
	Found        []Found     `json:"found,omitempty"`
	Drivers      []Driver    `json:"drivers,omitempty"`
	Why          string      `json:"why,omitempty"`  // "recheck": why it's encrypted now
	Next         int64       `json:"next,omitempty"` // "recheck": seconds until it's worth asking again
	Error        string      `json:"error,omitempty"`
	Code         string      `json:"code,omitempty"`
	Done         bool        `json:"done,omitempty"`
}
