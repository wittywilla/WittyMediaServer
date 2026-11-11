// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"database/sql"
)

// A rescan of a big library finds nearly everything as it was. What it takes to see that is read
// once, in one query, instead of a whole row or two for every file. Small on purpose: a library
// of a hundred thousand files has this many of them in memory while it's scanned.
type knownFile struct {
	id, mtime, size int64
	showID          int64 // 0: none
	season, episode int32 // -1: none
	missing         bool
	hasPoster       bool
	hasDisc         bool
	provider        string
	title           string // only where a scan looks at it: see loadKnown
}

// unchanged is what's known about this file, if the library's row is for the file as it is now.
func (c *candidate) unchanged() *knownFile {
	if c.known != nil && c.known.mtime == c.mtime && c.known.size == c.size {
		return c.known
	}
	return nil
}

// loadKnown fills in what the library knows about the files a walk found. Files of no interest to
// it stay as they are: anything it can't say is looked up the slow way, file by file.
func (s *Store) loadKnown(ctx context.Context, libraryID int64, all []candidate, wantTitle bool) error {
	byPath := make(map[string]int, len(all))
	for i := range all {
		byPath[all[i].path] = i
	}
	titles := "''"
	if wantTitle {
		titles = "title"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, path, `+titles+`, IFNULL(provider, ''), IFNULL(poster, '') <> '', IFNULL(mtime, 0), IFNULL(size_bytes, 0), missing,
		IFNULL(show_id, 0), IFNULL(season, -1), IFNULL(episode, -1), disc_json IS NOT NULL FROM items WHERE library_id = ?`, libraryID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var path sql.RawBytes
	for rows.Next() {
		var k knownFile
		if err := rows.Scan(&k.id, &path, &k.title, &k.provider, &k.hasPoster, &k.mtime, &k.size, &k.missing, &k.showID, &k.season, &k.episode, &k.hasDisc); err != nil {
			return err
		}
		if i, ok := byPath[string(path)]; ok {
			all[i].known = &k
		}
	}
	return rows.Err()
}
