// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hostsandbox

import "golang.org/x/sys/unix"

// macOS has no dup3.
func dupOnto(from, to int) error { return unix.Dup2(from, to) }
