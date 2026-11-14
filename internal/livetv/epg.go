// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/tuner/mpegts"
	"github.com/wittywilla/wittymediaserver/internal/tuner/xmltv"
)

// Guide data: XMLTV guides WittyTuner fetches from its sources on a schedule and hands over
// (Ingest), and what broadcasts say about themselves while a channel is on. Only the coming two weeks are kept,
// and only for channels on the server, so a guide file of a whole country does
// not fill the database.

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

const epgKeepDays = 14

func (m *Manager) epgLoop(ctx context.Context) {
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for {
		m.pruneGuide(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (m *Manager) pruneGuide(ctx context.Context) {
	_, _ = m.DB.ExecContext(ctx, `DELETE FROM epg_programmes WHERE stop < ?`, time.Now().Add(-24*time.Hour).Unix())
}

// DropSource forgets what a guide source WittyTuner removed gave: its channel ids and programmes.
func (m *Manager) DropSource(ctx context.Context, id int64) error {
	if _, err := m.DB.ExecContext(ctx, `DELETE FROM epg_channels WHERE source_id = ?`, id); err != nil {
		return err
	}
	_, err := m.DB.ExecContext(ctx, `DELETE FROM epg_programmes WHERE source IN (?, ?)`, fmt.Sprintf("xmltv:%d", id), fmt.Sprintf("xmltv:%d:coming", id))
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

// How much one source may take, and how long it may take over it. Nothing here holds the database
// while waiting for the source: a slow host must not stop every other write on the server.
var (
	maxGuideChannels   = 100_000
	maxGuideProgrammes = 2_000_000
	guideIdle          = time.Minute
)

const (
	guideMaxTime = 10 * time.Minute
	guideBatch   = 500
)

// idleReader gives up on a source that stops sending.
type idleReader struct {
	r     io.Reader
	timer *time.Timer
	idle  time.Duration
}

func (i *idleReader) Read(p []byte) (int, error) {
	i.timer.Reset(i.idle)
	return i.r.Read(p)
}

// Ingest reads a guide WittyTuner fetched from one of its sources (id), keeping only the programmes
// of channels on this server, and says which channels without a guide id of their own it matched
// by name, for WittyTuner to keep. A guide cut short changes nothing.
func (m *Manager) Ingest(ctx context.Context, id int64, r io.Reader) (int, map[int64]string, error) {
	ctx, cancel := context.WithTimeout(ctx, guideMaxTime)
	defer cancel()
	list, _, err := m.allChannels(ctx, true)
	if err != nil {
		return 0, nil, err
	}
	n, matched, err := m.fetch(ctx, cancel, id, r, list)
	if err == nil {
		m.Start()
		m.ForgetChannels()
	}
	return n, matched, err
}

func (m *Manager) fetch(ctx context.Context, cancel context.CancelFunc, id int64, r io.Reader, list []*Channel) (int, map[int64]string, error) {
	// Whatever is reading the guide is stopped with it: a body that stops sending holds a read.
	if c, ok := r.(io.Closer); ok {
		defer context.AfterFunc(ctx, func() { _ = c.Close() })()
	}
	stalled := time.AfterFunc(guideIdle, cancel)
	defer stalled.Stop()
	// What the server has: guide ids, and names to match on when there are none.
	wanted := map[string]bool{}
	byName := map[string]int64{}
	for _, c := range list {
		if !c.Enabled {
			continue
		}
		if k := c.GuideKey(); k != "" {
			wanted[k] = true
		} else {
			byName[normalizeName(c.Name)] = c.ID
		}
	}
	now := time.Now()
	from, to := now.Add(-12*time.Hour), now.AddDate(0, 0, epgKeepDays)
	src := fmt.Sprintf("xmltv:%d", id)
	// Programmes go in a few at a time under a name of their own, and take the source's name when
	// the guide has been read to its end, so a guide cut short changes nothing.
	coming := src + ":coming"
	discard := func() {
		_, _ = m.DB.ExecContext(context.WithoutCancel(ctx), `DELETE FROM epg_programmes WHERE source = ?`, coming)
	}
	discard()
	type channel struct{ id, name string }
	var channels []channel
	matched := map[int64]string{}
	var batch []*xmltv.Programme
	count := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		tx, err := m.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, p := range batch {
			if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO epg_programmes(channel, start, stop, title, sub, descr, category, rating, season, episode, source, extra)
				VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, p.Channel, p.Start.Unix(), p.Stop.Unix(), clip(p.Title, 300), clip(p.Sub, 300), clip(p.Desc, 2000), clip(p.Category, 80), clip(p.Rating, 30), p.Season, p.Episode, coming, detailsJSON(p.Details)); err != nil {
				return err
			}
		}
		batch = batch[:0]
		return tx.Commit()
	}
	open := map[string]*xmltv.Programme{}
	save := func(p *xmltv.Programme) error {
		if p.Stop.IsZero() || !p.Stop.After(p.Start) || p.Stop.Before(from) || p.Start.After(to) || count >= maxGuideProgrammes {
			return nil
		}
		count++
		batch = append(batch, p)
		if len(batch) >= guideBatch {
			return flush()
		}
		return nil
	}
	var failed error
	err := xmltv.Parse(&idleReader{r: r, timer: stalled, idle: guideIdle}, func(c xmltv.Channel) bool {
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
		}
		if len(channels) < maxGuideChannels {
			channels = append(channels, channel{c.ID, clip(name, 200)})
		}
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
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = flush()
	}
	if err != nil {
		discard()
		return 0, nil, fmt.Errorf("the guide could not be read: %w", err)
	}
	stalled.Stop()
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		discard()
		return 0, nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM epg_channels WHERE source_id = ?`, id); err != nil {
		return 0, nil, err
	}
	for _, c := range channels {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO epg_channels(source_id, epg_id, name) VALUES(?,?,?)`, id, c.id, c.name); err != nil {
			return 0, nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM epg_programmes WHERE source = ?`, src); err != nil {
		return 0, nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE epg_programmes SET source = ? WHERE source = ?`, src, coming); err != nil {
		return 0, nil, err
	}
	return count, matched, tx.Commit()
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
		m.background("guide from broadcast", func(ctx context.Context) { m.storeBroadcast(ctx, key, list) }, "cat", "tv")
	}
}

func (m *Manager) storeBroadcast(ctx context.Context, key string, list []mpegts.Event) {
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
