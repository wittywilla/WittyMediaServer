// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package layout knows what an installed WittyMediaServer looks like, on every system
// and however it was installed, and what its programs are called.
//
// A package has one folder a person looks in, the root, holding what they open. Everything
// else that runs lives in bin, out of sight:
//
//	<root>/WittyMediaServer(.exe)              the app, with the tray or menu-bar icon
//	<root>/Uninstall WittyMediaServer.exe      Windows only
//	<root>/package-info.json                   how this copy was installed
//	<root>/wms.json, <root>/data               only when someone chose portable mode
//	<root>/bin/wittymediaserver(.exe)          the server, and the command line
//	<root>/bin/ffmpeg(.exe), ffprobe(.exe)
//	<root>/modules                             signed modules for offline use, when it has any
//
// On a Mac the same parts sit where Apple wants them:
//
//	WittyMediaServer.app/Contents/MacOS/WittyMediaServer
//	WittyMediaServer.app/Contents/Resources/package-info.json, modules
//	WittyMediaServer.app/Contents/Helpers/wittymediaserver, ffmpeg, ffprobe, ...
//
// A program built from source, or a package from before 0.6.0, has no package-info.json:
// its root and its bin are simply the folder it is in.
package layout

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// DesktopApp is what a person opens: the tray or menu-bar app.
	DesktopApp = "WittyMediaServer"
	// ServerCommand is the server and its command line.
	ServerCommand = "wittymediaserver"
	// UninstallApp removes everything, asking first (Windows; a Mac has its own app).
	UninstallApp = "Uninstall WittyMediaServer"
	// Marker is the file that says a folder is an installed package.
	Marker = "package-info.json"
)

// How a copy was installed. The kind decides who may replace its files.
const (
	KindSource  = ""            // built from source, or older than 0.6.0
	KindArchive = "archive"     // a .zip or .tar.gz someone unpacked
	KindMSI     = "msi"         // the Windows installer
	KindDeb     = "deb"         // Debian, Ubuntu and relatives
	KindRPM     = "rpm"         // Fedora, RHEL, openSUSE and relatives
	KindPkg     = "pkg"         // the macOS installer
	KindFreeBSD = "freebsd-pkg" // FreeBSD's pkg
)

// Package is where a copy of the server is.
type Package struct {
	Root string // the folder a person looks in, and where portable files go
	Bin  string // the server and its helpers
	Kind string
}

// Info is the content of the marker file.
type Info struct {
	Product string `json:"product"`
	Kind    string `json:"kind"`
}

// Exe is a program's file name on this system.
func Exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Find works out the package that the program at exe belongs to.
func Find(exe string) Package {
	dir := filepath.Dir(exe)
	parent := filepath.Dir(dir)
	base := filepath.Base(dir)
	if filepath.Base(parent) == "Contents" && (base == "MacOS" || base == "Helpers") {
		if kind, ok := readKind(filepath.Join(parent, "Resources", Marker)); ok {
			return Package{Root: parent, Bin: filepath.Join(parent, "Helpers"), Kind: kind}
		}
	}
	if base == "bin" {
		if kind, ok := readKind(filepath.Join(parent, Marker)); ok {
			return Package{Root: parent, Bin: dir, Kind: kind}
		}
	}
	if kind, ok := readKind(filepath.Join(dir, Marker)); ok {
		return Package{Root: dir, Bin: filepath.Join(dir, "bin"), Kind: kind}
	}
	return Package{Root: dir, Bin: dir, Kind: KindSource}
}

// Current is the package the running program belongs to.
func Current() Package {
	exe, err := os.Executable()
	if err != nil {
		return Package{Root: ".", Bin: "."}
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	return Find(exe)
}

func readKind(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var info Info
	if err := json.Unmarshal(b, &info); err != nil || info.Product != DesktopApp {
		return "", false
	}
	return strings.ToLower(info.Kind), true
}

// ServerPath is the server program of this package.
func (p Package) ServerPath() string { return filepath.Join(p.Bin, Exe(ServerCommand)) }

// MarkerPath is this package's marker file.
func (p Package) MarkerPath() string {
	if filepath.Base(p.Root) == "Contents" {
		return filepath.Join(p.Root, "Resources", Marker)
	}
	return filepath.Join(p.Root, Marker)
}

// Modules is where a starter set of signed modules is, when this package has one.
func (p Package) Modules() string {
	if filepath.Base(p.Root) == "Contents" {
		return filepath.Join(p.Root, "Resources", "modules")
	}
	return filepath.Join(p.Root, "modules")
}

// ProgramDirs are the folders a helper program can be in, best first: its own folder,
// and for a package updated from an older one, the root.
func (p Package) ProgramDirs() []string {
	dirs := []string{p.Bin}
	if p.Root != p.Bin {
		dirs = append(dirs, p.Root)
	}
	return append(dirs, filepath.Join(p.Root, "bin"))
}

// Obsolete lists what packages up to 0.5.2-alpha kept in the root and a package no longer
// holds. An update leaves files it doesn't replace, so the new server clears these itself.
// Only ever exact names in the root of a package.
func Obsolete() []string {
	return []string{
		"wms-setup.exe", "install.ps1", "uninstall.ps1", "start-wms.cmd", "icon.ico", "icon.png",
		"wittymediaserver.desktop", "wittyplay-host", "wittyplay-host.exe",
		"CHANGELOG.md", "LICENSE", "NOTICE", "README.md", "THIRD_PARTY_LICENSES.md",
	}
}

// ClearObsolete removes what Obsolete names, once a package's new parts are in place.
// Files still in use stay until the next start.
func (p Package) ClearObsolete() {
	if p.Kind == KindSource || !fileExists(p.ServerPath()) {
		return
	}
	for _, n := range Obsolete() {
		_ = os.Remove(filepath.Join(p.Root, n))
	}
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
