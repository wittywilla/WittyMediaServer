package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Recovery builds a new database from what can still be read of a damaged one: a fresh
// file from this version's migrations, with the rows copied table by table. A row the
// damaged file can't give up, or that the new one refuses, is counted and left behind.
// The damaged file is only read.

// RecoveredTable says how one table came through.
type RecoveredTable struct {
	Name   string `json:"name"`
	Copied int64  `json:"copied"`
	// Lost is what couldn't be copied. Where a damaged stretch hides how many rows it
	// held, this is how many row numbers it spans, so it can be more than there were.
	Lost    int64 `json:"lost"`
	Refused int64 `json:"refused"` // rows the new table wouldn't take
	Failed  bool  `json:"failed"`  // nothing of this table could be read at all
}

// Recovered is the result of a recovery.
type Recovered struct {
	Tables   []RecoveredTable `json:"tables"`
	Copied   int64            `json:"copied"`
	Lost     int64            `json:"lost"`
	Refused  int64            `json:"refused"`
	Unlinked int64            `json:"unlinked"` // rows removed because what they pointed at was lost
	Unread   int              `json:"unread"`   // tables that couldn't be read at all
}

const recoverBatch = 500

// RecoverInto writes a new wms.db in dst from what can be read of the one in src. dst must
// be a new, empty folder of the caller's. A panic in the driver is turned into an error:
// a damaged file must never take down whoever is trying to repair it.
func RecoverInto(ctx context.Context, src, dst string) (res *Recovered, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("the database engine stopped on the damaged file: %v", p)
		}
	}()
	fresh, err := Open(dst)
	if err != nil {
		return nil, fmt.Errorf("could not start a new database: %w", err)
	}
	defer fresh.Close()
	res = &Recovered{}
	tables, err := fresh.Tables(ctx)
	if err != nil {
		return nil, err
	}
	old, oldErr := OpenExisting(src, true)
	if old != nil {
		defer old.Close()
	}
	conn, err := fresh.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return nil, err
	}
	for _, t := range tables {
		if t == "schema" {
			continue // the new file's own record of its migrations
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rt := RecoveredTable{Name: t}
		if oldErr != nil {
			rt.Failed = true
		} else {
			rt = recoverTable(ctx, old, fresh, conn, t)
		}
		res.Tables = append(res.Tables, rt)
		res.Copied += rt.Copied
		res.Lost += rt.Lost
		res.Refused += rt.Refused
		if rt.Failed {
			res.Unread++
		}
	}
	// With parents lost, their children point at nothing. They go, as a delete would have
	// taken them: repeatedly, because removing one can leave another without its parent.
	for range 8 {
		n, err := removeOrphans(ctx, conn)
		if err != nil {
			return nil, err
		}
		res.Unlinked += n
		if n == 0 {
			break
		}
	}
	var ok string
	if err := conn.QueryRowContext(ctx, `PRAGMA integrity_check(1)`).Scan(&ok); err != nil {
		return nil, err
	}
	if ok != "ok" {
		return nil, errors.New("the new database didn't pass its own check: " + ok)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return nil, err
	}
	return res, nil
}

