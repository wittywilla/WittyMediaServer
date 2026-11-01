package server

import (
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
	case "/api/setup", "/api/login", "/api/logout", "/api/identity", "/api/pair/start", "/api/pair/poll", "/api/pair/redeem":
		return true
	}
	return false
}

// limitBody bounds the size of a request's body and how long it may take to arrive.
func limitBody(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	if publicBody(r.URL.Path) {
		r.Body = http.MaxBytesReader(w, r.Body, maxCredentialBytes)
		_ = rc.SetReadDeadline(time.Now().Add(publicBodyTime))
		return
	}
	if !bigUploadPath(r.URL.Path) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	}
	_ = rc.SetReadDeadline(time.Now().Add(requestBodyTime))
}
