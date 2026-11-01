package diagnose

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/reset"
)

func dataChecks() []Check {
	return []Check{
		{ID: "database", Area: AreaData, Title: "Is the database sound?", Run: checkDatabase, Fix: fixDatabase},
		{ID: "database-links", Area: AreaData, Title: "Do the database's rows point at each other properly?", Run: checkLinks, Fix: fixLinks},
		{ID: "database-size", Area: AreaData, Title: "Is the database carrying dead weight?", Run: checkDBSize, Fix: fixDBSize},
		{ID: "temp-files", Area: AreaData, Title: "Are there leftover temporary files?", Run: checkTempFiles, Fix: fixTempFiles},
		{ID: "reset-state", Area: AreaData, Title: "Is a reset or repair stuck halfway?", Run: checkResetState, Fix: fixResetState},
		{ID: "restore-state", Area: AreaData, Title: "Is a restore waiting, or stuck part way?", Run: checkRestoreState, Fix: fixRestoreState},
		// No fix on purpose: Settings can run a fix, and getting into an account must not be
		// something anyone can do from another device. A person does this on the machine.
		{ID: "accounts", Area: AreaData, Title: "Can someone sign in as an administrator?", Run: checkAccounts},
	}
}

// --------------------------------------------------------------- can anyone sign in

