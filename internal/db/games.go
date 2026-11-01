package db

import (
	"context"
	"fmt"
	"strconv"
)

// The games catalogue owns these tables and keeps its own version (setting
// "schema.games"), as library metadata and WittyTV do.
var gameMigrations = []string{
	// 1: what each file in a games library turned out to be, so a scan reads a file once
	// until it changes. Anything a scan wouldn't list goes in game_review, with why.
	// game_decisions is what a person said about a file, and outlasts every scan.
	`CREATE TABLE IF NOT EXISTS game_files (
	   library_id INTEGER NOT NULL, path TEXT NOT NULL, size INTEGER NOT NULL, mtime INTEGER NOT NULL,
	   version INTEGER NOT NULL, verdict TEXT NOT NULL,
	   PRIMARY KEY(library_id, path)) WITHOUT ROWID;
	 CREATE TABLE IF NOT EXISTS game_review (
	   id INTEGER PRIMARY KEY, library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
	   path TEXT NOT NULL, size INTEGER NOT NULL DEFAULT 0, kind TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '',
	   suggest TEXT NOT NULL DEFAULT '[]', console TEXT NOT NULL DEFAULT '', detail TEXT NOT NULL DEFAULT '',
	   UNIQUE(library_id, path));
	 CREATE INDEX IF NOT EXISTS game_review_kind ON game_review(library_id, kind);
	 CREATE TABLE IF NOT EXISTS game_decisions (
	   library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE, path TEXT NOT NULL,
	   action TEXT NOT NULL, console TEXT NOT NULL DEFAULT '', at INTEGER NOT NULL,
	   PRIMARY KEY(library_id, path)) WITHOUT ROWID;`,
	// 2: a game on several discs is one item, and these are its discs. How a game was
	// recognised, and which file of an archive holding many is the one to play.
	`CREATE TABLE IF NOT EXISTS game_discs (
	   item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE, disc INTEGER NOT NULL, path TEXT NOT NULL,
	   PRIMARY KEY(item_id, disc)) WITHOUT ROWID;
	 CREATE INDEX IF NOT EXISTS game_discs_path ON game_discs(path);
	 ALTER TABLE game_meta ADD COLUMN header_title TEXT NOT NULL DEFAULT '';
	 ALTER TABLE game_meta ADD COLUMN detect_how TEXT NOT NULL DEFAULT '';
	 ALTER TABLE game_meta ADD COLUMN detect_sure INTEGER NOT NULL DEFAULT 0;
	 ALTER TABLE game_meta ADD COLUMN detect_proof TEXT NOT NULL DEFAULT '';
	 ALTER TABLE game_meta ADD COLUMN problem TEXT NOT NULL DEFAULT '';
	 ALTER TABLE game_meta ADD COLUMN chosen_entry TEXT NOT NULL DEFAULT '';
	 ALTER TABLE game_meta ADD COLUMN entry_name TEXT NOT NULL DEFAULT '';
	 ALTER TABLE game_meta ADD COLUMN alt_crcs TEXT NOT NULL DEFAULT '';
	 DROP TABLE IF EXISTS game_archives;`,
}

func (d *DB) migrateGames(ctx context.Context) error {
	// These add to what the main list makes, so they wait for it.
	var tables int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'game_meta'`).Scan(&tables); err != nil {
		return err
	}
	if tables == 0 {
		return nil
	}
	version, _ := strconv.Atoi(d.GetSetting(ctx, "schema.games", "0"))
	for i := version; i < len(gameMigrations); i++ {
		if _, err := d.ExecContext(ctx, gameMigrations[i]); err != nil {
			return fmt.Errorf("games migration %d: %w", i+1, err)
		}
		if err := d.SetSetting(ctx, "schema.games", strconv.Itoa(i+1)); err != nil {
			return err
		}
	}
	return nil
}
