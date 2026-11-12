// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"database/sql"
	"strings"
)

// A books library's bookshelf, grouped the way WittyInk's is: by series (a book's album,
// its place in the series its track) or by author (album artist, else artist). Like albums,
// the groups aren't stored: they are the books themselves, so they never disagree with them.

// BookGroup is one series or one author, with how many books it holds.
type BookGroup struct {
	// Name is "" for the books with no series, or no author.
	Name  string `json:"name"`
	Books int    `json:"books"`
	// CoverItem is a book of the group with a cover, 0 when none has one.
	CoverItem int64 `json:"coverItem"`
}

// BookGroups lists a books library's series (by "series") or authors (by "author"), by
// name, with the books that have none last. Books the account may not see aren't counted,
// and a group of only those isn't there.
func (s *Store) BookGroups(ctx context.Context, libraryID int64, by string, access *Access) ([]BookGroup, error) {
	key := "IFNULL(i.album, '')"
	if by == "author" {
		key = albumArtist
	}
	where := []string{"i.kind = 'book'", "i.missing = 0", "i.library_id = ?", hideLinked}
	args := []any{libraryID}
	if access != nil {
		c, a := access.ItemClause()
		where = append(where, c)
		args = append(args, a...)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+key+` AS g, COUNT(*), MIN(CASE WHEN IFNULL(i.poster, '') <> '' THEN i.id END)
		FROM items i WHERE `+strings.Join(where, " AND ")+` GROUP BY g ORDER BY g = '', g COLLATE NOCASE LIMIT 5000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BookGroup{}
	for rows.Next() {
		var g BookGroup
		var cover sql.NullInt64
		if err := rows.Scan(&g.Name, &g.Books, &cover); err != nil {
			return nil, err
		}
		g.CoverItem = cover.Int64
		out = append(out, g)
	}
	return out, rows.Err()
}
