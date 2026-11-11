// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// A client that asks for something and stops reading would hold its connection for good, so
// each piece of a response has this long to go out. It's a limit on standing still, not on the
// whole answer: a film can take hours to download as long as it keeps moving.
var writeStall = 2 * time.Minute

const (
	// mediaStall is for a file being played: a player that has read far enough ahead just
	// stops reading while someone is paused. Past this it asks again when they come back.
	mediaStall = 15 * time.Minute
	// writeChunk is the most that goes out on one deadline. With writeStall, the slowest
	// connection this leaves out is about 17 kilobits a second.
	writeChunk = 256 << 10
)

var mediaFileRoute = regexp.MustCompile(`^/api/(items/\d+/(file|stream|original|motion)$|share/[^/]+/(file|original)/[^/]+$|cd/\d+/audio$|radio/\d+/stream$|media/\d+/(canvas/file|stems/[a-z]+)$)`)

// stallLimit is how long a response to this address may stand still.
func stallLimit(p string) time.Duration {
	if rest, ok := strings.CutPrefix(p, apiPrefix); ok {
		p = "/api/" + rest
	}
	if mediaFileRoute.MatchString(p) {
		return mediaStall
	}
	return writeStall
}

// writeGuard moves a response's write deadline along as it goes. A socket that takes over its
// connection leaves this behind: the standard server clears the deadline when it lets go.
type writeGuard struct {
	http.ResponseWriter
	rc    *http.ResponseController
	limit time.Duration
	set   atomic.Int64 // when the deadline was last moved, in nanoseconds
}

func newWriteGuard(w http.ResponseWriter, path string) *writeGuard {
	g := &writeGuard{ResponseWriter: w, rc: http.NewResponseController(w), limit: stallLimit(path)}
	g.move()
	return g
}

// move sets the deadline, a little less often than it could: this comes before every write.
func (g *writeGuard) move() {
	now := time.Now()
	if last := g.set.Load(); last != 0 && now.Sub(time.Unix(0, last)) < g.limit/8 {
		return
	}
	g.set.Store(now.UnixNano())
	_ = g.rc.SetWriteDeadline(now.Add(g.limit))
}

// finish leaves time for what the standard server still sends after the handler returns,
// however long the handler took.
func (g *writeGuard) finish() {
	g.set.Store(0)
	g.move()
}

func (g *writeGuard) WriteHeader(code int) {
	g.move()
	g.ResponseWriter.WriteHeader(code)
}

func (g *writeGuard) Write(b []byte) (int, error) {
	total := 0
	for {
		g.move()
		n, err := g.ResponseWriter.Write(b[:min(len(b), writeChunk)])
		total += n
		if err != nil || n == len(b) {
			return total, err
		}
		b = b[n:]
	}
}

// ReadFrom sends a file a piece at a time. A piece of a file is still a file, so the system's
// own copy still does the work.
func (g *writeGuard) ReadFrom(src io.Reader) (int64, error) {
	lr, limited := src.(*io.LimitedReader)
	var total int64
	for {
		piece := &io.LimitedReader{R: src, N: writeChunk}
		if limited {
			if lr.N <= 0 {
				return total, nil
			}
			piece = &io.LimitedReader{R: lr.R, N: min(lr.N, writeChunk)}
		}
		want := piece.N
		g.move()
		n, err := io.Copy(g.ResponseWriter, piece)
		total += n
		if limited {
			lr.N -= n
		}
		if err != nil || n < want {
			return total, err
		}
	}
}

func (g *writeGuard) Flush() {
	g.move()
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap is for the response controller and whatever needs the connection itself.
func (g *writeGuard) Unwrap() http.ResponseWriter { return g.ResponseWriter }
