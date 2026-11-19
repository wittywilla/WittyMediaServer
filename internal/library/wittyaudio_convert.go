// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/lyrics"
	"github.com/wittywilla/wittymediaserver/pkg/wittyinfo"
)

// Converting songs to .wittyaudio files and back. A song's lyrics in an .lrc (or .txt)
// beside it go into its .wittyaudio; an album's cover (cover.jpg, folder.jpg and the rest
// albumCover reads) and album.nfo go into album.wittyaudio in its folder. Spreading writes
// the .lrc, the cover and album.nfo back out. The audio files themselves are never changed:
// tags inside them stay as they are.

func (s *Store) songIDs(ctx context.Context, libs []int64) ([]int64, error) {
	return s.idsIn(ctx, `SELECT id FROM items WHERE kind = 'audio' AND library_id = ? ORDER BY id`, libs)
}

// songLyricsFile is the .lrc (else .txt) beside a song.
func songLyricsFile(path string) string {
	base := strings.TrimSuffix(path, filepath.Ext(path))
	for _, ext := range []string{".lrc", ".LRC", ".txt"} {
		if fileThere(base + ext) {
			return base + ext
		}
	}
	return ""
}

// lyricsOf turns lyrics read from a file into a .wittyaudio's.
func lyricsOf(d lyrics.Doc) *wittyinfo.Lyrics {
	if d.Empty() {
		return nil
	}
	l := &wittyinfo.Lyrics{Plain: d.Plain}
	for _, ln := range d.Timed {
		line := wittyinfo.LyricLine{Start: wittyinfo.Seconds(float64(ln.T) / 1000), Text: ln.Text}
		if ln.End > ln.T {
			line.End = wittyinfo.Sec(float64(ln.End) / 1000)
		}
		for _, w := range ln.Words {
			line.Words = append(line.Words, wittyinfo.LyricWord{Start: wittyinfo.Seconds(float64(w.T) / 1000), Text: w.Text})
		}
		l.Lines = append(l.Lines, line)
	}
	if len(l.Lines) > 0 {
		l.Plain = ""
	}
	if len(l.Lines) > wittyinfo.MaxLyricLines {
		l.Lines = l.Lines[:wittyinfo.MaxLyricLines]
	}
	return l
}

// lrcOf writes a .wittyaudio's lyrics as an .lrc (timed) or plain text.
func lrcOf(l *wittyinfo.Lyrics) (string, string) {
	if l == nil {
		return "", ""
	}
	if len(l.Lines) == 0 {
		return l.Plain, ".txt"
	}
	lines := make([]lyrics.Line, 0, len(l.Lines))
	for _, ln := range l.Lines {
		lines = append(lines, lyrics.Line{T: int64(float64(ln.Start)*1000 + 0.5), Text: ln.Text})
	}
	return lyrics.Format(lines), ".lrc"
}

// albumNFO is what an album.nfo says, as other media software writes it.
type albumNFO struct {
	XMLName xml.Name `xml:"album"`
	Title   string   `xml:"title"`
	Artist  string   `xml:"artist,omitempty"`
	Year    int      `xml:"year,omitempty"`
	Genres  []string `xml:"genre"`
	Label   string   `xml:"label,omitempty"`
	Review  string   `xml:"review,omitempty"`
}

func readAlbumNFO(path string) *albumNFO {
	b, err := readPlain(path)
	if err != nil {
		return nil
	}
	var n albumNFO
	if xml.Unmarshal(b, &n) != nil || n.Title == "" {
		return nil
	}
	return &n
}

