// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"os"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
)

// TestMain makes password hashes cheap for this test program: nearly every test makes accounts,
// and at bcrypt's real cost hashing is most of the package's time, many times more under the race
// detector. internal/auth's own tests keep the real cost (cost_test.go).
func TestMain(m *testing.M) {
	auth.QuickHashesForTests()
	os.Exit(m.Run())
}
