package channels

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// What channels draw on beyond rules: the bumper library, the discs a scene can
// come from, and what each account keeps for itself (favourite channels and
// reminders).

// Bumper is a video in the bumper library: one uploaded for the purpose, one
// chosen from any library, or anything in a commercials library.
type Bumper struct {
	ItemID      int64   `json:"itemId"`
	Title       string  `json:"title"`
	DurationSec float64 `json:"durationSec"`
	Library     string  `json:"library"`
	Rating      string  `json:"rating,omitempty"`
	Uploaded    bool    `json:"uploaded"`
	// Chosen: picked from another library, so removing it only takes it off this
	// list. Neither this nor Uploaded means it comes with a commercials library.
	Chosen    bool  `json:"chosen"`
	UpdatedAt int64 `json:"updatedAt"`
}

const maxBumperList = 2000

// Bumpers lists the library, the owner's own first, then by title.
func (s *Store) Bumpers(ctx context.Context) ([]Bumper, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id, i.title, IFNULL(i.runtime_sec, 0), l.name, IFNULL(i.content_rating, ''), IFNULL(b.uploaded, 0), b.item_id IS NOT NULL, i.updated_at
		FROM items i JOIN libraries l ON l.id = i.library_id LEFT JOIN channel_bumpers b ON b.item_id = i.id
		WHERE i.missing = 0 AND (b.item_id IS NOT NULL OR (l.type = ? AND i.kind = 'video'))
		ORDER BY b.item_id IS NULL, i.sort_title COLLATE NOCASE, i.id LIMIT ?`, library.CommercialsType, maxBumperList)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Bumper{}
	for rows.Next() {
		var b Bumper
		var uploaded, chosen int
		if err := rows.Scan(&b.ItemID, &b.Title, &b.DurationSec, &b.Library, &b.Rating, &uploaded, &chosen, &b.UpdatedAt); err != nil {
			return nil, err
		}
		b.Uploaded, b.Chosen = uploaded != 0, chosen != 0 && uploaded == 0
		out = append(out, b)
	}
	return out, rows.Err()
}

// AddBumper puts a video into the library.
func (s *Store) AddBumper(ctx context.Context, itemID int64, uploaded bool) error {
	_, err := s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO channel_bumpers(item_id, uploaded, added_at) VALUES(?,?,?)`, itemID, boolInt(uploaded), db.Now())
	return err
}

// ErrBundledBumper: it belongs to a commercials library, so it is managed there.
var ErrBundledBumper = errors.New("this video comes from a commercials library: remove it there")

