package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/metafolder"
)

// The database in an archive is never used as it is. It's read as data: a clean database
// is built from this program's own migrations, at the version the archive says it has,
// the rows are copied into it, and that's brought up to date the normal way. So a trigger,
// view, generated column or odd index in the file has nowhere to go, and a database from
// an older version arrives migrated. The rows themselves are then checked: paths, picture
// references, settings that name programs, and text that holds markup.

// inspectDatabase does all of that in the work folder, and describes the result.
func inspectDatabase(ctx context.Context, in *Inspection, files []*zip.File, f Folders) error {
	var entry *zip.File
	for _, e := range files {
		if e.Name == dbEntry {
			entry = e
		}
	}
	if entry == nil {
		return nil
	}
	// It comes out twice over: as it is, and rebuilt. Most of what a ZIP bomb's ratio limit lets
	// through is this, and it happens before anyone has agreed to anything.
	if need := 2*int64(entry.UncompressedSize64) + 64<<20; int64(entry.UncompressedSize64) <= maxDatabase {
		if free := freeBytes(in.workDir); free >= 0 && free < need {
			return fmt.Errorf("there isn't room to check this backup: its database needs about %d MB, and the drive has %d MB free", need>>20, free>>20)
		}
	}
	archive := filepath.Join(in.workDir, "archive.db")
	if err := extractLimited(entry, archive, maxDatabase); err != nil {
		return Refused(dbEntry, err.Error())
	}
	head := readHead(archive)
	if !bytes.HasPrefix(head, []byte("SQLite format 3\x00")) {
		return Refused(dbEntry, "it isn't a database")
	}
	cleanDir := filepath.Join(in.workDir, "clean")
	if err := os.MkdirAll(cleanDir, 0o700); err != nil {
		return err
	}
	clean := filepath.Join(cleanDir, "wms.db")
	v, err := copyIntoClean(ctx, archive, clean, in.man.Schema)
	if err != nil {
		return err
	}
	in.man.Schema = v
	_ = os.Remove(archive)

	// Forward to this version, by the same path a server takes when it's updated.
	d, err := db.Open(cleanDir)
	if err != nil {
		return Refused(dbEntry, "it couldn't be brought up to date: "+err.Error())
	}
	d.Close()

	w, err := db.OpenPlainFile(clean)
	if err != nil {
		return err
	}
	defer w.Close()
	if err := sanitizeRows(ctx, w, in, f); err != nil {
		return err
	}
	if _, err := db.RemoveOrphans(ctx, w); err != nil {
		return fmt.Errorf("tidying the database: %w", err)
	}
	if err := describeDatabase(ctx, w, in); err != nil {
		return err
	}
	return nil
}

func extractLimited(e *zip.File, to string, limit int64) error {
	if int64(e.UncompressedSize64) > limit {
		return errors.New("it's too large")
	}
	rc, err := e.Open()
	if err != nil {
		return errors.New("it can't be read")
	}
	defer rc.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(rc, limit+1))
	cerr := out.Close()
	if err != nil {
		return errors.New("it's damaged")
	}
	if cerr != nil {
		return cerr
	}
	if n > limit || n != int64(e.UncompressedSize64) {
		return errors.New("it isn't the size the archive says")
	}
	return nil
}

// safeIdent keeps a table or column name from an archive from being anything but a name.
var safeIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func quote(name string) string { return `"` + name + `"` }

