package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittyplay"
)

// What each person keeps about their games: settings at three levels, and per game a
// favourite mark, a note, play counts and cheats. Their own images and patches are
// files, listed here. Every query has the person in it, so nobody reaches another's.

// GameLayer reads one level of a person's settings. An empty console and item is
// everything they play; a console is that console's games; an item is one game.
func (s *Store) GameLayer(ctx context.Context, userID int64, console string, itemID int64) (*wittyplay.Layer, error) {
	var data string
	err := s.DB.QueryRowContext(ctx, `SELECT data FROM wittyplay_settings WHERE user_id = ? AND console = ? AND item_id = ?`, userID, console, itemID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return &wittyplay.Layer{}, nil
	}
	if err != nil {
		return nil, err
	}
	var l wittyplay.Layer
	if json.Unmarshal([]byte(data), &l) != nil {
		return &wittyplay.Layer{}, nil // something unreadable never stops a game starting
	}
	return &l, nil
}

// SetGameLayer replaces one level. An empty one takes no row.
func (s *Store) SetGameLayer(ctx context.Context, userID int64, console string, itemID int64, l *wittyplay.Layer) error {
	if l == nil || l.Empty() {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM wittyplay_settings WHERE user_id = ? AND console = ? AND item_id = ?`, userID, console, itemID)
		return err
	}
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO wittyplay_settings(user_id, console, item_id, data, updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(user_id, console, item_id) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`, userID, console, itemID, string(b), db.Now())
	return err
}

// GameMeta is what a person keeps about one game.
type GameMeta struct {
	Favourite  bool              `json:"favourite"`
	Note       string            `json:"note"`
	Plays      int               `json:"plays"`
	LastPlayed int64             `json:"lastPlayed"` // seconds since 1970; 0 for never
	Cheats     []wittyplay.Cheat `json:"cheats"`
	HasBattery bool              `json:"hasBattery"` // it has a battery save to download
}

// GetGameMeta reads what a person keeps about a game.
func (s *Store) GetGameMeta(ctx context.Context, userID, itemID int64) (*GameMeta, error) {
	m := &GameMeta{Cheats: []wittyplay.Cheat{}}
	var fav int
	var cheats string
	err := s.DB.QueryRowContext(ctx, `SELECT favourite, note, plays, last_played, cheats FROM wittyplay_game_meta WHERE user_id = ? AND item_id = ?`, userID, itemID).
		Scan(&fav, &m.Note, &m.Plays, &m.LastPlayed, &cheats)
	if errors.Is(err, sql.ErrNoRows) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	m.Favourite = fav != 0
	if cheats != "" {
		if json.Unmarshal([]byte(cheats), &m.Cheats) != nil {
			m.Cheats = []wittyplay.Cheat{}
		}
	}
	return m, nil
}

// SetGameFavourite marks a game or doesn't.
func (s *Store) SetGameFavourite(ctx context.Context, userID, itemID int64, on bool) error {
	v := 0
	if on {
		v = 1
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO wittyplay_game_meta(user_id, item_id, favourite) VALUES(?,?,?)
		ON CONFLICT(user_id, item_id) DO UPDATE SET favourite = excluded.favourite`, userID, itemID, v)
	return err
}

// SetGameNote keeps a person's own words about a game.
func (s *Store) SetGameNote(ctx context.Context, userID, itemID int64, note string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO wittyplay_game_meta(user_id, item_id, note) VALUES(?,?,?)
		ON CONFLICT(user_id, item_id) DO UPDATE SET note = excluded.note`, userID, itemID, note)
	return err
}

// SetGameCheats replaces a person's cheats for a game.
func (s *Store) SetGameCheats(ctx context.Context, userID, itemID int64, list []wittyplay.Cheat) error {
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO wittyplay_game_meta(user_id, item_id, cheats) VALUES(?,?,?)
		ON CONFLICT(user_id, item_id) DO UPDATE SET cheats = excluded.cheats`, userID, itemID, string(b))
	return err
}

// NoteGamePlayed counts a game being started.
func (s *Store) NoteGamePlayed(ctx context.Context, userID, itemID int64) error {
	now := db.Now()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO wittyplay_game_meta(user_id, item_id, plays, last_played) VALUES(?,?,1,?)
		ON CONFLICT(user_id, item_id) DO UPDATE SET plays = plays + 1, last_played = excluded.last_played`, userID, itemID, now)
	return err
}

// GameListEntry is one row of a person's recent or favourite games.
type GameListEntry struct {
	ItemID     int64
	Favourite  bool
	Plays      int
	LastPlayed int64
}

