package db

import (
	"context"
	"fmt"
	"strconv"
)

// Library conversion keeps its own version (setting "schema.libtranscode"), as WittyTV
// does, so it can grow without taking a number in the server's main list.
var libTranscodeMigrations = []string{
	// 1: what each library's files should become, every run, every file of each run
	// (with what's needed to finish or undo a swap after a crash), and what has been
	// tried already, so a file that came out no smaller isn't converted again and again.
	`CREATE TABLE IF NOT EXISTS ltc_rules (
	   library_id INTEGER PRIMARY KEY REFERENCES libraries(id) ON DELETE CASCADE,
	   policy TEXT NOT NULL, output TEXT NOT NULL DEFAULT 'keep',
	   allow_writes INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL);
	 CREATE TABLE IF NOT EXISTS ltc_runs (
	   id INTEGER PRIMARY KEY, library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
	   dry INTEGER NOT NULL DEFAULT 0, policy TEXT NOT NULL, output TEXT NOT NULL, state TEXT NOT NULL,
	   by_id INTEGER, by_name TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, finished_at INTEGER,
	   summary TEXT NOT NULL DEFAULT '{}');
	 CREATE INDEX IF NOT EXISTS ltc_runs_library ON ltc_runs(library_id, id);
	 CREATE TABLE IF NOT EXISTS ltc_files (
	   id INTEGER PRIMARY KEY, run_id INTEGER NOT NULL REFERENCES ltc_runs(id) ON DELETE CASCADE,
	   item_id INTEGER NOT NULL, part INTEGER NOT NULL DEFAULT 0, path TEXT NOT NULL,
	   verdict TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '', plan TEXT NOT NULL DEFAULT '{}',
	   state TEXT NOT NULL, message TEXT NOT NULL DEFAULT '', detail TEXT NOT NULL DEFAULT '',
	   size_before INTEGER NOT NULL DEFAULT 0, mtime_before INTEGER NOT NULL DEFAULT 0, size_after INTEGER NOT NULL DEFAULT 0,
	   est_bytes INTEGER NOT NULL DEFAULT 0, est_sec REAL NOT NULL DEFAULT 0, duration REAL NOT NULL DEFAULT 0,
	   temp_path TEXT NOT NULL DEFAULT '', final_path TEXT NOT NULL DEFAULT '', kept_path TEXT NOT NULL DEFAULT '',
	   swap TEXT NOT NULL DEFAULT '', finished_at INTEGER);
	 CREATE INDEX IF NOT EXISTS ltc_files_run ON ltc_files(run_id, id);
	 CREATE INDEX IF NOT EXISTS ltc_files_item ON ltc_files(item_id);
	 CREATE INDEX IF NOT EXISTS ltc_files_swap ON ltc_files(swap) WHERE swap <> '';
	 CREATE TABLE IF NOT EXISTS ltc_memo (
	   path TEXT NOT NULL, policy TEXT NOT NULL, size INTEGER NOT NULL, mtime INTEGER NOT NULL,
	   outcome TEXT NOT NULL, at INTEGER NOT NULL, PRIMARY KEY(path, policy));
	 ALTER TABLE libraries ADD COLUMN read_only INTEGER NOT NULL DEFAULT 0;`,
}

func (d *DB) migrateLibTranscode(ctx context.Context) error {
	version, _ := strconv.Atoi(d.GetSetting(ctx, "schema.libtranscode", "0"))
	for i := version; i < len(libTranscodeMigrations); i++ {
		if _, err := d.ExecContext(ctx, libTranscodeMigrations[i]); err != nil {
			return fmt.Errorf("library conversion migration %d: %w", i+1, err)
		}
		if err := d.SetSetting(ctx, "schema.libtranscode", strconv.Itoa(i+1)); err != nil {
			return err
		}
	}
	return nil
}
