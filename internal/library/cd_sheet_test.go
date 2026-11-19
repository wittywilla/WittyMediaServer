// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	wittycd "github.com/wittywilla/wittymediaserver/internal/cd"
)

// A rip's sheet beside the songs it was encoded to and renamed as is passed over in a music
// library, with no failure; on its own, with no songs beside it, it still fails as before.
func TestARenamedRipsSheetIsPassedOverInMusic(t *testing.T) {
	// As WittyCD answers a sheet whose file isn't beside it.
	was := wittycd.Open
	t.Cleanup(func() { wittycd.Open = was })
	wittycd.Open = func(string) (*wittycd.Disc, error) {
		return nil, fmt.Errorf("the file \"01. Song.wav\" this image needs isn't in its folder: %w", os.ErrNotExist)
	}
	root := t.TempDir()
	s := testScanner(t, root)
	ctx := context.Background()
	lib, err := s.Store.CreateLibrary(ctx, "Music", "music", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	sheet := "PERFORMER \"Someone\"\nTITLE \"Album\"\nFILE \"01. Song.wav\" WAVE\n  TRACK 01 AUDIO\n    INDEX 01 00:00:00\n"
	write := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	withSongs := filepath.Join(root, "Album", "Album (CD01).cue")
	write(withSongs, sheet)
	write(filepath.Join(root, "Album", "101 Song.flac"), "fLaC")
	if err := s.indexCD(ctx, lib, candidate{path: withSongs, size: int64(len(sheet)), mtime: 1}); err != nil {
		t.Errorf("beside its songs: %v", err)
	}
	alone := filepath.Join(root, "Lonely", "Lonely.cue")
	write(alone, sheet)
	if err := s.indexCD(ctx, lib, candidate{path: alone, size: int64(len(sheet)), mtime: 1}); err == nil {
		t.Error("a sheet with nothing beside it no longer says its file is missing")
	}
}