// GameList lists a person's most recently played games, or their favourites, newest
// first. Whether they can still see each one is the caller's to check.
func (s *Store) GameList(ctx context.Context, userID int64, favourites bool, limit int) ([]GameListEntry, error) {
	where := `last_played > 0`
	if favourites {
		where = `favourite != 0`
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT item_id, favourite, plays, last_played FROM wittyplay_game_meta WHERE user_id = ? AND `+where+` ORDER BY last_played DESC, item_id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GameListEntry{}
	for rows.Next() {
		var e GameListEntry
		var fav int
		if err := rows.Scan(&e.ItemID, &fav, &e.Plays, &e.LastPlayed); err != nil {
			return nil, err
		}
		e.Favourite = fav != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// CoreOptionList is what a core last said it offers, kept so that settings screens can
// show it with no game running.
func (s *Store) CoreOptionList(ctx context.Context, core string) ([]wittyplay.OptionInfo, bool) {
	var data string
	if err := s.DB.QueryRowContext(ctx, `SELECT data FROM wittyplay_core_options WHERE core = ?`, core).Scan(&data); err != nil {
		return nil, false
	}
	var list []wittyplay.OptionInfo
	if json.Unmarshal([]byte(data), &list) != nil {
		return nil, false
	}
	return list, true
}

// SetCoreOptionList keeps what a core offers.
func (s *Store) SetCoreOptionList(ctx context.Context, core string, list []wittyplay.OptionInfo) error {
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO wittyplay_core_options(core, data, updated_at) VALUES(?,?,?)
		ON CONFLICT(core) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`, core, string(b), db.Now())
	return err
}

// GameBezel is an image of a person's own for around the picture.
type GameBezel struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Size      int64  `json:"size"`
	CreatedAt int64  `json:"createdAt"`
}

// GameBezels lists a person's images.
func (s *Store) GameBezels(ctx context.Context, userID int64) ([]GameBezel, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, width, height, size, created_at FROM wittyplay_bezels WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GameBezel{}
	for rows.Next() {
		var b GameBezel
		if err := rows.Scan(&b.ID, &b.Name, &b.Width, &b.Height, &b.Size, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// AddGameBezel records one, and says its number.
func (s *Store) AddGameBezel(ctx context.Context, userID int64, b GameBezel) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO wittyplay_bezels(user_id, name, width, height, size, created_at) VALUES(?,?,?,?,?,?)`, userID, b.Name, b.Width, b.Height, b.Size, db.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// HasGameBezel says whether this person owns that image.
func (s *Store) HasGameBezel(ctx context.Context, userID, id int64) bool {
	var one int
	return s.DB.QueryRowContext(ctx, `SELECT 1 FROM wittyplay_bezels WHERE id = ? AND user_id = ?`, id, userID).Scan(&one) == nil
}

// RemoveGameBezel forgets one, and says whether it was theirs.
func (s *Store) RemoveGameBezel(ctx context.Context, userID, id int64) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM wittyplay_bezels WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// GamePatch is a patch file of a person's own for one game.
type GamePatch struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // ips | bps
	Size      int64  `json:"size"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"createdAt"`
}

// GamePatches lists a person's patches for a game, in the order they apply.
func (s *Store) GamePatches(ctx context.Context, userID, itemID int64) ([]GamePatch, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, kind, size, enabled, created_at FROM wittyplay_patches WHERE user_id = ? AND item_id = ? ORDER BY id`, userID, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GamePatch{}
	for rows.Next() {
		var p GamePatch
		var on int
		if err := rows.Scan(&p.ID, &p.Name, &p.Kind, &p.Size, &on, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Enabled = on != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

// AddGamePatch records one, turned on, and says its number.
func (s *Store) AddGamePatch(ctx context.Context, userID, itemID int64, p GamePatch) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO wittyplay_patches(user_id, item_id, name, kind, size, enabled, created_at) VALUES(?,?,?,?,?,1,?)`, userID, itemID, p.Name, p.Kind, p.Size, db.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SetGamePatchEnabled turns one on or off, and says whether it was theirs.
func (s *Store) SetGamePatchEnabled(ctx context.Context, userID, itemID, id int64, on bool) (bool, error) {
	v := 0
	if on {
		v = 1
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE wittyplay_patches SET enabled = ? WHERE id = ? AND user_id = ? AND item_id = ?`, v, id, userID, itemID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// RemoveGamePatch forgets one, and says whether it was theirs.
func (s *Store) RemoveGamePatch(ctx context.Context, userID, itemID, id int64) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM wittyplay_patches WHERE id = ? AND user_id = ? AND item_id = ?`, id, userID, itemID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
