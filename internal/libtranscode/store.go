package libtranscode

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// Settings is how conversions run on this server. Off until the owner has read what it
// does to streaming and turned it on.
type Settings struct {
	Enabled bool `json:"enabled"`
	// Acknowledged: the warning has been read.
	Acknowledged bool `json:"acknowledged"`
	// MaxJobs: files converted at the same time.
	MaxJobs int `json:"maxJobs"`
	// Threads for each conversion. 0 means half of this machine's cores.
	Threads int `json:"threads"`
	// LowPriority: other work gets the processor first.
	LowPriority bool `json:"lowPriority"`
	// PauseWhileStreaming holds conversions while anyone is playing something.
	PauseWhileStreaming bool `json:"pauseWhileStreaming"`
	// HoursOn limits work to between HoursFrom and HoursTo, on this server's clock.
	HoursOn   bool `json:"hoursOn"`
	HoursFrom int  `json:"hoursFrom"`
	HoursTo   int  `json:"hoursTo"`
	// UseHardware allows the hardware encoders ticked on the Transcoding page.
	UseHardware bool `json:"useHardware"`
	// MinFreeGB: stop before a drive has less room than this.
	MinFreeGB int `json:"minFreeGb"`
	// SettleSeconds: automatic runs leave a new file alone until it hasn't changed for this long.
	SettleSeconds int `json:"settleSeconds"`
}

const settingsKey = "libtranscode.settings"

// DefaultSettings suit a modest machine that's mainly there for watching things.
func DefaultSettings() Settings {
	return Settings{MaxJobs: 1, LowPriority: true, PauseWhileStreaming: true, UseHardware: true, HoursFrom: 1, HoursTo: 7, MinFreeGB: 10, SettleSeconds: 120}
}

// LoadSettings reads the settings.
func LoadSettings(ctx context.Context, d *db.DB) Settings {
	s := DefaultSettings()
	if raw := d.GetSetting(ctx, settingsKey, ""); raw != "" {
		_ = json.Unmarshal([]byte(raw), &s)
	}
	return s
}

// Validate checks settings from outside.
func (s *Settings) Validate() error {
	switch {
	case s.MaxJobs < 1 || s.MaxJobs > 8:
		return errors.New("files at once must be between 1 and 8")
	case s.Threads < 0 || s.Threads > 256:
		return errors.New("processor threads must be between 0 and 256")
	case s.HoursFrom < 0 || s.HoursFrom > 23 || s.HoursTo < 0 || s.HoursTo > 23:
		return errors.New("hours must be between 0 and 23")
	case s.HoursOn && s.HoursFrom == s.HoursTo:
		return errors.New("the hours it may run can't start and end at the same time")
	case s.MinFreeGB < 1 || s.MinFreeGB > 100000:
		return errors.New("the free space to keep must be at least 1 GB")
	case s.SettleSeconds < 10 || s.SettleSeconds > 86400:
		return errors.New("the wait for new files must be between 10 seconds and a day")
	case s.Enabled && !s.Acknowledged:
		return errors.New("read and accept the warning about streaming before turning this on")
	}
	return nil
}

// SaveSettings stores the settings.
func SaveSettings(ctx context.Context, d *db.DB, s Settings) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return d.SetSetting(ctx, settingsKey, string(b))
}

// inHours says whether an hour of the day is inside the window, which can cross midnight.
func (s Settings) inHours(hour int) bool {
	if !s.HoursOn {
		return true
	}
	if s.HoursFrom < s.HoursTo {
		return hour >= s.HoursFrom && hour < s.HoursTo
	}
	return hour >= s.HoursFrom || hour < s.HoursTo
}

// Rule is what a library's files should become.
type Rule struct {
	LibraryID int64  `json:"libraryId"`
	Policy    Policy `json:"policy"`
	Output    string `json:"output"`
	// AllowWrites: conversions may write here even though the server has been told to
	// keep metadata out of media folders.
	AllowWrites bool `json:"allowWrites"`
	// Auto: new files are converted by themselves after a scan finds them, from AutoSince on.
	Auto      bool  `json:"auto"`
	AutoSince int64 `json:"autoSince"`
	Saved     bool  `json:"saved"`
}

