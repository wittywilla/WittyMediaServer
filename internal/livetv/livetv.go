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
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittytuner"
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
	InUse      int               `json:"inUse"`
}

// Channel is a tuner's channel as the guide and the settings see it. Number
// and Name are what the viewer sees: the owner's choice, else the source's.
type Channel struct {
	ID         int64  `json:"id"`
	TunerID    int64  `json:"tunerId"`
	SourceID   string `json:"sourceId"`
	Number     string `json:"number"`
	Name       string `json:"name"`
	Group      string `json:"group,omitempty"`
	CallSign   string `json:"callsign,omitempty"`
	Logo       string `json:"logo,omitempty"`
	HD         bool   `json:"hd,omitempty"`
	Radio      bool   `json:"radio,omitempty"`
	Encrypted  string `json:"encrypted,omitempty"`
	Program    int    `json:"program,omitempty"`
	Enabled    bool   `json:"enabled"`
	EPGID      string `json:"epgId,omitempty"`
	NumberSet  string `json:"numberSet,omitempty"`
	NameSet    string `json:"nameSet,omitempty"`
	EPGSet     string `json:"epgSet,omitempty"`
	locator    string
	sourceName string
}

// Manager owns the running sources.
type Manager struct {
	DB    *db.DB
	Tools *ffmpeg.Tools
	Log   *slog.Logger

	mu               sync.Mutex
	sources          map[int64]wittytuner.Source
	scans            map[int64]*ScanState
	inUse            map[int64]int
	viewers          map[int64]func() // live sessions by channel, for stopping one when a recording needs its tuner
	recordingCancels map[int64]context.CancelFunc
	dvrOnce          sync.Once
	epgOnce          sync.Once
	// Stop ends everything at shutdown.
	stopCtx context.Context
	stop    context.CancelFunc
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
	cancel  context.CancelFunc
}

// New makes a manager. Background work starts only when something needs it.
func New(database *db.DB, tools *ffmpeg.Tools, log *slog.Logger) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{DB: database, Tools: tools, Log: log, sources: map[int64]wittytuner.Source{}, scans: map[int64]*ScanState{},
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
	m.epgOnce.Do(func() { go m.epgLoop() })
	m.dvrOnce.Do(func() { go m.dvrLoop() })
}

// Close stops background work and releases sources.
func (m *Manager) Close() {
	m.stop()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sources {
		_ = s.Close()
	}
	m.sources = map[int64]wittytuner.Source{}
}

// Env is what drivers get.
func (m *Manager) Env() wittytuner.Env {
	env := wittytuner.Env{
		HTTP:          &http.Client{Timeout: 30 * time.Second},
		Stream:        &http.Client{},
		AllowCommands: os.Getenv("WMS_TUNER_COMMANDS") == "1",
		Command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return ffmpeg.Command(ctx, name, args...)
		},
	}
	if m.Tools != nil {
		env.FFmpeg = m.Tools.FFmpeg
	}
	return env
}

const tunerCols = `t.id, t.driver, t.name, t.options, t.enabled, t.max_streams, t.last_scan, t.last_error,
	(SELECT COUNT(*) FROM tuner_channels c WHERE c.tuner_id = t.id)`

