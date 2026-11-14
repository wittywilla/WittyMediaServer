// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"fmt"
	"sort"
)

// ItemFacets are what a library's items can be filtered by, each with how many
// there are: the generic facets. A module that knows more about its items (players, series)
// counts those itself.
type ItemFacets struct {
	Genres  []ItemFacet `json:"genres"`
	Decades []ItemFacet `json:"decades"`
	Rated   int         `json:"rated"`   // items with a score
	Letters []ItemFacet `json:"letters"` // first letters of the titles, in title order
}

// ItemFacet is one choice in a filter.
type ItemFacet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// LibraryItemFacets counts what's in a library, as far as this account can see: its items as
// lists show them (not other versions of one). One console narrows it.
func (s *Store) LibraryItemFacets(ctx context.Context, libraryID int64, console string, access *Access) (*ItemFacets, error) {
	where := `i.library_id = ? AND i.missing = 0 AND ` + hideLinked
	args := []any{libraryID}
	if console != "" {
		where += ` AND i.console = ?`
		args = append(args, console)
	}
	if access != nil {
		c, a := access.ItemClause()
		where += " AND " + c
		args = append(args, a...)
	}
	out := &ItemFacets{Genres: []ItemFacet{}, Decades: []ItemFacet{}, Letters: []ItemFacet{}}
	rows, err := s.DB.QueryContext(ctx, `SELECT IFNULL(i.genres, ''), COUNT(*) FROM items i WHERE `+where+` GROUP BY 1`, args...)
	if err != nil {
		return nil, err
	}
	genres := map[string]int{}
	for rows.Next() {
		var list string
		var n int
		if rows.Scan(&list, &n) == nil {
			for _, g := range splitList(list) {
				genres[g] += n
			}
		}
	}
	rows.Close()
	for g, n := range genres {
		out.Genres = append(out.Genres, ItemFacet{g, n})
	}
	sort.Slice(out.Genres, func(a, b int) bool {
		if out.Genres[a].Count != out.Genres[b].Count {
			return out.Genres[a].Count > out.Genres[b].Count
		}
		return out.Genres[a].Value < out.Genres[b].Value
	})
	rows, err = s.DB.QueryContext(ctx, `SELECT i.year / 10 * 10, COUNT(*) FROM items i WHERE `+where+` AND i.year IS NOT NULL GROUP BY 1 ORDER BY 1`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d, n int
		if rows.Scan(&d, &n) == nil {
			out.Decades = append(out.Decades, ItemFacet{fmt.Sprint(d), n})
		}
	}
	rows.Close()
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM items i WHERE `+where+` AND i.rating IS NOT NULL`, args...).Scan(&out.Rated)
	rows, err = s.DB.QueryContext(ctx, `SELECT UPPER(SUBSTR(i.sort_title, 1, 1)), COUNT(*) FROM items i WHERE `+where+` GROUP BY 1 ORDER BY 1 COLLATE NOCASE`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var f ItemFacet
		if rows.Scan(&f.Value, &f.Count) == nil {
			out.Letters = append(out.Letters, f)
		}
	}
	rows.Close()
	return out, nil
}

// FacetCounts says how many items a library holds for each value of the console facet (a
// games library's consoles).
func (s *Store) FacetCounts(ctx context.Context, libraryID int64, access *Access) (map[string]int, error) {
	q := `SELECT IFNULL(i.console, ''), COUNT(*) FROM items i WHERE i.library_id = ? AND i.missing = 0 AND ` + hideLinked
	args := []any{libraryID}
	if access != nil {
		c, a := access.ItemClause()
		q += " AND " + c
		args = append(args, a...)
	}
	rows, err := s.DB.QueryContext(ctx, q+` GROUP BY 1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var c string
		var n int
		if rows.Scan(&c, &n) == nil {
			out[c] = n
		}
	}
	return out, rows.Err()
}
