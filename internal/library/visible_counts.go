// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import "context"

// VisibleCounts puts in each library's ItemCount how many of its titles the
// account may see. A library's own count is of everything in it, and to a
// profile with limits that number says there is more than it is shown: hidden
// titles aren't to show in counts either (docs/PROFILES.md). An account with
// no limits keeps the counts it has.
func (s *Store) VisibleCounts(ctx context.Context, libs []Library, a *Access) error {
	if !a.Limited() {
		return nil
	}
	clause, args := a.ItemClause()
	for i := range libs {
		q := append([]any{libs[i].ID}, args...)
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM items i WHERE i.library_id = ? AND i.missing = 0 AND `+clause, q...).Scan(&libs[i].ItemCount); err != nil {
			return err
		}
	}
	return nil
}
