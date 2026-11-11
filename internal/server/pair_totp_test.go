// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/totp"
)

type authPairStarted struct {
	SessionID    string `json:"sessionId"`
	DeviceSecret string `json:"deviceSecret"`
	PIN          string `json:"pin"`
	PINExpiresAt int64  `json:"pinExpiresAt"`
	Period       int    `json:"period"`
	QR           string `json:"qr"`
	ApproveURL   string `json:"approveUrl"`
	Code         string `json:"code"`
	ExpiresAt    int64  `json:"expiresAt"`
}

// fakeClock is a clock tests move by hand.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// withClock gives the pairing store a clock at the start of a TOTP step.
func withClock(ts *testServer) *fakeClock {
	c := &fakeClock{t: time.Unix(1_900_000_020, 0)} // 1_900_000_020 is a multiple of 30
	ts.pair.mu.Lock()
	ts.pair.clock = c.now
	ts.pair.mu.Unlock()
	return c
}

func startAuthPair(t *testing.T, ts *testServer) authPairStarted {
	t.Helper()
	w := ts.do(t, "POST", "/api/v1/auth/pair", "", map[string]string{"device": "Living room TV", "platform": "androidtv"})
	if w.Code != 200 {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	return decode[authPairStarted](t, w)
}

// pinAt is the PIN for a session's secret, at a step offset from now.
func pinAt(ts *testServer, c *fakeClock, id string, steps int64) string {
	ts.pair.mu.Lock()
	defer ts.pair.mu.Unlock()
	return totp.At(ts.pair.byID[id].pinKey, totp.Step(c.now())+steps)
}

func approvePIN(t *testing.T, ts *testServer, tok, id, pin string) (int, map[string]any) {
	t.Helper()
	w := ts.do(t, "POST", "/api/auth/pair/"+id+"/approve", tok, map[string]any{"pin": pin})
	return w.Code, decode[map[string]any](t, w)
}

func pollAuthPair(t *testing.T, ts *testServer, id, secret string) (int, map[string]any) {
	t.Helper()
	ts.pair.mu.Lock()
	if p := ts.pair.byID[id]; p != nil {
		p.lastPoll = time.Time{}
	}
	ts.pair.mu.Unlock()
	w := ts.do(t, "GET", "/api/auth/pair/"+id, "", nil, pairSecretHeader, secret)
	return w.Code, decode[map[string]any](t, w)
}

func TestAuthPairHappyPath(t *testing.T) {
	ts := newTestServer(t)
	me, tok := ts.user(t, "alex", "adult", false, "")
	c := withClock(ts)
	p := startAuthPair(t, ts)
	if len(p.SessionID) != 32 || len(p.DeviceSecret) != 64 || len(p.PIN) != 6 || p.Period != 30 {
		t.Fatalf("start: %+v", p)
	}
	if p.ExpiresAt != c.now().Add(5*time.Minute).Unix() || p.PINExpiresAt != c.now().Add(30*time.Second).Unix() {
		t.Errorf("times: %+v", p)
	}
	if p.ApproveURL != "http://127.0.0.1:9797/pair?code="+p.Code {
		t.Errorf("approveUrl %q", p.ApproveURL)
	}
	// The QR code names this server, where it is, and the session, with a check.
	u, err := url.Parse(p.QR)
	if err != nil || u.Scheme != "wms" || u.Host != "pair" {
		t.Fatalf("qr %q", p.QR)
	}
	q := u.Query()
	id, _ := ts.Identity()
	if q.Get("v") != "1" || q.Get("s") != id.ID || q.Get("u") != "http://127.0.0.1:9797" || q.Get("c") != p.SessionID || len(q.Get("k")) != 16 {
		t.Errorf("qr fields: %v", q)
	}
	// The PIN secret never leaves the server.
	if strings.Contains(fmt.Sprint(p), "pinKey") {
		t.Error("secret in the answer")
	}

	// The display can read the current PIN, with its secret only.
	if w := ts.do(t, "GET", "/api/auth/pair/"+p.SessionID+"/pin", "", nil); w.Code != 400 {
		t.Errorf("pin without a secret: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/auth/pair/"+p.SessionID+"/pin", "", nil, pairSecretHeader, strings.Repeat("a", 64)); w.Code != 404 {
		t.Errorf("pin with the wrong secret: %d", w.Code)
	}
	c.add(31 * time.Second)
	w := ts.do(t, "GET", "/api/v1/auth/pair/"+p.SessionID+"/pin", "", nil, pairSecretHeader, p.DeviceSecret)
	got := decode[map[string]any](t, w)
	if w.Code != 200 || got["pin"] != pinAt(ts, c, p.SessionID, 0) || got["pin"] == p.PIN {
		t.Fatalf("pin rotated: %d %v (first %s)", w.Code, got, p.PIN)
	}

	if code, b := pollAuthPair(t, ts, p.SessionID, p.DeviceSecret); code != 200 || b["status"] != "pending" {
		t.Fatalf("pending: %d %v", code, b)
	}
	if code, b := approvePIN(t, ts, tok, p.SessionID, got["pin"].(string)); code != 200 || b["ok"] != true {
		t.Fatalf("approve: %d %v", code, b)
	}
	code, b := pollAuthPair(t, ts, p.SessionID, p.DeviceSecret)
	if code != 200 || b["status"] != "approved" || len(fmt.Sprint(b["token"])) != 64 {
		t.Fatalf("approved: %d %v", code, b)
	}
	if w := ts.do(t, "GET", "/api/me", b["token"].(string), nil); w.Code != 200 || decode[map[string]any](t, w)["name"] != me.Name {
		t.Errorf("token: %d %s", w.Code, w.Body)
	}
	// Collected once.
	if code, b := pollAuthPair(t, ts, p.SessionID, p.DeviceSecret); code != 410 || b["token"] != nil {
		t.Errorf("second collection: %d %v", code, b)
	}
}

func TestAuthPairPINWindow(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "alex", "adult", false, "")
	c := withClock(ts)

	// The previous step's PIN is still accepted.
	p := startAuthPair(t, ts)
	c.add(30 * time.Second)
	if code, b := approvePIN(t, ts, tok, p.SessionID, pinAt(ts, c, p.SessionID, -1)); code != 200 {
		t.Fatalf("previous step: %d %v", code, b)
	}

	// Two steps old is not.
	p = startAuthPair(t, ts)
	old := pinAt(ts, c, p.SessionID, 0)
	c.add(60 * time.Second)
	if old == pinAt(ts, c, p.SessionID, -1) || old == pinAt(ts, c, p.SessionID, 0) || old == pinAt(ts, c, p.SessionID, 1) {
		t.Skip("the old PIN happens to equal one in the window")
	}
	code, b := approvePIN(t, ts, tok, p.SessionID, old)
	if code != 400 || b["reason"] != pinWrong || b["triesLeft"] != float64(4) {
		t.Errorf("two steps old: %d %v", code, b)
	}
}

func TestAuthPairReplay(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "alex", "adult", false, "")
	c := withClock(ts)
	p := startAuthPair(t, ts)
	pin := pinAt(ts, c, p.SessionID, 0)
	if code, _ := approvePIN(t, ts, tok, p.SessionID, pin); code != 200 {
		t.Fatal(code)
	}
	if code, b := approvePIN(t, ts, tok, p.SessionID, pin); code != 410 || b["reason"] != pinUsed {
		t.Errorf("the same PIN again: %d %v", code, b)
	}
	// Even if the session were somehow waiting again, the step is spent.
	ts.pair.mu.Lock()
	ts.pair.byID[p.SessionID].userID = 0
	ts.pair.mu.Unlock()
	if code, b := approvePIN(t, ts, tok, p.SessionID, pin); code != 400 || b["reason"] != pinWrong {
		t.Errorf("replayed step: %d %v", code, b)
	}
}

