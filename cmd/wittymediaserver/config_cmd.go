// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/reset"
)

// wittymediaserver config path            show the config file and folders in use
// wittymediaserver config init            write a starter wms.json in the user config folder
// wittymediaserver config init --portable write it next to the program (portable mode)
// wittymediaserver config port [<port>]    show the port, or save one in settings.ini
func cmdConfig(args []string) int {
	if len(args) == 0 || args[0] == "path" {
		cfg, err := config.Parse(nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		file := cfg.ConfigFile
		if file == "" {
			file = "(none — using defaults; create one with: wittymediaserver config init)"
		}
		fmt.Printf("config file:   %s\nportable:      %v\ndata:          %s\nmetadata:      %s\ncache:         %s\ntranscode:     %s\nlogs:          %s\nlisten:        %s, %s\nsettings file: %s\n",
			file, cfg.Portable, cfg.DataDir, cfg.ImagesDir, cfg.CacheDir, cfg.TranscodeDir, cfg.LogDir, cfg.Listen, listenFrom(cfg), config.SettingsSaveFile(cfg.DataDir))
		return 0
	}
	if args[0] == "port" {
		return cmdConfigPort(args[1:])
	}
	if args[0] != "init" {
		fmt.Fprintln(os.Stderr, "usage: wittymediaserver config [path | init [--portable] | port [<port>]]")
		return 2
	}
	target := config.UserConfigFile()
	body := config.Example
	if len(args) > 1 && (args[1] == "--portable" || args[1] == "-portable") {
		target = filepath.Join(config.ProgramDir(), config.FileName)
		body = `{
  "portable": true,
  "dataDir": "data"
}
`
	}
	if _, err := os.Stat(target); err == nil {
		fmt.Fprintln(os.Stderr, "already exists:", target)
		return 1
	} else if !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("wrote", target)
	return 0
}

// listenFrom says in words where the address came from.
func listenFrom(c *config.Config) string {
	switch c.ListenFrom {
	case config.FromFlag:
		return "from -listen"
	case config.FromEnv:
		return "from WMS_LISTEN"
	case config.FromConfig:
		return "from " + c.ConfigFile
	case config.FromSettings:
		return "from " + c.SettingsFile
	}
	return "the default"
}

// cmdConfigPort shows the port, or saves one in the settings.ini Settings → Network uses, for the
// next start. Installers carry an earlier install's port over with it:
//
//	wittymediaserver config port [-data <dir>] [<port>]
//	wittymediaserver config port -data <dir> -in-data [-port <port>] [-from-command <an old service's command line>]
//
// -port wins. -from-command takes the port of -listen in the words after it. An empty -port, or
// a command line with no -listen, is nothing to carry over. -in-data saves in the data folder's
// copy, as a service started by install.sh can't change the one beside the program.
func cmdConfigPort(args []string) int {
	var rest []string
	port, carrying, inData := "", false, false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch name := strings.TrimLeft(a, "-"); {
		case name == "port" && i+1 < len(args):
			i++
			carrying = true
			if port == "" {
				port = args[i]
			}
		case name == "in-data":
			inData = true
		case name == "from-command":
			carrying = true
			if port == "" {
				port = listenInCommand(args[i+1:])
			}
			i = len(args)
		case (name == "data" || name == "config") && i+1 < len(args):
			rest = append(rest, a, args[i+1])
			i++
		case !strings.HasPrefix(a, "-") && port == "":
			port = a
		default:
			rest = append(rest, a)
		}
	}
	cfg, err := config.ParseOnly(rest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if port == "" {
		if carrying {
			fmt.Println("no port to carry over")
			return 0
		}
		_, p, _ := net.SplitHostPort(cfg.Listen)
		fmt.Printf("port %s, %s\nSettings → Network saves it in %s\n", p, listenFrom(cfg), config.SettingsSaveFile(cfg.DataDir))
		return 0
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < config.MinPort || n > config.MaxPort {
		fmt.Fprintf(os.Stderr, "%q isn't a port: a port is a number from %d to %d\n", port, config.MinPort, config.MaxPort)
		return 2
	}
	file := config.SettingsSaveFile(cfg.DataDir)
	if inData {
		file = config.DataSettingsFile(cfg.DataDir)
	}
	if err := config.SavePort(file, n); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("port %d saved in %s, for the next start\n", n, file)
	switch cfg.ListenFrom {
	case config.FromFlag, config.FromEnv, config.FromConfig:
		fmt.Printf("but %s, %s, wins over it: remove that to use this port\n", cfg.Listen, listenFrom(cfg))
	}
	return 0
}

// listenInCommand is the port of -listen in a command line, such as the Windows service's before
// 0.7.0: `"...\wittymediaserver.exe" --windows-service serve -data "C:\ProgramData\..." -listen :8080`.
// "" when there's none, or it isn't just a port on every network card.
func listenInCommand(words []string) string {
	for i, w := range words {
		v := ""
		switch {
		case (w == "-listen" || w == "--listen") && i+1 < len(words):
			v = words[i+1]
		case strings.HasPrefix(w, "-listen=") || strings.HasPrefix(w, "--listen="):
			v = w[strings.Index(w, "=")+1:]
		default:
			continue
		}
		if p, ok := strings.CutPrefix(strings.Trim(v, `"`), ":"); ok {
			if _, err := strconv.Atoi(p); err == nil {
				return p
			}
		}
		return ""
	}
	return ""
}

// wittymediaserver reset-2fa [-data <dir>] <name>: the way back for an account whose phone
// and recovery codes are both gone. It needs access to the server's files.
func cmdReset2FA(args []string) int {
	name := ""
	if n := len(args); n > 0 {
		name, args = args[n-1], args[:n-1]
	}
	if name == "" || strings.HasPrefix(name, "-") {
		fmt.Fprintln(os.Stderr, "usage: wittymediaserver reset-2fa [-data <dir>] <name>")
		return 2
	}
	env, err := doctorEnv(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := dataOwnerOK(env.Data); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	d, err := db.OpenExisting(env.Data, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, recoveryOpenError(env.Data, err))
		return 1
	}
	defer d.Close()
	if err := auth.ClearTwoFactorByName(context.Background(), d, name); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	recoveryLog(env.LogDir, "two-factor sign-in was turned off from the command line", name)
	fmt.Printf("Two-factor sign-in is off for %q. They can sign in with their password and set it up again.\n", name)
	return 0
}

// cmdReset asks for a reset from the command line, for someone who can't sign in or
// whose server won't start. It only writes the request: the server, which is the one
// holding these files, carries it out as it next starts.
func cmdReset(args []string) int {
	var del, yes, list, cancel bool
	restore := ""
	var rest []string
	for i := 0; i < len(args); i++ {
		switch strings.TrimLeft(args[i], "-") {
		case "delete":
			del = true
		case "yes":
			yes = true
		case "list":
			list = true
		case "cancel":
			cancel = true
		case "restore":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "usage: wittymediaserver reset -restore <copy>; wittymediaserver reset -list shows them")
				return 2
			}
			i++
			restore = args[i]
		default:
			rest = append(rest, args[i])
		}
	}
	cfg, err := config.Parse(rest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if list {
		for _, c := range reset.Copies(cfg.DataDir) {
			fmt.Printf("%s  %d MB\n", c.Name, c.Bytes>>20)
		}
		return 0
	}
	if cancel {
		found, err := reset.Cancel(cfg.DataDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if found {
			fmt.Println("The reset that was waiting is cancelled. Nothing changed.")
		} else {
			fmt.Println("No reset was waiting.")
		}
		return 0
	}
	var libraries []string
	if d, err := db.Open(cfg.DataDir); err == nil {
		if rows, err := d.Query(`SELECT path FROM library_paths`); err == nil {
			for rows.Next() {
				var p string
				if rows.Scan(&p) == nil {
					libraries = append(libraries, p)
				}
			}
			rows.Close()
		}
		d.Close()
	}
	outside, err := reset.Check(reset.Folders{Data: cfg.DataDir, Others: []string{cfg.CacheDir, cfg.TranscodeDir, cfg.ImagesDir},
		Program: config.ProgramDir(), Config: cfg.ConfigFile}, libraries)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	req := reset.Request{Mode: reset.ModeKeepCopy, Outside: outside, At: time.Now()}
	switch {
	case restore != "":
		req.Mode, req.Copy = reset.ModeRestore, restore
		fmt.Printf("This replaces everything in %s with the copy %s.\nWhat's there now is kept as another copy.\n", cfg.DataDir, restore)
	case del:
		req.Mode = reset.ModeDelete
		fmt.Printf("This DELETES everything in %s for good, including copies from earlier resets:\n", cfg.DataDir)
	default:
		fmt.Printf("This moves everything in %s into a copy inside it, and starts again from a new install:\n", cfg.DataDir)
	}
	if restore == "" {
		fmt.Println("  every account and sign-in, libraries and watch progress, all settings and keys, game saves and")
		fmt.Println("  cores, TV setup and recordings kept in that folder, installed modules, logs and caches.")
		fmt.Println("Your media files, the program and its config file stay.")
	}
	if !yes {
		fmt.Print("Type RESET to go ahead: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(line) != "RESET" {
			fmt.Println("Nothing changed.")
			return 1
		}
	}
	if err := reset.Ask(cfg.DataDir, req); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("It happens the next time the server starts. If it's running, restart it now.")
	fmt.Println("To change your mind before then: wittymediaserver reset -cancel")
	return 0
}
