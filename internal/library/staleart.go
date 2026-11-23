// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// StaleArt deals with a poster or backdrop stored as a picture beside the media that is no
// longer there: renamed, deleted, or replaced by another program under another name. It
// looks beside the media for the next best picture, as a scan would, and points the item
// at it, so the next request needs no second look. With none, a poster that is gone is
// forgotten, so the next scan makes one (a frame, or a title poster, by the noPoster
// setting); a backdrop that is gone is forgotten, and the poster stands in.
//
// It returns the picture to serve instead ("" for none). A stored value that isn't a path
// (a cached or remote picture, or a frame still to make), a picture that is there, or a
// media file that can't be reached either (its drive is away) changes nothing: what is
// stored stays, for when the drive is back.
func (s *Store) StaleArt(ctx context.Context, it *Item, backdrop bool) string {
	old := it.Poster
	column := "poster"
	if backdrop {
		old, column = it.Backdrop, "backdrop"
	}
	if !isLocalArt(old) || it.Path == "" {
		return ""
	}
	if _, err := os.Stat(old); !errors.Is(err, fs.ErrNotExist) {
		return "" // there, or can't be told
	}
	if _, err := os.Stat(it.Path); err != nil {
		return "" // the media itself is away: the drive, not the picture
	}
	base := strings.TrimSuffix(filepath.Base(it.Path), filepath.Ext(it.Path))
	dir := filepath.Dir(it.Path)
	if st, err := os.Stat(it.Path); err == nil && st.IsDir() { // a disc's folder
		base, dir = discFolderName(it.Path), it.Path
	}
	poster, back := localArtwork(dir, base)
	if it.ShowID != nil {
		// An episode's own picture, not the season's poster beside it.
		poster = findArt(dir, base+"-thumb", base)
	}
	found := poster
	if backdrop {
		found = back
	}
	if found == old {
		found = ""
	}
	// Only if it still says what was read: a scan or an edit may have changed it meanwhile.
	_, _ = s.DB.ExecContext(ctx, `UPDATE items SET `+column+` = ?, updated_at = ? WHERE id = ? AND `+column+` = ?`, found, db.Now(), it.ID, old)
	if backdrop {
		it.Backdrop = found
	} else {
		it.Poster = found
	}
	return found
}

// isLocalArt: a stored picture that is a file's path, not a cached, remote or still-to-make one.
func isLocalArt(v string) bool {
	return v != "" && !strings.HasPrefix(v, "cache:") && !strings.HasPrefix(v, LazyPoster) &&
		!strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://")
}

// repointStaleArt runs StaleArt over a library's items whose stored poster or backdrop is a
// file's path, after a scan: a picture renamed or replaced beside unchanged media is found
// again without waiting for someone to open the page. One look at each such picture.
func (s *Store) repointStaleArt(ctx context.Context, libraryID int64) (int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, path, IFNULL(poster, ''), IFNULL(backdrop, ''), show_id FROM items
		WHERE library_id = ? AND missing = 0 AND (IFNULL(poster, '') <> '' OR IFNULL(backdrop, '') <> '')`, libraryID)
	if err != nil {
		return 0, err
	}
	var todo []*Item
	for rows.Next() {
		it := &Item{}
		var show *int64
		if err := rows.Scan(&it.ID, &it.Path, &it.Poster, &it.Backdrop, &show); err != nil {
			rows.Close()
			return 0, err
		}
		it.ShowID = show
		if isLocalArt(it.Poster) || isLocalArt(it.Backdrop) {
			todo = append(todo, it)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	for _, it := range todo {
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		before := [2]string{it.Poster, it.Backdrop}
		s.StaleArt(ctx, it, false)
		s.StaleArt(ctx, it, true)
		if before != [2]string{it.Poster, it.Backdrop} {
			n++
		}
	}
	return n, nil
}
