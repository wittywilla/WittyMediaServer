package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
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

// A transaction that reads before it writes must wait for other writers, not
// fail: this is what a channel rescan does while a guide update is saving.
func TestTransactionsWaitForEachOther(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	if _, err := d.ExecContext(ctx, `CREATE TABLE tally(n INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO tally VALUES(0)`); err != nil {
		t.Fatal(err)
	}
	const writers = 6
	errs := make(chan error, writers)
	for range writers {
		go func() {
			errs <- func() error {
				tx, err := d.BeginTx(ctx, nil)
				if err != nil {
					return err
				}
				defer tx.Rollback()
				var n int
				if err := tx.QueryRowContext(ctx, `SELECT n FROM tally`).Scan(&n); err != nil {
					return err
				}
				time.Sleep(5 * time.Millisecond) // long enough for another write to land
				if _, err := tx.ExecContext(ctx, `UPDATE tally SET n = ?`, n+1); err != nil {
					return err
				}
				return tx.Commit()
			}()
		}()
	}
	for range writers {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
	var n int
	if err := d.QueryRowContext(ctx, `SELECT n FROM tally`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != writers {
		t.Errorf("tally = %d, want %d: a write was lost", n, writers)
	}
}

// A database built by BuildAt must be what Open makes. A new track of migrations that
// BuildAt doesn't know about would otherwise lose its tables in a restore.
func TestBuildAtIsWhatOpenMakes(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	b, err := BuildAt(ctx, filepath.Join(t.TempDir(), "built.db"), CurrentVersions())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	objects := func(q interface {
		QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	}) map[string]bool {
		rows, err := q.QueryContext(ctx, `SELECT type || ':' || name FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]bool{}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out[s] = true
		}
		return out
	}
	opened, built := objects(d.DB), objects(b)
	for k := range opened {
		if !built[k] {
			t.Errorf("Open makes %s and BuildAt doesn't. Add its track of migrations to Versions in baseline.go", k)
		}
	}
	for k := range built {
		if !opened[k] {
			t.Errorf("BuildAt makes %s and Open doesn't", k)
		}
	}
}

// Rows saved before numbers were checked: one infinite rating blanked every list holding it.
func TestInfiniteNumbersAreCleared(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	if _, err := d.ExecContext(ctx, `INSERT INTO libraries(id, name, type, created_at) VALUES(1, 'L', 'movies', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO items(id, library_id, kind, path, title, sort_title, rating, runtime_sec, added_at, updated_at) VALUES(1, 1, 'video', '/x', 't', 't', 9e999, 9e999, 0, 0), (2, 1, 'video', '/y', 't', 't', 7.5, 60, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO shows(id, library_id, path, title, sort_title, rating, added_at, updated_at) VALUES(1, 1, '/s', 't', 't', 9e999, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, clearInfinities); err != nil {
		t.Fatal(err)
	}
	var rating *float64
	var runtime float64
	if err := d.QueryRowContext(ctx, `SELECT rating, runtime_sec FROM items WHERE id = 1`).Scan(&rating, &runtime); err != nil {
		t.Fatal(err)
	}
	if rating != nil || runtime != 0 {
		t.Errorf("%v %v", rating, runtime)
	}
	if err := d.QueryRowContext(ctx, `SELECT rating, runtime_sec FROM items WHERE id = 2`).Scan(&rating, &runtime); err != nil {
		t.Fatal(err)
	}
	if rating == nil || *rating != 7.5 || runtime != 60 {
		t.Errorf("an ordinary row changed: %v %v", rating, runtime)
	}
	if err := d.QueryRowContext(ctx, `SELECT rating FROM shows WHERE id = 1`).Scan(&rating); err != nil || rating != nil {
		t.Errorf("%v %v", rating, err)
	}
}
