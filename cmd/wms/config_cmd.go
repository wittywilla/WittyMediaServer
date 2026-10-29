package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wittywilla/wittymediaserver/internal/config"
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
