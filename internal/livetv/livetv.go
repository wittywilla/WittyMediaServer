// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package livetv runs tuners for the server: their settings and channels, the
// guide data from XMLTV and from the broadcasts themselves, opening streams
// within each tuner's limits, and recording.
package livetv

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
	"github.com/wittywilla/wittymediaserver/internal/tuner"
)

// Tuner is one configured source.
type Tuner struct {
	ID         int64             `json:"id"`
	Driver     string            `json:"driver"`
	Name       string            `json:"name"`
	Options    map[string]string `json:"options"`
	Enabled    bool              `json:"enabled"`
	MaxStreams int               `json:"maxStreams"`
	LastScan   *int64            `json:"lastScan"`
	LastError  string            `json:"lastError"`
	Detail     string            `json:"detail"`
	Channels   int               `json:"channels"`
	Encrypted  int               `json:"encrypted"` // channels known to be protected
	Clear      int               `json:"clear"`     // channels known not to be
	InUse      int               `json:"inUse"`
}

// Channel is a tuner's channel as the guide and the settings see it. Number
// and Name are what the viewer sees: the owner's choice, else the source's.
type Channel struct {
	ID        int64  `json:"id"`
	TunerID   int64  `json:"tunerId"`
	SourceID  string `json:"sourceId"`
	Number    string `json:"number"`
	Name      string `json:"name"`
	Group     string `json:"group,omitempty"`
	CallSign  string `json:"callsign,omitempty"`
	Logo      string `json:"-"` // the source's address, never sent to clients: see handleTunerLogo
	HD        bool   `json:"hd,omitempty"`
	Radio     bool   `json:"radio,omitempty"`
	Encrypted string `json:"encrypted,omitempty"`
	// Security is encrypted, clear or unknown (nobody can tell until it is tuned).
	Security   string `json:"security"`
	ATSC3      bool   `json:"atsc3,omitempty"`
	VideoCodec string `json:"videoCodec,omitempty"`
	AudioCodec string `json:"audioCodec,omitempty"`
	// NoRecord is why this channel may not be recorded: the device said so, or
	// the owner did from what the device's documentation says.
	NoRecord  string `json:"noRecord,omitempty"`
	Program   int    `json:"program,omitempty"`
	Enabled   bool   `json:"enabled"`
	EPGID     string `json:"epgId,omitempty"`
	NumberSet string `json:"numberSet,omitempty"`
	NameSet   string `json:"nameSet,omitempty"`
	EPGSet    string `json:"epgSet,omitempty"`
	// LogoSet is what a person chose for its logo: "" to find it by name, "none", or
	// "kind|name" in the logo service.
	LogoSet string `json:"logoSet,omitempty"`
	// CatchupDays: the source keeps this many days of what the channel showed, to be
	// watched from the start. catchup and catchupSource say how to ask for it.
	CatchupDays int `json:"catchupDays,omitempty"`
	// Health is what the last check or tune found: HealthOK, HealthFailing, or "" for not yet.
	Health    string `json:"health,omitempty"`
	HealthAt  int64  `json:"healthAt,omitempty"`
	HealthMsg string `json:"healthMsg,omitempty"`
	// MergedInto: this channel is a backup of that one, hidden from the guide and tried
	// when that one won't open. Backups counts a channel's own backups (settings only).
	MergedInto    int64 `json:"mergedInto,omitempty"`
	Backups       int   `json:"backups,omitempty"`
	catchup       string
	catchupSource string
	locator       string
	sourceName    string
	driver        string
}

// Engine is WittyTuner, as the server reaches it.
type Engine interface {
	New(ctx context.Context, driver string, cfg tuner.Config, env tuner.Env) (tuner.Source, error)
	Discover(ctx context.Context, env tuner.Env) ([]tuner.Found, error)
	Drivers(ctx context.Context, env tuner.Env) ([]tuner.Driver, error)
	SecretFields(ctx context.Context, driver string, env tuner.Env) ([]string, error)
}

// ErrNoEngine: nothing has been set to reach WittyTuner with.
var ErrNoEngine = errors.New("WittyTuner isn't on this server")

var engine Engine

// SetEngine says how to reach WittyTuner. It is set once, before the server starts serving.
func SetEngine(e Engine) { engine = e }

// Drivers lists the kinds of tuner.
func (m *Manager) Drivers(ctx context.Context) ([]tuner.Driver, error) {
	if engine == nil {
		return nil, ErrNoEngine
	}
	return engine.Drivers(ctx, m.Env())
}

// newSource makes a running source through the WittyTuner version in use, which is also
// what checks its settings.
func (m *Manager) newSource(ctx context.Context, driver string, cfg tuner.Config) (tuner.Source, error) {
	if engine == nil {
		return nil, ErrNoEngine
	}
	return engine.New(ctx, driver, cfg, m.Env())
}

// secretFields are the settings of a driver that are never shown again. If WittyTuner can't
// say, every setting counts as one, since showing a password is the worse mistake.
func secretFields(ctx context.Context, driver string, options map[string]string) []string {
	if engine != nil {
		if out, err := engine.SecretFields(ctx, driver, tuner.Env{}); err == nil {
			return out
		}
	}
	out := make([]string, 0, len(options))
	for k := range options {
		out = append(out, k)
	}
	return out
}

