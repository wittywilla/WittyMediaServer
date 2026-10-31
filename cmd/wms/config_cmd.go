package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/reset"
)

// wms config path            show the config file and folders in use
// wms config init            write a starter wms.json in the user config folder
// wms config init --portable write it next to the program (portable mode)
func cmdConfig(args []string) int {
	if len(args) == 0 || args[0] == "path" {
		cfg, err := config.Parse(nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		file := cfg.ConfigFile
		if file == "" {
			file = "(none — using defaults; create one with: wms config init)"
		}
		fmt.Printf("config file:   %s\nportable:      %v\ndata:          %s\nmetadata:      %s\ncache:         %s\ntranscode:     %s\nlogs:          %s\nlisten:        %s\n",
			file, cfg.Portable, cfg.DataDir, cfg.ImagesDir, cfg.CacheDir, cfg.TranscodeDir, cfg.LogDir, cfg.Listen)
		return 0
	}
	if args[0] != "init" {
		fmt.Fprintln(os.Stderr, "usage: wms config [path | init [--portable]]")
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

// wms reset-2fa [-data <dir>] <name>: the way back for an account whose phone
// and recovery codes are both gone. It needs access to the server's files.
func cmdReset2FA(args []string) int {
	name := ""
	if n := len(args); n > 0 {
		name, args = args[n-1], args[:n-1]
	}
	if name == "" || strings.HasPrefix(name, "-") {
		fmt.Fprintln(os.Stderr, "usage: wms reset-2fa [-data <dir>] <name>")
		return 2
	}
	cfg, err := config.Parse(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	d, err := db.Open(cfg.DataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer d.Close()
	if err := auth.ClearTwoFactorByName(context.Background(), d, name); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("Two-factor sign-in is off for %s. They can sign in with their password and set it up again.\n", name)
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
				fmt.Fprintln(os.Stderr, "usage: wms reset -restore <copy>; wms reset -list shows them")
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
	fmt.Println("To change your mind before then: wms reset -cancel")
	return 0
}
