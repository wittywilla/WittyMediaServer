// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package datahome

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/config"
)

// setup makes a data folder with what a server keeps, and points the pointer somewhere private.
func setup(t *testing.T) (root, data string) {
	t.Helper()
	root = t.TempDir()
	t.Setenv("WMS_POINTER_FILE", filepath.Join(root, "config", config.PointerName))
	data = filepath.Join(root, "old")
	for name, body := range map[string]string{
		"wms.db": "database", "wms.db-wal": "wal", "settings.ini": "port = 9797\n",
		"cache/images/a.jpg": "jpeg", "logs/server.log": "log", "modules/wittyplay/x.bin": "module",
		"transcode/seg1.ts": "temporary", "instance.lock": "", "instance.json": "{}",
		"setup-code.txt": "123456", "themes/mine.css": "css",
	} {
		p := filepath.Join(data, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root, data
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMoveKeepsOldCopy(t *testing.T) {
	root, data := setup(t)
	to := filepath.Join(root, "Documents", "WittyMediaServer")
	if err := Ask(data, Request{To: to, Choice: config.ChoiceDocuments, KeepOld: true}); err != nil {
		t.Fatal(err)
	}
	prog := &Progress{}
	moved, got, res := Apply(data, prog)
	if !moved || got != to || res == nil || !res.OK {
		t.Fatalf("Apply = %v %q %+v", moved, got, res)
	}
	for name, want := range map[string]string{"wms.db": "database", "wms.db-wal": "wal", "settings.ini": "port = 9797\n", "cache/images/a.jpg": "jpeg", "modules/wittyplay/x.bin": "module", "setup-code.txt": "123456"} {
		if got := read(t, filepath.Join(to, filepath.FromSlash(name))); got != want {
			t.Fatalf("%s = %q", name, got)
		}
	}
	for _, name := range []string{"transcode", "instance.lock", "instance.json", requestFile, marker} {
		if _, err := os.Stat(filepath.Join(to, name)); err == nil {
			t.Fatalf("%s was copied", name)
		}
	}
	if prog.Done.Load() != prog.Total.Load() || prog.Total.Load() == 0 {
		t.Fatalf("progress %d of %d", prog.Done.Load(), prog.Total.Load())
	}
	// The pointer says where it went.
	p, err := config.ReadPointer()
	if err != nil || p == nil || p.DataDir != to || p.Choice != config.ChoiceDocuments {
		t.Fatalf("pointer %+v, %v", p, err)
	}
	// The old copy stays, with a note, until it's deleted.
	if read(t, filepath.Join(data, "wms.db")) != "database" || !strings.Contains(read(t, filepath.Join(data, NoteFile)), to) {
		t.Fatal("the old copy or its note is missing")
	}
	if Pending(data) != nil {
		t.Fatal("the request is still waiting")
	}
	old := OldCopy(to)
	if old == nil || old.From != data {
		t.Fatalf("OldCopy = %+v", old)
	}
	if Last(to) == nil || !Last(to).OK {
		t.Fatal("the new folder doesn't say the move worked")
	}
	// Something of someone's own in the old folder stays when the copy is deleted.
	if err := os.WriteFile(filepath.Join(data, "mine.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := DeleteOld(to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data, "wms.db")); err == nil {
		t.Fatal("the old database is still there")
	}
	if read(t, filepath.Join(data, "mine.txt")) != "mine" {
		t.Fatal("a file the move didn't copy was deleted")
	}
	for _, gone := range []string{NoteFile, "transcode", "instance.lock", "cache"} {
		if _, err := os.Stat(filepath.Join(data, gone)); err == nil {
			t.Fatalf("%s is still in the old folder", gone)
		}
	}
	if OldCopy(to) != nil {
		t.Fatal("the old copy is still offered")
	}
}

// First-time setup: nothing in the old folder is worth keeping, so it goes.
func TestMoveInSetupClearsOldFolder(t *testing.T) {
	root, data := setup(t)
	to := filepath.Join(root, "new")
	if err := Ask(data, Request{To: to, Choice: config.ChoiceCustom}); err != nil {
		t.Fatal(err)
	}
	// The running server's own files are what's left, as when it holds them.
	if moved, _, _ := Apply(data, nil); !moved {
		t.Fatal("didn't move")
	}
	ents, _ := os.ReadDir(data)
	for _, e := range ents {
		if e.Name() != "instance.lock" && e.Name() != "instance.json" {
			t.Fatalf("%s is still in the old folder", e.Name())
		}
	}
	if OldCopy(to) != nil {
		t.Fatal("an old copy is offered after setup")
	}
	// Tidy finishes the job once nothing holds them.
	_ = os.WriteFile(filepath.Join(data, "logs"), []byte("left behind"), 0o600)
	Tidy(to)
	if _, err := os.Stat(filepath.Join(data, "logs")); err == nil {
		t.Fatal("Tidy left the old logs")
	}
}

// A copy that fails changes nothing: the data stays, the half copy goes, and it isn't tried again.
func TestFailedMoveChangesNothing(t *testing.T) {
	root, data := setup(t)
	to := filepath.Join(root, "new")
	if err := Ask(data, Request{To: to, Choice: config.ChoiceCustom}); err != nil {
		t.Fatal(err)
	}
	// Someone puts a file where the copy is going, after it was asked for.
	if err := os.MkdirAll(to, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(to, "theirs.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	moved, _, res := Apply(data, nil)
	if moved || res == nil || res.OK || res.Error == "" {
		t.Fatalf("Apply = %v %+v", moved, res)
	}
	if read(t, filepath.Join(to, "theirs.txt")) != "x" {
		t.Fatal("their file was touched")
	}
	if read(t, filepath.Join(data, "wms.db")) != "database" {
		t.Fatal("the data was touched")
	}
	if p, _ := config.ReadPointer(); p != nil {
		t.Fatal("a pointer was written")
	}
	if Pending(data) != nil {
		t.Fatal("it would be tried again")
	}
	if l := Last(data); l == nil || l.OK {
		t.Fatal("the old folder doesn't say it failed")
	}
}

// A copy cut short (the computer turned off) is ours: the next try clears it and starts again.
func TestCopyCutShortIsRetried(t *testing.T) {
	root, data := setup(t)
	to := filepath.Join(root, "new")
	if err := os.MkdirAll(filepath.Join(to, "cache"), 0o750); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(to, marker), nil, 0o600)
	_ = os.WriteFile(filepath.Join(to, "wms.db"), []byte("data"), 0o600) // half of it
	if err := Ask(data, Request{To: to, Choice: config.ChoiceCustom, KeepOld: true}); err != nil {
		t.Fatal(err)
	}
	if moved, _, res := Apply(data, nil); !moved {
		t.Fatalf("didn't move: %+v", res)
	}
	if read(t, filepath.Join(to, "wms.db")) != "database" {
		t.Fatal("the half copy wasn't replaced")
	}
}

func TestCheckTarget(t *testing.T) {
	root, data := setup(t)
	full := filepath.Join(root, "full")
	_ = os.MkdirAll(full, 0o750)
	_ = os.WriteFile(filepath.Join(full, "x"), nil, 0o600)
	another := filepath.Join(root, "another")
	_ = os.MkdirAll(another, 0o750)
	_ = os.WriteFile(filepath.Join(another, "wms.db"), nil, 0o600)
	file := filepath.Join(root, "afile")
	_ = os.WriteFile(file, nil, 0o600)
	for _, c := range []struct {
		to, want string
	}{
		{"relative", "full path"},
		{data, "already in this folder"},
		{filepath.Join(data, "inner"), "inside the current data folder"},
		{root, "current data folder is inside"},
		{full, "has files in it"},
		{another, "already holds"},
		{file, "a file with that name"},
		{config.ProgramDir(), "program's own folder"},
		{filepath.Join(root, "fresh", "WittyMediaServer"), ""},
	} {
		got := CheckTarget(data, c.to, 0)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("CheckTarget(%s) = %q, want something with %q", c.to, got, c.want)
		}
	}
	if err := Ask(data, Request{To: full}); err == nil {
		t.Fatal("a move into a folder with files was asked for")
	}
}

func TestMovable(t *testing.T) {
	for from, want := range map[string]bool{
		config.DataFromDefault: true, config.DataFromPointer: true, config.DataFromInstall: true,
		config.DataFromFlag: false, config.DataFromEnv: false, config.DataFromConfig: false, config.DataFromPortable: false,
		config.DataFromContainer: false, config.DataFromService: false, "": false,
	} {
		if got, why := Movable(from); got != want || (!got && why == "") {
			t.Errorf("Movable(%q) = %v %q", from, got, why)
		}
	}
}
