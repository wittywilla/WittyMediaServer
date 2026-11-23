// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// VariousArtists is the album artist of an album whose songs are by different artists and
// whose folder doesn't say whose it is.
const VariousArtists = "Various Artists"

// unifyFolderAlbums makes the songs of one album folder that name the same album one album
// (#355). Albums are the songs grouped by album artist and album title, so two things split
// one folder's album:
//
//   - a compilation's songs have their own artists and no album artist, so each song's
//     artist stood in for it: the album is by the artist the folders name, when that is one
//     of its songs' artists, else by Various Artists; each song keeps its own artist.
//   - the album tags differ in case or spacing ("2.5 beats" and "2.5beats"): the spelling
//     most of the songs use is the album's.
//
// A disc's folder ("CD1", "Disc 2") counts as its album's folder. Nothing is written to the
// files; a scan reads the tags again and this runs after it.
func (s *Store) unifyFolderAlbums(ctx context.Context, libraryID int64) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, path, IFNULL(album, ''), IFNULL(album_artist, ''), IFNULL(artist, '')
		FROM items WHERE library_id = ? AND kind = 'audio' AND missing = 0 AND IFNULL(album, '') <> ''`, libraryID)
	if err != nil {
		return err
	}
	type song struct {
		id                       int64
		path, album, albumArtist string
		artist                   string
	}
	groups := map[string][]song{}
	var order []string
	for rows.Next() {
		var x song
		if err := rows.Scan(&x.id, &x.path, &x.album, &x.albumArtist, &x.artist); err != nil {
			rows.Close()
			return err
		}
		k := albumFolder(x.path) + "\x00" + looseAlbumName(x.album)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, k := range order {
		g := groups[k]
		if len(g) < 2 {
			continue
		}
		titles := map[string]int{}
		owners := map[string]string{} // lower case → as written
		for _, x := range g {
			titles[x.album]++
			if _, ok := owners[strings.ToLower(x.albumArtist)]; !ok {
				owners[strings.ToLower(x.albumArtist)] = x.albumArtist
			}
		}
		album := mostUsed(titles)
		owner := g[0].albumArtist
		if len(owners) > 1 {
			artists := make([]string, len(g))
			for i, x := range g {
				artists[i] = x.artist
			}
			owner = folderArtist(albumFolder(g[0].path), artists)
		}
		for _, x := range g {
			if x.album == album && (x.albumArtist == owner || len(owners) == 1) {
				continue
			}
			na := x.albumArtist
			if len(owners) > 1 {
				na = owner
			}
			if _, err := s.DB.ExecContext(ctx, `UPDATE items SET album = ?, album_artist = ? WHERE id = ?`, album, na, x.id); err != nil {
				return err
			}
		}
	}
	return nil
}

// albumFolder is the folder an album is in: a disc's folder counts as its album's.
func albumFolder(path string) string {
	dir := filepath.Dir(path)
	if discPartRe.MatchString(" " + filepath.Base(dir) + " ") {
		return filepath.Dir(dir)
	}
	return dir
}

// looseAlbumName is an album title as people tell titles apart: not by case or spacing.
func looseAlbumName(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}

// mostUsed is the name most songs use; of names used as often, the first in order.
func mostUsed(counts map[string]int) string {
	names := make([]string, 0, len(counts))
	for n := range counts {
		names = append(names, n)
	}
	slices.Sort(names)
	best := names[0]
	for _, n := range names[1:] {
		if counts[n] > counts[best] {
			best = n
		}
	}
	return best
}

// folderArtist is the album artist of a folder whose songs are by different artists: the
// artist the folder above the album is named after, when one of the songs is theirs
// (Music/MonoCulture/The Animal Economy), else Various Artists.
func folderArtist(albumDir string, artists []string) string {
	parent := filepath.Base(filepath.Dir(albumDir))
	for _, a := range artists {
		if a != "" && strings.EqualFold(a, parent) {
			return a
		}
	}
	return VariousArtists
}
