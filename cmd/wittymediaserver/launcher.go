// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import "os"

// modulesLauncher is this program, which also boxes a module's helper in before it runs.
func modulesLauncher() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}
