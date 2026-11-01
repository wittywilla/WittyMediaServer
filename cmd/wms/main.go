// Command wms is the WittyMediaServer binary. With no arguments it starts the
// server; subcommands expose disc tooling used during development and support.
package main

import (
	"fmt"
	"os"

	bdcli "github.com/wittywilla/wittybluray/cli"
	cdcli "github.com/wittywilla/wittycd/cli"
	"github.com/wittywilla/wittydvd/cli"

	"github.com/wittywilla/wittymediaserver/internal/modules"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case modules.SandboxFlag:
			os.Exit(modules.RunSandboxed(os.Args[2:]))
		case "disc":
			os.Exit(cli.Disc(os.Args[2:]))
		case "dvd":
			os.Exit(cli.DVD(os.Args[2:]))
		case "bluray":
			os.Exit(bdcli.Bluray(os.Args[2:]))
		case "cd":
			os.Exit(cdcli.CD(os.Args[2:]))
		case "config":
			os.Exit(cmdConfig(os.Args[2:]))
		case "serve":
			os.Exit(cmdServe(os.Args[2:]))
		case "reset-2fa":
			os.Exit(cmdReset2FA(os.Args[2:]))
		case "reset":
			os.Exit(cmdReset(os.Args[2:]))
		case "logs":
			os.Exit(cmdLogs(os.Args[2:]))
		case "metadata":
			os.Exit(cmdMetadata(os.Args[2:]))
		case "backup":
			os.Exit(cmdBackup(os.Args[2:]))
		case "doctor":
			os.Exit(cmdDoctor(os.Args[2:]))
		case "version", "--version", "-v":
			fmt.Println("wms", version)
			return
		case "help", "-h", "--help":
			usage()
			return
		}
	}
	os.Exit(cmdServe(os.Args[1:]))
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
  wms [serve] [flags]        start the server (default); -config <file> uses a wms.json
  wms config path            show the config file and folders in use
  wms config init [--portable]
                             write a starter wms.json (next to the program with --portable)
  wms reset-2fa [-data <dir>] <name>
                             turn off an account's two-factor sign-in (lost phone and recovery codes)
  wms reset [-data <dir>] [-delete] [-yes]
                             back to a new install, the next time the server starts; keeps a copy unless -delete
  wms reset -list | -restore <copy> | -cancel
                             see the copies kept, bring one back, or take back a reset that's waiting
  wms backup create|list|parts|inspect|restore|undo|cancel [-data <dir>]
                             a ZIP of your data and metadata, and bringing one back. wms backup help says more
  wms logs zip [-data <dir>] [-o out.zip] [-days N] [--raw]
                             the logs as a ZIP, sanitized for sharing unless --raw; no server needed
  wms logs sanitize [-data <dir>] [-o out.txt] <file>
                             a sanitized copy of any text log
  wms metadata export|check|apply [-data <dir>] [-library <id>] [--dry-run | --apply]
                             write a text file for each title to fix by hand, say what's wrong with one, and apply them
  wms doctor [-data <dir>] [--fix] [--only <id,id or area>] [--safe-only] [--json] [--yes]
                             find and fix problems with metadata, data, modules and the install, even if the
                             server won't start. Without --fix it only looks. wms doctor help says more
  wms doctor remap --from <old> --to <new> [--fix]
                             a library's drive or folder moved: rewrite the paths
  wms disc ls <image>        list files inside an ISO/UDF image
  wms dvd info <image>       dump DVD-Video structure (titles, menus, chapters)
  wms dvd walk <image> [n…]  run first-play, print the menu the viewer would see, press buttons
  wms dvd crawl <image>      press every button of every menu and report dead ends
  wms dvd pgc <image> fp|vmgm|vtsm|vts <vts> <pgcn>
                             disassemble one program chain
  wms dvd render <image> <out-dir>
  wms bluray info <disc>     Blu-ray titles, program, playlists, clips and streams
  wms bluray walk <disc> [button…]  simulate inserting a Blu-ray and pressing menu buttons
  wms bluray render <disc> <out-dir>  save a Blu-ray menu's pages as PNG files
  wms bluray bdj <disc> [seconds [picture.png]]  a Blu-ray's Java titles and what their code needs; with Java, run them
                             write menu overlay/highlight PNGs for the first menu
  wms cd info <image>        an audio CD image's tracks, indexes, CD-TEXT and disc ID
  wms cd check <image>       read a CD image from end to end and report what is wrong with it
  wms version`)
}
