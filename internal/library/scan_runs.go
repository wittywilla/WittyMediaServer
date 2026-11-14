// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// A scan has no time limit of its own: a library of a hundred thousand files takes as long as it
// takes. What stops one is a person, or nothing moving for StuckAfter (a share that hangs, a
// module that stopped answering). Each run is written down as it starts (table library_scans),
// so a scan a restart cut short carries on when the server is back, and a Full rescan carries on
// from where it got to instead of starting again.

// ScanMode is how much a scan reads again.
type ScanMode string

const (
	// ScanChanges reads only what is new, changed or gone: the usual scan.
	ScanChanges ScanMode = "changes"
	// ScanFull reads every file and its details again from scratch.
	ScanFull ScanMode = "full"
)

// ParseScanMode reads a scan mode; "" is ScanChanges.
func ParseScanMode(s string) (ScanMode, bool) {
	switch ScanMode(strings.TrimSpace(s)) {
	case "", ScanChanges:
		return ScanChanges, true
	case ScanFull:
		return ScanFull, true
	}
	return "", false
}

// StuckAfter is how long a scan may go without anything moving before it's stopped as stuck.
// Tests make it shorter.
var StuckAfter = 10 * time.Minute

// stuckCheck is how often the watchdog looks.
var stuckCheck = 15 * time.Second

// ErrScanStuck: nothing moved for StuckAfter.
var ErrScanStuck = errors.New("the scan was stopped because nothing moved for a long time")

// stuckError says what was stuck, in words the activity panel can show.
type stuckError struct{ what string }

func (e *stuckError) Error() string {
	return fmt.Sprintf("Nothing moved for %s while %s, so the scan was stopped as stuck.", roundMinutes(StuckAfter), e.what)
}

// stuckFix is what to do about a stuck scan.
const stuckFix = "Scan the library again. If it stops at the same place, the log has the details."

func (e *stuckError) Unwrap() error { return ErrScanStuck }

func roundMinutes(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	m := int(d.Round(time.Minute) / time.Minute)
	if m == 1 {
		return "a minute"
	}
	return strconv.Itoa(m) + " minutes"
}

// scanRun is a scan as written down: its mode, when it began, and whether a Full rescan has put
// every item on the list to be described yet (described in batches, which a restart keeps).
type scanRun struct {
	mode    ScanMode
	since   int64
	queued  bool
	resumed bool
}

func (r *scanRun) full() bool { return r != nil && r.mode == ScanFull }

type runKey struct{}

// runOf is the run this scan's context carries; nil outside a scan.
func runOf(ctx context.Context) *scanRun {
	r, _ := ctx.Value(runKey{}).(*scanRun)
	return r
}

// beginRun writes a scan down as it starts. A run already written down (cut short by a restart)
// goes on as it was, unless this one reads more: a Full rescan asked for over an unfinished
// scan for changes starts a full one now.
func (s *Store) beginRun(ctx context.Context, libraryID int64, mode ScanMode) (*scanRun, error) {
	r := &scanRun{}
	var queued int
	err := s.DB.QueryRowContext(ctx, `SELECT mode, started_at, queued FROM library_scans WHERE library_id = ?`, libraryID).Scan(&r.mode, &r.since, &queued)
	switch {
	case err == nil && !(mode == ScanFull && r.mode != ScanFull):
		r.queued, r.resumed = queued != 0, true
		return r, nil
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return nil, err
	}
	r = &scanRun{mode: mode, since: db.Now()}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO library_scans(library_id, mode, started_at, queued) VALUES(?,?,?,0)
		ON CONFLICT(library_id) DO UPDATE SET mode = excluded.mode, started_at = excluded.started_at, queued = 0`, libraryID, string(mode), r.since)
	return r, err
}

// endRun forgets a run: it finished, failed, or a person stopped it. Its describe list goes too.
func (s *Store) endRun(ctx context.Context, libraryID int64) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM library_scans WHERE library_id = ?`, libraryID)
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM scan_describe WHERE library_id = ?`, libraryID)
}

// ScanRun is a scan written down and not finished: running, or cut short by a restart.
type ScanRun struct {
	LibraryID int64    `json:"libraryId"`
	Mode      ScanMode `json:"mode"`
	Since     int64    `json:"since"` // when it began, Unix seconds
}

// ScanRuns are the scans written down and not finished.
func (s *Store) ScanRuns(ctx context.Context) ([]ScanRun, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT library_id, mode, started_at FROM library_scans ORDER BY started_at, library_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScanRun
	for rows.Next() {
		var r ScanRun
		if err := rows.Scan(&r.LibraryID, &r.Mode, &r.Since); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// queueDescribe puts items on a library's list to be described, which a restart keeps.
func (s *Store) queueDescribe(ctx context.Context, libraryID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO scan_describe(item_id, library_id) VALUES(?,?)`, id, libraryID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// queueAllDescribe puts every item of a library on its list to be described, once for a Full
// rescan, and notes that it has.
func (s *Store) queueAllDescribe(ctx context.Context, libraryID int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO scan_describe(item_id, library_id) SELECT id, library_id FROM items WHERE library_id = ? AND missing = 0`, libraryID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_scans SET queued = 1 WHERE library_id = ?`, libraryID); err != nil {
		return err
	}
	return tx.Commit()
}

