// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The repair tools have to look at a database that may be damaged, or be in use by a
// running server, without changing it by accident. So this opens the file as it is: no
// migrations, no logging of each failure (a damaged file fails on purpose here, many times),
// and schema contents aren't trusted to run anything.

// ErrNoDatabase: there's no database in that folder, which is what a new install looks like.
var ErrNoDatabase = errors.New("there is no database in this folder")

// escapeDBPath keeps a folder with # or % in its name from opening a different file.
func escapeDBPath(p string) string {
	return strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23").Replace(filepath.ToSlash(p))
}

// OpenExisting opens <dataDir>/wms.db without migrating or creating it. readOnly keeps
// every statement from writing. The first read happens here, so a file that isn't a
// database at all answers with an error rather than a handle.
func OpenExisting(dataDir string, readOnly bool) (*DB, error) {
	p := filepath.Join(dataDir, "wms.db")
	if st, err := os.Stat(p); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNoDatabase
		}
		return nil, err
	} else if st.IsDir() {
		return nil, fmt.Errorf("%s is a folder, and should be a file", p)
	}
	mode := "rw"
	extra := ""
	if readOnly {
		mode, extra = "ro", "&_pragma=query_only(1)"
	}
	dsn := fmt.Sprintf("file:%s?mode=%s%s&_pragma=trusted_schema(0)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", escapeDBPath(p), mode, extra)
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sdb.SetMaxOpenConns(3)
	var n int
	if err := sdb.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&n); err != nil {
		sdb.Close()
		return nil, err
	}
	return &DB{sdb}, nil
}

// TableColumn is one column of a table, as the database says.
type TableColumn struct {
	Name string
	Type string
}

// Tables lists the ordinary tables, leaving out SQLite's own.
func (d *DB) Tables(ctx context.Context) ([]string, error) {
	rows, err := d.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Columns lists a table's columns. table comes from Tables, never from a person.
func (d *DB) Columns(ctx context.Context, table string) ([]TableColumn, error) {
	rows, err := d.QueryContext(ctx, `SELECT name, type FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TableColumn
	for rows.Next() {
		var c TableColumn
		if err := rows.Scan(&c.Name, &c.Type); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// QuoteIdent writes a table or column name for a statement.
func QuoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
