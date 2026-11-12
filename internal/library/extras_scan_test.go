// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
)

func paths(root string, c []candidate) []string {
	var out []string
	for _, x := range c {
		rel, _ := filepath.Rel(root, x.path)
		out = append(out, filepath.ToSlash(rel))
	}
	sort.Strings(out)
	return out
}

// Stems and canvases put beside a song exactly as docs/LYRICS-KARAOKE.md and docs/MUSIC.md
// say are found by the walk, being sound and video files, and must not become songs.
func TestScanLeavesOutStemsAndCanvases(t *testing.T) {
	cand := func(kind string, size int64, names ...string) []candidate {
		var out []candidate
		for _, n := range names {
			out = append(out, candidate{path: filepath.FromSlash("/m/" + n), kind: kind, size: size})
		}
		return out
	}
	found := cand("audio", 1<<20,
		"A/Album/01 Song.flac",
		"A/Album/01 Song.vocals.flac",
		"A/Album/01 Song.instrumental.m4a",
		"A/Album/01 Song.stems/drums.wav",
		"A/Album/01 Song.stems/bass.wav",
		"A/Album/stems/01 Song/vocals.wav",
		"A/Album/stems/01 Song.other.mp3",
		"A/Album/02 Second.flac",
		// Songs with a dot in the name, and no song of the shorter name beside them.
		"A/Album/03 Mr. Bass.flac",
		"A/Album/04 Feat. Other.flac",
		// Folders that only look like stems: no song they'd belong to.
		"A/Album/Lost.stems/vocals.wav",
		"A/Stems/05 Drums.flac",
		"B/Drums/drums.flac",
		"B/Drums/bass.flac",
	)
	found = append(found, cand("video", 1<<20,
		"A/Album/01 Song.stem.mp4",
		"A/Album/01 Song.canvas.mp4",
		"A/Album/canvas.mp4",
		"Shows/Show/canvas.webm", // covers the episodes in the folders inside
		"Shows/Show/Season 1/S01E01.mkv",
		"Shows/Show/Season 1/S01E01.canvas.mp4",
		"Films/A Film.mkv",
		"Films/Alone.canvas.mp4",       // no "Alone" beside it: kept
		"Lonely/canvas.mp4",            // nothing else in the folder: a video called canvas
		"Films/Other/Another Film.mkv", // and its neighbour's stem-like name means nothing for a video
	)...)
	found = append(found, cand("video", mediaextras.MaxCanvasSidecar+1, "Films/Big/canvas.mp4", "Films/Big/Feature.mkv")...)
	found = append(found, cand(playlistKind, 100, "A/Album/01 Song.vocals.m3u")...)

	got := paths(filepath.FromSlash("/m"), dropTitleExtras(found))
	want := []string{
		"A/Album/01 Song.flac",
		"A/Album/01 Song.vocals.m3u",
		"A/Album/02 Second.flac",
		"A/Album/03 Mr. Bass.flac",
		"A/Album/04 Feat. Other.flac",
		"A/Album/Lost.stems/vocals.wav",
		"A/Stems/05 Drums.flac",
		"B/Drums/bass.flac",
		"B/Drums/drums.flac",
		"Films/A Film.mkv",
		"Films/Alone.canvas.mp4",
		"Films/Big/Feature.mkv",
		"Films/Big/canvas.mp4",
		"Films/Other/Another Film.mkv",
		"Lonely/canvas.mp4",
		"Shows/Show/Season 1/S01E01.mkv",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("kept:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// The whole way through a music library: stems that an earlier scan listed as songs are
// gone after the next one, and the song they belong to stays.
func TestRescanDropsStemsListedAsSongs(t *testing.T) {
	s, _, _ := scanFixture(t)
	s.Tools = &ffmpeg.Tools{Encoders: map[string]bool{}} // no FFprobe: songs are named from their files
	s.CacheDir = t.TempDir()
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "music")
	album := filepath.Join(root, "Artist", "Album")
	flac := append([]byte("fLaC\x00\x00\x00\x22"), make([]byte, 64)...)
	write := func(rel string) {
		p := filepath.Join(album, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, flac, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// First only what looks like stems, so the scan has to take them for songs.
	write("01 Song.vocals.flac")
	write("01 Song.stems/drums.flac")
	write("stems/01 Song/bass.flac")
	write("02 Mr. Bass.flac")
	lib, err := s.Store.CreateLibrary(ctx, "Music", "music", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	if got := visible(t, s, lib.ID); len(got) != 4 {
		t.Fatalf("without the song, its stems are just songs: found %d", len(got))
	}
	// The song arrives: the next scan knows them for what they are.
	write("01 Song.flac")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	got := visible(t, s, lib.ID)
	if len(got) != 2 || got["01 Song.flac"] == nil || got["02 Mr. Bass.flac"] == nil {
		names := []string{}
		for n := range got {
			names = append(names, n)
		}
		sort.Strings(names)
		t.Fatalf("after the song arrived the library lists %v", names)
	}
}
