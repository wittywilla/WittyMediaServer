// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package uninstall

import (
	"html"
	"os"
	"path/filepath"
)

const (
	macLabel  = "com.wittywilla.wittymediaserver"
	macApp    = "WittyMediaServer.app"
	macUninst = "Uninstall WittyMediaServer.app"
)

// planHomeMac is what one person's account holds on a Mac.
func planHomeMac(e *Env, pl *Plan, h string) {
	other := h != e.Home
	agent := filepath.Join(h, "Library", "LaunchAgents", macLabel+".tray.plist")
	if exists(agent) {
		pl.add(Item{Kind: "start at sign-in", Name: agent, Group: GroupProgram, Admin: other, step: stepStop,
			remove: func() error {
				_, _ = e.Sys.Run("launchctl", "unload", agent)
				return os.Remove(agent)
			}})
	}
	if other && !staysInHome(h, filepath.Join(h, "Library", "Application Support", "WittyMediaServer")) {
		pl.keep(h, "something in it leads out of it, so nothing was removed from it as an administrator")
		return
	}
	support := filepath.Join(h, "Library", "Application Support", "WittyMediaServer")
	planConfigFile(e, pl, filepath.Join(support, "wms.json"), other)
	planDataFolder(e, pl, support, true, other)
	for _, rel := range []string{
		"Library/Caches/" + macLabel,
		"Library/Preferences/" + macLabel + ".plist",
		"Library/Saved Application State/" + macLabel + ".savedState",
		"Library/Logs/WittyMediaServer",
		"Library/HTTPStorages/" + macLabel,
		"Library/WebKit/" + macLabel,
	} {
		if p := filepath.Join(h, filepath.FromSlash(rel)); exists(p) && (!other || staysInHome(h, p)) {
			pl.add(fileItem("app cache or preference", p, GroupData, other))
		}
	}
}

func planMac(e *Env, pl *Plan) {
	daemon := e.p("/Library/LaunchDaemons/" + macLabel + ".plist")
	if exists(daemon) {
		if b, err := os.ReadFile(daemon); err == nil {
			if d := launchdDataDir(string(b)); d != "" {
				planDataFolder(e, pl, e.Prefix+d, false, true)
			}
		}
		pl.add(Item{Kind: "service", Name: daemon, Group: GroupProgram, Admin: true, step: stepStop,
			remove: func() error {
				_, _ = e.Sys.Run("launchctl", "bootout", "system", daemon)
				return os.Remove(daemon)
			}})
	}
	for _, name := range []string{macApp, macUninst} {
		if p := e.p("/Applications/" + name); exists(p) {
			if name == macApp {
				pl.add(Item{Kind: "running program", Name: p, Group: GroupProgram, step: stepStop,
					remove: func() error { e.Sys.StopProgramsIn(p); return nil }})
			}
			pl.add(fileItem("app", p, GroupProgram, true))
		}
	}
	if p := e.p("/usr/local/bin/" + unixName); exists(p) {
		pl.add(fileItem("command", p, GroupProgram, true))
	}
	planProgramFolder(e, pl, e.p("/opt/"+unixName), true) // install.sh, up to 0.5.2-alpha

	if out, err := e.Sys.Run("pkgutil", "--pkgs="+macLabel+".*"); err == nil && out != "" {
		pl.add(Item{Kind: "installer record", Name: macLabel, Group: GroupProgram, Admin: true, step: stepFiles,
			remove: func() error {
				var last error
				for _, pkg := range splitLines(out) {
					if _, err := e.Sys.Run("pkgutil", "--forget", pkg); err != nil {
						last = err
					}
				}
				return last
			}})
	}

	if d := e.Getenv("WMS_DATA_DIR"); d != "" {
		planDataFolder(e, pl, d, false, false)
	}
	planDataFolder(e, pl, e.p("/Library/Application Support/WittyMediaServer"), true, true)
	planDataFolder(e, pl, e.p("/var/lib/"+unixName), true, true)
	planConfigFile(e, pl, e.p("/etc/"+unixName+"/wms.json"), true)
	for _, h := range homes(e, e.p("/Users")) {
		if filepath.Base(h) != "Shared" {
			planHomeMac(e, pl, h)
		}
	}
}

// launchdDataDir reads WMS_DATA_DIR out of a launchd property list. The installer writes it as
// XML text (packaging/macos/scripts/service/postinstall), so "&" there is "&amp;".
func launchdDataDir(plist string) string {
	const key = "<key>WMS_DATA_DIR</key>"
	i := indexOf(plist, key)
	if i < 0 {
		return ""
	}
	rest := plist[i+len(key):]
	start := indexOf(rest, "<string>")
	end := indexOf(rest, "</string>")
	if start < 0 || end < start {
		return ""
	}
	return html.UnescapeString(rest[start+len("<string>") : end])
}
