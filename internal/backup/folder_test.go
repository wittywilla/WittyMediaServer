// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestChosenFolderIsChecked(t *testing.T) {
	f, media := aServer(t)
	program := filepath.Join(t.TempDir(), "program")
	rules := FolderRules{Data: f.Data, Protect: []string{program}, Media: []string{media}}
	if err := os.MkdirAll(media, 0o750); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "second drive", "backups")
	if err := os.MkdirAll(other, 0o750); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(other, "a file")
	writeTestFile(t, file, []byte("x"))
	for name, c := range map[string]struct{ path, want string }{
		"relative":        {"backups", "whole path"},
		"not there":       {filepath.Join(other, "nope"), "no folder"},
		"a file":          {file, "is a file"},
		"the data folder": {f.Data, "data folder"},
		"inside the data": {filepath.Join(f.Data, "modules"), "data folder"},
		"holds the data":  {filepath.Dir(f.Data), "data folder"},
		"a library":       {media, "library"},
		"the program":     {program, "already uses"},
		"a whole drive":   {filepath.VolumeName(other) + string(filepath.Separator), "whole drive"},
		"a nul in it":     {other + "\x00x", "isn't a folder"},
	} {
		if name == "the program" {
			_ = os.MkdirAll(program, 0o750)
		}
		if name == "inside the data" {
			_ = os.MkdirAll(c.path, 0o750)
		}
		if _, err := CheckFolder(c.path, rules); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
	// The data folder's own backups folder is the default: it comes back empty.
	if got, err := CheckFolder(filepath.Join(f.Data, DirName), rules); err != nil || got != "" {
		t.Fatalf("default: %q %v", got, err)
	}
	got, err := CheckFolder("  "+other+"  ", rules)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(got, MarkerName)); err != nil {
		t.Fatal("no marker", err)
	}
}

func TestBackupsGoToTheChosenFolder(t *testing.T) {
	f, _ := aServer(t)
	// One made before the folder was chosen.
	old, _, err := Create(ctx, Options{Folders: f, Parts: []string{PartSettings}, ServerVersion: "1.0"})
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(t.TempDir(), "usb")
	if err := os.MkdirAll(store, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckFolder(store, FolderRules{Data: f.Data}); err != nil {
		t.Fatal(err)
	}
	f.Store = store
	out, _, err := Create(ctx, Options{Folders: f, Parts: []string{PartSettings}, Auto: true, ServerVersion: "1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(out) != store {
		t.Fatalf("made in %s", out)
	}
	list := List(f)
	if len(list) != 2 {
		t.Fatalf("both should be listed: %+v", list)
	}
	for _, b := range list {
		p, err := f.PathOf(b.Name)
		if err != nil {
			t.Fatal(err)
		}
		if b.Elsewhere != (filepath.Dir(p) == store) {
			t.Fatalf("%s elsewhere=%v at %s", b.Name, b.Elsewhere, p)
		}
	}
	if _, err := f.PathOf(filepath.Base(old)); err != nil {
		t.Fatal("the old one can't be found", err)
	}
	// Nothing of the work is left behind on the drive.
	entries, _ := os.ReadDir(store)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".backup-") {
			t.Fatalf("left %s", e.Name())
		}
	}

	// The drive unplugged, as an empty mount point: nothing is written there.
	if err := os.Remove(filepath.Join(store, MarkerName)); err != nil {
		t.Fatal(err)
	}
	if err := f.StoreReady(); err == nil {
		t.Fatal("a folder without its marker was taken")
	}
	if _, _, err := Create(ctx, Options{Folders: f, Parts: []string{PartSettings}, Auto: true, ServerVersion: "1.0"}); err == nil || !strings.Contains(err.Error(), "plugged in") {
		t.Fatalf("made a backup with no marker: %v", err)
	}
	// And a drive that's gone isn't made again.
	f.Store = filepath.Join(t.TempDir(), "gone", "backups")
	if _, _, err := Create(ctx, Options{Folders: f, Parts: []string{PartSettings}, ServerVersion: "1.0"}); err == nil {
		t.Fatal("made a backup on a drive that isn't there")
	}
	if _, err := os.Stat(f.Store); err == nil {
		t.Fatal("the missing folder was made")
	}
}

func TestPruneCoversBothFolders(t *testing.T) {
	f, _ := aServer(t)
	store := filepath.Join(t.TempDir(), "usb")
	if err := os.MkdirAll(f.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatal(err)
	}
	f.Store = store
	for i, p := range []string{filepath.Join(f.Dir(), "wms-backup-auto-20260101-000000.zip"), filepath.Join(store, "wms-backup-auto-20260102-000000.zip"), filepath.Join(store, "wms-backup-auto-20260103-000000.zip")} {
		writeTestFile(t, p, []byte("x"))
		at := time.Now().Add(time.Duration(i-10) * time.Hour)
		_ = os.Chtimes(p, at, at)
	}
	if removed := Prune(f, 2); len(removed) != 1 || removed[0] != "wms-backup-auto-20260101-000000.zip" {
		t.Fatalf("%v", removed)
	}
}
