// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListsAreCompressed(t *testing.T) {
	long := map[string]string{"text": strings.Repeat("the same words again ", 1000)}
	s := &Server{}
	h := s.compressLists(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("answer") {
		case "short":
			writeJSON(w, 200, map[string]string{"a": "b"})
		case "error":
			writeJSON(w, 418, long)
		default:
			writeJSON(w, 200, long)
		}
	}))
	ask := func(method, url, accept string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, url, nil)
		if accept != "" {
			r.Header.Set("Accept-Encoding", accept)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	unpack := func(w *httptest.ResponseRecorder) string {
		t.Helper()
		zr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(zr)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	whole, _ := json.Marshal(long)
	want := string(whole) + "\n"

	w := ask("GET", "/api/items", "gzip, br")
	if w.Code != 200 || w.Header().Get("Content-Encoding") != "gzip" || w.Body.Len() >= len(want)/10 {
		t.Fatalf("a list: %d %q %d bytes", w.Code, w.Header().Get("Content-Encoding"), w.Body.Len())
	}
	if got := unpack(w); got != want {
		t.Errorf("came out as %.80q", got)
	}
	if w = ask("GET", "/api/items?answer=error", "gzip"); w.Code != 418 || unpack(w) != want {
		t.Errorf("the status went missing: %d", w.Code)
	}
	if w = ask("GET", "/api/items", ""); w.Header().Get("Content-Encoding") != "" || w.Body.String() != want {
		t.Errorf("a client that doesn't take it: %q", w.Header().Get("Content-Encoding"))
	}
	if w = ask("GET", "/api/items?answer=short", "gzip"); w.Header().Get("Content-Encoding") != "" || w.Code != 200 {
		t.Errorf("a short answer: %q %d", w.Header().Get("Content-Encoding"), w.Code)
	}
	if w = ask("POST", "/api/items", "gzip"); w.Header().Get("Content-Encoding") != "" {
		t.Error("a POST was compressed")
	}
	// Answers that could hold a secret never are.
	for _, p := range []string{"/api/login", "/api/me", "/api/settings", "/api/sessions", "/api/remote", "/api/items/1/playback", "/api/items/1/file", "/api/share/x"} {
		if w = ask("GET", p, "gzip"); w.Header().Get("Content-Encoding") != "" {
			t.Errorf("%s was compressed", p)
		}
	}
}
