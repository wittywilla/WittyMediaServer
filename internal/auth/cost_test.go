// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package auth

import (
	"context"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// Other packages' tests lower the cost with QuickHashesForTests; these don't, so a server's real
// hashes are held to bcrypt's full cost here.
func TestPasswordsHashAtTheFullCost(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	if _, err := s.CreateUser(ctx, "cost", "a long enough password", false); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := s.DB.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE name = 'cost'`).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if c, err := bcrypt.Cost([]byte(hash)); err != nil || c != bcrypt.DefaultCost {
		t.Fatalf("cost %d, %v", c, err)
	}
}
