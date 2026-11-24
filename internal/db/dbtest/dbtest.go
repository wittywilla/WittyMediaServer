// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package dbtest opens a new database for a test quickly. Making one runs every migration, and
// SQLite reads the whole schema again after each: about 0.3 s on a PC, and many seconds under the
// race detector, which checks every step of the database engine too. So each test program makes
// one database once, and each test gets a copy of that file, opened with db.Open as the server
// opens it. The copy is what db.Open would have made: the migrations put nothing random in it.
//
// Tests of making a database, or of migrating one, use db.Open itself.
package dbtest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

var template = sync.OnceValues(func() ([]byte, error) {
	dir, err := os.MkdirTemp("", "wms-dbtest-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	d, err := db.Open(dir)
	if err != nil {
		return nil, err
	}
	// Everything into the main file, so that the file alone is the whole database.
	_, err = d.ExecContext(context.Background(), `PRAGMA wal_checkpoint(TRUNCATE)`)
	if cerr := d.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(dir, "wms.db"))
})

// Open opens the database in dataDir as db.Open does. A folder with no database yet gets a copy
// of a new one instead of making it again; anything else is left to db.Open.
func Open(dataDir string) (*db.DB, error) {
	file := filepath.Join(dataDir, "wms.db")
	if fi, err := os.Stat(dataDir); err == nil && fi.IsDir() {
		if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
			b, err := template()
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(file, b, 0o600); err != nil {
				return nil, err
			}
		}
	}
	return db.Open(dataDir)
}
