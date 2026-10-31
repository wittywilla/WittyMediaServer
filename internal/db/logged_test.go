package db

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureLog points the default logger at a buffer for the length of a test.
func captureLog(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return buf
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func (l *lockedBuffer) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.b.Reset()
}

func TestLoggedFailureNeverShowsArgs(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	logs := captureLog(t)
	ctx := context.Background()
	const secret = "planted-secret-value"

	_, err = d.ExecContext(ctx, `INSERT INTO no_such_table(a) VALUES (?)`, secret)
	if err == nil {
		t.Fatal("expected an error")
	}
	out := logs.String()
	if strings.Count(out, "database error") != 1 {
		t.Fatalf("want one record, got: %s", out)
	}
	for _, want := range []string{"level=ERROR", "cat=server", "no such table", "INSERT INTO no_such_table"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
	if strings.Contains(out, secret) {
		t.Fatalf("an argument reached the log: %s", out)
	}

	// The same through a prepared statement.
	logs.Reset()
	if _, err := d.ExecContext(ctx, `CREATE TABLE t(a TEXT UNIQUE, b TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	st, err := d.PrepareContext(ctx, `INSERT INTO t(a, b) VALUES (?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.ExecContext(ctx, secret, nil); err == nil {
		t.Fatal("expected a NOT NULL failure")
	}
	if out := logs.String(); !strings.Contains(out, "level=WARN") || strings.Contains(out, secret) {
		t.Fatalf("got: %s", out)
	}
}

func TestLoggedLevels(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	if _, err := d.ExecContext(ctx, `CREATE TABLE t(a TEXT UNIQUE)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO t VALUES ('x')`); err != nil {
		t.Fatal(err)
	}
	logs := captureLog(t)

	// A UNIQUE failure: callers check for this on purpose.
	if _, err := d.ExecContext(ctx, `INSERT INTO t VALUES ('x')`); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("got %v", err)
	}
	if out := logs.String(); !strings.Contains(out, "level=DEBUG") {
		t.Fatalf("UNIQUE: %s", out)
	}

	// No rows: not a failure at all.
	logs.Reset()
	var a string
	if err := d.QueryRowContext(ctx, `SELECT a FROM t WHERE a = 'nope'`).Scan(&a); err != sql.ErrNoRows {
		t.Fatalf("got %v", err)
	}
	if out := logs.String(); out != "" {
		t.Fatalf("no rows: %s", out)
	}

	// A context already cancelled.
	logs.Reset()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, _ = d.ExecContext(cancelled, `INSERT INTO t VALUES ('y')`)
	if out := logs.String(); strings.Contains(out, "level=ERROR") || strings.Contains(out, "level=WARN") {
		t.Fatalf("cancelled: %s", out)
	}

	// Busy: another connection holds the write lock past the wait.
	logs.Reset()
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	conn, err := d.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA busy_timeout = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO t VALUES ('z')`); err == nil {
		t.Fatal("expected busy")
	}
	if out := logs.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "locked") {
		t.Fatalf("busy: %s", out)
	}
}

// A new database asks whether it has a schema yet, and a clean start says nothing.
func TestFreshOpenLogsNothing(t *testing.T) {
	logs := captureLog(t)
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if out := logs.String(); out != "" {
		t.Fatalf("got: %s", out)
	}
}

func TestStatementHead(t *testing.T) {
	if got := statementHead("SELECT  a\n\t FROM b"); got != "SELECT a FROM b" {
		t.Fatalf("got %q", got)
	}
	long := "SELECT " + strings.Repeat("é", 300)
	got := statementHead(long)
	if len(got) > maxStatementLog+len("…") || !strings.HasSuffix(got, "…") || strings.Contains(got, "\xef\xbf\xbd") {
		t.Fatalf("got %q", got)
	}
}

// The driver's optional interfaces: if an upgrade drops or adds one, the layer
// over it should be looked at again.
func TestDriverInterfacesForwarded(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	raw := filepath.ToSlash(filepath.Join(t.TempDir(), "x.db"))
	c, err := loggedDriver{inner: d.Driver().(loggedDriver).inner}.Open("file:" + raw)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	inner := c.(*loggedConn).Conn
	for name, ok := range map[string]bool{
		"ExecerContext":      is[driver.ExecerContext](inner),
		"QueryerContext":     is[driver.QueryerContext](inner),
		"ConnPrepareContext": is[driver.ConnPrepareContext](inner),
		"ConnBeginTx":        is[driver.ConnBeginTx](inner),
		"Pinger":             is[driver.Pinger](inner),
		"SessionResetter":    is[driver.SessionResetter](inner),
		"Validator":          is[driver.Validator](inner),
	} {
		if !ok {
			t.Errorf("the driver no longer has %s", name)
		}
	}
	// Our layer has them whatever the driver does.
	var _ interface {
		driver.ExecerContext
		driver.QueryerContext
		driver.ConnPrepareContext
		driver.ConnBeginTx
		driver.Pinger
		driver.SessionResetter
		driver.Validator
		driver.NamedValueChecker
	} = (*loggedConn)(nil)
	var _ interface {
		driver.StmtExecContext
		driver.StmtQueryContext
		driver.NamedValueChecker
	} = (*loggedStmt)(nil)
	var _ driver.ColumnConverter = (*convertingStmt)(nil)
	var _ interface {
		driver.RowsNextResultSet
		driver.RowsColumnTypeScanType
		driver.RowsColumnTypeDatabaseTypeName
		driver.RowsColumnTypeLength
		driver.RowsColumnTypeNullable
		driver.RowsColumnTypePrecisionScale
	} = (*loggedRows)(nil)

	// A prepared statement still converts arguments that aren't driver values.
	if _, err := d.ExecContext(context.Background(), `CREATE TABLE t(a INTEGER, b TEXT, c INTEGER)`); err != nil {
		t.Fatal(err)
	}
	st, err := d.PrepareContext(context.Background(), `INSERT INTO t VALUES (?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.ExecContext(context.Background(), int(7), sql.NullString{String: "x", Valid: true}, time.Unix(5, 0)); err != nil {
		t.Fatal(err)
	}
}

func is[T any](v any) bool {
	_, ok := v.(T)
	return ok
}

// The DSN's settings still reach the driver.
func TestDSNPragmasStillApply(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	var journal string
	var foreign int
	if err := d.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	if journal != "wal" || foreign != 1 {
		t.Fatalf("journal %q, foreign_keys %d", journal, foreign)
	}
}

func BenchmarkQueryThroughLoggedDriver(b *testing.B) {
	d, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer d.Close()
	benchQuery(b, d.DB)
}

func BenchmarkQueryBareDriver(b *testing.B) {
	sdb, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(b.TempDir(), "x.db"))+"?_pragma=journal_mode(WAL)")
	if err != nil {
		b.Fatal(err)
	}
	defer sdb.Close()
	benchQuery(b, sdb)
}

func benchQuery(b *testing.B, sdb *sql.DB) {
	ctx := context.Background()
	if _, err := sdb.ExecContext(ctx, `CREATE TABLE b(a INTEGER)`); err != nil {
		b.Fatal(err)
	}
	if _, err := sdb.ExecContext(ctx, `INSERT INTO b VALUES (1)`); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var a int
		if err := sdb.QueryRowContext(ctx, `SELECT a FROM b WHERE a = ?`, 1).Scan(&a); err != nil {
			b.Fatal(err)
		}
	}
}
