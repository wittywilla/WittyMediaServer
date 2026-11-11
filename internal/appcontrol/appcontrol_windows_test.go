// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build windows

package appcontrol

import (
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPolicyBlock(t *testing.T) {
	blocked := &exec.Error{Name: `C:\data\modules\wittydvd\bin\wittydvd.exe`, Err: windows.Errno(4551)}
	err := Explain(fmt.Errorf("starting the helper: %w", blocked), blocked.Name, "Ask its publisher for a signed build.")
	var b *Blocked
	if !errors.As(err, &b) || !b.Ran || b.Files[0] != "wittydvd.exe" {
		t.Fatalf("a 4551 wasn't named as a block: %v", err)
	}
	if policyBlock(windows.ERROR_FILE_NOT_FOUND) || policyBlock(errors.New("4551")) {
		t.Error("something else taken for a block")
	}
}

// This computer's state reads as one of the three, or nothing on a Windows without it.
func TestState(t *testing.T) {
	switch s := State(); s {
	case "", Off, On, Evaluation:
		t.Logf("Smart App Control here: %q", s)
	default:
		t.Errorf("State() = %q", s)
	}
}

// A program signed by its publisher passes. Git for Windows' is, here and on GitHub's runners.
// (Windows' own files are mostly signed through catalogs instead, which this check doesn't
// read: nothing of ours is.)
func TestSignedPasses(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git here to try")
	}
	if !IsPE(git) {
		t.Skipf("%s isn't a Windows program", git)
	}
	if err := verify(git); err != nil {
		t.Skipf("%s isn't signed in this installation: %v", git, err)
	}
	if got, err := Unsigned(git); err != nil || len(got) != 0 {
		t.Errorf("Unsigned(%s) = %v, %v", git, got, err)
	}
}
