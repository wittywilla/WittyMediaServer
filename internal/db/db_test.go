package db

import (
	"context"
	"path/filepath"
	"testing"
)

func indexExists(t *testing.T, d *DB, name string) bool {
	t.Helper()
	var n int
	if err := d.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func TestFreshDatabaseIsCurrent(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var v, rows int
	if err := d.QueryRowContext(context.Background(), `SELECT version, (SELECT COUNT(*) FROM schema) FROM schema`).Scan(&v, &rows); err != nil {
		t.Fatal(err)
	}
	if v != len(migrations) || rows != 1 {
		t.Fatalf("version %d in %d rows, want %d in 1", v, rows, len(migrations))
	}
	d.Close()
	// Opening again changes nothing and doesn't fail on the ALTER TABLEs.
	d, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
}

// A database from an earlier version moves up one step at a time.
func TestUpgradeFromEarlierVersion(t *testing.T) {
	dir := t.TempDir()
	saved := migrations
	migrations = saved[:9]
	d, err := Open(dir)
	migrations = saved
	if err != nil {
		t.Fatal(err)
	}
	if indexExists(t, d, "items_lib_title") {
		t.Fatal("the old version has the new index")
	}
	d.Close()
	if d, err = Open(dir); err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if !indexExists(t, d, "items_lib_title") || indexExists(t, d, "items_lib") {
		t.Fatal("the indexes were not replaced")
	}
}

// A step that fails leaves nothing behind, so the next start can try it again.
// It used to leave its earlier statements applied and no record of them, and
// ALTER TABLE then failed on every later start.
func TestFailedMigrationLeavesNoTrace(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
	saved := migrations
	defer func() { migrations = saved }()
	migrations = append(append([]string{}, saved...), `ALTER TABLE users ADD COLUMN half_done TEXT; THIS IS NOT SQL;`)
	if _, err := Open(dir); err == nil {
		t.Fatal("a broken migration was accepted")
	}
	migrations = append(append([]string{}, saved...), `ALTER TABLE users ADD COLUMN half_done TEXT;`)
	d, err = Open(dir)
	if err != nil {
		t.Fatalf("the step can't be run again: %v", err)
	}
	d.Close()
}

// "file:" names are URIs: these folder names would open another file.
func TestPathsWithSpecialCharacters(t *testing.T) {
	for _, name := range []string{"a#b", "a?b", "100%", "a b"} {
		if filepath.Separator == '\\' && name == "a?b" {
			continue // not a possible name on Windows
		}
		dir := filepath.Join(t.TempDir(), name)
		if err := mkdir(dir); err != nil {
			t.Fatal(err)
		}
		d, err := Open(dir)
		if err != nil {
			t.Errorf("%q: %v", name, err)
			continue
		}
		d.Close()
		if !fileExists(filepath.Join(dir, "wms.db")) {
			t.Errorf("%q: the database went elsewhere", name)
		}
	}
}
