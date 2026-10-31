package library

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// What is known about a station, and whether it is still on the air.

// RadioMeta is a station's details. Favorite and LastPlayed belong to the account asking.
type RadioMeta struct {
	Source      string `json:"source,omitempty"`   // the directory it came from; "" when added by address
	SourceID    string `json:"sourceId,omitempty"` // its ID there
	Homepage    string `json:"homepage,omitempty"`
	Country     string `json:"country,omitempty"`
	CountryCode string `json:"countryCode,omitempty"`
	Language    string `json:"language,omitempty"`
	Codec       string `json:"codec,omitempty"`
	Bitrate     int    `json:"bitrate,omitempty"`
	HLS         bool   `json:"hls"` // the stream is HLS: GET /radio/{id}/stream answers with a playlist
	// Status: ok | trouble (recent failures, may recover) | dead | unknown (not tried yet).
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"` // why it isn't ok, in plain words
	LastOK     int64  `json:"lastOk,omitempty"`
	LastCheck  int64  `json:"lastCheck,omitempty"`
	Favorite   bool   `json:"favorite"`
	LastPlayed int64  `json:"lastPlayed,omitempty"`
}

const (
	// radioDeadAfter failures in a row, each radioFailGap apart: a listener retrying
	// during a station's bad minute doesn't bury it.
	radioDeadAfter  = 3
	radioRecentOK   = 7 * 24 * time.Hour
	radioDirRefresh = 20 * time.Hour
	radioCheckEvery = 7 * 24 * time.Hour
	radioDirBatch   = 50
	maxRadioChecks  = 200 // in one scheduled round
	radioOfflineAt  = 5   // this many failures with no success: it's this server that's offline
)

// radioFailGap and radioPauseFor can be shortened by tests.
var (
	radioFailGap  = int64(600)
	radioPauseFor = 3 * time.Second
)

const radioMetaCols = `IFNULL(m.source, ''), IFNULL(m.source_id, ''), IFNULL(m.homepage, ''), IFNULL(m.country, ''), IFNULL(m.country_code, ''), IFNULL(m.language, ''),
	IFNULL(m.codec, ''), IFNULL(m.bitrate, 0), IFNULL(m.hls, 0), IFNULL(m.dir_ok, -1), IFNULL(m.dir_gone, 0), IFNULL(m.fail_count, 0), IFNULL(m.last_ok, 0), IFNULL(m.last_check, 0), IFNULL(m.last_error, '')`

type radioRow struct {
	meta           RadioMeta
	dirOK, dirGone int
	failCount      int
	lastError      string
}

func (r *radioRow) dest() []any {
	m := &r.meta
	return []any{&m.Source, &m.SourceID, &m.Homepage, &m.Country, &m.CountryCode, &m.Language, &m.Codec, &m.Bitrate, &m.HLS, &r.dirOK, &r.dirGone, &r.failCount, &m.LastOK, &m.LastCheck, &r.lastError}
}

// status works out how a station is doing: its record here first, the directory's
// checks where this server hasn't heard from it lately.
func (r *radioRow) status(now int64) {
	m := &r.meta
	recent := m.LastOK > now-int64(radioRecentOK/time.Second) && r.failCount == 0
	switch {
	case r.failCount >= radioDeadAfter:
		m.Status, m.Reason = "dead", firstNonEmpty(r.lastError, "It hasn't answered lately.")
	case r.dirOK == 0 && !recent:
		m.Status, m.Reason = "dead", "The directory's own checks can't reach it."
	case r.dirGone != 0 && !recent:
		m.Status, m.Reason = "trouble", "The directory no longer lists it."
	case r.failCount > 0:
		m.Status, m.Reason = "trouble", firstNonEmpty(r.lastError, "It didn't answer last time.")
	case m.LastOK > 0 || r.dirOK == 1:
		m.Status = "ok"
	default:
		m.Status = "unknown"
	}
}

