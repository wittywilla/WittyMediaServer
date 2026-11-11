// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package uninstall

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/layout"
)

// TakeOver clears what a copy from 0.5.2-alpha or older left when an installer replaces it:
// the folder its setup program made, the boot task install.ps1 made, its firewall rule and
// startup entry. The library, accounts and settings stay, so the new copy finds them.
//
// The Windows installer runs it twice, as the person installing and as an administrator, so
// each pass does what it has the rights for.
func TakeOver(e *Env) []string {
	if e.GOOS != "windows" {
		return nil
	}
	var did []string
	note := func(format string, a ...any) { did = append(did, fmt.Sprintf(format, a...)) }

	sys32 := filepath.Join(e.Getenv("SystemRoot"), "System32")
	if e.IsAdmin {
		if out, err := e.Sys.Run(filepath.Join(sys32, "schtasks.exe"), "/Query", "/TN", winName); err == nil && strings.Contains(out, winName) {
			_, _ = e.Sys.Run(filepath.Join(sys32, "schtasks.exe"), "/End", "/TN", winName)
			if _, err := e.Sys.Run(filepath.Join(sys32, "schtasks.exe"), "/Delete", "/TN", winName, "/F"); err == nil {
				note("the old boot task")
			}
		}
		// The new rule, if one was asked for, is added after this runs.
		if out, err := e.Sys.Run(filepath.Join(sys32, "netsh.exe"), "advfirewall", "firewall", "show", "rule", "name="+winName); err == nil && strings.Contains(out, winName) {
			if _, err := e.Sys.Run(filepath.Join(sys32, "netsh.exe"), "advfirewall", "firewall", "delete", "rule", "name="+winName); err == nil {
				note("the old firewall rule")
			}
		}
	}

	for _, s := range []struct {
		dir   string
		admin bool
		hive  string
	}{
		{filepath.Join(e.Getenv("LOCALAPPDATA"), "Programs", winName), false, "HKCU"},
		{filepath.Join(e.Getenv("ProgramFiles"), winName), true, "HKLM"},
	} {
		if s.admin && !e.IsAdmin {
			continue
		}
		if !exists(filepath.Join(s.dir, "wms.exe")) {
			continue
		}
		e.Sys.StopProgramsIn(s.dir)
		for _, name := range append([]string{"wms.exe"}, layout.Obsolete()...) {
			p := filepath.Join(s.dir, name)
			if exists(p) && os.Remove(p) == nil {
				note("%s", p)
			}
		}
		// Is this folder now the new copy's? Then only the old parts go, as above.
		if _, ok := e.Sys.RegValue(s.hive, UninstallKey, "ProductCode"); !ok && !exists(filepath.Join(s.dir, layout.Marker)) {
			_ = os.Remove(filepath.Join(s.dir, "WittyMediaServer.exe"))
			_ = os.RemoveAll(filepath.Join(s.dir, "bin"))
			_ = os.RemoveAll(filepath.Join(s.dir, updateStage))
			_ = os.Remove(s.dir)
		}
		// The entries the old setup program wrote point at files that are gone.
		if _, ok := e.Sys.RegValue(s.hive, UninstallKey, "ProductCode"); !ok && e.Sys.RegKeyExists(s.hive, UninstallKey) {
			if e.Sys.RegDeleteKey(s.hive, UninstallKey) == nil {
				note(`%s\%s`, s.hive, UninstallKey)
			}
		}
		if v, ok := e.Sys.RegValue(s.hive, runKey, winName); ok && strings.Contains(strings.ToLower(v), strings.ToLower(filepath.Join(s.dir, "WittyMediaServer.exe"))) {
			if e.Sys.RegDeleteValue(s.hive, runKey, winName) == nil {
				note("the old start at sign-in")
			}
		}
	}
	return did
}

// updateStage is internal/update's staging folder, by its name there.
const updateStage = ".update"
