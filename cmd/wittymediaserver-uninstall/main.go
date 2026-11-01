// Command wittymediaserver-uninstall is the uninstaller with windows and buttons: Uninstall
// WittyMediaServer.exe on Windows, Uninstall WittyMediaServer.app on a Mac. The work is in
// internal/uninstall, the same as `wittymediaserver --uninstall`.
//
//	Uninstall WittyMediaServer.exe                 asks
//	Uninstall WittyMediaServer.exe /S              silently, keeps your data
//	Uninstall WittyMediaServer.exe /S /RemoveData  silently, a full clean-up (recordings and backups stay)
//	... /RemoveRecordings /RemoveBackups           those too
//
// It asks first, running unprivileged from wherever it is. The work is then done by a worker,
// which may need raising to an administrator, and on Windows runs from a copy in the temp
// folder, since a running program can't be deleted there.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/uninstall"
)

const title = "Uninstall WittyMediaServer"

func main() {
	a, err := uninstall.ParseArgs(os.Args[1:])
	if err != nil {
		if !a.Yes {
			show(title, err.Error())
		}
		os.Exit(2)
	}
	e := uninstall.Here()
	if a.ProgramFolder != "" {
		e.SelfRoot = a.ProgramFolder
	}
	if a.DryRun {
		os.Exit(dryRun(a))
	}
	if a.Worker {
		os.Exit(work(e, a))
	}
	os.Exit(ask(e, a))
}

// dryRun says what would go and changes nothing, as the command line's --dry-run does.
func dryRun(a uninstall.Args) int {
	var out strings.Builder
	code := uninstall.RunCLI(os.Args[1:], strings.NewReader(""), &out)
	if a.Yes || a.JSON {
		fmt.Print(out.String())
	} else {
		show(title, shorten(out.String(), 30))
	}
	return code
}

// ask is the part a person sees first.
func ask(e *uninstall.Env, a uninstall.Args) int {
	pl := uninstall.MakePlan(e)
	if len(pl.Items) == 0 {
		if !a.Yes {
			show(title, "Nothing of WittyMediaServer was found on this computer.")
		}
		return 0
	}
	if !a.Yes {
		if !confirm(title, "Remove WittyMediaServer from this computer?\n\nThis removes the program, its background service, shortcuts and anything that starts it.\n\n"+
			"Your movies, shows, music, photos, books and games are never touched.") {
			return 1
		}
		if !a.KeepData && !a.RemoveData && pl.Has(uninstall.GroupData) {
			a.RemoveData = confirm(title, "Also delete your library, accounts and settings?\n\n"+
				"This is your watch progress, artwork, logs, caches, game saves and downloaded modules ("+uninstall.Size(pl.Bytes(uninstall.GroupData))+").\n\n"+
				"Choose Yes for a full clean-up. Choose No to keep them, so they're still there if you install it again.")
		}
		if !a.RemoveRecordings && pl.Has(uninstall.GroupRecordings) {
			a.RemoveRecordings = confirm(title, "Also delete your recorded TV ("+uninstall.Size(pl.Bytes(uninstall.GroupRecordings))+")?\n\nChoose No to keep the recordings where they are.")
		}
		if !a.RemoveBackups && pl.Has(uninstall.GroupBackups) {
			a.RemoveBackups = confirm(title, "Also delete your backups ("+uninstall.Size(pl.Bytes(uninstall.GroupBackups))+")?\n\nChoose No to keep them, so you can restore from them after installing it again.")
		}
	}
	a.ProgramFolder = e.SelfRoot
	if a.Report == "" {
		a.Report = uninstall.ReportPath()
	}
	if err := startWork(e, a, pl.NeedsAdmin(a.Options) && !e.IsAdmin); err != nil {
		if !a.Yes {
			show(title, fmt.Sprintf("WittyMediaServer wasn't removed.\n\n%s", err))
		}
		return 1
	}
	return 0
}

// work removes everything, and says what happened.
func work(e *uninstall.Env, a uninstall.Args) int {
	res := uninstall.Execute(uninstall.MakePlan(e), a.Options)
	uninstall.WriteReport(a.Report, res)
	uninstall.RemoveSelfLater()
	if a.Yes && !a.AskedByPerson() {
		if res.OK() {
			return 0
		}
		return 1
	}
	text := "WittyMediaServer was removed."
	if !res.OK() {
		text = "Most of WittyMediaServer was removed, but some of it couldn't be."
	}
	text += "\n\n" + shorten(res.Report(), 30)
	if a.Report != "" {
		text += "\n\nThe full list is in " + filepath.ToSlash(a.Report)
	}
	show(title, text)
	if res.OK() {
		return 0
	}
	return 1
}

// shorten keeps a dialog on the screen: the full list is in the report.
func shorten(s string, lines int) string {
	n := 0
	for i, c := range s {
		if c == '\n' {
			n++
			if n == lines {
				return s[:i] + "\n..."
			}
		}
	}
	return s
}