// Manager owns the running sources.
type Manager struct {
	// Tasks is where guide updates, channel scans and recordings show in the activity
	// panel. May be nil.
	Tasks *tasks.Registry
	DB    *db.DB
	Tools *ffmpeg.Tools
	Log   *slog.Logger

	internet *http.Client // what internetClient gives, for a test

	mu               sync.Mutex
	sources          map[int64]tuner.Source
	scans            map[int64]*ScanState
	inUse            map[int64]int
	viewers          map[int64]func() // live sessions by channel, for stopping one when a recording needs its tuner
	recordingCancels map[int64]context.CancelFunc
	health           map[int64]*HealthState // channel checks, by tuner
	dvrOnce          sync.Once
	epgOnce          sync.Once
	// Stop ends everything at shutdown.
	stopCtx context.Context
	stop    context.CancelFunc
	// Work the manager started on its own, which Close ends and waits for: see background.
	bgMu      sync.Mutex
	bgWork    sync.WaitGroup
	bgRunning map[string]int // by name, to say what Close couldn't wait for
	bgClosed  bool
	closeOnce sync.Once
	// AfterRecording is told when a recording has finished (to rescan a library).
	AfterRecording func(path string)
	// Folder answers where recordings go.
	Folder func() string
}

// ScanState is a scan under way, or the last one.
type ScanState struct {
	Running bool   `json:"running"`
	Percent int    `json:"percent"`
	Found   int    `json:"found"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Added   int    `json:"added"`
	// What the finished scan found.
	Clear     int `json:"clear"`
	Encrypted int `json:"encrypted"`
	Unknown   int `json:"unknown"`
	cancel    context.CancelFunc
}

// New makes a manager. Background work starts only when something needs it.
func New(database *db.DB, tools *ffmpeg.Tools, log *slog.Logger) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{DB: database, Tools: tools, Log: log.With("cat", "tv"), sources: map[int64]tuner.Source{}, scans: map[int64]*ScanState{},
		inUse: map[int64]int{}, viewers: map[int64]func(){}, stopCtx: ctx, stop: cancel}
}

// Start runs the guide refresher and the recording scheduler, if there is
// anything for them to do. Called at startup and whenever a tuner is added.
func (m *Manager) Start() {
	var n int
	_ = m.DB.QueryRow(`SELECT (SELECT COUNT(*) FROM tuners) + (SELECT COUNT(*) FROM epg_sources)`).Scan(&n)
	if n == 0 {
		return
	}
	m.epgOnce.Do(func() { m.loop("guide refresher", m.epgLoop, "cat", "tv") })
	m.dvrOnce.Do(func() {
		m.failInterrupted()
		m.loop("recording scheduler", m.dvrLoop, "cat", "tv")
	})
}

// tunerLog is for what a tuner did, as opposed to what the guide or a recording did.
func (m *Manager) tunerLog() *slog.Logger { return m.Log.With("module", "wittytuner") }

var addressInText = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"']+`)

// PublicError is what people may be told about a tuner or stream that failed. Connection errors say
// what's on the network, and hold the address, which can hold a login. Anything else that quotes
// an address, as a helper program's errors can, has it taken out.
func PublicError(err error) string {
	if err == nil {
		return ""
	}
	var ue *url.Error
	var op *net.OpError
	var dns *net.DNSError
	var pe *fs.PathError
	if errors.As(err, &ue) || errors.As(err, &op) || errors.As(err, &dns) || errors.Is(err, tuner.ErrUnreachable) {
		return "the stream could not be reached"
	}
	if errors.As(err, &pe) {
		return "the recording could not be written to its folder"
	}
	return addressInText.ReplaceAllString(err.Error(), "(address)")
}

// internetClient reaches the internet and nothing else, checking each connection as it is made.
func (m *Manager) internetClient() *http.Client {
	if m.internet != nil {
		return m.internet
	}
	return library.SafeClient
}

// Env is what drivers get.
func (m *Manager) Env() tuner.Env {
	env := tuner.Env{AllowCommands: os.Getenv("WMS_TUNER_COMMANDS") == "1"}
	if m.Tools != nil {
		env.FFmpeg = m.Tools.FFmpeg
	}
	return env
}

const tunerCols = `t.id, t.driver, t.name, t.options, t.enabled, t.max_streams, t.last_scan, t.last_error,
	(SELECT COUNT(*) FROM tuner_channels c WHERE c.tuner_id = t.id),
	(SELECT COUNT(*) FROM tuner_channels c WHERE c.tuner_id = t.id AND (c.encrypted = 1 OR c.encrypted_seen <> '')),
	(SELECT COUNT(*) FROM tuner_channels c WHERE c.tuner_id = t.id AND c.encrypted = 0 AND c.encrypted_seen = '' AND (c.checked = 1 OR c.seen_clear = 1))`

func scanTuner(sc interface{ Scan(...any) error }) (*Tuner, error) {
	var t Tuner
	var opts string
	var enabled int
	var last sql.NullInt64
	if err := sc.Scan(&t.ID, &t.Driver, &t.Name, &opts, &enabled, &t.MaxStreams, &last, &t.LastError, &t.Channels, &t.Encrypted, &t.Clear); err != nil {
		return nil, err
	}
	t.Enabled = enabled != 0
	if last.Valid {
		t.LastScan = &last.Int64
	}
	if err := json.Unmarshal([]byte(opts), &t.Options); err != nil || t.Options == nil {
		t.Options = map[string]string{}
	}
	return &t, nil
}

