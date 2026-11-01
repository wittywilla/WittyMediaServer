// Command wittymediaserver is the server and its command line. With no arguments it starts
// the server; subcommands are for repairs, support and removing it.
package main

import (
	"fmt"
	"os"

	bdcli "github.com/wittywilla/wittybluray/cli"
	cdcli "github.com/wittywilla/wittycd/cli"
	"github.com/wittywilla/wittydvd/cli"

	"github.com/wittywilla/wittymediaserver/internal/layout"
	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/uninstall"
)

func main() {
	if code, ok := runAsService(os.Args[1:]); ok {
		os.Exit(code)
	}
	os.Exit(run(os.Args[1:]))
}

// serviceStop closes when Windows' service manager asks the server to stop.
var serviceStop = make(chan struct{})

func run(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case modules.SandboxFlag:
			return modules.RunSandboxed(args[1:])
		case "disc":
			return cli.Disc(args[1:])
		case "dvd":
			return cli.DVD(args[1:])
		case "bluray":
			return bdcli.Bluray(args[1:])
		case "cd":
			return cdcli.CD(args[1:])
		case "config":
			return cmdConfig(args[1:])
		case "serve":
			return cmdServe(args[1:])
		case "reset-2fa":
			return cmdReset2FA(args[1:])
		case "reset":
			return cmdReset(args[1:])
		case "logs":
			return cmdLogs(args[1:])
		case "metadata":
			return cmdMetadata(args[1:])
		case "backup":
			return cmdBackup(args[1:])
		case "doctor":
			return cmdDoctor(args[1:])
		case "uninstall", "--uninstall", "-uninstall", "/uninstall":
			return uninstall.RunCLI(args[1:], os.Stdin, os.Stdout)
		case "--take-over-old-copy": // run by the Windows installer
			for _, what := range uninstall.TakeOver(uninstall.Here()) {
				fmt.Println("cleared:", what)
			}
			return 0
		case "version", "--version", "-v":
			fmt.Println(layout.ServerCommand, version)
			return 0
		case "help", "-h", "--help":
			usage()
			return 0
		}
	}
	return cmdServe(args)
}

// startExe is this program's path when it started: after an update has
// renamed it to *.old, the system may report the new name instead.
var startExe, _ = os.Executable()

// version is stamped by the release scripts (-X main.version=...); this is
// the default for source builds.
var version = "0.6.0-alpha"

func usage() {
	fmt.Println(`WittyMediaServer

usage:
  wittymediaserver [serve] [flags]
                             start the server (default); -config <file> uses a wms.json
  wittymediaserver --uninstall [--keep-data | --remove-data] [--remove-recordings] [--yes] [--dry-run]
                             remove the program, its service and shortcuts, asking about your data
  wittymediaserver config path            show the config file and folders in use
  wittymediaserver config init [--portable]
                             write a starter wms.json (next to the program with --portable)
  wittymediaserver reset-2fa [-data <dir>] <name>
                             turn off an account's two-factor sign-in (lost phone and recovery codes)
  wittymediaserver reset [-data <dir>] [-delete] [-yes]
                             back to a new install, the next time the server starts; keeps a copy unless -delete
  wittymediaserver reset -list | -restore <copy> | -cancel
                             see the copies kept, bring one back, or take back a reset that's waiting
  wittymediaserver backup create|list|parts|inspect|restore|undo|cancel [-data <dir>]
                             a ZIP of your data and metadata, and bringing one back. wittymediaserver backup help says more
  wittymediaserver logs zip [-data <dir>] [-o out.zip] [-days N] [--raw]
                             the logs as a ZIP, sanitized for sharing unless --raw; no server needed
  wittymediaserver logs sanitize [-data <dir>] [-o out.txt] <file>
                             a sanitized copy of any text log
  wittymediaserver metadata export|check|apply [-data <dir>] [-library <id>] [--dry-run | --apply]
                             write a text file for each title to fix by hand, say what's wrong with one, and apply them
  wittymediaserver doctor [-data <dir>] [--fix] [--only <id,id or area>] [--safe-only] [--json] [--yes]
                             find and fix problems with metadata, data, modules and the install, even if the
                             server won't start. Without --fix it only looks. wittymediaserver doctor help says more
  wittymediaserver doctor remap --from <old> --to <new> [--fix]
                             a library's drive or folder moved: rewrite the paths
  wittymediaserver disc ls <image>        list files inside an ISO/UDF image
  wittymediaserver dvd info <image>       dump DVD-Video structure (titles, menus, chapters)
  wittymediaserver dvd walk <image> [n…]  run first-play, print the menu the viewer would see, press buttons
  wittymediaserver dvd crawl <image>      press every button of every menu and report dead ends
  wittymediaserver dvd pgc <image> fp|vmgm|vtsm|vts <vts> <pgcn>
                             disassemble one program chain
  wittymediaserver dvd render <image> <out-dir>
  wittymediaserver bluray info <disc>     Blu-ray titles, program, playlists, clips and streams
  wittymediaserver bluray walk <disc> [button…]  simulate inserting a Blu-ray and pressing menu buttons
  wittymediaserver bluray render <disc> <out-dir>  save a Blu-ray menu's pages as PNG files
  wittymediaserver bluray bdj <disc> [seconds [picture.png]]  a Blu-ray's Java titles and what their code needs; with Java, run them
                             write menu overlay/highlight PNGs for the first menu
  wittymediaserver cd info <image>        an audio CD image's tracks, indexes, CD-TEXT and disc ID
  wittymediaserver cd check <image>       read a CD image from end to end and report what is wrong with it
  wittymediaserver version`)
}
