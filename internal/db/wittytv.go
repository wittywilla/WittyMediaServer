package db

import (
	"context"
	"fmt"
	"strconv"
)

// WittyTV owns its tables and keeps its own version (setting "schema.wittytv"),
// so it can grow without taking a number in the server's main list.
var wittyTVMigrations = []string{
	// 1: library channels. channel_days holds the generated schedule of one
	// day; channel_history the definitions a save replaced.
	`CREATE TABLE IF NOT EXISTS channels (
	   id INTEGER PRIMARY KEY, number TEXT NOT NULL, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
	   enabled INTEGER NOT NULL DEFAULT 1, definition TEXT NOT NULL, seed INTEGER NOT NULL,
	   rev INTEGER NOT NULL DEFAULT 1, anchor_day INTEGER NOT NULL, logo TEXT NOT NULL DEFAULT '',
	   visibility TEXT NOT NULL DEFAULT '', count_progress INTEGER NOT NULL DEFAULT 0,
	   created_by INTEGER, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
	 CREATE TABLE IF NOT EXISTS channel_days (
	   channel_id INTEGER NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
	   day INTEGER NOT NULL, rev INTEGER NOT NULL, programmes TEXT NOT NULL, state TEXT NOT NULL,
	   PRIMARY KEY(channel_id, day));
	 CREATE TABLE IF NOT EXISTS channel_history (
	   id INTEGER PRIMARY KEY, channel_id INTEGER NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
	   saved_at INTEGER NOT NULL, name TEXT NOT NULL, definition TEXT NOT NULL);
	 CREATE INDEX IF NOT EXISTS channel_history_ch ON channel_history(channel_id, id);
	 ALTER TABLE shows ADD COLUMN air_days TEXT;   -- e.g. Monday|Thursday, from metadata
	 ALTER TABLE shows ADD COLUMN air_time TEXT;   -- HH:MM in the network's own time`,
	// 2: tuners, their channels, guide data and recordings.
	`CREATE TABLE IF NOT EXISTS tuners (
	   id INTEGER PRIMARY KEY, driver TEXT NOT NULL, name TEXT NOT NULL, options TEXT NOT NULL,
	   enabled INTEGER NOT NULL DEFAULT 1, max_streams INTEGER NOT NULL DEFAULT 0,
	   last_scan INTEGER, last_error TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL);
	 CREATE TABLE IF NOT EXISTS tuner_channels (
	   id INTEGER PRIMARY KEY, tuner_id INTEGER NOT NULL REFERENCES tuners(id) ON DELETE CASCADE,
	   source_id TEXT NOT NULL, number TEXT NOT NULL DEFAULT '', name TEXT NOT NULL,
	   grp TEXT NOT NULL DEFAULT '', callsign TEXT NOT NULL DEFAULT '', locator TEXT NOT NULL DEFAULT '',
	   logo TEXT NOT NULL DEFAULT '', hd INTEGER NOT NULL DEFAULT 0, radio INTEGER NOT NULL DEFAULT 0,
	   encrypted INTEGER NOT NULL DEFAULT 0, encrypted_why TEXT NOT NULL DEFAULT '',
	   program INTEGER NOT NULL DEFAULT 0, enabled INTEGER NOT NULL DEFAULT 1,
	   number_set TEXT NOT NULL DEFAULT '', name_set TEXT NOT NULL DEFAULT '',
	   epg_id TEXT NOT NULL DEFAULT '', epg_set TEXT NOT NULL DEFAULT '',
	   UNIQUE(tuner_id, source_id));
	 CREATE INDEX IF NOT EXISTS tuner_channels_epg ON tuner_channels(epg_id);
	 CREATE TABLE IF NOT EXISTS epg_sources (
	   id INTEGER PRIMARY KEY, name TEXT NOT NULL, url TEXT NOT NULL, auth TEXT NOT NULL DEFAULT '', tuner_id INTEGER,
	   last_fetch INTEGER, last_error TEXT NOT NULL DEFAULT '', programmes INTEGER NOT NULL DEFAULT 0,
	   UNIQUE(url));
	 CREATE TABLE IF NOT EXISTS epg_channels (
	   source_id INTEGER NOT NULL REFERENCES epg_sources(id) ON DELETE CASCADE,
	   epg_id TEXT NOT NULL, name TEXT NOT NULL, PRIMARY KEY(source_id, epg_id));
	 CREATE TABLE IF NOT EXISTS epg_programmes (
	   channel TEXT NOT NULL, start INTEGER NOT NULL, stop INTEGER NOT NULL, title TEXT NOT NULL,
	   sub TEXT NOT NULL DEFAULT '', descr TEXT NOT NULL DEFAULT '', category TEXT NOT NULL DEFAULT '',
	   rating TEXT NOT NULL DEFAULT '', season INTEGER, episode INTEGER, source TEXT NOT NULL DEFAULT '',
	   PRIMARY KEY(channel, start));
	 CREATE INDEX IF NOT EXISTS epg_programmes_stop ON epg_programmes(stop);
	 CREATE TABLE IF NOT EXISTS recordings (
	   id INTEGER PRIMARY KEY, channel_id INTEGER NOT NULL, user_id INTEGER, title TEXT NOT NULL, sub TEXT NOT NULL DEFAULT '',
	   start INTEGER NOT NULL, stop INTEGER NOT NULL, status TEXT NOT NULL, path TEXT NOT NULL DEFAULT '',
	   error TEXT NOT NULL DEFAULT '', rule_id INTEGER, created_at INTEGER NOT NULL);
	 CREATE INDEX IF NOT EXISTS recordings_start ON recordings(status, start);
	 CREATE TABLE IF NOT EXISTS recording_rules (
	   id INTEGER PRIMARY KEY, channel_id INTEGER NOT NULL, user_id INTEGER, title TEXT NOT NULL, created_at INTEGER NOT NULL);`,
	// 3: whether a commercials library also appears in the main menus and Home.
	`ALTER TABLE libraries ADD COLUMN show_main INTEGER NOT NULL DEFAULT 0;`,
}

func (d *DB) migrateWittyTV(ctx context.Context) error {
	version, _ := strconv.Atoi(d.GetSetting(ctx, "schema.wittytv", "0"))
	for i := version; i < len(wittyTVMigrations); i++ {
		if _, err := d.ExecContext(ctx, wittyTVMigrations[i]); err != nil {
			return fmt.Errorf("wittytv migration %d: %w", i+1, err)
		}
		if err := d.SetSetting(ctx, "schema.wittytv", strconv.Itoa(i+1)); err != nil {
			return err
		}
	}
	return nil
}
