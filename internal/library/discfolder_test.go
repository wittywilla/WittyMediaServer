// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var mkvHead = append([]byte("\x1A\x45\xDF\xA3\x01\x42\x82\x88matroska"), make([]byte, 1024)...)

// found lists what a walk of root finds, as "kind name" with names from root.
func found(t *testing.T, root string) []string {
	t.Helper()
	c, err := discoverClips(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, x := range c {
		rel, _ := filepath.Rel(filepath.Dir(root), x.path)
		out = append(out, x.kind+" "+filepath.ToSlash(rel))
	}
	slices.Sort(out)
	return out
}

func TestDiscFoldersInAnyCase(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Discs")
	write(t, filepath.Join(dir, "Upper (2001)", "VIDEO_TS", "VIDEO_TS.IFO"), []byte("DVDVIDEO-VMG"))
	write(t, filepath.Join(dir, "Lower", "video_ts", "video_ts.ifo"), []byte("DVDVIDEO-VMG"))
	write(t, filepath.Join(dir, "Lower", "Lower.mkv"), mkvHead) // beside the disc: part of it, not a film of its own
	write(t, filepath.Join(dir, "Bluray", "bdmv", "INDEX.BDMV"), []byte("INDX0200"))
	write(t, filepath.Join(dir, "Bluray", "bdmv", "STREAM", "00000.m2ts"), mkvHead)
	write(t, filepath.Join(dir, "Card", "PRIVATE", "AVCHD", "BDMV", "INDEX.BDM"), []byte("INDX0100"))
	write(t, filepath.Join(dir, "Recording", "BDAV", "CLIPINF", "00001.clpi"), []byte("HDMV0200"))
	write(t, filepath.Join(dir, "Recording", "BDAV", "STREAM", "00001.m2ts"), mkvHead)
	write(t, filepath.Join(dir, "Not a disc", "VIDEO_TS", "notes.txt"), []byte("no IFO"))
	write(t, filepath.Join(dir, "Not a recording", "BDAV", "film.mkv"), mkvHead)
	got := found(t, dir)
	want := []string{
		"bluray-folder Discs/Bluray",
		"bluray-folder Discs/Card",
		"bluray-folder Discs/Recording",
		"dvd-folder Discs/Lower",
		"dvd-folder Discs/Upper (2001)",
		"video Discs/Not a recording/BDAV/film.mkv",
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestLibraryRootedAtADiscsStructure(t *testing.T) {
	base := t.TempDir()
	dvd := filepath.Join(base, "Film (1999)", "VIDEO_TS")
	write(t, filepath.Join(dvd, "VIDEO_TS.IFO"), []byte("DVDVIDEO-VMG"))
	write(t, filepath.Join(dvd, "VTS_01_1.VOB"), []byte{0, 0, 1, 0xBA})
	if got := found(t, dvd); !slices.Equal(got, []string{"dvd-folder VIDEO_TS"}) {
		t.Errorf("a library that is a VIDEO_TS folder found %v", got)
	}
	bd := filepath.Join(base, "Other (2010)", "bdmv")
	write(t, filepath.Join(bd, "index.bdmv"), []byte("INDX0200"))
	write(t, filepath.Join(bd, "STREAM", "00000.m2ts"), mkvHead)
	if got := found(t, bd); !slices.Equal(got, []string{"bluray-folder bdmv"}) {
		t.Errorf("a library that is a BDMV folder found %v", got)
	}
	// Such a disc is named after the folder it's in, not "VIDEO_TS".
	if n := discFolderName(dvd); n != "Film (1999)" {
		t.Errorf("named %q", n)
	}
	if n := discFolderName(filepath.Dir(dvd)); n != "Film (1999)" {
		t.Errorf("a disc folder keeps its own name, got %q", n)
	}
}

func TestDiscInsideAFolderBeingAdopted(t *testing.T) {
	base := t.TempDir()
	for name, want := range map[string]string{
		"a/VIDEO_TS/VIDEO_TS.IFO": "dvd-folder",
		"b/Video_TS/Video_TS.ifo": "dvd-folder",
		"c/BDMV/index.bdmv":       "bluray-folder",
		"d/Bdmv/Index.Bdmv":       "bluray-folder",
		"e/BDAV/info.bdav":        "bluray-folder",
		"f/VIDEO_TS/readme.txt":   "",
		"g/film.mkv":              "",
	} {
		p := filepath.Join(base, filepath.FromSlash(name))
		write(t, p, []byte("x"))
		top := filepath.Join(base, strings.SplitN(name, "/", 2)[0])
		if got := discInside(top); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
	if err := os.MkdirAll(filepath.Join(base, "h", "BDAV", "CLIPINF"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := discInside(filepath.Join(base, "h")); got != "bluray-folder" {
		t.Errorf("a BDAV recording with its clip information is a Blu-ray, got %q", got)
	}
}

func TestVideosCopiedFromADisc(t *testing.T) {
	for _, c := range []struct {
		path, codec string
		height      int
		want        string
	}{
		{"/m/Film/VTS_01_1.VOB", "mpeg2video", 576, "dvd"},
		{"/m/Film/feature.evo", "vc1", 1080, "dvd"},
		{"/m/rec/2024-01-01.vro", "mpeg2video", 576, "dvd"},
		{"/m/Film/BDMV/STREAM/00800.m2ts", "h264", 1080, "bluray"},
		{"/m/cam/00001.MTS", "h264", 1080, "bluray"},
		{"/m/cam/clip.m2t", "mpeg2video", 1080, "bluray"},
		{"/m/Film.2010.1080p.BluRay.x264.mkv", "h264", 1080, "bluray"},
		{"/m/Film.2010.BDRip.mkv", "h264", 720, "bluray"},
		{"/m/Film 2010 BRRip.mp4", "h264", 720, "bluray"},
		{"/m/Film.2010.2160p.BD-Remux.mkv", "hevc", 2160, "bluray"},
		{"/m/Film (2010) [Blu-ray]/Film (2010).mkv", "h264", 1080, "bluray"},
		{"/m/Film.2003.DVDRip.XviD.avi", "mpeg4", 480, "dvd"},
		{"/m/Film (2003) DVD/Film.mkv", "mpeg2video", 576, "dvd"},
		{"/m/Film.2003.DVD9.mkv", "mpeg2video", 576, "dvd"},
		{"/m/FILM_LABEL/title_t00.mkv", "mpeg2video", 480, "dvd"},
		{"/m/FILM_LABEL/FILM_LABEL_t03.mkv", "vc1", 1080, "bluray"},
		{"/m/Film.2006.HD-DVD.mkv", "vc1", 1080, ""},
		{"/m/Film (2010).mkv", "h264", 1080, ""},
		{"/m/DVDs/Film (2010).mkv", "h264", 1080, ""},
		{"/m/Show/Show.S01E01.WEB-DL.mkv", "h264", 1080, ""},
		{"/m/recording.ts", "mpeg2video", 576, ""},
	} {
		if got := discSourceOf(c.path, c.codec, c.height); got != c.want {
			t.Errorf("%s: %q, want %q", c.path, got, c.want)
		}
	}
}

func TestMacSidecarsBesideDiscsAreNoVideos(t *testing.T) {
	dir := t.TempDir()
	// macOS writes "._" files beside what it copies to a drive it didn't format.
	write(t, filepath.Join(dir, "._Film.ISO"), append([]byte("\x00\x05\x16\x07"), make([]byte, 4092)...))
	write(t, filepath.Join(dir, "._Film.mkv"), mkvHead)
	write(t, filepath.Join(dir, "Other", "VIDEO_TS", "VIDEO_TS.IFO"), []byte("DVDVIDEO-VMG"))
	c, err := discoverVideos(dir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 1 || c[0].kind != "dvd-folder" {
		t.Errorf("found %+v, want only the DVD folder", c)
	}
}
