// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/klauspost/compress/gzip"
)

// Pages of lists are JSON that repeats itself, and come out about ten times smaller compressed:
// a library page on a phone away from home is half a second sooner. The compressor here keeps no
// state between answers, so an idle server holds nothing for this, and it costs a fraction of a
// millisecond per answer. On the home network the same answer simply takes less of the air.
//
// Only these routes, and only GET: they hold what's in the libraries, and nothing secret. An
// answer with a secret in it isn't compressed, because its size can give that away piece by piece
// to someone who can both add text to it and watch the encrypted traffic.
var compressedLists = regexp.MustCompile(`^/api/(` +
	`home|items|browse|browse/facets|browse/shelves|shows|search|genres|brands|libraries|collections|` +
	`guide|channels|recommended|me/recommended|` +
	`libraries/\d+/(albums|artists|filters)|shows/\d+(/seasons)?|items/\d+|` +
	`music/[a-z]+|photos(/[a-z]+)?|modulestore(/[a-z]+)?|games(/[a-z]+)?|views/\d+/titles|collections/\d+|` +
	`podcasts|radio)$`)

// Smaller than this isn't worth it: a few packets either way.
const compressAtLeast = 2 << 10

var compressBuffers = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func (s *Server) compressLists(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !compressedLists.MatchString(r.URL.Path) || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(&listWriter{ResponseWriter: w}, r)
	})
}

// listWriter compresses an answer written in one piece, as writeJSON writes. Anything else
// goes through as it is.
type listWriter struct {
	http.ResponseWriter
	code    int
	started bool
}

func (l *listWriter) WriteHeader(code int) {
	if !l.started && code >= 200 {
		l.code = code
		return
	}
	l.ResponseWriter.WriteHeader(code)
}

func (l *listWriter) start() {
	if l.started {
		return
	}
	l.started = true
	if l.code != 0 {
		l.ResponseWriter.WriteHeader(l.code)
	}
}

func (l *listWriter) Write(b []byte) (int, error) {
	h := l.Header()
	if l.started || len(b) < compressAtLeast || h.Get("Content-Encoding") != "" || !strings.HasPrefix(h.Get("Content-Type"), "application/json") {
		l.start()
		return l.ResponseWriter.Write(b)
	}
	buf := compressBuffers.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		if buf.Cap() <= 1<<20 { // a rare huge answer doesn't stay in memory
			compressBuffers.Put(buf)
		}
	}()
	zw, err := gzip.NewWriterLevel(buf, gzip.StatelessCompression)
	if err == nil {
		if _, err = zw.Write(b); err == nil {
			err = zw.Close()
		}
	}
	if err != nil {
		l.start()
		return l.ResponseWriter.Write(b)
	}
	h.Set("Content-Encoding", "gzip")
	h.Add("Vary", "Accept-Encoding")
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	l.start()
	if _, err := l.ResponseWriter.Write(buf.Bytes()); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (l *listWriter) Flush() {
	l.start()
	if f, ok := l.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (l *listWriter) Unwrap() http.ResponseWriter { return l.ResponseWriter }
