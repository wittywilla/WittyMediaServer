// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package mediaextras

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var (
	mp4Head  = []byte("\x00\x00\x00\x20ftypisom\x00\x00\x02\x00isomiso2mp41")
	webmHead = []byte("\x1a\x45\xdf\xa3\x9f\x42\x86\x81\x01\x42\xf7\x81\x01\x42\xf2\x81\x04\x42\xf3\x81\x08\x42\x82\x84webm")
)

func write(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMagic(t *testing.T) {
	for _, c := range []struct {
		b         []byte
		video, au string
	}{
		{mp4Head, ".mp4", ".m4a"},
		{webmHead, ".webm", ".webm"},
		{[]byte("fLaC\x00\x00\x00\x22"), "", ".flac"},
		{[]byte("RIFF\x24\x00\x00\x00WAVEfmt "), "", ".wav"},
		{[]byte("OggS\x00\x02"), "", ".ogg"},
		{[]byte("ID3\x04\x00\x00"), "", ".mp3"},
		{[]byte{0xFF, 0xFB, 0x90, 0x00}, "", ".mp3"},
		{[]byte{0xFF, 0xF1, 0x50, 0x80}, "", ".aac"},
		{[]byte("<html><script>"), "", ""},
		{[]byte("\x89PNG\r\n\x1a\n"), "", ""},
		{[]byte("\x1a\x45\xdf\xa3 matroska"), "", ""},
		{nil, "", ""},
	} {
		if got := VideoExt(c.b); got != c.video {
			t.Errorf("video %q: %q", c.b, got)
		}
		if got := AudioExt(c.b); got != c.au {
			t.Errorf("audio %q: %q", c.b, got)
		}
	}
}

func TestCanvasCandidates(t *testing.T) {
	got := CanvasCandidates("/m/Album/01 Song.flac", false, "")
	want := []string{"/m/Album/01 Song.canvas.mp4", "/m/Album/01 Song.canvas.webm", "/m/Album/.canvas/01 Song.mp4", "/m/Album/.canvas/01 Song.webm",
		"/m/Album/canvas.mp4", "/m/Album/canvas.webm", "/m/Album/.canvas/canvas.mp4", "/m/Album/.canvas/canvas.webm"}
	for i := range want {
		want[i] = filepath.FromSlash(want[i])
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%q", got)
	}
	ep := CanvasCandidates(filepath.FromSlash("/tv/Show/Season 1/E1.mkv"), false, filepath.FromSlash("/tv/Show"))
	if ep[len(ep)-4] != filepath.FromSlash("/tv/Show/canvas.mp4") {
		t.Errorf("a show's canvas: %q", ep)
	}
	disc := CanvasCandidates(filepath.FromSlash("/m/Film"), true, "")
	if disc[0] != filepath.FromSlash("/m/Film/canvas.mp4") {
		t.Errorf("disc folder: %q", disc)
	}
}

func TestFindInMetaFolder(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "Movies", "My Film (2001) [item-12]-canvas.mp4"), mp4Head)
	write(t, filepath.Join(root, "Movies", "Other [item-121]-canvas.mp4"), mp4Head)
	write(t, filepath.Join(root, "Shows", "A Show [show-2]", "S01E01 [item-31]-canvas.webm"), webmHead)
	if p := FindInMetaFolder(root, 12, "canvas", CanvasExts); filepath.Base(p) != "My Film (2001) [item-12]-canvas.mp4" {
		t.Errorf("12: %q", p)
	}
	if p := FindInMetaFolder(root, 31, "canvas", CanvasExts); !strings.HasSuffix(p, "[item-31]-canvas.webm") {
		t.Errorf("31: %q", p)
	}
	if p := FindInMetaFolder(root, 1, "canvas", CanvasExts); p != "" {
		t.Errorf("1: %q", p)
	}
	if runtime.GOOS != "windows" {
		outside := filepath.Join(t.TempDir(), "secret.mp4")
		write(t, outside, mp4Head)
		if err := os.Symlink(outside, filepath.Join(root, "Movies", "Linked [item-77]-canvas.mp4")); err != nil {
			t.Fatal(err)
		}
		if p := FindInMetaFolder(root, 77, "canvas", CanvasExts); p != "" {
			t.Errorf("a link was followed: %q", p)
		}
	}
}

