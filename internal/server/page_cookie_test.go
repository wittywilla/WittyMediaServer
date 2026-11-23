// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
)

func pageCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.CookieName {
			return c
		}
	}
	return nil
}

// A page of this server's own that signed in by header gets the cookie, so its pictures and
// videos need no token in their addresses. A page elsewhere never does.
func TestStatusGivesTheServersOwnPageItsCookie(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")

	w := ts.do(t, "GET", "/api/status", tok, nil, "Sec-Fetch-Site", "same-origin")
	c := pageCookie(w)
	if c == nil || c.Value != tok || !c.HttpOnly {
		t.Fatalf("same-origin page: cookie %+v", c)
	}
	// Already there: nothing new is sent.
	w = ts.do(t, "GET", "/api/status", tok, nil, "Sec-Fetch-Site", "same-origin", "Cookie", auth.CookieName+"="+tok)
	if c := pageCookie(w); c != nil {
		t.Fatalf("cookie sent again: %+v", c)
	}
	// Another site's page, or an app elsewhere, keeps its token to itself.
	for _, hdr := range [][]string{{"Sec-Fetch-Site", "cross-site"}, {"Origin", "https://elsewhere.example"}} {
		w = ts.do(t, "GET", "/api/status", tok, nil, hdr...)
		if w.Code != 200 {
			t.Fatalf("%v: %d", hdr, w.Code)
		}
		if c := pageCookie(w); c != nil {
			t.Fatalf("%v: cookie %+v", hdr, c)
		}
	}
	// Signed out: nothing.
	if c := pageCookie(ts.do(t, "GET", "/api/status", "", nil, "Sec-Fetch-Site", "same-origin")); c != nil {
		t.Fatalf("signed out: cookie %+v", c)
	}
}

// A browser that signs in by pairing on this server's own page gets the cookie with its token.
func TestPairingInTheServersOwnPageSetsTheCookie(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	start := func() (code, secret string) {
		w := ts.do(t, "POST", "/api/pair/start", "", map[string]string{"device": "browser"})
		var started struct {
			Code         string `json:"code"`
			DeviceSecret string `json:"deviceSecret"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &started); err != nil {
			t.Fatal(err)
		}
		if w := ts.do(t, "POST", "/api/pair/approve", tok, map[string]any{"code": started.Code, "password": "correct-horse-1"}); w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		return started.Code, started.DeviceSecret
	}

	_, secret := start()
	w := ts.do(t, "POST", "/api/pair/poll", "", map[string]string{"deviceSecret": secret}, "Sec-Fetch-Site", "same-origin")
	var got struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Token == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if c := pageCookie(w); c == nil || c.Value != got.Token {
		t.Fatalf("same-origin pairing: cookie %+v", c)
	}

	_, secret = start()
	w = ts.do(t, "POST", "/api/pair/poll", "", map[string]string{"deviceSecret": secret}, "Origin", "capacitor://localhost") // the mobile app
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if c := pageCookie(w); c != nil {
		t.Fatalf("pairing in an app: cookie %+v", c)
	}
}
