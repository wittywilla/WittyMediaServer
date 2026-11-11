// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build darwin

package desktop

import (
	"errors"
	"os/exec"
)

// ask uses AppleScript's own dialog.
func ask(title, text string, buttons []string) (int, error) {
	out, err := exec.Command("/usr/bin/osascript", osascriptArgs(title, text, buttons)...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return -1, nil // the cancel button, or Escape: error -128
		}
		return -1, ErrUnavailable
	}
	return osascriptAnswer(string(out), buttons), nil
}
