// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package channels

import (
	"context"
	"strings"
)

// VideoQuery asks for a page of the short videos a channel can use as
// bumpers or commercials, so the editor never has to load a whole library.
type VideoQuery struct {
	Libraries []int64 // any of these libraries
	Own       bool    // and the bumper list: uploads and videos chosen from other libraries
	IDs       []int64 // only these (to show what is chosen); alone, these wherever they are
	Search    string
	Offset    int
	Limit     int
}

// VideoPage is one page and how many there are in all.
type VideoPage struct {
	Items []Bumper `json:"items"`
	Total int      `json:"total"`
}

const maxVideoPage = 200

// Videos lists a page of videos by title.
func (s *Store) Videos(ctx context.Context, q VideoQuery) (VideoPage, error) {
	out := VideoPage{Items: []Bumper{}}
	if len(q.Libraries) > maxPickLibs || len(q.IDs) > maxListLen {
		return out, nil
	}
	var from []string
	var args []any
	if len(q.Libraries) > 0 {
		from = append(from, "i.library_id IN ("+placeholders(len(q.Libraries))+")")
		args = append(args, anyArgs(q.Libraries)...)
	}
	if q.Own {
		from = append(from, "b.item_id IS NOT NULL")
	}
	if len(from) == 0 {
		if len(q.IDs) == 0 {
			return out, nil
		}
		from = append(from, "1") // just the videos asked for, wherever they are
	}
	where := "i.missing = 0 AND i.kind = 'video' AND (" + strings.Join(from, " OR ") + ")"
	if len(q.IDs) > 0 {
		where += " AND i.id IN (" + placeholders(len(q.IDs)) + ")"
		args = append(args, anyArgs(q.IDs)...)
	}
	if s := strings.TrimSpace(q.Search); s != "" {
		if len(s) > maxTextLen {
			s = s[:maxTextLen]
		}
		where += " AND i.title LIKE ?"
		args = append(args, "%"+strings.NewReplacer("%", "", "_", " ").Replace(s)+"%")
	}
	const join = ` FROM items i JOIN libraries l ON l.id = i.library_id LEFT JOIN channel_bumpers b ON b.item_id = i.id WHERE `
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*)`+join+where, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	limit := q.Limit
	if limit <= 0 || limit > maxVideoPage {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id, i.title, IFNULL(i.runtime_sec, 0), l.name, IFNULL(i.content_rating, ''), IFNULL(b.uploaded, 0), b.item_id IS NOT NULL, i.updated_at`+
		join+where+` ORDER BY i.sort_title COLLATE NOCASE, i.id LIMIT ? OFFSET ?`, append(args, limit, max(q.Offset, 0))...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var b Bumper
		var uploaded, chosen int
		if err := rows.Scan(&b.ItemID, &b.Title, &b.DurationSec, &b.Library, &b.Rating, &uploaded, &chosen, &b.UpdatedAt); err != nil {
			return out, err
		}
		b.Uploaded, b.Chosen = uploaded != 0, chosen != 0 && uploaded == 0
		out.Items = append(out.Items, b)
	}
	return out, rows.Err()
}
