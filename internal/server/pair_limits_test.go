// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"fmt"
	"testing"
)

// One IPv6 network is one address, and it can't take every place at the table.
func TestStrangersCantStopPairingAtHome(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	putRemote(t, h, tok, map[string]any{"enabled": true})
	body := map[string]string{"device": "TV"}
	got := 0
	for i := range 300 {
		from := fmt.Sprintf("2001:4860:4860:1::%x", i+1)
		if r := call(h, "POST", "/api/pair/start", from, body, "TLS: yes"); r.code == 200 {
			got++
		}
	}
	if got > pairMaxPerIP {
		t.Fatalf("one network opened %d codes", got)
	}
	for a := range 300 {
		call(h, "POST", "/api/pair/start", fmt.Sprintf("198.18.%d.1", a), body, "TLS: yes")
	}
	if r := call(h, "POST", "/api/pair/start", lan, body); r.code != 200 {
		t.Fatalf("a TV at home, with strangers holding every place: %d %s", r.code, r.body)
	}
}

// Wrong codes from strangers don't reach a code made at home.
func TestStrangersCantCancelCodesAtHome(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	putRemote(t, h, tok, map[string]any{"enabled": true})
	r := call(h, "POST", "/api/pair/invite", lan, map[string]any{}, "Authorization: Bearer "+tok)
	var in struct {
		Code string `json:"code"`
	}
	r.json(t, &in)
	if in.Code == "" {
		t.Fatalf("%d %s", r.code, r.body)
	}
	for a := range inviteWrongLimit + 5 {
		from := fmt.Sprintf("198.18.%d.1", a)
		call(h, "POST", "/api/pair/redeem", from, map[string]string{"code": ""}, "TLS: yes")
		wrong := "000000"
		if in.Code == wrong {
			wrong = "000001"
		}
		call(h, "POST", "/api/pair/redeem", from, map[string]string{"code": wrong}, "TLS: yes")
	}
	if r := call(h, "POST", "/api/pair/redeem", lan, map[string]string{"code": in.Code, "device": "TV"}); r.code != 200 {
		t.Fatalf("the real code at home: %d %s", r.code, r.body)
	}
}

// A signed-in adult can't reset the wrong-code limit with a code of their own.
func TestOwnCodeDoesntResetTheWrongCodeLimit(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "adult", "adult", false, "")
	w := ts.do(t, "POST", "/api/pair/start", "", map[string]string{"device": "TV"})
	var started struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &started); err != nil || started.Code == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for i := range 60 {
		ts.do(t, "POST", "/api/pair/info", tok, map[string]string{"code": "BBBB-BBBB"})
		ts.do(t, "POST", "/api/pair/info", tok, map[string]string{"code": started.Code})
		if w := ts.do(t, "POST", "/api/pair/info", tok, map[string]string{"code": "BBBB-BBBC"}); w.Code == 429 {
			if i == 0 {
				t.Fatal("locked at once")
			}
			return
		}
	}
	t.Fatal("never limited")
}
