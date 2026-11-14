// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import "context"

// PlayedGame is a game someone has played: its item, and when they last did.
type PlayedGame struct {
	ItemID     int64
	LastPlayed int64
}

// RecentlyPlayedGames is someone's most recently played games, newest first, from the time
// played kept in progress (WittyPlay's sessions add to it through the server). Whether they can
// still see each one is the caller's to check.
func (s *Store) RecentlyPlayedGames(ctx context.Context, userID int64, limit int) ([]PlayedGame, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT p.item_id, p.updated_at FROM progress p JOIN items i ON i.id = p.item_id
		WHERE p.user_id = ? AND i.kind = 'game' AND i.missing = 0 AND p.position_sec > 0
		ORDER BY p.updated_at DESC, p.item_id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlayedGame{}
	for rows.Next() {
		var g PlayedGame
		if err := rows.Scan(&g.ItemID, &g.LastPlayed); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
