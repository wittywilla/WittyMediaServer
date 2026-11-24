// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

const testParentPIN = "4821"

func setParentPIN(t *testing.T, ts *testServer, pin string) {
	t.Helper()
	hash := ""
	if pin != "" {
		var err error
		if hash, err = auth.HashSecret(pin); err != nil {
			t.Fatal(err)
		}
	}
	if err := ts.DB.SetSetting(context.Background(), parentPINKey, hash); err != nil {
		t.Fatal(err)
	}
}

func codeOf(t *testing.T, body []byte) string {
	t.Helper()
	var b struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(body, &b)
	return b.Code
}

// pinFixture: a kids' profile kept on its device, a sibling with no password, a grown-up with one.
func pinFixture(t *testing.T) (ts *testServer, kid, kidTok, adultTok string) {
	t.Helper()
	ts = newTestServer(t)
	ts.user(t, "owner", auth.KindAdult, true, "")
	_, adultTok = ts.user(t, "mum", auth.KindAdult, false, "")
	_, kidTok = ts.user(t, "kiddo", "kids", false, `{"maxAge":7,"liveTv":true,"libraryChannels":true,"pinToLeave":true}`)
	sib, _ := ts.user(t, "sib", "guest", false, `{"maxAge":13}`)
	if err := ts.Auth.ClearPassword(context.Background(), sib.ID); err != nil {
		t.Fatal(err)
	}
	setParentPIN(t, ts, testParentPIN)
	return ts, "kiddo", kidTok, adultTok
}

func stillSignedIn(t *testing.T, ts *testServer, tok string) bool {
	t.Helper()
	return ts.do(t, "GET", "/api/me", tok, nil).Code == 200
}

