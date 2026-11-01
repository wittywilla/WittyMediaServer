//go:build !windows

package main

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
)

// readHidden reads a line from the terminal without showing it.
func readHidden() (string, error) {
	stty := func(arg string) error {
		c := exec.Command("stty", arg)
		c.Stdin = os.Stdin
		return c.Run()
	}
	if stty("-echo") == nil {
		defer func() { _ = stty("echo") }()
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}