// gatherSongs takes the libraries' songs' sidecars into .wittyaudio files.
func (s *Scanner) gatherSongs(ctx context.Context, libs []int64, o ConvertOptions, rep *ConvertReport, step func(string)) error {
	ids, err := s.Store.songIDs(ctx, libs)
	if err != nil {
		return err
	}
	albums := map[int64]bool{}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		it, err := s.Store.Item(ctx, id)
		if err != nil || it.IsCDTrack() || isRemoteRef(it.Path) || s.Store.libraryReadOnly(ctx, it.LibraryID) {
			continue
		}
		step(it.Title)
		rep.Titles++
		changed := false
		// The song: its lyrics.
		if p := songLyricsFile(it.Path); p != "" {
			if raw, err := readSmall(p, lyrics.MaxBytes); err == nil {
				t := InfoTarget{Kind: InfoItem, ID: it.ID}
				l := s.Store.LoadInfo(ctx, t)
				x := wittyinfo.NewAudio(wittyinfo.ScopeSong)
				if l.Info != nil {
					x = l.Info.Clone()
				}
				if x.Audio == nil {
					x.Audio = &wittyinfo.Audio{}
				}
				if x.Audio.Lyrics == nil {
					if ly := lyricsOf(lyrics.Parse(string(raw))); ly != nil {
						x.Audio.Lyrics = ly
						if x.Identity == nil {
							x.Identity = s.Store.IdentityOf(ctx, it)
						}
						if err := s.Store.saveConverted(ctx, t, l, x, o); err != nil {
							rep.fail(it.Title, err)
						} else {
							changed = true
							if o.Retire {
								rep.Retired += s.Store.retire(ctx, []string{p})
							}
						}
					}
				}
			}
		}
		// Its album, once: the cover and album.nfo.
		key := s.Store.AlbumKey(ctx, it)
		if !albums[key] {
			albums[key] = true
			dir := filepath.Dir(it.Path)
			var used []string
			t := InfoTarget{Kind: InfoAlbum, ID: key}
			l := s.Store.LoadInfo(ctx, t)
			x := wittyinfo.NewAudio(wittyinfo.ScopeAlbum)
			if l.Info != nil {
				x = l.Info.Clone()
			}
			if c := albumCover(dir); c != "" {
				if ok, err := takePicture(x, "poster", c); err != nil {
					rep.fail(it.Album, err)
				} else if ok {
					used = append(used, c)
				}
			}
			// The back and the disc, and further covers: cover-2.jpg, front2.jpg and on.
			if p := findArt(dir, "back", "back-cover", "backcover"); p != "" {
				if ok, err := takePicture(x, "back", p); err != nil {
					rep.fail(it.Album, err)
				} else if ok {
					used = append(used, p)
				}
			}
			if p := findArt(dir, "cd", "disc", "discart"); p != "" {
				if ok, err := takePicture(x, "disc", p); err != nil {
					rep.fail(it.Album, err)
				} else if ok {
					used = append(used, p)
				}
			}
			more := morePictures(dir, map[string][]string{"poster": {"cover", "folder", "front"}, "back": {"back"}}, false, map[string]string{"poster": albumCover(dir)})
			for _, role := range []string{"poster", "back"} {
				if n, err := takeMorePictures(x, role, more[role]); err != nil {
					rep.fail(it.Album, err)
				} else if n > 0 {
					used = append(used, more[role][:n]...)
				}
			}
			if p := filepath.Join(dir, "album.nfo"); fileThere(p) {
				if n := readAlbumNFO(p); n != nil {
					m := &wittyinfo.Metadata{Title: n.Title, Overview: n.Review, Genres: n.Genres}
					if n.Year > 0 {
						y := n.Year
						m.Year = &y
					}
					x.Metadata = fillMetadata(x.Metadata, m)
					if x.Audio == nil {
						x.Audio = &wittyinfo.Audio{}
					}
					if x.Audio.AlbumArtist == "" {
						x.Audio.AlbumArtist = n.Artist
					}
					if x.Audio.Label == "" {
						x.Audio.Label = n.Label
					}
					used = append(used, p)
				}
			}
			if len(used) > 0 {
				if err := s.Store.saveConverted(ctx, t, l, x, o); err != nil {
					rep.fail(it.Album, err)
				} else {
					changed = true
					if o.Retire {
						rep.Retired += s.Store.retire(ctx, used)
					}
				}
			}
		}
		if changed {
			rep.Converted++
			_ = s.ApplyInfoNow(ctx, it.ID)
		} else {
			rep.Unchanged++
		}
	}
	return nil
}

