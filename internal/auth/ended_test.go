// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package auth

import (
	"context"
	"testing"
)

// Every way a sign-in is ended on purpose is announced, so the server can end what it had open.
func TestEndedSignInsAreAnnounced(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	u, err := s.CreateProfile(ctx, "anna", "correct-horse-1", KindAdult, false, "")
	if err != nil {
		t.Fatal(err)
	}
	var got []SessionEnd
	s.OnEnd = func(e SessionEnd) { got = append(got, e) }
	login := func() string {
		tok, _, err := s.Login(ctx, "anna", "correct-horse-1", "test", false)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	last := func(what string, want SessionEnd) {
		t.Helper()
		if len(got) == 0 || got[len(got)-1] != want {
			t.Fatalf("%s: announced %+v, want %+v", what, got, want)
		}
		got = nil
	}
	a, b, c := login(), login(), login()
	if len(got) != 0 {
		t.Fatalf("signing in announced an end: %+v", got)
	}
	s.Logout(ctx, a)
	last("signing out", SessionEnd{Session: SessionID(a)})
	if ok, err := s.EndSession(ctx, u.ID, SessionID(b)); !ok || err != nil {
		t.Fatal(ok, err)
	}
	last("a device signed out from another", SessionEnd{Session: SessionID(b)})
	if ok, _ := s.EndSession(ctx, u.ID, "no-such-session"); ok || len(got) != 0 {
		t.Fatalf("ending a sign-in that isn't there: %v %+v", ok, got)
	}
	if err := s.EndSessions(ctx, u.ID, c); err != nil {
		t.Fatal(err)
	}
	last("signing out everywhere else", SessionEnd{UserID: u.ID, Keep: SessionID(c)})
	if _, err := s.Resolve(ctx, c); err != nil {
		t.Fatalf("the sign-in that was kept: %v", err)
	}
	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	last("the account deleted", SessionEnd{UserID: u.ID})
}
