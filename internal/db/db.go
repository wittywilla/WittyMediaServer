// Package db wraps the SQLite store (pure Go driver, no CGO) and migrations.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func Open(dataDir string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", filepath.ToSlash(filepath.Join(dataDir, "wms.db")))
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// WAL mode lets readers run alongside the single writer; busy_timeout
	// serialises writers. Keep the pool small but never 1: nested queries
	// (a row cursor plus a lookup) would deadlock on a single connection.
	sdb.SetMaxOpenConns(6)
	sdb.SetConnMaxLifetime(0)
	d := &DB{sdb}
	if err := d.migrate(context.Background()); err != nil {
		sdb.Close()
		return nil, err
	}
	return d, nil
}

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	 CREATE TABLE IF NOT EXISTS users (
	   id INTEGER PRIMARY KEY, name TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL,
	   is_admin INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL);
	 CREATE TABLE IF NOT EXISTS sessions (
	   token TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   created_at INTEGER NOT NULL, expires_at INTEGER NOT NULL, device TEXT);
	 CREATE TABLE IF NOT EXISTS libraries (
	   id INTEGER PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL, created_at INTEGER NOT NULL,
	   last_scan_at INTEGER, scanning INTEGER NOT NULL DEFAULT 0);
	 CREATE TABLE IF NOT EXISTS library_paths (
	   id INTEGER PRIMARY KEY, library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE, path TEXT NOT NULL);
	 CREATE TABLE IF NOT EXISTS items (
	   id INTEGER PRIMARY KEY,
	   library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
	   kind TEXT NOT NULL,             -- video | dvd-iso | dvd-folder
	   path TEXT NOT NULL,
	   title TEXT NOT NULL, sort_title TEXT NOT NULL, year INTEGER, overview TEXT,
	   tagline TEXT, genres TEXT, rating REAL, content_rating TEXT, studio TEXT, runtime_sec REAL,
	   poster TEXT, backdrop TEXT,
	   container TEXT, video_codec TEXT, width INTEGER, height INTEGER, audio_langs TEXT, sub_langs TEXT,
	   disc_json TEXT,                 -- cached DVD structure summary
	   provider TEXT, provider_id TEXT,
	   size_bytes INTEGER, mtime INTEGER,
	   added_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
	   missing INTEGER NOT NULL DEFAULT 0,
	   UNIQUE(library_id, path));
	 CREATE INDEX IF NOT EXISTS items_lib ON items(library_id, sort_title);
	 CREATE TABLE IF NOT EXISTS progress (
	   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   position_sec REAL NOT NULL, duration_sec REAL NOT NULL, watched INTEGER NOT NULL DEFAULT 0,
	   disc_title INTEGER, updated_at INTEGER NOT NULL, PRIMARY KEY(user_id, item_id));
	 CREATE TABLE IF NOT EXISTS schema (version INTEGER NOT NULL);`,
	// 2: TV libraries. Episodes stay rows in items (so playback, progress and
	// search keep working) and point at a show; seasons hold per-season art.
	`CREATE TABLE IF NOT EXISTS shows (
	   id INTEGER PRIMARY KEY,
	   library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
	   path TEXT NOT NULL,             -- show folder
	   title TEXT NOT NULL, sort_title TEXT NOT NULL, year INTEGER, overview TEXT,
	   genres TEXT, rating REAL, content_rating TEXT, network TEXT, status TEXT,
	   poster TEXT, backdrop TEXT, provider TEXT, provider_id TEXT,
	   added_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
	   missing INTEGER NOT NULL DEFAULT 0,
	   UNIQUE(library_id, path));
	 CREATE TABLE IF NOT EXISTS seasons (
	   show_id INTEGER NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
	   number INTEGER NOT NULL, title TEXT, overview TEXT, poster TEXT,
	   PRIMARY KEY(show_id, number));
	 ALTER TABLE items ADD COLUMN show_id INTEGER REFERENCES shows(id) ON DELETE SET NULL;
	 ALTER TABLE items ADD COLUMN season INTEGER;
	 ALTER TABLE items ADD COLUMN episode INTEGER;
	 ALTER TABLE items ADD COLUMN episode_end INTEGER;
	 ALTER TABLE items ADD COLUMN aired TEXT;
	 ALTER TABLE items ADD COLUMN play_mode TEXT;  -- DVDs: menu | title ('' = server default)
	 ALTER TABLE items ADD COLUMN insert_fx TEXT;  -- DVDs: on | off ('' = server default)
	 ALTER TABLE items ADD COLUMN intro_start REAL; -- from an "Intro"/"Opening" chapter
	 ALTER TABLE items ADD COLUMN intro_end REAL;
	 ALTER TABLE shows ADD COLUMN intro_start REAL; -- show-wide fallback set by the user
	 ALTER TABLE shows ADD COLUMN intro_end REAL;
	 ALTER TABLE shows ADD COLUMN view TEXT;        -- episode list: cards | list ('' = server default)
	 CREATE INDEX IF NOT EXISTS items_show ON items(show_id, season, episode);`,
	// 3: user names for disc titles and chapters (JSON, see library.DiscLabels).
	`ALTER TABLE items ADD COLUMN disc_labels TEXT;`,
	// 4: profiles (adult | kids | guest) with limits, and the minimum age of
	// every item and show (from its content rating) for those limits.
	`ALTER TABLE users ADD COLUMN kind TEXT NOT NULL DEFAULT 'adult';
	 ALTER TABLE users ADD COLUMN restrictions TEXT;
	 ALTER TABLE items ADD COLUMN min_age INTEGER;
	 ALTER TABLE shows ADD COLUMN min_age INTEGER;`,
	// 5: per-account interface choices (home widgets...), JSON.
	`ALTER TABLE users ADD COLUMN ui TEXT;`,
	// 6: music, audiobooks and podcasts: tag fields of audio files.
	`ALTER TABLE items ADD COLUMN artist TEXT;
	 ALTER TABLE items ADD COLUMN album TEXT;
	 ALTER TABLE items ADD COLUMN album_artist TEXT;
	 ALTER TABLE items ADD COLUMN track INTEGER;
	 ALTER TABLE items ADD COLUMN disc_no INTEGER;
	 CREATE INDEX IF NOT EXISTS items_album ON items(library_id, album_artist, album);`,
	// 7: podcast subscriptions (episodes are audio items).
	`CREATE TABLE IF NOT EXISTS podcasts (
	   id INTEGER PRIMARY KEY, library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
	   url TEXT NOT NULL, title TEXT NOT NULL, author TEXT, image TEXT, description TEXT,
	   last_checked INTEGER, auto_download INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL,
	   UNIQUE(library_id, url));`,
	// 8: books: reading direction (ltr | rtl | webtoon).
	`ALTER TABLE items ADD COLUMN reading TEXT;`,
	// 9: e-reader bookmarks, highlights, notes and ink (per account).
	`CREATE TABLE IF NOT EXISTS annotations (
	   id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   kind TEXT NOT NULL, chapter INTEGER NOT NULL, pos REAL NOT NULL,
	   quote TEXT NOT NULL DEFAULT '', prefix TEXT NOT NULL DEFAULT '', suffix TEXT NOT NULL DEFAULT '',
	   color TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', data TEXT NOT NULL DEFAULT '',
	   created_at INTEGER NOT NULL);
	 CREATE INDEX IF NOT EXISTS annotations_user_item ON annotations(user_id, item_id);`,
}

func (d *DB) migrate(ctx context.Context) error {
	var version int
	_ = d.QueryRowContext(ctx, `SELECT version FROM schema LIMIT 1`).Scan(&version)
	for i := version; i < len(migrations); i++ {
		if _, err := d.ExecContext(ctx, migrations[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	if version == 0 {
		if _, err := d.ExecContext(ctx, `INSERT INTO schema(version) VALUES (?)`, len(migrations)); err != nil {
			return err
		}
	} else if version < len(migrations) {
		if _, err := d.ExecContext(ctx, `UPDATE schema SET version = ?`, len(migrations)); err != nil {
			return err
		}
	}
	return nil
}

func Now() int64 { return time.Now().Unix() }

// Setting helpers.
func (d *DB) GetSetting(ctx context.Context, key, def string) string {
	var v string
	if err := d.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

func (d *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := d.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
