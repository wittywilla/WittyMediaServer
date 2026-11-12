// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"database/sql"
	"errors"
	"slices"
)

// WittyPlay Online's records: whether a profile lets others on this server see what it plays,
// and the friends an administrator lets a Kids profile have. Friends are the other profiles of this server: there is no list of them beyond
// these rules (docs/GAMING-API.md, Playing together).

// PresenceShared says whether a profile lets others see what it plays and whether it's here.
// With no choice of its own the answer is yes, except for a guest: a Kids profile is seen only
// by the friends an administrator chose anyway.
func (s *Store) PresenceShared(ctx context.Context, userID int64, kind string) bool {
	var share int
	err := s.DB.QueryRowContext(ctx, `SELECT share FROM wittyplay_presence WHERE user_id = ?`, userID).Scan(&share)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return kind != "guest"
	}
	return share != 0
}

// SetPresenceShared keeps a profile's choice.
func (s *Store) SetPresenceShared(ctx context.Context, userID int64, share bool) error {
	v := 0
	if share {
		v = 1
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO wittyplay_presence(user_id, share) VALUES(?, ?)
		ON CONFLICT(user_id) DO UPDATE SET share = excluded.share`, userID, v)
	return err
}

// KidsFriends is the profiles an administrator lets a Kids profile see and be seen by.
func (s *Store) KidsFriends(ctx context.Context, kidID int64) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT friend_id FROM wittyplay_kids_friends WHERE kid_id = ? ORDER BY friend_id`, kidID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetKidsFriends replaces a Kids profile's friends. A profile is never its own friend.
func (s *Store) SetKidsFriends(ctx context.Context, kidID int64, friends []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM wittyplay_kids_friends WHERE kid_id = ?`, kidID); err != nil {
		return err
	}
	seen := []int64{}
	for _, f := range friends {
		if f == kidID || slices.Contains(seen, f) {
			continue
		}
		seen = append(seen, f)
		if _, err := tx.ExecContext(ctx, `INSERT INTO wittyplay_kids_friends(kid_id, friend_id) VALUES(?, ?)`, kidID, f); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GamePlayedSeconds is how long a profile has played a game, in all: a game's progress is its
// time played (recordPlayTime in the server).
func (s *Store) GamePlayedSeconds(ctx context.Context, userID, itemID int64) float64 {
	var sec sql.NullFloat64
	_ = s.DB.QueryRowContext(ctx, `SELECT position_sec FROM progress WHERE user_id = ? AND item_id = ?`, userID, itemID).Scan(&sec)
	return sec.Float64
}
