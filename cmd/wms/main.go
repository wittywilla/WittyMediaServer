// Command wms is the WittyMediaServer binary. With no arguments it starts the
// server; subcommands expose disc tooling used during development and support.
package main

import (
	"fmt"
	"os"

	"github.com/wittywilla/wittymediaserver/internal/modules"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case modules.SandboxFlag:
			os.Exit(modules.RunSandboxed(os.Args[2:]))
		case "disc", "dvd", "bluray", "cd":
			fmt.Fprintf(os.Stderr, "wms %s is now a tool of its own, which comes with its module: wittydvd, wittybluray or wittycd\n", os.Args[1])
			os.Exit(2)
		case "config":
			os.Exit(cmdConfig(os.Args[2:]))
		case "serve":
			os.Exit(cmdServe(os.Args[2:]))
		case "reset-2fa":
			os.Exit(cmdReset2FA(os.Args[2:]))
		case "reset":
			os.Exit(cmdReset(os.Args[2:]))
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
  wms version

Tools for looking inside discs now come with their modules: wittydvd, wittybluray and wittycd.`)
}
