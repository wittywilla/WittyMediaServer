// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
)

// Skip markers: where the intro, recap, credits and adverts are in a video. The scan reads
// them from chapter names into the scanned column, which each scan rewrites. What an
// administrator sets goes in the manual column, which no scan touches.

// chapterMarkers reads what a file's chapter names say.
func chapterMarkers(p *ffmpeg.Probe) *mediaextras.Markers {
	var chs []mediaextras.Chapter
	for _, ch := range p.Chapters {
		a, err1 := strconv.ParseFloat(ch.Start, 64)
		b, err2 := strconv.ParseFloat(ch.End, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		chs = append(chs, mediaextras.Chapter{Start: a, End: b, Title: ch.Tags["title"]})
	}
	m := mediaextras.FromChapters(chs)
	return &m
}

// SaveScannedMarkers keeps what a scan read, replacing what an earlier scan read. What an
// administrator set is left as it is.
func (s *Store) SaveScannedMarkers(ctx context.Context, id int64, m *mediaextras.Markers) error {
	if m == nil || id == 0 {
		return nil
	}
	body := ""
	if !m.Empty() {
		b, err := json.Marshal(m.ToSkipFile())
		if err != nil {
			return err
		}
		body = string(b)
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO media_markers(item_id, scanned, updated_at) VALUES(?,?,?)
		ON CONFLICT(item_id) DO UPDATE SET scanned = excluded.scanned, updated_at = excluded.updated_at`, id, body, db.Now())
	return err
}

// StoredMarkers are an item's kept markers: what the scan read and what someone set.
type StoredMarkers struct {
	Scanned *mediaextras.Markers
	Manual  *mediaextras.Markers
	// Detected is what comparing the season's episodes by ear found.
	Detected *mediaextras.Markers
}

func decodeMarkers(s string) *mediaextras.Markers {
	if s == "" {
		return nil
	}
	var f mediaextras.SkipFile
	if json.Unmarshal([]byte(s), &f) != nil {
		return nil
	}
	m, err := f.Markers(0)
	if err != nil {
		return nil
	}
	return &m
}

// Markers reads an item's kept markers.
func (s *Store) Markers(ctx context.Context, id int64) StoredMarkers {
	var scanned, manual, detected string
	err := s.DB.QueryRowContext(ctx, `SELECT scanned, manual, detected FROM media_markers WHERE item_id = ?`, id).Scan(&scanned, &manual, &detected)
	if err != nil {
		return StoredMarkers{}
	}
	return StoredMarkers{Scanned: decodeMarkers(scanned), Manual: decodeMarkers(manual), Detected: decodeMarkers(detected)}
}

// SaveDetectedMarkers keeps what comparing episodes found for one of them, replacing what
// an earlier comparison found; nil or empty forgets it. What a scan read and what someone
// set are left as they are.
func (s *Store) SaveDetectedMarkers(ctx context.Context, id int64, m *mediaextras.Markers) error {
	body := ""
	if m != nil && !m.Empty() {
		b, err := json.Marshal(m.ToSkipFile())
		if err != nil {
			return err
		}
		body = string(b)
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO media_markers(item_id, detected, updated_at) VALUES(?,?,?)
		ON CONFLICT(item_id) DO UPDATE SET detected = excluded.detected, updated_at = excluded.updated_at`, id, body, db.Now())
	return err
}

// SetManualMarkers keeps an administrator's markers for an item; nil forgets them, and
// the file's own come back.
func (s *Store) SetManualMarkers(ctx context.Context, id int64, m *mediaextras.Markers) error {
	if _, err := s.Item(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	body := ""
	if m != nil {
		b, err := json.Marshal(m.ToSkipFile())
		if err != nil {
			return err
		}
		body = string(b)
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO media_markers(item_id, manual, updated_at) VALUES(?,?,?)
		ON CONFLICT(item_id) DO UPDATE SET manual = excluded.manual, updated_at = excluded.updated_at`, id, body, db.Now())
	return err
}