func TestFindStems(t *testing.T) {
	dir := t.TempDir()
	song := filepath.Join(dir, "Album", "01 Song.flac")
	write(t, song, []byte("fLaC"))
	write(t, filepath.Join(dir, "Album", "01 Song.vocals.flac"), []byte("fLaC vocals"))
	write(t, filepath.Join(dir, "Album", "01 Song.accompaniment.m4a"), mp4Head)
	write(t, filepath.Join(dir, "Album", "01 Song.stems", "drums.wav"), []byte("RIFF....WAVE"))
	write(t, filepath.Join(dir, "Album", "01 Song.stems", "vocals.wav"), []byte("RIFF....WAVE")) // the one beside the song came first
	write(t, filepath.Join(dir, "Album", "stems", "01 Song", "bass.mp3"), []byte("ID3"))
	write(t, filepath.Join(dir, "Album", "stems", "01 Song.other.ogg"), []byte("OggS"))
	write(t, filepath.Join(dir, "Album", "01 Song.stem.mp4"), mp4Head)
	write(t, filepath.Join(dir, "Album", "02 Other.vocals.flac"), []byte("fLaC"))
	write(t, filepath.Join(dir, "Album", "01 Song.lyrics.txt"), []byte("words"))
	write(t, filepath.Join(dir, "Album", "01 Song.unknown.flac"), []byte("fLaC"))

	got := FindStems(song, "")
	var names []string
	for _, s := range got {
		names = append(names, s.Name+"="+filepath.Base(s.Path))
	}
	want := "vocals=01 Song.vocals.flac instrumental=01 Song.accompaniment.m4a drums=drums.wav bass=bass.mp3 other=01 Song.other.ogg multitrack=01 Song.stem.mp4"
	if strings.Join(names, " ") != want {
		t.Errorf("got  %s\nwant %s", strings.Join(names, " "), want)
	}
	for _, s := range got {
		if s.Name == Multitrack && (!s.Multitrack || s.MIME != "audio/mp4") {
			t.Errorf("multitrack %+v", s)
		}
		if s.Source != "sidecar" || s.Uploaded {
			t.Errorf("%+v", s)
		}
	}

	// An upload comes first, and Demucs' no_vocals is the instrumental.
	up := t.TempDir()
	write(t, filepath.Join(up, "stems", "vocals.wav"), []byte("RIFF....WAVE"))
	write(t, filepath.Join(up, "stems", "multitrack.stem.mp4"), mp4Head)
	got = FindStems(song, up)
	if got[0].Name != "vocals" || !got[0].Uploaded || got[0].Source != "upload" {
		t.Errorf("upload first: %+v", got[0])
	}
	if got[len(got)-1].Name != Multitrack || !got[len(got)-1].Uploaded {
		t.Errorf("uploaded multitrack: %+v", got[len(got)-1])
	}
	demucs := filepath.Join(t.TempDir(), "s.mp3")
	write(t, demucs, []byte("ID3"))
	write(t, filepath.Join(filepath.Dir(demucs), "s.stems", "no_vocals.wav"), []byte("RIFF....WAVE"))
	if g := FindStems(demucs, ""); len(g) != 1 || g[0].Name != "instrumental" {
		t.Errorf("demucs: %+v", g)
	}

	if runtime.GOOS != "windows" {
		outside := filepath.Join(t.TempDir(), "secret.flac")
		write(t, outside, []byte("fLaC secret"))
		lone := filepath.Join(t.TempDir(), "x.flac")
		write(t, lone, []byte("fLaC"))
		if err := os.Symlink(outside, filepath.Join(filepath.Dir(lone), "x.vocals.flac")); err != nil {
			t.Fatal(err)
		}
		if g := FindStems(lone, ""); len(g) != 0 {
			t.Errorf("a link was followed: %+v", g)
		}
	}
}

