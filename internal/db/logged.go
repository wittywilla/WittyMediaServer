package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Every statement in the server goes through this thin layer over the SQLite
// driver, so that one that fails is logged here, once, and no call site has to
// remember to. It shows the start of the statement and never its values: those
// are bound separately and never reach this file's log line.
//
// When nothing fails, it costs one comparison per call and a small struct per
// query or transaction. database/sql looks for optional driver interfaces by
// type assertion, so each is forwarded here; a method the driver doesn't have
// answers the way database/sql expects from a driver without it.

// driverName is what db.Open opens: the same driver and DSN, with logging.
const driverName = "wms-sqlite"

func init() { sql.Register(driverName, loggedDriver{&sqlite.Driver{}}) }

// maxStatementLog is how much of a statement goes in the log.
const maxStatementLog = 200

// report logs one failure, at the level it deserves.
func report(ctx context.Context, op, query string, err error) {
	// The logger is looked up each time: it is replaced when the server starts.
	log := slog.Default()
	lv := failureLevel(ctx, err)
	if !log.Enabled(ctx, lv) {
		return
	}
	attrs := []any{"cat", "server", "op", op, "err", err}
	if query != "" {
		attrs = append(attrs, "statement", statementHead(query))
	}
	log.Log(ctx, lv, "database error", attrs...)
}

// failureLevel sorts failures. Debug is for what isn't a fault: the caller gave
// up, or a constraint the caller checks for on purpose. Warn is for what passes:
// busy, and constraints no caller expects (usually a row deleted meanwhile).
func failureLevel(ctx context.Context, err error) slog.Level {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, sql.ErrNoRows) || errors.Is(err, driver.ErrBadConn) {
		return slog.LevelDebug
	}
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return slog.LevelError
	}
	switch code := se.Code(); code & 0xff {
	case sqlite3.SQLITE_INTERRUPT: // how a cancelled context shows up in the middle of a query
		return slog.LevelDebug
	case sqlite3.SQLITE_BUSY, sqlite3.SQLITE_LOCKED:
		return slog.LevelWarn
	case sqlite3.SQLITE_CONSTRAINT:
		if code == sqlite3.SQLITE_CONSTRAINT_UNIQUE || code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
			return slog.LevelDebug
		}
		return slog.LevelWarn
	}
	return slog.LevelError
}

// statementHead is the start of a statement on one line.
func statementHead(query string) string {
	if len(query) > 4*maxStatementLog { // don't tidy a long migration just to cut it
		query = query[:4*maxStatementLog]
	}
	query = strings.Join(strings.Fields(query), " ")
	if len(query) <= maxStatementLog {
		return query
	}
	cut := maxStatementLog
	for cut > 0 && query[cut]&0xC0 == 0x80 { // not in the middle of a character
		cut--
	}
	return query[:cut] + "…"
}

type loggedDriver struct{ inner driver.Driver }

func (d loggedDriver) Open(name string) (driver.Conn, error) {
	c, err := d.inner.Open(name)
	if err != nil {
		report(context.Background(), "open", "", err)
		return nil, err
	}
	return &loggedConn{Conn: c}, nil
}

// loggedConn: the embedded Prepare, Begin and Close are the driver's. database/sql
// only calls the first two when the context versions below don't exist.
type loggedConn struct{ driver.Conn }

func (c *loggedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	e, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip // database/sql prepares it instead
	}
	r, err := e.ExecContext(ctx, query, args)
	if err != nil && err != driver.ErrSkip {
		report(ctx, "exec", query, err)
	}
	return r, err
}

func (c *loggedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	q, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	r, err := q.QueryContext(ctx, query, args)
	if err != nil {
		if err != driver.ErrSkip {
			report(ctx, "query", query, err)
		}
		return r, err
	}
	return &loggedRows{Rows: r, ctx: ctx, query: query}, nil
}

func (c *loggedConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	var s driver.Stmt
	var err error
	if p, ok := c.Conn.(driver.ConnPrepareContext); ok {
		s, err = p.PrepareContext(ctx, query)
	} else {
		s, err = c.Conn.Prepare(query)
	}
	if err != nil {
		report(ctx, "prepare", query, err)
		return nil, err
	}
	ls := loggedStmt{Stmt: s, conn: c.Conn, query: query}
	// Only when the driver has it: database/sql skips converting altogether for
	// a statement that answers this but doesn't know its number of arguments.
	if _, ok := s.(driver.ColumnConverter); ok {
		return &convertingStmt{ls}, nil
	}
	return &ls, nil
}

func (c *loggedConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	var t driver.Tx
	var err error
	if b, ok := c.Conn.(driver.ConnBeginTx); ok {
		t, err = b.BeginTx(ctx, opts)
	} else {
		t, err = c.Conn.Begin() //nolint:staticcheck // the driver offers nothing newer
	}
	if err != nil {
		report(ctx, "begin", "", err)
		return nil, err
	}
	return loggedTx{t}, nil
}

