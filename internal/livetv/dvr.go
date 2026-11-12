// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

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
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
	"github.com/wittywilla/wittymediaserver/internal/tuner"
)

// Recording: tuner channels only. Raw MPEG-TS goes straight to disk, which
// costs almost nothing; other sources go through FFmpeg in copy mode. A
// recording takes a tuner when it needs one, stopping a live viewer if it must.

// Recording is one recording, past or to come.
type Recording struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"` // who set it up, or whose rule it came from; 0 for nobody now
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
	// RuleTitle names the series recording this came from.
	RuleTitle string `json:"ruleTitle,omitempty"`
	// PadBefore and PadAfter are minutes more than the usual minute and two either side.
	PadBefore int `json:"padBefore,omitempty"`
	PadAfter  int `json:"padAfter,omitempty"`
	// Conflict: when it starts, more recordings want its tuner than it has. See markConflicts.
	Conflict bool `json:"conflict,omitempty"`
	tunerID  int64
}

// Rule records every airing of a title on a channel.
type Rule struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	ChannelID int64  `json:"channelId"`
	Channel   string `json:"channel"`
	Title     string `json:"title"`
	RuleOptions
}

// RuleOptions are how a series recording goes about it.
type RuleOptions struct {
	// AnyChannel: record the title wherever it airs, not only on its channel.
	AnyChannel bool `json:"anyChannel"`
	// NewOnly skips an episode this rule has already recorded: the same season and episode,
	// or failing those the same episode title.
	NewOnly bool `json:"newOnly"`
	// Keep is how many finished recordings of this rule to keep, newest first; 0 keeps all.
	Keep int `json:"keep"`
	// PadBefore and PadAfter: minutes more either side.
	PadBefore int `json:"padBefore"`
	PadAfter  int `json:"padAfter"`
}

// Check holds the options to what makes sense.
func (o RuleOptions) Check() error {
	if o.Keep < 0 || o.Keep > 1000 {
		return errors.New("keep between 0 (all of them) and 1000 recordings")
	}
	return checkPadding(o.PadBefore, o.PadAfter)
}

func checkPadding(before, after int) error {
	if before < 0 || before > 60 || after < 0 || after > 180 {
		return errors.New("start up to an hour early, and end up to three hours late")
	}
	return nil
}

// Padding starts recordings early and ends them late, as broadcasts drift.
const (
	padBefore = 60
	padAfter  = 120
)

// sameAiring: two starts this close on one channel are one programme, however its title is spelled.
const sameAiring = 60

// How many recordings run at once, whichever tuners they use: a source that names no limit
// of its own could otherwise be asked for any number.
const maxRecordingsAtOnce = 16

// recordingFreeFloor is the room a recording leaves on its disk. The server's database may
// share that disk, and stops when it's full. recordingFreeEvery is how often it looks.
var (
	recordingFreeFloor int64 = 2 << 30
	recordingFreeEvery       = 30 * time.Second
)

const recCols = `r.id, IFNULL(r.user_id, IFNULL((SELECT u.user_id FROM recording_rules u WHERE u.id = r.rule_id), 0)), r.channel_id, IFNULL(NULLIF(c.name_set, ''), IFNULL(c.name, '')), IFNULL(NULLIF(c.number_set, ''), IFNULL(c.number, '')), r.title, r.sub, r.start, r.stop, r.status, r.error, r.path, r.rule_id,
	IFNULL((SELECT u.title FROM recording_rules u WHERE u.id = r.rule_id), ''), r.pad_before, r.pad_after, IFNULL(c.tuner_id, 0)`

func (m *Manager) scanRec(sc interface{ Scan(...any) error }) (*Recording, error) {
	var r Recording
	var rule sql.NullInt64
	if err := sc.Scan(&r.ID, &r.UserID, &r.ChannelID, &r.Channel, &r.Number, &r.Title, &r.Sub, &r.Start, &r.Stop, &r.Status, &r.Error, &r.Path, &rule,
		&r.RuleTitle, &r.PadBefore, &r.PadAfter, &r.tunerID); err != nil {
		return nil, err
	}
	if rule.Valid {
		r.RuleID = &rule.Int64
	}
	if m.inRecordingsFolder(r.Path) {
		if st, err := os.Stat(r.Path); err == nil {
			r.SizeBytes = st.Size()
		}
	}
	return &r, nil
}

