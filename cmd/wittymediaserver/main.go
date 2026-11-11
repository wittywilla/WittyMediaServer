// SPDX-License-Identifier: AGPL-3.0-or-later
//
// WittyMediaServer - A self-hosted media server that plays DVDs and Blu-rays with their real menus, in any browser.
// Copyright (C) 2026 WittyWilla and contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

// Command wittymediaserver is the server and its command line. With no arguments it starts
// the server; subcommands are for repairs, support and removing it.
package main

import (
	"fmt"
	"os"
	"strings"

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

// asService: Windows' service manager started this process.
var asService bool

func run(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case modules.SandboxFlag:
			return modules.RunSandboxed(args[1:])
		case "disc", "dvd", "bluray", "cd":
			fmt.Fprintf(os.Stderr, "wittymediaserver %s has gone. Looking inside a disc is now a developer's tool in each disc module's project, wittydvd, wittybluray or wittycd, which playing discs doesn't need. The DVD, Blu-ray and Music guides say how to get it.\n", args[0])
			return 2
		case "config":
			return cmdConfig(args[1:])
		case "serve":
			return cmdServe(args[1:])
		case "reset-2fa":
			return cmdReset2FA(args[1:])
		case "reset-password":
			return cmdResetPassword(args[1:])
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
		case "hardware":
			return cmdHardware(args[1:])
		case "health":
			return cmdHealth(args[1:])
		case "uninstall", "--uninstall", "-uninstall", "/uninstall":
			return uninstall.RunCLI(args[1:], os.Stdin, os.Stdout)
		case "--secure-service-data": // run by the Windows installer, as an administrator
			if len(args) != 2 {
				fmt.Fprintln(os.Stderr, "usage: wittymediaserver --secure-service-data <folder>")
				return 2
			}
			if err := uninstall.SecureServiceData(args[1]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			return 0
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
		// A misspelt command would otherwise start a second server on the same data,
		// as docker exec wittymediaserver wittymediaserver <typo> did.
		if !strings.HasPrefix(args[0], "-") {
			fmt.Fprintf(os.Stderr, "wittymediaserver: %q isn't a command. wittymediaserver help lists them.\n", args[0])
			return 2
		}
	}
	return cmdServe(args)
}

// startExe is this program's path when it started: after an update has
// renamed it to *.old, the system may report the new name instead.
var startExe, _ = os.Executable()

// version is stamped by the release scripts (-X main.version=...); this is
// the default for source builds.
var version = "0.8.0-alpha"

func usage() {
	fmt.Println(`WittyMediaServer

usage:
  wittymediaserver [serve] [flags]
                             start the server (default); -config <file> uses a wms.json
  wittymediaserver --uninstall [--keep-data | --remove-data] [--remove-recordings] [--remove-backups] [--yes] [--dry-run]
                             remove the program, its service and shortcuts, asking about your data
  wittymediaserver config path            show the config file and folders in use
  wittymediaserver config init [--portable]
                             write a starter wms.json (next to the program with --portable)
  wittymediaserver config port [<port>]
                             show the port and where it comes from, or save one in settings.ini
  wittymediaserver reset-password [-data <dir>] [--generate] <name>
                             a new password for an account that can't sign in; --list shows the usernames
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
  wittymediaserver metadata export|check|apply|undo [-data <dir>] [-library <id>] [--dry-run | --apply]
                             write a text file for each title to fix by hand, say what's wrong with one, apply them, and take one apply back
  wittymediaserver doctor [-data <dir>] [--fix] [--only <id,id or area>] [--safe-only] [--json] [--yes]
                             find and fix problems with metadata, data, modules and the install, even if the
                             server won't start. Without --fix it only looks. wittymediaserver doctor help says more
  wittymediaserver doctor remap --from <old> --to <new> [--fix]
                             a library's drive or folder moved: rewrite the paths
  wittymediaserver hardware [--json]
                             which graphics devices the server can use and which hardware encoders work,
                             and what to change when one can't be used (docker exec wittymediaserver ...)
  wittymediaserver health
                             exits 0 if the server on this machine answers: the container's health check
  wittymediaserver version

Tools for looking inside discs are in each disc module's project, not in its download: wittydvd, wittybluray and wittycd.`)
}