func TestAuthPairLocksAfterFiveWrongPINs(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "alex", "adult", false, "")
	_, tok2 := ts.user(t, "robin", "adult", false, "")
	c := withClock(ts)
	p := startAuthPair(t, ts)
	right := pinAt(ts, c, p.SessionID, 0)
	wrong := func(i int) string {
		for n := i; ; n++ {
			s := fmt.Sprintf("%06d", n)
			if s != right && s != pinAt(ts, c, p.SessionID, -1) && s != pinAt(ts, c, p.SessionID, 1) {
				return s
			}
		}
	}
	// Spread across two people, so the per-person limit isn't what stops it.
	for i := range authPairMaxTries {
		who := tok
		if i%2 == 1 {
			who = tok2
		}
		code, b := approvePIN(t, ts, who, p.SessionID, wrong(i*7))
		if i < authPairMaxTries-1 && (code != 400 || b["triesLeft"] != float64(authPairMaxTries-1-i)) {
			t.Fatalf("try %d: %d %v", i, code, b)
		}
		if i == authPairMaxTries-1 && (code != 410 || b["reason"] != pinLocked) {
			t.Fatalf("fifth: %d %v", code, b)
		}
	}
	if code, b := approvePIN(t, ts, tok2, p.SessionID, right); code != 410 || b["reason"] != pinLocked {
		t.Errorf("the right PIN after the lock: %d %v", code, b)
	}
	if w := ts.do(t, "GET", "/api/auth/pair/"+p.SessionID+"/pin", "", nil, pairSecretHeader, p.DeviceSecret); w.Code != 410 {
		t.Errorf("pin after lock: %d", w.Code)
	}
	if code, b := pollAuthPair(t, ts, p.SessionID, p.DeviceSecret); code != 410 || b["status"] != "locked" {
		t.Errorf("device told: %d %v", code, b)
	}
}

