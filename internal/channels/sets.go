// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package channels

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// A set is a person's own list of channels, such as Favourites, Kids or Sports, in their
// own order. Its channels are guide keys (lib-5, tun-12), so one set can mix channels from
// the library and from every tuner. An administrator can share a set with the household;
// everyone then sees it, and only an administrator changes it. What a profile may not
// watch stays in a set and is simply never shown to that profile: limits are applied when
// the guide is drawn, not here.

// Set is one custom set.
type Set struct {
	ID       int64    `json:"-"`
	UserID   int64    `json:"-"`
	Name     string   `json:"name"`
	Shared   bool     `json:"shared"`
	Channels []string `json:"channels"`
}

// Limits on what one account keeps, so a set can't grow without end.
const (
	MaxSets         = 50
	MaxSetChannels  = 5000
	maxSetNameRunes = 40
)

var (
	// ErrSetNotFound: no such set, or not one this account can see.
	ErrSetNotFound = errors.New("set not found")
	// ErrSetNotYours: a set the account can see but not change.
	ErrSetNotYours = errors.New("only an administrator can change a set shared with everyone")
)

// CleanSetName checks a set's name and tidies it.
func CleanSetName(name string) (string, error) {
	n := strings.Join(strings.Fields(name), " ")
	switch {
	case n == "":
		return "", errors.New("give the set a name")
	case utf8.RuneCountInString(n) > maxSetNameRunes:
		return "", fmt.Errorf("a set's name can be up to %d characters", maxSetNameRunes)
	}
	return n, nil
}

// ValidKey says whether s looks like a guide key.
func ValidKey(s string) bool {
	k, v, ok := strings.Cut(s, "-")
	if !ok || (k != "lib" && k != "tun") || v == "" || len(v) > 18 {
		return false
	}
	return strings.Trim(v, "0123456789") == "" && v[0] != '0'
}

// cleanKeys drops what isn't a key and repeats, keeping the first place of each.
func cleanKeys(keys []string) ([]string, error) {
	out := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if !ValidKey(k) || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	if len(out) > MaxSetChannels {
		return nil, fmt.Errorf("a set can hold up to %d channels", MaxSetChannels)
	}
	return out, nil
}

// SetSummary is a set as a list of sets shows it.
type SetSummary struct {
	ID     int64
	UserID int64
	Name   string
	Shared bool
	Keys   []string
}

// Sets lists the sets an account sees: its own and every shared one, in its own order,
// with sets it hasn't placed yet after the rest, its own first, then oldest first.
func (s *Store) Sets(ctx context.Context, userID int64) ([]SetSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT s.id, s.user_id, s.name, s.shared FROM tv_sets s
		LEFT JOIN tv_set_order o ON o.set_id = s.id AND o.user_id = ?
		WHERE s.user_id = ? OR s.shared = 1
		ORDER BY o.position IS NULL, o.position, s.user_id != ?, s.id`, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	var out []SetSummary
	byID := map[int64]int{}
	for rows.Next() {
		var x SetSummary
		var shared int
		if err := rows.Scan(&x.ID, &x.UserID, &x.Name, &shared); err != nil {
			rows.Close()
			return nil, err
		}
		x.Shared = shared != 0
		byID[x.ID] = len(out)
		out = append(out, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	// Every set's channels in one pass: a household has a few dozen sets at most.
	crows, err := s.DB.QueryContext(ctx, `SELECT c.set_id, c.channel FROM tv_set_channels c JOIN tv_sets s ON s.id = c.set_id
		WHERE s.user_id = ? OR s.shared = 1 ORDER BY c.set_id, c.position`, userID)
	if err != nil {
		return nil, err
	}
	defer crows.Close()
	for crows.Next() {
		var id int64
		var key string
		if err := crows.Scan(&id, &key); err != nil {
			return nil, err
		}
		if i, ok := byID[id]; ok {
			out[i].Keys = append(out[i].Keys, key)
		}
	}
	return out, crows.Err()
}

// GetSet is one set the account can see.
func (s *Store) GetSet(ctx context.Context, userID, id int64) (*Set, error) {
	var x Set
	var shared int
	err := s.DB.QueryRowContext(ctx, `SELECT id, user_id, name, shared FROM tv_sets WHERE id = ? AND (user_id = ? OR shared = 1)`, id, userID).
		Scan(&x.ID, &x.UserID, &x.Name, &shared)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSetNotFound
	}
	if err != nil {
		return nil, err
	}
	x.Shared = shared != 0
	x.Channels, err = s.setKeys(ctx, id)
	return &x, err
}

func (s *Store) setKeys(ctx context.Context, id int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT channel FROM tv_set_channels WHERE set_id = ? ORDER BY position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// MayEditSet says whether an account may change a set: its own, or any shared one for an administrator.
func MayEditSet(x *Set, userID int64, admin bool) bool {
	if x.Shared {
		return admin
	}
	return x.UserID == userID
}

// nameTaken: names are unique among the sets one account sees, so a picker never shows two alike.
func (s *Store) nameTaken(ctx context.Context, tx *sql.Tx, userID, except int64, name string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tv_sets WHERE (user_id = ? OR shared = 1) AND id != ? AND name = ? COLLATE NOCASE`, userID, except, name).Scan(&n)
	return n > 0, err
}

