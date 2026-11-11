// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !linux

package libtranscode

import "os/exec"

// dieWithServer has no equivalent here. A conversion left running by a crashed server ends
// when it ends, writing to a temporary file that Recover has already removed.
func dieWithServer(*exec.Cmd) {}
