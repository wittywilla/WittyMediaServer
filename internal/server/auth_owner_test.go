// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
)

// turnOnTwoFactor gives an account two-factor sign-in, and hands back its recovery codes.
func turnOnTwoFactor(t *testing.T, ts *testServer, tok string) []string {
	t.Helper()
	w := ts.do(t, "POST", "/api/me/2fa/setup", tok, map[string]string{"password": "correct-horse-1"})
	var setup auth.TwoFactorSetup
	if err := json.Unmarshal(w.Body.Bytes(), &setup); err != nil {
		t.Fatal(err)
	}
	w = ts.do(t, "POST", "/api/me/2fa/confirm", tok, map[string]string{"code": auth.CurrentCode(setup.Secret)})
	var out struct {
		Recovery []string `json:"recovery"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Recovery) == 0 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	return out.Recovery
}

// A stolen administrator's session has no password or phone. It used to get around both by making
// a second administrator, which could then reset the server and delete the owner.
func TestAdminAccountChangesAskForTheOwnersPassword(t *testing.T) {
	ts := newTestServer(t)
	owner, tok := ts.user(t, "owner", "adult", true, "")
	other, _ := ts.user(t, "other", "adult", true, "")
	codes := turnOnTwoFactor(t, ts, tok)
	ctx := context.Background()

	helper := map[string]any{"name": "helper", "password": "attacker-chosen-1", "isAdmin": true}
	if w := ts.do(t, "POST", "/api/users", tok, helper); w.Code != 403 {
		t.Fatalf("a new administrator, nothing asked: %d %s", w.Code, w.Body)
	}
	helper["current"] = "correct-horse-1"
	if w := ts.do(t, "POST", "/api/users", tok, helper); w.Code != 403 {
		t.Fatalf("a new administrator, no code: %d %s", w.Code, w.Body)
	}
	for _, u := range []string{"helper"} {
		if users, _ := ts.Auth.Users(ctx); len(users) != 2 {
			t.Fatalf("%s was made: %+v", u, users)
		}
	}
	ts.logins.clear()

	pw := fmt.Sprintf("/api/users/%d/password", other.ID)
	if w := ts.do(t, "PUT", pw, tok, map[string]string{"password": "attacker-chosen-2"}); w.Code != 403 {
		t.Fatalf("another administrator's password, nothing asked: %d %s", w.Code, w.Body)
	}
	if !ts.Auth.CheckPassword(ctx, other.ID, "correct-horse-1") {
		t.Fatal("their password was changed")
	}
	del := fmt.Sprintf("/api/users/%d", other.ID)
	if w := ts.do(t, "DELETE", del, tok, nil); w.Code != 403 {
		t.Fatalf("deleting another administrator, nothing asked: %d %s", w.Code, w.Body)
	}
	ts.logins.clear()

	helper["code"] = codes[0]
	if w := ts.do(t, "POST", "/api/users", tok, helper); w.Code != 201 {
		t.Fatalf("with both: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", pw, tok, map[string]string{"password": "a chosen password", "current": "correct-horse-1", "code": codes[1]}); w.Code != 200 {
		t.Fatalf("with both: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "DELETE", del, tok, map[string]string{"current": "correct-horse-1", "code": codes[2]}); w.Code != 200 {
		t.Fatalf("with both: %d %s", w.Code, w.Body)
	}
	_ = owner
}

// Everyday changes to people who aren't administrators ask for nothing.
func TestOtherAccountChangesAskForNothing(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	kid, _ := ts.user(t, "kid", "kids", false, "")
	adult, _ := ts.user(t, "adult", "adult", false, "")
	if w := ts.do(t, "POST", "/api/users", tok, map[string]any{"name": "new", "password": "long enough pw"}); w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/users/%d/password", adult.ID), tok, map[string]string{"password": "another long one"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "DELETE", fmt.Sprintf("/api/users/%d", kid.ID), tok, nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