// spreadSongs writes the songs' .wittyaudio files back out as .lrc files, covers and
// album.nfo files.
func (s *Scanner) spreadSongs(ctx context.Context, libs []int64, o ConvertOptions, rep *ConvertReport, step func(string)) error {
	ids, err := s.Store.songIDs(ctx, libs)
	if err != nil {
		return err
	}
	roots := s.Store.Roots(ctx)
	albums := map[int64]bool{}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		it, err := s.Store.Item(ctx, id)
		if err != nil || it.IsCDTrack() || isRemoteRef(it.Path) || s.Store.libraryReadOnly(ctx, it.LibraryID) {
			continue
		}
		step(it.Title)
		rep.Titles++
		did := false
		infos := s.Store.InfosFor(ctx, it)
		if x := infos.UsableItem(); x != nil && x.Audio != nil && x.Audio.Lyrics != nil {
			if text, ext := lrcOf(x.Audio.Lyrics); text != "" {
				dst := strings.TrimSuffix(it.Path, filepath.Ext(it.Path)) + ext
				if err := writeBeside(dst, []byte(text), roots); err != nil {
					rep.fail(it.Title, err)
				} else {
					did = true
				}
			}
			if o.Retire && infos.Item.Origin == "file" {
				rep.Retired += s.Store.retire(ctx, []string{infos.Item.Path})
			}
			if o.Retire {
				_ = s.Store.SetStoredInfo(ctx, InfoTarget{Kind: InfoItem, ID: it.ID}, nil)
			}
		}
		key := s.Store.AlbumKey(ctx, it)
		if a := infos.Album; a.Info != nil && !albums[key] {
			albums[key] = true
			dir := filepath.Dir(it.Path)
			x := a.Info.Localize(s.Store.serverLanguage(ctx))
			// The covers: cover.jpg, then cover-2.jpg and on; the back as back.jpg.
			for role, file := range map[string]string{"poster": "cover", "back": "back", "disc": "cd"} {
				for i, name := range x.Artwork[role] {
					if len(x.Files[name]) == 0 {
						continue
					}
					stem := file
					if i > 0 {
						stem += "-" + strconv.Itoa(i+1)
					}
					dst := filepath.Join(dir, stem+strings.ToLower(filepath.Ext(name)))
					if i > 0 && fileThere(dst) {
						continue
					}
					if err := writeBeside(dst, x.Files[name], roots); err != nil {
						rep.fail(it.Album, err)
					} else {
						did = true
					}
				}
			}
			if m := x.Metadata; m != nil && m.Title != "" {
				n := albumNFO{Title: m.Title, Genres: m.Genres, Review: m.Overview}
				if m.Year != nil {
					n.Year = *m.Year
				}
				if x.Audio != nil {
					n.Artist, n.Label = x.Audio.AlbumArtist, x.Audio.Label
				}
				if err := writeNFO(filepath.Join(dir, "album.nfo"), n, roots); err != nil {
					rep.fail(it.Album, err)
				} else {
					did = true
				}
			}
			if o.Retire {
				if a.Origin == "file" {
					rep.Retired += s.Store.retire(ctx, []string{a.Path})
				}
				_ = s.Store.SetStoredInfo(ctx, InfoTarget{Kind: InfoAlbum, ID: key}, nil)
			}
		}
		if did {
			rep.Converted++
		} else {
			rep.Unchanged++
		}
	}
	return nil
}

func isRemoteRef(p string) bool { return strings.Contains(p, "://") }

// countSongs adds the songs to a count of what there is to convert.
func (s *Store) countSongs(ctx context.Context, libs []int64, c *ConvertCounts) error {
	ids, err := s.songIDs(ctx, libs)
	if err != nil {
		return err
	}
	dirs := map[string]bool{}
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		it, err := s.Item(ctx, id)
		if err != nil || it.IsCDTrack() || isRemoteRef(it.Path) {
			continue
		}
		c.Songs++
		if songLyricsFile(it.Path) != "" {
			c.SongsWithLyrics++
		}
		for _, p := range InfoPaths(it) {
			if fileThere(p) {
				c.SongsWithInfo++
				break
			}
		}
		dir := filepath.Dir(it.Path)
		if !dirs[dir] {
			dirs[dir] = true
			c.Albums++
			if albumCover(dir) != "" || fileThere(filepath.Join(dir, "album.nfo")) {
				c.AlbumsWithArt++
			}
			if _, err := os.Stat(filepath.Join(dir, "album"+wittyinfo.ExtAudio)); err == nil {
				c.AlbumsWithInfo++
			}
		}
	}
	_ = fmt.Sprint
	return nil
}