// AttachRadio fills in the details of a library's stations for one account.
func (s *Store) AttachRadio(ctx context.Context, userID int64, items []*Item) error {
	if len(items) == 0 {
		return nil
	}
	byID := make(map[int64]*Item, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	now := time.Now().Unix()
	scan := func(q string, args ...any) error {
		rows, err := s.DB.QueryContext(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var r radioRow
			var fav bool
			var last int64
			if err := rows.Scan(append(append([]any{&id}, r.dest()...), &fav, &last)...); err != nil {
				return err
			}
			r.status(now)
			r.meta.Favorite, r.meta.LastPlayed = fav, last
			if it := byID[id]; it != nil {
				m := r.meta
				it.Radio = &m
			}
		}
		return rows.Err()
	}
	const cols = `SELECT i.id, ` + radioMetaCols + `,
		EXISTS(SELECT 1 FROM item_reactions x WHERE x.user_id = ? AND x.item_id = i.id AND x.value > 0),
		IFNULL((SELECT st.last_at FROM item_stats st WHERE st.user_id = ? AND st.item_id = i.id), 0)
		FROM items i LEFT JOIN radio_meta m ON m.item_id = i.id `
	if len(items) == 1 {
		return scan(cols+`WHERE i.id = ?`, userID, userID, items[0].ID)
	}
	return scan(cols+`WHERE i.library_id = ? AND i.kind = 'radio'`, userID, userID, items[0].LibraryID)
}

// RadioAddresses says which directory stations and addresses a library already has.
func (s *Store) RadioAddresses(ctx context.Context, libraryID int64) (map[string]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT i.path, IFNULL(m.source_id, '') FROM items i LEFT JOIN radio_meta m ON m.item_id = i.id WHERE i.library_id = ? AND i.kind = 'radio'`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var path, id string
		if err := rows.Scan(&path, &id); err != nil {
			return nil, err
		}
		out[path] = true
		if id != "" {
			out[id] = true
		}
	}
	return out, rows.Err()
}

// RadioResult records whether a station answered. Failures close together count
// once, so it takes a real outage to mark a station dead.
func (s *Store) RadioResult(ctx context.Context, id int64, failure error) {
	now := db.Now()
	if failure == nil {
		_, _ = s.DB.ExecContext(ctx, `INSERT INTO radio_meta(item_id, last_ok, last_check) VALUES(?,?,?)
			ON CONFLICT(item_id) DO UPDATE SET last_ok = excluded.last_ok, last_check = excluded.last_check, fail_count = 0, last_error = ''`, id, now, now)
		return
	}
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO radio_meta(item_id, fail_count, last_check, last_error) VALUES(?,1,?,?)
		ON CONFLICT(item_id) DO UPDATE SET fail_count = fail_count + (CASE WHEN ? - last_check >= ? OR fail_count = 0 THEN 1 ELSE 0 END), last_check = excluded.last_check, last_error = excluded.last_error`,
		id, now, radioFailText(failure), now, radioFailGap)
}

// radioFailText keeps an error's plain words, not the address it may quote.
func radioFailText(err error) string {
	switch {
	case errors.Is(err, ErrNotAudio):
		return "That address isn't sending audio."
	case errors.Is(err, ErrStationEncrypted):
		return "Its stream is encrypted, so it can't be played."
	case strings.HasPrefix(err.Error(), "can't be reached"):
		return "It can't be reached."
	}
	return CleanRadioText(err.Error(), 120)
}

