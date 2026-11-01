//go:build windows

package main

import (
	"errors"

	"golang.org/x/sys/windows"

	"github.com/wittywilla/wittymediaserver/internal/uninstall"
)

func box(caption, text string, flags uint32) int32 {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString(caption)
	r, _ := windows.MessageBox(0, t, c, flags|windows.MB_SETFOREGROUND)
	return r
}

func show(caption, text string) { box(caption, text, windows.MB_OK|windows.MB_ICONINFORMATION) }

const idYes = 6

func confirm(caption, text string) bool {
	return box(caption, text, windows.MB_YESNO|windows.MB_ICONQUESTION) == idYes
}

// startWork hands over to a copy in the temp folder and ends, so its own file can go.
func startWork(_ *uninstall.Env, a uninstall.Args, elevate bool) error {
	flags := a.Flags()
	if a.Yes {
		flags = append(flags, "--unattended")
	}
	err := uninstall.StartWorker(flags, elevate)
	if err != nil && elevate {
		return errors.New("It needs an administrator's permission, and that wasn't given.")
	}
	return err
}
