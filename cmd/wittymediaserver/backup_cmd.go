// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/backup"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/server"
)

// wittymediaserver backup create|list|inspect|restore|undo|cancel: everything Settings, Backup does, with
// no server needed. It works on whichever data folder -data or the config file names.

const backupUsage = `usage:
  wittymediaserver backup create [-data <dir>] [-o out.zip] [--parts a,b,c] [--password-file f | --password-stdin | --ask-password]
                    a ZIP of this server's data and metadata; with no password, secrets are left out
  wittymediaserver backup parts  what can be included, and how large each part is
  wittymediaserver backup list [-data <dir>]
  wittymediaserver backup inspect <file> [-data <dir>] [--password-file f | --password-stdin | --ask-password]
                    check a backup from end to end, and say what's in it. Nothing changes
  wittymediaserver backup restore <file> [-data <dir>] [--parts a,b,c] [--yes] [--password-file f | --password-stdin | --ask-password]
                    bring a backup back. With a server running it's asked for, and happens as that server restarts
  wittymediaserver backup undo [<safety copy>] [-data <dir>]
                    put back what the last restore replaced
  wittymediaserver backup cancel [-data <dir>]
                    take back a restore waiting for the next start
A password is never taken on the command line, where other programs could see it. WMS_BACKUP_PASSWORD also works.`

type backupArgs struct {
	output, parts, passFile string
	passStdin, passAsk, yes bool
	positional              []string
	rest                    []string
}

func parseBackupArgs(args []string) (backupArgs, error) {
	var a backupArgs
	need := func(i int) (string, error) {
		if i+1 >= len(args) {
			return "", fmt.Errorf("%s needs a value", args[i])
		}
		return args[i+1], nil
	}
	for i := 0; i < len(args); i++ {
		var err error
		switch strings.TrimLeft(args[i], "-") {
		case "o":
			a.output, err = need(i)
			i++
		case "parts":
			a.parts, err = need(i)
			i++
		case "password-file":
			a.passFile, err = need(i)
			i++
		case "password-stdin":
			a.passStdin = true
		case "ask-password":
			a.passAsk = true
		case "yes":
			a.yes = true
		case "password":
			return a, errors.New("a password can't be given on the command line, where other programs could see it. Use --ask-password, --password-file, --password-stdin or WMS_BACKUP_PASSWORD")
		case "data", "config", "listen", "log", "log-dir":
			a.rest = append(a.rest, args[i])
			if i+1 < len(args) {
				i++
				a.rest = append(a.rest, args[i])
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				return a, fmt.Errorf("unknown option %s", args[i])
			}
			a.positional = append(a.positional, args[i])
		}
		if err != nil {
			return a, err
		}
	}
	return a, nil
}

func (a backupArgs) password(prompt string) (string, error) {
	switch {
	case a.passFile != "":
		b, err := os.ReadFile(a.passFile)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	case a.passStdin:
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	case a.passAsk:
		fmt.Fprint(os.Stderr, prompt)
		b, err := readHidden()
		fmt.Fprintln(os.Stderr)
		return b, err
	}
	return os.Getenv("WMS_BACKUP_PASSWORD"), nil
}

func backupFolders(rest []string) (backup.Folders, *config.Config, error) {
	cfg, err := config.Parse(rest)
	if err != nil {
		return backup.Folders{}, nil, err
	}
	return backup.Folders{Data: cfg.DataDir, Images: cfg.ImagesDir}, cfg, nil
}

