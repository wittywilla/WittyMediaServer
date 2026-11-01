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
