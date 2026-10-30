package livetv

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittytuner/mpegts"
	"github.com/wittywilla/wittytuner/xmltv"
)

// Guide data: XMLTV sources fetched on a schedule, and what broadcasts say
// about themselves while a channel is on. Only the coming two weeks are kept,
// and only for channels on the server, so a guide file of a whole country does
// not fill the database.

// EPGSource is a place guide data comes from.
type EPGSource struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	LastFetch  *int64 `json:"lastFetch"`
	LastError  string `json:"lastError"`
	Programmes int    `json:"programmes"`
	TunerID    *int64 `json:"tunerId,omitempty"`
}

// Programme is one guide entry.
type Programme struct {
	Start    int64  `json:"start"`
	End      int64  `json:"end"`
	Title    string `json:"title"`
	Sub      string `json:"sub,omitempty"`
	Desc     string `json:"desc,omitempty"`
	Category string `json:"category,omitempty"`
	Rating   string `json:"rating,omitempty"`
	Season   *int   `json:"season,omitempty"`
	Episode  *int   `json:"episode,omitempty"`
}

const (
	epgRefresh  = 12 * time.Hour
	epgKeepDays = 14
)

// Sources lists guide sources.
func (m *Manager) Sources(ctx context.Context) ([]EPGSource, error) {
	rows, err := m.DB.QueryContext(ctx, `SELECT id, name, url, last_fetch, last_error, programmes, tuner_id FROM epg_sources ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EPGSource{}
	for rows.Next() {
		var s EPGSource
		var last, tuner sql.NullInt64
		if err := rows.Scan(&s.ID, &s.Name, &s.URL, &last, &s.LastError, &s.Programmes, &tuner); err != nil {
			return nil, err
		}
		if last.Valid {
			s.LastFetch = &last.Int64
		}
		if tuner.Valid {
			s.TunerID = &tuner.Int64
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func checkEPGPlace(place string) error {
	if strings.Contains(place, "://") {
		u, err := url.Parse(place)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("guide data comes from an http(s) address or a file on the server")
		}
		return nil
	}
	if st, err := os.Stat(place); err != nil || st.IsDir() {
		return errors.New("that guide file was not found on the server")
	}
	return nil
}

// AddSource adds a guide source and fetches it.
func (m *Manager) AddSource(ctx context.Context, name, place string) (*EPGSource, error) {
	place = strings.TrimSpace(place)
	name = strings.TrimSpace(name)
	if err := checkEPGPlace(place); err != nil {
		return nil, err
	}
	if name == "" {
		name = place
	}
	if len(name) > 200 || len(place) > 2000 {
		return nil, errors.New("that is too long")
	}
	res, err := m.DB.ExecContext(ctx, `INSERT INTO epg_sources(name, url) VALUES(?, ?)`, name, place)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, errors.New("that guide source is already there")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	m.Start()
	go func() {
		if err := m.FetchSource(context.WithoutCancel(ctx), id); err != nil {
			m.Log.Info("guide source", "cat", "tv", "err", err)
		}
	}()
	return &EPGSource{ID: id, Name: name, URL: place}, nil
}

// DeleteSource removes a source.
func (m *Manager) DeleteSource(ctx context.Context, id int64) error {
	res, err := m.DB.ExecContext(ctx, `DELETE FROM epg_sources WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *Manager) epgLoop() {
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for {
		m.refreshDue(m.stopCtx)
		m.pruneGuide(m.stopCtx)
		select {
		case <-m.stopCtx.Done():
			return
		case <-t.C:
		}
	}
}

// refreshDue fetches sources not fetched for a while.
func (m *Manager) refreshDue(ctx context.Context) {
	list, err := m.Sources(ctx)
	if err != nil {
		return
	}
	for _, s := range list {
		if s.LastFetch != nil && time.Since(time.Unix(*s.LastFetch, 0)) < epgRefresh {
			continue
		}
		if err := m.FetchSource(ctx, s.ID); err != nil {
			m.Log.Info("guide source", "cat", "tv", "source", s.Name, "err", err)
		}
	}
}

func (m *Manager) pruneGuide(ctx context.Context) {
	_, _ = m.DB.ExecContext(ctx, `DELETE FROM epg_programmes WHERE stop < ?`, time.Now().Add(-24*time.Hour).Unix())
}

func (m *Manager) openGuide(ctx context.Context, place, auth string) (io.ReadCloser, error) {
	if !strings.Contains(place, "://") {
		return os.Open(place)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", place, nil)
	if err != nil {
		return nil, err
	}
	if auth != "" {
		var h http.Header
		if json.Unmarshal([]byte(auth), &h) == nil {
			for k, v := range h {
				req.Header[k] = v
			}
		}
	}
	req.Header.Set("User-Agent", "WittyMediaServer")
	res, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		return nil, fmt.Errorf("the guide source answered %s", res.Status)
	}
	return res.Body, nil
}

// FetchSource reads one source now, storing only programmes of channels on
// this server.
func (m *Manager) FetchSource(ctx context.Context, id int64) error {
	var place, auth string
	if err := m.DB.QueryRowContext(ctx, `SELECT url, auth FROM epg_sources WHERE id = ?`, id).Scan(&place, &auth); err != nil {
		return ErrNotFound
	}
	if err := checkEPGPlace(place); err != nil {
		_, _ = m.DB.ExecContext(ctx, `UPDATE epg_sources SET last_error = ?, last_fetch = ? WHERE id = ?`, err.Error(), db.Now(), id)
		return err
	}
	n, err := m.fetch(ctx, id, place, auth)
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	_, _ = m.DB.ExecContext(ctx, `UPDATE epg_sources SET last_error = ?, last_fetch = ?, programmes = ? WHERE id = ?`, msg, db.Now(), n, id)
	return err
}

func normalizeName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSuffix(b.String(), "hd")
}