func TestAuthPairExpires(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "alex", "adult", false, "")
	c := withClock(ts)
	p := startAuthPair(t, ts)
	c.add(5*time.Minute + time.Second)
	if code, b := approvePIN(t, ts, tok, p.SessionID, pinAt(ts, c, p.SessionID, 0)); code != 410 || b["reason"] != pinExpired {
		t.Errorf("approve after expiry: %d %v", code, b)
	}
	if w := ts.do(t, "GET", "/api/auth/pair/"+p.SessionID+"/pin", "", nil, pairSecretHeader, p.DeviceSecret); w.Code != 410 {
		t.Errorf("pin after expiry: %d", w.Code)
	}
	if code, _ := pollAuthPair(t, ts, p.SessionID, p.DeviceSecret); code != 410 {
		t.Errorf("poll after expiry: %d", code)
	}
}

func TestAuthPairNeedsASignedInAdult(t *testing.T) {
	ts := newTestServer(t)
	_, adult := ts.user(t, "alex", "adult", false, "")
	_, kid := ts.user(t, "sam", "kids", false, "")
	_, guest := ts.user(t, "visitor", "guest", false, "")
	c := withClock(ts)
	p := startAuthPair(t, ts)
	pin := pinAt(ts, c, p.SessionID, 0)
	if w := ts.do(t, "POST", "/api/auth/pair/"+p.SessionID+"/approve", "", map[string]any{"pin": pin}); w.Code != 401 {
		t.Errorf("anonymous: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/auth/pair/"+p.SessionID+"/approve", p.DeviceSecret, map[string]any{"pin": pin}); w.Code != 401 {
		t.Errorf("the device's secret as a token: %d", w.Code)
	}
	for name, tok := range map[string]string{"kids": kid, "guest": guest} {
		if w := ts.do(t, "POST", "/api/auth/pair/"+p.SessionID+"/approve", tok, map[string]any{"pin": pin}); w.Code != 403 {
			t.Errorf("%s approve: %d", name, w.Code)
		}
		if w := ts.do(t, "POST", "/api/auth/pair/"+p.SessionID+"/deny", tok, nil); w.Code != 403 {
			t.Errorf("%s deny: %d", name, w.Code)
		}
	}
	// Signing in as a profile one doesn't manage.
	other, _ := ts.user(t, "robin", "adult", false, "")
	if w := ts.do(t, "POST", "/api/auth/pair/"+p.SessionID+"/approve", adult, map[string]any{"pin": pin, "userId": other.ID}); w.Code != 403 {
		t.Errorf("as someone else: %d", w.Code)
	}
	if code, b := pollAuthPair(t, ts, p.SessionID, p.DeviceSecret); code != 200 || b["status"] != "pending" {
		t.Fatalf("still waiting: %d %v", code, b)
	}
	// The old route can't skip the PIN.
	if w := ts.do(t, "POST", "/api/pair/approve", adult, map[string]any{"code": p.Code}); w.Code != 400 {
		t.Errorf("code without the PIN: %d %s", w.Code, w.Body)
	}
	// A wrong QR check is refused, the right one passes.
	if code, b := approvePINCheck(t, ts, adult, p.SessionID, pin, "AAAAAAAAAAAAAAAA"); code != 400 || b["reason"] != pinBadCheck {
		t.Errorf("bad check: %d %v", code, b)
	}
	if w := ts.do(t, "POST", "/api/auth/pair/"+p.SessionID+"/deny", adult, nil); w.Code != 200 {
		t.Fatalf("deny: %d", w.Code)
	}
	if code, b := pollAuthPair(t, ts, p.SessionID, p.DeviceSecret); code != 403 || b["status"] != "denied" {
		t.Errorf("denied: %d %v", code, b)
	}
}

func approvePINCheck(t *testing.T, ts *testServer, tok, id, pin, check string) (int, map[string]any) {
	t.Helper()
	w := ts.do(t, "POST", "/api/auth/pair/"+id+"/approve", tok, map[string]any{"pin": pin, "check": check})
	return w.Code, decode[map[string]any](t, w)
}

// The web page approves with the code and the PIN, through the old route.
func TestAuthPairApprovedFromThePage(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "alex", "adult", false, "")
	c := withClock(ts)
	p := startAuthPair(t, ts)
	w := ts.do(t, "POST", "/api/pair/info", tok, map[string]string{"code": p.Code})
	if info := decode[map[string]any](t, w); w.Code != 200 || info["needsPin"] != true {
		t.Fatalf("info: %d %v", w.Code, info)
	}
	q, _ := url.Parse(p.QR)
	if code, b := approvePINCheck(t, ts, tok, p.SessionID, pinAt(ts, c, p.SessionID, 0), q.Query().Get("k")); code != 200 {
		t.Fatalf("approve with the QR's check: %d %v", code, b)
	}
	p = startAuthPair(t, ts)
	if w := ts.do(t, "POST", "/api/pair/approve", tok, map[string]any{"code": p.Code, "pin": pinAt(ts, c, p.SessionID, 0)}); w.Code != 200 {
		t.Fatalf("page approve: %d %s", w.Code, w.Body)
	}
}