// objectNames lists a database's objects of one kind.
func objectNames(ctx context.Context, d *sql.DB, kind string) ([]string, error) {
	rows, err := d.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = ? ORDER BY name`, kind)
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

func columnsOf(ctx context.Context, d *sql.DB, table string) (map[string]bool, []string, error) {
	rows, err := d.QueryContext(ctx, `SELECT name, hidden FROM pragma_table_xinfo(?)`, table)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	set := map[string]bool{}
	var list []string
	for rows.Next() {
		var n string
		var hidden int
		if err := rows.Scan(&n, &hidden); err != nil {
			return nil, nil, err
		}
		if hidden != 0 {
			return nil, nil, Refused(dbEntry, "table "+printableName(table)+" has a computed column, which this server's database never has")
		}
		set[n] = true
		list = append(list, n)
	}
	return set, list, rows.Err()
}

// copyIntoClean checks the file, then copies its rows into a new database built by this
// program. The file is opened to read, with its own schema not trusted to run anything.
func copyIntoClean(ctx context.Context, archive, clean string, said db.Versions) (db.Versions, error) {
	old, err := db.OpenForeign(archive)
	if err != nil {
		return said, Refused(dbEntry, "it can't be opened")
	}
	defer old.Close()
	var ok string
	if err := old.QueryRowContext(ctx, `PRAGMA integrity_check(1)`).Scan(&ok); err != nil {
		return said, Refused(dbEntry, "it's damaged: "+shortErr(err))
	}
	if ok != "ok" {
		return said, Refused(dbEntry, "it's damaged: "+printableName(ok))
	}
	v, err := db.ReadVersions(ctx, old, "")
	if err != nil {
		return said, Refused(dbEntry, shortErr(err))
	}
	if v != said {
		return said, Refused(dbEntry, "its version isn't the one the manifest says")
	}
	if v.Newer(db.CurrentVersions()) {
		return said, &Refusal{Reason: "its database is from a newer version. Update this server first"}
	}
	base, err := db.BuildAt(ctx, clean, v)
	if err != nil {
		return said, Refused(dbEntry, "its version can't be rebuilt: "+shortErr(err))
	}
	defer base.Close()

	// What the file holds that this version's schema doesn't is what someone added.
	baseTables, err := objectNames(ctx, base, "table")
	if err != nil {
		return said, err
	}
	baseTriggers, err := objectNames(ctx, base, "trigger")
	if err != nil {
		return said, err
	}
	allowed := map[string]bool{}
	for _, t := range baseTriggers {
		allowed[t] = true
	}
	if views, err := objectNames(ctx, old, "view"); err != nil {
		return said, Refused(dbEntry, shortErr(err))
	} else if len(views) > 0 {
		return said, Refused(dbEntry, "it has a view ("+printableName(views[0])+"), which this server's database never has")
	}
	triggers, err := objectNames(ctx, old, "trigger")
	if err != nil {
		return said, Refused(dbEntry, shortErr(err))
	}
	for _, t := range triggers {
		if !allowed[t] {
			return said, Refused(dbEntry, "it has a trigger this server doesn't know ("+printableName(t)+"), so its SQL has been changed")
		}
	}
	var virtual int
	if err := old.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND sql LIKE 'CREATE VIRTUAL%'`).Scan(&virtual); err != nil {
		return said, Refused(dbEntry, shortErr(err))
	}
	if virtual > 0 {
		return said, Refused(dbEntry, "it has a virtual table, which this server's database never has")
	}

	inArchive := map[string]bool{}
	tables, err := objectNames(ctx, old, "table")
	if err != nil {
		return said, Refused(dbEntry, shortErr(err))
	}
	for _, t := range tables {
		inArchive[t] = true
	}
	// Rows are read one table at a time, through a second connection, and written here.
	for _, t := range baseTables {
		if !inArchive[t] || t == "schema" || t == "sqlite_sequence" || neverTables[t] || !safeIdent.MatchString(t) {
			continue
		}
		if err := copyTableRows(ctx, old, base, t); err != nil {
			return said, err
		}
	}
	return v, nil
}

