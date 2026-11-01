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
	if err := windows.GetConsoleMode(h, &mode); err == nil {
		if err := windows.SetConsoleMode(h, mode&^windows.ENABLE_ECHO_INPUT); err == nil {
			defer func() { _ = windows.SetConsoleMode(h, mode) }()
		}
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}
