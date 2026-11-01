package uninstall

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/layout"
)

const (
	winName = "WittyMediaServer"
	// UninstallKey is the entry in Installed apps. Up to 0.5.2-alpha the setup program wrote
	// it in HKCU; the MSI writes it in HKCU or HKLM, with ProductCode in it.
	UninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\WittyMediaServer`
	runKey       = `Software\Microsoft\Windows\CurrentVersion\Run`
	settingsKey  = `Software\WittyWilla\WittyMediaServer`
	makerKey     = `Software\WittyWilla`
)

// programEntries is what a program folder holds, and what goes from it. A data folder or
// wms.json someone keeps there is the data question's, and anything else isn't ours.
func programEntry(name string) bool {
	switch strings.ToLower(name) {
	case "bin", "modules", "docs", "share", ".update", "package-info.json", "wms.exe", "wms", "install.sh", "readme.txt",
		"wittymediaserver.exe", "wittymediaserver", "uninstall wittymediaserver.exe":
		return true
	}
	if strings.HasSuffix(strings.ToLower(name), ".old") {
		return true
	}
	for _, o := range layout.Obsolete() {
		if strings.EqualFold(o, name) {
			return true
		}
	}
	return false
}

// looksLikeProgramFolder: never a folder we can't vouch for, even with the right name.
func looksLikeProgramFolder(dir string) bool {
	return exists(filepath.Join(dir, layout.Marker)) || exists(filepath.Join(dir, "wms.exe")) || exists(filepath.Join(dir, "wms"))
}

// planProgramFolder adds what's ours in a folder holding the program. A portable copy keeps
// its data here too.
func planProgramFolder(e *Env, pl *Plan, dir string, admin bool) {
	if !isDir(dir) || !looksLikeProgramFolder(dir) || !pl.first(dir) {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, ent := range entries {
		name, p := ent.Name(), filepath.Join(dir, ent.Name())
		switch {
		case strings.EqualFold(name, "data"):
			planDataFolder(e, pl, p, true, admin)
		case strings.EqualFold(name, "wms.json"):
			planConfigFile(e, pl, p, admin)
		case programEntry(name):
			pl.add(fileItem("program file", p, GroupProgram, admin))
		default:
			pl.keep(p, "it wasn't made by WittyMediaServer")
		}
	}
	pl.add(emptyFolderItem(dir, GroupProgram, admin))
}

func planWindows(e *Env, pl *Plan) {
	sys32 := filepath.Join(e.Getenv("SystemRoot"), "System32")
	tool := func(n string) string { return filepath.Join(sys32, n) }
	programFiles := filepath.Join(e.Getenv("ProgramFiles"), winName)
	programData := filepath.Join(e.Getenv("ProgramData"), winName)
	localApp, roaming := e.Getenv("LOCALAPPDATA"), e.Getenv("APPDATA")
	perUser := filepath.Join(localApp, "Programs", winName)

	// What runs. A service that's registered is stopped first, whoever installed it.
	if out, err := e.Sys.Run(tool("sc.exe"), "qc", winName); err == nil {
		if d := dataDirFromCommand(out); d != "" {
			planDataFolder(e, pl, d, false, true)
		}
		pl.add(Item{Kind: "service", Name: winName, Group: GroupProgram, Admin: true, step: stepStop,
			remove: func() error {
				_, _ = e.Sys.Run(tool("sc.exe"), "stop", winName)
				_, err := e.Sys.Run(tool("sc.exe"), "delete", winName)
				return err
			}})
	}
	// The boot task install.ps1 made, up to 0.5.2-alpha.
	if out, err := e.Sys.Run(tool("schtasks.exe"), "/Query", "/TN", winName, "/XML"); err == nil && strings.Contains(out, winName) {
		if d := dataDirFromCommand(strings.ReplaceAll(out, "&quot;", `"`)); d != "" {
			planDataFolder(e, pl, d, false, true)
		}
		pl.add(Item{Kind: "scheduled task", Name: winName, Group: GroupProgram, Admin: true, step: stepStop,
			remove: func() error {
				_, _ = e.Sys.Run(tool("schtasks.exe"), "/End", "/TN", winName)
				_, err := e.Sys.Run(tool("schtasks.exe"), "/Delete", "/TN", winName, "/F")
				return err
			}})
	}
	for _, dir := range []string{programFiles, perUser, e.SelfRoot} {
		if dir != "" && isDir(dir) && looksLikeProgramFolder(dir) {
			pl.add(Item{Kind: "running program", Name: dir, Group: GroupProgram, step: stepStop,
				remove: func() error { e.Sys.StopProgramsIn(dir); return nil }})
		}
	}

	// The installer takes out what it put in, first: its database has to agree.
	for _, hive := range []string{"HKLM", "HKCU"} {
		if code, ok := e.Sys.RegValue(hive, UninstallKey, "ProductCode"); ok && strings.HasPrefix(code, "{") {
			pl.add(Item{Kind: "Windows Installer product", Name: code, Group: GroupProgram, Admin: hive == "HKLM", step: stepInstaller,
				remove: func() error {
					_, err := e.Sys.Run(tool("msiexec.exe"), "/x", code, "/qn", "/norestart")
					// 3010: done, and a restart will finish it.
					if err != nil && strings.Contains(err.Error(), "exit status 3010") {
						return nil
					}
					return err
				}})
		}
	}

	// Firewall rules, by name.
	if out, err := e.Sys.Run(tool("netsh.exe"), "advfirewall", "firewall", "show", "rule", "name="+winName); err == nil && strings.Contains(out, winName) {
		pl.add(Item{Kind: "firewall rule", Name: winName, Group: GroupProgram, Admin: true, step: stepFiles,
			remove: func() error {
				_, err := e.Sys.Run(tool("netsh.exe"), "advfirewall", "firewall", "delete", "rule", "name="+winName)
				return err
			},
			gone: func() bool {
				out, err := e.Sys.Run(tool("netsh.exe"), "advfirewall", "firewall", "show", "rule", "name="+winName)
				return err != nil || !strings.Contains(out, winName)
			}})
	}

	// Registry: starting at sign-in, and the Installed apps entry.
	for _, hive := range []string{"HKCU", "HKLM"} {
		if _, ok := e.Sys.RegValue(hive, runKey, winName); ok {
			pl.add(Item{Kind: "start at sign-in", Name: hive + `\` + runKey + `\` + winName, Group: GroupProgram, Admin: hive == "HKLM", step: stepFiles,
				remove: func() error { return e.Sys.RegDeleteValue(hive, runKey, winName) },
				gone:   func() bool { _, ok := e.Sys.RegValue(hive, runKey, winName); return !ok }})
		}
		if e.Sys.RegKeyExists(hive, UninstallKey) {
			pl.add(Item{Kind: "Installed apps entry", Name: hive + `\` + UninstallKey, Group: GroupProgram, Admin: hive == "HKLM", step: stepFiles,
				remove: func() error { return e.Sys.RegDeleteKey(hive, UninstallKey) },
				gone:   func() bool { return !e.Sys.RegKeyExists(hive, UninstallKey) }})
		}
	}

	// Where the installer remembers its answers. The maker's key goes only once it's empty:
	// another of our products may be using it.
	for _, hive := range []string{"HKCU", "HKLM"} {
		if e.Sys.RegKeyExists(hive, settingsKey) {
			pl.add(Item{Kind: "installer's choices", Name: hive + `\` + settingsKey, Group: GroupProgram, Admin: hive == "HKLM", step: stepEmptyFolders,
				remove: func() error {
					if err := e.Sys.RegDeleteKey(hive, settingsKey); err != nil {
						return err
					}
					_ = e.Sys.RegDeleteKey(hive, makerKey)
					return nil
				},
				gone: func() bool { return !e.Sys.RegKeyExists(hive, settingsKey) }})
		}
	}

	// Shortcuts, in this account and for everyone.
	for _, s := range []struct {
		path  string
		admin bool
	}{
		{filepath.Join(roaming, `Microsoft\Windows\Start Menu\Programs`, winName+".lnk"), false},
		{filepath.Join(e.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs`, winName+".lnk"), true},
		{filepath.Join(e.Getenv("USERPROFILE"), "Desktop", winName+".lnk"), false},
		{filepath.Join(e.Getenv("PUBLIC"), "Desktop", winName+".lnk"), true},
	} {
		if exists(s.path) {
			pl.add(fileItem("shortcut", s.path, GroupProgram, s.admin))
		}
	}

	// The program, wherever it is.
	planProgramFolder(e, pl, programFiles, true)
	planProgramFolder(e, pl, perUser, false)
	if e.SelfRoot != "" {
		planProgramFolder(e, pl, e.SelfRoot, false)
	}

	// Library, accounts and settings.
	if d := e.Getenv("WMS_DATA_DIR"); d != "" {
		planDataFolder(e, pl, d, false, false)
	}
	planDataFolder(e, pl, filepath.Join(localApp, winName), true, false)
	planDataFolder(e, pl, programData, true, true)
	if d := filepath.Join(roaming, winName); isDir(d) {
		planConfigFile(e, pl, filepath.Join(d, "wms.json"), false)
		pl.add(emptyFolderItem(d, GroupData, false))
	}
	// Other accounts on this computer used the app too: an administrator can clear those.
	if e.IsAdmin {
		users := filepath.Dir(e.Getenv("USERPROFILE"))
		if entries, err := os.ReadDir(users); err == nil && e.Getenv("USERPROFILE") != "" {
			for _, u := range entries {
				base := filepath.Join(users, u.Name())
				if !u.IsDir() || samePath(base, e.Getenv("USERPROFILE")) {
					continue
				}
				planDataFolder(e, pl, filepath.Join(base, `AppData\Local`, winName), true, true)
				planConfigFile(e, pl, filepath.Join(base, `AppData\Roaming`, winName, "wms.json"), true)
				for _, s := range []string{`AppData\Roaming\Microsoft\Windows\Start Menu\Programs\` + winName + ".lnk", `Desktop\` + winName + ".lnk"} {
					if p := filepath.Join(base, s); exists(p) {
						pl.add(fileItem("shortcut", p, GroupProgram, true))
					}
				}
			}
		}
	}
}

var errNotWindows = errors.New("there is no registry here")