func (c *loggedConn) Ping(ctx context.Context) error {
	p, ok := c.Conn.(driver.Pinger)
	if !ok {
		return nil
	}
	err := p.Ping(ctx)
	if err != nil {
		report(ctx, "ping", "", err)
	}
	return err
}

func (c *loggedConn) ResetSession(ctx context.Context) error {
	if r, ok := c.Conn.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

func (c *loggedConn) IsValid() bool {
	if v, ok := c.Conn.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

func (c *loggedConn) CheckNamedValue(nv *driver.NamedValue) error {
	if n, ok := c.Conn.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(nv)
	}
	return driver.ErrSkip // the default conversion
}

type loggedTx struct{ driver.Tx }

func (t loggedTx) Commit() error {
	err := t.Tx.Commit()
	if err != nil {
		report(context.Background(), "commit", "", err)
	}
	return err
}

func (t loggedTx) Rollback() error {
	err := t.Tx.Rollback()
	// SQLite rolls back by itself on some failures, and that failure was logged.
	if err != nil && !strings.Contains(err.Error(), "no transaction is active") {
		report(context.Background(), "rollback", "", err)
	}
	return err
}

type loggedStmt struct {
	driver.Stmt
	conn  driver.Conn
	query string
}

func (s *loggedStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	var r driver.Result
	var err error
	if e, ok := s.Stmt.(driver.StmtExecContext); ok {
		r, err = e.ExecContext(ctx, args)
	} else if err = ctx.Err(); err == nil {
		r, err = s.Stmt.Exec(plainValues(args)) //nolint:staticcheck // the driver offers nothing newer
	}
	if err != nil {
		report(ctx, "exec", s.query, err)
	}
	return r, err
}

func (s *loggedStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	var r driver.Rows
	var err error
	if q, ok := s.Stmt.(driver.StmtQueryContext); ok {
		r, err = q.QueryContext(ctx, args)
	} else if err = ctx.Err(); err == nil {
		r, err = s.Stmt.Query(plainValues(args)) //nolint:staticcheck // the driver offers nothing newer
	}
	if err != nil {
		report(ctx, "query", s.query, err)
		return nil, err
	}
	return &loggedRows{Rows: r, ctx: ctx, query: s.query}, nil
}

// CheckNamedValue: database/sql asks a statement before its connection, so this
// has to ask both.
func (s *loggedStmt) CheckNamedValue(nv *driver.NamedValue) error {
	if n, ok := s.Stmt.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(nv)
	}
	if n, ok := s.conn.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

// convertingStmt is a statement whose driver type converts its own arguments.
type convertingStmt struct{ loggedStmt }

func (s *convertingStmt) ColumnConverter(idx int) driver.ValueConverter {
	return s.Stmt.(driver.ColumnConverter).ColumnConverter(idx)
}

func plainValues(args []driver.NamedValue) []driver.Value {
	out := make([]driver.Value, len(args))
	for i, a := range args {
		out[i] = a.Value
	}
	return out
}

// loggedRows catches what fails while reading: SQLite does the work as rows are
// asked for, so a damaged page or a read error appears here, not in the query.
type loggedRows struct {
	driver.Rows
	ctx   context.Context
	query string
}

func (r *loggedRows) Next(dest []driver.Value) error {
	err := r.Rows.Next(dest)
	if err != nil && err != io.EOF {
		report(r.ctx, "next", r.query, err)
	}
	return err
}

func (r *loggedRows) HasNextResultSet() bool {
	n, ok := r.Rows.(driver.RowsNextResultSet)
	return ok && n.HasNextResultSet()
}

func (r *loggedRows) NextResultSet() error {
	n, ok := r.Rows.(driver.RowsNextResultSet)
	if !ok {
		return io.EOF
	}
	err := n.NextResultSet()
	if err != nil && err != io.EOF {
		report(r.ctx, "next", r.query, err)
	}
	return err
}

func (r *loggedRows) ColumnTypeScanType(i int) reflect.Type {
	if c, ok := r.Rows.(driver.RowsColumnTypeScanType); ok {
		return c.ColumnTypeScanType(i)
	}
	return reflect.TypeFor[any]()
}

func (r *loggedRows) ColumnTypeDatabaseTypeName(i int) string {
	if c, ok := r.Rows.(driver.RowsColumnTypeDatabaseTypeName); ok {
		return c.ColumnTypeDatabaseTypeName(i)
	}
	return ""
}

func (r *loggedRows) ColumnTypeLength(i int) (int64, bool) {
	if c, ok := r.Rows.(driver.RowsColumnTypeLength); ok {
		return c.ColumnTypeLength(i)
	}
	return 0, false
}

func (r *loggedRows) ColumnTypeNullable(i int) (bool, bool) {
	if c, ok := r.Rows.(driver.RowsColumnTypeNullable); ok {
		return c.ColumnTypeNullable(i)
	}
	return false, false
}

func (r *loggedRows) ColumnTypePrecisionScale(i int) (int64, int64, bool) {
	if c, ok := r.Rows.(driver.RowsColumnTypePrecisionScale); ok {
		return c.ColumnTypePrecisionScale(i)
	}
	return 0, 0, false
}
