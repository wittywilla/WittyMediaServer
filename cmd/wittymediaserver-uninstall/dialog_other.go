// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows && !darwin

package main

import (
	"fmt"
	"os"

	"github.com/wittywilla/wittymediaserver/internal/uninstall"
)

// On Linux and BSD the uninstaller is `wittymediaserver --uninstall` or the package manager.
// This only keeps the package building everywhere.
func show(_, text string) { fmt.Fprintln(os.Stderr, text) }

func confirm(_, text string) bool {
	fmt.Fprintln(os.Stderr, text)
	return false
}

func startWork(*uninstall.Env, uninstall.Args, bool) error {
	return fmt.Errorf("use: wittymediaserver --uninstall")
}