// MoveBumperFolder points the uploads' library, and the videos in it, at a new
// folder. It returns how many videos moved: none, usually.
func (s *Store) MoveBumperFolder(ctx context.Context, libraryID int64, dir string) (int64, error) {
	var old string
	err := s.DB.QueryRowContext(ctx, `SELECT path FROM library_paths WHERE library_id = ? LIMIT 1`, libraryID).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil || old == dir {
		return 0, err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE library_paths SET path = ? WHERE library_id = ? AND path = ?`, dir, libraryID, old); err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE items SET path = ? || substr(path, ?) WHERE library_id = ? AND substr(path, 1, ?) = ?`, dir, len(old)+1, libraryID, len(old), old)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RemoveBumper takes a video off the list. For an upload it says so, and the
// caller removes the file and the item too.
func (s *Store) RemoveBumper(ctx context.Context, itemID int64) (uploaded bool, err error) {
	var up int
	err = s.DB.QueryRowContext(ctx, `SELECT uploaded FROM channel_bumpers WHERE item_id = ?`, itemID).Scan(&up)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrBundledBumper
	}
	if err != nil {
		return false, err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM channel_bumpers WHERE item_id = ?`, itemID); err != nil {
		return false, err
	}
	return up != 0, nil
}

// DiscOption is a disc in the scene picker. Encrypted discs are listed too, so
// that the picker can say why they can't be used.
type DiscOption struct {
	ID        int64   `json:"id"`
	Title     string  `json:"title"`
	Year      int     `json:"year,omitempty"`
	Format    string  `json:"format"` // dvd | bluray
	Scrambled bool    `json:"scrambled"`
	MainTitle int     `json:"mainTitle"`
	Runtime   float64 `json:"runtimeSec"`
	UpdatedAt int64   `json:"updatedAt"`
	Titles    int     `json:"titles"`
}

// Discs lists every disc in the libraries.
func (s *Store) Discs(ctx context.Context) ([]DiscOption, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id, i.title, IFNULL(i.year, 0), i.kind, i.disc_json, IFNULL(i.runtime_sec, 0), i.updated_at FROM items i
		WHERE i.missing = 0 AND i.kind IN ('dvd-iso', 'dvd-folder', 'bluray-iso', 'bluray-folder') ORDER BY i.sort_title COLLATE NOCASE, i.id LIMIT 5000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DiscOption{}
	for rows.Next() {
		var d DiscOption
		var kind string
		var raw sql.NullString
		if err := rows.Scan(&d.ID, &d.Title, &d.Year, &kind, &raw, &d.Runtime, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.Format = "dvd"
		if kind == "bluray-iso" || kind == "bluray-folder" {
			d.Format = "bluray"
		}
		var info library.DiscInfo
		if raw.Valid && json.Unmarshal([]byte(raw.String), &info) == nil {
			d.Scrambled, d.MainTitle, d.Titles = info.Scrambled, info.MainTitle, len(info.Titles)
			if t := discLength(raw.String, 0); t > 0 {
				d.Runtime = t
			}
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CollectionOption is a hand-made collection, for rules.
type CollectionOption struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Items int    `json:"items"`
}

func (s *Store) collectionOptions(ctx context.Context) ([]CollectionOption, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id, c.name, (SELECT COUNT(*) FROM collection_items x WHERE x.collection_id = c.id) + (SELECT COUNT(*) FROM collection_shows x WHERE x.collection_id = c.id)
		FROM collections c WHERE c.kind = 'manual' ORDER BY c.name COLLATE NOCASE LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CollectionOption{}
	for rows.Next() {
		var c CollectionOption
		if err := rows.Scan(&c.ID, &c.Name, &c.Items); err != nil {
			return nil, err
		}
		if c.Items > 0 {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// One account's own: favourites and reminders
// ---------------------------------------------------------------------------

// Favourites is the set of channels an account has starred.
func (s *Store) Favourites(ctx context.Context, userID int64) (map[string]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT channel FROM tv_favourites WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}

const maxFavourites = 5000

// SetFavourite stars or unstars a channel.
func (s *Store) SetFavourite(ctx context.Context, userID int64, key string, on bool) error {
	if !on {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM tv_favourites WHERE user_id = ? AND channel = ?`, userID, key)
		return err
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM tv_favourites WHERE user_id = ?`, userID).Scan(&n); err != nil {
		return err
	}
	if n >= maxFavourites {
		return errors.New("that's as many favourite channels as there can be")
	}
	_, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO tv_favourites(user_id, channel, added_at) VALUES(?,?,?)`, userID, key, db.Now())
	return err
}

// Reminder is a programme an account wants to be told about.
type Reminder struct {
	ID      int64  `json:"id"`
	Channel string `json:"channel"`
	Title   string `json:"title"`
	Sub     string `json:"sub,omitempty"`
	Start   int64  `json:"start"`
	End     int64  `json:"end"`
}

const maxReminders = 200

// Reminders lists what is still to come, and forgets what has passed.
func (s *Store) Reminders(ctx context.Context, userID, now int64) ([]Reminder, error) {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM tv_reminders WHERE user_id = ? AND stop <= ?`, userID, now); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, channel, title, sub, start, stop FROM tv_reminders WHERE user_id = ? ORDER BY start LIMIT ?`, userID, maxReminders)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Reminder{}
	for rows.Next() {
		var r Reminder
		if err := rows.Scan(&r.ID, &r.Channel, &r.Title, &r.Sub, &r.Start, &r.End); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddReminder saves one. The same programme twice is one reminder.
func (s *Store) AddReminder(ctx context.Context, userID int64, r *Reminder) error {
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM tv_reminders WHERE user_id = ?`, userID).Scan(&n); err != nil {
		return err
	}
	if n >= maxReminders {
		return errors.New("that's as many reminders as there can be: remove some first")
	}
	_, err := s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO tv_reminders(user_id, channel, title, sub, start, stop) VALUES(?,?,?,?,?,?)`, userID, r.Channel, r.Title, r.Sub, r.Start, r.End)
	if err != nil {
		return err
	}
	return s.DB.QueryRowContext(ctx, `SELECT id FROM tv_reminders WHERE user_id = ? AND channel = ? AND start = ?`, userID, r.Channel, r.Start).Scan(&r.ID)
}

// RemoveReminder deletes one of this account's reminders.
func (s *Store) RemoveReminder(ctx context.Context, userID, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM tv_reminders WHERE user_id = ? AND id = ?`, userID, id)
	return err
}