// Every way off a kids' profile kept on its device: switching to a profile with no password,
// switching with a made-up password, signing out, signing out with a second token sent along,
// and ending its own session from the devices list. Each needs the parents' PIN, and the profile
// stays signed in when it is refused.
func TestLeavingAKidsProfileNeedsTheParentsPIN(t *testing.T) {
	ts, _, kid, adult := pinFixture(t)

	w := ts.do(t, "POST", "/api/profiles/switch", kid, map[string]any{"name": "sib"})
	if w.Code != 403 || codeOf(t, w.Body.Bytes()) != pinNeededCode {
		t.Fatalf("switch to a profile with no password, no PIN: %d %s", w.Code, w.Body)
	}
	w = ts.do(t, "POST", "/api/profiles/switch", kid, map[string]any{"name": "sib", "password": "anything-at-all"})
	if w.Code != 403 || codeOf(t, w.Body.Bytes()) != pinNeededCode {
		t.Fatalf("switch with a password typed for a profile that has none: %d %s", w.Code, w.Body)
	}
	w = ts.do(t, "POST", "/api/profiles/switch", kid, map[string]any{"name": "sib", "parentPin": "0000"})
	if w.Code != 403 || codeOf(t, w.Body.Bytes()) != "wrong-pin" {
		t.Fatalf("switch with a wrong PIN: %d %s", w.Code, w.Body)
	}
	if !stillSignedIn(t, ts, kid) {
		t.Fatal("a refused switch signed the kids' profile out")
	}

	w = ts.do(t, "POST", "/api/logout", kid, nil)
	if w.Code != 403 || codeOf(t, w.Body.Bytes()) != pinNeededCode {
		t.Fatalf("sign out with no PIN: %d %s", w.Code, w.Body)
	}
	w = ts.do(t, "POST", "/api/logout", adult, nil, "Cookie", auth.CookieName+"="+kid)
	if w.Code != 403 {
		t.Fatalf("sign out with a grown-up's token in front of the kids' one: %d %s", w.Code, w.Body)
	}
	w = ts.do(t, "POST", "/api/logout", kid, map[string]any{"pin": "1234"})
	if w.Code != 403 || codeOf(t, w.Body.Bytes()) != "wrong-pin" {
		t.Fatalf("sign out with a wrong PIN: %d %s", w.Code, w.Body)
	}
	w = ts.do(t, "DELETE", "/api/sessions/anything", kid, nil)
	if w.Code != 403 || codeOf(t, w.Body.Bytes()) != pinNeededCode {
		t.Fatalf("end its own session from the devices list: %d %s", w.Code, w.Body)
	}
	if !stillSignedIn(t, ts, kid) || !stillSignedIn(t, ts, adult) {
		t.Fatal("a refused sign-out signed someone out")
	}

	// The sign-in screen, on the device the kids' profile is still signed in on: tapping a
	// profile with no password is leaving it too.
	w = ts.do(t, "POST", "/api/login", "", map[string]any{"name": "sib"}, "Cookie", auth.CookieName+"="+kid)
	if w.Code != 403 || codeOf(t, w.Body.Bytes()) != pinNeededCode {
		t.Fatalf("sign in to a profile with no password from the kids' device: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/login", "", map[string]any{"name": "sib", "parentPin": testParentPIN}, "Cookie", auth.CookieName+"="+kid); w.Code != 200 {
		t.Fatalf("the same with the PIN: %d %s", w.Code, w.Body)
	}

	// A grown-up's own password is a parent at the device: no PIN asked for.
	w = ts.do(t, "POST", "/api/profiles/switch", kid, map[string]any{"name": "mum", "password": "correct-horse-1"})
	if w.Code != 200 {
		t.Fatalf("switch to a grown-up with their password: %d %s", w.Code, w.Body)
	}
	if stillSignedIn(t, ts, kid) {
		t.Fatal("the switch left the kids' session open")
	}
}

func TestTheParentsPINLetsAKidsProfileBeLeft(t *testing.T) {
	ts, _, kid, _ := pinFixture(t)
	w := ts.do(t, "POST", "/api/profiles/switch", kid, map[string]any{"name": "sib", "parentPin": testParentPIN})
	if w.Code != 200 {
		t.Fatalf("switch with the PIN: %d %s", w.Code, w.Body)
	}
	_, kid2 := ts.user(t, "kiddo2", "kids", false, `{"maxAge":7,"pinToLeave":true}`)
	if w := ts.do(t, "POST", "/api/logout", kid2, map[string]any{"pin": testParentPIN}); w.Code != 200 {
		t.Fatalf("sign out with the PIN: %d %s", w.Code, w.Body)
	}
	if stillSignedIn(t, ts, kid2) {
		t.Fatal("still signed in after signing out with the PIN")
	}
}

// Without a PIN set there is nothing to ask for, and a profile that isn't kept on its device is
// left as before.
func TestLeavingIsAsBeforeWithoutAPINOrTheChoice(t *testing.T) {
	ts, _, kid, _ := pinFixture(t)
	_, free := ts.user(t, "freekid", "kids", false, `{"maxAge":7}`)
	if w := ts.do(t, "POST", "/api/logout", free, nil); w.Code != 200 {
		t.Fatalf("a kids' profile not kept on its device: %d %s", w.Code, w.Body)
	}
	setParentPIN(t, ts, "")
	if w := ts.do(t, "POST", "/api/logout", kid, nil); w.Code != 200 {
		t.Fatalf("no PIN set: %d %s", w.Code, w.Body)
	}
}

// Guessing the PIN through leaving is counted like guessing it anywhere: the tries run out.
func TestTheLeavingPINCantBeGuessed(t *testing.T) {
	ts, _, kid, _ := pinFixture(t)
	limited := false
	for i := 0; i < 40 && !limited; i++ {
		w := ts.do(t, "POST", "/api/profiles/switch", kid, map[string]any{"name": "sib", "parentPin": fmt.Sprintf("9%03d", i)})
		if w.Code == 200 {
			t.Fatalf("a guess got through at try %d", i)
		}
		limited = w.Code == 429
	}
	if !limited {
		t.Fatal("40 wrong PINs and still no limit")
	}
	// The right PIN waits too, so the limit can't be told from a wrong guess.
	if w := ts.do(t, "POST", "/api/logout", kid, map[string]any{"pin": testParentPIN}); w.Code != 429 {
		t.Fatalf("the right PIN straight after: %d", w.Code)
	}
}

// A parent changes a kids' profile's screen time and age limit on the child's device, with the
// PIN. The profile can't do it without, can't change anything else that way, and a grown-up's
// profile is changed in Settings, not here.
func TestChangingLimitsOnTheChildsDevice(t *testing.T) {
	ts, _, kid, adult := pinFixture(t)
	change := map[string]any{"time": map[string]any{"schoolDayMinutes": 45, "schoolNightUntil": "19:00"}, "maxAge": 10,
		"off": []string{}, "libraries": []int64{}, "tunerChannels": true, "pinToLeave": false}

	if w := ts.do(t, "PUT", "/api/me/limits", kid, change); w.Code != 403 || codeOf(t, w.Body.Bytes()) != pinNeededCode {
		t.Fatalf("no PIN: %d %s", w.Code, w.Body)
	}
	change["pin"] = "1111"
	if w := ts.do(t, "PUT", "/api/me/limits", kid, change); w.Code != 403 || codeOf(t, w.Body.Bytes()) != "wrong-pin" {
		t.Fatalf("a wrong PIN: %d %s", w.Code, w.Body)
	}
	change["pin"] = testParentPIN
	w := ts.do(t, "PUT", "/api/me/limits", kid, change)
	if w.Code != 200 {
		t.Fatalf("the PIN: %d %s", w.Code, w.Body)
	}
	u, err := ts.Auth.User(context.Background(), userIDOf(t, ts, kid))
	if err != nil {
		t.Fatal(err)
	}
	a := accessFor(u)
	if a.MaxAge != 10 || a.Time == nil || a.Time.SchoolDayMinutes != 45 || a.Time.SchoolNightUntil != "19:00" {
		t.Fatalf("not changed as asked: %+v %+v", a, a.Time)
	}
	if !a.PinToLeave || a.TunerChannels {
		t.Fatalf("changed more than time and age: %+v", a)
	}
	if w := ts.do(t, "PUT", "/api/me/limits", adult, change); w.Code != 403 {
		t.Fatalf("a grown-up's profile: %d %s", w.Code, w.Body)
	}
	bad := map[string]any{"pin": testParentPIN, "time": map[string]any{"schoolNightUntil": "25:00"}}
	if w := ts.do(t, "PUT", "/api/me/limits", kid, bad); w.Code != 400 {
		t.Fatalf("a bedtime that isn't a time: %d %s", w.Code, w.Body)
	}
}

func userIDOf(t *testing.T, ts *testServer, tok string) int64 {
	t.Helper()
	var v struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(ts.do(t, "GET", "/api/me", tok, nil).Body.Bytes(), &v); err != nil || v.ID == 0 {
		t.Fatalf("who: %v", err)
	}
	return v.ID
}

// A kids' profile made now starts kid-safe; one saved before keeps what it had.
func TestNewKidsProfilesStartKidSafe(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", auth.KindAdult, true, "")
	w := ts.do(t, "POST", "/api/users", admin, map[string]any{"name": "newkid", "password": "1234", "kind": "kids"})
	if w.Code != 201 {
		t.Fatalf("make a kids' profile: %d %s", w.Code, w.Body)
	}
	var made struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &made); err != nil || made.ID == 0 {
		t.Fatalf("%s %v", w.Body, err)
	}
	u, err := ts.Auth.User(context.Background(), made.ID)
	if err != nil {
		t.Fatal(err)
	}
	a := accessFor(u)
	if !a.PinToLeave || a.TunerChannels || a.Time.Empty() || a.Can(library.FeatureRemote) || a.Can(library.FeatureDownloads) ||
		a.Can(library.FeatureSharing) || a.Can(library.FeatureRecordings) || a.Kinds["music"].MaxAge != 12 || !a.Kinds["music"].AllowUnrated ||
		a.Kinds["game"].MaxAge != 7 || a.Kinds["book"].AllowUnrated {
		t.Fatalf("not kid-safe: %+v", a)
	}
	// Explicit songs are kept from it; songs with no rating aren't.
	if a.AllowsOutside("audio", "Explicit", nil) || !a.AllowsOutside("audio", "", nil) || !a.AllowsOutside("audio", "Clean", nil) {
		t.Fatal("music limits")
	}
	// Limits saved before kid-safe defaults existed mean what they did.
	old, _ := ts.user(t, "oldkid", "kids", false, `{"maxAge":7,"liveTv":true,"libraryChannels":true,"tunerChannels":true}`)
	if oa := accessFor(old); oa.PinToLeave || oa.Time != nil || !oa.Can(library.FeatureRemote) || !oa.TunerChannels {
		t.Fatalf("an older profile changed: %+v", oa)
	}
}
