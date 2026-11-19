// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The track list a rip leaves beside an album isn't one of the owner's playlists; a playlist
// that gathers songs from other folders is.
func TestAnAlbumsOwnTrackListIsNotAPlaylist(t *testing.T) {
	root := t.TempDir()
	s := testScanner(t, root)
	ctx := context.Background()
	if _, err := s.Store.DB.ExecContext(ctx, `INSERT INTO users(id, name, password_hash, is_admin, created_at) VALUES(1, 'owner', '', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	lib, err := s.Store.CreateLibrary(ctx, "Music", "music", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	song := func(dir, name string) string {
		p := filepath.Join(root, dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := s.Store.UpdateItem(ctx, &Item{LibraryID: lib.ID, Kind: "audio", Path: p, Title: name, SortTitle: name}); err != nil {
			t.Fatal(err)
		}
		return p
	}
	song("Album", "01-one.flac")
	song("Album", "02-two.flac")
	song("Other", "01-three.flac")
	write := func(rel, body string) candidate {
		p := filepath.Join(root, rel)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return candidate{path: p, size: int64(len(body))}
	}
	playlists := func() (names []string) {
		rows, err := s.Store.DB.QueryContext(ctx, `SELECT name FROM collections WHERE description LIKE 'Playlist file: %' ORDER BY name`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var n string
			_ = rows.Scan(&n)
			names = append(names, n)
		}
		return names
	}
	own := write(filepath.Join("Album", "00-artist-album.m3u"), "#EXTM3U\n01-one.flac\n02-two.flac\n")
	mixed := write("Mixed.m3u", "Album/02-two.flac\nOther/01-three.flac\n")
	for _, c := range []candidate{own, mixed} {
		if err := s.importPlaylist(ctx, lib, c, 1); err != nil {
			t.Fatal(err)
		}
	}
	if got := playlists(); len(got) != 1 || got[0] != "Mixed" {
		t.Fatalf("playlists %v, want only Mixed", got)
	}
	// One imported by an earlier version goes on the next scan.
	if _, err := s.Store.DB.ExecContext(ctx, `INSERT INTO collections(owner_id, name, description, kind, shared, created_at, updated_at) VALUES(1, '00-artist-album', ?, 'manual', 1, 1, 1)`, "Playlist file: "+own.path); err != nil {
		t.Fatal(err)
	}
	if err := s.importPlaylist(ctx, lib, own, 1); err != nil {
		t.Fatal(err)
	}
	if got := playlists(); len(got) != 1 || got[0] != "Mixed" {
		t.Fatalf("after a rescan: %v", got)
	}
}