// CreateSet makes a set. Only an administrator's call should pass shared.
func (s *Store) CreateSet(ctx context.Context, userID int64, name string, keys []string, shared bool) (*Set, error) {
	name, err := CleanSetName(name)
	if err != nil {
		return nil, err
	}
	if keys, err = cleanKeys(keys); err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tv_sets WHERE user_id = ?`, userID).Scan(&n); err != nil {
		return nil, err
	}
	if n >= MaxSets {
		return nil, fmt.Errorf("that's as many sets as one account can have (%d)", MaxSets)
	}
	if taken, err := s.nameTaken(ctx, tx, userID, 0, name); err != nil {
		return nil, err
	} else if taken {
		return nil, fmt.Errorf("there's already a set called %s", name)
	}
	now := db.Now()
	res, err := tx.ExecContext(ctx, `INSERT INTO tv_sets(user_id, name, shared, created_at, updated_at) VALUES(?,?,?,?,?)`, userID, name, boolInt(shared), now, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	if err := writeKeys(ctx, tx, id, keys); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Set{ID: id, UserID: userID, Name: name, Shared: shared, Channels: keys}, nil
}

func writeKeys(ctx context.Context, tx *sql.Tx, id int64, keys []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM tv_set_channels WHERE set_id = ?`, id); err != nil {
		return err
	}
	st, err := tx.PrepareContext(ctx, `INSERT INTO tv_set_channels(set_id, channel, position) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for i, k := range keys {
		if _, err := st.ExecContext(ctx, id, k, i); err != nil {
			return err
		}
	}
	return nil
}

// SetChange is what an edit changes; nil leaves a field as it is.
type SetChange struct {
	Name     *string   `json:"name"`
	Channels *[]string `json:"channels"`
	Shared   *bool     `json:"shared"`
	Add      []string  `json:"add"`
	Remove   []string  `json:"remove"`
}

// UpdateSet changes a set the caller may change; the caller has checked MayEditSet, and
// that only an administrator touches Shared.
func (s *Store) UpdateSet(ctx context.Context, userID, id int64, c SetChange) (*Set, error) {
	cur, err := s.GetSet(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if c.Name != nil {
		name, err := CleanSetName(*c.Name)
		if err != nil {
			return nil, err
		}
		if taken, err := s.nameTaken(ctx, tx, cur.UserID, id, name); err != nil {
			return nil, err
		} else if taken {
			return nil, fmt.Errorf("there's already a set called %s", name)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tv_sets SET name = ? WHERE id = ?`, name, id); err != nil {
			return nil, err
		}
		cur.Name = name
	}
	if c.Shared != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE tv_sets SET shared = ? WHERE id = ?`, boolInt(*c.Shared), id); err != nil {
			return nil, err
		}
		cur.Shared = *c.Shared
	}
	keys := cur.Channels
	changed := false
	if c.Channels != nil {
		keys, changed = *c.Channels, true
	}
	if len(c.Add) > 0 || len(c.Remove) > 0 {
		gone := make(map[string]bool, len(c.Remove))
		for _, k := range c.Remove {
			gone[k] = true
		}
		next := make([]string, 0, len(keys)+len(c.Add))
		for _, k := range append(append([]string{}, keys...), c.Add...) {
			if !gone[k] {
				next = append(next, k)
			}
		}
		keys, changed = next, true
	}
	if changed {
		if keys, err = cleanKeys(keys); err != nil {
			return nil, err
		}
		if err := writeKeys(ctx, tx, id, keys); err != nil {
			return nil, err
		}
		cur.Channels = keys
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tv_sets SET updated_at = ? WHERE id = ?`, db.Now(), id); err != nil {
		return nil, err
	}
	return cur, tx.Commit()
}

// DeleteSet removes a set; the caller has checked MayEditSet.
func (s *Store) DeleteSet(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM tv_sets WHERE id = ?`, id)
	return err
}

// OrderSets puts the sets an account sees in the order given. Sets it can't see are ignored,
// and ones left out keep coming after these.
func (s *Store) OrderSets(ctx context.Context, userID int64, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM tv_set_order WHERE user_id = ?`, userID); err != nil {
		return err
	}
	pos := 0
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		res, err := tx.ExecContext(ctx, `INSERT INTO tv_set_order(user_id, set_id, position)
			SELECT ?, id, ? FROM tv_sets WHERE id = ? AND (user_id = ? OR shared = 1)`, userID, pos, id, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			pos++
		}
	}
	return tx.Commit()
}