func TestAuthPairRateLimits(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "alex", "adult", false, "")
	withClock(ts)
	// Starting shares the plain codes' limits on open sessions per address.
	for range pairMaxPerIP {
		startAuthPair(t, ts)
	}
	if w := ts.do(t, "POST", "/api/auth/pair", "", map[string]string{"device": "TV"}); w.Code != 429 {
		t.Errorf("past the limit: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/pair/start", "", map[string]string{"device": "TV"}); w.Code != 429 {
		t.Errorf("a plain code past the shared limit: %d", w.Code)
	}
	// Guessing at sessions is limited per person, as with codes.
	for i := range 60 {
		w := ts.do(t, "POST", "/api/auth/pair/"+fmt.Sprintf("%032x", i)+"/approve", tok, map[string]any{"pin": "123456"})
		if w.Code == 429 {
			if i == 0 {
				t.Fatal("locked at once")
			}
			return
		}
		if w.Code != 404 {
			t.Fatalf("unknown session: %d", w.Code)
		}
	}
	t.Fatal("never limited")
}

// Many devices and approvers at once: run with -race.
func TestAuthPairConcurrent(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "alex", "adult", false, "")
	var wg sync.WaitGroup
	for i := range pairMaxPerIP {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := ts.do(t, "POST", "/api/auth/pair", "", map[string]string{"device": fmt.Sprint("TV ", i)})
			if w.Code != 200 {
				t.Errorf("start: %d", w.Code)
				return
			}
			p := decode[authPairStarted](t, w)
			for range 3 {
				ts.do(t, "GET", "/api/auth/pair/"+p.SessionID+"/pin", "", nil, pairSecretHeader, p.DeviceSecret)
				ts.do(t, "GET", "/api/auth/pair/"+p.SessionID, "", nil, pairSecretHeader, p.DeviceSecret)
			}
			ts.do(t, "POST", "/api/auth/pair/"+p.SessionID+"/approve", tok, map[string]any{"pin": p.PIN})
		}()
	}
	wg.Wait()
}