func (m *Manager) fetch(ctx context.Context, id int64, place, auth string) (int, error) {
	r, err := m.openGuide(ctx, place, auth)
	if err != nil {
		return 0, err
	}
	defer r.Close()
	// What the server has: guide ids, and names to match on when there are none.
	wanted := map[string]bool{}
	byName := map[string]int64{}
	rows, err := m.DB.QueryContext(ctx, `SELECT c.id, c.epg_id, c.epg_set, c.name, c.name_set FROM tuner_channels c WHERE c.enabled = 1`)
	if err != nil {
		return 0, err
	}
	type row struct {
		id           int64
		key, set, nm string
	}
	for rows.Next() {
		var c row
		var nameSet string
		if rows.Scan(&c.id, &c.key, &c.set, &c.nm, &nameSet) != nil {
			continue
		}
		if k := firstNonEmpty(c.set, c.key); k != "" {
			wanted[k] = true
		} else {
			byName[normalizeName(firstNonEmpty(nameSet, c.nm))] = c.id
		}
	}
	rows.Close()
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM epg_channels WHERE source_id = ?`, id); err != nil {
		return 0, err
	}
	matched := map[int64]string{}
	addChannel, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO epg_channels(source_id, epg_id, name) VALUES(?,?,?)`)
	if err != nil {
		return 0, err
	}
	defer addChannel.Close()
	addProg, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO epg_programmes(channel, start, stop, title, sub, descr, category, rating, season, episode, source)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return 0, err
	}
	defer addProg.Close()
	now := time.Now()
	from, to := now.Add(-12*time.Hour), now.AddDate(0, 0, epgKeepDays)
	src := fmt.Sprintf("xmltv:%d", id)
	if _, err := tx.ExecContext(ctx, `DELETE FROM epg_programmes WHERE source = ?`, src); err != nil {
		return 0, err
	}
	count := 0
	open := map[string]*xmltv.Programme{}
	save := func(p *xmltv.Programme) error {
		if p.Stop.IsZero() || !p.Stop.After(p.Start) || p.Stop.Before(from) || p.Start.After(to) {
			return nil
		}
		_, err := addProg.ExecContext(ctx, p.Channel, p.Start.Unix(), p.Stop.Unix(), clip(p.Title, 300), clip(p.Sub, 300), clip(p.Desc, 2000), clip(p.Category, 80), clip(p.Rating, 30), p.Season, p.Episode, src)
		if err == nil {
			count++
		}
		return err
	}
	var failed error
	err = xmltv.Parse(r, func(c xmltv.Channel) bool {
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
		}
		_, _ = addChannel.ExecContext(ctx, id, c.ID, clip(name, 200))
		for _, n := range c.Names {
			if cid, ok := byName[normalizeName(n)]; ok && matched[cid] == "" {
				matched[cid] = c.ID
				wanted[c.ID] = true
			}
		}
		return ctx.Err() == nil
	}, func(p xmltv.Programme) bool {
		if !wanted[p.Channel] {
			return ctx.Err() == nil
		}
		// A programme without an end time ends when the next begins.
		if prev := open[p.Channel]; prev != nil && prev.Stop.IsZero() {
			prev.Stop = p.Start
			if failed = save(prev); failed != nil {
				return false
			}
		}
		cur := p
		if !cur.Stop.IsZero() {
			if failed = save(&cur); failed != nil {
				return false
			}
		}
		open[p.Channel] = &cur
		return ctx.Err() == nil
	})
	if err == nil {
		err = failed
	}
	if err != nil {
		return 0, fmt.Errorf("the guide could not be read: %w", err)
	}
	for cid, key := range matched {
		if _, err := tx.ExecContext(ctx, `UPDATE tuner_channels SET epg_id = ? WHERE id = ? AND epg_id = '' AND epg_set = ''`, key, cid); err != nil {
			return 0, err
		}
	}
	return count, tx.Commit()
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && s[cut]&0xc0 == 0x80 {
		cut--
	}
	return s[:cut]
}

