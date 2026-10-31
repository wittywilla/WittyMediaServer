package livetv

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittytuner"
)

// Recording: tuner channels only. Raw MPEG-TS goes straight to disk, which
// costs almost nothing; other sources go through FFmpeg in copy mode. A
// recording takes a tuner when it needs one, stopping a live viewer if it must.

// Recording is one recording, past or to come.
type Recording struct {
	ID        int64  `json:"id"`
	ChannelID int64  `json:"channelId"`
	Channel   string `json:"channel"`
	Number    string `json:"number"`
	Title     string `json:"title"`
	Sub       string `json:"sub,omitempty"`
	Start     int64  `json:"start"`
	Stop      int64  `json:"stop"`
	Status    string `json:"status"` // scheduled | recording | done | failed | cancelled
	Error     string `json:"error,omitempty"`
	Path      string `json:"path,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	RuleID    *int64 `json:"ruleId,omitempty"`
}

// Rule records every airing of a title on a channel.
type Rule struct {
	ID        int64  `json:"id"`
	ChannelID int64  `json:"channelId"`
	Channel   string `json:"channel"`
	Title     string `json:"title"`
}

// Padding starts recordings early and ends them late, as broadcasts drift.
const (
	padBefore = 60
	padAfter  = 120
)

const recCols = `r.id, r.channel_id, IFNULL(NULLIF(c.name_set, ''), IFNULL(c.name, '')), IFNULL(NULLIF(c.number_set, ''), IFNULL(c.number, '')), r.title, r.sub, r.start, r.stop, r.status, r.error, r.path, r.rule_id`

func scanRec(sc interface{ Scan(...any) error }) (*Recording, error) {
	var r Recording
	var rule sql.NullInt64
	if err := sc.Scan(&r.ID, &r.ChannelID, &r.Channel, &r.Number, &r.Title, &r.Sub, &r.Start, &r.Stop, &r.Status, &r.Error, &r.Path, &rule); err != nil {
		return nil, err
	}
	if rule.Valid {
		r.RuleID = &rule.Int64
	}
	if r.Path != "" {
		if st, err := os.Stat(r.Path); err == nil {
			r.SizeBytes = st.Size()
		}
	}
	return &r, nil
}

// Recordings lists every recording, newest first.
func (m *Manager) Recordings(ctx context.Context) ([]*Recording, error) {
	rows, err := m.DB.QueryContext(ctx, `SELECT `+recCols+` FROM recordings r LEFT JOIN tuner_channels c ON c.id = r.channel_id ORDER BY r.start DESC LIMIT 2000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Recording{}
	for rows.Next() {
		r, err := scanRec(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Schedule adds a recording. A channel that is encrypted is refused here, not
// when the time comes.
func (m *Manager) Schedule(ctx context.Context, channelID, userID int64, title, sub string, start, stop int64) (*Recording, error) {
	c, err := m.Channel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if c.Encrypted != "" {
		return nil, &wittytuner.EncryptedError{Channel: c.Name, Why: c.Encrypted}
	}
	if c.NoRecord != "" {
		return nil, fmt.Errorf("%s can't be recorded: %s", c.Name, c.NoRecord)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("name the recording")
	}
	if stop <= start || stop-start > 24*3600 {
		return nil, errors.New("recordings run from a moment to a day")
	}
	if stop <= time.Now().Unix() {
		return nil, errors.New("that programme has already finished")
	}
	var dup int64
	if m.DB.QueryRowContext(ctx, `SELECT id FROM recordings WHERE channel_id = ? AND start = ? AND status IN ('scheduled', 'recording')`, channelID, start).Scan(&dup) == nil {
		return nil, errors.New("that programme is already being recorded")
	}
	res, err := m.DB.ExecContext(ctx, `INSERT INTO recordings(channel_id, user_id, title, sub, start, stop, status, created_at) VALUES(?,?,?,?,?,?,'scheduled',?)`,
		channelID, userID, clip(title, 300), clip(sub, 300), start, stop, db.Now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	m.Start()
	return m.recording(ctx, id)
}

func (m *Manager) recording(ctx context.Context, id int64) (*Recording, error) {
	r, err := scanRec(m.DB.QueryRowContext(ctx, `SELECT `+recCols+` FROM recordings r LEFT JOIN tuner_channels c ON c.id = r.channel_id WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

// CancelRecording stops or removes a recording. Files stay unless deleteFile.
func (m *Manager) CancelRecording(ctx context.Context, id int64, deleteFile bool) error {
	r, err := m.recording(ctx, id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	cancel := m.running()[id]
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if deleteFile && r.Path != "" {
		if err := os.Remove(r.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("the file could not be deleted: %w", err)
		}
	}
	_, err = m.DB.ExecContext(ctx, `DELETE FROM recordings WHERE id = ?`, id)
	return err
}

// AddRule records every airing of a title on a channel.
func (m *Manager) AddRule(ctx context.Context, channelID, userID int64, title string) (*Rule, error) {
	c, err := m.Channel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if c.Encrypted != "" {
		return nil, &wittytuner.EncryptedError{Channel: c.Name, Why: c.Encrypted}
	}
	if c.NoRecord != "" {
		return nil, fmt.Errorf("%s can't be recorded: %s", c.Name, c.NoRecord)
	}
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 300 {
		return nil, errors.New("name what to record")
	}
	res, err := m.DB.ExecContext(ctx, `INSERT INTO recording_rules(channel_id, user_id, title, created_at) VALUES(?,?,?,?)`, channelID, userID, title, db.Now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	m.Start()
	return &Rule{ID: id, ChannelID: channelID, Channel: c.Name, Title: title}, nil
}

// Rules lists series recordings.
func (m *Manager) Rules(ctx context.Context) ([]Rule, error) {
	rows, err := m.DB.QueryContext(ctx, `SELECT r.id, r.channel_id, IFNULL(NULLIF(c.name_set, ''), IFNULL(c.name, '')), r.title FROM recording_rules r LEFT JOIN tuner_channels c ON c.id = r.channel_id ORDER BY r.title COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.ChannelID, &r.Channel, &r.Title); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteRule stops a series recording. What is scheduled from it goes too.
func (m *Manager) DeleteRule(ctx context.Context, id int64) error {
	res, err := m.DB.ExecContext(ctx, `DELETE FROM recording_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, err = m.DB.ExecContext(ctx, `DELETE FROM recordings WHERE rule_id = ? AND status = 'scheduled'`, id)
	return err
}

// applyRules schedules new airings of each rule, from the guide.
func (m *Manager) applyRules(ctx context.Context) {
	rules, err := m.Rules(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, rule := range rules {
		c, err := m.Channel(ctx, rule.ChannelID)
		if err != nil {
			continue
		}
		list, err := m.Guide(ctx, c, now, now.AddDate(0, 0, 7))
		if err != nil {
			continue
		}
		for _, p := range list {
			if !strings.EqualFold(p.Title, rule.Title) || p.Start < now.Unix() {
				continue
			}
			// One statement: the scheduler and a new rule can both be here at once.
			_, _ = m.DB.ExecContext(ctx, `INSERT INTO recordings(channel_id, title, sub, start, stop, status, rule_id, created_at)
				SELECT ?,?,?,?,?,'scheduled',?,? WHERE NOT EXISTS (SELECT 1 FROM recordings WHERE channel_id = ? AND start = ?)`,
				rule.ChannelID, p.Title, p.Sub, p.Start, p.End, rule.ID, db.Now(), rule.ChannelID, p.Start)
		}
	}
}

// running must be called with m.mu held.
func (m *Manager) running() map[int64]context.CancelFunc {
	if m.recordingCancels == nil {
		m.recordingCancels = map[int64]context.CancelFunc{}
	}
	return m.recordingCancels
}

// failInterrupted runs once at startup: anything marked as recording was
// interrupted by a restart.
func (m *Manager) failInterrupted() {
	rows, err := m.DB.Query(`SELECT id FROM recordings WHERE status = 'recording'`)
	if err != nil {
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		m.finish(id, "failed", "the server stopped during this recording", "")
	}
}

// recLog is the logger of one recording. Its session comes from the recording's
// ID, so what a recording says can be found from the recording.
func (m *Manager) recLog(id int64) *slog.Logger {
	return m.Log.With("session", recSession(id), "recording", id)
}

func recSession(id int64) string { return "rec-" + strconv.FormatInt(id, 10) }

// dvrLoop starts recordings when their time comes. It does almost nothing
// when none are due.
func (m *Manager) dvrLoop() {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	n := 0
	for {
		if n%20 == 0 {
			m.applyRules(m.stopCtx)
		}
		n++
		m.startDue(m.stopCtx)
		select {
		case <-m.stopCtx.Done():
			return
		case <-t.C:
		}
	}
}

func (m *Manager) startDue(ctx context.Context) {
	now := time.Now().Unix()
	rows, err := m.DB.QueryContext(ctx, `SELECT `+recCols+` FROM recordings r LEFT JOIN tuner_channels c ON c.id = r.channel_id WHERE r.status = 'scheduled' AND r.start - ? <= ?`, padBefore, now)
	if err != nil {
		return
	}
	var due []*Recording
	for rows.Next() {
		if r, err := scanRec(rows); err == nil {
			due = append(due, r)
		}
	}
	rows.Close()
	for _, r := range due {
		if r.Stop+padAfter <= now {
			m.finish(r.ID, "failed", "the server was not running when this was due", "")
			continue
		}
		_, _ = m.DB.ExecContext(ctx, `UPDATE recordings SET status = 'recording' WHERE id = ?`, r.ID)
		logging.Go("recording", func() { m.record(r) }, "cat", "tv", "session", recSession(r.ID))
	}
}

// finish is where every recording ends, so each that fails leaves a record here.
func (m *Manager) finish(id int64, status, msg, path string) {
	_, err := m.DB.Exec(`UPDATE recordings SET status = ?, error = ?, path = CASE WHEN ? <> '' THEN ? ELSE path END WHERE id = ?`, status, msg, path, path, id)
	log := m.recLog(id)
	if status == "failed" {
		log.Error("recording failed", "reason", msg, "file", path)
	} else {
		log.Info("recording finished", "status", status)
	}
	if err != nil {
		log.Error("could not save how a recording ended", "status", status, "err", err)
	}
}

// record runs one recording to its end.
func (m *Manager) record(r *Recording) {
	end := time.Unix(r.Stop+padAfter, 0)
	ctx, cancel := context.WithDeadline(logging.WithSession(m.stopCtx, recSession(r.ID)), end)
	defer cancel()
	log := m.recLog(r.ID)
	log.Info("recording started", "channel", r.ChannelID)
	defer func() {
		if p := recover(); p != nil {
			logging.ReportPanic("recording", p, "cat", "tv", "session", recSession(r.ID))
			m.finish(r.ID, "failed", "this recording stopped on an internal error", "")
		}
	}()
	m.mu.Lock()
	m.running()[r.ID] = cancel
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.running(), r.ID)
		m.mu.Unlock()
	}()
	if c, err := m.Channel(ctx, r.ChannelID); err == nil && c.NoRecord != "" {
		m.finish(r.ID, "failed", fmt.Sprintf("%s can't be recorded: %s", c.Name, c.NoRecord), "")
		return
	}
	opened, err := m.openForRecording(ctx, r.ChannelID)
	if err != nil {
		m.finish(r.ID, "failed", err.Error(), "")
		return
	}
	done := ""
	defer func() {
		if done != "" && m.AfterRecording != nil {
			m.AfterRecording(done)
		}
	}()
	defer opened.Release() // first: whoever is told may use the tuner
	path, err := m.recordPath(r)
	if err != nil {
		m.finish(r.ID, "failed", err.Error(), "")
		return
	}
	_, _ = m.DB.Exec(`UPDATE recordings SET path = ? WHERE id = ?`, path, r.ID)
	err = m.write(ctx, log, opened, path)
	if err != nil && wittytuner.IsEncrypted(err) {
		_ = os.Remove(path)
		m.finish(r.ID, "failed", wittytuner.RefusalMessage(opened.Channel.Name, err).Error(), "")
		return
	}
	// The deadline ending a recording is normal; so is a viewer cancelling it.
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) && ctx.Err() == nil {
		m.finish(r.ID, "failed", err.Error(), path)
		return
	}
	m.finish(r.ID, "done", "", path)
	done = path
}

// openForRecording takes a tuner, stopping a live viewer if that is the only
// way: a recording cannot wait, and someone watching can start again.
func (m *Manager) openForRecording(ctx context.Context, channelID int64) (*Opened, error) {
	var last error
	for i := 0; i < 3; i++ {
		o, err := m.Open(ctx, channelID)
		if err == nil {
			return o, nil
		}
		last = err
		if !errors.Is(err, ErrBusy) {
			return nil, err
		}
		c, cerr := m.Channel(ctx, channelID)
		if cerr != nil || !m.freeTuner(ctx, c.TunerID) {
			return nil, errors.New("no tuner was free when the recording started")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, last
}

func (m *Manager) write(ctx context.Context, log *slog.Logger, o *Opened, path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if o.Stream.Reader != nil {
		stop := context.AfterFunc(ctx, func() { _ = o.Stream.Close() })
		defer stop()
		_, err := io.Copy(f, o.Stream.Reader)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	if m.Tools == nil {
		return errors.New("FFmpeg is needed to record this source")
	}
	args := append([]string{"-hide_banner", "-loglevel", "error", "-nostdin"}, o.Stream.Input...)
	args = append(args, "-map", "0", "-c", "copy", "-f", "mpegts", "pipe:1")
	cmd := ffmpeg.Command(ctx, m.Tools.FFmpeg, args...)
	cmd.Stdout = f
	var stderr logging.Tail
	cmd.Stderr = &stderr
	started := time.Now()
	err = cmd.Run()
	logging.ChildExit(log, "ffmpeg", err, ctx.Err() != nil, &stderr, started)
	return err
}

var unsafeName = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// safeFileName makes a title safe to use as a file name on every platform.
func safeFileName(s string) string {
	s = strings.TrimSpace(unsafeName.ReplaceAllString(s, " "))
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, ". ")
	if s == "" {
		s = "Recording"
	}
	if len(s) > 120 {
		s = clip(s, 120)
	}
	return s
}

// recordPath: <folder>/<title>/<title> - <date> <time>.ts, always inside the
// recordings folder.
func (m *Manager) recordPath(r *Recording) (string, error) {
	folder := ""
	if m.Folder != nil {
		folder = m.Folder()
	}
	if folder == "" {
		return "", errors.New("no recordings folder is set")
	}
	title := safeFileName(r.Title)
	dir := filepath.Join(folder, title)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("the recordings folder could not be used: %w", err)
	}
	stamp := time.Unix(r.Start, 0).Format("2006-01-02 15.04")
	name := title + " - " + stamp
	if r.Sub != "" {
		name += " - " + safeFileName(r.Sub)
	}
	p := filepath.Join(dir, name+".ts")
	for i := 2; ; i++ {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p, nil
		}
		p = filepath.Join(dir, name+" ("+strconv.Itoa(i)+").ts")
	}
}
