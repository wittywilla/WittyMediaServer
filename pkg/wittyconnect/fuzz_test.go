// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyconnect

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The local address answers only this app: a request gets through only with
// the session's secret and a Host that names this device on this port, and
// the secret never goes on to the server. Whatever a page elsewhere, or
// another program, puts in a request.
func FuzzTheLocalAddress(f *testing.F) {
	const secret = "s3cr3t-s3cr3t-s3cr3t-s3cr3t-0123"
	f.Add("127.0.0.1:5555", secret, "", "a=b")
	f.Add("p2p-abc.localhost:5555", "", secret, "wittyconnect=x; a=b")
	f.Add("evil.example:5555", secret, "", "")
	f.Add("127.0.0.1:5556", secret, "", "")
	f.Add("localhost:5555", secret[:len(secret)-1], "", "")
	f.Add("x.localhost.:5555", secret, "", "")
	f.Fuzz(func(t *testing.T, hostHeader, header, cookie, others string) {
		c := &Connector{secret: secret, host: "127.0.0.1:5555"}
		reached := false
		var leaked bool
		h := c.guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reached = true
			stripSecret(r)
			leaked = strings.Contains(r.Header.Get("Cookie"), secret) || r.Header.Get(SecretHeader) != ""
		}))
		req := httptest.NewRequest("GET", "http://127.0.0.1:5555/", nil)
		req.Host = hostHeader
		if header != "" {
			req.Header.Set(SecretHeader, header)
		}
		ck := others
		if cookie != "" {
			ck = strings.TrimPrefix(ck+"; "+SecretCookie+"="+cookie, "; ")
		}
		if ck != "" {
			req.Header.Set("Cookie", ck)
		}
		// What the request carries, read before the guard strips it.
		got := header
		if got == "" {
			if ck, err := req.Cookie(SecretCookie); err == nil {
				got = ck.Value
			}
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
		if !reached {
			return
		}
		if !localHost(hostHeader, "5555") {
			t.Fatalf("Host %q got through", hostHeader)
		}
		if got != secret {
			t.Fatalf("got through with %q", got)
		}
		if leaked {
			t.Fatal("the secret went on towards the server")
		}
	})
}
