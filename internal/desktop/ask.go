// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package desktop

import (
	"strings"
)

// Ask shows a question in the system's own dialog and returns the index of the button
// chosen, or -1 when it was closed or Escape was pressed. The first button is the
// default (Enter). ErrUnavailable means there's no desktop, or no way to show one.
//
// Tests never call it: it opens a real window on someone's screen.
func Ask(title, text string, buttons []string) (int, error) {
	if len(buttons) == 0 || Headless() {
		return -1, ErrUnavailable
	}
	return ask(title, text, buttons)
}

// What the tools are given and how their answers are read, apart from running them so
// that tests can check them anywhere.

// appleScript quotes text for AppleScript: quotes and backslashes are the only special characters.
func appleScript(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// osascriptArgs: a Mac lays buttons out right to left with the default on the right, so
// the list is reversed. The last button is the cancel button (Escape).
func osascriptArgs(title, text string, buttons []string) []string {
	quoted := make([]string, len(buttons))
	for i, b := range buttons {
		quoted[len(buttons)-1-i] = appleScript(b)
	}
	if title != "" {
		text = title + "\n\n" + text
	}
	line := "display dialog " + appleScript(text) + " with title " + appleScript("WittyMediaServer") +
		" buttons {" + strings.Join(quoted, ", ") + "} default button " + appleScript(buttons[0])
	if len(buttons) > 1 {
		line += " cancel button " + appleScript(buttons[len(buttons)-1])
	}
	return []string{"-e", "activate", "-e", line}
}

// osascriptAnswer reads "button returned:<label>". A cancel button ends the script with
// an error instead, which the caller turns into -1.
func osascriptAnswer(out string, buttons []string) int {
	_, label, ok := strings.Cut(strings.TrimSpace(out), "button returned:")
	if !ok {
		return -1
	}
	label, _, _ = strings.Cut(label, ",")
	for i, b := range buttons {
		if b == label {
			return i
		}
	}
	return -1
}

// zenityArgs: OK is the first button, Cancel the last, extra buttons the ones between.
func zenityArgs(title, text string, buttons []string) []string {
	kind := "--question"
	if len(buttons) == 1 {
		kind = "--info"
	}
	args := []string{kind, "--title=WittyMediaServer", "--width=440", "--text=" + zenityText(title, text), "--ok-label=" + buttons[0]}
	if len(buttons) == 1 {
		return args
	}
	args = append(args, "--cancel-label="+buttons[len(buttons)-1])
	for _, b := range buttons[1 : len(buttons)-1] {
		args = append(args, "--extra-button="+b)
	}
	return args
}

// zenity reads Pango markup in its text.
func zenityText(title, text string) string {
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	if title == "" {
		return esc.Replace(text)
	}
	return "<b>" + esc.Replace(title) + "</b>\n\n" + esc.Replace(text)
}

// zenityAnswer: exit 0 is OK; an extra button prints its label and exits 1, as Cancel
// and Escape do without printing anything.
func zenityAnswer(code int, out string, buttons []string) int {
	if code == 0 {
		return 0
	}
	label := strings.TrimSpace(out)
	for i, b := range buttons {
		if i > 0 && b == label {
			return i
		}
	}
	if code == 1 && len(buttons) > 1 {
		return len(buttons) - 1
	}
	return -1
}

// kdialogArgs: kdialog has yes/no/cancel, which is all these questions need.
func kdialogArgs(title, text string, buttons []string) []string {
	body := text
	if title != "" {
		body = title + "\n\n" + text
	}
	switch len(buttons) {
	case 1:
		return []string{"--title", "WittyMediaServer", "--msgbox", body, "--ok-label", buttons[0]}
	case 2:
		return []string{"--title", "WittyMediaServer", "--yesno", body, "--yes-label", buttons[0], "--no-label", buttons[1]}
	}
	return []string{"--title", "WittyMediaServer", "--yesnocancel", body, "--yes-label", buttons[0], "--no-label", buttons[1], "--cancel-label", buttons[2]}
}

// kdialogAnswer: 0 yes, 1 no, 2 cancel (and Escape).
func kdialogAnswer(code int, buttons []string) int {
	switch {
	case code == 0:
		return 0
	case code == 1 && len(buttons) > 1:
		return 1
	case code == 2 && len(buttons) > 2:
		return 2
	}
	return -1
}
