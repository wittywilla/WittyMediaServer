// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package mediaextras

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEvictKaraokeLeastRecentlyUsedFirst(t *testing.T) {
	root := t.TempDir()
	write := func(rel string, n int, age time.Duration) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
			t.Fatal(err)
		}
		when := time.Now().Add(-age)
		_ = os.Chtimes(p, when, when)
	}
	exists := func(rel string) bool { _, err := os.Stat(filepath.Join(root, rel)); return err == nil }
	// old: an approximate pair made long ago. sung: made long ago but used today. new: just made.
	write("old/approx/instrumental.m4a", 100, 72*time.Hour)
	write("old/approx/full.m4a", 100, 72*time.Hour)
	write("sung/approx/instrumental.m4a", 100, 96*time.Hour)
	write("sung/.karaoke-used", 0, time.Minute)
	write("new/stems/vocals.m4a", 100, 0)
	write("new/stems/drums.m4a", 100, 0)
	write("new/stems/.separated", 0, 0)
	if err := os.WriteFile(filepath.Join(root, "new/stems/.separated"), []byte("vocals\ndrums\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// An administrator's upload, and a canvas: never counted, never removed.
	write("mine/stems/vocals.flac", 5000, 200*time.Hour)
	write("mine/canvas.mp4", 5000, 200*time.Hour)
	write("new/stems/bass.flac", 5000, 200*time.Hour) // uploaded beside separated parts

	if b, n := KaraokeUsage(root); b != 500 || n != 3 {
		t.Fatalf("usage: %d bytes for %d songs", b, n)
	}
	if n, freed := EvictKaraoke(root, 500, ""); n != 0 || freed != 0 {
		t.Fatalf("under the cap, removed %d", n)
	}
	// 500 kept, room for 350: the least recently used goes, and that is enough.
	if n, freed := EvictKaraoke(root, 350, "new"); n != 1 || freed != 200 {
		t.Fatalf("removed %d songs, %d bytes", n, freed)
	}
	if exists("old/approx/full.m4a") || exists("old") || !exists("sung/approx/instrumental.m4a") || !exists("new/stems/vocals.m4a") {
		t.Fatal("the wrong one went")
	}
	// Room for nothing: everything made goes except the one being kept, and uploads stay.
	if n, _ := EvictKaraoke(root, 0, "new"); n != 1 {
		t.Fatalf("removed %d", n)
	}
	if exists("sung/approx") || !exists("new/stems/vocals.m4a") || !exists("mine/stems/vocals.flac") || !exists("mine/canvas.mp4") {
		t.Fatal("kept or removed the wrong files")
	}
	if n, _ := EvictKaraoke(root, 0, ""); n != 1 || exists("new/stems/vocals.m4a") || exists("new/stems/.separated") || !exists("new/stems/bass.flac") {
		t.Fatal("the separated parts should go and the upload beside them stay")
	}
}

func TestTouchKaraoke(t *testing.T) {
	dir := t.TempDir()
	TouchKaraoke(filepath.Join(dir, "missing")) // no folder: nothing is made
	if _, err := os.Stat(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("made a folder")
	}
	TouchKaraoke(dir)
	p := filepath.Join(dir, usedMark)
	old := time.Now().Add(-3 * time.Hour)
	_ = os.Chtimes(p, old, old)
	TouchKaraoke(dir)
	if fi, err := os.Stat(p); err != nil || time.Since(fi.ModTime()) > time.Minute {
		t.Fatalf("not touched: %v", err)
	}
}

func TestSeparatorPart(t *testing.T) {
	for in, want := range map[string]string{
		"vocals": "vocals", "no_vocals": "instrumental", "Song_(Vocals)_htdemucs": "vocals",
		"My Song_(Instrumental)_UVR-MDX": "instrumental", "Song_(Guitar)_x": "", "readme": "", "a_(b": "",
	} {
		if got := separatorPart(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
