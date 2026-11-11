// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"bufio"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// readHidden reads a line from the console without showing it.
func readHidden() (string, error) {
	h := windows.Handle(os.Stdin.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return "", errCantHide
	}
	if err := windows.SetConsoleMode(h, mode&^windows.ENABLE_ECHO_INPUT); err != nil {
		return "", errCantHide
	}
	stop := restoreOnSignal(func() { _ = windows.SetConsoleMode(h, mode) })
	defer stop()
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}
