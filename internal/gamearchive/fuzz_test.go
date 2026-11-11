// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gamearchive

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzCleanName(f *testing.F) {
	for _, s := range []string{"game.bin", "../x", "..\\x", "/abs", "C:x", "a/../../b", "CON", "a\x00b", "x:stream", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		out, err := cleanName(s)
		if err == nil && !filepath.IsLocal(filepath.FromSlash(out)) {
			t.Fatalf("cleanName(%q) = %q", s, out)
		}
	})
}

// A game's own list of its parts, naming whatever it likes.
func FuzzCheckReferences(f *testing.F) {
	f.Add([]byte(`FILE "game.bin" BINARY`+"\n"+`  TRACK 01 MODE1/2352`), "game.cue")
	f.Add([]byte(`FILE "../../etc/passwd" BINARY`), "game.cue")
	f.Add([]byte("1 0 4 2352 ..\\x.bin 0\n"), "game.gdi")
	f.Add([]byte("a.cue\n../b.cue\n"), "game.m3u")
	f.Fuzz(func(t *testing.T, b []byte, entry string) {
		if !filepath.IsLocal(entry) || filepath.Base(entry) != entry || entry == "." {
			return
		}
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, entry), b, 0o600); err != nil {
			return // a name this system won't have
		}
		_ = checkReferences(dir, entry, []string{entry, "game.bin"})
	})
}

func FuzzListFile(f *testing.F) {
	f.Add([]byte("PK\x03\x04"))
	f.Add([]byte("PK\x05\x06\x00\x00\x00\x00\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\x00\x00"))
	f.Add([]byte("Rar!\x1a\x07\x00"))
	f.Add([]byte("7z\xbc\xaf\x27\x1c"))
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, name := range []string{"a.zip", "a.7z", "a.rar"} {
			p := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(p, b, 0o600); err != nil {
				t.Fatal(err)
			}
			_, _ = listFile(p, 1<<20)
		}
	})
}
