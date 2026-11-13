// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/wittyplayfile"
)

// How a game is pictured in WittyPlay, server-wide (docs/specs/wittyplay-file.md): what an
// administrator chose for one game, and what a .wittyplay file beside the game says. A person's own
// choices are kept with their profile (/me/ui), and the server-wide default and per-console looks in
// the setting GamePicturesSetting.

// Where a game's look came from.
const (
	PictureByAdmin = "admin"
	PictureByFile  = "file"
)

// GamePicturesSetting holds the server's own .wittyplay document: its default and per-console looks.
const GamePicturesSetting = "wittyplay.pictures"

// SetGamePicture keeps one game's look from one source; an empty look takes it away.
func (s *Store) SetGamePicture(ctx context.Context, itemID int64, source string, l wittyplayfile.Look) error {
	l.Extra = nil
	if l.Empty() {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM game_pictures WHERE item_id = ? AND source = ?`, itemID, source)
		return err
	}
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO game_pictures(item_id, source, look, updated_at) VALUES(?,?,?,?)
		ON CONFLICT(item_id, source) DO UPDATE SET look = excluded.look, updated_at = excluded.updated_at`, itemID, source, string(b), db.Now())
	return err
}

// GamePictures is every game's server-wide look, by item: an administrator's choice over a file's, field by field.
func (s *Store) GamePictures(ctx context.Context) (map[int64]wittyplayfile.Look, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT item_id, source, look FROM game_pictures ORDER BY item_id, source = 'admin' DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]wittyplayfile.Look{}
	for rows.Next() {
		var id int64
		var source, raw string
		if err := rows.Scan(&id, &source, &raw); err != nil {
			return nil, err
		}
		var l wittyplayfile.Look
		if json.Unmarshal([]byte(raw), &l) != nil {
			continue
		}
		l.Extra = nil
		out[id] = out[id].Over(l)
	}
	return out, rows.Err()
}

// GamePictureFrom is one game's look from one source, if there is one.
func (s *Store) GamePictureFrom(ctx context.Context, itemID int64, source string) (wittyplayfile.Look, error) {
	var raw string
	err := s.DB.QueryRowContext(ctx, `SELECT look FROM game_pictures WHERE item_id = ? AND source = ?`, itemID, source).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return wittyplayfile.Look{}, nil
	}
	if err != nil {
		return wittyplayfile.Look{}, err
	}
	var l wittyplayfile.Look
	err = json.Unmarshal([]byte(raw), &l)
	l.Extra = nil
	return l, err
}

// ServerPictures is the server's own .wittyplay document, or an empty one.
func (s *Store) ServerPictures(ctx context.Context) *wittyplayfile.Doc {
	raw := s.DB.GetSetting(ctx, GamePicturesSetting, "")
	if raw != "" {
		if d, err := wittyplayfile.Parse([]byte(raw)); err == nil {
			return d
		}
	}
	return &wittyplayfile.Doc{Version: wittyplayfile.Version}
}

// SetServerPictures keeps the server's own document: its default and consoles. Its games are kept
// per game instead (SetGamePicture), so they're dropped here.
func (s *Store) SetServerPictures(ctx context.Context, d *wittyplayfile.Doc) error {
	keep := *d
	keep.Games = nil
	keep.Version = wittyplayfile.Version
	if err := keep.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(keep)
	if err != nil {
		return err
	}
	return s.DB.SetSetting(ctx, GamePicturesSetting, string(b))
}

// GameMatch finds the games a .wittyplay entry is for, among this server's games: by its id, or
// by its file name and title (and console, when it says one). At most limit.
func (s *Store) GameMatch(ctx context.Context, m wittyplayfile.Match, limit int) ([]int64, error) {
	if m.Item > 0 {
		var id int64
		var console, path, title string
		err := s.DB.QueryRowContext(ctx, `SELECT id, COALESCE(console, ''), path, COALESCE(title, '') FROM items WHERE id = ? AND kind = 'game'`, m.Item).Scan(&id, &console, &path, &title)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !m.Matches(id, console, filepath.Base(path), title) {
			return nil, nil
		}
		return []int64{id}, nil
	}
	q := `SELECT id, COALESCE(console, ''), path, COALESCE(title, '') FROM items WHERE kind = 'game'`
	var args []any
	if m.Title != "" {
		q += ` AND title = ? COLLATE NOCASE`
		args = append(args, strings.TrimSpace(m.Title))
	}
	if m.Console != "" {
		q += ` AND console = ?`
		args = append(args, m.Console)
	}
	if m.File != "" {
		// The file name is the end of the path; the exact test is below.
		q += ` AND path LIKE ? ESCAPE '\'`
		args = append(args, "%"+likeEscapeExact(m.File))
	}
	rows, err := s.DB.QueryContext(ctx, q+` LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() && len(out) < limit {
		var id int64
		var console, path, title string
		if err := rows.Scan(&id, &console, &path, &title); err != nil {
			return nil, err
		}
		if m.Matches(id, console, filepath.Base(path), title) {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

func likeEscapeExact(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// sidecarLook reads the .wittyplay file beside a game, if there is one: "Game (USA).wittyplay" or
// "Game (USA).sfc.wittyplay". It only ever reads: a games folder may well be read-only, and nothing
// is written there. A file that can't be read or isn't a valid .wittyplay counts as none.
func sidecarLook(gamePath, console, title string) (wittyplayfile.Look, bool) {
	dir, name := filepath.Dir(gamePath), filepath.Base(gamePath)
	for _, p := range []string{filepath.Join(dir, gameBaseName(name)+wittyplayfile.Ext), gamePath + wittyplayfile.Ext} {
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() || st.Size() > wittyplayfile.MaxSize {
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		d, err := wittyplayfile.Read(f)
		f.Close()
		if err != nil {
			continue
		}
		return d.For(0, console, name, title), true
	}
	return wittyplayfile.Look{}, false
}

// noteSidecar keeps what the .wittyplay file beside a game says, and forgets it when the file has gone.
func (s *Store) noteSidecar(ctx context.Context, it *Item) error {
	if it == nil || it.ID == 0 || isRemote(it.Path) {
		return nil
	}
	l, _ := sidecarLook(it.Path, it.Console, it.Title)
	return s.SetGamePicture(ctx, it.ID, PictureByFile, l)
}
