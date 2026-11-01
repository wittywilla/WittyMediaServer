//go:build darwin

package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/uninstall"
)

// script is text for AppleScript: quotes and backslashes are the only special characters.
func script(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func show(caption, text string) {
	_ = exec.Command("/usr/bin/osascript", "-e",
		"display dialog "+script(text)+" with title "+script(caption)+` buttons {"OK"} default button "OK"`).Run()
}

func confirm(caption, text string) bool {
	out, err := exec.Command("/usr/bin/osascript", "-e",
		"display dialog "+script(text)+" with title "+script(caption)+` buttons {"No", "Yes"} default button "No"`).Output()
	return err == nil && strings.Contains(string(out), "Yes")
}

func shellQuote(s string) string { return `'` + strings.ReplaceAll(s, `'`, `'\''`) + `'` }

// startWork runs the worker here and waits: on a Mac a running program can be deleted. An
// administrator's password comes from the Mac's own prompt.
func startWork(_ *uninstall.Env, a uninstall.Args, elevate bool) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	flags := a.Flags()
	if a.Yes {
		flags = append(flags, "--unattended")
	}
	if !elevate {
		cmd := exec.Command(self, flags...)
		return cmd.Run()
	}
	line := shellQuote(self)
	for _, f := range flags {
		line += " " + shellQuote(f)
	}
	if err := exec.Command("/usr/bin/osascript", "-e", "do shell script "+script(line)+" with administrator privileges").Run(); err != nil {
		return errors.New("It needs an administrator's password, and that wasn't given.")
	}
	return nil
}
