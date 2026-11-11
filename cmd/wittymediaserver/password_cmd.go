// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

const resetPasswordUsage = `wittymediaserver reset-password: a new password for an account, for someone who can't sign in.
It runs on the machine with the server and writes to the server's files, so it only works for
someone who could already do that. The server has no address that does this. The account is
signed out everywhere, and its two-factor sign-in stays as it was.

  wittymediaserver reset-password [-data <dir>] <username>
                                  asks for the new password, twice, without showing it
    --generate                    makes a random password and shows it once, instead of asking
    --password-stdin              reads it from one line of input, for a script
  wittymediaserver reset-password --list [-data <dir>]
                                  every account's username, in case that's forgotten too

A password can't be given on the command line, where the system's list of running programs
and your shell's history could keep it. Run this as the account that runs the server. It works
with the server running or stopped.`

// cmdResetPassword is also `wittymediaserver doctor reset-password`.
func cmdResetPassword(args []string) int {
	return resetPassword(args, os.Stdin, os.Stdout, os.Stderr, askTwice)
}

// askTwice asks at the terminal, so that a slip of the finger can't lock anyone out again.
func askTwice(stderr io.Writer) (string, error) {
	fmt.Fprint(stderr, "New password: ")
	first, err := readHidden()
	fmt.Fprintln(stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(stderr, "The same again: ")
	second, err := readHidden()
	fmt.Fprintln(stderr)
	if err != nil {
		return "", err
	}
	if first != second {
		return "", errors.New("those two didn't match, so nothing was changed")
	}
	return first, nil
}

func resetPassword(args []string, stdin io.Reader, stdout, stderr io.Writer, ask func(io.Writer) (string, error)) int {
	var list, generate, fromStdin bool
	var positional, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" { // what follows is a username, even one that starts with a dash
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") { // a username can be "list" or "help" too
			positional = append(positional, a)
			continue
		}
		switch strings.TrimLeft(a, "-") {
		case "list":
			list = true
		case "generate":
			generate = true
		case "password-stdin":
			fromStdin = true
		case "help", "h":
			fmt.Fprintln(stdout, resetPasswordUsage)
			return 0
		default:
			rest = append(rest, a)
			if !strings.Contains(a, "=") && i+1 < len(args) {
				i++
				rest = append(rest, args[i])
			}
		}
	}
	if (list && len(positional) != 0) || (!list && len(positional) != 1) || (generate && fromStdin) {
		fmt.Fprintln(stderr, "usage: wittymediaserver reset-password [-data <dir>] [--generate | --password-stdin] <username>\n       wittymediaserver reset-password --list [-data <dir>]\nA second word here may mean a password was typed: passwords aren't taken on the command line.")
		return 2
	}
	env, err := doctorEnv(rest)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	// Before the database is opened at all: opening it can already make its journal files.
	if err := dataOwnerOK(env.Data); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	// Not the usual db.Open: that makes a new, empty database in a folder that was mistyped.
	d, err := db.OpenExisting(env.Data, false)
	if err != nil {
		fmt.Fprintln(stderr, recoveryOpenError(env.Data, err))
		return 1
	}
	defer d.Close()
	ctx := context.Background()
	if list {
		accounts, err := auth.RecoverableAccounts(ctx, d)
		if err != nil {
			fmt.Fprintln(stderr, recoveryOpenError(env.Data, err))
			return 1
		}
		if len(accounts) == 0 {
			fmt.Fprintln(stdout, "This server has no accounts yet. First-time setup makes the first one.")
			return 0
		}
		for _, a := range accounts {
			fmt.Fprintf(stdout, "%-42q %s\n", a.Name, describeAccount(a)) // quoted: a name can hold anything
		}
		return 0
	}
	name := positional[0]
	password := ""
	switch {
	case generate:
		password, err = auth.TemporaryPassword()
	case fromStdin:
		line, rerr := bufio.NewReader(io.LimitReader(stdin, maxPasswordInput)).ReadString('\n')
		if rerr != nil && !errors.Is(rerr, io.EOF) {
			err = rerr
		}
		password = strings.TrimRight(line, "\r\n")
	default:
		// Whether the name is right is known before anyone types a password for it.
		accounts, lerr := auth.RecoverableAccounts(ctx, d)
		if lerr != nil {
			fmt.Fprintln(stderr, recoveryOpenError(env.Data, lerr))
			return 1
		}
		found := false
		for _, a := range accounts {
			if strings.EqualFold(a.Name, strings.TrimSpace(name)) {
				found = true
				fmt.Fprintf(stderr, "A new password for %q (%s). At least %d characters.\n", a.Name, describeAccount(a), auth.MinimumPasswordFor(a.Kind))
			}
		}
		if !found {
			fmt.Fprintf(stderr, "There's no account called %q. wittymediaserver reset-password --list shows them.\n", name)
			return 1
		}
		password, err = ask(stderr)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	a, err := auth.RecoverPassword(ctx, d, name, password)
	if errors.Is(err, auth.ErrNoAccount) {
		fmt.Fprintf(stderr, "There's no account called %q. wittymediaserver reset-password --list shows them.\n", name)
		return 1
	}
	if filesDenied(err) {
		fmt.Fprintln(stderr, recoveryOpenError(env.Data, err))
		return 1
	}
	if err != nil {
		fmt.Fprintln(stderr, "Nothing was changed:", err)
		return 1
	}
	recoveryLog(env.LogDir, "a password was reset from the command line", a.Name)
	if generate {
		fmt.Fprintf(stdout, "The new password for %q, shown only this once:\n\n    %s\n\nSign in with it, then change it under Settings, Profiles.\n", a.Name, password)
	} else {
		fmt.Fprintf(stdout, "The password for %q is changed. It's signed out everywhere.\n", a.Name)
		if auth.WeakPassword(a.Name, password) {
			fmt.Fprintln(stdout, "That password would be easy to guess, which matters if this server can be reached from the internet.")
		}
	}
	if a.TwoFactor {
		fmt.Fprintf(stdout, "Two-factor sign-in is still on. If the phone and recovery codes are gone too: wittymediaserver reset-2fa %q\n", a.Name)
	}
	fmt.Fprintln(stdout, "If the server says there were too many wrong tries, wait for the time it gives, or restart it.")
	return 0
}

