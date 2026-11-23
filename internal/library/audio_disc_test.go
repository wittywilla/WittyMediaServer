// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package library

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscFromName(t *testing.T) {
	one, two, d201 := 1, 2, 201
	cases := []struct {
		base, dir    string
		track        *int
		disc, track2 int
	}{
		{"201-lady_gaga_feat._colby_odonis-just_dance", filepath.Join("m", "Fame Monster"), &one, 2, 1},
		{"101-lady_gaga-bad_romance", filepath.Join("m", "Fame Monster"), &one, 1, 1},
		{"201-song", filepath.Join("m", "x"), &d201, 2, 1},
		{"2-01 Song", filepath.Join("m", "x"), nil, 2, 1},
		{"1.02 Song", filepath.Join("m", "x"), &two, 1, 2},
		{"01 - Song", filepath.Join("m", "Album CD2"), &one, 2, 1},
		{"05 Song", filepath.Join("m", "Disc 3"), nil, 3, 0},
	}
	for _, c := range cases {
		d, tr := discFromName(c.base, c.dir, c.track)
		if d == nil || *d != c.disc {
			t.Errorf("%s: disc %v, want %d", c.base, d, c.disc)
			continue
		}
		if c.track2 > 0 && (tr == nil || *tr != c.track2) {
			t.Errorf("%s: track %v, want %d", c.base, tr, c.track2)
		}
	}
	// Not discs: no track tag to agree with a 3-digit name, a tag that disagrees, a plain album.
	for _, c := range []struct {
		base  string
		track *int
	}{{"101 Dalmatians", nil}, {"205-song", &two}, {"01 - Song", &one}, {"12 Song", nil}} {
		if d, _ := discFromName(c.base, filepath.Join("m", "Album"), c.track); d != nil {
			t.Errorf("%s: disc %d, want none", c.base, *d)
		}
	}
}

// picture is a JPEG of w by w pixels; cut gives only its first half, as a broken download does.
func picture(t *testing.T, w int, cut bool) []byte {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, w)), nil); err != nil {
		t.Fatal(err)
	}
	if cut {
		return buf.Bytes()[:buf.Len()/2]
	}
	return buf.Bytes()
}

func TestAlbumCoverTakesReleaseNames(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("000-lady_gaga-the_fame_monster-(2725296)-2cd-flac-2009-.jpg", picture(t, 60, false))
	write("00-lady_gaga-the_fame_monster-back.jpg", picture(t, 90, false))
	write("._000-lady_gaga-the_fame_monster.jpg", picture(t, 200, false))
	write("000-lady_gaga-the_fame_monster.nfo", make([]byte, 5000))
	write("00-lady_gaga-the_fame_monster-scan.jpg", picture(t, 400, true)) // larger, but cut short
	if got := filepath.Base(albumCover(dir)); got != "000-lady_gaga-the_fame_monster-(2725296)-2cd-flac-2009-.jpg" {
		t.Fatalf("cover %q", got)
	}
	// A usual name wins over a release's name, however small.
	write("folder.jpg", make([]byte, 10))
	if got := filepath.Base(albumCover(dir)); got != "folder.jpg" {
		t.Fatalf("cover %q, want folder.jpg", got)
	}
	other := t.TempDir()
	for _, n := range []string{"Album (Front).jpg", "Album (Back).jpg", "scan-cd1.jpg"} {
		if err := os.WriteFile(filepath.Join(other, n), picture(t, 20, false), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := filepath.Base(albumCover(other)); got != "Album (Front).jpg" {
		t.Fatalf("cover %q, want the front", got)
	}
	if got := albumCover(t.TempDir()); got != "" {
		t.Fatalf("an empty folder has cover %q", got)
	}
	// Only a cut-short release picture: none, so the songs' own art is used.
	broken := t.TempDir()
	if err := os.WriteFile(filepath.Join(broken, "00-artist-album.jpg"), picture(t, 400, true), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := albumCover(broken); got != "" {
		t.Fatalf("a cut-short picture was taken as the cover: %q", got)
	}
}

// A picture whose header reads but whose data Go can't decode (some release groups' JPEGs, which
// browsers show) is served whole: the file must be back at its start for whoever sends it.
func TestSmallCopyOfRewindsWhenItCannotShrink(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 400, 400)), nil); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "00-artist-album.jpg")
	if err := os.WriteFile(p, buf.Bytes()[:buf.Len()/2], 0o644); err != nil { // cut short: the header reads, the picture doesn't
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := &Scanner{ImagesDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if got := s.SmallCopyOf(context.Background(), f, p, 160); got != p {
		t.Fatalf("SmallCopyOf = %q, want the original", got)
	}
	if at, _ := f.Seek(0, io.SeekCurrent); at != 0 {
		t.Fatalf("left at %d, want 0", at)
	}
}
