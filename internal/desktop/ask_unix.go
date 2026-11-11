// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows && !darwin

package desktop

import (
	"errors"
	"os/exec"
)

// ask uses zenity (GNOME and most others) or kdialog (KDE), whichever is installed.
func ask(title, text string, buttons []string) (int, error) {
	var cmd *exec.Cmd
	zenity := has("zenity")
	switch {
	case zenity:
		cmd = exec.Command("zenity", zenityArgs(title, text, buttons)...)
	case has("kdialog") && len(buttons) <= 3:
		cmd = exec.Command("kdialog", kdialogArgs(title, text, buttons)...)
	default:
		return -1, ErrUnavailable
	}
	out, err := cmd.Output()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return -1, ErrUnavailable
		}
		code = ee.ExitCode()
	}
	if zenity {
		return zenityAnswer(code, string(out), buttons), nil
	}
	return kdialogAnswer(code, buttons), nil
}
