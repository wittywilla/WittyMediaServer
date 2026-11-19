// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// isolate points every per-user folder at a fresh one, so nothing of the real account is read
// or written, and returns the home folder.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for k, v := range map[string]string{
		"APPDATA": filepath.Join(home, "Roaming"), "LOCALAPPDATA": filepath.Join(home, "Local"),
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"), "XDG_DATA_HOME": filepath.Join(home, ".local", "share"),
		"HOME": home, "USERPROFILE": home, "WMS_DATA_DIR": "", "WMS_CONFIG": "", "WMS_POINTER_FILE": "",
		"WMS_DOCUMENTS_DIR": filepath.Join(home, "Documents"), "WMS_LISTEN": "",
	} {
		t.Setenv(k, v)
	}
	if _, err := os.Stat("/config"); err == nil && runtime.GOOS != "windows" {
		t.Skip("this machine has a /config folder, which a container's server uses before anything else")
	}
	if os.Geteuid() == 0 {
		t.Skip("root may be a package's service, which has a folder of its own")
	}
	return home
}

func TestDataFolderResolutionOrder(t *testing.T) {
	home := isolate(t)
	legacy := LegacyDataDir()
	if !strings.HasPrefix(legacy, home) {
		t.Fatalf("the platform folder %s isn't in the test's home %s", legacy, home)
	}

	// Nothing chosen: the platform's own folder.
	c, err := ParseOnly(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.DataDir != legacy || c.DataFrom != DataFromDefault {
		t.Fatalf("nothing chosen: %s from %s, want %s from default", c.DataDir, c.DataFrom, legacy)
	}

	// An install from before setup asked: its data is still found where it was.
	if err := os.MkdirAll(legacy, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "wms.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, _ = ParseOnly(nil); c.DataDir != legacy || c.DataFrom != DataFromDefault {
		t.Fatalf("an upgraded install: %s from %s, want its old folder", c.DataDir, c.DataFrom)
	}

	// The pointer wins over the platform folder.
	docs := DocumentsDataDir()
	if err := os.MkdirAll(docs, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := WritePointer(Pointer{DataDir: docs, Choice: ChoiceDocuments}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(PointerFile(), home) {
		t.Fatalf("the pointer %s isn't in the test's home", PointerFile())
	}
	if c, _ = ParseOnly(nil); c.DataDir != docs || c.DataFrom != DataFromPointer {
		t.Fatalf("with a pointer: %s from %s, want %s from pointer", c.DataDir, c.DataFrom, docs)
	}
	if c.LogDir != filepath.Join(docs, "logs") || c.TranscodeDir != filepath.Join(docs, "transcode") {
		t.Fatalf("the folders inside follow the data folder: logs %s, transcode %s", c.LogDir, c.TranscodeDir)
	}

	// The command line and the environment win over the pointer.
	other := filepath.Join(home, "other")
	if c, _ = ParseOnly([]string{"-data", other}); c.DataDir != other || c.DataFrom != DataFromFlag {
		t.Fatalf("-data: %s from %s", c.DataDir, c.DataFrom)
	}
	t.Setenv("WMS_DATA_DIR", other)
	if c, _ = ParseOnly(nil); c.DataDir != other || c.DataFrom != DataFromEnv {
		t.Fatalf("WMS_DATA_DIR: %s from %s", c.DataDir, c.DataFrom)
	}
	t.Setenv("WMS_DATA_DIR", "")

	// A config file's dataDir wins over the pointer too.
	cfgFile := filepath.Join(home, "wms.json")
	if err := os.WriteFile(cfgFile, []byte(`{"dataDir": "fromfile"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, _ = ParseOnly([]string{"-config", cfgFile}); c.DataDir != filepath.Join(home, "fromfile") || c.DataFrom != DataFromConfig {
		t.Fatalf("config file: %s from %s", c.DataDir, c.DataFrom)
	}
}

// A pointer to a folder that isn't there (a drive not connected) stops the start, rather than
// making a new, empty server.
func TestPointerToMissingFolderStops(t *testing.T) {
	home := isolate(t)
	gone := filepath.Join(home, "unplugged", "WittyMediaServer")
	if err := WritePointer(Pointer{DataDir: gone, Choice: ChoiceCustom}); err != nil {
		t.Fatal(err)
	}
	_, err := ParseOnly(nil)
	if err == nil || !strings.Contains(err.Error(), gone) || !strings.Contains(err.Error(), PointerFile()) {
		t.Fatalf("want an error naming the folder and the pointer, got %v", err)
	}
	if _, err := os.Stat(gone); err == nil {
		t.Fatal("the missing folder was made")
	}
	// -data still starts it, somewhere else.
	if c, err := ParseOnly([]string{"-data", filepath.Join(home, "x")}); err != nil || c.DataFrom != DataFromFlag {
		t.Fatalf("-data with a broken pointer: %v", err)
	}
}

func TestPointerMustBeAFullPath(t *testing.T) {
	isolate(t)
	if err := os.MkdirAll(filepath.Dir(PointerFile()), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(PointerFile(), []byte(`{"dataDir": "relative/data"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseOnly(nil); err == nil {
		t.Fatal("a relative pointer was used")
	}
	if err := WritePointer(Pointer{DataDir: "relative"}); err == nil {
		t.Fatal("a relative pointer was written")
	}
}

func TestSyncedFolders(t *testing.T) {
	none := func(string) string { return "" }
	never := func(string) bool { return false }
	sep := string(filepath.Separator)
	j := func(parts ...string) string { return sep + filepath.Join(parts...) }
	if runtime.GOOS == "windows" {
		j = func(parts ...string) string { return `C:\` + filepath.Join(parts...) }
	}
	home := j("home", "you")
	cases := []struct {
		name, path, goos, want string
		env                    map[string]string
		icloud                 bool
	}{
		{name: "OneDrive's own folder, from its variable", path: j("home", "you", "OneDrive", "Documents", "WittyMediaServer"), env: map[string]string{"OneDrive": j("home", "you", "OneDrive")}, want: "OneDrive"},
		{name: "a work OneDrive", path: j("home", "you", "Docs", "WittyMediaServer"), env: map[string]string{"OneDriveCommercial": j("home", "you", "Docs")}, want: "OneDrive"},
		{name: "OneDrive by its folder's name", path: j("home", "you", "OneDrive - Contoso", "Documents"), want: "OneDrive"},
		{name: "Dropbox", path: j("home", "you", "Dropbox", "WittyMediaServer"), want: "Dropbox"},
		{name: "Dropbox (Personal)", path: j("home", "you", "Dropbox (Personal)", "x"), want: "Dropbox"},
		{name: "Nextcloud", path: j("home", "you", "Nextcloud", "WittyMediaServer"), want: "Nextcloud"},
		{name: "a plain folder", path: j("home", "you", "Documents", "WittyMediaServer"), want: ""},
		{name: "a folder merely beside OneDrive", path: j("home", "you", "OneDriveBackup"), env: map[string]string{"OneDrive": j("home", "you", "OneDrive")}, want: ""},
		{name: "iCloud Desktop & Documents", goos: "darwin", path: j("home", "you", "Documents", "WittyMediaServer"), icloud: true, want: "iCloud Drive"},
		{name: "Documents without iCloud", goos: "darwin", path: j("home", "you", "Documents", "WittyMediaServer"), want: ""},
		{name: "iCloud Drive itself", goos: "darwin", path: j("home", "you", "Library", "Mobile Documents", "com~apple~CloudDocs", "x"), want: "iCloud Drive"},
		{name: "a Mac's CloudStorage", goos: "darwin", path: j("home", "you", "Library", "CloudStorage", "GoogleDrive-x", "y"), want: "a cloud storage program"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			getenv := none
			if c.env != nil {
				getenv = func(k string) string { return c.env[k] }
			}
			isDir := never
			if c.icloud {
				isDir = func(p string) bool { return strings.Contains(p, "com~apple~CloudDocs") }
			}
			goos := c.goos
			if goos == "" {
				goos = runtime.GOOS
			}
			if got := syncedBy(c.path, getenv, home, goos, isDir); got != c.want {
				t.Fatalf("syncedBy(%s) = %q, want %q", c.path, got, c.want)
			}
		})
	}
}

func TestXDGDocuments(t *testing.T) {
	home := t.TempDir()
	conf := filepath.Join(home, ".config")
	if err := os.MkdirAll(conf, 0o750); err != nil {
		t.Fatal(err)
	}
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(conf, "user-dirs.dirs"), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("# comment\nXDG_DESKTOP_DIR=\"$HOME/Desktop\"\nXDG_DOCUMENTS_DIR=\"$HOME/Dokumente\"\n")
	if got, want := xdgDocuments(home, ""), filepath.Join(home, "Dokumente"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	write("XDG_DOCUMENTS_DIR=\"$HOME/\"\n")
	if got := xdgDocuments(home, ""); got != "" {
		t.Fatalf("Documents set to the home folder should be no answer, got %q", got)
	}
}