func splitParts(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sizeWords(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}

// progressLine keeps one line up to date, on a terminal, and says little otherwise.
func progressLine() func(backup.Step) {
	st, err := os.Stderr.Stat()
	tty := err == nil && st.Mode()&os.ModeCharDevice != 0
	phase := ""
	var shown time.Time
	return func(s backup.Step) {
		if !tty {
			if s.Phase != phase {
				phase = s.Phase
				fmt.Fprintln(os.Stderr, s.Phase)
			}
			return
		}
		if time.Since(shown) < 200*time.Millisecond && s.Phase == phase {
			return
		}
		shown, phase = time.Now(), s.Phase
		if s.Total > 0 {
			fmt.Fprintf(os.Stderr, "\r\x1b[K%s: %s of about %s", s.Phase, sizeWords(s.Done), sizeWords(s.Total))
		} else {
			fmt.Fprintf(os.Stderr, "\r\x1b[K%s", s.Phase)
		}
	}
}

func cmdBackup(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Println(backupUsage)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	sub := args[0]
	a, err := parseBackupArgs(args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	f, _, err := backupFolders(a.rest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	switch sub {
	case "create":
		return backupCreate(ctx, f, a)
	case "parts":
		for _, p := range backup.Estimate(ctx, f, nil) {
			on := " "
			if p.Default {
				on = "*"
			}
			fmt.Printf("%s %-14s %10s  %s\n", on, p.ID, sizeWords(p.Bytes), p.Label)
		}
		fmt.Println("* included unless you choose with --parts")
		return 0
	case "list":
		list := backup.List(f)
		if len(list) == 0 {
			fmt.Println("No backups in", f.Dir())
			return 0
		}
		for _, b := range list {
			how := "secrets left out"
			if b.Protected {
				how = "password protected"
			}
			fmt.Printf("%s  %10s  %s  %s\n", b.Name, sizeWords(b.Size), b.Created.Local().Format("2006-01-02 15:04"), how)
		}
		return 0
	case "inspect", "restore":
		if len(a.positional) != 1 {
			fmt.Fprintln(os.Stderr, backupUsage)
			return 2
		}
		return backupInspectRestore(ctx, f, a, sub == "restore")
	case "undo":
		return backupUndo(f, a)
	case "cancel":
		found, err := backup.Cancel(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if found {
			fmt.Println("The restore that was waiting is cancelled. Nothing changed.")
		} else {
			fmt.Println("No restore was waiting.")
		}
		return 0
	}
	fmt.Fprintln(os.Stderr, backupUsage)
	return 2
}

func backupCreate(ctx context.Context, f backup.Folders, a backupArgs) int {
	parts := splitParts(a.parts)
	if parts == nil {
		parts = backup.DefaultParts()
	}
	password, err := a.password("Password for this backup: ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if a.passAsk {
		again, err := a.password("And again: ")
		if err != nil || again != password {
			fmt.Fprintln(os.Stderr, "The two passwords aren't the same.")
			return 2
		}
	}
	out, man, err := backup.Create(ctx, backup.Options{Folders: f, Parts: parts, Password: password, SecretSettings: server.BackupSecretSettings(), ServerVersion: version,
		Output: a.output, Progress: progressLine(), Problem: func(file, why string) { fmt.Fprintf(os.Stderr, "\nLeft out %s: %s\n", file, why) }})
	fmt.Fprintln(os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	st, _ := os.Stat(out)
	fmt.Printf("Backed up %d files to %s", len(man.Files), out)
	if st != nil {
		fmt.Printf(" (%s)", sizeWords(st.Size()))
	}
	fmt.Println()
	if man.Protected {
		fmt.Println("It's protected with your password. If you lose the password, nobody can open it.")
	} else {
		fmt.Println("It has no password, so API keys, two-factor secrets and share links were left out. Passwords are in it as hashes: keep it private.")
	}
	return 0
}

func describeInspection(in *backup.Inspection) {
	fmt.Printf("Backup from %s, made by version %s", in.Created.Local().Format("2006-01-02 15:04"), in.ServerVersion)
	if in.Protected {
		fmt.Print(", password protected")
	}
	fmt.Println(":")
	for _, p := range in.Parts {
		fmt.Printf("  %-32s", p.Label)
		if p.Files > 0 {
			fmt.Printf(" %d files, %s", p.Files, sizeWords(p.Bytes))
		}
		if p.Rows > 0 {
			fmt.Printf(" %d rows", p.Rows)
		}
		fmt.Println()
	}
	if len(in.Accounts) > 0 {
		fmt.Println("Accounts afterwards:")
		for _, u := range in.Accounts {
			if u.Admin {
				fmt.Printf("  %s (administrator)\n", u.Name)
			} else {
				fmt.Printf("  %s\n", u.Name)
			}
		}
	}
	if len(in.Libraries) > 0 {
		fmt.Println("Library folders it points at:")
		for _, l := range in.Libraries {
			fmt.Printf("  %s: %s\n", l.Name, strings.Join(l.Folders, ", "))
		}
	}
	for _, label := range []struct {
		title string
		list  []string
	}{{"Web addresses it adds:", in.Addresses}, {"Left out of what would be restored:", in.Dropped}, {"Metadata files changed by hand:", in.HandEdited}, {"Problems in those files:", in.Problems}, {"Good to know:", in.Notes}} {
		if len(label.list) > 0 {
			fmt.Println(label.title)
			for _, s := range label.list {
				fmt.Println("  " + s)
			}
		}
	}
}

func backupInspectRestore(ctx context.Context, f backup.Folders, a backupArgs, restore bool) int {
	path := a.positional[0]
	password, err := a.password("Password of this backup: ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	progress := progressLine()
	in, err := backup.Inspect(ctx, path, password, f, progress)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer in.Close()
	fmt.Println("Every file checked out.")
	describeInspection(in)
	if !restore {
		return 0
	}
	parts := splitParts(a.parts)
	if parts == nil {
		for _, p := range in.Parts {
			parts = append(parts, p.ID)
		}
	}
	if len(parts) == 0 {
		fmt.Fprintln(os.Stderr, "This backup holds nothing to restore.")
		return 1
	}
	fmt.Printf("\nThis replaces %s in %s.\nWhat's there now is moved to a safety copy in its backups folder first.\n", strings.Join(parts, ", "), f.Data)
	if !a.yes {
		fmt.Print("Type RESTORE to go ahead: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(line) != "RESTORE" {
			fmt.Println("Nothing changed.")
			return 1
		}
	}
	if _, err := in.Stage(ctx, f, backup.StageOptions{Parts: parts, Progress: progress}); err != nil {
		fmt.Fprintln(os.Stderr, "\n"+err.Error())
		return 1
	}
	fmt.Fprintln(os.Stderr)
	if pid, running := logging.ServerRunning(filepath.Join(f.Data, "crash")); running {
		fmt.Printf("A server is running on this folder (process %d). Restart it, and the restore happens as it starts.\n", pid)
		fmt.Println("To change your mind before then: wittymediaserver backup cancel")
		return 0
	}
	return finishApply(f)
}

func finishApply(f backup.Folders) int {
	r := backup.Apply(f)
	if r == nil {
		fmt.Fprintln(os.Stderr, "Nothing was waiting.")
		return 1
	}
	if !r.OK {
		fmt.Fprintln(os.Stderr, r.Error)
		return 1
	}
	if r.Mode == "undo" {
		fmt.Println("Done: what the restore replaced is back.")
	} else {
		fmt.Println("Restored. Everyone signs in again with the name and password in the backup.")
	}
	if r.Safety != "" {
		fmt.Printf("What was there is kept in %s. To go back: wittymediaserver backup undo\n", filepath.Join(f.Dir(), r.Safety))
	}
	for _, n := range r.Notes {
		fmt.Println(n)
	}
	return 0
}

func backupUndo(f backup.Folders, a backupArgs) int {
	copies := backup.SafetyCopies(f)
	name := ""
	if len(a.positional) > 0 {
		name = a.positional[0]
	} else if len(copies) > 0 {
		name = copies[0].Name
	}
	if name == "" {
		fmt.Println("There's no safety copy to go back to.")
		return 1
	}
	if err := backup.AskUndo(f, name); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if pid, running := logging.ServerRunning(filepath.Join(f.Data, "crash")); running {
		fmt.Printf("A server is running on this folder (process %d). Restart it, and this happens as it starts.\n", pid)
		return 0
	}
	return finishApply(f)
}