// broadcastKey is the guide key of data from a broadcast itself.
func broadcastKey(c *Channel) string { return fmt.Sprintf("tuner:%d", c.ID) }

// broadcastEvents stores guide data seen in a stream. Events name a service;
// only this channel's own are kept, because the other services on a multiplex
// would need their own channel ids to mean anything.
func (m *Manager) broadcastEvents(c *Channel) func(mpegts.Event) {
	key := broadcastKey(c)
	var last time.Time
	var buf []mpegts.Event
	return func(e mpegts.Event) {
		if c.Program != 0 && e.ServiceID != c.Program && !e.ATSC {
			return
		}
		buf = append(buf, e)
		if len(buf) < 32 && time.Since(last) < 30*time.Second {
			return
		}
		list := buf
		buf, last = nil, time.Now()
		go m.storeBroadcast(key, list)
	}
}

func (m *Manager) storeBroadcast(key string, list []mpegts.Event) {
	ctx := m.stopCtx
	for _, e := range list {
		stop := e.Start.Add(e.Duration)
		if !stop.After(time.Now()) || e.Start.After(time.Now().AddDate(0, 0, epgKeepDays)) {
			continue
		}
		_, _ = m.DB.ExecContext(ctx, `INSERT INTO epg_programmes(channel, start, stop, title, descr, rating, source) VALUES(?,?,?,?,?,?,'broadcast')
			ON CONFLICT(channel, start) DO UPDATE SET stop = excluded.stop, title = excluded.title, descr = CASE WHEN excluded.descr <> '' THEN excluded.descr ELSE descr END, rating = excluded.rating`,
			key, e.Start.Unix(), stop.Unix(), clip(e.Title, 300), clip(e.Desc, 2000), clip(e.Rating, 30))
	}
}

// Guide lists the programmes of a channel in a window, from XMLTV and the
// broadcast, with the XMLTV source winning where both speak.
func (m *Manager) Guide(ctx context.Context, c *Channel, from, to time.Time) ([]Programme, error) {
	keys := []any{broadcastKey(c)}
	if k := c.GuideKey(); k != "" {
		keys = append(keys, k)
	}
	q := `SELECT channel, start, stop, title, sub, descr, category, rating, season, episode FROM epg_programmes
		WHERE channel IN (` + strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",") + `) AND stop > ? AND start < ? ORDER BY start`
	rows, err := m.DB.QueryContext(ctx, q, append(keys, from.Unix(), to.Unix())...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Programme{}
	bKey := broadcastKey(c)
	for rows.Next() {
		var p Programme
		var ch string
		var season, episode sql.NullInt64
		if err := rows.Scan(&ch, &p.Start, &p.End, &p.Title, &p.Sub, &p.Desc, &p.Category, &p.Rating, &season, &episode); err != nil {
			return nil, err
		}
		if season.Valid {
			v := int(season.Int64)
			p.Season = &v
		}
		if episode.Valid {
			v := int(episode.Int64)
			p.Episode = &v
		}
		// Overlaps: the stored guide wins over the broadcast's own data.
		if n := len(out); n > 0 && p.Start < out[n-1].End {
			if ch == bKey {
				continue
			}
			out[n-1] = p
			continue
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GuideIDs lists XMLTV channel ids, for mapping channels by hand.
func (m *Manager) GuideIDs(ctx context.Context) ([]map[string]string, error) {
	rows, err := m.DB.QueryContext(ctx, `SELECT DISTINCT epg_id, name FROM epg_channels ORDER BY name COLLATE NOCASE LIMIT 20000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out = append(out, map[string]string{"id": id, "name": name})
	}
	return out, rows.Err()
}
