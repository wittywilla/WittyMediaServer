package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// What restoring from a backup needs: a database file from outside is never opened as
// the server's database. A clean one is built from this program's own migrations, at the
// version the file says it has, and only the rows are copied across. Whatever else is
// in that file (a trigger, a view, a check, an odd index) never reaches the server.

// Versions says how far each track of migrations has gone.
type Versions struct {
	Main     int `json:"main"`
	TV       int `json:"tv"`
	Metadata int `json:"metadata"`
	Music    int `json:"music"`
}

// CurrentVersions is what this program builds.
func CurrentVersions() Versions {
	return Versions{Main: len(migrations), TV: len(wittyTVMigrations), Metadata: len(metadataMigrations), Music: len(musicMigrations)}
}

// Newer says whether v has anything this program doesn't know about.
func (v Versions) Newer(than Versions) bool {
	return v.Main > than.Main || v.TV > than.TV || v.Metadata > than.Metadata || v.Music > than.Music
}

// ReadVersions reads a database's versions without changing it.
func ReadVersions(ctx context.Context, q interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}, prefix string) (Versions, error) {
	var v Versions
	if err := q.QueryRowContext(ctx, `SELECT version FROM `+prefix+`schema LIMIT 1`).Scan(&v.Main); err != nil {
		return v, fmt.Errorf("it has no schema version: %w", err)
	}
	for key, dst := range map[string]*int{"schema.wittytv": &v.TV, "schema.metadata": &v.Metadata, "schema.music": &v.Music} {
		var s string
		err := q.QueryRowContext(ctx, `SELECT value FROM `+prefix+`settings WHERE key = ?`, key).Scan(&s)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return v, err
		}
		if s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 {
				return v, fmt.Errorf("%s isn't a version", key)
			}
			*dst = n
		}
	}
	if v.Main < 1 {
		return v, errors.New("it has no schema version")
	}
	return v, nil
}

// BuildAt makes an empty database in dir with exactly these versions of the schema, and
// returns it open on one connection with foreign keys off, ready to take rows.
func BuildAt(ctx context.Context, path string, v Versions) (*sql.DB, error) {
	if v.Newer(CurrentVersions()) || v.Main < 1 {
		return nil, errors.New("that schema version isn't one this program knows")
	}
	dsn := "file:" + escapeDBPath(path) + "?_pragma=foreign_keys(0)&_pragma=trusted_schema(0)&_pragma=synchronous(OFF)&_pragma=journal_mode(DELETE)"
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1)
	step := func(list []string, n int, what string) error {
		for i := 0; i < n; i++ {
			if _, err := d.ExecContext(ctx, list[i]); err != nil {
				return fmt.Errorf("%s migration %d: %w", what, i+1, err)
			}
		}
		return nil
	}
	if err := step(migrations, v.Main, "main"); err != nil {
		d.Close()
		return nil, err
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM schema`); err != nil {
		d.Close()
		return nil, err
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO schema(version) VALUES (?)`, v.Main); err != nil {
		d.Close()
		return nil, err
	}
	for _, track := range []struct {
		list []string
		n    int
		key  string
	}{{wittyTVMigrations, v.TV, "schema.wittytv"}, {metadataMigrations, v.Metadata, "schema.metadata"}, {musicMigrations, v.Music, "schema.music"}} {
		if err := step(track.list, track.n, track.key); err != nil {
			d.Close()
			return nil, err
		}
		if _, err := d.ExecContext(ctx, `INSERT OR REPLACE INTO settings(key, value) VALUES (?, ?)`, track.key, strconv.Itoa(track.n)); err != nil {
			d.Close()
			return nil, err
		}
	}
	return d, nil
}

// OpenForeign opens a database file from outside, to read, trusting nothing in it.
func OpenForeign(path string) (*sql.DB, error) {
	dsn := "file:" + escapeDBPath(path) + "?mode=ro&immutable=1&_pragma=query_only(1)&_pragma=trusted_schema(0)&_pragma=cell_size_check(1)&_pragma=mmap_size(0)"
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1)
	return d, nil
}

// SnapshotTo writes a consistent copy of the live database to path, while it's in use:
// VACUUM INTO reads one point in time and takes no write lock.
func SnapshotTo(ctx context.Context, live *sql.DB, path string) error {
	if _, err := live.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("copying the database: %w", err)
	}
	return nil
}

// OpenPlainFile opens a working copy with no pool and no checks on foreign keys, for
// rewriting one before it's used.
func OpenPlainFile(path string) (*sql.DB, error) {
	dsn := "file:" + escapeDBPath(path) + "?_pragma=foreign_keys(0)&_pragma=trusted_schema(0)&_pragma=synchronous(OFF)&_pragma=journal_mode(DELETE)"
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1)
	return d, nil
}


// RemoveOrphans deletes rows whose parent has gone, and says how many.
func RemoveOrphans(ctx context.Context, d *sql.DB) (int64, error) {
	conn, err := d.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	return removeOrphans(ctx, conn)
}

// RestoreTriggers puts this program's triggers on a database they were taken off for a
// bulk rewrite, from a clean build of the same schema rather than anything in the file.
func RestoreTriggers(ctx context.Context, path string) error {
	tmp, err := os.MkdirTemp(filepath.Dir(path), ".schema-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	base, err := BuildAt(ctx, filepath.Join(tmp, "base.db"), CurrentVersions())
	if err != nil {
		return err
	}
	defer base.Close()
	rows, err := base.QueryContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'trigger'`)
	if err != nil {
		return err
	}
	var list []string
	for rows.Next() {
		var q string
		if err := rows.Scan(&q); err != nil {
			rows.Close()
			return err
		}
		list = append(list, q)
	}
	rows.Close()
	d, err := OpenPlainFile(path)
	if err != nil {
		return err
	}
	defer d.Close()
	for _, q := range list {
		if _, err := d.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