// toDescribe is a library's list to be described, in the items' order.
func (s *Store) toDescribe(ctx context.Context, libraryID int64) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT item_id FROM scan_describe WHERE library_id = ? ORDER BY item_id`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// described takes items off the list.
func (s *Store) described(ctx context.Context, ids []int64) {
	for _, id := range ids {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM scan_describe WHERE item_id = ?`, id)
	}
}

// liveScan is what's known of a scan while it runs: when something last moved, whether it's
// waiting (its turn, a pause: not stuck), and where a module's reports of its own progress go.
type liveScan struct {
	mu      sync.Mutex
	last    time.Time
	waiting int
	what    string // what it's doing, for when it's stuck
	module  string // the module whose call is under way, "" for none
	report  func(done, total int64)
	mode    ScanMode
	resumed bool
	cancel  context.CancelCauseFunc
}

// moved says something moved.
func (l *liveScan) moved() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.last = time.Now()
	l.mu.Unlock()
}

// doing says what the scan is doing now, which also counts as moving. While a module's call is
// under way, what that call is for says it better.
func (l *liveScan) doing(what string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.last = time.Now()
	if l.module == "" {
		l.what = what
	}
	l.mu.Unlock()
}

// wait marks a stretch in which the scan waits, which isn't being stuck. The func it returns
// ends it.
func (l *liveScan) wait() func() {
	if l == nil {
		return func() {}
	}
	l.mu.Lock()
	l.waiting++
	l.mu.Unlock()
	return func() {
		l.mu.Lock()
		l.waiting--
		l.last = time.Now()
		l.mu.Unlock()
	}
}

// calling marks a module's call under way, saying what it's for: its reports of progress go to
// report.
func (l *liveScan) calling(module, what string, report func(done, total int64)) func() {
	if l == nil {
		return func() {}
	}
	l.mu.Lock()
	l.module, l.what, l.report, l.last = module, what, report, time.Now()
	l.mu.Unlock()
	return func() {
		l.mu.Lock()
		l.module, l.report, l.last = "", nil, time.Now()
		l.mu.Unlock()
	}
}

// watch stops the scan once nothing has moved for StuckAfter, until ctx ends.
func (l *liveScan) watch(ctx context.Context) {
	t := time.NewTicker(stuckCheck)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		l.mu.Lock()
		stuck := l.waiting == 0 && time.Since(l.last) >= StuckAfter
		what := l.what
		l.mu.Unlock()
		if stuck {
			if what == "" {
				what = "scanning"
			}
			l.cancel(&stuckError{what: what})
			return
		}
	}
}

type liveKey struct{}

// liveOf is the scan this context belongs to; nil outside one.
func liveOf(ctx context.Context) *liveScan {
	l, _ := ctx.Value(liveKey{}).(*liveScan)
	return l
}

// scanMoved says, from deep in a scan, that something moved: for work with no count of its own.
func scanMoved(ctx context.Context) { liveOf(ctx).moved() }

// ModuleReport is a module saying how far a scan call it's answering has got (the host API's
// task "scan-<library>"). It counts as the scan moving, and the scan's task shows it. False
// when that module isn't scanning that library now.
func (s *Scanner) ModuleReport(module string, libraryID int64, done, total int64) bool {
	s.mu.Lock()
	l := s.live[libraryID]
	s.mu.Unlock()
	if l == nil {
		return false
	}
	l.mu.Lock()
	ok := l.module == module
	report := l.report
	if ok {
		l.last = time.Now()
	}
	l.mu.Unlock()
	if ok && report != nil && done >= 0 && total >= 0 {
		report(done, total)
	}
	return ok
}

// RunOf is how a library's scan is going now, if one runs or waits to carry on after a restart:
// its mode and when it began.
func (s *Scanner) RunOf(ctx context.Context, libraryID int64) (ScanRun, bool) {
	var r ScanRun
	err := s.Store.DB.QueryRowContext(ctx, `SELECT library_id, mode, started_at FROM library_scans WHERE library_id = ?`, libraryID).Scan(&r.LibraryID, &r.Mode, &r.Since)
	return r, err == nil
}

// Stopping says the server is stopping: a scan that fails from now on (its module's service
// going away under it) is carried on at the next start rather than forgotten.
func (s *Scanner) Stopping() { s.stopping.Store(true) }

// ResumeScans starts again the scans a restart cut short, each as it was asked for.
func (s *Scanner) ResumeScans(ctx context.Context) int {
	runs, err := s.Store.ScanRuns(ctx)
	if err != nil {
		s.Log.WarnContext(ctx, "could not read which scans to carry on", "err", err)
		return 0
	}
	n := 0
	for _, r := range runs {
		if _, err := s.Store.Library(ctx, r.LibraryID); err != nil {
			s.Store.endRun(ctx, r.LibraryID)
			continue
		}
		if s.StartBy(r.LibraryID, r.Mode, tasks.By{}) {
			n++
			s.Log.InfoContext(ctx, "carrying on a scan a restart cut short", "library", r.LibraryID, "mode", string(r.Mode), "began", time.Unix(r.Since, 0).Format(time.RFC3339))
		}
	}
	return n
}
