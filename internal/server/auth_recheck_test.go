// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
)

// burst sends n requests at once, and counts how many got past the limit to be checked.
func burst(t *testing.T, ts *testServer, n int, want int, method, target, token string, body any) int64 {
	t.Helper()
	var wg sync.WaitGroup
	var checked atomic.Int64
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := ts.do(t, method, target, token, body); w.Code == want {
				checked.Add(1)
			}
		}()
	}
	wg.Wait()
	return checked.Load()
}

// A stolen session can't be used to guess the password behind it. A guess used to count when its
// check ended, so a burst all got checked.
func TestOwnPasswordGuessesCountAsTheyStart(t *testing.T) {
	ts := newTestServer(t)
	u, tok := ts.user(t, "owner", "adult", true, "")
	body := map[string]string{"password": "a new long password", "current": "not the password"}
	if n := burst(t, ts, 150, 403, "PUT", fmt.Sprintf("/api/users/%d/password", u.ID), tok, body); n > freeTriesPerName {
		t.Fatalf("%d guesses were checked, want at most %d", n, freeTriesPerName)
	}
	right := map[string]string{"password": "a new long password", "current": "correct-horse-1"}
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/users/%d/password", u.ID), tok, right); w.Code != 429 {
		t.Fatalf("still locked: %d %s", w.Code, w.Body)
	}
}

func TestConfirmingACodeCountsAsItStarts(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	if w := ts.do(t, "POST", "/api/me/2fa/setup", tok, map[string]string{"password": "correct-horse-1"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if n := burst(t, ts, 300, 400, "POST", "/api/me/2fa/confirm", tok, map[string]string{"code": "000000"}); n > freeTriesPerName {
		t.Fatalf("%d codes were checked, want at most %d", n, freeTriesPerName)
	}
}

// Someone who has the session and the password still can't try every code: the right password
// used to clear the count the wrong codes had built up, and codes were counted after their check.
func TestOwnCodeGuessesCountAsTheyStart(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	w := ts.do(t, "POST", "/api/me/2fa/setup", tok, map[string]string{"password": "correct-horse-1"})
	var setup auth.TwoFactorSetup
	if err := json.Unmarshal(w.Body.Bytes(), &setup); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "POST", "/api/me/2fa/confirm", tok, map[string]string{"code": auth.CurrentCode(setup.Secret)}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	wrong := map[string]string{"password": "correct-horse-1", "code": "000000"}
	if n := burst(t, ts, 100, 403, "POST", "/api/me/2fa/disable", tok, wrong); n > freeTriesPerName {
		t.Fatalf("%d codes were checked at once, want at most %d", n, freeTriesPerName)
	}
	ts.logins.clear()
	seen := 0
	for range 40 {
		w := ts.do(t, "POST", "/api/me/2fa/disable", tok, wrong)
		if w.Code == 403 && strings.Contains(w.Body.String(), "code") {
			seen++
		}
	}
	if seen > freeTriesPerName {
		t.Fatalf("%d codes were checked one after another, want at most %d", seen, freeTriesPerName)
	}
}
