// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package main

// runAsService is for Windows, where the service manager starts programs its own way.
func runAsService([]string) (int, bool) { return 0, false }
