package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittyplay"
)

// A person's controller profiles and the background of their controller screen. Every
// query here is by user: there is no way to ask for someone else's.

// ErrTooManyProfiles: this account has saved as many profiles as it may.
var ErrTooManyProfiles = errors.New("you have as many controller profiles as an account can keep; delete one first")

// ErrNoProfile: no such profile, for this account.
var ErrNoProfile = errors.New("no such profile")

// profileBody is what's kept as JSON; the scope has columns, so it can be searched.
type profileBody struct {
	ControllerName string                     `json:"controllerName,omitempty"`
	Map            map[int]*wittyplay.Binding `json:"map,omitempty"`
	Keys           map[int]string             `json:"keys,omitempty"`
	Options        wittyplay.ProfileOptions   `json:"options"`
}

const profileCols = `p.id, p.name, p.kind, p.controller, p.console, p.game_id, p.raw, p.active, p.data, p.updated_at, COALESCE(i.title, '')`
const profileFrom = ` FROM wittyplay_profiles p LEFT JOIN items i ON i.id = p.game_id AND p.game_id != 0 `

func scanProfile(r interface{ Scan(...any) error }) (wittyplay.Profile, error) {
	var p wittyplay.Profile
	var data string
	var raw, active int
	if err := r.Scan(&p.ID, &p.Name, &p.Kind, &p.Controller, &p.Console, &p.Game, &raw, &active, &data, &p.UpdatedAt, &p.GameTitle); err != nil {
		return p, err
	}
	p.Raw, p.Active = raw != 0, active != 0
	var b profileBody
	if err := json.Unmarshal([]byte(data), &b); err != nil {
		return p, err
	}
	p.ControllerName, p.Map, p.Keys, p.Options = b.ControllerName, b.Map, b.Keys, b.Options
	return p, nil
}

// GameProfiles lists a person's profiles, most recently changed first.
func (s *Store) GameProfiles(ctx context.Context, userID int64) ([]wittyplay.Profile, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+profileCols+profileFrom+`WHERE p.user_id = ? ORDER BY p.updated_at DESC, p.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []wittyplay.Profile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GameProfile finds one of a person's profiles.
func (s *Store) GameProfile(ctx context.Context, userID, id int64) (wittyplay.Profile, error) {
	p, err := scanProfile(s.DB.QueryRowContext(ctx, `SELECT `+profileCols+profileFrom+`WHERE p.user_id = ? AND p.id = ?`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNoProfile
	}
	return p, err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// SaveGameProfile adds a profile, or replaces one of this person's when p.ID is set. A
// profile made active takes over from any other with the same scope.
func (s *Store) SaveGameProfile(ctx context.Context, userID int64, p wittyplay.Profile) (wittyplay.Profile, error) {
	data, err := json.Marshal(profileBody{ControllerName: p.ControllerName, Map: p.Map, Keys: p.Keys, Options: p.Options})
	if err != nil {
		return p, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	now := db.Now()
	if p.ID == 0 {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM wittyplay_profiles WHERE user_id = ?`, userID).Scan(&n); err != nil {
			return p, err
		}
		if n >= wittyplay.MaxProfiles {
			return p, ErrTooManyProfiles
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO wittyplay_profiles(user_id, name, kind, controller, console, game_id, raw, active, data, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			userID, p.Name, p.Kind, p.Controller, p.Console, p.Game, boolInt(p.Raw), boolInt(p.Active), string(data), now)
		if err != nil {
			return p, err
		}
		if p.ID, err = res.LastInsertId(); err != nil {
			return p, err
		}
	} else {
		res, err := tx.ExecContext(ctx, `UPDATE wittyplay_profiles SET name = ?, kind = ?, controller = ?, console = ?, game_id = ?, raw = ?, active = ?, data = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
			p.Name, p.Kind, p.Controller, p.Console, p.Game, boolInt(p.Raw), boolInt(p.Active), string(data), now, userID, p.ID)
		if err != nil {
			return p, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return p, ErrNoProfile
		}
	}
	if p.Active {
		if _, err := tx.ExecContext(ctx, `UPDATE wittyplay_profiles SET active = 0 WHERE user_id = ? AND kind = ? AND controller = ? AND console = ? AND game_id = ? AND id != ?`,
			userID, p.Kind, p.Controller, p.Console, p.Game, p.ID); err != nil {
			return p, err
		}
	}
	if err := tx.Commit(); err != nil {
		return p, err
	}
	return s.GameProfile(ctx, userID, p.ID)
}

// DeleteGameProfile removes one of a person's profiles.
func (s *Store) DeleteGameProfile(ctx context.Context, userID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM wittyplay_profiles WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoProfile
	}
	return nil
}

// GameBackground is how a person's picture sits behind their controller screen.
type GameBackground struct {
	Version int64  `json:"version"` // 0 when there's no picture; changes with each new one
	Fit     string `json:"fit"`     // cover | contain
	X       int    `json:"x"`       // which part shows, 0 to 100 across
	Y       int    `json:"y"`       // and down
	Dim     int    `json:"dim"`     // how far it's faded towards the theme's background, 0 to 90
}

// DefaultGameBackground is a picture's first setting: dimmed enough for most pictures.
func DefaultGameBackground() GameBackground {
	return GameBackground{Fit: "cover", X: 50, Y: 50, Dim: 50}
}

// GetGameBackground returns a person's setting, or the default.
func (s *Store) GetGameBackground(ctx context.Context, userID int64) (GameBackground, error) {
	b := DefaultGameBackground()
	err := s.DB.QueryRowContext(ctx, `SELECT version, fit, x, y, dim FROM wittyplay_backgrounds WHERE user_id = ?`, userID).Scan(&b.Version, &b.Fit, &b.X, &b.Y, &b.Dim)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultGameBackground(), nil
	}
	return b, err
}

// SetGameBackground saves it.
func (s *Store) SetGameBackground(ctx context.Context, userID int64, b GameBackground) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO wittyplay_backgrounds(user_id, version, fit, x, y, dim) VALUES(?,?,?,?,?,?)
		ON CONFLICT(user_id) DO UPDATE SET version = excluded.version, fit = excluded.fit, x = excluded.x, y = excluded.y, dim = excluded.dim`,
		userID, b.Version, b.Fit, b.X, b.Y, b.Dim)
	return err
}

// RemoveGameBackground forgets it.
func (s *Store) RemoveGameBackground(ctx context.Context, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM wittyplay_backgrounds WHERE user_id = ?`, userID)
	return err
}
