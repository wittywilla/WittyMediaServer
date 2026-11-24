// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package dbtest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// dump is everything a new database holds: its schema, and every row of every table.
func dump(t *testing.T, d *db.DB) string {
	t.Helper()
	ctx := context.Background()
	rows, err := d.QueryContext(ctx, `SELECT type, name, tbl_name, COALESCE(sql, '') FROM sqlite_master ORDER BY type, name`)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	var tables []string
	for rows.Next() {
		var typ, name, tbl, sql string
		if err := rows.Scan(&typ, &name, &tbl, &sql); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&out, "%s %s %s %s\n", typ, name, tbl, sql)
		if typ == "table" && !strings.HasPrefix(name, "sqlite_") {
			tables = append(tables, name)
		}
	}
	rows.Close()
	for _, table := range tables {
		rows, err := d.QueryContext(ctx, `SELECT * FROM "`+table+`" ORDER BY 1`)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&out, "%s %v\n", table, vals)
		}
		rows.Close()
	}
	for _, p := range []string{"user_version", "application_id", "journal_mode", "foreign_keys", "auto_vacuum"} {
		var v string
		if err := d.QueryRowContext(ctx, `PRAGMA `+p).Scan(&v); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&out, "pragma %s %s\n", p, v)
	}
	return out.String()
}

// A copy holds exactly what db.Open makes.
func TestCopyIsANewDatabase(t *testing.T) {
	made, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer made.Close()
	copied, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer copied.Close()
	if a, b := dump(t, made), dump(t, copied); a != b {
		t.Errorf("a copy differs from a new database:\n--- db.Open\n%s\n--- dbtest.Open\n%s", a, b)
	}
}

// Each copy is a database of its own, and a folder that has one already keeps it.
func TestCopiesAreSeparate(t *testing.T) {
	dir := t.TempDir()
	a, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetSetting(context.Background(), "probe", "a"); err != nil {
		t.Fatal(err)
	}
	a.Close()
	b, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if v := b.GetSetting(context.Background(), "probe", ""); v != "" {
		t.Errorf("a new copy has another test's setting: %q", v)
	}
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if v := again.GetSetting(context.Background(), "probe", ""); v != "a" {
		t.Errorf("opening a folder again lost its database: %q", v)
	}
}

// A folder that isn't there is db.Open's to report.
func TestMissingFolderIsLeftToOpen(t *testing.T) {
	missing := t.TempDir() + "/not-here"
	_, want := db.Open(missing)
	_, got := Open(missing)
	if (want == nil) != (got == nil) {
		t.Errorf("db.Open: %v; dbtest.Open: %v", want, got)
	}
}
