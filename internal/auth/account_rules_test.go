// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package auth

import (
	"context"
	"testing"
	"time"
)

// Confirming again while it is on used to hand out new recovery codes and end the old ones, for
// someone holding only a session and a code.
func TestConfirmingAgainChangesNothing(t *testing.T) {
	ctx, s := context.Background(), newService(t)
	u, err := s.CreateUser(ctx, "Sam", "a long password", false)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := s.StartTwoFactor(ctx, u.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	codes, err := s.ConfirmTwoFactor(ctx, u.ID, CurrentCode(setup.Secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfirmTwoFactor(ctx, u.ID, CurrentCode(setup.Secret)); err == nil {
		t.Fatal("confirmed again")
	}
	if !s.CheckCode(ctx, u.ID, codes[0]) {
		t.Error("the recovery codes were replaced")
	}
}

// Names differing only in case can't both exist: sign-in ignores case and takes the oldest, so
// the later account was locked out, or signed in as the other one.
func TestNamesDifferingInCaseAreTaken(t *testing.T) {
	ctx, s := context.Background(), newService(t)
	if _, err := s.CreateProfile(ctx, "sam", "", KindKids, false, ""); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SAM", "Sam", " sam "} {
		if _, err := s.CreateUser(ctx, name, "a long password", true); err == nil {
			t.Errorf("%q was made beside sam", name)
		}
	}
	if _, err := s.CreateUser(ctx, "Samantha", "a long password", true); err != nil {
		t.Errorf("a different name: %v", err)
	}
}

// A password-free profile used to answer with no password work, so timing told a stranger which
// names are those profiles.
func TestPasswordFreeProfileTakesTheSameWork(t *testing.T) {
	ctx, s := context.Background(), newService(t)
	if _, err := s.CreateProfile(ctx, "Tapper", "", KindKids, false, ""); err != nil {
		t.Fatal(err)
	}
	fastest := func(name string) time.Duration {
		best := time.Hour
		for range 3 {
			at := time.Now()
			if _, _, err := s.Login(ctx, name, "a guess", "test", false); err == nil {
				t.Fatalf("%s signed in", name)
			}
			best = min(best, time.Since(at))
		}
		return best
	}
	unknown, free := fastest("nobody"), fastest("Tapper")
	if free < unknown/2 {
		t.Errorf("a password-free profile answers in %v, an unknown name in %v", free, unknown)
	}
}
