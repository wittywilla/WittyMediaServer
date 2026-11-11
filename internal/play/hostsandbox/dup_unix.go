// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build unix && !darwin

package hostsandbox

import "golang.org/x/sys/unix"

func dupOnto(from, to int) error { return unix.Dup3(from, to, 0) }
