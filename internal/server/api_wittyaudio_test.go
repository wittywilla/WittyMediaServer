// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/wittyinfo"
)

// A song's .wittyaudio gives its lyrics (in the viewer's language) and loudness, and is
// edited, gathered and spread like a film's .wittyinfo.
func TestWittyaudioSong(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	ctx := context.Background()
	stem := strings.TrimSuffix(f.song.Path, filepath.Ext(f.song.Path))
	dir := filepath.Dir(f.song.Path)

	// Gathering: the .lrc, the album's cover and album.nfo.
	put(t, stem+".lrc", []byte("[00:12.50]First line\n[00:16.00]Second line\n"))
	put(t, filepath.Join(dir, "cover.jpg"), jpegBytes(t))
	put(t, filepath.Join(dir, "album.nfo"), []byte(`<album><title>An Album</title><artist>Someone</artist><year>2024</year><genre>Pop</genre></album>`))
	c := decode[map[string]any](t, ts.do(t, "GET", "/api/wittyinfo/convert/counts", f.admin, nil))["counts"].(map[string]any)
	if c["songs"].(float64) != 1 || c["songsWithLyrics"].(float64) != 1 || c["albumsWithArt"].(float64) != 1 {
		t.Fatalf("counts: %v", c)
	}
	rep, err := ts.Scanner.GatherInfo(ctx, []int64{f.song.LibraryID}, library.ConvertOptions{Beside: true, Format: wittyinfo.FormatAudio}, nil)
	if err != nil || rep.Converted != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	song, err := os.ReadFile(stem + ".wittyaudio")
	if err != nil {
		t.Fatal("no .wittyaudio beside the song:", err)
	}
	x, err := wittyinfo.Parse(song)
	if err != nil || !x.IsAudio() || len(x.Audio.Lyrics.Lines) != 2 || x.Audio.Lyrics.Lines[1].Start != 16 {
		t.Fatalf("%v %+v", err, x)
	}
	album, err := os.ReadFile(filepath.Join(dir, "album.wittyaudio"))
	if err != nil {
		t.Fatal("no album.wittyaudio:", err)
	}
	a, _ := wittyinfo.Parse(album)
	if a.ScopeOr() != wittyinfo.ScopeAlbum || a.Metadata.Title != "An Album" || a.Audio.AlbumArtist != "Someone" || a.Artwork["poster"] == "" {
		t.Fatalf("album: %+v %+v", a.Metadata, a.Audio)
	}
	if it, _ := ts.Store.Item(ctx, f.song.ID); it.Album != "An Album" || it.AlbumArtist != "Someone" || it.Title != "Song" {
		t.Errorf("the album's details are the song's, its title its own: %q %q %q", it.Album, it.AlbumArtist, it.Title)
	}

	// Editing: a translation and loudness, through the same API as a film's.
	x.Language = "en"
	x.Audio.Loudness = &wittyinfo.Gain{TrackGain: ptr(-7.25)}
	x.Translations = map[string]*wittyinfo.Translation{"es": {Lyrics: &wittyinfo.Lyrics{Lines: []wittyinfo.LyricLine{{Start: 12.5, Text: "Primera línea"}, {Start: 16, Text: "Segunda línea"}}}}}
	man, _ := x.Manifest()
	w := ts.do(t, "PUT", f.url("/api/wittyinfo/item/%d", f.song), f.admin, map[string]any{"info": jsonRaw(man)})
	ans := decode[infoAnswer](t, w)
	if w.Code != 200 || ans.Info.Format != wittyinfo.FormatAudio || ans.Info.Scope != wittyinfo.ScopeSong {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	ly := decode[lyricsReply](t, ts.do(t, "GET", f.url("/api/music/lyrics/%d", f.song), f.adult, nil, "Accept-Language", "es-MX"))
	if len(ly.Lines) != 2 || ly.Lines[0].Text != "Primera línea" || ly.Lines[1].T != 16000 {
		t.Errorf("lyrics in Spanish: %+v", ly.Lines)
	}
	ly = decode[lyricsReply](t, ts.do(t, "GET", f.url("/api/music/lyrics/%d", f.song), f.adult, nil))
	if len(ly.Lines) != 2 || ly.Lines[0].Text != "First line" {
		t.Errorf("lyrics: %+v", ly.Lines)
	}
	if l := decode[loudness](t, ts.do(t, "GET", f.url("/api/music/loudness/%d", f.song), f.adult, nil)); l.TrackDB == nil || *l.TrackDB != -7.25 {
		t.Errorf("loudness: %+v", l)
	}
	// The album's own file, by a song of it.
	if a := decode[infoAnswer](t, ts.do(t, "GET", f.url("/api/wittyinfo/album/%d", f.song), f.admin, nil)); a.Info == nil || a.Info.ScopeOr() != wittyinfo.ScopeAlbum {
		t.Errorf("album answer: %+v", a.Info)
	}

	// And back out: the .lrc, the cover and album.nfo.
	os.Remove(stem + ".lrc")
	os.Remove(filepath.Join(dir, "album.nfo"))
	if err := ts.DB.SetSetting(ctx, "saveMetadata", "1"); err != nil {
		t.Fatal(err)
	}
	rep, err = ts.Scanner.SpreadInfo(ctx, []int64{f.song.LibraryID}, library.ConvertOptions{Format: wittyinfo.FormatAudio}, nil)
	if err != nil || rep.Converted != 1 {
		t.Fatalf("spread: %+v %v", rep, err)
	}
	if b, _ := os.ReadFile(stem + ".lrc"); !strings.Contains(string(b), "[00:16.00]Second line") {
		t.Errorf("lrc: %s", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "album.nfo")); !strings.Contains(string(b), "<title>An Album</title>") {
		t.Errorf("album.nfo: %s", b)
	}
}

// jsonRaw sends bytes that are JSON already as they are.
func jsonRaw(b []byte) json.RawMessage { return json.RawMessage(b) }