// ApplyStationFacts fills what a station says about itself into the gaps of what
// is known. It never overwrites, and writes only when something is new.
func (s *Store) ApplyStationFacts(ctx context.Context, it *Item, f StationFacts) {
	var r radioRow
	err := s.DB.QueryRowContext(ctx, `SELECT `+radioMetaCols+` FROM items i LEFT JOIN radio_meta m ON m.item_id = i.id WHERE i.id = ?`, it.ID).Scan(r.dest()...)
	if err != nil {
		return
	}
	m := r.meta
	changed := m.HLS != f.HLS
	fill := func(have *string, v string) {
		if *have == "" && v != "" {
			*have, changed = v, true
		}
	}
	fill(&m.Homepage, f.Homepage)
	fill(&m.Codec, f.Codec)
	if m.Bitrate == 0 && f.Bitrate > 0 {
		m.Bitrate, changed = f.Bitrate, true
	}
	if changed {
		_, _ = s.DB.ExecContext(ctx, `INSERT INTO radio_meta(item_id, homepage, codec, bitrate, hls) VALUES(?,?,?,?,?)
			ON CONFLICT(item_id) DO UPDATE SET homepage = excluded.homepage, codec = excluded.codec, bitrate = excluded.bitrate, hls = excluded.hls`, it.ID, m.Homepage, m.Codec, m.Bitrate, f.HLS)
	}
	// A station added by its address alone is still called by its host name.
	title, overview, genres := it.Title, it.Overview, it.Genres
	if u, err := url.Parse(it.Path); err == nil && title == u.Host && f.Name != "" {
		title = f.Name
	}
	if overview == "" {
		overview = f.Description
	}
	if len(genres) == 0 && f.Genre != "" {
		genres = []string{f.Genre}
	}
	if title != it.Title || overview != it.Overview || len(genres) != len(it.Genres) {
		_, _ = s.DB.ExecContext(ctx, `UPDATE items SET title = ?, sort_title = ?, overview = ?, genres = ?, updated_at = ? WHERE id = ? AND kind = 'radio'`, title, SortTitle(title), overview, joinList(genres), db.Now(), it.ID)
		it.Title, it.Overview, it.Genres = title, overview, genres
	}
}

// AddListening adds time to a station's record without counting another play.
func (s *Store) AddListening(ctx context.Context, userID, itemID int64, seconds float64) {
	if seconds <= 0 {
		return
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE item_stats SET seconds = seconds + ?, last_at = ? WHERE user_id = ? AND item_id = ?`, seconds, db.Now(), userID, itemID)
	_ = s.LogPlay(ctx, userID, itemID, seconds)
}

// ---------------------------------------------------------------------------
// From the directory
// ---------------------------------------------------------------------------

// AddFromDirectory adds a station by its directory ID. The details are read from
// the directory here, never taken from the client.
func (s *Store) AddFromDirectory(ctx context.Context, libraryID int64, id string) (*Item, error) {
	found, err := DirectoryByID(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, ErrBadInput("that station is no longer in the directory, or its address can't be used")
	}
	st := found[0]
	it, err := s.saveDirectoryStation(ctx, libraryID, st)
	if err != nil {
		return nil, err
	}
	directoryClick(st.ID)
	return it, nil
}

func (s *Store) saveDirectoryStation(ctx context.Context, libraryID int64, st DirectoryStation) (*Item, error) {
	it := &Item{LibraryID: libraryID, Kind: "radio", Path: st.URL, Title: st.Name, SortTitle: SortTitle(st.Name), Container: "radio", Genres: st.Tags, Poster: st.Logo}
	if existing, err := s.itemByPath(ctx, libraryID, st.URL); err == nil {
		it.ID, it.AddedAt, it.Overview = existing.ID, existing.AddedAt, existing.Overview
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if err := s.upsertItem(ctx, it); err != nil {
		return nil, err
	}
	return it, s.saveDirectoryMeta(ctx, it.ID, st)
}

func (s *Store) saveDirectoryMeta(ctx context.Context, itemID int64, st DirectoryStation) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO radio_meta(item_id, source, source_id, homepage, country, country_code, language, codec, bitrate, hls, dir_ok, dir_gone, dir_checked)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,0,?)
		ON CONFLICT(item_id) DO UPDATE SET source = excluded.source, source_id = excluded.source_id, homepage = excluded.homepage, country = excluded.country,
		country_code = excluded.country_code, language = excluded.language, codec = excluded.codec, bitrate = excluded.bitrate, hls = excluded.hls,
		dir_ok = excluded.dir_ok, dir_gone = 0, dir_checked = excluded.dir_checked`,
		itemID, RadioDirectorySource, st.ID, st.Homepage, st.Country, st.CountryCode, st.Language, st.Codec, st.Bitrate, st.HLS, boolInt(st.Working), db.Now())
	return err
}

