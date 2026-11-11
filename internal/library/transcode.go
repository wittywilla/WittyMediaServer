// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// What library conversion needs from the library: which items are files it could work on,
// and a way to say an item's file has changed without losing anything about the item.

// ConvertedInfix marks a converted copy written beside its original: "Film.wms-converted.mkv".
// A scan leaves it out, so it never shows up as a film of its own.
const ConvertedInfix = ".wms-converted."

// Hidden folders beside a video. A scan never goes into a folder whose name starts with a dot.
const (
	OriginalsFolder = ".wittymedia-originals"
	WorkFolder      = ".wittymedia-work"
)

// IsConvertedCopy says whether a file name is such a copy.
func IsConvertedCopy(name string) bool {
	return strings.Contains(strings.ToLower(name), ConvertedInfix)
}

// ConversionItem is a file of a library, as a conversion sees it.
type ConversionItem struct {
	ID      int64
	Path    string
	Kind    string
	Title   string
	Missing bool
}

// ConversionItems lists what's in a library, in the order it would be worked through.
func (s *Store) ConversionItems(ctx context.Context, libraryID int64) ([]ConversionItem, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, path, kind, title, missing FROM items WHERE library_id = ? ORDER BY sort_title, path`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConversionItem{}
	for rows.Next() {
		var it ConversionItem
		var missing int
		if err := rows.Scan(&it.ID, &it.Path, &it.Kind, &it.Title, &missing); err != nil {
			return nil, err
		}
		it.Missing = missing != 0
		out = append(out, it)
	}
	return out, rows.Err()
}

// ConversionItemsSince lists the video files of a library that are new or have changed since
// a time. A row changes for other reasons too (someone edits its details), so this is who to
// look at, and doesn't say that anything needs doing.
func (s *Store) ConversionItemsSince(ctx context.Context, libraryID, since int64) ([]ConversionItem, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, path, kind, title, missing FROM items
		WHERE library_id = ? AND kind = 'video' AND missing = 0 AND (added_at >= ? OR updated_at >= ?) ORDER BY added_at, id`, libraryID, since, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConversionItem{}
	for rows.Next() {
		var it ConversionItem
		var missing int
		if err := rows.Scan(&it.ID, &it.Path, &it.Kind, &it.Title, &missing); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// SetLibraryReadOnly says that nothing the server does may change this library's files.
func (s *Store) SetLibraryReadOnly(ctx context.Context, id int64, on bool) error {
	v := 0
	if on {
		v = 1
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE libraries SET read_only = ? WHERE id = ?`, v, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ErrReadOnly: someone asked to add a file to a library set to read-only.
var ErrReadOnly = errors.New("this library is set to read-only, so nothing can be added to its folders. An administrator can change that under its conversion settings")

// libraryReadOnly: nothing may be written into this library's folders.
func (s *Store) libraryReadOnly(ctx context.Context, id int64) bool {
	var on int
	_ = s.DB.QueryRowContext(ctx, `SELECT read_only FROM libraries WHERE id = ?`, id).Scan(&on)
	return on != 0
}

// FileReplaced records that an item's file is now the one at path, after a conversion.
// Only what the file itself says changes. The item keeps its id, so everything anyone
// has done with it stays: what's been watched and how far, ratings, its match and every
// edit. And because the size and time are up to date, the next scan sees nothing new.
func (s *Scanner) FileReplaced(ctx context.Context, id int64, path string) error {
	old, err := s.Store.Item(ctx, id)
	if err != nil {
		return err
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	fresh := &Item{Path: path}
	s.probeVideo(ctx, fresh)
	runtime := fresh.RuntimeSec
	if runtime <= 0 {
		runtime = old.RuntimeSec
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// A scan running at the wrong moment may have added the new name as a new item,
	// which nobody has had time to do anything with.
	if path != old.Path {
		if _, err := tx.ExecContext(ctx, `DELETE FROM items WHERE library_id = ? AND path = ? AND id <> ?`, old.LibraryID, path, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE items SET path = ?, size_bytes = ?, mtime = ?, runtime_sec = ?, container = ?, video_codec = ?, width = ?, height = ?,
		audio_langs = ?, sub_langs = ?, updated_at = ?, missing = 0 WHERE id = ?`,
		path, st.Size(), st.ModTime().Unix(), runtime, fresh.Container, fresh.VideoCodec, fresh.Width, fresh.Height,
		joinList(fresh.AudioLangs), joinList(fresh.SubLangs), db.Now(), id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE title_meta SET audio_format = ? WHERE target = ? AND target_id = ?`, audioFormatOf(fresh), TargetItem, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ScanHold keeps scans away for a moment, however this scanner was made.
func (s *Scanner) ScanHold(ctx context.Context) (func(), error) {
	if s.slot == nil {
		return func() {}, nil
	}
	return s.HoldScans(ctx)
}