// LoadRule reads a library's rule, or the safe starting point when it has none.
func LoadRule(ctx context.Context, d *db.DB, library int64) Rule {
	r := Rule{LibraryID: library, Output: OutputKeep}
	r.Policy, _ = PresetPolicy(PresetCompatible)
	var raw string
	var allow, auto int
	err := d.QueryRowContext(ctx, `SELECT policy, output, allow_writes, auto, auto_since FROM ltc_rules WHERE library_id = ?`, library).Scan(&raw, &r.Output, &allow, &auto, &r.AutoSince)
	if err != nil {
		return r
	}
	var p Policy
	if json.Unmarshal([]byte(raw), &p) == nil && p.Validate() == nil {
		r.Policy, r.Saved = p, true
	}
	r.AllowWrites = allow != 0
	r.Auto = auto != 0
	return r
}

// SaveRule stores a checked rule. Turning automatic runs on starts counting new files from now:
// what's already in the library is left to a person.
func SaveRule(ctx context.Context, d *db.DB, r Rule) error {
	b, err := json.Marshal(r.Policy)
	if err != nil {
		return err
	}
	allow, auto := 0, 0
	var since int64
	if r.AllowWrites {
		allow = 1
	}
	if r.Auto {
		auto, since = 1, db.Now()
	}
	_, err = d.ExecContext(ctx, `INSERT INTO ltc_rules(library_id, policy, output, allow_writes, updated_at, auto, auto_since) VALUES(?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(library_id) DO UPDATE SET policy = excluded.policy, output = excluded.output, allow_writes = excluded.allow_writes, updated_at = excluded.updated_at,
		auto = excluded.auto, auto_since = CASE WHEN excluded.auto <> 0 AND ltc_rules.auto = 0 THEN excluded.auto_since ELSE ltc_rules.auto_since END`,
		r.LibraryID, string(b), r.Output, allow, db.Now(), auto, since)
	return err
}

// States of one file in a run.
const (
	FilePlanned   = "planned" // a dry run looked at it
	FilePending   = "pending" // waiting its turn
	FileRunning   = "running"
	FileDone      = "done"      // converted, checked and in place
	FileSkipped   = "skipped"   // nothing to do
	FileRefused   = "refused"   // left alone on purpose
	FileFailed    = "failed"    // went wrong; the old file is untouched
	FileChanged   = "changed"   // the file changed while it was being converted
	FileNotWorth  = "notworth"  // came out no smaller
	FilePostponed = "postponed" // someone was playing it
	FileUndone    = "undone"    // the original was put back
)

// Run is one go at a library.
type Run struct {
	ID        int64   `json:"id"`
	LibraryID int64   `json:"libraryId"`
	Dry       bool    `json:"dry"`
	Auto      bool    `json:"auto"` // a scan started it, not a person
	Policy    Policy  `json:"policy"`
	Output    string  `json:"output"`
	State     string  `json:"state"` // queued, running, done, failed, cancelled, interrupted
	By        string  `json:"by,omitempty"`
	CreatedAt int64   `json:"createdAt"`
	Finished  *int64  `json:"finishedAt,omitempty"`
	Summary   Summary `json:"summary"`
}

// Summary adds up a run, for the page and the estimate shown before starting.
type Summary struct {
	Files       int     `json:"files"`
	Convert     int     `json:"convert"`
	Remux       int     `json:"remux"`
	Skip        int     `json:"skip"`
	Refuse      int     `json:"refuse"`
	Done        int     `json:"done"`
	Failed      int     `json:"failed"`
	NotWorth    int     `json:"notWorth"`
	Postponed   int     `json:"postponed"`
	Changed     int     `json:"changed"`
	BytesBefore int64   `json:"bytesBefore"`
	EstBytes    int64   `json:"estBytes"`
	EstSec      float64 `json:"estSec"`
	BytesAfter  int64   `json:"bytesAfter"`
	NeedsBytes  int64   `json:"needsBytes"`
	// Drives: room on each drive involved against what the job needs there.
	Drives []DriveRoom `json:"drives,omitempty"`
}

// DriveRoom says whether a drive has room for a job.
type DriveRoom struct {
	Folder string `json:"folder"`
	Free   int64  `json:"free"`
	Needs  int64  `json:"needs"`
	Enough bool   `json:"enough"`
}