// LookUpRadio finds a station added by address in the directory, on an
// administrator's request, and fills in what it knows. The address is sent only when
// it can't be someone's private one: a query string or sign-in in it may be a key.
func (s *Store) LookUpRadio(ctx context.Context, it *Item) error {
	u, err := url.Parse(it.Path)
	if err != nil || u.User != nil || u.RawQuery != "" {
		return ErrBadInput("this station's address may hold a private key, so it isn't sent to the directory")
	}
	found, err := directoryByURL(ctx, it.Path)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return ErrNotFound
	}
	st := found[0]
	if it.Poster == "" {
		it.Poster = st.Logo
	}
	genres := it.Genres
	if len(st.Tags) > 0 {
		genres = st.Tags
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE items SET poster = ?, genres = ?, updated_at = ? WHERE id = ?`, it.Poster, joinList(genres), db.Now(), it.ID); err != nil {
		return err
	}
	return s.saveDirectoryMeta(ctx, it.ID, st)
}

// refreshFromDirectory re-reads stations that came from the directory, in small
// batches with a pause between them, a little at a time on low-end machines. It says
// what it found out: new logos and details, and stations no longer listed or working.
func (s *Store) refreshFromDirectory(ctx context.Context) error {
	cutoff := time.Now().Add(-radioDirRefresh).Unix()
	rows, err := s.DB.QueryContext(ctx, `SELECT m.item_id, m.source_id FROM radio_meta m WHERE m.source = ? AND m.source_id != '' AND m.dir_checked < ?`, RadioDirectorySource, cutoff)
	if err != nil {
		return err
	}
	type due struct {
		item int64
		id   string
	}
	var all []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.item, &d.id); err != nil {
			rows.Close()
			return err
		}
		all = append(all, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for start := 0; start < len(all); start += radioDirBatch {
		batch := all[start:min(start+radioDirBatch, len(all))]
		ids := make([]string, len(batch))
		for i, d := range batch {
			ids[i] = d.id
		}
		found, err := DirectoryByID(ctx, ids)
		if err != nil {
			return err // try again next time
		}
		byID := map[string]DirectoryStation{}
		for _, st := range found {
			byID[st.ID] = st
		}
		now := db.Now()
		for _, d := range batch {
			st, ok := byID[strings.ToLower(d.id)]
			if !ok {
				_, _ = s.DB.ExecContext(ctx, `UPDATE radio_meta SET dir_gone = 1, dir_checked = ? WHERE item_id = ?`, now, d.item)
				continue
			}
			s.applyDirectoryRefresh(ctx, d.item, st)
		}
		if start+radioDirBatch < len(all) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(radioPauseFor):
			}
		}
	}
	return nil
}

func (s *Store) applyDirectoryRefresh(ctx context.Context, itemID int64, st DirectoryStation) {
	_ = s.saveDirectoryMeta(ctx, itemID, st)
	var poster, path, genres string
	var fails int
	err := s.DB.QueryRowContext(ctx, `SELECT IFNULL(i.poster, ''), i.path, IFNULL(i.genres, ''), IFNULL(m.fail_count, 0) FROM items i LEFT JOIN radio_meta m ON m.item_id = i.id WHERE i.id = ?`, itemID).Scan(&poster, &path, &genres, &fails)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return
	}
	newPath := path
	// A station that stopped answering here and moved in the directory has a new
	// address. One that works is left alone: nobody asked for it to change.
	if fails > 0 && st.URL != path {
		newPath = st.URL
	}
	newGenres := joinList(st.Tags)
	if len(st.Tags) == 0 {
		newGenres = genres
	}
	if st.Logo == poster && newPath == path && newGenres == genres {
		return
	}
	logo := st.Logo
	if logo == "" {
		logo = poster
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE items SET poster = ?, path = ?, genres = ?, updated_at = ? WHERE id = ?`, logo, newPath, newGenres, db.Now(), itemID); err != nil {
		// The new address is already another station in this library.
		_, _ = s.DB.ExecContext(ctx, `UPDATE items SET poster = ?, genres = ?, updated_at = ? WHERE id = ?`, logo, newGenres, db.Now(), itemID)
	}
}

