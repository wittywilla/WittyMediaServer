// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/layout"
)

// settings.ini is the settings file a person edits in Notepad: a few `key = value` lines with
// notes, today only the port. It works without portable mode, and is read from two places, the
// first that sets a value winning:
//
//  1. settings.ini in the data folder. Settings → Network saves here when the program's folder
//     can't be changed, such as Program Files, /usr, or an app on a Mac, and installers carry an
//     earlier install's port over into it.
//  2. settings.ini in the program's top folder, beside WittyMediaServer(.exe). Archives and
//     installers put one there, and Settings → Network saves to it when it can.
//
// A port given with -listen, WMS_LISTEN or "listen" in wms.json wins over both: those are for
// services, containers and scripts, and Settings → Network says so instead of saving.
const SettingsName = "settings.ini"

// Where the address came from, strongest first.
const (
	FromFlag     = "flag"     // -listen
	FromEnv      = "env"      // WMS_LISTEN
	FromConfig   = "config"   // "listen" in wms.json
	FromSettings = "settings" // port in a settings.ini
	FromDefault  = "default"  // DefaultPort
)

// The lowest and highest ports Settings accepts. A file can say anything a port can be.
const (
	MinPort = 1
	MaxPort = 65535
)

// programDir and kindOf are this program's folder and how it was installed, replaced by tests.
var (
	programDir = ProgramDir
	kindOf     = func() string { return layout.Current().Kind }
)

// ProgramSettingsFile is the settings.ini beside the program. "" inside a Mac app, where a
// changed file would break the app's signature, so nothing there is read or written.
func ProgramSettingsFile() string {
	dir := programDir()
	if filepath.Base(dir) == "Contents" {
		return ""
	}
	return filepath.Join(dir, SettingsName)
}

// DataSettingsFile is the settings.ini in a data folder.
func DataSettingsFile(dataDir string) string { return filepath.Join(dataDir, SettingsName) }

// SettingsFiles are the settings files read for a data folder, strongest first.
func SettingsFiles(dataDir string) []string {
	out := []string{DataSettingsFile(dataDir)}
	if p := ProgramSettingsFile(); p != "" && p != out[0] {
		out = append(out, p)
	}
	return out
}

// Settings is what a settings.ini says. Port is 0 when it says nothing about it.
type Settings struct {
	Port int
	// Problems are lines that were skipped, each naming the file and line.
	Problems []string
}

// ReadSettings reads a settings.ini. A missing file is no error and sets nothing. A line it
// can't use is skipped and reported, instead of stopping the server: a beginner's typo should
// leave a server that starts, says what's wrong, and can be fixed from Settings.
func ReadSettings(path string) (Settings, error) {
	var s Settings
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\uFEFF")) // Notepad's byte order mark
		if line == "" || line[0] == '#' || line[0] == ';' || line[0] == '[' {
			continue
		}
		bad := func(why string) { s.Problems = append(s.Problems, fmt.Sprintf("%s, line %d: %s", path, n, why)) }
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			bad(fmt.Sprintf("%q isn't a setting: write it as name = value", line))
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if i := strings.IndexAny(value, "#;"); i >= 0 {
			value = value[:i]
		}
		value = strings.TrimSpace(value)
		switch key {
		case "port":
			p, err := strconv.Atoi(value)
			if err != nil || p < MinPort || p > MaxPort {
				bad(fmt.Sprintf("the port should be a number from %d to %d, not %q", MinPort, MaxPort, value))
				continue
			}
			s.Port = p
		default:
			bad(fmt.Sprintf("there's no setting called %q", key))
		}
	}
	return s, sc.Err()
}

// settingsPort is the port the settings files set for a data folder, and the file that set it.
func settingsPort(dataDir string) (port int, file string, problems []string) {
	for _, p := range SettingsFiles(dataDir) {
		s, err := ReadSettings(p)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		problems = append(problems, s.Problems...)
		if s.Port != 0 && port == 0 {
			port, file = s.Port, p
		}
	}
	return port, file, problems
}

// SettingsSaveFile is where Settings saves the port for a data folder: the data folder's copy if
// there is one, as it wins; else the one beside the program, if it's there, no installer owns
// that folder, and this program may change it; else a new copy in the data folder.
func SettingsSaveFile(dataDir string) string {
	data := DataSettingsFile(dataDir)
	if exists(data) {
		return data
	}
	if p := ProgramSettingsFile(); p != "" && !Managed(kindOf()) && exists(p) && canWrite(p) {
		return p
	}
	return data
}

