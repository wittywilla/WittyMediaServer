// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package appcontrol

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsPE(t *testing.T) {
	dir := t.TempDir()
	text := filepath.Join(dir, "notes.dll")
	if err := os.WriteFile(text, []byte("MZ but not really a program, just words that go on for a while"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsPE(text) {
		t.Error("a text file named .dll was taken for a program")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if got := IsPE(exe); got != (filepath.Ext(exe) == ".exe") {
		t.Errorf("IsPE(this test program) = %v", got)
	}
}

// The test program was just built here, so nobody signed it: on Windows it's listed, wherever it
// is and whatever it's called.
func TestUnsigned(t *testing.T) {
	if !canVerify {
		if got, _ := Unsigned(t.TempDir()); got != nil {
			t.Errorf("listed %v where there's nothing to check", got)
		}
		t.Skip("signatures are Windows' own")
	}
	exe, _ := os.Executable()
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "helper.data"), b, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("words"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Unsigned(dir)
	if err != nil || len(got) != 1 || got[0] != "bin/helper.data" {
		t.Fatalf("Unsigned = %v, %v", got, err)
	}

	// With Smart App Control on, that's refused before anything is installed, naming the file.
	// In evaluation it blocks nothing, and neither does the check.
	t.Cleanup(func() { currentState = state })
	currentState = func() string { return On }
	var blocked *Blocked
	if err := CheckInstall(dir, "this module", "Ask its publisher."); !errors.As(err, &blocked) || blocked.Files[0] != "bin/helper.data" || blocked.Ran {
		t.Errorf("CheckInstall with it on = %v", err)
	}
	currentState = func() string { return Evaluation }
	if err := CheckInstall(dir, "this module", ""); err != nil {
		t.Errorf("refused in evaluation: %v", err)
	}
}

func TestBlockedWords(t *testing.T) {
	b := &Blocked{What: "this module", Files: []string{"a.exe", "b.dll", "c.dll", "d.dll"}, Fix: "Ask its publisher for a signed build."}
	s := b.Error()
	for _, want := range []string{"Smart App Control is on", "a.exe, b.dll, c.dll and 1 more", "in this module", "Nothing was installed", "Ask its publisher", "can't turn it back on"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q is missing %q", s, want)
		}
	}
	if strings.Contains(strings.ToLower(s), "turn it off") || strings.Contains(strings.ToLower(s), "turn smart app control off") {
		t.Errorf("suggests turning it off: %q", s)
	}
	if err := Explain(errors.New("file not found"), `C:\x\helper.exe`, ""); err == nil || errors.As(err, &b) {
		t.Errorf("an ordinary error was taken for a block: %v", err)
	}
	if Explain(nil, "x", "") != nil {
		t.Error("nil became an error")
	}
	if CheckInstall(t.TempDir(), "this module", "") != nil && State() != On {
		t.Error("refused while Smart App Control isn't on")
	}
	if w := Words(State()); State() != "" && w == "" {
		t.Errorf("no words for %q", State())
	}
}