// ---------------------------------------------------------------------------
// Checking stations
// ---------------------------------------------------------------------------

// CheckStation tries a station now and records the result. A failure on this end
// (the request was cancelled) says nothing about the station, and isn't recorded.
func (s *Store) CheckStation(ctx context.Context, it *Item) error {
	f, err := ProbeStation(ctx, it.Path)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.RadioResult(ctx, it.ID, err)
	if err == nil {
		s.ApplyStationFacts(ctx, it, f)
	}
	return err
}

var radioChecking atomic.Bool

// CheckStations tries the stations given, one at a time with a pause between them.
// If nothing answers, the trouble is probably this server's own connection, so
// none of them are marked. It answers false if a round is already running.
func (s *Store) CheckStations(ctx context.Context, stations []*Item, log func(string, ...any)) bool {
	if !radioChecking.CompareAndSwap(false, true) {
		return false
	}
	defer radioChecking.Store(false)
	var waiting []struct {
		id  int64
		err error
	}
	anyOK := false
	for i, it := range stations {
		if i > 0 {
			select {
			case <-ctx.Done():
				return true
			case <-time.After(radioPauseFor):
			}
		}
		f, err := ProbeStation(ctx, it.Path)
		if ctx.Err() != nil {
			return true
		}
		switch {
		case err == nil:
			anyOK = true
			s.RadioResult(ctx, it.ID, nil)
			s.ApplyStationFacts(ctx, it, f)
			for _, w := range waiting {
				s.RadioResult(ctx, w.id, w.err)
			}
			waiting = nil
		case anyOK:
			s.RadioResult(ctx, it.ID, err)
		default:
			waiting = append(waiting, struct {
				id  int64
				err error
			}{it.ID, err})
			if len(waiting) >= radioOfflineAt && len(stations) > radioOfflineAt {
				log("radio check stopped: nothing answers, so this server may be offline")
				return true
			}
		}
	}
	// Only a few stations, and none answered: there's nothing to compare with.
	for _, w := range waiting {
		s.RadioResult(ctx, w.id, w.err)
	}
	return true
}

// stationsDueForCheck lists stations this server hasn't tried for a while.
func (s *Store) stationsDueForCheck(ctx context.Context) ([]*Item, error) {
	cutoff := time.Now().Add(-radioCheckEvery).Unix()
	rows, err := s.DB.QueryContext(ctx, `SELECT `+itemCols+` FROM items i LEFT JOIN radio_meta m ON m.item_id = i.id
		WHERE i.kind = 'radio' AND IFNULL(m.last_check, 0) < ? ORDER BY IFNULL(m.last_check, 0) LIMIT ?`, cutoff, maxRadioChecks)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// RefreshRadio is the quiet background work, run a few times a day: details from the
// directory (unless turned off), and checks on stations if the owner turned those on.
func (s *Scanner) RefreshRadio(ctx context.Context) {
	if s.Store.DB.GetSetting(ctx, "radioDirectory", "1") == "1" {
		if err := s.Store.refreshFromDirectory(ctx); err != nil {
			s.Log.Info("radio directory refresh failed", "cat", "library", "err", err)
		}
	}
	if s.Store.DB.GetSetting(ctx, "radioChecks", "0") == "1" {
		due, err := s.Store.stationsDueForCheck(ctx)
		if err == nil && len(due) > 0 {
			s.Store.CheckStations(ctx, due, func(msg string, args ...any) { s.Log.Info(msg, append([]any{"cat", "library"}, args...)...) })
		}
	}
}