// canWrite says whether this program may change a file, without changing it.
func canWrite(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// SavePort writes the port into a settings.ini, keeping every other line as it was. A file that
// isn't there yet starts as the one an installer ships.
func SavePort(path string, port int) error {
	if port < MinPort || port > MaxPort {
		return fmt.Errorf("the port should be a number from %d to %d", MinPort, MaxPort)
	}
	b, err := readPlainFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return writeFileAtomic(path, []byte(SettingsText(port, true)))
	}
	if err != nil {
		return err
	}
	text := string(b)
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n" // as Notepad left it
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	found := false
	for i, l := range lines {
		t := strings.TrimSpace(strings.TrimPrefix(l, "\uFEFF"))
		key, _, ok := strings.Cut(t, "=")
		if !ok || t == "" || t[0] == '#' || t[0] == ';' || strings.ToLower(strings.TrimSpace(key)) != "port" {
			continue
		}
		if found {
			lines[i] = "" // a second port line would be confusing: the first one is the one used
			continue
		}
		lead := l[:len(l)-len(strings.TrimLeft(l, " \t\uFEFF"))]
		lines[i] = lead + "port = " + strconv.Itoa(port)
		found = true
	}
	if !found {
		if n := len(lines); n > 0 && lines[n-1] == "" {
			lines = lines[:n-1]
		}
		lines = append(lines, "port = "+strconv.Itoa(port), "")
	}
	return writeFileAtomic(path, []byte(strings.Join(lines, nl)))
}

// readPlainFile reads a file that is itself there: not a link, and not a second name for another
// file. Installers run this as root on the service's data folder, which the service can write to,
// and what they read goes back into that folder.
func readPlainFile(path string) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || extraLinks(before) {
		return nil, fmt.Errorf("%s isn't a plain file: remove it, and it's made again", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if now, err := f.Stat(); err != nil || !os.SameFile(before, now) {
		return nil, fmt.Errorf("%s changed while it was read", path)
	}
	return io.ReadAll(io.LimitReader(f, 1<<20))
}

// writeFileAtomic replaces a file in one step, so a power cut leaves the old one or the new one.
func writeFileAtomic(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	mode := os.FileMode(0o644) // nothing in it is secret, and someone should be able to read it
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	_ = os.Chmod(tmp, mode)
	return os.Rename(tmp, path)
}

// SettingsText is a whole settings.ini: the one archives and installers put beside the program,
// or with dataCopy, the copy in the data folder.
func SettingsText(port int, dataCopy bool) string {
	where := `# This file is beside the program. Where that folder can't be changed (Program Files,
# /usr, a Mac's Applications), Settings → Network saves in a copy in the data folder
# instead, and that copy wins:
#   Windows service:          C:\ProgramData\WittyMediaServer\settings.ini
#   Windows, the app:         %LOCALAPPDATA%\WittyMediaServer\settings.ini
#   Linux package:            /var/lib/wittymediaserver/settings.ini
#   FreeBSD package:          /var/db/wittymediaserver/settings.ini
#   Mac:                      ~/Library/Application Support/WittyMediaServer/settings.ini
#   Portable mode:            data\settings.ini, beside this file`
	if dataCopy {
		where = `# This copy is in the server's data folder, and wins over the settings.ini beside the
# program. Settings → Network saves here.`
	}
	return `# WittyMediaServer settings
#
# Change the number after "port =", save this file, and restart WittyMediaServer: from its
# tray or menu-bar icon, or Settings → Network, which can also change it for you and checks
# that the port is free. Lines that start with # are notes, and are skipped.
#
` + where + `
#
# -listen, WMS_LISTEN and "listen" in wms.json win over this file. In Docker, change the
# published port in docker-compose.yml instead.

# The port browsers and apps connect to: http://<this computer>:` + strconv.Itoa(port) + `
# A number from 1024 to 65535 that nothing else on this computer uses.
port = ` + strconv.Itoa(port) + `
`
}

// Managed says whether an installer owns the program's folder: Settings then saves in the data
// folder, and an update or upgrade puts its own files back.
func Managed(kind string) bool {
	switch kind {
	case layout.KindMSI, layout.KindDeb, layout.KindRPM, layout.KindPkg, layout.KindFreeBSD, layout.KindAppImage, layout.KindFlatpak, layout.KindSnap:
		return true
	}
	return false
}