// inRecordingsFolder says a path is a file this server would have recorded to. A row's path can
// have come from a backup, whoever made that, so nothing outside the folder is looked at or removed.
func (m *Manager) inRecordingsFolder(p string) bool {
	if p == "" || m.Folder == nil {
		return false
	}
	folder := m.Folder()
	if folder == "" {
		return false
	}
	rel, err := filepath.Rel(folder, p)
	return err == nil && rel != "." && filepath.IsLocal(rel)
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
		r, err := m.scanRec(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	m.markConflicts(ctx, out)
	return out, nil
}

// markConflicts flags each scheduled recording that, when it starts, would want its tuner
// alongside more recordings than the tuner can stream at once. A tuner that names no limit
// never conflicts. Watching isn't counted: a recording stops a viewer, not the other way round.
func (m *Manager) markConflicts(ctx context.Context, list []*Recording) {
	limits := map[int64]int{}
	if ts, err := m.Tuners(ctx); err == nil {
		for _, t := range ts {
			limits[t.ID] = t.MaxStreams
			if limits[t.ID] == 0 {
				if s, _, err := m.source(ctx, t.ID); err == nil {
					limits[t.ID] = s.Info().MaxStreams
				}
			}
		}
	}
	busy := func(r *Recording) bool { return r.Status == "scheduled" || r.Status == "recording" }
	for _, r := range list {
		limit := limits[r.tunerID]
		if r.Status != "scheduled" || limit <= 0 {
			continue
		}
		at := r.Start - m.padBeforeOf(r)
		n := 0
		for _, o := range list {
			if busy(o) && o.tunerID == r.tunerID && o.Start-m.padBeforeOf(o) <= at && o.Stop+m.padAfterOf(o) > at {
				n++
			}
		}
		r.Conflict = n > limit
	}
}

func (m *Manager) padBeforeOf(r *Recording) int64 { return max(padBefore, int64(r.PadBefore)*60) }
func (m *Manager) padAfterOf(r *Recording) int64  { return max(padAfter, int64(r.PadAfter)*60) }

// Schedule adds a recording. A channel that is encrypted is refused here, not
// when the time comes.
func (m *Manager) Schedule(ctx context.Context, channelID, userID int64, title, sub string, start, stop int64) (*Recording, error) {
	return m.SchedulePadded(ctx, channelID, userID, title, sub, start, stop, 0, 0)
}

// SchedulePadded is Schedule, starting padBefore minutes early and ending padAfter late.
func (m *Manager) SchedulePadded(ctx context.Context, channelID, userID int64, title, sub string, start, stop int64, padBeforeMin, padAfterMin int) (*Recording, error) {
	if err := checkPadding(padBeforeMin, padAfterMin); err != nil {
		return nil, err
	}
	c, err := m.Channel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if !c.Enabled {
		return nil, ErrNotFound
	}
	if c.Encrypted != "" {
		return nil, &tuner.EncryptedError{Channel: c.Name, Why: c.Encrypted}
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
	if m.DB.QueryRowContext(ctx, `SELECT id FROM recordings WHERE channel_id = ? AND start > ? AND start < ? AND status IN ('scheduled', 'recording')`, channelID, start-sameAiring, start+sameAiring).Scan(&dup) == nil {
		return nil, errors.New("that programme is already being recorded")
	}
	res, err := m.DB.ExecContext(ctx, `INSERT INTO recordings(channel_id, user_id, title, sub, start, stop, status, created_at, pad_before, pad_after) VALUES(?,?,?,?,?,?,'scheduled',?,?,?)`,
		channelID, userID, clip(title, 300), clip(sub, 300), start, stop, db.Now(), padBeforeMin, padAfterMin)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	m.recLog(id).InfoContext(ctx, "recording scheduled", "channel", channelID, "title", title, "start", time.Unix(start, 0).UTC().Format(time.RFC3339),
		"minutes", (stop-start)/60, "account", userID)
	m.Start()
	return m.recording(ctx, id)
}

// Recording is one recording.
func (m *Manager) Recording(ctx context.Context, id int64) (*Recording, error) {
	return m.recording(ctx, id)
}

// PendingRecordings is how many recordings of an account are yet to finish.
func (m *Manager) PendingRecordings(ctx context.Context, userID int64) (int, error) {
	var n int
	err := m.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM recordings r WHERE status IN ('scheduled', 'recording')
		AND IFNULL(r.user_id, IFNULL((SELECT u.user_id FROM recording_rules u WHERE u.id = r.rule_id), 0)) = ?`, userID).Scan(&n)
	return n, err
}

// RuleCount is how many series recordings an account has.
func (m *Manager) RuleCount(ctx context.Context, userID int64) (int, error) {
	var n int
	err := m.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM recording_rules WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

// RuleOwner is who made a series recording; 0 for nobody now.
func (m *Manager) RuleOwner(ctx context.Context, id int64) (int64, error) {
	var owner sql.NullInt64
	err := m.DB.QueryRowContext(ctx, `SELECT user_id FROM recording_rules WHERE id = ?`, id).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return owner.Int64, err
}

// ForgetUser leaves an account's recordings with nobody, for administrators to look after.
// Numbers of removed accounts come round again, and these shouldn't go with them.
func (m *Manager) ForgetUser(ctx context.Context, userID int64) error {
	if _, err := m.DB.ExecContext(ctx, `UPDATE recordings SET user_id = NULL WHERE user_id = ?`, userID); err != nil {
		return err
	}
	_, err := m.DB.ExecContext(ctx, `UPDATE recording_rules SET user_id = NULL WHERE user_id = ?`, userID)
	return err
}

func (m *Manager) recording(ctx context.Context, id int64) (*Recording, error) {
	r, err := m.scanRec(m.DB.QueryRowContext(ctx, `SELECT `+recCols+` FROM recordings r LEFT JOIN tuner_channels c ON c.id = r.channel_id WHERE r.id = ?`, id))
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
	log := m.recLog(id)
	if deleteFile && m.inRecordingsFolder(r.Path) {
		if err := os.Remove(r.Path); err != nil && !os.IsNotExist(err) {
			log.WarnContext(ctx, "could not delete a recording's file", "file", r.Path, "err", err)
			return fmt.Errorf("the file could not be deleted: %w", err)
		}
	}
	if _, err = m.DB.ExecContext(ctx, `DELETE FROM recordings WHERE id = ?`, id); err != nil {
		return err
	}
	what := "recording cancelled"
	if r.Status != "scheduled" && r.Status != "recording" {
		what = "recording removed"
	}
	log.InfoContext(ctx, what, "status", r.Status, "wasRunning", cancel != nil, "fileDeleted", deleteFile && r.Path != "")
	return nil
}

// AddRule records every airing of a title on a channel.
func (m *Manager) AddRule(ctx context.Context, channelID, userID int64, title string) (*Rule, error) {
	return m.AddRuleWith(ctx, channelID, userID, title, RuleOptions{})
}

// AddRuleWith is AddRule with options.
func (m *Manager) AddRuleWith(ctx context.Context, channelID, userID int64, title string, o RuleOptions) (*Rule, error) {
	if err := o.Check(); err != nil {
		return nil, err
	}
	c, err := m.Channel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if !c.Enabled {
		return nil, ErrNotFound
	}
	if c.Encrypted != "" {
		return nil, &tuner.EncryptedError{Channel: c.Name, Why: c.Encrypted}
	}
	if c.NoRecord != "" {
		return nil, fmt.Errorf("%s can't be recorded: %s", c.Name, c.NoRecord)
	}
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 300 {
		return nil, errors.New("name what to record")
	}
	res, err := m.DB.ExecContext(ctx, `INSERT INTO recording_rules(channel_id, user_id, title, created_at, any_channel, new_only, keep, pad_before, pad_after) VALUES(?,?,?,?,?,?,?,?,?)`,
		channelID, userID, title, db.Now(), boolInt(o.AnyChannel), boolInt(o.NewOnly), o.Keep, o.PadBefore, o.PadAfter)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	m.Log.InfoContext(ctx, "series recording added", "rule", id, "channel", channelID, "title", title, "account", userID,
		"anyChannel", o.AnyChannel, "newOnly", o.NewOnly, "keep", o.Keep)
	m.Start()
	return &Rule{ID: id, UserID: userID, ChannelID: channelID, Channel: c.Name, Title: title, RuleOptions: o}, nil
}

// UpdateRule changes how a series recording goes about it. What it has scheduled is
// scheduled again from the guide with the new options.
func (m *Manager) UpdateRule(ctx context.Context, id int64, o RuleOptions) error {
	if err := o.Check(); err != nil {
		return err
	}
	res, err := m.DB.ExecContext(ctx, `UPDATE recording_rules SET any_channel = ?, new_only = ?, keep = ?, pad_before = ?, pad_after = ? WHERE id = ?`,
		boolInt(o.AnyChannel), boolInt(o.NewOnly), o.Keep, o.PadBefore, o.PadAfter, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := m.DB.ExecContext(ctx, `DELETE FROM recordings WHERE rule_id = ? AND status = 'scheduled'`, id); err != nil {
		return err
	}
	m.Log.InfoContext(ctx, "series recording changed", "rule", id, "anyChannel", o.AnyChannel, "newOnly", o.NewOnly, "keep", o.Keep,
		"padBefore", o.PadBefore, "padAfter", o.PadAfter)
	m.applyRules(ctx)
	return nil
}

// Rules lists series recordings.
func (m *Manager) Rules(ctx context.Context) ([]Rule, error) {
	rows, err := m.DB.QueryContext(ctx, `SELECT r.id, IFNULL(r.user_id, 0), r.channel_id, IFNULL(NULLIF(c.name_set, ''), IFNULL(c.name, '')), r.title,
		r.any_channel, r.new_only, r.keep, r.pad_before, r.pad_after FROM recording_rules r LEFT JOIN tuner_channels c ON c.id = r.channel_id ORDER BY r.title COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		var r Rule
		var anyCh, newOnly int
		if err := rows.Scan(&r.ID, &r.UserID, &r.ChannelID, &r.Channel, &r.Title, &anyCh, &newOnly, &r.Keep, &r.PadBefore, &r.PadAfter); err != nil {
			return nil, err
		}
		r.AnyChannel, r.NewOnly = anyCh != 0, newOnly != 0
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
	res, err = m.DB.ExecContext(ctx, `DELETE FROM recordings WHERE rule_id = ? AND status = 'scheduled'`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	m.Log.InfoContext(ctx, "series recording removed", "rule", id, "unscheduled", n)
	return nil
}

// applyRules schedules new airings of each rule, from the guide.
func (m *Manager) applyRules(ctx context.Context) {
	rules, err := m.Rules(ctx)
	if err != nil {
		if ctx.Err() == nil {
			m.Log.WarnContext(ctx, "could not read the series recordings to schedule them", "err", err)
		}
		return
	}
	now := time.Now()
	added := 0
	defer func() {
		if added > 0 {
			m.Log.InfoContext(ctx, "series recordings scheduled", "recordings", added, "rules", len(rules))
		}
	}()
	for _, rule := range rules {
		for _, a := range m.airings(ctx, rule, now, now.AddDate(0, 0, 7)) {
			p := a.p
			if rule.NewOnly && m.alreadyRecorded(ctx, rule.ID, p) {
				continue
			}
			// One statement: the scheduler and a new rule can both be here at once. The same
			// airing is recorded once per rule, whichever channel it was found on.
			// The database logs a failed statement itself; the next pass tries again.
			res, err := m.DB.ExecContext(ctx, `INSERT INTO recordings(channel_id, user_id, title, sub, start, stop, status, rule_id, created_at, pad_before, pad_after, season, episode)
				SELECT ?,NULLIF(?, 0),?,?,?,?,'scheduled',?,?,?,?,?,? WHERE NOT EXISTS (SELECT 1 FROM recordings WHERE (channel_id = ? OR rule_id = ?) AND start = ?)`,
				a.channel, rule.UserID, p.Title, p.Sub, p.Start, p.End, rule.ID, db.Now(), rule.PadBefore, rule.PadAfter, p.Season, p.Episode, a.channel, rule.ID, p.Start)
			if err == nil {
				if n, _ := res.RowsAffected(); n > 0 {
					added++
					m.Log.DebugContext(ctx, "series recording scheduled an airing", "rule", rule.ID, "channel", a.channel, "title", p.Title,
						"start", time.Unix(p.Start, 0).UTC().Format(time.RFC3339))
				}
			}
		}
	}
}

type airing struct {
	channel int64
	p       Programme
}

// airings are a rule's programmes still to come: on its own channel, or, for a rule that
// follows a title anywhere, on every channel that can be recorded, one channel per airing.
func (m *Manager) airings(ctx context.Context, rule Rule, from, to time.Time) []airing {
	var out []airing
	if !rule.AnyChannel {
		c, err := m.Channel(ctx, rule.ChannelID)
		if err != nil || !c.Enabled {
			return nil
		}
		list, err := m.Guide(ctx, c, from, to)
		if err != nil {
			return nil
		}
		for _, p := range list {
			if strings.EqualFold(p.Title, rule.Title) && p.Start >= from.Unix() {
				out = append(out, airing{c.ID, p})
			}
		}
		return out
	}
	rows, err := m.DB.QueryContext(ctx, `SELECT channel, start, stop, title, sub, season, episode FROM epg_programmes
		WHERE title = ? COLLATE NOCASE AND start >= ? AND start < ? ORDER BY start LIMIT 500`, rule.Title, from.Unix(), to.Unix())
	if err != nil {
		return nil
	}
	type found struct {
		key string
		p   Programme
	}
	var hits []found
	for rows.Next() {
		var f found
		var season, episode sql.NullInt64
		if rows.Scan(&f.key, &f.p.Start, &f.p.End, &f.p.Title, &f.p.Sub, &season, &episode) != nil {
			continue
		}
		if season.Valid {
			v := int(season.Int64)
			f.p.Season = &v
		}
		if episode.Valid {
			v := int(episode.Int64)
			f.p.Episode = &v
		}
		hits = append(hits, f)
	}
	rows.Close()
	seen := map[int64]bool{}
	for _, f := range hits {
		if seen[f.p.Start] {
			continue
		}
		if id := m.recordableFor(ctx, f.key, rule.ChannelID); id != 0 {
			seen[f.p.Start] = true
			out = append(out, airing{id, f.p})
		}
	}
	return out
}

// recordableFor is a channel that guide data with this key belongs to and that can be
// recorded, the rule's own channel first.
func (m *Manager) recordableFor(ctx context.Context, key string, prefer int64) int64 {
	var ids []int64
	if strings.HasPrefix(key, "tuner:") {
		if id, err := strconv.ParseInt(strings.TrimPrefix(key, "tuner:"), 10, 64); err == nil {
			ids = append(ids, id)
		}
	} else {
		rows, err := m.DB.QueryContext(ctx, `SELECT id FROM tuner_channels WHERE epg_set = ? OR (epg_set = '' AND epg_id = ?) ORDER BY id != ?, id LIMIT 20`, key, key, prefer)
		if err != nil {
			return 0
		}
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
	}
	for _, id := range ids {
		c, err := m.Channel(ctx, id)
		if err == nil && c.Enabled && c.Encrypted == "" && c.NoRecord == "" && c.MergedInto == 0 {
			return id
		}
	}
	return 0
}

// alreadyRecorded: this rule has recorded, or will record, this episode.
func (m *Manager) alreadyRecorded(ctx context.Context, ruleID int64, p Programme) bool {
	var n int
	switch {
	case p.Season != nil && p.Episode != nil:
		_ = m.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM recordings WHERE rule_id = ? AND season = ? AND episode = ? AND status IN ('scheduled', 'recording', 'done') AND start != ?`,
			ruleID, *p.Season, *p.Episode, p.Start).Scan(&n)
	case strings.TrimSpace(p.Sub) != "":
		_ = m.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM recordings WHERE rule_id = ? AND sub = ? COLLATE NOCASE AND status IN ('scheduled', 'recording', 'done') AND start != ?`,
			ruleID, strings.TrimSpace(p.Sub), p.Start).Scan(&n)
	}
	return n > 0
}

// keepNewest removes a rule's oldest finished recordings, files and all, beyond what it
// keeps. Only files in the recordings folder are ever removed.
func (m *Manager) keepNewest(ctx context.Context, recordingID int64) {
	var ruleID sql.NullInt64
	var keep int
	if m.DB.QueryRowContext(ctx, `SELECT r.rule_id, IFNULL(u.keep, 0) FROM recordings r LEFT JOIN recording_rules u ON u.id = r.rule_id WHERE r.id = ?`, recordingID).Scan(&ruleID, &keep) != nil ||
		!ruleID.Valid || keep <= 0 {
		return
	}
	rows, err := m.DB.QueryContext(ctx, `SELECT id, path FROM recordings WHERE rule_id = ? AND status = 'done' ORDER BY start DESC LIMIT -1 OFFSET ?`, ruleID.Int64, keep)
	if err != nil {
		return
	}
	type old struct {
		id   int64
		path string
	}
	var gone []old
	for rows.Next() {
		var o old
		if rows.Scan(&o.id, &o.path) == nil {
			gone = append(gone, o)
		}
	}
	rows.Close()
	for _, o := range gone {
		if m.inRecordingsFolder(o.path) {
			if err := os.Remove(o.path); err != nil && !os.IsNotExist(err) {
				m.recLog(o.id).Warn("could not remove an old recording a series no longer keeps", "err", err)
				continue
			}
		}
		_, _ = m.DB.ExecContext(ctx, `DELETE FROM recordings WHERE id = ?`, o.id)
		m.recLog(o.id).Info("removed an old recording: its series keeps only the newest", "keep", keep)
	}
}

// running must be called with m.mu held.
func (m *Manager) running() map[int64]context.CancelFunc {
	if m.recordingCancels == nil {
		m.recordingCancels = map[int64]context.CancelFunc{}
	}
	return m.recordingCancels
}

// stoppedReason is why a recording the server's stopping cut short failed: said as it stops, or
// at the next start if it couldn't say then.
const stoppedReason = "the server stopped during this recording"

// failInterrupted runs once at startup: anything marked as recording was
// interrupted by a restart.
func (m *Manager) failInterrupted() {
	rows, err := m.DB.Query(`SELECT id FROM recordings WHERE status = 'recording'`)
	if err != nil {
		m.Log.Warn("could not look for recordings a stop interrupted", "err", err)
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
		m.finish(id, "failed", stoppedReason, "")
	}
	if len(ids) > 0 {
		m.Log.Warn("recordings were interrupted when the server stopped", "recordings", len(ids))
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
func (m *Manager) dvrLoop(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	n := 0
	for {
		if n%20 == 0 {
			m.applyRules(ctx)
		}
		n++
		m.startDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (m *Manager) startDue(ctx context.Context) {
	now := time.Now().Unix()
	rows, err := m.DB.QueryContext(ctx, `SELECT `+recCols+` FROM recordings r LEFT JOIN tuner_channels c ON c.id = r.channel_id WHERE r.status = 'scheduled' AND r.start - MAX(?, r.pad_before * 60) <= ?`, padBefore, now)
	if err != nil {
		return
	}
	var due []*Recording
	for rows.Next() {
		if r, err := m.scanRec(rows); err == nil {
			due = append(due, r)
		}
	}
	rows.Close()
	for _, r := range due {
		if r.Stop+m.padAfterOf(r) <= now {
			m.finish(r.ID, "failed", "the server was not running when this was due", "")
			continue
		}
		// Whoever changes the status starts it, so a recording never runs twice.
		res, err := m.DB.ExecContext(ctx, `UPDATE recordings SET status = 'recording' WHERE id = ? AND status = 'scheduled'`, r.ID)
		if err != nil {
			continue
		}
		if n, err := res.RowsAffected(); err != nil || n == 0 {
			continue
		}
		started := m.background("recording", func(context.Context) {
			task := m.Tasks.Start(tasks.Spec{Type: tasks.TypeRecording, Title: "Recording " + r.Title, AdminOnly: true})
			m.record(r) // it stops with the manager
			var status, msg string
			_ = m.DB.QueryRow(`SELECT status, error FROM recordings WHERE id = ?`, r.ID).Scan(&status, &msg)
			if status == "failed" {
				task.Fail("The recording failed. "+msg, "See the recordings page.", nil)
			} else {
				task.Finish(nil)
			}
		}, "cat", "tv", "session", recSession(r.ID))
		if !started { // the server is stopping: it starts next time, if it's still on
			_, _ = m.DB.Exec(`UPDATE recordings SET status = 'scheduled' WHERE id = ? AND status = 'recording'`, r.ID)
		}
	}
}

const diskFullReason = "the disk it records to is nearly full"

func diskNearlyFull(dir string) bool {
	free := sysinfo.FreeBytes(dir)
	return free >= 0 && free < recordingFreeFloor
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
	if status == "done" {
		m.keepNewest(m.stopCtx, id)
	}
}

// record runs one recording to its end.
func (m *Manager) record(r *Recording) {
	end := time.Unix(r.Stop+m.padAfterOf(r), 0)
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
	busy := len(m.running()) >= maxRecordingsAtOnce
	if !busy {
		m.running()[r.ID] = cancel
	}
	m.mu.Unlock()
	if busy {
		m.finish(r.ID, "failed", "too many recordings were already running", "")
		return
	}
	defer func() {
		m.mu.Lock()
		delete(m.running(), r.ID)
		m.mu.Unlock()
	}()
	if c, err := m.Channel(ctx, r.ChannelID); err == nil && !c.Enabled {
		m.finish(r.ID, "failed", "this channel has been turned off", "")
		return
	} else if err == nil && c.NoRecord != "" {
		m.finish(r.ID, "failed", fmt.Sprintf("%s can't be recorded: %s", c.Name, c.NoRecord), "")
		return
	}
	opened, err := m.openForRecording(ctx, r.ChannelID)
	if err != nil && m.stopCtx.Err() != nil {
		m.finish(r.ID, "failed", stoppedReason, "")
		return
	}
	if err != nil {
		log.Warn("a recording couldn't start", "err", err)
		m.finish(r.ID, "failed", PublicError(err), "")
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
		log.Warn("a recording has nowhere to go", "err", err)
		m.finish(r.ID, "failed", PublicError(err), "")
		return
	}
	if diskNearlyFull(filepath.Dir(path)) {
		m.finish(r.ID, "failed", diskFullReason, "")
		return
	}
	_, _ = m.DB.Exec(`UPDATE recordings SET path = ? WHERE id = ?`, path, r.ID)
	var outOfRoom atomic.Bool
	writing, stopLooking := context.WithCancel(ctx)
	defer stopLooking()
	m.background("recording disk space", func(context.Context) {
		for {
			select {
			case <-writing.Done():
				return
			case <-time.After(recordingFreeEvery):
				if diskNearlyFull(filepath.Dir(path)) {
					outOfRoom.Store(true)
					stopLooking()
					return
				}
			}
		}
	}, "cat", "tv")
	err = m.write(writing, log, opened, path)
	stopLooking()
	if outOfRoom.Load() {
		m.finish(r.ID, "failed", diskFullReason, path)
		done = path
		return
	}
	if refused := opened.Refusal(); refused != nil {
		// All of it was clear until then, and stays. It stops here, for the same reason
		// as anyone trying to watch.
		if st, serr := os.Stat(path); serr != nil || st.Size() == 0 {
			_ = os.Remove(path)
			m.finish(r.ID, "failed", refused.Error(), "")
			return
		}
		m.finish(r.ID, "failed", refused.Error(), path)
		done = path
		return
	}
	if err != nil && tuner.IsEncrypted(err) {
		_ = os.Remove(path)
		m.finish(r.ID, "failed", tuner.RefusalMessage(opened.Channel.Name, err).Error(), "")
		return
	}
	if m.stopCtx.Err() != nil {
		// What was recorded stays. The library sees it when it's next scanned: nothing new starts
		// as the server stops.
		m.finish(r.ID, "failed", stoppedReason, path)
		return
	}
	// The deadline ending a recording is normal; so is a viewer cancelling it.
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) && ctx.Err() == nil {
		log.Warn("a recording stopped", "err", err)
		m.finish(r.ID, "failed", PublicError(err), path)
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
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	m.background("recording re-check", func(context.Context) {
		select {
		case <-o.Refused():
			cancel()
		case <-run.Done():
		}
	}, "cat", "tv")
	cmd := ffmpeg.Command(run, m.Tools.FFmpeg, args...)
	cmd.Stdout = f
	var stderr logging.Tail
	cmd.Stderr = &stderr
	started := time.Now()
	err = cmd.Run()
	logging.ChildExit(log, "ffmpeg", err, run.Err() != nil, &stderr, started)
	if err != nil && ctx.Err() == nil {
		// FFmpeg may have stopped at the key by itself, before the next look.
		if refused := o.CheckNow(ctx); refused != nil {
			return refused
		}
	}
	if refused := o.Refusal(); refused != nil {
		return refused
	}
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
	// A programme called CON would be a console on Windows, not a folder.
	if library.ReservedName(s) {
		s = "Recording " + s
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