// copyTableRows moves one table's rows across as values, by column name.
func copyTableRows(ctx context.Context, old, base *sql.DB, table string) error {
	oldCols, _, err := columnsOf(ctx, old, table)
	if err != nil {
		var r *Refusal
		if errors.As(err, &r) {
			return err
		}
		return Refused(dbEntry, shortErr(err))
	}
	_, baseCols, err := columnsOf(ctx, base, table)
	if err != nil {
		return err
	}
	var cols []string
	for _, c := range baseCols {
		if oldCols[c] && safeIdent.MatchString(c) {
			cols = append(cols, quote(c))
		}
	}
	if len(cols) == 0 {
		return nil
	}
	list := strings.Join(cols, ",")
	where := ""
	if table == "settings" {
		where = ` WHERE key NOT LIKE 'schema.%'` // the clean database's own are the true ones
	}
	rows, err := old.QueryContext(ctx, `SELECT `+list+` FROM `+quote(table)+where)
	if err != nil {
		return Refused(dbEntry, "table "+table+" can't be read: "+shortErr(err))
	}
	defer rows.Close()
	marks := strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",")
	tx, err := base.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ins, err := tx.PrepareContext(ctx, `INSERT INTO `+quote(table)+`(`+list+`) VALUES (`+marks+`)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return Refused(dbEntry, "table "+table+" can't be read: "+shortErr(err))
		}
		if _, err := ins.ExecContext(ctx, vals...); err != nil {
			return Refused(dbEntry, "a row in table "+table+" doesn't fit: "+shortErr(err))
		}
	}
	if err := rows.Err(); err != nil {
		return Refused(dbEntry, "table "+table+" can't be read: "+shortErr(err))
	}
	return tx.Commit()
}

func shortErr(err error) string { return printableName(err.Error()) }

// Settings that name programs or folders on another machine. The server starts programs
// and writes files by these, so they aren't taken from a file, whoever made it. Remote
// Access is off after a restore until someone turns it on again from home.
var (
	placeSetting  = regexp.MustCompile(`(?i)(path|dir|folder|exe|binary|command|cmd|program)$`)
	alwaysDropped = map[string]bool{"remoteAccess": true, "trustedProxies": true, "publicUrl": true, "portMapping": true, "backupKey": true, "backupFolder": true, "backupLast": true,
		// Custom cores are programs, which no backup holds: turning them on is for this machine's owner.
		"gamingCustomCores": true}
	// Settings that hold addresses the server will contact. Kept only from a protected
	// archive, as nobody can have changed one of those without the password.
	addressSetting = map[string]bool{"agents": true, "lyricsMirror": true, "moduleSources": true, "moduleStores": true}
)

// looksLikeAddress finds a setting holding a web address, whatever it's called.
var looksLikeAddress = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://`)

func sanitizeSettings(ctx context.Context, d *sql.DB, in *Inspection) error {
	rows, err := d.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return err
	}
	type kv struct{ k, v string }
	var all []kv
	for rows.Next() {
		var x kv
		if err := rows.Scan(&x.k, &x.v); err != nil {
			rows.Close()
			return err
		}
		all = append(all, x)
	}
	rows.Close()
	var places, offs, addrs []string
	for _, x := range all {
		if strings.HasPrefix(x.k, "schema.") {
			continue
		}
		drop := false
		switch {
		case alwaysDropped[x.k]:
			drop = true
			offs = append(offs, x.k)
		case placeSetting.MatchString(x.k):
			drop = true
			places = append(places, x.k)
		case !in.man.Protected && isSecretSetting(x.k, nil):
			// Making a backup with no password leaves these out, so one in the file was put
			// there afterwards: this server's identity, or a key of someone else's.
			drop = true
		case !in.man.Protected && (addressSetting[x.k] || looksLikeAddress.MatchString(x.v)):
			drop = true
			addrs = append(addrs, x.k)
		}
		if drop {
			if _, err := d.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, x.k); err != nil {
				return err
			}
		}
	}
	if len(places) > 0 {
		in.Dropped = append(in.Dropped, "Settings that name a program or folder on the old machine, which you set again here: "+strings.Join(places, ", ")+".")
	}
	if len(offs) > 0 {
		in.Notes = append(in.Notes, "Remote Access stays off after a restore until you turn it on again from home.")
	}
	if len(addrs) > 0 {
		in.Dropped = append(in.Dropped, "Settings holding web addresses, since this backup has no password to prove nobody changed them: "+strings.Join(addrs, ", ")+".")
	}
	return nil
}

// A path from an archive compared with a folder from an archive. It may be from another
// system, so this works on the text, not on this machine's idea of a path.
func normalizePath(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	if len(p) > 1 && p[1] == ':' {
		p = strings.ToLower(p)
	}
	return strings.TrimRight(path.Clean(p), "/")
}

