// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package db

import (
	"context"
	"fmt"
	"strconv"
)

// Library metadata owns its tables and keeps its own version (setting
// "schema.metadata"), as WittyTV does: field locks, match history, logos,
// categories, saved views and streaming listings can grow without taking a
// number in the server's main list.
var metadataMigrations = []string{
	// 1: what's known about each title beyond its row in items or shows, and how
	// it got there. target is "item" or "show". locks and sources say which fields a
	// person owns and where each value came from; baseline keeps what a source
	// said before someone typed over it, for Revert.
	`CREATE TABLE IF NOT EXISTS title_meta (
	   target TEXT NOT NULL, target_id INTEGER NOT NULL,
	   locks TEXT NOT NULL DEFAULT '', sources TEXT NOT NULL DEFAULT '{}', baseline TEXT NOT NULL DEFAULT '{}',
	   match_state TEXT NOT NULL DEFAULT '', suggestions TEXT NOT NULL DEFAULT '', suggested_at INTEGER NOT NULL DEFAULT 0,
	   director TEXT NOT NULL DEFAULT '', edition TEXT NOT NULL DEFAULT '', tags TEXT NOT NULL DEFAULT '',
	   banner TEXT NOT NULL DEFAULT '', logo TEXT NOT NULL DEFAULT '', audio_format TEXT NOT NULL DEFAULT '',
	   PRIMARY KEY(target, target_id)) WITHOUT ROWID;
	 CREATE TRIGGER IF NOT EXISTS title_meta_item_gone AFTER DELETE ON items BEGIN
	   DELETE FROM title_meta WHERE target = 'item' AND target_id = OLD.id; END;
	 CREATE TRIGGER IF NOT EXISTS title_meta_show_gone AFTER DELETE ON shows BEGIN
	   DELETE FROM title_meta WHERE target = 'show' AND target_id = OLD.id; END;
	 CREATE TABLE IF NOT EXISTS meta_history (
	   id INTEGER PRIMARY KEY, batch TEXT NOT NULL, at INTEGER NOT NULL, user_name TEXT NOT NULL DEFAULT '', action TEXT NOT NULL,
	   target TEXT NOT NULL, target_id INTEGER NOT NULL, title TEXT NOT NULL DEFAULT '', before_json TEXT NOT NULL, undone INTEGER NOT NULL DEFAULT 0);
	 CREATE INDEX IF NOT EXISTS meta_history_batch ON meta_history(batch);`,
	// 2: studios and networks of a title, one row each. name is how it is written;
	// key is how two spellings are told to be the same.
	`CREATE TABLE IF NOT EXISTS title_brands (
	   kind TEXT NOT NULL, key TEXT NOT NULL, name TEXT NOT NULL, library_id INTEGER NOT NULL,
	   item_id INTEGER REFERENCES items(id) ON DELETE CASCADE,
	   show_id INTEGER REFERENCES shows(id) ON DELETE CASCADE);
	 CREATE INDEX IF NOT EXISTS title_brands_key ON title_brands(kind, key);
	 CREATE INDEX IF NOT EXISTS title_brands_lib ON title_brands(library_id, kind);
	 CREATE INDEX IF NOT EXISTS title_brands_item ON title_brands(item_id) WHERE item_id IS NOT NULL;
	 CREATE INDEX IF NOT EXISTS title_brands_show ON title_brands(show_id) WHERE show_id IS NOT NULL;`,
	// 3: the logo service: one row per thing that may have a logo. Never bundled:
	// a row only gets a file after the owner agreed, or uploaded one themselves.
	`CREATE TABLE IF NOT EXISTS logos (
	   id INTEGER PRIMARY KEY, kind TEXT NOT NULL, key TEXT NOT NULL, name TEXT NOT NULL,
	   status TEXT NOT NULL DEFAULT 'pending', source TEXT NOT NULL DEFAULT '', source_id TEXT NOT NULL DEFAULT '',
	   source_url TEXT NOT NULL DEFAULT '', license TEXT NOT NULL DEFAULT '', license_url TEXT NOT NULL DEFAULT '',
	   credit TEXT NOT NULL DEFAULT '', file TEXT NOT NULL DEFAULT '', opaque INTEGER NOT NULL DEFAULT 0,
	   version INTEGER NOT NULL DEFAULT 0, tried_at INTEGER NOT NULL DEFAULT 0,
	   UNIQUE(kind, key));`,
	// 4: a film attached to another: an extra (bonus feature) or another version
	// of the same film. Attached items leave lists and appear on their parent's page.
	`CREATE TABLE IF NOT EXISTS item_links (
	   item_id INTEGER PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
	   link TEXT NOT NULL, extra_type TEXT NOT NULL DEFAULT '',
	   parent_item INTEGER REFERENCES items(id) ON DELETE CASCADE,
	   parent_show INTEGER REFERENCES shows(id) ON DELETE CASCADE,
	   label TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL);
	 CREATE INDEX IF NOT EXISTS item_links_parent ON item_links(parent_item) WHERE parent_item IS NOT NULL;
	 CREATE INDEX IF NOT EXISTS item_links_show ON item_links(parent_show) WHERE parent_show IS NOT NULL;`,
	// 5: a file or a show's folder that belongs to another library than its
	// folder says, or (to_library 0) that was removed and must not come back.
	`CREATE TABLE IF NOT EXISTS library_moves (
	   path TEXT PRIMARY KEY, from_library INTEGER NOT NULL, to_library INTEGER NOT NULL,
	   title TEXT NOT NULL DEFAULT '', moved_at INTEGER NOT NULL);
	 CREATE INDEX IF NOT EXISTS library_moves_from ON library_moves(from_library);
	 CREATE INDEX IF NOT EXISTS library_moves_to ON library_moves(to_library);`,
	// 6: how a library's categories were changed by hand. Categories themselves
	// are worked out from genres, so only the changes are kept.
	`CREATE TABLE IF NOT EXISTS categories (
	   library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE, key TEXT NOT NULL,
	   name TEXT NOT NULL DEFAULT '', hidden INTEGER NOT NULL DEFAULT 0, position INTEGER NOT NULL DEFAULT 0,
	   merged_into TEXT NOT NULL DEFAULT '', art_item INTEGER NOT NULL DEFAULT 0,
	   PRIMARY KEY(library_id, key)) WITHOUT ROWID;`,
	// 7: a browse someone saved, which can be pinned to Home.
	`CREATE TABLE IF NOT EXISTS saved_views (
	   id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   library_id INTEGER NOT NULL DEFAULT 0, name TEXT NOT NULL, query TEXT NOT NULL,
	   pinned INTEGER NOT NULL DEFAULT 0, position INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL);
	 CREATE INDEX IF NOT EXISTS saved_views_user ON saved_views(user_id, position);`,
	// 8: rules that let a folder's naming guide matching.
	`CREATE TABLE IF NOT EXISTS match_rules (
	   id INTEGER PRIMARY KEY, library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
	   position INTEGER NOT NULL DEFAULT 0, pattern TEXT NOT NULL, lookup TEXT NOT NULL DEFAULT 'auto',
	   tags TEXT NOT NULL DEFAULT '', extra TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1);
	 CREATE INDEX IF NOT EXISTS match_rules_lib ON match_rules(library_id, position);`,
	// 9: which streaming services list a local title, by region: off until the
	// owner has read the disclosures. checked_at says how old an answer is.
	`CREATE TABLE IF NOT EXISTS listing_titles (
	   target TEXT NOT NULL, target_id INTEGER NOT NULL, region TEXT NOT NULL, checked_at INTEGER NOT NULL,
	   tmdb_kind TEXT NOT NULL DEFAULT '', tmdb_id TEXT NOT NULL DEFAULT '',
	   PRIMARY KEY(target, target_id, region)) WITHOUT ROWID;
	 CREATE TABLE IF NOT EXISTS listings (
	   target TEXT NOT NULL, target_id INTEGER NOT NULL, region TEXT NOT NULL, platform_key TEXT NOT NULL,
	   platform TEXT NOT NULL, offer TEXT NOT NULL,
	   PRIMARY KEY(target, target_id, region, platform_key)) WITHOUT ROWID;
	 CREATE INDEX IF NOT EXISTS listings_platform ON listings(region, platform_key);
	 CREATE TRIGGER IF NOT EXISTS listings_item_gone AFTER DELETE ON items BEGIN
	   DELETE FROM listings WHERE target = 'item' AND target_id = OLD.id;
	   DELETE FROM listing_titles WHERE target = 'item' AND target_id = OLD.id; END;
	 CREATE TRIGGER IF NOT EXISTS listings_show_gone AFTER DELETE ON shows BEGIN
	   DELETE FROM listings WHERE target = 'show' AND target_id = OLD.id;
	   DELETE FROM listing_titles WHERE target = 'show' AND target_id = OLD.id; END;`,
	// 10: where-to-watch answers, by TMDB id and region, kept as long as listings are, and
	// the skip markers of each video. scanned is what the scan read from chapters and is
	// rewritten by each scan; manual is what an administrator set, which no scan touches.
	`CREATE TABLE IF NOT EXISTS watch_offers_cache (
	   source TEXT NOT NULL, kind TEXT NOT NULL, tmdb_id TEXT NOT NULL, region TEXT NOT NULL,
	   fetched_at INTEGER NOT NULL, body TEXT NOT NULL,
	   PRIMARY KEY(source, kind, tmdb_id, region)) WITHOUT ROWID;
	 CREATE TABLE IF NOT EXISTS media_markers (
	   item_id INTEGER PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
	   scanned TEXT NOT NULL DEFAULT '', manual TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL DEFAULT 0);`,
	// What comparing a season's episodes by ear found (internal/introdetect), kept apart
	// from what a scan read and what someone set, so neither overwrites the other.
	`ALTER TABLE media_markers ADD COLUMN detected TEXT NOT NULL DEFAULT '';`,
}

func (d *DB) migrateMetadata(ctx context.Context) error {
	version, _ := strconv.Atoi(d.GetSetting(ctx, "schema.metadata", "0"))
	for i := version; i < len(metadataMigrations); i++ {
		if _, err := d.ExecContext(ctx, metadataMigrations[i]); err != nil {
			return fmt.Errorf("metadata migration %d: %w", i+1, err)
		}
		if err := d.SetSetting(ctx, "schema.metadata", strconv.Itoa(i+1)); err != nil {
			return err
		}
	}
	return nil
}