// FileRow is one file of a run.
type FileRow struct {
	ID         int64   `json:"id"`
	RunID      int64   `json:"runId"`
	ItemID     int64   `json:"itemId"`
	Part       int     `json:"part"`
	Path       string  `json:"path"`
	Verdict    string  `json:"verdict"`
	Reason     string  `json:"reason"`
	State      string  `json:"state"`
	Message    string  `json:"message,omitempty"`
	SizeBefore int64   `json:"sizeBefore"`
	SizeAfter  int64   `json:"sizeAfter"`
	EstBytes   int64   `json:"estBytes"`
	EstSec     float64 `json:"estSec"`
	FinalPath  string  `json:"finalPath,omitempty"`
	KeptPath   string  `json:"keptPath,omitempty"`
	Plan       *Plan   `json:"plan,omitempty"`
}

func (e *Engine) createRun(ctx context.Context, libraryID int64, p Policy, output string, dry, auto bool, byID int64, by string) (int64, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return 0, err
	}
	d, a := 0, 0
	if dry {
		d = 1
	}
	if auto {
		a = 1
	}
	res, err := e.DB.ExecContext(ctx, `INSERT INTO ltc_runs(library_id, dry, auto, policy, output, state, by_id, by_name, created_at) VALUES(?, ?, ?, ?, ?, 'queued', ?, ?, ?)`,
		libraryID, d, a, string(b), output, byID, by, db.Now())
	if err != nil {
		return 0, err
	}
	// A library doesn't need its whole past. The last 20 runs are plenty.
	_, _ = e.DB.ExecContext(ctx, `DELETE FROM ltc_runs WHERE library_id = ? AND finished_at IS NOT NULL AND id NOT IN
		(SELECT id FROM ltc_runs WHERE library_id = ? ORDER BY id DESC LIMIT 20)
		AND id NOT IN (SELECT run_id FROM ltc_files WHERE kept_path <> '' OR (final_path <> '' AND state = 'done'))`, libraryID, libraryID)
	return res.LastInsertId()
}

func (e *Engine) setRunState(ctx context.Context, id int64, state string, sum *Summary) {
	var finished any
	if state != "queued" && state != "running" {
		finished = db.Now()
	}
	var b []byte
	if sum != nil {
		b, _ = json.Marshal(sum)
	}
	var err error
	if b != nil {
		_, err = e.DB.ExecContext(ctx, `UPDATE ltc_runs SET state = ?, finished_at = ?, summary = ? WHERE id = ?`, state, finished, string(b), id)
	} else {
		_, err = e.DB.ExecContext(ctx, `UPDATE ltc_runs SET state = ?, finished_at = ? WHERE id = ?`, state, finished, id)
	}
	if err != nil {
		e.Log.WarnContext(ctx, "could not save a conversion's state", "run", id, "err", err)
	}
}

func scanRun(sc interface{ Scan(...any) error }) (*Run, error) {
	var r Run
	var dry, auto int
	var policy, summary string
	var finished sql.NullInt64
	if err := sc.Scan(&r.ID, &r.LibraryID, &dry, &auto, &policy, &r.Output, &r.State, &r.By, &r.CreatedAt, &finished, &summary); err != nil {
		return nil, err
	}
	r.Dry = dry != 0
	r.Auto = auto != 0
	if finished.Valid {
		r.Finished = &finished.Int64
	}
	_ = json.Unmarshal([]byte(policy), &r.Policy)
	_ = json.Unmarshal([]byte(summary), &r.Summary)
	return &r, nil
}

const runCols = `id, library_id, dry, auto, policy, output, state, by_name, created_at, finished_at, summary`

