// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build windows

package uninstall

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The uninstaller's temp copy clears its own folder after it has exited.
func TestRemoveLaterClearsTheFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "wittymediaserver-uninstall-with a space")
	write(t, filepath.Join(dir, "Uninstall WittyMediaServer.exe"), "x")
	cmd := removeLater(dir)
	if cmd == nil {
		t.Fatal("no command")
	}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return
		}
	}
	t.Fatal("the folder is still there")
}
