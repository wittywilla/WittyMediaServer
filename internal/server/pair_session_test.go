// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"fmt"
	"testing"
)

// What a stolen session started can't outlive the owner changing their password. A script keeping a
// code open used to get the account back after every change, and every "sign out everywhere".
func TestPendingCodesEndWithTheSessionThatMadeThem(t *testing.T) {
	ts := newTestServer(t)
	u, stolen := ts.user(t, "owner", "adult", true, "")
	id := fmt.Sprintf("/api/users/%d/password", u.ID)

	w := ts.do(t, "POST", "/api/pair/invite", stolen, map[string]any{})
	var in struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &in); err != nil || in.Code == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = ts.do(t, "POST", "/api/pair/start", "", map[string]string{"device": "Thief's box"})
	var started struct {
		Code         string `json:"code"`
		DeviceSecret string `json:"deviceSecret"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &started); err != nil || started.Code == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/pair/approve", stolen, map[string]any{"code": started.Code}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	// The owner, on another device, changes the password.
	mine := ts.loginAs(t, "owner", "correct-horse-1")
	if w := ts.do(t, "PUT", id, mine, map[string]string{"password": "a brand new password", "current": "correct-horse-1"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/me", stolen, nil); w.Code != 401 {
		t.Fatalf("the stolen session lives: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/pair/redeem", "", map[string]string{"code": in.Code, "device": "x"}); w.Code == 200 {
		t.Errorf("the invite made a new session: %s", w.Body)
	}
	if w := ts.do(t, "POST", "/api/pair/poll", "", map[string]string{"deviceSecret": started.DeviceSecret}); w.Code == 200 {
		var got map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if got["token"] != nil {
			t.Errorf("the approved pairing made a new session: %s", w.Body)
		}
	}
}

// Both still work while the session that made them does.
func TestPendingCodesWorkWhileTheSessionLasts(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	w := ts.do(t, "POST", "/api/pair/invite", tok, map[string]any{})
	var in struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &in); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "POST", "/api/pair/redeem", "", map[string]string{"code": in.Code, "device": "x"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = ts.do(t, "POST", "/api/pair/start", "", map[string]string{"device": "TV"})
	var started struct {
		Code         string `json:"code"`
		DeviceSecret string `json:"deviceSecret"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "POST", "/api/pair/approve", tok, map[string]any{"code": started.Code}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/pair/poll", "", map[string]string{"deviceSecret": started.DeviceSecret}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func (ts *testServer) loginAs(t *testing.T, name, password string) string {
	t.Helper()
	w := ts.do(t, "POST", "/api/login", "", map[string]string{"name": name, "password": password})
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	return out.Token
}
