package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/metafolder"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

// Restoring is checked and unpacked while the server runs, into a folder of its own. The
// swap is made as the server next starts, before anything has these files open: what's
// there is moved aside into a safety copy first, and moved back if anything goes wrong.

const (
	requestName = "restore-request.json"
	resultName  = "restore-result.json"
	stagedName  = "restore-staged"
	safetyPref  = "before-restore-"
	journalName = "journal.json"
	keepSafety  = 3
)

// retrySwap is how long a file in use gets, for the process before this one to let go.
var retrySwap = 30 * time.Second

// Request is what the next start is to do.
type Request struct {
	Mode  string    `json:"mode"` // restore or undo
	Parts []string  `json:"parts,omitempty"`
	HasDB bool      `json:"hasDB"`
	Copy  string    `json:"copy,omitempty"` // undo: the safety copy to bring back
	Name  string    `json:"name,omitempty"` // the archive it came from
	At    time.Time `json:"at"`
}

// Result says how it went.
type Result struct {
	OK     bool      `json:"ok"`
	Mode   string    `json:"mode"`
	Name   string    `json:"name,omitempty"`
	Parts  []string  `json:"parts,omitempty"`
	Safety string    `json:"safety,omitempty"` // the safety copy, by name
	Error  string    `json:"error,omitempty"`
	Notes  []string  `json:"notes,omitempty"`
	At     time.Time `json:"at"`
}

// SafetyCopy is one kept from an earlier restore.
type SafetyCopy struct {
	Name string    `json:"name"`
	At   time.Time `json:"at"`
}

var safetyName = regexp.MustCompile(`^before-restore-\d{8}-\d{6}(-\d+)?$`)

// Pending says what's waiting for the next start, or nil.
func Pending(f Folders) *Request {
	b, err := os.ReadFile(filepath.Join(f.Data, requestName))
	if err != nil {
		return nil
	}
	var r Request
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}

// Last says how the last restore went, or nil.
func Last(f Folders) *Result {
	b, err := os.ReadFile(filepath.Join(f.Data, resultName))
	if err != nil {
		return nil
	}
	var r Result
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}