func checkAccounts(ctx context.Context, e *DoctorEnv) []Issue {
	d, err := e.readDB()
	if err != nil {
		if errors.Is(err, db.ErrNoDatabase) {
			return nil
		}
		return notChecked("accounts:failed", "accounts", "The database couldn't be read: "+err.Error()+".")
	}
	var accounts, admins int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(is_admin = 1 AND password_hash <> ''), 0) FROM users`).Scan(&accounts, &admins); err != nil {
		return notChecked("accounts:failed", "accounts", "The accounts couldn't be read: "+err.Error()+".")
	}
	if accounts == 0 || admins > 0 {
		return nil
	}
	return []Issue{{Key: "accounts:no-admin", Severity: SevProblem, Title: "No administrator can sign in", Safety: SafetyManual,
		What: "There are accounts here, but none is an administrator with a password, so nobody can change this server's settings.",
		Fix:  "Give an account a new password from a terminal on this machine. It's kept off this page, so that it can't be done from another device.",
		Steps: []string{"On this machine, run: wittymediaserver reset-password --list",
			"Then run: wittymediaserver reset-password <username>",
			"If nobody on the list is an administrator, a backup or a reset is the way back. See Backup and restore."}}}
}

// --------------------------------------------------------------- is the database sound

var indexOnly = regexp.MustCompile(`^(row \d+ missing from index |wrong # of entries in index |non-unique entry in index )`)

type damage struct{ lines []string }

func checkDatabase(ctx context.Context, e *DoctorEnv) []Issue {
	d, err := e.readDB()
	if errors.Is(err, db.ErrNoDatabase) {
		if _, statErr := os.Stat(filepath.Join(e.Data, "wms.db-wal")); statErr == nil {
			e.damaged = "the database file is missing"
			return []Issue{{Key: "database:gone", Severity: SevProblem, Title: "The database file is missing", Safety: SafetyManual,
				What:  "Its journal is there, but wms.db itself isn't, so it was moved or deleted while the server had it.",
				Fix:   "Put wms.db back if you moved it, or restore a backup.",
				Steps: []string{"Look for wms.db where you may have moved it, and put it back in " + e.Data + ".", "If it has gone, restore a backup in Settings, Backup, or from first-time setup."}}}
		}
		e.damaged = "there's no database yet"
		return []Issue{{Key: "database:new", Severity: SevInfo, Title: "This data folder has no database yet", Safety: SafetyManual,
			What: "That's what a new install looks like. The server makes one when it first starts."}}
	}
	if err != nil {
		e.damaged = err.Error()
		return []Issue{{Key: "database:unreadable", Severity: SevProblem, Title: "The database can't be opened", Safety: SafetyCareful, Exclusive: true,
			What: "SQLite says: " + err.Error() + ".",
			Fix:  "Builds a new database from whatever can still be read, and keeps the old file. If nothing can be read, nothing is replaced, and a backup is the way back.",
			data: damage{}}}
	}
	lines, err := integrityLines(ctx, d)
	if err != nil {
		e.damaged = err.Error()
		return []Issue{{Key: "database:unreadable", Severity: SevProblem, Title: "The database can't be read to the end", Safety: SafetyCareful, Exclusive: true,
			What: "Checking it stopped with: " + err.Error() + ".",
			Fix:  "Builds a new database from whatever can still be read, and keeps the old file.",
			data: damage{}}}
	}
	if len(lines) == 0 {
		return nil
	}
	e.damaged = lines[0]
	allIndex := true
	for _, l := range lines {
		if !indexOnly.MatchString(l) {
			allIndex = false
			break
		}
	}
	if allIndex {
		e.damaged = ""
		return []Issue{{Key: "database:indexes", Severity: SevWarn, Title: "The database's indexes are out of step with its rows", Safety: SafetySafe, Count: len(lines), Details: lines,
			What: "Your data is intact, but lists can come out wrong or incomplete until the indexes are rebuilt.",
			Fix:  "Rebuilds every index from the rows. Nothing is lost.",
			data: "reindex"}}
	}
	return []Issue{{Key: "database:damaged", Severity: SevProblem, Title: "The database is damaged", Safety: SafetyCareful, Exclusive: true, Count: len(lines), Details: lines,
		What: "Part of the file can't be trusted. A power cut, a failing drive or a copy taken while the server was running can all do this.",
		Fix:  "Builds a new database from whatever can still be read, says what couldn't be saved, and keeps the old file. If you have a recent backup, restoring it may save more.",
		data: damage{lines}}}
}

func integrityLines(ctx context.Context, d *db.DB) (out []string, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("the database engine stopped: %v", p)
		}
	}()
	rows, err := d.QueryContext(ctx, `PRAGMA integrity_check(50)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		if s != "ok" {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}

func fixDatabase(ctx context.Context, e *DoctorEnv, i *Issue, f *FixContext) (string, error) {
	if i.data == "reindex" {
		err := withWrite(ctx, e, func(d *db.DB) error {
			_, err := d.ExecContext(ctx, `REINDEX`)
			return err
		})
		if err != nil {
			return "", err
		}
		return "Every index was rebuilt.", nil
	}
	e.closeDB()
	return recoverDatabase(ctx, e, f)
}

// recoverDatabase builds the new file beside the old one. With a server running, or being
// the server, the swap waits for the next start; with none, it happens now.
func recoverDatabase(ctx context.Context, e *DoctorEnv, f *FixContext) (string, error) {
	stage, err := reset.NewRepairStage(e.Data, e.now())
	if err != nil {
		return "", err
	}
	stagePath := filepath.Join(e.Data, stage)
	res, err := db.RecoverInto(ctx, e.Data, stagePath)
	if err != nil {
		_ = os.RemoveAll(stagePath)
		return "", fmt.Errorf("the new database couldn't be built, and the old one wasn't touched: %w", err)
	}
	if res.Copied == 0 {
		_ = os.RemoveAll(stagePath)
		return "", errors.New("nothing could be read from the damaged database, so it wasn't replaced. Restore a backup, or use Settings, Reset to start again. A copy of the file is kept in " + f.copyDir)
	}
	said := fmt.Sprintf("%d rows were saved in the new database.", res.Copied)
	if res.Lost > 0 || res.Refused > 0 {
		said += fmt.Sprintf(" About %d rows couldn't be read or weren't valid.", res.Lost+res.Refused)
	}
	if res.Unlinked > 0 {
		said += fmt.Sprintf(" %d more rows pointed at something lost, and were left out.", res.Unlinked)
	}
	var lost []string
	for _, t := range res.Tables {
		if t.Lost > 0 || t.Refused > 0 || t.Failed {
			lost = append(lost, t.Name)
		}
	}
	if len(lost) > 0 {
		said += " Affected: " + strings.Join(lost, ", ") + "."
		if slices.Contains(lost, "items") {
			said += " A scan of your libraries finds titles whose files are still there again, without what was known about them."
		}
	}
	rel, err := filepath.Rel(e.Data, f.copyDir)
	if err != nil {
		return "", err
	}
	if err := reset.AskRepair(e.Data, reset.RepairRequest{Stage: stage, KeepIn: rel, At: e.now()}); err != nil {
		_ = os.RemoveAll(stagePath)
		return "", err
	}
	if e.ServerUp() {
		f.restart = true
		return said + " It takes over when the server next starts, and the damaged file is kept in the safety copy.", nil
	}
	r := reset.ApplyRepair(e.Data)
	if r == nil || !r.OK {
		why := "it couldn't be swapped in"
		if r != nil {
			why = r.Error
		}
		return "", errors.New(why + ". The old database is still in place.")
	}
	return said + " It is now in use, and the damaged file is kept in the safety copy.", nil
}

// --------------------------------------------------------------- rows pointing at nothing

type fkRow struct {
	table  string
	rowid  int64
	parent string
}

func checkLinks(ctx context.Context, e *DoctorEnv) []Issue {
	if e.damaged != "" {
		return nil // the first check has spoken; these wouldn't mean anything yet
	}
	d, err := e.readDB()
	if err != nil {
		return nil
	}
	rows, err := d.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return notChecked("database-links:failed", "links between rows", "The check couldn't run: "+err.Error()+".")
	}
	defer rows.Close()
	counts := map[string]int{}
	total := 0
	for rows.Next() {
		var t, parent string
		var rowid *int64
		var fk int64
		if err := rows.Scan(&t, &rowid, &parent, &fk); err != nil {
			return notChecked("database-links:failed", "links between rows", "The check couldn't run: "+err.Error()+".")
		}
		counts[t+" points at something missing in "+parent]++
		total++
	}
	if total == 0 {
		return nil
	}
	var details []string
	for k, n := range counts {
		details = append(details, fmt.Sprintf("%d rows of %s", n, k))
	}
	return []Issue{{Key: "database-links", Severity: SevWarn, Title: "Some rows point at things that no longer exist", Safety: SafetyCareful, Count: total, Details: details,
		What: "These are left over from something that was removed, such as watch progress for a title that's gone.",
		Fix:  "Removes those leftover rows. They can't be used by anything. A safety copy is made first."}}
}

func fixLinks(ctx context.Context, e *DoctorEnv, _ *Issue, _ *FixContext) (string, error) {
	var removed int64
	err := withWrite(ctx, e, func(d *db.DB) error {
		for range 8 {
			rows, err := d.QueryContext(ctx, `PRAGMA foreign_key_check`)
			if err != nil {
				return err
			}
			var list []fkRow
			for rows.Next() {
				var r fkRow
				var id *int64
				var fk int64
				if err := rows.Scan(&r.table, &id, &r.parent, &fk); err != nil {
					rows.Close()
					return err
				}
				if id != nil {
					r.rowid = *id
					list = append(list, r)
				}
			}
			rows.Close()
			if len(list) == 0 {
				return nil
			}
			tx, err := d.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			for _, r := range list {
				res, err := tx.ExecContext(ctx, `DELETE FROM `+db.QuoteIdent(r.table)+` WHERE rowid = ?`, r.rowid)
				if err != nil {
					_ = tx.Rollback()
					return err
				}
				n, _ := res.RowsAffected()
				removed += n
			}
			if err := tx.Commit(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d leftover rows were removed.", removed), nil
}

// --------------------------------------------------------------- dead weight

const (
	walTooBig    = 64 << 20
	freeTooBig   = 128 << 20
	freeTooShare = 4 // a quarter of the file
)

func checkDBSize(ctx context.Context, e *DoctorEnv) []Issue {
	if e.damaged != "" {
		return nil
	}
	d, err := e.readDB()
	if err != nil {
		return nil
	}
	var out []Issue
	if st, err := os.Stat(filepath.Join(e.Data, "wms.db-wal")); err == nil && st.Size() > walTooBig && !e.ServerUp() {
		out = append(out, Issue{Key: "database-size:wal", Severity: SevInfo, Title: "The database's journal has grown large", Safety: SafetySafe,
			What: fmt.Sprintf("Its journal is %d MB and nothing is using it. Starts are slower until it's folded in.", st.Size()>>20),
			Fix:  "Folds the journal into the database. Nothing is lost.", data: "checkpoint"})
	}
	var pageSize, free, pages int64
	_ = d.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize)
	_ = d.QueryRowContext(ctx, `PRAGMA freelist_count`).Scan(&free)
	_ = d.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pages)
	if pages > 0 && free*pageSize > freeTooBig && free*freeTooShare > pages {
		out = append(out, Issue{Key: "database-size:vacuum", Severity: SevInfo, Title: "The database has empty space it could give back", Safety: SafetySafe,
			What: fmt.Sprintf("%d MB of the file is empty, left by things that were removed.", free*pageSize>>20),
			Fix:  "Compacts the file. Nothing is lost, but it's slow on a big database, and the server waits meanwhile.", data: "vacuum"})
	}
	return out
}

func fixDBSize(ctx context.Context, e *DoctorEnv, i *Issue, _ *FixContext) (string, error) {
	q, said := `PRAGMA wal_checkpoint(TRUNCATE)`, "The journal was folded into the database."
	if i.data == "vacuum" {
		q, said = `VACUUM`, "The database was compacted."
	}
	err := withWrite(ctx, e, func(d *db.DB) error {
		_, err := d.ExecContext(ctx, q)
		return err
	})
	if err != nil {
		return "", err
	}
	return said, nil
}

// --------------------------------------------------------------- leftover temporary files

const tempAge = 24 * time.Hour

var (
	tempInData   = regexp.MustCompile(`(?i)(\.tmp|\.partial|\.part)$`)
	tempInImages = regexp.MustCompile(`^(\.game-|cover-|r-.*\.tmp$)`)
)

type tempFiles struct {
	moved  []string // these move to the safety copy
	remove []string // made-again working files: see fixTempFiles
}

func checkTempFiles(_ context.Context, e *DoctorEnv) []Issue {
	now := e.now()
	t := &tempFiles{}
	var names []string
	add := func(p string, d fs.DirEntry) {
		info, err := d.Info()
		if err == nil && olderThan(info, now, tempAge) {
			t.moved = append(t.moved, p)
			names = append(names, d.Name())
		}
	}
	if entries, err := os.ReadDir(e.Data); err == nil {
		for _, d := range entries {
			if !d.IsDir() && tempInData.MatchString(d.Name()) {
				add(filepath.Join(e.Data, d.Name()), d)
			}
		}
	}
	if entries, err := os.ReadDir(e.Images); err == nil {
		for _, d := range entries {
			if !d.IsDir() && tempInImages.MatchString(d.Name()) {
				add(filepath.Join(e.Images, d.Name()), d)
			}
		}
	}
	var size int64
	// With no server running, the folders of old streams and the place game files unpack to
	// are left over. With one running, some of it is in use. The conversion folder can be
	// anywhere the owner chose, with anything else in it, so only a stream's folder counts,
	// as when the server starts up.
	if !e.ServerUp() {
		for _, dir := range []string{e.Transcode, filepath.Join(e.Data, "gaming", "tmp")} {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, d := range entries {
				if dir == e.Transcode && !(d.IsDir() && hls.IsSessionDir(d.Name())) {
					continue
				}
				p := filepath.Join(dir, d.Name())
				t.remove = append(t.remove, p)
				size += treeSize(p)
			}
		}
	}
	n := len(t.moved) + len(t.remove)
	if n == 0 {
		return nil
	}
	what := fmt.Sprintf("%s left behind by something that stopped partway.", countOf(n, "temporary file or folder", "temporary files and folders"))
	if size >= 1<<20 {
		what += fmt.Sprintf(" %d MB of them are converted video and unpacked game files.", size>>20)
	}
	return []Issue{{Key: "temp-files", Severity: SevWarn, Title: "Leftover temporary files", Safety: SafetySafe, Count: n, Names: names, data: t, What: what,
		Fix: "Moves stray files to the safety copy, and removes the leftover converted video and unpacked game files, which the server makes again when they're needed."}}
}

func treeSize(p string) int64 {
	var n int64
	_ = filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if i, err := d.Info(); err == nil {
				n += i.Size()
			}
		}
		return nil
	})
	return n
}

// fixTempFiles keeps stray files in the safety copy, but not converted video. That can be
// many gigabytes, is never anyone's only copy of anything, and isn't worth a safety copy.
func fixTempFiles(_ context.Context, e *DoctorEnv, i *Issue, f *FixContext) (string, error) {
	t, ok := i.data.(*tempFiles)
	if !ok {
		return "", errors.New("run the check again first")
	}
	for _, p := range t.moved {
		if err := f.Remove(p); err != nil {
			return "", err
		}
	}
	if e.ServerUp() && len(t.remove) > 0 {
		return "", errors.New("a server has started since the check, so its working files were left alone")
	}
	for _, p := range t.remove {
		if err := os.RemoveAll(p); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%s cleared.", countOf(len(t.moved)+len(t.remove), "temporary file or folder", "temporary files and folders")), nil
}

// --------------------------------------------------------------- a reset or repair stuck

const staleRequest = 10 * time.Minute

func checkResetState(_ context.Context, e *DoctorEnv) []Issue {
	var out []Issue
	now := e.now()
	reqPath := filepath.Join(e.Data, "reset-request.json")
	if b, err := os.Stat(reqPath); err == nil {
		switch r := reset.Pending(e.Data); {
		case r == nil:
			out = append(out, Issue{Key: "reset-state:unreadable", Severity: SevWarn, Title: "A reset request can't be read", Safety: SafetySafe, data: "reset",
				What: "reset-request.json is there, but it isn't a request this version understands. The server would ignore it.",
				Fix:  "Moves the file to the safety copy. Nothing else changes."})
		case now.Sub(b.ModTime()) > staleRequest:
			out = append(out, Issue{Key: "reset-state:stale", Severity: SevWarn, Title: "A reset is still waiting from " + b.ModTime().Format("2 Jan 2006 15:04"), Safety: SafetyCareful, data: "reset",
				What: "Someone asked for this server to be reset, and it will happen the next time the server starts. Every account and setting would go" + resetKeeps(r.Mode) + ".",
				Fix:  "Takes the request back, so the next start is an ordinary one. If you do want the reset, leave this and restart the server."})
		}
	}
	if b, err := os.Stat(filepath.Join(e.Data, "repair-request.json")); err == nil && now.Sub(b.ModTime()) > staleRequest {
		out = append(out, Issue{Key: "reset-state:repair", Severity: SevWarn, Title: "A repaired database is waiting from " + b.ModTime().Format("2 Jan 2006 15:04"), Safety: SafetyCareful, data: "repair",
			What: "A new database was built by an earlier repair, and takes over at the next start. It may be older than what's in use now.",
			Fix:  "Takes the request back and removes the new database. The one in use stays. If you do want it, leave this and restart the server."})
	}
	if l := reset.Last(e.Data); l != nil && !l.OK && l.Error != "" {
		if b, err := os.Stat(filepath.Join(e.Data, "reset-result.json")); err == nil {
			if now.Sub(b.ModTime()) < 7*24*time.Hour {
				out = append(out, Issue{Key: "reset-state:failed", Severity: SevInfo, Title: "The last reset didn't happen", Safety: SafetyManual,
					What: "It stopped with: " + l.Error + ". Nothing was changed by it, and you can try it again in Settings, Reset."})
			}
		}
	}
	// Half done: the data moved into a copy, and the server never made a new database.
	copies := reset.Copies(e.Data)
	if len(copies) > 0 {
		_, dbErr := os.Stat(filepath.Join(e.Data, "wms.db"))
		if errors.Is(dbErr, fs.ErrNotExist) {
			out = append(out, Issue{Key: "reset-state:half", Severity: SevProblem, Title: "A reset stopped halfway", Safety: SafetyCareful, Exclusive: true, data: copies[0].Name,
				What: "There's no database here, but a copy from a reset is: " + copies[0].Name + ". Your data is in that copy.",
				Fix:  "Brings that copy back the next time the server starts. If you meant to start again from new, leave this and just start the server: it makes a new database."})
		} else {
			var total int64
			for _, c := range copies {
				total += c.Bytes
			}
			out = append(out, Issue{Key: "reset-state:copies", Severity: SevInfo, Title: "Copies from earlier resets are taking up space", Safety: SafetyManual, Count: len(copies),
				What: fmt.Sprintf("%s, %d MB in all.", countOf(len(copies), "copy", "copies"), total>>20),
				Fix:  "Delete the ones you don't need under Settings, Reset. Nothing here removes them by itself."})
		}
	}
	return out
}

func resetKeeps(mode string) string {
	switch mode {
	case reset.ModeDelete:
		return " for good, along with copies from earlier resets"
	case reset.ModeRestore:
		return ", replaced by a copy from an earlier reset"
	}
	return ", with a copy kept"
}

func fixResetState(_ context.Context, e *DoctorEnv, i *Issue, f *FixContext) (string, error) {
	switch v := i.data.(type) {
	case string:
		switch v {
		case "reset":
			if err := f.Remove(filepath.Join(e.Data, "reset-request.json")); err != nil {
				return "", err
			}
			return "The waiting reset was taken back. The next start is an ordinary one.", nil
		case "repair":
			if err := reset.CancelRepair(e.Data); err != nil {
				return "", err
			}
			return "The waiting database was removed, and the one in use stays.", nil
		default:
			if err := reset.Ask(e.Data, reset.Request{Mode: reset.ModeRestore, Copy: v, At: e.now()}); err != nil {
				return "", err
			}
			if e.ServerUp() {
				f.restart = true
			}
			return "That copy comes back when the server next starts. What's there now is kept as another copy.", nil
		}
	}
	return "", errors.New("run the check again first")
}
