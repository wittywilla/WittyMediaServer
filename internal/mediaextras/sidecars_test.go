// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package mediaextras

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestStemOwners(t *testing.T) {
	a := filepath.FromSlash("/m/Artist/Album")
	own := func(dir, base string) Owner { return Owner{Dir: filepath.FromSlash(dir), Base: base} }
	cases := []struct {
		path string
		want []Owner
	}{
		// Every place docs/LYRICS-KARAOKE.md says a stem can be put.
		{"01 Song.vocals.flac", []Owner{own("/m/Artist/Album", "01 song")}},
		{"01 Song.instrumental.m4a", []Owner{own("/m/Artist/Album", "01 song")}},
		{"01 Song.no_vocals.wav", []Owner{own("/m/Artist/Album", "01 song")}},
		{"01 Song.stem.mp4", []Owner{own("/m/Artist/Album", "01 song")}},
		{"01 Song.STEM.MP4", []Owner{own("/m/Artist/Album", "01 song")}},
		{"01 Song.stems/vocals.wav", []Owner{own("/m/Artist/Album", "01 song")}},
		{"01 Song.stems/Drums.WAV", []Owner{own("/m/Artist/Album", "01 song")}},
		{"stems/01 Song/vocals.wav", []Owner{own("/m/Artist/Album", "01 song")}},
		{"stems/01 Song.bass.mp3", []Owner{own("/m/Artist/Album/stems", "01 song"), own("/m/Artist/Album", "01 song")}},
		{"stems/01 Song.stem.mp4", []Owner{own("/m/Artist/Album/stems", "01 song"), own("/m/Artist/Album", "01 song")}},
		// A dot in a song's name is fine: it has an owner by name, which the scan then looks for.
		{"Mr. Bass.flac", []Owner{own("/m/Artist/Album", "mr")}},
		// Not stems at all.
		{"01 Song.flac", nil},
		{"01 Song.live.flac", nil},
		{"01 Song.vocals.txt", nil},
		{"01 Song.multitrack.flac", nil},
		{"vocals.flac", nil},
		{".vocals.flac", nil},
		{"01 Song.stems/notes.flac", nil},
		{".stems/vocals.wav", nil},
		{"other/01 Song/vocals.wav", nil},
	}
	for _, c := range cases {
		got := StemOwners(filepath.Join(a, filepath.FromSlash(c.path)))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.path, got, c.want)
		}
	}
}

func TestCanvasOwner(t *testing.T) {
	d := filepath.FromSlash("/m/Films")
	cases := []struct {
		name string
		size int64
		want Owner
		ok   bool
	}{
		{"My Film (2001).canvas.mp4", 1 << 20, Owner{Dir: d, Base: "my film (2001)"}, true},
		{"01 Song.Canvas.WEBM", 1 << 20, Owner{Dir: d, Base: "01 song"}, true},
		{"canvas.mp4", 1 << 20, Owner{Dir: d}, true},
		{"Canvas.webm", MaxCanvasSidecar, Owner{Dir: d}, true},
		// Too big to be taken for a canvas, so it is a film called "canvas".
		{"canvas.mp4", MaxCanvasSidecar + 1, Owner{}, false},
		{"My Film.canvas.mp4", MaxCanvasSidecar + 1, Owner{}, false},
		{"canvas.mkv", 1 << 20, Owner{}, false},
		{"The Canvas (2006).mp4", 1 << 20, Owner{}, false},
		{".canvas.mp4", 1 << 20, Owner{}, false},
	}
	for _, c := range cases {
		got, ok := CanvasOwner(filepath.Join(d, c.name), c.size)
		if ok != c.ok || got != c.want {
			t.Errorf("%s (%d bytes): got %v %v, want %v %v", c.name, c.size, got, ok, c.want, c.ok)
		}
	}
}
