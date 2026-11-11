// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package update

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzParseSums(f *testing.F) {
	f.Add([]byte("0000000000000000000000000000000000000000000000000000000000000000  a.zip\n# x\n"))
	f.Add([]byte("zz  ../../x\n"))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = ParseSums(b) })
}

// Unpacking is only ever done on a package whose signature checked out, and still must not leave its folder.
func FuzzExtract(f *testing.F) {
	f.Add([]byte("PK\x03\x04"), "a.zip")
	f.Add([]byte("\x1f\x8b\x08"), "a.tar.gz")
	f.Fuzz(func(t *testing.T, b []byte, _ string) {
		for _, name := range []string{"a.zip", "a.tar.gz"} {
			dir := t.TempDir()
			p := filepath.Join(dir, name)
			if err := os.WriteFile(p, b, 0o600); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(dir, "out")
			_ = Extract(p, dest, 1<<20)
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if e.Name() != name && e.Name() != "out" {
					t.Fatalf("%s appeared beside the folder", e.Name())
				}
			}
		}
	})
}
