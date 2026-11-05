package db

import (
	"context"
	"fmt"
	"strconv"
)

// Music keeps its own version (setting "schema.music"), as WittyTV does, so it can
// grow without taking a number in the server's main list.
var musicMigrations = []string{
	// 1: how far each person has moved a track's lyrics to fit what they hear.
	`CREATE TABLE IF NOT EXISTS lyrics_offsets (
	   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	   item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   offset_ms INTEGER NOT NULL, PRIMARY KEY(user_id, item_id));`,
	// 2: songs tied to their music videos (internal/music), and the codes in a file's tags
	// that say which recording it is. A link someone made or undid has "by" set, and a
	// new match never changes it. sync_ms is where the song starts in the video; NULL when
	// the two don't line up.
	`CREATE TABLE IF NOT EXISTS music_recording_ids (
	   item_id INTEGER PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
	   isrc TEXT NOT NULL DEFAULT '', recording TEXT NOT NULL DEFAULT '');
	 CREATE TABLE IF NOT EXISTS music_video_links (
	   song_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   video_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	   state TEXT NOT NULL,  -- tied | suggested | untied
	   how TEXT NOT NULL,    -- isrc | recording | name | close | hand
	   sync_ms INTEGER, by TEXT NOT NULL DEFAULT '',
	   created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
	   PRIMARY KEY(song_id, video_id));
	 CREATE INDEX IF NOT EXISTS music_video_links_video ON music_video_links(video_id, state);`,
}

func (d *DB) migrateMusic(ctx context.Context) error {
	version, _ := strconv.Atoi(d.GetSetting(ctx, "schema.music", "0"))
	for i := version; i < len(musicMigrations); i++ {
		if _, err := d.ExecContext(ctx, musicMigrations[i]); err != nil {
			return fmt.Errorf("music migration %d: %w", i+1, err)
		}
		if err := d.SetSetting(ctx, "schema.music", strconv.Itoa(i+1)); err != nil {
			return err
		}
	}
	return nil
}
