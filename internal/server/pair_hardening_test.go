// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// The server makes the PIN and checks it with one clock, so the PIN that will be shown
// next is never right: accepting it would only be a third way to guess.
func TestAuthPairRefusesAPINNotYetShown(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "alex", "adult", false, "")
	c := withClock(ts)
	p := startAuthPair(t, ts)
	next := pinAt(ts, c, p.SessionID, 1)
	if next == pinAt(ts, c, p.SessionID, 0) || next == pinAt(ts, c, p.SessionID, -1) {
		t.Skip("the next PIN happens to equal one in the window")
	}
	if code, b := approvePIN(t, ts, tok, p.SessionID, next); code != 400 || b["reason"] != pinWrong {
		t.Errorf("the next step's PIN: %d %v", code, b)
	}
	if code, b := pollAuthPair(t, ts, p.SessionID, p.DeviceSecret); code != 200 || b["status"] != "pending" {
		t.Errorf("after a refused PIN: %d %v", code, b)
	}
}

// Signing a device in as an administrator asks for that administrator's password again,
// by every way a device can be signed in. Signing it in as a Kids profile doesn't.
func TestPairingAsAnAdministratorNeedsThePassword(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	kid, _ := ts.user(t, "sam", "kids", false, "")
	c := withClock(ts)

	// With a PIN.
	p := startAuthPair(t, ts)
	approve := func(body map[string]any) (int, map[string]any) {
		body["pin"] = pinAt(ts, c, p.SessionID, 0)
		w := ts.do(t, "POST", "/api/auth/pair/"+p.SessionID+"/approve", admin, body)
		return w.Code, decode[map[string]any](t, w)
	}
	if code, b := approve(map[string]any{}); code != 403 || b["reason"] != pairNeedsPassword {
		t.Fatalf("no password: %d %v", code, b)
	}
	if code, b := approve(map[string]any{"password": "not-it-at-all"}); code != 403 {
		t.Fatalf("wrong password: %d %v", code, b)
	}
	if code, b := pollAuthPair(t, ts, p.SessionID, p.DeviceSecret); code != 200 || b["status"] != "pending" {
		t.Fatalf("the device was answered without the password: %d %v", code, b)
	}
	// A refused password takes none of the PIN's five tries.
	ts.pair.mu.Lock()
	tries := ts.pair.byID[p.SessionID].tries
	ts.pair.mu.Unlock()
	if tries != 0 {
		t.Errorf("%d PIN tries were used up by password refusals", tries)
	}
	if code, b := approve(map[string]any{"password": "correct-horse-1"}); code != 200 {
		t.Fatalf("right password: %d %v", code, b)
	}
	if code, b := pollAuthPair(t, ts, p.SessionID, p.DeviceSecret); code != 200 || b["status"] != "approved" || b["token"] == "" {
		t.Fatalf("approved: %d %v", code, b)
	}

	// As a Kids profile: no password, and the device gets the Kids profile, not the administrator.
	p = startAuthPair(t, ts)
	w := ts.do(t, "POST", "/api/auth/pair/"+p.SessionID+"/approve", admin, map[string]any{"pin": pinAt(ts, c, p.SessionID, 0), "userId": kid.ID})
	if w.Code != 200 {
		t.Fatalf("as a Kids profile: %d %s", w.Code, w.Body)
	}
	_, b := pollAuthPair(t, ts, p.SessionID, p.DeviceSecret)
	if u, _ := b["user"].(map[string]any); u == nil || u["isAdmin"] != false || u["name"] != "sam" {
		t.Errorf("the device signed in as %v", b["user"])
	}

	// With a plain code.
	st := decode[map[string]any](t, ts.do(t, "POST", "/api/pair/start", "", map[string]string{"device": "Old TV"}))
	code, _ := st["code"].(string)
	if w := ts.do(t, "POST", "/api/pair/approve", admin, map[string]any{"code": code}); w.Code != 403 || decode[map[string]any](t, w)["reason"] != pairNeedsPassword {
		t.Errorf("code, no password: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/pair/approve", admin, map[string]any{"code": code, "password": "correct-horse-1"}); w.Code != 200 {
		t.Errorf("code and password: %d %s", w.Code, w.Body)
	}
	// With digits to type on the device.
	if w := ts.do(t, "POST", "/api/pair/invite", admin, map[string]any{}); w.Code != 403 || decode[map[string]any](t, w)["reason"] != pairNeedsPassword {
		t.Errorf("invite, no password: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/pair/invite", admin, map[string]any{"userId": kid.ID}); w.Code != 200 {
		t.Errorf("invite for a Kids profile: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/pair/invite", admin, map[string]any{"password": "correct-horse-1"}); w.Code != 200 {
		t.Errorf("invite with the password: %d %s", w.Code, w.Body)
	}
}

// An app that scanned the QR code asks what is asking, by session, before approving.
func TestAuthPairInfoBySession(t *testing.T) {
	ts := newTestServer(t)
	_, adult := ts.user(t, "alex", "adult", false, "")
	_, kid := ts.user(t, "sam", "kids", false, "")
	_, admin := ts.user(t, "owner", "adult", true, "")
	withClock(ts)
	p := startAuthPair(t, ts)
	qr, err := url.Parse(p.QR)
	if err != nil {
		t.Fatal(err)
	}
	k := qr.Query().Get("k")
	path := "/api/v1/auth/pair/" + p.SessionID + "/info"
	if w := ts.do(t, "GET", path, "", nil); w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}
	if w := ts.do(t, "GET", path, kid, nil); w.Code != 403 {
		t.Errorf("a Kids profile: %d", w.Code)
	}
	if w := ts.do(t, "GET", path+"?k=AAAAAAAAAAAAAAAA", adult, nil); w.Code != 400 || decode[map[string]any](t, w)["reason"] != pinBadCheck {
		t.Errorf("a QR code that was altered: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/auth/pair/00000000000000000000000000000000/info", adult, nil); w.Code != 404 {
		t.Errorf("no such session: %d", w.Code)
	}
	w := ts.do(t, "GET", path+"?k="+k, adult, nil)
	info := decode[map[string]any](t, w)
	if w.Code != 200 || info["device"] != "Living room TV" || info["needsPin"] != true || info["sessionId"] != p.SessionID {
		t.Fatalf("info: %d %v", w.Code, info)
	}
	// It never says the PIN or the device's secret.
	if body := w.Body.String(); strings.Contains(body, p.PIN) || strings.Contains(body, p.DeviceSecret) {
		t.Error("the answer holds the PIN or the device's secret")
	}
	// An administrator is told their own profile needs the password.
	info = decode[map[string]any](t, ts.do(t, "GET", path, admin, nil))
	profiles, _ := info["profiles"].([]any)
	if len(profiles) < 2 || profiles[0].(map[string]any)["needsPassword"] != true || profiles[1].(map[string]any)["needsPassword"] == true {
		t.Errorf("profiles: %v", profiles)
	}
	// Too many wrong sessions and the person waits, like wrong codes.
	for i := 0; i < 20; i++ {
		ts.do(t, "GET", "/api/auth/pair/00000000000000000000000000000000/info", adult, nil)
	}
	if w := ts.do(t, "GET", path, adult, nil); w.Code != 429 {
		t.Errorf("after many wrong sessions: %d", w.Code)
	}
}

// With "pairRequirePin" on, a device can't ask for a code that signs it in without a PIN.
func TestPairRequirePin(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	if w := ts.do(t, "POST", "/api/pair/start", "", map[string]string{"device": "TV"}); w.Code != 200 {
		t.Fatalf("code-only pairing by default: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"pairRequirePin": "1"}); w.Code != 200 {
		t.Fatalf("turning it on: %d %s", w.Code, w.Body)
	}
	w := ts.do(t, "POST", "/api/pair/start", "", map[string]string{"device": "TV"})
	if w.Code != 426 || decode[map[string]any](t, w)["reason"] != "pin-required" {
		t.Errorf("code-only pairing with the setting on: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/auth/pair", "", map[string]string{"device": "TV"}); w.Code != 200 {
		t.Errorf("pairing with a PIN: %d %s", w.Code, w.Body)
	}
}

// Another device, or anyone who only saw the screen, can't collect the sign-in or read the PIN.
func TestAuthPairIsBoundToItsDevice(t *testing.T) {
	ts := newTestServer(t)
	_, adult := ts.user(t, "alex", "adult", false, "")
	c := withClock(ts)
	mine, theirs := startAuthPair(t, ts), startAuthPair(t, ts)
	if code, b := approvePIN(t, ts, adult, mine.SessionID, pinAt(ts, c, mine.SessionID, 0)); code != 200 {
		t.Fatalf("approve: %d %v", code, b)
	}
	for name, secret := range map[string]string{"another device's secret": theirs.DeviceSecret, "the session id": mine.SessionID + mine.SessionID, "the QR check": "0000000000000000000000000000000000000000000000000000000000000000"} {
		if code, b := pollAuthPair(t, ts, mine.SessionID, secret); code == 200 || b["token"] != nil {
			t.Errorf("polled with %s: %d %v", name, code, b)
		}
		if w := ts.do(t, "GET", "/api/auth/pair/"+theirs.SessionID+"/pin", "", nil, pairSecretHeader, mine.DeviceSecret); w.Code == 200 {
			t.Errorf("read another session's PIN with %s", name)
		}
	}
	// The right device collects it, once.
	if code, b := pollAuthPair(t, ts, mine.SessionID, mine.DeviceSecret); code != 200 || b["status"] != "approved" {
		t.Fatalf("the device's own poll: %d %v", code, b)
	}
	c.add(4 * time.Second)
	if code, b := pollAuthPair(t, ts, mine.SessionID, mine.DeviceSecret); code == 200 {
		t.Errorf("collected twice: %v", b)
	}
}