func scanTuner(sc interface{ Scan(...any) error }) (*Tuner, error) {
	var t Tuner
	var opts string
	var enabled int
	var last sql.NullInt64
	if err := sc.Scan(&t.ID, &t.Driver, &t.Name, &opts, &enabled, &t.MaxStreams, &last, &t.LastError, &t.Channels); err != nil {
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
	for _, k := range wittytuner.SecretFields(t.Driver) {
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
	if _, err := wittytuner.New(t.Driver, wittytuner.Config{Name: t.Name, Options: cloneMap(t.Options)}, m.Env()); err != nil {
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
		m.Log.Info("tuner added, channels not read yet", "cat", "tv", "tuner", t.Name, "err", err)
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
		for _, k := range wittytuner.SecretFields(t.Driver) {
			if options[k] == Masked {
				options[k] = t.Options[k]
			}
		}
	} else {
		options = t.Options
	}
	if _, err := wittytuner.New(t.Driver, wittytuner.Config{Name: name, Options: cloneMap(options)}, m.Env()); err != nil {
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

func (m *Manager) source(ctx context.Context, id int64) (wittytuner.Source, *Tuner, error) {
	t, err := m.Tuner(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sources[id]; s != nil {
		return s, t, nil
	}
	s, err := wittytuner.New(t.Driver, wittytuner.Config{Name: t.Name, Options: t.Options}, m.Env())
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
		m.setError(id, err.Error())
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
func (m *Manager) ensureGuide(ctx context.Context, id int64, s wittytuner.Source) {
	info := s.Info()
	if info.GuideURL == "" {
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
	go m.refreshDue(context.WithoutCancel(ctx))
}

// saveChannels merges a channel list: what the owner chose stays, what has gone
// from the source goes. It returns how many are new.
func (m *Manager) saveChannels(ctx context.Context, id int64, list []wittytuner.Channel) (int, error) {
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
		_, err := tx.ExecContext(ctx, `INSERT INTO tuner_channels(tuner_id, source_id, number, name, grp, callsign, locator, logo, hd, radio, encrypted, encrypted_why, program, epg_id)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(tuner_id, source_id) DO UPDATE SET number=excluded.number, name=excluded.name, grp=excluded.grp, callsign=excluded.callsign, locator=excluded.locator,
			logo=excluded.logo, hd=excluded.hd, radio=excluded.radio, encrypted=excluded.encrypted, encrypted_why=excluded.encrypted_why, program=excluded.program, epg_id=excluded.epg_id`,
			id, c.ID, c.Number, c.Name, c.Group, c.CallSign, c.Locator, c.Logo, boolInt(c.HD), boolInt(c.Radio), boolInt(c.Encrypted != ""), c.Encrypted, c.Program, c.EPGID)
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
	return after - before + len(gone), tx.Commit()
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
	go func() {
		defer cancel()
		ch, err := s.Scan(ctx, wittytuner.ScanOptions{Source: source}, func(p wittytuner.Progress) {
			m.mu.Lock()
			st.Percent, st.Found, st.Message = p.Percent, p.Found, p.Message
			m.mu.Unlock()
		})
		added := 0
		if err == nil && ch != nil {
			added, err = m.saveChannels(context.Background(), id, ch)
		}
		_, _ = m.DB.Exec(`UPDATE tuners SET last_scan = ? WHERE id = ?`, db.Now(), id)
		m.mu.Lock()
		m.inUse[id]--
		st.Running, st.Added = false, added
		if err != nil {
			st.Error = err.Error()
		} else {
			st.Percent, st.Message = 100, ""
		}
		m.mu.Unlock()
		if err != nil {
			m.setError(id, err.Error())
		} else {
			m.setError(id, "")
			m.ensureGuide(context.Background(), id, s)
		}
	}()
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
	c.number_set, c.name_set, c.epg_id, c.epg_set, t.name`

func scanChannel(sc interface{ Scan(...any) error }) (*Channel, error) {
	var c Channel
	var hd, radio, enc, enabled int
	var why string
	if err := sc.Scan(&c.ID, &c.TunerID, &c.SourceID, &c.Number, &c.Name, &c.Group, &c.CallSign, &c.locator, &c.Logo, &hd, &radio, &enc, &why, &c.Program, &enabled,
		&c.NumberSet, &c.NameSet, &c.EPGID, &c.EPGSet, &c.sourceName); err != nil {
		return nil, err
	}
	c.HD, c.Radio, c.Enabled = hd != 0, radio != 0, enabled != 0
	if enc != 0 {
		c.Encrypted = firstNonEmpty(why, "it is marked as encrypted")
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
	sortChannels(out)
	return out, rows.Err()
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
	rows, err := m.DB.QueryContext(ctx, `SELECT `+channelCols+` FROM tuner_channels c JOIN tuners t ON t.id = c.tuner_id WHERE c.enabled = 1 AND t.enabled = 1`)
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
var ErrBusy = wittytuner.ErrBusy

// Opened is a stream with the tuner slot it holds.
type Opened struct {
	Stream  *wittytuner.Stream
	Channel *Channel
	release func()
	once    sync.Once
}

// Release frees the tuner.
func (o *Opened) Release() {
	o.once.Do(func() {
		_ = o.Stream.Close()
		o.release()
	})
}

// Open starts a channel, within the tuner's limit. Guide data found in the
// stream is stored as it goes by.
func (m *Manager) Open(ctx context.Context, channelID int64) (*Opened, error) {
	c, err := m.Channel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if c.Encrypted != "" {
		return nil, &wittytuner.EncryptedError{Channel: c.Name, Why: c.Encrypted}
	}
	s, t, err := m.source(ctx, c.TunerID)
	if err != nil {
		return nil, err
	}
	if !t.Enabled {
		return nil, errors.New("that tuner is turned off")
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
	st, err := s.Open(ctx, wittytuner.Channel{ID: c.SourceID, Number: c.Number, Name: c.Name, Locator: c.locator, Program: c.Program})
	if err != nil {
		release()
		return nil, err
	}
	if st.Guard != nil {
		st.Guard.OnEvent = m.broadcastEvents(c)
	}
	return &Opened{Stream: st, Channel: c, release: release}, nil
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
func (m *Manager) Discover(ctx context.Context) []wittytuner.Found {
	found := wittytuner.Discover(ctx, m.Env())
	tuners, _ := m.Tuners(ctx)
	out := []wittytuner.Found{}
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
	return out
}

// Describe gives a short account of a tuner, for logs.
func (t *Tuner) Describe() string { return fmt.Sprintf("%s (%s)", t.Name, t.Driver) }
