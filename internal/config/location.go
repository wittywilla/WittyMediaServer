// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Where the data folder came from (Config.DataFrom). Only a folder that came from the pointer,
// the install folder or the default can be moved from Settings: the others were set by hand,
// and are changed where they were set.
const (
	DataFromFlag      = "flag"      // -data
	DataFromEnv       = "env"       // WMS_DATA_DIR
	DataFromConfig    = "config"    // "dataDir" in wms.json
	DataFromPortable  = "portable"  // wms.json next to the program
	DataFromPointer   = "pointer"   // chosen in setup or Settings: data-location.json
	DataFromInstall   = "install"   // a data folder in the install folder, found without a pointer
	DataFromContainer = "container" // /config in a container
	DataFromService   = "service"   // a package's service folder, for root
	DataFromDefault   = "default"   // the platform's own folder: nothing was chosen
)

// Choices of where to keep the data, as setup offers them.
const (
	ChoiceDocuments = "documents"
	ChoiceInstall   = "install"
	ChoiceCustom    = "custom"
)

// PointerName is the small file that says where the data folder is. It sits beside the user's
// config file, which is read before anything opens the data.
const PointerName = "data-location.json"

// Pointer is what PointerName holds.
type Pointer struct {
	DataDir string    `json:"dataDir"`
	Choice  string    `json:"choice,omitempty"` // documents, install or custom
	At      time.Time `json:"at"`
}

// PointerFile is where the pointer is: %APPDATA%\WittyMediaServer on Windows, ~/Library/Application
// Support/WittyMediaServer on macOS, ~/.config/wittymediaserver elsewhere. WMS_POINTER_FILE moves it,
// for tests.
func PointerFile() string {
	if v := os.Getenv("WMS_POINTER_FILE"); v != "" {
		return v
	}
	return filepath.Join(filepath.Dir(UserConfigFile()), PointerName)
}

// ReadPointer reads the pointer. A missing file is (nil, nil).
func ReadPointer() (*Pointer, error) {
	b, err := os.ReadFile(PointerFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) > 64<<10 {
		return nil, fmt.Errorf("%s is too large", PointerFile())
	}
	var p Pointer
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", PointerFile(), err)
	}
	if p.DataDir == "" || !filepath.IsAbs(p.DataDir) {
		return nil, fmt.Errorf("%s: the data folder in it must be a full path", PointerFile())
	}
	p.DataDir = filepath.Clean(p.DataDir)
	return &p, nil
}

// WritePointer saves the pointer, all at once: a half-written pointer would lose the data folder.
func WritePointer(p Pointer) error {
	if !filepath.IsAbs(p.DataDir) {
		return errors.New("the data folder must be a full path")
	}
	p.DataDir = filepath.Clean(p.DataDir)
	if p.At.IsZero() {
		p.At = time.Now().UTC()
	}
	file := PointerFile()
	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

// LegacyDataDir is the platform's own data folder, where every desktop install kept its data
// before setup asked: %LOCALAPPDATA%\WittyMediaServer, ~/Library/Application Support/WittyMediaServer,
// ~/.local/share/wittymediaserver. A server on it keeps using it until someone moves it.
func LegacyDataDir() string { return platformDataDir() }

// InstallDataDir is the data folder in the install folder ("portable").
func InstallDataDir() string { return filepath.Join(ProgramDir(), "data") }

// DocumentsDataDir is WittyMediaServer's folder in the user's Documents.
func DocumentsDataDir() string {
	d := DocumentsDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "WittyMediaServer")
}

// DocumentsDir is the user's Documents folder, where the system says it is: OneDrive's when
// Windows moved it there, the folder in user-dirs.dirs on Linux. WMS_DOCUMENTS_DIR stands in for it,
// for tests. "" when there's none.
func DocumentsDir() string {
	if v := os.Getenv("WMS_DOCUMENTS_DIR"); v != "" {
		return v
	}
	if d := systemDocuments(); d != "" {
		return d
	}
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return ""
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		if d := xdgDocuments(h, os.Getenv("XDG_CONFIG_HOME")); d != "" {
			return d
		}
	}
	return filepath.Join(h, "Documents")
}