// maxPasswordInput is far more than any password: what's after it can't be one anyway.
const maxPasswordInput = 4 << 10

// recoveryLog puts it in the server's own log, where WittyLog shows it, so a change made here
// can be seen later. Nothing about the password goes in. If the log can't be written the change
// has still been made, and that's said on the screen.
func recoveryLog(dir, what, name string) {
	logs, err := logging.New(dir, slog.LevelInfo, nil)
	if err != nil {
		return
	}
	defer logs.Close()
	slog.New(logs.Handler()).Warn(what, "cat", "auth", "account", name)
}

func describeAccount(a auth.RecoverableAccount) string {
	what := "Adult"
	switch {
	case a.IsAdmin:
		what = "Administrator"
	case a.Kind == auth.KindKids:
		what = "Kids"
	case a.Kind == auth.KindGuest:
		what = "Guest"
	}
	if a.TwoFactor {
		what += ", two-factor on"
	}
	return what
}

// recoveryOpenError says what's in the way in terms of what someone can do about it.
func recoveryOpenError(data string, err error) string {
	switch {
	case errors.Is(err, db.ErrNoDatabase):
		return fmt.Sprintf("There's no server database in %s. wittymediaserver config path shows the folder in use, and -data <dir> chooses another. A service keeps its data in a folder of its own.", data)
	case filesDenied(err):
		return fmt.Sprintf("This account can't change the server's files in %s, and that's what this command depends on. Run it as the account that runs the server, or on Windows in a terminal opened with Run as administrator.", data)
	}
	return fmt.Sprintf("The database in %s can't be used: %v. wittymediaserver doctor says more.", data, err)
}

// filesDenied: the system said no, or the database opened only for reading.
func filesDenied(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return errors.Is(err, os.ErrPermission) || strings.Contains(msg, "readonly") || strings.Contains(msg, "unable to open")
}
