// Package db wraps the SQLite store (pure Go driver, no CGO) and migrations.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func Open(dataDir string) (*DB, error) {
	// The path is escaped: "file:" names are URIs, so a folder with # or % in
	// its name would otherwise open a different file.
	name := strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23").Replace(filepath.ToSlash(filepath.Join(dataDir, "wms.db")))
	// Every transaction here writes, so each takes the write lock when it begins.
	// One that reads first and writes later fails at once, without waiting, if
	// another write lands in between: busy_timeout can't help it.
	dsn := fmt.Sprintf("file:%s?_txlock=immediate&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", name)
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// WAL mode lets readers run alongside the single writer; busy_timeout
	// serialises writers. Keep the pool small but never 1: nested queries
	// (a row cursor plus a lookup) would deadlock on a single connection.
	sdb.SetMaxOpenConns(6)
	sdb.SetMaxIdleConns(3) // opening one means reading the schema again
	sdb.SetConnMaxLifetime(0)
	d := &DB{sdb}
	if err := d.migrate(context.Background()); err != nil {
		sdb.Close()
		return nil, err
	}
	// The database holds password hashes, API keys and sessions: others on a
	// shared machine have no business reading it (no effect on Windows, where
	// the folder's permissions decide).
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Chmod(filepath.Join(dataDir, "wms.db"+suffix), 0o600)
	}
	if err := d.migrateWittyTV(context.Background()); err != nil {
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
	// 10: WittyPhotos: what EXIF says about each picture (photos are items),
	// and albums. Times are the clock on the wall where the picture was taken,
	// stored as if it were UTC, so a photo stays in its month wherever it is viewed.
	`CREATE TABLE IF NOT EXISTS photo_meta (
	   item_id INTEGER PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
	   library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
	   taken_at INTEGER NOT NULL, ym INTEGER NOT NULL, folder TEXT NOT NULL DEFAULT '',
	   make TEXT, model TEXT, lens TEXT, fnumber REAL, exposure TEXT, iso INTEGER, focal REAL,
	   orientation INTEGER NOT NULL DEFAULT 0, lat REAL, lon REAL, alt REAL,
	   raw INTEGER NOT NULL DEFAULT 0, motion TEXT);
	 CREATE INDEX IF NOT EXISTS photo_time ON photo_meta(library_id, taken_at DESC, item_id DESC);
	 CREATE INDEX IF NOT EXISTS photo_ym ON photo_meta(library_id, ym);
	 CREATE INDEX IF NOT EXISTS photo_geo ON photo_meta(library_id, lat, lon) WHERE lat IS NOT NULL;
	 CREATE TABLE IF NOT EXISTS photo_albums (
	   id INTEGER PRIMARY KEY, library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
	   owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
	   cover_item INTEGER REFERENCES items(id) ON DELETE SET NULL, shared INTEGER NOT NULL DEFAULT 0,
	   created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
	 CREATE INDEX IF NOT EXISTS photo_albums_lib ON photo_albums(library_id, name);
	 CREATE TABLE IF NOT EXISTS photo_album_items (
	   album_id INTEGER NOT NULL REFERENCES photo_albums(id) ON DELETE CASCADE,
	   item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   added_at INTEGER NOT NULL, PRIMARY KEY(album_id, item_id));
	 CREATE INDEX IF NOT EXISTS photo_album_items_item ON photo_album_items(item_id);`,
	// 11: what viewers do with items, per account: likes and favourites,
	// comments, what was watched and for how long, and links that share an item
	// or album with someone without an account (only a hash of the link is kept).
	`CREATE TABLE IF NOT EXISTS item_reactions (
	   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   value INTEGER NOT NULL, created_at INTEGER NOT NULL, PRIMARY KEY(user_id, item_id));
	 CREATE INDEX IF NOT EXISTS item_reactions_item ON item_reactions(item_id, value);
	 CREATE TABLE IF NOT EXISTS item_comments (
	   id INTEGER PRIMARY KEY, item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   user_id INTEGER REFERENCES users(id) ON DELETE SET NULL, parent_id INTEGER REFERENCES item_comments(id) ON DELETE CASCADE,
	   author TEXT NOT NULL DEFAULT '', body TEXT NOT NULL, imported INTEGER NOT NULL DEFAULT 0,
	   likes INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL, edited_at INTEGER);
	 CREATE INDEX IF NOT EXISTS item_comments_item ON item_comments(item_id, created_at);
	 CREATE TABLE IF NOT EXISTS item_stats (
	   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   plays INTEGER NOT NULL DEFAULT 0, seconds REAL NOT NULL DEFAULT 0, completes INTEGER NOT NULL DEFAULT 0,
	   skips INTEGER NOT NULL DEFAULT 0, last_at INTEGER NOT NULL, PRIMARY KEY(user_id, item_id));
	 CREATE INDEX IF NOT EXISTS item_stats_item ON item_stats(item_id);
	 CREATE INDEX IF NOT EXISTS item_stats_user ON item_stats(user_id, last_at DESC);
	 CREATE TABLE IF NOT EXISTS play_log (
	   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   day INTEGER NOT NULL, seconds REAL NOT NULL, PRIMARY KEY(user_id, item_id, day));
	 CREATE INDEX IF NOT EXISTS play_log_user_day ON play_log(user_id, day);
	 CREATE TABLE IF NOT EXISTS share_links (
	   id INTEGER PRIMARY KEY, token_hash TEXT NOT NULL UNIQUE,
	   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   kind TEXT NOT NULL, target_id INTEGER NOT NULL, title TEXT NOT NULL,
	   allow_download INTEGER NOT NULL DEFAULT 0, expires_at INTEGER, views INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL);
	 CREATE INDEX IF NOT EXISTS share_links_user ON share_links(user_id, created_at DESC);`,
	// 12: WittyShorts and WittyChannels: creators, what a creator video has on
	// top of an item, and who follows whom.
	`CREATE TABLE IF NOT EXISTS creators (
	   id INTEGER PRIMARY KEY, library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
	   key TEXT NOT NULL, name TEXT NOT NULL, handle TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '',
	   avatar TEXT NOT NULL DEFAULT '', banner TEXT NOT NULL DEFAULT '', platform TEXT NOT NULL DEFAULT '', url TEXT NOT NULL DEFAULT '',
	   folder TEXT NOT NULL DEFAULT '', added_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, UNIQUE(library_id, key));
	 CREATE TABLE IF NOT EXISTS video_meta (
	   item_id INTEGER PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
	   library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
	   creator_id INTEGER REFERENCES creators(id) ON DELETE SET NULL,
	   platform TEXT NOT NULL DEFAULT '', source_url TEXT NOT NULL DEFAULT '', upload_date TEXT NOT NULL DEFAULT '', tags TEXT NOT NULL DEFAULT '',
	   orig_views INTEGER NOT NULL DEFAULT 0, orig_likes INTEGER NOT NULL DEFAULT 0, vertical INTEGER NOT NULL DEFAULT 0,
	   playlist TEXT NOT NULL DEFAULT '', custom TEXT NOT NULL DEFAULT '', edited INTEGER NOT NULL DEFAULT 0);
	 CREATE INDEX IF NOT EXISTS video_meta_lib ON video_meta(library_id, upload_date DESC);
	 CREATE INDEX IF NOT EXISTS video_meta_creator ON video_meta(creator_id, upload_date DESC);
	 CREATE TABLE IF NOT EXISTS creator_subs (
	   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   creator_id INTEGER NOT NULL REFERENCES creators(id) ON DELETE CASCADE,
	   created_at INTEGER NOT NULL, PRIMARY KEY(user_id, creator_id));`,
	// 13: collections and smart playlists, across libraries.
	`CREATE TABLE IF NOT EXISTS collections (
	   id INTEGER PRIMARY KEY, owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', kind TEXT NOT NULL DEFAULT 'manual', rule TEXT NOT NULL DEFAULT '',
	   shared INTEGER NOT NULL DEFAULT 0, system TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
	 CREATE INDEX IF NOT EXISTS collections_owner ON collections(owner_id, name);
	 CREATE TABLE IF NOT EXISTS collection_items (
	   collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
	   item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   added_at INTEGER NOT NULL, PRIMARY KEY(collection_id, item_id));
	 CREATE TABLE IF NOT EXISTS collection_shows (
	   collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
	   show_id INTEGER NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
	   added_at INTEGER NOT NULL, PRIMARY KEY(collection_id, show_id));`,
	// 14: indexes for what the main pages ask: pages of a library by title
	// (NOCASE, as they sort) or newest first, library and show totals without
	// reading each row, and removing items without scanning all progress.
	`DROP INDEX IF EXISTS items_lib;
	 DROP INDEX IF EXISTS items_show;
	 CREATE INDEX IF NOT EXISTS items_lib_title ON items(library_id, sort_title COLLATE NOCASE);
	 CREATE INDEX IF NOT EXISTS items_lib_added ON items(library_id, added_at);
	 CREATE INDEX IF NOT EXISTS items_lib_missing ON items(library_id, missing);
	 CREATE INDEX IF NOT EXISTS items_added ON items(added_at);
	 CREATE INDEX IF NOT EXISTS items_show_totals ON items(show_id, missing, season, added_at);
	 CREATE INDEX IF NOT EXISTS progress_item ON progress(item_id);
	 CREATE INDEX IF NOT EXISTS progress_recent ON progress(user_id, watched, updated_at);
	 CREATE INDEX IF NOT EXISTS annotations_item ON annotations(item_id);`,
	// 15: remote access: two-factor sign-in, whether a password is weak
	// (NULL = not known yet), and each session's latest address and use.
	`ALTER TABLE users ADD COLUMN password_weak INTEGER;
	 ALTER TABLE users ADD COLUMN totp_secret TEXT;
	 ALTER TABLE users ADD COLUMN totp_on INTEGER NOT NULL DEFAULT 0;
	 ALTER TABLE users ADD COLUMN totp_last INTEGER NOT NULL DEFAULT 0;
	 ALTER TABLE users ADD COLUMN recovery_codes TEXT;
	 ALTER TABLE sessions ADD COLUMN last_seen INTEGER;
	 ALTER TABLE sessions ADD COLUMN last_ip TEXT;
	 ALTER TABLE sessions ADD COLUMN last_where TEXT;
	 ALTER TABLE sessions ADD COLUMN last_via TEXT;`,
	// 16: games: which console a library holds (or auto), and each game's console.
	`ALTER TABLE libraries ADD COLUMN console TEXT;
	 ALTER TABLE items ADD COLUMN console TEXT;
	 CREATE TABLE IF NOT EXISTS wittyplay_controls (
	   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   console TEXT NOT NULL, kind TEXT NOT NULL, key TEXT NOT NULL,
	   data TEXT NOT NULL, updated_at INTEGER NOT NULL,
	   PRIMARY KEY(user_id, console, kind, key));`,
	// What's inside each archive in a games library, so a scan reads one only once.
	`CREATE TABLE IF NOT EXISTS game_archives (
	   library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
	   path TEXT NOT NULL, size INTEGER NOT NULL, mtime INTEGER NOT NULL,
	   exts TEXT NOT NULL, problem TEXT NOT NULL,
	   PRIMARY KEY(library_id, path));`,
	// 18: what's known about games. game_meta is the result, and the queue: a row with
	// status pending is still to be looked up, so a restart carries on where it stopped.
	// game_meta_sources keeps each source's answer, so changing the order of sources
	// asks nobody anything, and adding one only asks that one.
	`CREATE TABLE IF NOT EXISTS game_meta (
	   item_id INTEGER PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
	   library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
	   status TEXT NOT NULL DEFAULT 'pending', next_try INTEGER NOT NULL DEFAULT 0, tries INTEGER NOT NULL DEFAULT 0,
	   locked INTEGER NOT NULL DEFAULT 0, edited TEXT NOT NULL DEFAULT '', applied TEXT NOT NULL DEFAULT '',
	   file_size INTEGER NOT NULL DEFAULT -1, file_mtime INTEGER NOT NULL DEFAULT -1, crcs TEXT NOT NULL DEFAULT '', crc_size INTEGER NOT NULL DEFAULT 0, serial TEXT NOT NULL DEFAULT '',
	   query_title TEXT NOT NULL DEFAULT '', tags TEXT NOT NULL DEFAULT '',
	   match_source TEXT NOT NULL DEFAULT '', match_id TEXT NOT NULL DEFAULT '', match_method TEXT NOT NULL DEFAULT '', confidence INTEGER NOT NULL DEFAULT 0,
	   released TEXT NOT NULL DEFAULT '', developer TEXT NOT NULL DEFAULT '', publisher TEXT NOT NULL DEFAULT '', franchise TEXT NOT NULL DEFAULT '', players INTEGER NOT NULL DEFAULT 0,
	   scores TEXT NOT NULL DEFAULT '', ages TEXT NOT NULL DEFAULT '', art TEXT NOT NULL DEFAULT '', credits TEXT NOT NULL DEFAULT '',
	   poster_refs TEXT NOT NULL DEFAULT '', backdrop_refs TEXT NOT NULL DEFAULT '', poster_ref TEXT NOT NULL DEFAULT '', backdrop_ref TEXT NOT NULL DEFAULT '', art_wanted INTEGER NOT NULL DEFAULT 0, art_tries INTEGER NOT NULL DEFAULT 0, art_next INTEGER NOT NULL DEFAULT 0,
	   fetched_at INTEGER NOT NULL DEFAULT 0);
	 CREATE INDEX IF NOT EXISTS game_meta_queue ON game_meta(status, next_try);
	 CREATE INDEX IF NOT EXISTS game_meta_art ON game_meta(art_wanted, art_next);
	 CREATE INDEX IF NOT EXISTS game_meta_lib ON game_meta(library_id, players);
	 CREATE TABLE IF NOT EXISTS game_meta_sources (
	   item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   source TEXT NOT NULL, source_id TEXT NOT NULL DEFAULT '', method TEXT NOT NULL DEFAULT '', confidence INTEGER NOT NULL DEFAULT 0,
	   data TEXT NOT NULL DEFAULT '', fetched_at INTEGER NOT NULL,
	   PRIMARY KEY(item_id, source));`,
	// 17: internet radio: what is known about a station beyond its item, where it
	// came from, and whether it is still on the air.
	`CREATE TABLE IF NOT EXISTS radio_meta (
	   item_id INTEGER PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
	   source TEXT NOT NULL DEFAULT '', source_id TEXT NOT NULL DEFAULT '',
	   homepage TEXT NOT NULL DEFAULT '', country TEXT NOT NULL DEFAULT '', country_code TEXT NOT NULL DEFAULT '',
	   language TEXT NOT NULL DEFAULT '', codec TEXT NOT NULL DEFAULT '', bitrate INTEGER NOT NULL DEFAULT 0, hls INTEGER NOT NULL DEFAULT 0,
	   dir_ok INTEGER NOT NULL DEFAULT -1, dir_gone INTEGER NOT NULL DEFAULT 0, dir_checked INTEGER NOT NULL DEFAULT 0,
	   fail_count INTEGER NOT NULL DEFAULT 0, last_ok INTEGER NOT NULL DEFAULT 0, last_check INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '');
	 CREATE INDEX IF NOT EXISTS radio_meta_source ON radio_meta(source, dir_checked);`,
	// 17: games: named controller profiles, and the picture behind the controller screen.
	`CREATE TABLE wittyplay_profiles (
	   id INTEGER PRIMARY KEY AUTOINCREMENT,
	   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   name TEXT NOT NULL, kind TEXT NOT NULL, controller TEXT NOT NULL, console TEXT NOT NULL,
	   game_id INTEGER NOT NULL, raw INTEGER NOT NULL, active INTEGER NOT NULL,
	   data TEXT NOT NULL, updated_at INTEGER NOT NULL);
	 CREATE INDEX idx_wittyplay_profiles_user ON wittyplay_profiles(user_id);
	 CREATE TABLE wittyplay_backgrounds (
	   user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	   version INTEGER NOT NULL, fit TEXT NOT NULL, x INTEGER NOT NULL, y INTEGER NOT NULL, dim INTEGER NOT NULL);`,
	// 17: games: each person's settings, what they keep about each game, and their own files.
	`CREATE TABLE wittyplay_settings (
	   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   console TEXT NOT NULL, item_id INTEGER NOT NULL,
	   data TEXT NOT NULL, updated_at INTEGER NOT NULL,
	   PRIMARY KEY(user_id, console, item_id));
	 CREATE TABLE wittyplay_game_meta (
	   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   favourite INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '',
	   plays INTEGER NOT NULL DEFAULT 0, last_played INTEGER NOT NULL DEFAULT 0,
	   cheats TEXT NOT NULL DEFAULT '',
	   PRIMARY KEY(user_id, item_id));
	 CREATE INDEX idx_wittyplay_meta_recent ON wittyplay_game_meta(user_id, last_played);
	 CREATE TABLE wittyplay_core_options (
	   core TEXT PRIMARY KEY, data TEXT NOT NULL, updated_at INTEGER NOT NULL);
	 CREATE TABLE wittyplay_bezels (
	   id INTEGER PRIMARY KEY AUTOINCREMENT,
	   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   name TEXT NOT NULL, width INTEGER NOT NULL, height INTEGER NOT NULL, size INTEGER NOT NULL, created_at INTEGER NOT NULL);
	 CREATE INDEX idx_wittyplay_bezels_user ON wittyplay_bezels(user_id);
	 CREATE TABLE wittyplay_patches (
	   id INTEGER PRIMARY KEY AUTOINCREMENT,
	   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   name TEXT NOT NULL, kind TEXT NOT NULL, size INTEGER NOT NULL, enabled INTEGER NOT NULL, created_at INTEGER NOT NULL);
	 CREATE INDEX idx_wittyplay_patches_game ON wittyplay_patches(user_id, item_id);`,
	// 17: custom game cores and consoles: what was approved, the versions kept, which
	// core a game uses, how play went, and an audit log.
	`CREATE TABLE IF NOT EXISTS custom_cores (
	   id TEXT PRIMARY KEY, name TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', sha256 TEXT NOT NULL, size INTEGER NOT NULL DEFAULT 0,
	   license TEXT NOT NULL DEFAULT '', license_source TEXT NOT NULL DEFAULT '', non_commercial INTEGER NOT NULL DEFAULT 0,
	   consoles TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '', source_url TEXT NOT NULL DEFAULT '',
	   added_by TEXT NOT NULL DEFAULT '', added_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, report TEXT NOT NULL DEFAULT '{}', system_files TEXT NOT NULL DEFAULT '[]');
	 CREATE TABLE IF NOT EXISTS custom_core_versions (
	   core_id TEXT NOT NULL, sha256 TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', added_at INTEGER NOT NULL,
	   PRIMARY KEY(core_id, sha256));
	 CREATE TABLE IF NOT EXISTS custom_consoles (
	   id TEXT PRIMARY KEY, data TEXT NOT NULL, aliases TEXT NOT NULL DEFAULT '[]', cores TEXT NOT NULL DEFAULT '[]',
	   created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
	 CREATE TABLE IF NOT EXISTS game_core_choice (
	   item_id INTEGER PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE, core_id TEXT NOT NULL);
	 CREATE TABLE IF NOT EXISTS game_core_plays (
	   core_id TEXT NOT NULL, item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   starts INTEGER NOT NULL DEFAULT 0, failures INTEGER NOT NULL DEFAULT 0, seconds REAL NOT NULL DEFAULT 0,
	   last_error TEXT NOT NULL DEFAULT '', last_at INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(core_id, item_id));
	 CREATE TABLE IF NOT EXISTS game_core_audit (
	   id INTEGER PRIMARY KEY AUTOINCREMENT, at INTEGER NOT NULL, user_name TEXT NOT NULL DEFAULT '', action TEXT NOT NULL,
	   core TEXT NOT NULL DEFAULT '', detail TEXT NOT NULL DEFAULT '');`,
	// 17: CD images: a track item says which disc item it belongs to.
	`ALTER TABLE items ADD COLUMN cd_disc INTEGER;
	 CREATE INDEX IF NOT EXISTS items_cd_disc ON items(cd_disc) WHERE cd_disc IS NOT NULL;`,
	// 17: game rigs: gaming PCs running Sunshine, each with a library of its games. The
	// certificate here is the rig's own, which isn't secret: this server's key is in a file.
	`CREATE TABLE IF NOT EXISTS wittysunshine_rigs (
	   id INTEGER PRIMARY KEY,
	   library_id INTEGER NOT NULL UNIQUE REFERENCES libraries(id) ON DELETE CASCADE,
	   name TEXT NOT NULL, address TEXT NOT NULL, port INTEGER NOT NULL DEFAULT 0,
	   remote_address TEXT NOT NULL DEFAULT '', allow_public INTEGER NOT NULL DEFAULT 0,
	   uuid TEXT NOT NULL DEFAULT '', mac TEXT NOT NULL DEFAULT '', cert TEXT NOT NULL DEFAULT '',
	   paired_at INTEGER, created_at INTEGER NOT NULL);
	 CREATE TABLE IF NOT EXISTS wittysunshine_apps (
	   rig_id INTEGER NOT NULL REFERENCES wittysunshine_rigs(id) ON DELETE CASCADE,
	   app_id INTEGER NOT NULL, title TEXT NOT NULL, shown INTEGER NOT NULL,
	   item_id INTEGER REFERENCES items(id) ON DELETE SET NULL,
	   PRIMARY KEY(rig_id, app_id));
	 CREATE INDEX IF NOT EXISTS wittysunshine_apps_item ON wittysunshine_apps(item_id);`,
	// 18: WittyMoonlight: each person's own list of PCs running Sunshine. The certificate here is the
	// PC's own, which isn't secret: each person's key is in a file.
	`CREATE TABLE IF NOT EXISTS wittymoonlight_hosts (
	   id INTEGER PRIMARY KEY,
	   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   name TEXT NOT NULL, address TEXT NOT NULL, port INTEGER NOT NULL DEFAULT 0,
	   uuid TEXT NOT NULL, mac TEXT NOT NULL DEFAULT '', cert TEXT NOT NULL DEFAULT '',
	   public INTEGER NOT NULL DEFAULT 0, paired_at INTEGER, created_at INTEGER NOT NULL,
	   UNIQUE(user_id, uuid));`,
}

func (d *DB) migrate(ctx context.Context) error {
	var version int
	if err := d.QueryRowContext(ctx, `SELECT version FROM schema LIMIT 1`).Scan(&version); err != nil {
		// No schema table or row: a new database. Anything else (busy, damaged)
		// must not be mistaken for that.
		if !errors.Is(err, sql.ErrNoRows) && !strings.Contains(err.Error(), "no such table") {
			return fmt.Errorf("reading the schema version: %w", err)
		}
	}
	for i := version; i < len(migrations); i++ {
		if err := d.applyMigration(ctx, i); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	return nil
}

// applyMigration runs one step and records it in one transaction: a crash or
// a failure part-way leaves the database as it was, not half changed (ALTER
// TABLE can't be run twice, so a half-done step would stop every later start).
func (d *DB) applyMigration(ctx context.Context, i int) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM schema`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema(version) VALUES (?)`, i+1); err != nil {
		return err
	}
	return tx.Commit()
}

// Optimize lets SQLite refresh its query statistics; cheap, and done after a
// scan has changed a lot of rows.
func (d *DB) Optimize(ctx context.Context) {
	_, _ = d.ExecContext(ctx, `PRAGMA optimize`)
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