// Tuners lists every tuner.
func (m *Manager) Tuners(ctx context.Context) ([]*Tuner, error) {
	rows, err := m.DB.QueryContext(ctx, `SELECT `+tunerCols+` FROM tuners t ORDER BY t.name COLLATE NOCASE, t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Tuner{}
	for rows.Next() {
		t, err := scanTuner(rows)
		if err != nil {
			return nil, err
		}
		m.mu.Lock()
		t.InUse = m.inUse[t.ID]
		if src := m.sources[t.ID]; src != nil {
			t.Detail = src.Info().Detail
		}
		m.mu.Unlock()
		out = append(out, t)
	}
	return out, rows.Err()
}

// Tuner returns one tuner.
func (m *Manager) Tuner(ctx context.Context, id int64) (*Tuner, error) {
	t, err := scanTuner(m.DB.QueryRowContext(ctx, `SELECT `+tunerCols+` FROM tuners t WHERE t.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// ErrNotFound: no such tuner, channel or recording.
var ErrNotFound = errors.New("not found")

// Masked replaces passwords when options go to a browser.
const Masked = "••••••••"

// PublicOptions hides passwords.
func PublicOptions(t *Tuner) map[string]string {
	out := make(map[string]string, len(t.Options))
	for k, v := range t.Options {
		out[k] = v
	}
	for _, k := range secretFields(context.Background(), t.Driver, out) {
		if out[k] != "" {
			out[k] = Masked
		}
	}
	return out
}

// Create adds a tuner, checking its settings, and lists its channels. A
// channel list that cannot be read yet is not an error: the tuner is kept, with
// the reason, because the device may simply be off.
func (m *Manager) Create(ctx context.Context, t *Tuner) (*Tuner, error) {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return nil, errors.New("give the tuner a name")
	}
	if len(t.Name) > 200 {
		return nil, errors.New("the name is too long")
	}
	if _, err := m.newSource(ctx, t.Driver, tuner.Config{Name: t.Name, Options: cloneMap(t.Options)}); err != nil {
		return nil, err
	}
	opts, _ := json.Marshal(t.Options)
	res, err := m.DB.ExecContext(ctx, `INSERT INTO tuners(driver, name, options, max_streams, created_at) VALUES(?,?,?,?,?)`, t.Driver, t.Name, string(opts), max(t.MaxStreams, 0), db.Now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	m.Start()
	if err := m.Refresh(ctx, id); err != nil {
		m.Log.WarnContext(ctx, "tuner added, channels not read yet", "tuner", t.Name, "err", err)
	}
	return m.Tuner(ctx, id)
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// Update changes a tuner's settings. A masked password keeps the stored one.
func (m *Manager) Update(ctx context.Context, id int64, name string, options map[string]string, enabled bool, maxStreams int) (*Tuner, error) {
	t, err := m.Tuner(ctx, id)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("give the tuner a name")
	}
	if options != nil {
		for _, k := range secretFields(ctx, t.Driver, t.Options) {
			if options[k] == Masked {
				options[k] = t.Options[k]
			}
		}
	} else {
		options = t.Options
	}
	if _, err := m.newSource(ctx, t.Driver, tuner.Config{Name: name, Options: cloneMap(options)}); err != nil {
		return nil, err
	}
	opts, _ := json.Marshal(options)
	en := 0
	if enabled {
		en = 1
	}
	if _, err := m.DB.ExecContext(ctx, `UPDATE tuners SET name=?, options=?, enabled=?, max_streams=? WHERE id=?`, name, string(opts), en, max(maxStreams, 0), id); err != nil {
		return nil, err
	}
	m.forget(id)
	return m.Tuner(ctx, id)
}

// Delete removes a tuner, its channels and its guide source.
func (m *Manager) Delete(ctx context.Context, id int64) error {
	res, err := m.DB.ExecContext(ctx, `DELETE FROM tuners WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, _ = m.DB.ExecContext(ctx, `DELETE FROM epg_sources WHERE tuner_id = ?`, id)
	m.forget(id)
	m.unorphanBackups(ctx)
	return nil
}

func (m *Manager) forget(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sources[id]; s != nil {
		_ = s.Close()
		delete(m.sources, id)
	}
}

func (m *Manager) source(ctx context.Context, id int64) (tuner.Source, *Tuner, error) {
	t, err := m.Tuner(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sources[id]; s != nil {
		return s, t, nil
	}
	s, err := m.newSource(ctx, t.Driver, tuner.Config{Name: t.Name, Options: t.Options})
	if err != nil {
		return nil, t, err
	}
	m.sources[id] = s
	return s, t, nil
}

// Refresh re-reads a tuner's channels without scanning.
func (m *Manager) Refresh(ctx context.Context, id int64) error {
	s, _, err := m.source(ctx, id)
	if err != nil {
		return err
	}
	ch, err := s.Channels(ctx)
	if err != nil {
		m.setError(id, PublicError(err))
		return err
	}
	if ch != nil {
		if _, err := m.saveChannels(ctx, id, ch); err != nil {
			return err
		}
	}
	m.setError(id, "")
	m.ensureGuide(ctx, id, s)
	return nil
}

func (m *Manager) setError(id int64, msg string) {
	_, _ = m.DB.Exec(`UPDATE tuners SET last_error = ? WHERE id = ?`, msg, id)
}

// ensureGuide offers a source's own guide address as a guide source.
func (m *Manager) ensureGuide(ctx context.Context, id int64, s tuner.Source) {
	info := s.Info()
	if !webGuide(info.GuideURL) { // nor can a playlist choose a file on this machine, or a network share
		return
	}
	auth := ""
	if b, ok := s.(interface{ GuideHeaders() http.Header }); ok {
		if h := b.GuideHeaders(); len(h) > 0 {
			raw, _ := json.Marshal(h)
			auth = string(raw)
		}
	}
	_, _ = m.DB.ExecContext(ctx, `INSERT INTO epg_sources(name, url, auth, tuner_id) VALUES(?,?,?,?) ON CONFLICT(url) DO UPDATE SET auth = excluded.auth, tuner_id = excluded.tuner_id`,
		"From "+info.Name, info.GuideURL, auth, id)
	m.Start()
	m.background("guide refresh", m.refreshDue, "cat", "tv")
}

// saveChannels merges a channel list: what the owner chose stays, what has gone
// from the source goes. It returns how many are new.
func (m *Manager) saveChannels(ctx context.Context, id int64, list []tuner.Channel) (int, error) {
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var before int
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tuner_channels WHERE tuner_id = ?`, id).Scan(&before)
	keep := make(map[string]bool, len(list))
	for _, c := range list {
		if c.ID == "" || keep[c.ID] {
			continue
		}
		keep[c.ID] = true
		_, err := tx.ExecContext(ctx, `INSERT INTO tuner_channels(tuner_id, source_id, number, name, grp, callsign, locator, logo, hd, radio, encrypted, encrypted_why, program, epg_id, checked, atsc3, video_codec, audio_codec, no_record, catchup_days, catchup, catchup_source)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(tuner_id, source_id) DO UPDATE SET number=excluded.number, name=excluded.name, grp=excluded.grp, callsign=excluded.callsign, locator=excluded.locator,
			logo=excluded.logo, hd=excluded.hd, radio=excluded.radio, encrypted=excluded.encrypted, encrypted_why=excluded.encrypted_why, program=excluded.program, epg_id=excluded.epg_id,
			checked=excluded.checked, atsc3=excluded.atsc3, video_codec=excluded.video_codec, audio_codec=excluded.audio_codec, no_record=excluded.no_record,
			catchup_days=excluded.catchup_days, catchup=excluded.catchup, catchup_source=excluded.catchup_source`,
			id, c.ID, c.Number, c.Name, c.Group, c.CallSign, c.Locator, c.Logo, boolInt(c.HD), boolInt(c.Radio), boolInt(c.Encrypted != ""), c.Encrypted, c.Program, c.EPGID,
			boolInt(c.Checked), boolInt(c.ATSC3), c.VideoCodec, c.AudioCodec, c.NoRecord, max(c.CatchupDays, 0), c.Catchup, c.CatchupSource)
		if err != nil {
			return 0, err
		}
	}
	// Delete in batches: SQLite limits the arguments of one statement.
	rows, err := tx.QueryContext(ctx, `SELECT id, source_id FROM tuner_channels WHERE tuner_id = ?`, id)
	if err != nil {
		return 0, err
	}
	var gone []int64
	for rows.Next() {
		var cid int64
		var sid string
		if err := rows.Scan(&cid, &sid); err != nil {
			rows.Close()
			return 0, err
		}
		if !keep[sid] {
			gone = append(gone, cid)
		}
	}
	rows.Close()
	for _, cid := range gone {
		if _, err := tx.ExecContext(ctx, `DELETE FROM tuner_channels WHERE id = ?`, cid); err != nil {
			return 0, err
		}
	}
	var after int
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tuner_channels WHERE tuner_id = ?`, id).Scan(&after)
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if len(gone) > 0 {
		m.unorphanBackups(ctx)
	}
	return after - before + len(gone), nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// StartScan begins a scan in the background.
func (m *Manager) StartScan(id int64, source string) error {
	ctx, cancel := context.WithTimeout(m.stopCtx, 2*time.Hour)
	s, _, err := m.source(ctx, id)
	if err != nil {
		cancel()
		return err
	}
	m.mu.Lock()
	if cur := m.scans[id]; cur != nil && cur.Running {
		m.mu.Unlock()
		cancel()
		return errors.New("that tuner is already scanning")
	}
	if m.inUse[id] > 0 {
		m.mu.Unlock()
		cancel()
		return errors.New("that tuner is in use: stop watching or recording first")
	}
	st := &ScanState{Running: true, cancel: cancel}
	m.scans[id] = st
	m.inUse[id]++
	m.mu.Unlock()
	session := logging.NewID("tv")
	ctx = logging.WithSession(ctx, session)
	log := m.tunerLog().With("topic", "tuner", "tuner", id)
	log.InfoContext(ctx, "channel scan started")
	task := m.Tasks.Start(tasks.Spec{Type: tasks.TypeGuide, Title: "Finding channels on a tuner", Cancel: cancel, AdminOnly: true})
	// ctx, the scan's own, ends with the manager too.
	started := m.background("channel scan", func(context.Context) {
		defer cancel()
		ch, err := s.Scan(ctx, tuner.ScanOptions{Source: source}, func(p tuner.Progress) {
			m.mu.Lock()
			st.Percent, st.Found, st.Message = p.Percent, p.Found, p.Message
			m.mu.Unlock()
			task.Progress(int64(p.Percent), 100)
			task.Phase(p.Message)
		})
		defer func() { task.Finish(err) }()
		added := 0
		if err == nil && ch != nil {
			added, err = m.saveChannels(context.Background(), id, ch)
		}
		_, _ = m.DB.Exec(`UPDATE tuners SET last_scan = ? WHERE id = ?`, db.Now(), id)
		found, _ := m.Channels(context.Background(), id)
		m.mu.Lock()
		m.inUse[id]--
		st.Running, st.Added = false, added
		st.Clear, st.Encrypted, st.Unknown = 0, 0, 0
		for _, c := range found {
			switch c.Security {
			case SecurityClear:
				st.Clear++
			case SecurityEncrypted:
				st.Encrypted++
			default:
				st.Unknown++
			}
		}
		if err != nil {
			st.Error = PublicError(err)
		} else {
			st.Percent, st.Message = 100, ""
		}
		found2 := st.Found
		m.mu.Unlock()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				log.InfoContext(ctx, "channel scan cancelled")
			} else {
				log.WarnContext(ctx, "channel scan failed", "err", err)
			}
			m.setError(id, PublicError(err))
		} else {
			log.InfoContext(ctx, "channel scan finished", "found", found2, "added", added)
			m.setError(id, "")
			m.ensureGuide(context.Background(), id, s)
		}
	}, "cat", "tv", "session", session)
	if !started {
		cancel()
		m.mu.Lock()
		m.inUse[id]--
		st.Running, st.Error = false, errStopping.Error()
		m.mu.Unlock()
		task.Finish(errStopping)
		return errStopping
	}
	return nil
}

// ScanProgress reads a scan's state.
func (m *Manager) ScanProgress(id int64) ScanState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.scans[id]; s != nil {
		return *s
	}
	return ScanState{}
}

// --------------------------------------------------------------------------
// Channels
// --------------------------------------------------------------------------

const channelCols = `c.id, c.tuner_id, c.source_id, c.number, c.name, c.grp, c.callsign, c.locator, c.logo, c.hd, c.radio, c.encrypted, c.encrypted_why, c.program, c.enabled,
	c.number_set, c.name_set, c.epg_id, c.epg_set, t.name, t.driver, c.encrypted_seen, c.checked, c.seen_clear, c.atsc3, c.video_codec, c.audio_codec, c.no_record, c.no_record_set, c.logo_set,
	c.catchup_days, c.catchup, c.catchup_source, c.health, c.health_at, c.health_msg, c.merged_into`

// What is known about a channel's protection.
const (
	SecurityClear     = "clear"
	SecurityEncrypted = "encrypted"
	SecurityUnknown   = "unknown"
)

func scanChannel(sc interface{ Scan(...any) error }) (*Channel, error) {
	var c Channel
	var hd, radio, enc, enabled, checked, seenClear, atsc3, noRecordSet int
	var why, seen, noRecord string
	var merged sql.NullInt64
	if err := sc.Scan(&c.ID, &c.TunerID, &c.SourceID, &c.Number, &c.Name, &c.Group, &c.CallSign, &c.locator, &c.Logo, &hd, &radio, &enc, &why, &c.Program, &enabled,
		&c.NumberSet, &c.NameSet, &c.EPGID, &c.EPGSet, &c.sourceName, &c.driver, &seen, &checked, &seenClear, &atsc3, &c.VideoCodec, &c.AudioCodec, &noRecord, &noRecordSet, &c.LogoSet,
		&c.CatchupDays, &c.catchup, &c.catchupSource, &c.Health, &c.HealthAt, &c.HealthMsg, &merged); err != nil {
		return nil, err
	}
	c.HD, c.Radio, c.Enabled, c.ATSC3 = hd != 0, radio != 0, enabled != 0, atsc3 != 0
	c.MergedInto = merged.Int64
	switch {
	case enc != 0:
		c.Encrypted = firstNonEmpty(why, "it is marked as encrypted")
	case seen != "":
		c.Encrypted = seen
	}
	switch {
	case c.Encrypted != "":
		c.Security = SecurityEncrypted
	case checked != 0 || seenClear != 0:
		c.Security = SecurityClear
	default:
		c.Security = SecurityUnknown
	}
	c.NoRecord = noRecord
	if noRecordSet != 0 {
		c.NoRecord = firstNonEmpty(noRecord, "you marked it as not to be recorded, from what the device says")
	}
	if c.NumberSet != "" {
		c.Number = c.NumberSet
	}
	if c.NameSet != "" {
		c.Name = c.NameSet
	}
	return &c, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// GuideKey is the id that links a channel to guide data.
func (c *Channel) GuideKey() string { return firstNonEmpty(c.EPGSet, c.EPGID) }

// Channels lists a tuner's channels, for settings.
func (m *Manager) Channels(ctx context.Context, tunerID int64) ([]*Channel, error) {
	rows, err := m.DB.QueryContext(ctx, `SELECT `+channelCols+` FROM tuner_channels c JOIN tuners t ON t.id = c.tuner_id WHERE c.tuner_id = ?`, tunerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Channel{}
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if counts, err := m.backupCounts(ctx); err == nil {
		for _, c := range out {
			c.Backups = counts[c.ID]
		}
	}
	sortChannels(out)
	return out, nil
}

func sortChannels(l []*Channel) {
	sort.SliceStable(l, func(i, j int) bool {
		a, ea := strconv.ParseFloat(l[i].Number, 64)
		b, eb := strconv.ParseFloat(l[j].Number, 64)
		switch {
		case ea == nil && eb == nil:
			return a < b
		case ea == nil:
			return true
		case eb == nil:
			return false
		}
		return l[i].Number < l[j].Number
	})
}

// GuideChannels lists every channel that can appear in the guide, across
// enabled tuners. Encrypted channels stay, so people can see why they do not play.
func (m *Manager) GuideChannels(ctx context.Context) ([]*Channel, error) {
	rows, err := m.DB.QueryContext(ctx, `SELECT `+channelCols+` FROM tuner_channels c JOIN tuners t ON t.id = c.tuner_id WHERE c.enabled = 1 AND t.enabled = 1 AND c.merged_into IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Channel{}
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Channel returns one channel.
func (m *Manager) Channel(ctx context.Context, id int64) (*Channel, error) {
	c, err := scanChannel(m.DB.QueryRowContext(ctx, `SELECT `+channelCols+` FROM tuner_channels c JOIN tuners t ON t.id = c.tuner_id WHERE c.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// ChannelEdit is what an owner may change about a channel.
type ChannelEdit struct {
	Enabled *bool   `json:"enabled"`
	Number  *string `json:"number"`
	Name    *string `json:"name"`
	EPG     *string `json:"epg"`
	Logo    *string `json:"logo"` // see Channel.LogoSet
	// NoRecord: the device's documentation says this may not be recorded.
	NoRecord *bool `json:"noRecord"`
	// CheckAgain forgets what a stream showed, for a station that has changed.
	// A channel still encrypted is refused again the next time it is tuned.
	CheckAgain bool `json:"checkAgain"`
}

// EditChannel saves the owner's choices. They survive rescans.
func (m *Manager) EditChannel(ctx context.Context, id int64, e ChannelEdit) (*Channel, error) {
	if e.Number != nil {
		n := strings.TrimSpace(*e.Number)
		if len(n) > 12 || strings.Trim(n, "0123456789.-") != "" {
			return nil, errors.New("channel numbers look like 101 or 5.1")
		}
		if _, err := m.DB.ExecContext(ctx, `UPDATE tuner_channels SET number_set = ? WHERE id = ?`, n, id); err != nil {
			return nil, err
		}
	}
	if e.Name != nil {
		if len(*e.Name) > 200 {
			return nil, errors.New("the name is too long")
		}
		if _, err := m.DB.ExecContext(ctx, `UPDATE tuner_channels SET name_set = ? WHERE id = ?`, strings.TrimSpace(*e.Name), id); err != nil {
			return nil, err
		}
	}
	if e.Logo != nil {
		if _, err := m.DB.ExecContext(ctx, `UPDATE tuner_channels SET logo_set = ? WHERE id = ?`, *e.Logo, id); err != nil {
			return nil, err
		}
	}
	if e.EPG != nil {
		if len(*e.EPG) > 400 {
			return nil, errors.New("that guide id is too long")
		}
		if _, err := m.DB.ExecContext(ctx, `UPDATE tuner_channels SET epg_set = ? WHERE id = ?`, strings.TrimSpace(*e.EPG), id); err != nil {
			return nil, err
		}
	}
	if e.Enabled != nil {
		if _, err := m.DB.ExecContext(ctx, `UPDATE tuner_channels SET enabled = ? WHERE id = ?`, boolInt(*e.Enabled), id); err != nil {
			return nil, err
		}
	}
	if e.NoRecord != nil {
		if _, err := m.DB.ExecContext(ctx, `UPDATE tuner_channels SET no_record_set = ? WHERE id = ?`, boolInt(*e.NoRecord), id); err != nil {
			return nil, err
		}
	}
	if e.CheckAgain {
		if _, err := m.DB.ExecContext(ctx, `UPDATE tuner_channels SET encrypted_seen = '', seen_clear = 0 WHERE id = ?`, id); err != nil {
			return nil, err
		}
	}
	return m.Channel(ctx, id)
}

// SetAllEnabled turns every channel of a tuner on or off, for long playlists.
func (m *Manager) SetAllEnabled(ctx context.Context, tunerID int64, on bool) error {
	_, err := m.DB.ExecContext(ctx, `UPDATE tuner_channels SET enabled = ? WHERE tuner_id = ?`, boolInt(on), tunerID)
	return err
}

// --------------------------------------------------------------------------
// Opening streams
// --------------------------------------------------------------------------

// ErrBusy: every tuner of a source is in use.
var ErrBusy = tuner.ErrBusy

// Opened is a stream with the tuner slot it holds.
type Opened struct {
	Stream  *tuner.Stream
	Channel *Channel
	release func()
	note    func(encryptedWhy string)
	once    sync.Once
	// A stream that was clear on opening can turn out not to be: see watch.
	stopWatch  context.CancelFunc
	gate       *hlsGate // what FFmpeg reads a live stream through
	refused    chan struct{}
	refuseOnce sync.Once
	refusal    error
}

// Refused is closed when a stream that was playing was found to be encrypted. Whatever
// is using it stops, and Refusal says why. nil for a stream that can't be looked at again.
func (o *Opened) Refused() <-chan struct{} { return o.refused }

// Refusal is why the stream was refused, or nil.
func (o *Opened) Refusal() error {
	select {
	case <-o.refused:
		return o.refusal
	default:
		return nil
	}
}

func (o *Opened) refuse(why string) {
	o.refuseOnce.Do(func() {
		o.refusal = &tuner.EncryptedError{Channel: o.Channel.Name, Why: why}
		o.note(why)
		close(o.refused)
	})
}

// CheckNow looks at the stream again straight away, for when it has just stopped by
// itself, and says if it was refused.
func (o *Opened) CheckNow(ctx context.Context) error {
	if o.Stream.Recheck != nil && o.refused != nil {
		if why, _, err := o.Stream.Recheck(ctx); err == nil && why != "" {
			o.refuse(why)
		}
	}
	return o.Refusal()
}

// watch looks at a playing stream again for as long as it plays. Encrypted is final; not
// being able to find out says nothing, so a clear stream goes on through a network failure.
func (o *Opened) watch(ctx context.Context, log *slog.Logger) {
	for {
		why, next, err := o.Stream.Recheck(ctx)
		if ctx.Err() != nil {
			return
		}
		if why != "" {
			log.WarnContext(ctx, "a playing stream turned out to be encrypted; stopping it", "why", why)
			o.refuse(why)
			return
		}
		if err != nil {
			log.DebugContext(ctx, "could not look at a stream's playlist again", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(max(next, time.Millisecond)):
		}
	}
}

// Release frees the tuner, and remembers what the stream showed about itself.
func (o *Opened) Release() {
	o.once.Do(func() {
		if o.stopWatch != nil {
			o.stopWatch()
		}
		if g := o.Stream.Guard; g != nil {
			switch {
			case g.Fatal() != nil:
				o.note(tuner.ScrambledReason)
			case g.Verified():
				o.note("")
			}
		}
		_ = o.Stream.Close()
		o.gate.Close()
		o.release()
	})
}

// Open starts a channel, within the tuner's limit. Guide data found in the
// stream is stored as it goes by. When the channel won't open and has backups (the same
// station from other sources, see Merge), they're tried in turn, the ones that worked last
// time first; if none opens, the channel's own error is the answer.
func (m *Manager) Open(ctx context.Context, channelID int64) (*Opened, error) {
	o, err := m.openOne(ctx, channelID)
	if !worthFailingOver(ctx, err) {
		return o, err
	}
	backups, berr := m.backupsOf(ctx, channelID)
	if berr != nil {
		m.Log.WarnContext(ctx, "could not list a channel's backups", "cat", "tv", "channel", channelID, "err", berr)
	}
	for _, b := range backups {
		bo, berr := m.openOne(ctx, b)
		if berr == nil {
			m.tunerLog().InfoContext(ctx, "a channel didn't open, so its backup is playing", "topic", "tuner", "channel", channelID, "backup", b, "why", err)
			return bo, nil
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil, err
}

// openOne opens one channel, with no backups.
func (m *Manager) openOne(ctx context.Context, channelID int64) (*Opened, error) {
	return m.openWith(ctx, channelID, 0, 0)
}

// ErrNoArchive: what was asked for isn't in the source's archive.
var ErrNoArchive = errors.New("that isn't in this channel's archive")

// OpenArchive plays a stretch of what a channel showed, from its source's archive (catch-up),
// within the same limits and checks as watching it live. Backups aren't tried: another
// source's archive needn't hold the same programme at the same time.
func (m *Manager) OpenArchive(ctx context.Context, channelID, start, end int64) (*Opened, error) {
	c, err := m.Channel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	if c.CatchupDays <= 0 || start <= 0 || start > now || start < now-int64(c.CatchupDays)*86400 || (end != 0 && end <= start) {
		return nil, ErrNoArchive
	}
	return m.openWith(ctx, channelID, start, end)
}

func (m *Manager) openWith(ctx context.Context, channelID, archiveStart, archiveEnd int64) (*Opened, error) {
	c, err := m.Channel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if c.Encrypted != "" {
		return nil, &tuner.EncryptedError{Channel: c.Name, Why: c.Encrypted}
	}
	s, t, err := m.source(ctx, c.TunerID)
	if err != nil {
		return nil, err
	}
	if !t.Enabled {
		return nil, errors.New("that tuner is turned off")
	}
	// FFmpeg makes its own connections for anything but HTTP, and those can't be held to the
	// internet, so a list from the internet only gets to name web addresses.
	if !mayLeadHome(t) && !webGuide(c.locator) {
		return nil, errors.New("a playlist from the internet can only name web addresses. To use this channel, keep the playlist on your home network, or add it as a single stream")
	}
	limit := t.MaxStreams
	if limit == 0 {
		limit = s.Info().MaxStreams
	}
	m.mu.Lock()
	if limit > 0 && m.inUse[c.TunerID] >= limit {
		m.mu.Unlock()
		return nil, ErrBusy
	}
	m.inUse[c.TunerID]++
	m.mu.Unlock()
	release := func() {
		m.mu.Lock()
		m.inUse[c.TunerID]--
		m.mu.Unlock()
	}
	st, err := s.Open(ctx, tuner.Channel{ID: c.SourceID, Number: c.Number, Name: c.Name, Locator: c.locator, Program: c.Program,
		CatchupDays: c.CatchupDays, Catchup: c.catchup, CatchupSource: c.catchupSource, ArchiveStart: archiveStart, ArchiveEnd: archiveEnd})
	if err != nil {
		release()
		m.noteOpenFailure(ctx, c, err)
		if archiveStart == 0 {
			m.noteHealth(c.ID, err)
		}
		var enc *tuner.EncryptedError
		if errors.As(err, &enc) && enc.Why != "" {
			m.noteStream(c.ID, enc.Why)
		}
		return nil, err
	}
	if st.Guard != nil {
		st.Guard.OnEvent = m.broadcastEvents(c)
	}
	if c.Health != HealthOK && archiveStart == 0 {
		m.noteHealth(c.ID, nil)
	}
	m.Log.DebugContext(ctx, "channel opened", "module", "wittytuner", "topic", "tuner", "tuner", c.TunerID, "channel", c.Name)
	note := func(why string) { m.noteStream(c.ID, why) }
	if st.Verified {
		note("")
	}
	o := &Opened{Stream: st, Channel: c, release: release, note: note}
	home := mayLeadHome(t)
	addr, gated := gateInput(st)
	// What FFmpeg would read by itself, a DASH manifest being the one kind left, could lead it
	// anywhere. From a list on the internet, that isn't on offer.
	if !home && !gated && st.Reader == nil {
		_ = st.Close()
		release()
		return nil, errors.New("this channel is a kind of stream that can't be played from a playlist on the internet. To use it, keep the playlist on your home network, or add it as a single stream")
	}
	if gated {
		g, err := newHLSGate(addr, st.Input, home, m.tunerLog(), o.refuse)
		if err != nil {
			_ = st.Close()
			release()
			return nil, err
		}
		if !home {
			g.client = m.internetClient()
		}
		o.gate, o.refused = g, make(chan struct{})
		st.Input = g.args(st.Input)
	}
	if st.Recheck != nil {
		wctx, cancel := context.WithCancel(ctx)
		o.stopWatch = cancel
		log := m.tunerLog().With("topic", "tuner", "tuner", c.TunerID, "channel", c.Name)
		// It ends with the stream, or with the manager.
		m.background("stream re-check", func(stop context.Context) {
			defer context.AfterFunc(stop, cancel)()
			o.watch(wctx, log)
		}, "cat", "tv")
	}
	return o, nil
}

// noteOpenFailure leaves a record of a tuner that wouldn't give a channel.
// Being busy, or the caller leaving, isn't a fault; a refusal is only worth a note.
func (m *Manager) noteOpenFailure(ctx context.Context, c *Channel, err error) {
	log := m.tunerLog().With("topic", "tuner", "tuner", c.TunerID, "channel", c.Name)
	switch {
	case ctx.Err() != nil, errors.Is(err, ErrBusy):
		log.DebugContext(ctx, "channel not opened", "err", err)
	case tuner.IsEncrypted(err):
		log.InfoContext(ctx, "channel refused: encrypted", "err", err)
	default:
		log.WarnContext(ctx, "could not open a tuner channel", "err", err)
	}
}

// noteStream keeps what a stream showed, so the guide and settings are right
// from then on: why it is encrypted, or "" for clear.
func (m *Manager) noteStream(id int64, why string) {
	var err error
	if why != "" {
		_, err = m.DB.Exec(`UPDATE tuner_channels SET encrypted_seen = ?, seen_clear = 0 WHERE id = ?`, why, id)
	} else {
		_, err = m.DB.Exec(`UPDATE tuner_channels SET seen_clear = 1, encrypted_seen = '' WHERE id = ?`, id)
	}
	if err != nil {
		m.Log.Warn("could not save what a stream showed", "channel", id, "err", err)
	}
	if why != "" {
		m.Log.Info("channel marked as encrypted", "module", "wittytuner", "topic", "tuner", "channel", id, "why", why)
	}
}

// Alternative is a clear way to watch a station that is encrypted here.
type Alternative struct {
	Key    string `json:"key"`
	Number string `json:"number"`
	Name   string `json:"name"`
	ATSC3  bool   `json:"atsc3,omitempty"`
	// CanRecord: whether the profile asking may record it. The server fills it in.
	CanRecord bool `json:"canRecord"`
}

var stationSuffix = []string{"atsc30", "atsc3", "nextgen", "nextgentv", "ngtv", "4k", "hd", "dt"}

// stationKey lets WTST, WTST-HD and "WTST ATSC3" count as one station.
func stationKey(name string) string {
	k := normalizeName(name)
	for changed := true; changed; {
		changed = false
		for _, s := range stationSuffix {
			if len(k) > len(s)+1 && strings.HasSuffix(k, s) {
				k, changed = strings.TrimSuffix(k, s), true
			}
		}
	}
	return k
}

// overTheAir: a virtual channel number means the same station on these.
func overTheAir(driver string) bool { return driver == "network" || driver == "dvb" }

// Alternatives finds, for each channel that needs one, the same station from a
// clear source: an ATSC 1.0 version, or another tuner or playlist. It matches
// by call sign and name, and by number over the air, where a number names a
// station. It never offers a channel not known to be watchable. need says which
// channels want one, besides the encrypted: those that would play without sound.
func Alternatives(list []*Channel, need func(*Channel) bool) map[int64]*Alternative {
	var byName, byNumber = map[string][]*Channel{}, map[string][]*Channel{}
	for _, c := range list {
		if c.Security == SecurityEncrypted || !c.Enabled || (need != nil && need(c)) {
			continue
		}
		if k := stationKey(firstNonEmpty(c.CallSign, c.Name)); k != "" {
			byName[k] = append(byName[k], c)
		}
		if overTheAir(c.driver) && c.Number != "" {
			byNumber[c.Number] = append(byNumber[c.Number], c)
		}
	}
	best := func(cands []*Channel, me *Channel) *Channel {
		var pick *Channel
		for _, d := range cands {
			if d.ID == me.ID {
				continue
			}
			// Sound and picture every client can have comes first.
			if pick == nil || (pick.ATSC3 && !d.ATSC3) {
				pick = d
			}
		}
		return pick
	}
	out := map[int64]*Alternative{}
	for _, c := range list {
		if c.Security != SecurityEncrypted && (need == nil || !need(c)) {
			continue
		}
		d := best(byName[stationKey(firstNonEmpty(c.CallSign, c.Name))], c)
		if d == nil && overTheAir(c.driver) {
			d = best(byNumber[c.Number], c)
		}
		if d != nil {
			out[c.ID] = &Alternative{Key: "tun-" + strconv.FormatInt(d.ID, 10), Number: d.Number, Name: d.Name, ATSC3: d.ATSC3}
		}
	}
	return out
}

// TunerBusy says whether a tuner is at its limit.
func (m *Manager) TunerBusy(ctx context.Context, tunerID int64) bool {
	s, t, err := m.source(ctx, tunerID)
	if err != nil {
		return false
	}
	limit := t.MaxStreams
	if limit == 0 {
		limit = s.Info().MaxStreams
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return limit > 0 && m.inUse[tunerID] >= limit
}

// RegisterViewer records how to stop the live stream on a channel, so that a
// recording can take its tuner.
func (m *Manager) RegisterViewer(channelID int64, stop func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.viewers[channelID] = stop
}

// ForgetViewer undoes RegisterViewer.
func (m *Manager) ForgetViewer(channelID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.viewers, channelID)
}

// freeTuner stops a live viewer on a tuner, because a recording needs it.
func (m *Manager) freeTuner(ctx context.Context, tunerID int64) bool {
	m.mu.Lock()
	var stop func()
	var who int64
	for cid, f := range m.viewers {
		var t int64
		if m.DB.QueryRowContext(ctx, `SELECT tuner_id FROM tuner_channels WHERE id = ?`, cid).Scan(&t) == nil && t == tunerID {
			stop, who = f, cid
			break
		}
	}
	if stop != nil {
		delete(m.viewers, who)
	}
	m.mu.Unlock()
	if stop == nil {
		return false
	}
	stop()
	return true
}

// Discover looks for tuners, and leaves out those already added.
func (m *Manager) Discover(ctx context.Context) ([]tuner.Found, error) {
	if engine == nil {
		return nil, ErrNoEngine
	}
	found, err := engine.Discover(ctx, m.Env())
	if err != nil {
		return nil, err
	}
	tuners, _ := m.Tuners(ctx)
	out := []tuner.Found{}
	for _, f := range found {
		dup := false
		for _, t := range tuners {
			same := t.Driver == f.Driver
			for k, v := range f.Options {
				same = same && t.Options[k] == v
			}
			dup = dup || same
		}
		if !dup {
			out = append(out, f)
		}
	}
	return out, nil
}

// Describe gives a short account of a tuner, for logs.
func (t *Tuner) Describe() string { return fmt.Sprintf("%s (%s)", t.Name, t.Driver) }