func TestStemName(t *testing.T) {
	for in, want := range map[string]string{
		"vocals": "vocals", "Vocals": "vocals", "accompaniment": "instrumental", "instrumental": "instrumental", "no_vocals": "instrumental",
		"drums": "drums", "bass": "bass", "other": "other", "multitrack": "multitrack",
		"../vocals": "", "vocals/../../etc": "", "..": "", "": "", "piano": "", "vocals.flac": "", "%2e%2e": "",
	} {
		if got := StemName(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestSaveUpload(t *testing.T) {
	root := t.TempDir()
	body := append(append([]byte{}, mp4Head...), bytes.Repeat([]byte{1}, 1000)...)
	dst, n, err := SaveUpload(root, "12-abc/canvas", bytes.NewReader(body), 4096, VideoExt)
	if err != nil || n != int64(len(body)) || filepath.Base(dst) != "canvas.mp4" {
		t.Fatalf("%v %d %s", err, n, dst)
	}
	if b, _ := os.ReadFile(dst); !bytes.Equal(b, body) {
		t.Error("not the same file")
	}
	if p := FirstPlain(root, "12-abc/canvas", CanvasExts); p != dst {
		t.Errorf("first plain %q", p)
	}

	// Wrong kind: nothing is kept.
	if _, _, err := SaveUpload(root, "13-abc/canvas", strings.NewReader("<html><script>alert(1)</script>"), 4096, VideoExt); !errors.Is(err, ErrBadType) {
		t.Errorf("html: %v", err)
	}
	// Too big: nothing is kept, and the old one stays.
	big := append(append([]byte{}, mp4Head...), bytes.Repeat([]byte{1}, 5000)...)
	if _, _, err := SaveUpload(root, "12-abc/canvas", bytes.NewReader(big), 4096, VideoExt); !errors.Is(err, ErrTooBig) {
		t.Errorf("big: %v", err)
	}
	if b, _ := os.ReadFile(dst); !bytes.Equal(b, body) {
		t.Error("a refused upload replaced the old one")
	}
	if _, err := os.Stat(filepath.Join(root, "13-abc")); err == nil {
		entries, _ := os.ReadDir(filepath.Join(root, "13-abc"))
		if len(entries) != 0 {
			t.Errorf("left behind: %v", entries)
		}
	}
	// Names that lead elsewhere.
	for _, rel := range []string{"../x", "a/../../x", "/abs", "a//b", "a/./b", ""} {
		if _, _, err := SaveUpload(root, rel, bytes.NewReader(body), 4096, VideoExt); err == nil {
			t.Errorf("%q was taken", rel)
		}
	}
	// A link where a folder goes isn't written through.
	if runtime.GOOS != "windows" {
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "evil")); err != nil {
			t.Fatal(err)
		}
		if _, _, err := SaveUpload(root, "evil/canvas", bytes.NewReader(body), 4096, VideoExt); err == nil {
			t.Error("wrote through a link")
		}
		if entries, _ := os.ReadDir(outside); len(entries) != 0 {
			t.Errorf("outside: %v", entries)
		}
	}

	found, err := RemoveMatching(root, "12-abc/canvas", CanvasExts)
	if err != nil || !found {
		t.Errorf("remove: %v %v", found, err)
	}
	if found, _ := RemoveMatching(root, "12-abc/canvas", CanvasExts); found {
		t.Error("removed twice")
	}
	if _, err := RemoveMatching(root, "../x", CanvasExts); err == nil {
		t.Error("removed outside")
	}
}

func TestKey(t *testing.T) {
	a, b := Key(5, "/m/a.mp4"), Key(5, "/m/b.mp4")
	if a == b || !strings.HasPrefix(a, "5-") {
		t.Errorf("%s %s", a, b)
	}
	if Key(5, "/m/a.mp4") != a {
		t.Error("not stable")
	}
}
