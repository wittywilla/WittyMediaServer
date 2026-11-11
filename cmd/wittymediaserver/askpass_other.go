// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package main

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
)

// readHidden reads a line from the terminal without showing it. If that can't be promised it
// doesn't ask, and Ctrl-C at the prompt leaves the terminal showing what's typed again.
func readHidden() (string, error) {
	stty := func(arg string) error {
		c := exec.Command("stty", arg)
		c.Stdin = os.Stdin
		return c.Run()
	}
	if err := stty("-echo"); err != nil {
		return "", errCantHide
	}
	stop := restoreOnSignal(func() { _ = stty("echo") })
	defer stop()
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}
