// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OpenForReading opens the database of a data folder for looking only: no migrations, no
// writes, and a short wait if a server is holding it. For tools that run beside a server,
// or in place of one that won't start, where a broken database has to be an answer rather
// than a failure. The caller closes it.
func OpenForReading(dataDir string) (*sql.DB, error) {
	path := filepath.Join(dataDir, "wms.db")
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	name := strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23").Replace(filepath.ToSlash(path))
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(2000)", name)
	d, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.PingContext(ctx); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}