func looksAbsolute(p string) bool {
	return strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\\`) || (len(p) > 2 && p[1] == ':' && (p[2] == '\\' || p[2] == '/'))
}

func under(p string, roots []string) bool {
	n := normalizePath(p)
	for _, r := range roots {
		if n == r || strings.HasPrefix(n, r+"/") {
			return true
		}
	}
	return false
}

// Places a library must never be pointed at, whatever a file says.
var systemRoots = []string{"/etc", "/proc", "/sys", "/dev", "/boot", "/root", "/run", "/bin", "/sbin", "/lib", "/lib64", "/usr", "/var/lib", "c:/windows", "c:/program files", "c:/program files (x86)", "c:/programdata", "/System", "/Library"}

func dangerousRoot(p string, f Folders) string {
	n := normalizePath(p)
	if !looksAbsolute(p) {
		return "it isn't a full path"
	}
	if s := strings.ReplaceAll(p, `\`, "/"); strings.HasPrefix(s, "//?/") || strings.HasPrefix(s, "//./") {
		return "it's a device path, which nobody needs for a library"
	}
	if n == "" || n == "." || (len(n) == 2 && n[1] == ':') {
		return "it's a whole drive's top level"
	}
	if strings.Contains(strings.ReplaceAll(p, `\`, "/"), "/../") {
		return "it steps out of itself"
	}
	for _, s := range systemRoots {
		if n == s || strings.HasPrefix(n, s+"/") {
			return "it's a system folder"
		}
	}
	if under(p, []string{normalizePath(f.Data)}) {
		return "it's inside the server's own data folder"
	}
	return ""
}

// Columns that hold pictures. Each has to be a picture this server already serves safely.
var artColumns = []struct{ table, col string }{
	{"items", "poster"}, {"items", "backdrop"}, {"shows", "poster"}, {"shows", "backdrop"}, {"seasons", "poster"},
	{"creators", "avatar"}, {"creators", "banner"}, {"channels", "logo"}, {"title_meta", "banner"}, {"title_meta", "logo"},
	{"tuner_channels", "logo"}, {"podcasts", "image"},
}

// Text shown to people. Markup never reaches a page as markup, and it's taken out here too.
var textColumns = []struct {
	table, col string
	max        int
	lines      bool
}{
	{"items", "title", 500, false}, {"items", "sort_title", 500, false}, {"items", "overview", 20000, true}, {"items", "tagline", 1000, false}, {"items", "studio", 500, false},
	{"shows", "title", 500, false}, {"shows", "sort_title", 500, false}, {"shows", "overview", 20000, true}, {"shows", "network", 500, false},
	{"seasons", "title", 500, false}, {"seasons", "overview", 20000, true},
	{"libraries", "name", 200, false}, {"channels", "name", 200, false}, {"channels", "description", 5000, true},
	{"collections", "name", 500, false}, {"collections", "description", 5000, true},
	{"photo_albums", "name", 500, false}, {"photo_albums", "description", 5000, true},
	{"item_comments", "author", 200, false}, {"item_comments", "body", 20000, true}, {"annotations", "note", 20000, true},
	{"creators", "name", 500, false}, {"creators", "description", 20000, true},
	{"podcasts", "title", 500, false}, {"podcasts", "author", 500, false}, {"podcasts", "description", 20000, true},
	{"users", "name", 200, false}, {"users", "display_name", 200, false},
}

func sanitizeRows(ctx context.Context, d *sql.DB, in *Inspection, f Folders) error {
	if _, err := d.ExecContext(ctx, `DELETE FROM sessions`); err != nil {
		return err
	}
	if err := sanitizeSettings(ctx, d, in); err != nil {
		return err
	}
	// Library folders first, as paths are checked against what's left.
	type lp struct {
		id   int64
		path string
	}
	var paths []lp
	rows, err := d.QueryContext(ctx, `SELECT id, path FROM library_paths`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var x lp
		if err := rows.Scan(&x.id, &x.path); err != nil {
			rows.Close()
			return err
		}
		paths = append(paths, x)
	}
	rows.Close()
	var roots []string
	for _, x := range paths {
		why := dangerousRoot(x.path, f)
		// Scanning a share sends this machine's sign-in to it. One from a backup with a password
		// is shown before anyone agrees, and with none it has to be added again.
		if why == "" && !in.man.Protected && strings.HasPrefix(strings.ReplaceAll(x.path, `\`, "/"), "//") {
			why = "it's on another machine, and this backup has no password to prove it's yours. Add it again under Libraries"
		}
		if why != "" {
			in.Dropped = append(in.Dropped, "The library folder "+printableName(x.path)+", because "+why+".")
			if _, err := d.ExecContext(ctx, `DELETE FROM library_paths WHERE id = ?`, x.id); err != nil {
				return err
			}
			continue
		}
		roots = append(roots, normalizePath(x.path))
	}
	// Anything with a file path must be inside a library folder. Otherwise a row in a
	// file could make the server serve any file on this machine.
	for _, t := range []struct{ table, col string }{{"items", "path"}, {"shows", "path"}, {"game_archives", "path"}, {"game_files", "path"}, {"game_review", "path"}, {"game_decisions", "path"}, {"game_discs", "path"}} {
		n, err := dropOutsideRoots(ctx, d, t.table, t.col, roots)
		if err != nil {
			return err
		}
		if n > 0 {
			in.Dropped = append(in.Dropped, fmt.Sprintf("%d rows in %s with a path outside every library folder.", n, t.table))
		}
	}
	for _, c := range artColumns {
		if err := sanitizeArt(ctx, d, c.table, c.col, roots); err != nil {
			return err
		}
	}
	for _, c := range textColumns {
		if err := sanitizeText(ctx, d, c.table, c.col, c.max, c.lines); err != nil {
			return err
		}
	}
	if err := sanitizeUsers(ctx, d); err != nil {
		return err
	}
	if err := sanitizeRecordings(ctx, d, in, f); err != nil {
		return err
	}
	if _, err := d.ExecContext(ctx, `UPDATE logos SET file = '' WHERE file <> '' AND (file LIKE '%/%' OR file LIKE '%\%' OR file LIKE '%..%')`); err != nil {
		return err
	}
	if !in.man.Protected {
		// A token's hash can be planted as easily as anything else.
		if _, err := d.ExecContext(ctx, `DELETE FROM share_links`); err != nil {
			return err
		}
		// A rule that goes on replacing files by itself is a person's decision, and this file
		// has no password to say whose.
		res, err := d.ExecContext(ctx, `UPDATE ltc_rules SET auto = 0, allow_writes = 0 WHERE auto <> 0 OR allow_writes <> 0`)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err == nil && n > 0 {
			in.Dropped = append(in.Dropped, "Libraries that convert new files by themselves, which you turn on again if you want them, since this backup has no password to prove they're yours.")
		}
	}
	return nil
}

// eachValue goes through one column and changes or deletes rows by what fn says. It uses
// each table's own key, as some have no rowid.
func eachValue(ctx context.Context, d *sql.DB, table, col string, fn func(v string) (string, bool)) (deleted int, err error) {
	if !safeIdent.MatchString(table) || !safeIdent.MatchString(col) {
		return 0, errors.New("bad name")
	}
	// A later version may have dropped a table or a column. Nothing to go through, then.
	var has int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, col).Scan(&has); err != nil {
		return 0, err
	}
	if has == 0 {
		return 0, nil
	}
	rows, err := d.QueryContext(ctx, `SELECT name FROM pragma_table_info(?) WHERE pk > 0 ORDER BY pk`, table)
	if err != nil {
		return 0, err
	}
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return 0, err
		}
		keys = append(keys, quote(k))
	}
	rows.Close()
	if len(keys) == 0 {
		keys = []string{"rowid"}
	}
	var where []string
	for _, k := range keys {
		where = append(where, k+" IS ?")
	}
	match := strings.Join(where, " AND ")
	rows, err = d.QueryContext(ctx, `SELECT `+strings.Join(keys, ",")+`, `+quote(col)+` FROM `+quote(table)+` WHERE `+quote(col)+` IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	type change struct {
		key []any
		v   string
		del bool
	}
	var changes []change
	dest := make([]any, len(keys)+1)
	vals := make([]any, len(keys))
	for i := range vals {
		dest[i] = &vals[i]
	}
	var cur sql.NullString
	dest[len(keys)] = &cur
	for rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			rows.Close()
			return 0, err
		}
		nv, del := fn(cur.String)
		if del || nv != cur.String {
			changes = append(changes, change{key: append([]any{}, vals...), v: nv, del: del})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(changes) == 0 {
		return 0, nil
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, c := range changes {
		if c.del {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+quote(table)+` WHERE `+match, c.key...); err != nil {
				return 0, err
			}
			deleted++
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE OR IGNORE `+quote(table)+` SET `+quote(col)+` = ? WHERE `+match, append([]any{c.v}, c.key...)...); err != nil {
			return 0, err
		}
	}
	return deleted, tx.Commit()
}

func dropOutsideRoots(ctx context.Context, d *sql.DB, table, col string, roots []string) (int, error) {
	return eachValue(ctx, d, table, col, func(v string) (string, bool) {
		switch {
		case v == "" || !looksAbsolute(v) && !strings.Contains(v, ".."):
			// Not a file: a station's address, a podcast's, a game on a game PC.
			return v, strings.HasPrefix(strings.ToLower(v), "file:")
		case strings.Contains(strings.ReplaceAll(v, `\`, "/"), "/../"), !under(v, roots):
			return v, true
		}
		return v, false
	})
}

func sanitizeArt(ctx context.Context, d *sql.DB, table, col string, roots []string) error {
	_, err := eachValue(ctx, d, table, col, func(v string) (string, bool) {
		if v == "" || safeArt(v, roots) {
			return v, false
		}
		return "", false
	})
	return err
}

func safeArt(v string, roots []string) bool {
	if rest, ok := strings.CutPrefix(v, "cache:"); ok {
		return rest != "" && !strings.ContainsAny(rest, `/\:`) && rest != ".." && unsafeName(rest) == ""
	}
	if strings.HasPrefix(v, "/api/") {
		return !strings.Contains(v, "..")
	}
	if looksAbsolute(v) {
		return !strings.Contains(strings.ReplaceAll(v, `\`, "/"), "/../") && under(v, roots)
	}
	if strings.Contains(v, "://") {
		_, ok := metafolder.URL(v)
		return ok
	}
	return false
}

func sanitizeText(ctx context.Context, d *sql.DB, table, col string, max int, lines bool) error {
	_, err := eachValue(ctx, d, table, col, func(v string) (string, bool) {
		if lines {
			return metafolder.Text(v, max), false
		}
		return metafolder.Line(v, max), false
	})
	return err
}

// bcryptShape is what a stored password is. Anything else can't be checked, so nobody can
// sign in with it, and it isn't kept.
var bcryptShape = regexp.MustCompile(`^\$2[aby]\$\d\d\$[./A-Za-z0-9]{53}$`)

func sanitizeUsers(ctx context.Context, d *sql.DB) error {
	rows, err := d.QueryContext(ctx, `SELECT id, name, password_hash, COALESCE(ui, ''), COALESCE(restrictions, '') FROM users`)
	if err != nil {
		return err
	}
	type u struct {
		id                 int64
		name, hash, ui, re string
	}
	var all []u
	for rows.Next() {
		var x u
		if err := rows.Scan(&x.id, &x.name, &x.hash, &x.ui, &x.re); err != nil {
			rows.Close()
			return err
		}
		all = append(all, x)
	}
	rows.Close()
	for _, x := range all {
		switch {
		case x.name == "":
			if _, err := d.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, x.id); err != nil {
				return err
			}
			continue
		case !bcryptShape.MatchString(x.hash) && x.hash != "":
			if _, err := d.ExecContext(ctx, `UPDATE users SET password_hash = '' WHERE id = ?`, x.id); err != nil {
				return err
			}
		}
		if x.ui != "" && !json.Valid([]byte(x.ui)) {
			if _, err := d.ExecContext(ctx, `UPDATE users SET ui = NULL WHERE id = ?`, x.id); err != nil {
				return err
			}
		}
		if x.re != "" && !json.Valid([]byte(x.re)) {
			if _, err := d.ExecContext(ctx, `UPDATE users SET restrictions = NULL WHERE id = ?`, x.id); err != nil {
				return err
			}
		}
	}
	return nil
}

// sanitizeRecordings points recordings at the new data folder, and drops the rest of the
// file paths: a recording is served as it stands.
func sanitizeRecordings(ctx context.Context, d *sql.DB, in *Inspection, f Folders) error {
	rows, err := d.QueryContext(ctx, `SELECT id, COALESCE(path, '') FROM recordings WHERE COALESCE(path, '') <> ''`)
	if err != nil {
		return err
	}
	type rec struct {
		id int64
		p  string
	}
	var all []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.p); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	rows.Close()
	oldRoot := normalizePath(in.man.DataDir + "/recordings")
	newRoot := filepath.Join(f.Data, "recordings")
	dropped := 0
	for _, r := range all {
		n := normalizePath(r.p)
		switch {
		case in.man.DataDir != "" && strings.HasPrefix(n, oldRoot+"/") && !strings.Contains(n, ".."):
			rel := strings.TrimPrefix(n, oldRoot+"/")
			if _, err := d.ExecContext(ctx, `UPDATE recordings SET path = ? WHERE id = ?`, filepath.Join(newRoot, filepath.FromSlash(rel)), r.id); err != nil {
				return err
			}
		case in.man.Protected:
			// Their own folder, perhaps, and it's set again here. The server only touches a
			// recording's file inside the recordings folder, so this names nothing it will use.
		default:
			dropped++
			if _, err := d.ExecContext(ctx, `DELETE FROM recordings WHERE id = ?`, r.id); err != nil {
				return err
			}
		}
	}
	if dropped > 0 {
		in.Dropped = append(in.Dropped, fmt.Sprintf("%d recordings kept outside the data folder, since this backup has no password to prove their paths. The files themselves aren't touched.", dropped))
	}
	return nil
}

// describeDatabase fills in what a person sees before deciding.
func describeDatabase(ctx context.Context, d *sql.DB, in *Inspection) error {
	rows, err := d.QueryContext(ctx, `SELECT name, is_admin, COALESCE(kind, '') FROM users ORDER BY is_admin DESC, name`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a AccountInfo
		var admin int
		if err := rows.Scan(&a.Name, &admin, &a.Kind); err != nil {
			rows.Close()
			return err
		}
		a.Admin = admin != 0
		in.Accounts = append(in.Accounts, a)
	}
	rows.Close()

	rows, err = d.QueryContext(ctx, `SELECT l.id, l.name, l.type, (SELECT COUNT(*) FROM items i WHERE i.library_id = l.id) FROM libraries l ORDER BY l.name`)
	if err != nil {
		return err
	}
	type lib struct {
		id   int64
		info LibraryInfo
	}
	var libs []lib
	for rows.Next() {
		var x lib
		if err := rows.Scan(&x.id, &x.info.Name, &x.info.Type, &x.info.Items); err != nil {
			rows.Close()
			return err
		}
		x.info.Folders = []string{}
		libs = append(libs, x)
	}
	rows.Close()
	for _, x := range libs {
		rows, err := d.QueryContext(ctx, `SELECT path FROM library_paths WHERE library_id = ?`, x.id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var p string
			if rows.Scan(&p) == nil {
				x.info.Folders = append(x.info.Folders, p)
			}
		}
		rows.Close()
		in.Libraries = append(in.Libraries, x.info)
	}

	// Places on the network the server would contact, so a person can see them.
	for _, q := range []string{`SELECT COALESCE(options, '') FROM tuners`, `SELECT COALESCE(url, '') FROM epg_sources`} {
		rows, err := d.QueryContext(ctx, q)
		if err != nil {
			return err
		}
		for rows.Next() {
			var v string
			if rows.Scan(&v) == nil {
				for _, m := range addressIn.FindAllString(v, 3) {
					in.Addresses = append(in.Addresses, metafolder.Line(m, 200))
				}
			}
		}
		rows.Close()
	}
	sort.Strings(in.Addresses)

	counts := map[string]int64{}
	for table, part := range tableParts {
		var n int64
		if table == "settings" {
			if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings WHERE key NOT LIKE 'schema.%'`).Scan(&n); err != nil {
				continue
			}
		} else if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM "`+table+`"`).Scan(&n); err != nil {
			continue
		}
		counts[part] += n
	}
	for i := range in.Parts {
		in.Parts[i].Rows = counts[in.Parts[i].ID]
	}
	in.Notes = append(in.Notes,
		"Every sign-in ends, on every device. Everyone signs in again with the name and password in the backup.",
		"Modules, game cores and BIOS files aren't in a backup. Modules install again from the Modules page, and game PCs need pairing again.")
	if !in.man.Protected {
		in.Notes = append(in.Notes, "This backup has no password, so it holds no API keys or two-factor secrets, and share links stop working. Add the keys again in Settings, and set two-factor up again.")
	} else {
		in.Notes = append(in.Notes, "API keys, two-factor sign-in and share links come back as they were.")
	}
	return nil
}

var addressIn = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>,]+`)
