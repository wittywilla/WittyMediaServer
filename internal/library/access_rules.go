// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"strings"
)

// Limits by collection and by tag name titles through other tables. Lists
// made in SQL follow those tables directly (ItemClause). A check of one
// title in Go needs them looked up first, which is what this file does.

// accessSet is what an account's collections and tags come to right now.
type accessSet struct {
	blockItems, blockShows, allowItems, allowShows, tagItems map[int64]bool
}

func (r *accessSet) blocks(item, show int64) bool {
	return r != nil && ((item != 0 && r.blockItems[item]) || (show != 0 && r.blockShows[show]))
}

func (r *accessSet) allows(item, show int64) bool {
	return r != nil && ((item != 0 && r.allowItems[item]) || (show != 0 && r.allowShows[show]))
}

func (r *accessSet) tagged(item int64) bool { return r != nil && r.tagItems[item] }

// needsSet reports whether the account has limits that Resolve looks up.
func (a *Access) needsSet() bool {
	return a != nil && (len(a.BlockCollections) > 0 || len(a.AllowCollections) > 0 || len(a.BlockTags) > 0)
}

// maxRuleTitles bounds what one account's rules can name. A collection holds
// far fewer; past this the rest are left to the lists made in SQL, and a check
// of one title falls back to asking the database (see Resolve).
const maxRuleTitles = 20000

// Resolve looks up the titles an account's collections and tags name, so
// AllowsItem and AllowsShow can answer for them. It returns the same limits
// when there is nothing to look up. An error leaves the rules unresolved, and
// unresolved rules allow nothing.
func (s *Store) Resolve(ctx context.Context, a *Access) (*Access, error) {
	if !a.needsSet() {
		return a, nil
	}
	out := *a
	set := &accessSet{blockItems: map[int64]bool{}, blockShows: map[int64]bool{}, allowItems: map[int64]bool{}, allowShows: map[int64]bool{}, tagItems: map[int64]bool{}}
	fill := func(into map[int64]bool, q string, args ...any) error {
		rows, err := s.DB.QueryContext(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			into[id] = true
		}
		return rows.Err()
	}
	limit := " LIMIT " + ids([]int64{maxRuleTitles})
	for _, c := range []struct {
		list         []int64
		items, shows map[int64]bool
	}{{a.BlockCollections, set.blockItems, set.blockShows}, {a.AllowCollections, set.allowItems, set.allowShows}} {
		if len(c.list) == 0 {
			continue
		}
		in := ids(c.list)
		if err := fill(c.items, `SELECT item_id FROM collection_items WHERE collection_id IN (`+in+`)`+limit); err != nil {
			return nil, err
		}
		if err := fill(c.shows, `SELECT show_id FROM collection_shows WHERE collection_id IN (`+in+`)`+limit); err != nil {
			return nil, err
		}
	}
	if len(a.BlockTags) > 0 {
		var where []string
		var args []any
		for _, t := range a.BlockTags {
			where = append(where, "('|' || tags || '|') LIKE ?")
			args = append(args, "%|"+t+"|%")
		}
		if err := fill(set.tagItems, `SELECT item_id FROM video_meta WHERE tags <> '' AND (`+strings.Join(where, " OR ")+`)`+limit, args...); err != nil {
			return nil, err
		}
	}
	out.set = set
	return &out, nil
}

// Resolved reports whether the limits can answer for one title: they need no
// looking up, or have been.
func (a *Access) Resolved() bool { return !a.needsSet() || a.set != nil }
