package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
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
