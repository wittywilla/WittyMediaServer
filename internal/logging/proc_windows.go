// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package logging

import "syscall"

func waitReason(syscall.WaitStatus) (string, bool) { return "", false }