func recoverTable(ctx context.Context, old, fresh *DB, conn *sql.Conn, table string) RecoveredTable {
	rt := RecoveredTable{Name: table}
	oldCols, err := old.Columns(ctx, table)
	if err != nil || len(oldCols) == 0 {
		// An older database may simply not have this table yet: that's nothing lost.
		rt.Failed = err != nil
		return rt
	}
	newCols, err := fresh.Columns(ctx, table)
	if err != nil {
		rt.Failed = true
		return rt
	}
	have := map[string]bool{}
	for _, c := range oldCols {
		have[c.Name] = true
	}
	var cols []string
	for _, c := range newCols {
		if have[c.Name] {
			cols = append(cols, QuoteIdent(c.Name))
		}
	}
	if len(cols) == 0 {
		return rt
	}
	list := strings.Join(cols, ", ")
	marks := strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ")
	ins, err := conn.PrepareContext(ctx, `INSERT OR IGNORE INTO `+QuoteIdent(table)+` (`+list+`) VALUES (`+marks+`)`)
	if err != nil {
		rt.Failed = true
		return rt
	}
	defer ins.Close()
	store := func(vals []any) {
		r, err := ins.ExecContext(ctx, vals...)
		if err != nil {
			rt.Refused++
			return
		}
		if n, _ := r.RowsAffected(); n == 0 {
			rt.Refused++
			return
		}
		rt.Copied++
	}
	if r, qerr := old.QueryContext(ctx, `SELECT rowid FROM `+QuoteIdent(table)+` LIMIT 0`); qerr != nil {
		if strings.Contains(qerr.Error(), "no such column") {
			recoverPlain(ctx, old, table, list, len(cols), store, &rt)
			return rt
		}
	} else {
		r.Close()
	}
	q := `SELECT rowid, ` + list + ` FROM ` + QuoteIdent(table) + ` WHERE rowid > ? ORDER BY rowid LIMIT ?`
	const start = int64(-1 << 62)
	last, step := start, int64(0)
	for ctx.Err() == nil {
		n, next, rerr := readBatch(ctx, old, q, last, len(cols), store)
		if n > 0 {
			last, step = next, 0
		}
		if rerr == nil {
			if n < recoverBatch {
				return rt
			}
			continue
		}
		if n > 0 {
			continue // the good rows before the damage are in; ask again from there
		}
		// Reading from here fails, so a stretch of the table can't be read. Jump ahead in
		// growing steps until reading works again.
		if last == start {
			last = 0 // the damage is at the very start: row numbers begin at 1
		}
		if step == 0 {
			step = 1
		} else {
			step *= 8
		}
		if step > 1<<40 {
			rt.Failed = rt.Copied == 0
			return rt
		}
		rt.Lost += step
		last += step
	}
	return rt
}

// readBatch reads a batch of rows. It answers how many rows it got before any error.
func readBatch(ctx context.Context, old *DB, q string, after int64, n int, store func([]any)) (got int, last int64, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%v", p)
		}
	}()
	last = after
	rows, err := old.QueryContext(ctx, q, after, recoverBatch)
	if err != nil {
		return 0, after, err
	}
	defer rows.Close()
	vals := make([]any, n)
	ptrs := make([]any, n+1)
	ptrs[0] = &last
	for i := range vals {
		ptrs[i+1] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return got, last, err
		}
		store(vals)
		got++
	}
	return got, last, rows.Err()
}

// recoverPlain is for a table with no row numbers to jump over: it reads until it can't.
func recoverPlain(ctx context.Context, old *DB, table, list string, n int, store func([]any), rt *RecoveredTable) {
	rows, err := old.QueryContext(ctx, `SELECT `+list+` FROM `+QuoteIdent(table))
	if err != nil {
		rt.Failed = true
		return
	}
	defer rows.Close()
	vals := make([]any, n)
	ptrs := make([]any, n)
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			rt.Lost++
			continue
		}
		store(vals)
	}
	if rows.Err() != nil {
		rt.Lost++ // somewhere past here: how many isn't known
	}
}

func removeOrphans(ctx context.Context, conn *sql.Conn) (int64, error) {
	rows, err := conn.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return 0, err
	}
	type orphan struct {
		table string
		rowid sql.NullInt64
	}
	var list []orphan
	for rows.Next() {
		var o orphan
		var parent string
		var fk int64
		if err := rows.Scan(&o.table, &o.rowid, &parent, &fk); err != nil {
			rows.Close()
			return 0, err
		}
		list = append(list, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var n int64
	for _, o := range list {
		if !o.rowid.Valid {
			continue
		}
		r, err := conn.ExecContext(ctx, `DELETE FROM `+QuoteIdent(o.table)+` WHERE rowid = ?`, o.rowid.Int64)
		if err != nil {
			return n, err
		}
		c, _ := r.RowsAffected()
		n += c
	}
	return n, nil
}
