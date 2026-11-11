// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gameid

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run by hand with WMS_GAME_FOLDER set to a real folder of games, to see what each
// loose file proves. It only reads. It names files, so its output stays on this machine.
func TestRealFolder(t *testing.T) {
	root := os.Getenv("WMS_GAME_FOLDER")
	if root == "" {
		t.Skip("set WMS_GAME_FOLDER to run this")
	}
	skip := map[string]bool{".jpg": true, ".nfo": true, ".png": true}
	tally := map[string]int{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || skip[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		f, err := SniffFile(p)
		rel, _ := filepath.Rel(root, p)
		line := fmt.Sprintf("%-6s %-7s %v serial=%q title=%q region=%q enc=%v new=%v | %s | %s", f.Console, f.Kind, f.Proven(), f.Serial, f.Title, f.Region, f.Encrypted, f.NewOnly, rel, f.Proof)
		if err != nil {
			line += " ERROR " + err.Error()
		}
		k := filepath.Dir(rel) + " -> " + f.Console + "/" + string(f.Kind)
		tally[k]++
		if tally[k] <= 6 {
			t.Log(line)
		}
		return nil
	})
	for k, n := range tally {
		t.Logf("%6d %s", n, k)
	}
}
