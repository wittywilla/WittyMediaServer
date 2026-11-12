// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"strings"
	"time"
)

// DayNumber is a day as the play log writes it: YYYYMMDD on the server's calendar.
func DayNumber(t time.Time) int { return dayNumber(t) }

// What has been watched, read and listened to, added up from the play log: the statistics
// a page can show as cards (the SDUI source "statistics"). Whose record it is, is the
// caller's to decide; what it names keeps to what the account may open.

// WatchQuery asks for the most watched titles, shows or libraries.
type WatchQuery struct {
	// UserID is whose record: 0 is everyone's, which only an administrator may be shown.
	UserID int64
	// By is "title", "show" (a show's episodes together) or "library".
	By string
	// FromDay is the first day counted, as the play log writes days (YYYYMMDD); 0 is all.
	FromDay int
	// LibraryID keeps to one library; 0 is every one the account may open.
	LibraryID int64
	Limit     int
}

// WatchTotal is one title's, show's or library's time.
type WatchTotal struct {
	// ID is the item's, the show's or the library's.
	ID      int64
	Seconds float64
	// People is how many accounts it was; Titles how many titles of a library or show.
	People, Titles int
}

// MostWatched adds up the play log as q asks, most time first. Only titles the account may
// see count, titles no longer on disk aren't named, and a show is there only if it may see
// the show itself.
func (s *Store) MostWatched(ctx context.Context, q WatchQuery, access *Access) ([]WatchTotal, error) {
	where := []string{"1=1"}
	var args []any
	if q.UserID != 0 {
		where = append(where, "p.user_id = ?")
		args = append(args, q.UserID)
	}
	if q.FromDay > 0 {
		where = append(where, "p.day >= ?")
		args = append(args, q.FromDay)
	}
	if q.LibraryID != 0 {
		where = append(where, "i.library_id = ?")
		args = append(args, q.LibraryID)
	}
	c, a := access.ItemClause()
	where = append(where, c)
	args = append(args, a...)
	from, key := "play_log p JOIN items i ON i.id = p.item_id", "p.item_id"
	switch q.By {
	case "show":
		from += " JOIN shows s ON s.id = i.show_id"
		key = "i.show_id"
		sc, sa := access.ShowClause()
		where = append(where, sc)
		args = append(args, sa...)
	case "library":
		key = "i.library_id"
	default:
		where = append(where, "i.missing = 0")
	}
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+key+`, SUM(p.seconds), COUNT(DISTINCT p.user_id), COUNT(DISTINCT p.item_id) FROM `+from+`
		WHERE `+strings.Join(where, " AND ")+` GROUP BY 1 HAVING SUM(p.seconds) > 0 ORDER BY 2 DESC, 1 LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WatchTotal
	for rows.Next() {
		var t WatchTotal
		if err := rows.Scan(&t.ID, &t.Seconds, &t.People, &t.Titles); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
