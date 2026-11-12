// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// What a client can make the server hold on to while it sends a request.
const (
	// maxJSONBody is more than any ordinary request. A dense pen stroke, the longest
	// thing anyone saves this way, is limited to 256 KB by the library.
	maxJSONBody = 1 << 20
	// maxPlaylistBody is for a radio playlist pasted in by an administrator.
	maxPlaylistBody = 4 << 20
)

// publicBodyTime is how long someone without an account has to send their request.
var publicBodyTime = 15 * time.Second

// publicBody says which routes take a body from anyone, signed in or not. They get
// the smallest limits, because nothing has been checked yet about who is asking.
// TestEveryPublicWriteHasTheSmallLimit keeps this list up to date.
func publicBody(p string) bool {
	if rest, ok := strings.CutPrefix(p, apiPrefix); ok {
		p = "/api/" + rest
	}
	switch p {
	case "/api/setup", "/api/setup/state", "/api/login", "/api/logout", "/api/identity", "/api/pair/start", "/api/pair/poll", "/api/pair/redeem", "/api/auth/pair":
		return true
	}
	return false
}

// limitBody bounds the size of a request's body and how long it may take to arrive.
func limitBody(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	noBody := r.Body == http.NoBody
	if publicBody(r.URL.Path) {
		r.Body = http.MaxBytesReader(w, r.Body, maxCredentialBytes)
		_ = rc.SetReadDeadline(time.Now().Add(publicBodyTime))
	} else {
		if !bigUploadPath(r.URL.Path) {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		}
		_ = rc.SetReadDeadline(time.Now().Add(requestBodyTime))
	}
	if noBody {
		_ = rc.SetReadDeadline(time.Time{})
		return
	}
	r.Body = &arrivedBody{ReadCloser: r.Body, rc: rc}
}

// arrivedBody takes the deadline off once the body is all here. It's a limit on the body, and
// past that it would end the request: with nothing left to read, the standard server watches
// the connection to see if the client has gone, and takes that read failing to mean it has. So
// a request cancels itself when the deadline passes, however well it's doing.
type arrivedBody struct {
	io.ReadCloser
	rc *http.ResponseController
}

func (b *arrivedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err == io.EOF {
		_ = b.rc.SetReadDeadline(time.Time{})
	}
	return n, err
}