// GetRun reads one run.
func (e *Engine) GetRun(ctx context.Context, id int64) (*Run, error) {
	r, err := scanRun(e.DB.QueryRowContext(ctx, `SELECT `+runCols+` FROM ltc_runs WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoRun
	}
	return r, err
}

// ErrNoRun: there's no such run.
var ErrNoRun = errors.New("that conversion no longer exists")

// ListRuns lists recent runs, newest first. library 0 means every library.
func (e *Engine) ListRuns(ctx context.Context, library int64, limit int) ([]*Run, error) {
	rows, err := e.DB.QueryContext(ctx, `SELECT `+runCols+` FROM ltc_runs WHERE ? = 0 OR library_id = ? ORDER BY id DESC LIMIT ?`, library, library, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const fileCols = `id, run_id, item_id, part, path, verdict, reason, state, message, size_before, size_after, est_bytes, est_sec, final_path, kept_path, plan`

func scanFile(sc interface{ Scan(...any) error }, withPlan bool) (*FileRow, error) {
	var f FileRow
	var plan string
	if err := sc.Scan(&f.ID, &f.RunID, &f.ItemID, &f.Part, &f.Path, &f.Verdict, &f.Reason, &f.State, &f.Message, &f.SizeBefore, &f.SizeAfter, &f.EstBytes, &f.EstSec, &f.FinalPath, &f.KeptPath, &plan); err != nil {
		return nil, err
	}
	if withPlan && plan != "" {
		var p Plan
		if json.Unmarshal([]byte(plan), &p) == nil {
			f.Plan = &p
		}
	}
	return &f, nil
}

// ListFiles lists a run's files. state "" means all of them.
func (e *Engine) ListFiles(ctx context.Context, run int64, state string, limit, offset int) ([]*FileRow, int, error) {
	var total int
	if err := e.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM ltc_files WHERE run_id = ? AND (? = '' OR state = ?)`, run, state, state).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := e.DB.QueryContext(ctx, `SELECT `+fileCols+` FROM ltc_files WHERE run_id = ? AND (? = '' OR state = ?) ORDER BY
		CASE state WHEN 'running' THEN 0 WHEN 'failed' THEN 1 ELSE 2 END, id LIMIT ? OFFSET ?`, run, state, state, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*FileRow{}
	for rows.Next() {
		f, err := scanFile(rows, true)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, f)
	}
	return out, total, rows.Err()
}

// GetFile reads one file row.
func (e *Engine) GetFile(ctx context.Context, id int64) (*FileRow, error) {
	f, err := scanFile(e.DB.QueryRowContext(ctx, `SELECT `+fileCols+` FROM ltc_files WHERE id = ?`, id), false)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoRun
	}
	return f, err
}

// Leftovers lists kept originals and converted copies waiting for a decision.
func (e *Engine) Leftovers(ctx context.Context, library int64, limit int) ([]*FileRow, int64, error) {
	rows, err := e.DB.QueryContext(ctx, `SELECT `+prefixed(fileCols)+` FROM ltc_files f JOIN ltc_runs r ON r.id = f.run_id
		WHERE f.state = 'done' AND (f.kept_path <> '' OR (r.output = 'beside' AND f.final_path <> '')) AND (? = 0 OR r.library_id = ?) ORDER BY f.id DESC LIMIT ?`, library, library, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*FileRow{}
	var total int64
	for rows.Next() {
		f, err := scanFile(rows, false)
		if err != nil {
			return nil, 0, err
		}
		total += f.SizeBefore
		out = append(out, f)
	}
	return out, total, rows.Err()
}

func prefixed(cols string) string {
	out := "f."
	for _, c := range cols {
		out += string(c)
		if c == ' ' {
			out += "f."
		}
	}
	return out
}

func (e *Engine) updateFile(ctx context.Context, id int64, fields map[string]any) {
	q := "UPDATE ltc_files SET "
	args := make([]any, 0, len(fields)+1)
	first := true
	for k, v := range fields {
		if !first {
			q += ", "
		}
		first = false
		q += k + " = ?"
		args = append(args, v)
	}
	q += " WHERE id = ?"
	args = append(args, id)
	if _, err := e.DB.ExecContext(context.WithoutCancel(ctx), q, args...); err != nil {
		e.Log.WarnContext(ctx, "could not save a converted file's state", "file", id, "err", err)
	}
}

// policyHash identifies a recipe for remembering what it did to a file.
func policyHash(p Policy) string {
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func (e *Engine) remember(ctx context.Context, path string, p Policy, size, mtime int64, outcome string) {
	_, err := e.DB.ExecContext(context.WithoutCancel(ctx), `INSERT INTO ltc_memo(path, policy, size, mtime, outcome, at) VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(path, policy) DO UPDATE SET size = excluded.size, mtime = excluded.mtime, outcome = excluded.outcome, at = excluded.at`,
		path, policyHash(p), size, mtime, outcome, db.Now())
	if err != nil {
		e.Log.WarnContext(ctx, "could not remember a conversion's outcome", "err", err)
	}
}

func (e *Engine) recall(ctx context.Context, path string, p Policy, size, mtime int64) string {
	var outcome string
	var s, m int64
	err := e.DB.QueryRowContext(ctx, `SELECT size, mtime, outcome FROM ltc_memo WHERE path = ? AND policy = ?`, path, policyHash(p)).Scan(&s, &m, &outcome)
	if err != nil || s != size || m != mtime {
		return ""
	}
	return outcome
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
