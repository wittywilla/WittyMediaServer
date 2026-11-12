// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
)

// The tests that run real modules tell them which server they're on. It has to be this one,
// or a module that needs this server refuses to be added and those tests fail.
func TestModuleTestsUseThisServersVersion(t *testing.T) {
	if modtest.ServerVersion != version {
		t.Fatalf("modtest.ServerVersion is %q, and this server is %q", modtest.ServerVersion, version)
	}
}
