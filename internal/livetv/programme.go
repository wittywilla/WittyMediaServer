// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/tuner/xmltv"
)

// One programme in full, for WittyTV's More info: what the guide's grid shows, the rest of what
// the guide says about it, and where else the same title airs.

// detailsJSON is what is stored for a programme's details: nothing when the guide said nothing more.
func detailsJSON(d xmltv.Details) string {
	b, err := json.Marshal(d)
	if err != nil || string(b) == "{}" {
		return ""
	}
	return string(b)
}

// ErrNoProgramme: the guide has nothing on that channel at that time.
var ErrNoProgramme = errors.New("the guide has no programme then")

// ProgrammeAt is the programme on a channel at a moment, with its details. The stored guide wins
// over what the broadcast said about itself, as in the grid.
func (m *Manager) ProgrammeAt(ctx context.Context, c *Channel, at int64) (*Programme, xmltv.Details, error) {
	keys := []any{broadcastKey(c)}
	if k := c.GuideKey(); k != "" {
		keys = append(keys, k)
	}
	q := `SELECT channel, start, stop, title, sub, descr, category, rating, season, episode, extra FROM epg_programmes
		WHERE channel IN (` + strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",") + `) AND start <= ? AND stop > ?
		ORDER BY channel = ? LIMIT 1`
	var p Programme
	var ch, extra string
	var season, episode sql.NullInt64
	err := m.DB.QueryRowContext(ctx, q, append(keys, at, at, broadcastKey(c))...).
		Scan(&ch, &p.Start, &p.End, &p.Title, &p.Sub, &p.Desc, &p.Category, &p.Rating, &season, &episode, &extra)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, xmltv.Details{}, ErrNoProgramme
	}
	if err != nil {
		return nil, xmltv.Details{}, err
	}
	if season.Valid {
		v := int(season.Int64)
		p.Season = &v
	}
	if episode.Valid {
		v := int(episode.Int64)
		p.Episode = &v
	}
	var d xmltv.Details
	if extra != "" {
		// A row written by an older version, or by the broadcast, has none: that's not a fault.
		_ = json.Unmarshal([]byte(extra), &d)
	}
	return &p, d, nil
}

// Airing is one time a title is on.
type Airing struct {
	ChannelID int64  `json:"-"`
	Start     int64  `json:"start"`
	End       int64  `json:"end"`
	Sub       string `json:"sub,omitempty"`
	Season    *int   `json:"season,omitempty"`
	Episode   *int   `json:"episode,omitempty"`
}

// maxAirings bounds what one title's schedule looks through.
const maxAirings = 200

// Airings are the coming airings of a title on every channel in the guide (turned on, on a tuner
// that's on, not a backup), soonest first. The caller keeps those the person may watch.
func (m *Manager) Airings(ctx context.Context, title string, from, to time.Time, limit int) ([]Airing, error) {
	title = strings.TrimSpace(title)
	if title == "" || limit <= 0 {
		return nil, nil
	}
	rows, err := m.DB.QueryContext(ctx, `SELECT channel, start, stop, sub, season, episode FROM epg_programmes
		WHERE title = ? COLLATE NOCASE AND stop > ? AND start < ? ORDER BY start LIMIT ?`, title, from.Unix(), to.Unix(), maxAirings)
	if err != nil {
		return nil, err
	}
	type hit struct {
		key string
		a   Airing
	}
	var hits []hit
	for rows.Next() {
		var h hit
		var season, episode sql.NullInt64
		if rows.Scan(&h.key, &h.a.Start, &h.a.End, &h.a.Sub, &season, &episode) != nil {
			continue
		}
		if season.Valid {
			v := int(season.Int64)
			h.a.Season = &v
		}
		if episode.Valid {
			v := int(episode.Int64)
			h.a.Episode = &v
		}
		hits = append(hits, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	byKey := map[string][]int64{}
	var out []Airing
	seen := map[[2]int64]bool{}
	for _, h := range hits {
		ids, ok := byKey[h.key]
		if !ok {
			ids = m.guideChannelsFor(ctx, h.key)
			byKey[h.key] = ids
		}
		for _, id := range ids {
			if k := [2]int64{id, h.a.Start}; !seen[k] {
				seen[k] = true
				a := h.a
				a.ChannelID = id
				out = append(out, a)
			}
		}
		if len(out) >= limit {
			break
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// guideChannelsFor are the channels in the guide whose programmes are stored under this key:
// "tuner:<id>" for what a broadcast said, else a guide id a channel was matched or set to.
func (m *Manager) guideChannelsFor(ctx context.Context, key string) []int64 {
	const inGuide = `c.enabled = 1 AND t.enabled = 1 AND c.merged_into IS NULL`
	var rows *sql.Rows
	var err error
	if v, ok := strings.CutPrefix(key, "tuner:"); ok {
		id, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil {
			return nil
		}
		rows, err = m.DB.QueryContext(ctx, `SELECT c.id FROM tuner_channels c JOIN tuners t ON t.id = c.tuner_id WHERE c.id = ? AND `+inGuide, id)
	} else {
		rows, err = m.DB.QueryContext(ctx, `SELECT c.id FROM tuner_channels c JOIN tuners t ON t.id = c.tuner_id
			WHERE (c.epg_set = ? OR (c.epg_set = '' AND c.epg_id = ?)) AND `+inGuide+` ORDER BY c.id LIMIT 20`, key, key)
	}
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}
