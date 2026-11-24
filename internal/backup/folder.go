// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// A folder of one's own: backups can be kept somewhere other than the data folder, such as
// a second drive or a share, which an administrator types. Only finished archives (and the
// work of making one) go there. A restore's journal, safety copies and checks stay in the
// data folder's own backups folder, beside what they put back.
//
// A drive that isn't plugged in, or a mount point with nothing mounted on it, mustn't be
// written to by mistake: the folder gets a marker file when it's chosen, and a backup is
// only made there while the marker is still there.

// MarkerName is the file that says a folder is the one chosen for backups.
const MarkerName = ".wittymediaserver-backups"

const markerText = "WittyMediaServer keeps its backups in this folder. Leave this file here: without it, backups stop, in case this is a drive that isn't plugged in.\n"

// ArchiveDir is where new archives go: the chosen folder, or the data folder's.
func (f Folders) ArchiveDir() string {
	if f.Store != "" {
		return f.Store
	}
	return f.Dir()
}

// archiveDirs are every folder archives are listed from: the chosen one first, then the
// data folder's, so the ones made before a change of folder can still be used.
func (f Folders) archiveDirs() []string {
	if f.Store == "" || samePath(f.Store, f.Dir()) {
		return []string{f.Dir()}
	}
	return []string{f.Store, f.Dir()}
}

// StoreReady says whether the chosen folder can take a backup now. It's nil when there's no
// chosen folder.
func (f Folders) StoreReady() error {
	if f.Store == "" {
		return nil
	}
	st, err := os.Stat(f.Store)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("the backup folder %s isn't there. Is the drive plugged in?", f.Store)
	}
	if m, err := os.Lstat(filepath.Join(f.Store, MarkerName)); err != nil || !m.Mode().IsRegular() {
		return fmt.Errorf("the backup folder %s is missing its %s file, so it may not be the drive you chose. Is the drive plugged in? Choose the folder again to carry on", f.Store, MarkerName)
	}
	return nil
}

// FolderRules are the places a chosen folder mustn't be in, or hold.
type FolderRules struct {
	Data    string   // the data folder: its own backups folder is the default, so not chosen
	Protect []string // the program, cache, logs and other folders of this server
	Media   []string // library folders: a scan would find the archives
}

// CheckFolder checks a typed folder, and marks it as chosen. An empty path means the
// data folder's own, and comes back empty. The folder must already exist: making one on a
// drive that isn't there would write to the wrong disk.
func CheckFolder(path string, rules FolderRules) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if len(path) > 1024 || strings.ContainsRune(path, 0) {
		return "", errors.New("that isn't a folder")
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("type the whole path, from the drive or the top folder")
	}
	path = filepath.Clean(path)
	if rules.Data != "" && samePath(path, filepath.Join(rules.Data, DirName)) {
		return "", nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("there's no folder at %s. Make it first, then type it here", path)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("%s is a file, not a folder", path)
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if isRoot(path) {
		return "", errors.New("choose a folder on that drive, not the whole drive")
	}
	inside := func(list []string, what string) error {
		for _, p := range list {
			if p == "" {
				continue
			}
			if real, err := filepath.EvalSymlinks(p); err == nil {
				p = real
			}
			if within(path, p) || within(p, path) {
				return fmt.Errorf("that folder is %s. Choose one of its own", what)
			}
		}
		return nil
	}
	if err := inside([]string{rules.Data}, "part of the server's data folder, or holds it"); err != nil {
		return "", err
	}
	if err := inside(rules.Protect, "one the server already uses, or holds one"); err != nil {
		return "", err
	}
	if err := inside(rules.Media, "in a library, or holds one: a scan would find the backups"); err != nil {
		return "", err
	}
	marker := filepath.Join(path, MarkerName)
	if m, err := os.Lstat(marker); err == nil && !m.Mode().IsRegular() {
		return "", fmt.Errorf("%s in that folder isn't an ordinary file", MarkerName)
	}
	if err := os.WriteFile(marker, []byte(markerText), 0o600); err != nil {
		return "", fmt.Errorf("the server can't write to %s: %v", path, err)
	}
	return path, nil
}

// within says whether p is dir or inside it.
func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		rel = strings.ToLower(rel)
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func isRoot(p string) bool {
	return filepath.Dir(p) == p
}