// xdgDocuments reads XDG_DOCUMENTS_DIR from user-dirs.dirs, as the desktop's own file manager does.
func xdgDocuments(home, configHome string) string {
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	b, err := os.ReadFile(filepath.Join(configHome, "user-dirs.dirs"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		v, ok := strings.CutPrefix(line, "XDG_DOCUMENTS_DIR=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"`)
		v = strings.Replace(v, "$HOME", home, 1)
		// A desktop with no Documents of its own points it at the home folder: that's no folder
		// to put ours in.
		if !filepath.IsAbs(v) || filepath.Clean(v) == filepath.Clean(home) {
			return ""
		}
		return filepath.Clean(v)
	}
	return ""
}

// Synced says which program keeps a folder in step with the cloud, or "". A live SQLite database
// in such a folder can be damaged when the copy on another computer, or the sync itself, writes
// to it while the server has it open.
func Synced(path string) string {
	h, _ := os.UserHomeDir()
	return syncedBy(path, os.Getenv, h, runtime.GOOS, dirExists)
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// syncedBy is Synced with what it looks at handed in, so tests can play every system.
func syncedBy(path string, getenv func(string) string, home, goos string, isDir func(string) bool) string {
	if path == "" {
		return ""
	}
	p := filepath.Clean(path)
	under := func(root string) bool {
		if root == "" {
			return false
		}
		root = filepath.Clean(root)
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return false
		}
		if goos == "windows" || goos == "darwin" {
			rel2, err := filepath.Rel(strings.ToLower(root), strings.ToLower(p))
			if err == nil {
				rel = rel2
			}
		}
		return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
	}
	// OneDrive says where it is, on Windows: Known Folder Move puts Documents inside it.
	for _, v := range []string{"OneDrive", "OneDriveConsumer", "OneDriveCommercial"} {
		if under(getenv(v)) {
			return "OneDrive"
		}
	}
	if goos == "darwin" && home != "" {
		// iCloud Desktop & Documents keeps ~/Documents itself in iCloud Drive, which shows as a
		// Documents folder inside it.
		if under(filepath.Join(home, "Documents")) && isDir(filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs", "Documents")) {
			return "iCloud Drive"
		}
		if under(filepath.Join(home, "Library", "Mobile Documents")) {
			return "iCloud Drive"
		}
		if under(filepath.Join(home, "Library", "CloudStorage")) {
			return "a cloud storage program"
		}
	}
	// Otherwise the folder names the sync programs give their own folders.
	names := map[string]string{
		"onedrive": "OneDrive", "dropbox": "Dropbox", "google drive": "Google Drive", "googledrive": "Google Drive",
		"my drive": "Google Drive", "icloud drive": "iCloud Drive", "iclouddrive": "iCloud Drive", "nextcloud": "Nextcloud",
		"owncloud": "ownCloud", "pclouddrive": "pCloud", "pcloud drive": "pCloud", "box sync": "Box",
		"megasync": "MEGA", "seafile": "Seafile", "tresorit": "Tresorit",
	}
	vol := filepath.VolumeName(p)
	for _, part := range strings.Split(strings.TrimPrefix(p, vol), string(filepath.Separator)) {
		low := strings.ToLower(part)
		if who, ok := names[low]; ok {
			return who
		}
		// "OneDrive - Contoso", "Dropbox (Personal)"
		for _, pre := range []string{"onedrive - ", "onedrive-", "dropbox (", "google drive ("} {
			if strings.HasPrefix(low, pre) {
				return names[strings.TrimRight(strings.TrimSuffix(pre, " ("), " -")]
			}
		}
	}
	return ""
}