// Cancel takes back a restore that hasn't happened yet.
func Cancel(f Folders) (bool, error) {
	_ = os.RemoveAll(filepath.Join(f.Data, stagedName))
	err := os.Remove(filepath.Join(f.Data, requestName))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// SafetyCopies lists what earlier restores moved aside, newest first.
func SafetyCopies(f Folders) []SafetyCopy {
	out := []SafetyCopy{}
	entries, err := os.ReadDir(f.Dir())
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() && safetyName.MatchString(e.Name()) {
			c := SafetyCopy{Name: e.Name()}
			if fi, err := e.Info(); err == nil {
				c.At = fi.ModTime()
			}
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out
}

// StageOptions say what to bring back.
type StageOptions struct {
	Parts []string
	// LiveDB is the running server's database, to keep what isn't being restored.
	LiveDB   *sql.DB
	Progress func(Step)
}

// Stage unpacks a checked archive beside the data, and writes the request. Nothing a
// running server uses is touched: until the next start, this can be taken back.
func (in *Inspection) Stage(ctx context.Context, f Folders, o StageOptions) (*Request, error) {
	parts, err := CleanParts(o.Parts)
	if err != nil {
		return nil, err
	}
	chosen := map[string]bool{}
	for _, p := range parts {
		chosen[p] = true
	}
	present := map[string]bool{}
	for _, p := range in.Parts {
		present[p.ID] = true
	}
	for _, p := range parts {
		if !present[p] {
			part, _ := partByID(p)
			return nil, fmt.Errorf("this backup doesn't hold %s", part.Label)
		}
		part, _ := partByID(p)
		for _, n := range part.Needs {
			if present[n] && !chosen[n] {
				need, _ := partByID(n)
				return nil, fmt.Errorf("%s needs %s too: their numbers only match each other", part.Label, need.Label)
			}
			if !present[n] {
				need, _ := partByID(n)
				return nil, fmt.Errorf("%s needs %s, which this backup doesn't hold", part.Label, need.Label)
			}
		}
	}
	if free := sysinfo.FreeBytes(f.Data); free > 0 && free < in.Needed+in.Needed/10 {
		return nil, fmt.Errorf("there isn't room to restore this: it needs about %d MB, and the data folder's drive has %d MB free", in.Needed>>20, free>>20)
	}
	staged := filepath.Join(f.Data, stagedName)
	if err := os.RemoveAll(staged); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(staged, 0o700); err != nil {
		return nil, err
	}
	fail := func(err error) (*Request, error) {
		_ = os.RemoveAll(staged)
		return nil, err
	}
	zr, err := zip.OpenReader(in.zipPath)
	if err != nil {
		return fail(err)
	}
	defer zr.Close()
	known := map[string]FileEntry{}
	for _, e := range in.man.Files {
		known[e.Path] = e
	}
	var total, done int64
	for _, e := range zr.File {
		if src, _ := sourceFor(e.Name); src != nil && chosen[src.Part] {
			total += int64(e.UncompressedSize64)
		}
	}
	for _, e := range zr.File {
		src, rel := sourceFor(e.Name)
		if src == nil || !chosen[src.Part] || strings.HasSuffix(e.Name, "/") {
			continue
		}
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		// Checked again: the archive is read a second time, and this is what gets used.
		if why := unsafeName(e.Name); why != "" {
			return fail(Refused(printableName(e.Name), why))
		}
		dst, err := safeTarget(staged, filepath.FromSlash(strings.TrimSuffix(src.Prefix, "/")), rel)
		if err != nil {
			return fail(Refused(printableName(e.Name), err.Error()))
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return fail(err)
		}
		entry, listed := known[e.Name]
		sum, err := writeEntry(ctx, e, dst)
		if err != nil {
			return fail(Refused(printableName(e.Name), err.Error()))
		}
		if listed && src.Part != PartMetaFiles && sum != entry.SHA256 {
			return fail(Refused(printableName(e.Name), "it's been changed since it was checked"))
		}
		done += int64(e.UncompressedSize64)
		if o.Progress != nil {
			o.Progress(Step{Phase: "Unpacking", Done: done, Total: total, Item: e.Name})
		}
	}
	if chosen[PartModules] {
		if err := cleanModuleSettings(filepath.Join(staged, "modules")); err != nil {
			return fail(err)
		}
	}
	hasDB := false
	for _, p := range parts {
		if part, _ := partByID(p); part.HasTables {
			hasDB = true
		}
	}
	if hasDB {
		if o.Progress != nil {
			o.Progress(Step{Phase: "Preparing the database"})
		}
		if _, err := os.Stat(filepath.Join(in.workDir, "clean", "wms.db")); err != nil {
			return fail(errors.New("this backup holds no database to restore those from"))
		}
		if err := buildFinalDB(ctx, in, f, staged, chosen, o.LiveDB); err != nil {
			return fail(err)
		}
	}
	req := &Request{Mode: "restore", Parts: parts, HasDB: hasDB, Name: filepath.Base(in.Name), At: time.Now().UTC()}
	if err := writeRequest(f, req); err != nil {
		return fail(err)
	}
	return req, nil
}

func writeRequest(f Folders, req *Request) error {
	b, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(f.Data, requestName+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(f.Data, requestName))
}

// AskUndo asks the next start to put a safety copy back.
func AskUndo(f Folders, name string) error {
	if !safetyName.MatchString(name) {
		return errors.New("no such safety copy")
	}
	if st, err := os.Stat(filepath.Join(f.Dir(), name, journalName)); err != nil || st.IsDir() {
		return errors.New("no such safety copy")
	}
	return writeRequest(f, &Request{Mode: "undo", Copy: name, At: time.Now().UTC()})
}

// safeTarget joins a checked relative path under a folder, and refuses anything that would
// still end up outside it on this system.
func safeTarget(root, sub, rel string) (string, error) {
	local := filepath.FromSlash(rel)
	if !filepath.IsLocal(local) {
		return "", errors.New("its path tries to leave its folder")
	}
	base := filepath.Join(root, sub)
	p := filepath.Join(base, local)
	r, err := filepath.Rel(base, p)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", errors.New("its path tries to leave its folder")
	}
	return p, nil
}

func writeEntry(ctx context.Context, e *zip.File, dst string) (string, error) {
	rc, err := e.Open()
	if err != nil {
		return "", errors.New("it can't be read")
	}
	defer rc.Close()
	// O_EXCL: nothing can already be there, least of all a link.
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o640)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	limit := int64(e.UncompressedSize64)
	n, err := io.Copy(io.MultiWriter(out, h), ctxReader(ctx, io.LimitReader(rc, limit+1)))
	cerr := out.Close()
	if err != nil {
		return "", errors.New("it's damaged")
	}
	if cerr != nil {
		return "", cerr
	}
	if n != limit {
		return "", errors.New("it isn't the size the archive says")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// cleanModuleSettings keeps only what a module's settings are: whether it's off, and who
// chooses its updates. Which version was in use, and where it came from, can't be taken
// from a file: the program isn't here, and a source would be a place to install from.
func cleanModuleSettings(dir string) error {
	p := filepath.Join(dir, "state.json")
	if b, err := os.ReadFile(p); err == nil {
		var saved map[string]map[string]any
		if err := json.Unmarshal(b, &saved); err != nil {
			return Refused("modules/state.json", "it isn't valid")
		}
		clean := map[string]map[string]any{}
		for id, st := range saved {
			if !moduleIDShape.MatchString(id) {
				continue
			}
			keep := map[string]any{}
			if v, ok := st["removed"].(bool); ok && v {
				keep["removed"] = true
			}
			if v, ok := st["noAuto"].(bool); ok && v {
				keep["noAuto"] = true
			}
			if len(keep) > 0 {
				clean[id] = keep
			}
		}
		out, err := json.MarshalIndent(clean, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(p, out, 0o640); err != nil {
			return err
		}
	}
	p = filepath.Join(dir, "library-names.json")
	if b, err := os.ReadFile(p); err == nil {
		var names map[string]string
		if err := json.Unmarshal(b, &names); err != nil {
			return Refused("modules/library-names.json", "it isn't valid")
		}
		clean := map[string]string{}
		for k, v := range names {
			if moduleIDShape.MatchString(k) {
				clean[k] = metafolder.Line(v, 200)
			}
		}
		out, err := json.MarshalIndent(clean, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(p, out, 0o640); err != nil {
			return err
		}
	}
	return nil
}

var moduleIDShape = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// buildFinalDB makes the database the next start will use: what was chosen from the
// archive, and everything else as it is on this server now. A part whose numbers depend
// on a part being replaced can't keep what it has: a profile or a film's number would
// point at something else. Those are emptied, and the page says so before anyone agrees.
func buildFinalDB(ctx context.Context, in *Inspection, f Folders, staged string, chosen map[string]bool, live *sql.DB) error {
	dir := filepath.Join(staged, "database")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	final := filepath.Join(dir, "wms.db")
	if err := copyFile(filepath.Join(in.workDir, "clean", "wms.db"), final); err != nil {
		return err
	}
	d, err := db.OpenPlainFile(final)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := dropTriggers(ctx, d); err != nil {
		return err
	}

	// What this server has now, as a consistent copy.
	current := filepath.Join(staged, "current.db")
	haveCurrent := false
	if live != nil {
		if err := db.SnapshotTo(ctx, live, current); err != nil {
			return err
		}
		haveCurrent = true
	} else if _, err := os.Stat(filepath.Join(f.Data, "wms.db")); err == nil {
		if ro, err := db.OpenExisting(f.Data, false); err == nil {
			err = db.SnapshotTo(ctx, ro.DB, current)
			ro.Close()
			if err == nil {
				haveCurrent = true
			}
		}
	}
	defer os.Remove(current)

	tables, err := objectNames(ctx, d, "table")
	if err != nil {
		return err
	}
	if haveCurrent {
		if _, err := d.ExecContext(ctx, `ATTACH DATABASE ? AS cur`, current); err != nil {
			return fmt.Errorf("reading this server's database: %w", err)
		}
		defer func() { _, _ = d.ExecContext(context.Background(), `DETACH DATABASE cur`) }()
	}
	replaced := func(part string) bool { return chosen[part] }
	keepCurrent := func(part string) bool {
		if chosen[part] {
			return false
		}
		def, ok := partByID(part)
		if !ok {
			return false
		}
		for _, n := range def.Needs {
			if replaced(n) {
				return false
			}
		}
		return true
	}
	for _, t := range tables {
		if t == "schema" || t == "sqlite_sequence" || !safeIdent.MatchString(t) {
			continue
		}
		part, known := tableParts[t]
		switch {
		case t == "sessions":
			// Whoever is signed in keeps that only if accounts aren't being replaced.
			if _, err := d.ExecContext(ctx, `DELETE FROM sessions`); err != nil {
				return err
			}
			if haveCurrent && !chosen[PartUsers] {
				if err := copyFromCurrent(ctx, d, t, ""); err != nil {
					return err
				}
			}
		case !known:
			// Tables no backup holds (cores, game PCs, measurements) stay as they are here.
			if _, err := d.ExecContext(ctx, `DELETE FROM `+quote(t)); err != nil {
				return err
			}
			if haveCurrent && !chosen[PartLibrary] && !chosen[PartUsers] {
				if err := copyFromCurrent(ctx, d, t, ""); err != nil {
					return err
				}
			} else if haveCurrent && (t == "metrics_hourly" || t == "stream_bytes" || t == "task_history") {
				if err := copyFromCurrent(ctx, d, t, ""); err != nil {
					return err
				}
			}
		case chosen[part]:
			// From the archive, as it is.
		case t == "settings":
			if _, err := d.ExecContext(ctx, `DELETE FROM settings WHERE key NOT LIKE 'schema.%'`); err != nil {
				return err
			}
			if haveCurrent {
				if err := copyFromCurrent(ctx, d, t, ` WHERE key NOT LIKE 'schema.%'`); err != nil {
					return err
				}
			}
		default:
			if _, err := d.ExecContext(ctx, `DELETE FROM `+quote(t)); err != nil {
				return err
			}
			if haveCurrent && keepCurrent(part) {
				if err := copyFromCurrent(ctx, d, t, ""); err != nil {
					return err
				}
			}
		}
	}
	if haveCurrent {
		if _, err := d.ExecContext(ctx, `DETACH DATABASE cur`); err != nil {
			return err
		}
	}
	if _, err := db.RemoveOrphans(ctx, d); err != nil {
		return err
	}
	d.Close()
	// The triggers come back the way everything else does: from this program.
	return db.RestoreTriggers(ctx, final)
}

func copyFromCurrent(ctx context.Context, d *sql.DB, table, where string) error {
	_, cols, err := columnsOf(ctx, d, table)
	if err != nil {
		return err
	}
	var names []string
	for _, c := range cols {
		if safeIdent.MatchString(c) {
			names = append(names, quote(c))
		}
	}
	list := strings.Join(names, ",")
	_, err = d.ExecContext(ctx, `INSERT OR REPLACE INTO main.`+quote(table)+`(`+list+`) SELECT `+list+` FROM cur.`+quote(table)+where)
	return err
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ---------------------------------------------------------------- the swap

// swap is one thing to move: what's at Target goes to Aside, and Source to Target.
type swap struct {
	Target string `json:"target"`
	Source string `json:"source,omitempty"`
	Aside  string `json:"aside"`
	Moved  bool   `json:"moved,omitempty"` // what was at Target has gone to Aside
	In     bool   `json:"in,omitempty"`    // Source has come to Target
}

type journal struct {
	State string `json:"state"` // applying or done
	Swaps []swap `json:"swaps"`
	Mode  string `json:"mode"`
	Parts []string
	Name  string `json:"name,omitempty"`
}

func saveJournal(dir string, j *journal) error {
	b, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, journalName+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, journalName))
}

// planSwaps lists what a restore moves, all within what its parts cover.
func planSwaps(f Folders, safety string, req *Request) []swap {
	staged := filepath.Join(f.Data, stagedName)
	chosen := map[string]bool{}
	for _, p := range req.Parts {
		chosen[p] = true
	}
	var out []swap
	add := func(target, source, aside string) {
		out = append(out, swap{Target: target, Source: source, Aside: filepath.Join(safety, aside)})
	}
	if req.HasDB {
		add(filepath.Join(f.Data, "wms.db"), filepath.Join(staged, "database", "wms.db"), filepath.Join("database", "wms.db"))
		add(filepath.Join(f.Data, "wms.db-wal"), "", filepath.Join("database", "wms.db-wal"))
		add(filepath.Join(f.Data, "wms.db-shm"), "", filepath.Join("database", "wms.db-shm"))
	}
	for _, s := range sources() {
		if !chosen[s.Part] {
			continue
		}
		name := strings.TrimSuffix(s.Prefix, "/")
		root := s.Root(f)
		st := filepath.Join(staged, name)
		switch name {
		case "themes", "modules":
			// These folders hold more than a backup does: only the files it has are swapped.
			_ = walkRegular(st, func(rel string, _ fs.FileInfo) error {
				add(filepath.Join(root, filepath.FromSlash(rel)), filepath.Join(st, filepath.FromSlash(rel)), filepath.Join(name, filepath.FromSlash(rel)))
				return nil
			})
		default:
			add(root, st, name)
		}
	}
	return out
}

// moveWithin renames, waiting out a file in use. Across drives (a metadata folder set
// elsewhere), it copies and then removes.
func moveWithin(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
		return err
	}
	deadline := time.Now().Add(retrySwap)
	for {
		err := os.Rename(from, to)
		if err == nil {
			return nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if isCrossDevice(err) {
			return copyAcross(from, to)
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func isCrossDevice(err error) bool {
	var le *os.LinkError
	if !errors.As(err, &le) {
		return false
	}
	s := le.Err.Error()
	return strings.Contains(s, "cross-device") || strings.Contains(s, "different disk drive")
}

func copyAcross(from, to string) error {
	st, err := os.Lstat(from)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		if err := copyFile(from, to); err != nil {
			return err
		}
		return os.Remove(from)
	}
	if err := walkRegular(from, func(rel string, _ fs.FileInfo) error {
		dst := filepath.Join(to, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return err
		}
		return copyFile(filepath.Join(from, filepath.FromSlash(rel)), dst)
	}); err != nil {
		_ = os.RemoveAll(to)
		return err
	}
	return os.RemoveAll(from)
}

func runSwaps(dir string, j *journal) error {
	for i := range j.Swaps {
		s := &j.Swaps[i]
		if _, err := os.Lstat(s.Target); err == nil {
			if err := moveWithin(s.Target, s.Aside); err != nil {
				return fmt.Errorf("%s couldn't be moved aside: %w", filepath.Base(s.Target), err)
			}
			s.Moved = true
			if err := saveJournal(dir, j); err != nil {
				return err
			}
		}
		if s.Source != "" {
			if _, err := os.Lstat(s.Source); err == nil {
				if err := moveWithin(s.Source, s.Target); err != nil {
					return fmt.Errorf("%s couldn't be put in place: %w", filepath.Base(s.Target), err)
				}
				s.In = true
				if err := saveJournal(dir, j); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// undoSwaps reverses whatever a journal says happened. It's also what a start does on
// finding one left half done by a power cut.
func undoSwaps(j *journal) []string {
	var trouble []string
	for i := len(j.Swaps) - 1; i >= 0; i-- {
		s := &j.Swaps[i]
		if s.In {
			if s.Source != "" {
				if err := moveWithin(s.Target, s.Source); err != nil && !errors.Is(err, fs.ErrNotExist) {
					trouble = append(trouble, filepath.Base(s.Target)+": "+err.Error())
				}
			}
			s.In = false
		}
		if s.Moved {
			if err := moveWithin(s.Aside, s.Target); err != nil && !errors.Is(err, fs.ErrNotExist) {
				trouble = append(trouble, filepath.Base(s.Target)+": "+err.Error())
			}
			s.Moved = false
		}
	}
	return trouble
}

// checkRestoredDB opens what was put in place, the way the server is about to.
func checkRestoredDB(data string) error {
	if _, err := os.Stat(filepath.Join(data, "wms.db")); err != nil {
		return errors.New("the restored database isn't there")
	}
	d, err := db.Open(data)
	if err != nil {
		return err
	}
	defer d.Close()
	var ok string
	if err := d.QueryRow(`PRAGMA quick_check(1)`).Scan(&ok); err != nil {
		return err
	}
	if ok != "ok" {
		return errors.New(ok)
	}
	return nil
}

// recoverInterrupted puts things back if a previous restore was cut off part way.
func recoverInterrupted(f Folders) (string, error) {
	for _, c := range SafetyCopies(f) {
		dir := filepath.Join(f.Dir(), c.Name)
		b, err := os.ReadFile(filepath.Join(dir, journalName))
		if err != nil {
			continue
		}
		var j journal
		if json.Unmarshal(b, &j) != nil || j.State != "applying" {
			continue
		}
		if trouble := undoSwaps(&j); len(trouble) > 0 {
			return c.Name, errors.New(strings.Join(trouble, "; "))
		}
		j.State = "rolled-back"
		_ = saveJournal(dir, &j)
		_ = os.RemoveAll(dir)
		return c.Name, nil
	}
	return "", nil
}

// Apply carries out what's waiting. It returns nil when nothing was. On any failure,
// everything is put back as it was, and the server starts as usual.
func Apply(f Folders) *Result {
	cleanLeftovers(f)
	res := &Result{At: time.Now().UTC()}
	if name, err := recoverInterrupted(f); name != "" {
		res.Mode = "recovery"
		if err != nil {
			res.Error = "A restore was cut off, and not everything could be put back: " + err.Error() + ". What was there is in the backups folder, " + name + "."
		} else {
			res.OK = true
			res.Notes = append(res.Notes, "A restore was cut off last time, so everything was put back as it was.")
		}
		writeResult(f, res)
	}
	req := Pending(f)
	if req == nil {
		_ = os.Remove(filepath.Join(f.Data, requestName))
		if res.Mode != "" {
			return res
		}
		return nil
	}
	res = &Result{At: time.Now().UTC(), Mode: req.Mode, Name: req.Name, Parts: req.Parts}
	finish := func(err error) *Result {
		_ = os.Remove(filepath.Join(f.Data, requestName))
		_ = os.RemoveAll(filepath.Join(f.Data, stagedName))
		res.OK = err == nil
		if err != nil {
			res.Error = err.Error()
		}
		writeResult(f, res)
		return res
	}
	stamp := time.Now().Format(stampForm)
	name := safetyPref + stamp
	for i := 2; ; i++ {
		if _, err := os.Lstat(filepath.Join(f.Dir(), name)); errors.Is(err, fs.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("%s%s-%d", safetyPref, stamp, i)
	}
	safety := filepath.Join(f.Dir(), name)
	if err := os.MkdirAll(safety, 0o700); err != nil {
		return finish(err)
	}
	j := &journal{State: "applying", Mode: req.Mode, Parts: req.Parts, Name: req.Name}
	switch req.Mode {
	case "restore":
		if _, err := os.Stat(filepath.Join(f.Data, stagedName)); err != nil {
			_ = os.Remove(safety)
			return finish(errors.New("what was unpacked isn't there any more, so nothing was changed"))
		}
		j.Swaps = planSwaps(f, safety, req)
	case "undo":
		if !safetyName.MatchString(req.Copy) {
			_ = os.Remove(safety)
			return finish(errors.New("no such safety copy"))
		}
		old := filepath.Join(f.Dir(), req.Copy)
		b, err := os.ReadFile(filepath.Join(old, journalName))
		var prev journal
		if err == nil {
			err = json.Unmarshal(b, &prev)
		}
		if err != nil {
			_ = os.Remove(safety)
			return finish(errors.New("that safety copy can't be read, so nothing was changed"))
		}
		res.Parts = prev.Parts
		for _, s := range prev.Swaps {
			if s.Aside == "" {
				continue
			}
			src := s.Aside
			if !under(src, []string{normalizePath(old)}) {
				continue // a journal can only speak for its own folder
			}
			rel, err := filepath.Rel(old, src)
			if err != nil {
				continue
			}
			j.Swaps = append(j.Swaps, swap{Target: s.Target, Source: src, Aside: filepath.Join(safety, rel)})
		}
		if !targetsAllowed(f, j.Swaps) {
			_ = os.Remove(safety)
			return finish(errors.New("that safety copy names somewhere it shouldn't, so nothing was changed"))
		}
	default:
		_ = os.Remove(safety)
		return finish(fmt.Errorf("no such kind of restore: %q", req.Mode))
	}
	if err := saveJournal(safety, j); err != nil {
		return finish(err)
	}
	err := runSwaps(safety, j)
	if err == nil && hasDBSwap(f, j) {
		if cerr := checkRestoredDB(f.Data); cerr != nil {
			err = fmt.Errorf("the restored database didn't open: %w", cerr)
		}
	}
	if err != nil {
		trouble := undoSwaps(j)
		if len(trouble) == 0 {
			j.State = "rolled-back"
			_ = saveJournal(safety, j)
			_ = os.RemoveAll(safety)
			return finish(fmt.Errorf("%w. Everything was put back as it was", err))
		}
		_ = saveJournal(safety, j)
		return finish(fmt.Errorf("%w, and not everything could be put back (%s). What was there is in %s", err, strings.Join(trouble, "; "), name))
	}
	j.State = "done"
	if serr := saveJournal(safety, j); serr != nil {
		res.Notes = append(res.Notes, "The safety copy's list couldn't be saved: "+serr.Error())
	}
	res.Safety = name
	if req.Mode == "undo" {
		_ = os.RemoveAll(filepath.Join(f.Dir(), req.Copy))
		res.Notes = append(res.Notes, "What the restore replaced is back, and what it brought is kept as a safety copy.")
	}
	pruneSafety(f)
	// A folder a part held may have gone with it, and the server expects these.
	_ = os.MkdirAll(f.Images, 0o750)
	return finish(nil)
}

func hasDBSwap(f Folders, j *journal) bool {
	want := filepath.Join(f.Data, "wms.db")
	for _, s := range j.Swaps {
		if s.Target == want && s.In {
			return true
		}
	}
	return false
}

// targetsAllowed: a journal is a file on disk, and only ever moves things this package
// knows about.
func targetsAllowed(f Folders, swaps []swap) bool {
	roots := []string{normalizePath(f.Data), normalizePath(f.Images)}
	for _, s := range swaps {
		if !under(s.Target, roots) || strings.Contains(s.Target, "..") {
			return false
		}
	}
	return true
}

// cleanLeftovers removes what a backup or check that was cut off left behind. A day's
// grace, in case the command line is making one right now.
func cleanLeftovers(f Folders) {
	entries, err := os.ReadDir(f.Dir())
	if err != nil {
		return
	}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, ".backup-") && !strings.HasPrefix(n, ".restore-check-") {
			continue
		}
		if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) > 24*time.Hour {
			_ = os.RemoveAll(filepath.Join(f.Dir(), n))
		}
	}
}

func pruneSafety(f Folders) {
	list := SafetyCopies(f)
	for i, c := range list {
		if i >= keepSafety {
			_ = os.RemoveAll(filepath.Join(f.Dir(), c.Name))
		}
	}
}

func writeResult(f Folders, r *Result) {
	if b, err := json.MarshalIndent(r, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(f.Data, resultName), b, 0o600)
	}
}
