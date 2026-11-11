// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package auth

import (
	"context"
	"sync"
	"testing"
)

// A sign-in that has checked a password can't finish with it once that password has changed:
// otherwise "signed out everywhere" leaves behind whoever was signing in at that moment.
func TestSignInUnderWayCannotOutliveThePassword(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	u, err := s.CreateUser(ctx, "owner", "the-first-password", true)
	if err != nil {
		t.Fatal(err)
	}
	var checked string
	if err := s.DB.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, u.ID).Scan(&checked); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.issueFor(ctx, u.ID, "before", false, &checked); err != nil {
		t.Fatalf("with the password as it was checked: %v", err)
	}
	if _, err := RecoverPassword(ctx, s.DB, "owner", "the-second-password"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.issueFor(ctx, u.ID, "after", false, &checked); err != ErrInvalid {
		t.Fatalf("a session was made for a password that's gone: %v", err)
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, u.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d sessions left, %v", n, err)
	}
}

// The same, as it really happens: sign-ins going on all through a change of password. Once it's
// done, nobody is signed in, whatever they were in the middle of.
func TestSignInsDuringARecoveryLeaveNoSession(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	u, err := s.CreateUser(ctx, "owner", "the-first-password", true)
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _, _ = s.Login(ctx, "owner", "the-first-password", "racing", false)
				}
			}
		}()
	}
	if _, err := RecoverPassword(ctx, s.DB, "owner", "the-second-password"); err != nil {
		t.Fatal(err)
	}
	close(stop)
	wg.Wait()
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, u.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d sessions outlived the old password, %v", n, err)
	}
}
