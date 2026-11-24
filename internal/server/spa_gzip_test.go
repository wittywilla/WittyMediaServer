// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"compress/gzip"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// Scripts are stored compressed only. A browser that takes that gets the stored bytes, and one
// that doesn't gets the script.
func TestCompressedOnlyAssets(t *testing.T) {
	const script = "console.log('made up')\n"
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write([]byte(script))
	zw.Close()
	s := &Server{Web: fstest.MapFS{
		"index.html":       {Data: []byte("<html></html>")},
		"assets/a.js.gz":   {Data: gz.Bytes()},
		"assets/plain.txt": {Data: []byte("plain")},
	}}
	h := s.spa()
	get := func(path, accept string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if accept != "" {
			r.Header.Set("Accept-Encoding", accept)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := get("/assets/a.js", "gzip, deflate")
	if w.Code != 200 || w.Header().Get("Content-Encoding") != "gzip" || !bytes.Equal(w.Body.Bytes(), gz.Bytes()) {
		t.Errorf("takes gzip: %d %q %d bytes", w.Code, w.Header().Get("Content-Encoding"), w.Body.Len())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/javascript; charset=utf-8" {
		t.Errorf("sent as %q", ct)
	}
	w = get("/assets/a.js", "")
	if w.Code != 200 || w.Header().Get("Content-Encoding") != "" || w.Body.String() != script {
		t.Errorf("doesn't take gzip: %d %q %q", w.Code, w.Header().Get("Content-Encoding"), w.Body)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("cached as %q", cc)
	}
	if w = get("/assets/plain.txt", "gzip"); w.Code != 200 || w.Body.String() != "plain" {
		t.Errorf("a file kept as it is: %d %q", w.Code, w.Body)
	}
	if w = get("/assets/none.js", "gzip"); w.Header().Get("Content-Encoding") != "" {
		t.Errorf("a file that isn't there: %d %q", w.Code, w.Header().Get("Content-Encoding"))
	}
}

// The client's files outside assets/ (fonts, icons) carry a tag from their content, so a
// browser asking again gets 304 and not the whole file each visit (#643), and still asks each
// time (no-cache), so an update reaches it at once.
func TestWebFilesAnswerNotModified(t *testing.T) {
	font := bytes.Repeat([]byte("font"), 1000)
	s := &Server{Web: fstest.MapFS{"index.html": {Data: []byte("<html></html>")}, "fonts/a.woff2": {Data: font}}}
	h := s.spa()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/fonts/a.woff2", nil))
	tag := w.Header().Get("ETag")
	if w.Code != 200 || w.Body.Len() != len(font) || tag == "" || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("first visit: %d, %d bytes, tag %q, %q", w.Code, w.Body.Len(), tag, w.Header().Get("Cache-Control"))
	}
	r := httptest.NewRequest("GET", "/fonts/a.woff2", nil)
	r.Header.Set("If-None-Match", tag)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 304 || w.Body.Len() != 0 {
		t.Fatalf("asked again with its tag: %d, %d bytes; want 304 and nothing", w.Code, w.Body.Len())
	}
}
