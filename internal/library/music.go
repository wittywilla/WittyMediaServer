// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"database/sql"
	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
	"strconv"
	"strings"
)

// Albums and artists are not stored separately: they are the audio items
// grouped by album artist and album, so they can never disagree with the
// files. The items_album index keeps this fast.

// Album is a group of tracks (or an audiobook's chapters).
type Album struct {
	Artist      string  `json:"artist"`
	Title       string  `json:"title"`
	Year        *int    `json:"year,omitempty"`
	Tracks      int     `json:"tracks"`
	DurationSec float64 `json:"durationSec"`
	CoverItem   int64   `json:"coverItem"` // an item whose poster is the cover
	// FirstItem is the album's first track (disc, then track number): what Play starts
	// with, and what tells two albums apart when neither has a cover.
	FirstItem int64  `json:"firstItem,omitempty"`
	AddedAt   int64  `json:"addedAt"`
	Genre     string `json:"genre,omitempty"`
}

// Artist is everyone credited as album artist (or the author).
type Artist struct {
	Name      string `json:"name"`
	Albums    int    `json:"albums"`
	Tracks    int    `json:"tracks"`
	CoverItem int64  `json:"coverItem"`
}

const albumArtist = "IFNULL(NULLIF(i.album_artist, ''), IFNULL(i.artist, ''))"

func (s *Store) audioWhere(libraryID int64, access *Access) ([]string, []any) {
	where := []string{"i.kind = 'audio'", "i.missing = 0", "i.library_id = ?"}
	args := []any{libraryID}
	if access != nil {
		c, a := access.ItemClause()
		where = append(where, c)
		args = append(args, a...)
	}
	return where, args
}

// AlbumFilter narrows and orders a list of albums. Sort: title | artist | year |
// added | played (this person's own plays, which needs UserID).
type AlbumFilter struct {
	Artist, Search, Sort, Genre string
	UserID                      int64
	Limit                       int
}

// Albums lists a library's albums (or books). sort: title | artist | year | added.
func (s *Store) Albums(ctx context.Context, libraryID int64, artist, search, sort string, access *Access) ([]Album, error) {
	return s.AlbumsFiltered(ctx, libraryID, AlbumFilter{Artist: artist, Search: search, Sort: sort}, access)
}

// AlbumsFiltered is Albums with the rest of what a library's page can ask for.
func (s *Store) AlbumsFiltered(ctx context.Context, libraryID int64, f AlbumFilter, access *Access) ([]Album, error) {
	where, args := s.audioWhere(libraryID, access)
	if f.Artist != "" {
		where = append(where, albumArtist+" = ?")
		args = append(args, f.Artist)
	}
	if f.Search != "" {
		where = append(where, "(i.album LIKE ? OR "+albumArtist+" LIKE ? OR i.title LIKE ?)")
		like := "%" + f.Search + "%"
		args = append(args, like, like, like)
	}
	if f.Genre != "" {
		where = append(where, "('|' || IFNULL(i.genres,'') || '|') LIKE ?")
		args = append(args, "%|"+f.Genre+"|%")
	}
	limit := 5000
	if f.Limit > 0 && f.Limit < limit {
		limit = f.Limit
	}
	order := "i2 COLLATE NOCASE"
	plays := "0"
	switch f.Sort {
	case "artist":
		order = "a COLLATE NOCASE, y"
	case "year":
		order = "y DESC"
	case "added":
		order = "added DESC"
	case "played":
		// Each album's listening by this person, whichever tracks it was.
		plays = "IFNULL(SUM((SELECT st.seconds FROM item_stats st WHERE st.user_id = ? AND st.item_id = i.id)), 0)"
		args = append([]any{f.UserID}, args...)
		order = "plays DESC, added DESC"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+albumArtist+` AS a, IFNULL(i.album, '') AS i2, MIN(i.year) AS y, COUNT(*), SUM(IFNULL(i.runtime_sec, 0)),
		MIN(CASE WHEN IFNULL(i.poster, '') <> '' THEN i.id END), MAX(i.added_at) AS added, MAX(IFNULL(i.genres, '')), `+plays+` AS plays,
		MIN(printf('%04d%06d%019d', IFNULL(i.disc_no, 1), IFNULL(i.track, 999999), i.id))
		FROM items i WHERE `+strings.Join(where, " AND ")+` GROUP BY a, i2 ORDER BY `+order+` LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Album{}
	for rows.Next() {
		var al Album
		var year, cover sql.NullInt64
		var genres string
		var played float64
		var first sql.NullString
		if err := rows.Scan(&al.Artist, &al.Title, &year, &al.Tracks, &al.DurationSec, &cover, &al.AddedAt, &genres, &played, &first); err != nil {
			return nil, err
		}
		al.Year, al.CoverItem = nullInt(year), cover.Int64
		// The last 19 digits of the smallest disc+track+id key are the first track's id.
		if k := first.String; len(k) >= 19 {
			al.FirstItem, _ = strconv.ParseInt(k[len(k)-19:], 10, 64)
		}
		if g := splitList(genres); len(g) > 0 {
			al.Genre = g[0]
		}
		out = append(out, al)
	}
	return out, rows.Err()
}

// Artists lists the album artists (or authors) of a library.
func (s *Store) Artists(ctx context.Context, libraryID int64, access *Access) ([]Artist, error) {
	where, args := s.audioWhere(libraryID, access)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+albumArtist+` AS a, COUNT(DISTINCT IFNULL(i.album, '')), COUNT(*),
		MIN(CASE WHEN IFNULL(i.poster, '') <> '' THEN i.id END)
		FROM items i WHERE `+strings.Join(where, " AND ")+` GROUP BY a ORDER BY a COLLATE NOCASE LIMIT 5000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Artist{}
	for rows.Next() {
		var ar Artist
		var cover sql.NullInt64
		if err := rows.Scan(&ar.Name, &ar.Albums, &ar.Tracks, &cover); err != nil {
			return nil, err
		}
		ar.CoverItem = cover.Int64
		out = append(out, ar)
	}
	return out, rows.Err()
}

// audioPlaysDirect: a browser plays this sound file as it is. Everything else is
// converted to AAC while streaming.
func audioPlaysDirect(it *Item) bool {
	// A track of a CD comes as plain samples, and a podcast from wherever it lives: there
	// is no file here to look at, only what the sound is.
	if it.IsCDTrack() || it.Container == "" || strings.Contains(it.Path, "://") {
		c := mediatypes.CodecByID(it.VideoCodec)
		return c != nil && c.Browser
	}
	return mediatypes.AudioPlaysDirect(mediatypes.Canonical(it.Container, it.Path, "", it.VideoCodec), it.VideoCodec)
}

// PlaysDirect is audioPlaysDirect for the server's handlers.
func PlaysDirect(it *Item) bool { return audioPlaysDirect(it) }

// MusicQuery asks for songs in a way a plain list of items can't: the order and the
// shelf depend on who is listening.
type MusicQuery struct {
	LibraryID int64
	UserID    int64
	Artist    string
	Genre     string
	Search    string
	// View: "" every song, liked, played (most played), recent (played lately), added.
	View string
	// Sort: title | artist | album | year | added | runtime | random. A view has its own.
	Sort   string
	Desc   bool
	Limit  int
	Offset int
}

// MusicTracks answers a MusicQuery. It works out which and in what order first, then
// reads those items, so a long library costs one simple query.
func (s *Store) MusicTracks(ctx context.Context, q MusicQuery, access *Access) ([]*Item, error) {
	where, args := s.audioWhere(q.LibraryID, access)
	join := ""
	var joinArgs []any
	if q.Artist != "" {
		where = append(where, "("+albumArtist+" = ? OR i.artist = ?)")
		args = append(args, q.Artist, q.Artist)
	}
	if q.Genre != "" {
		where = append(where, "('|' || IFNULL(i.genres,'') || '|') LIKE ?")
		args = append(args, "%|"+q.Genre+"|%")
	}
	if q.Search != "" {
		like := "%" + q.Search + "%"
		where = append(where, "(i.title LIKE ? OR i.artist LIKE ? OR i.album LIKE ?)")
		args = append(args, like, like, like)
	}
	// Which way applies to what's being sorted by: songs of one album stay in album order.
	way := ""
	if q.Desc {
		way = " DESC"
	}
	order := "i.sort_title COLLATE NOCASE" + way
	switch q.Sort {
	case "artist":
		order = "IFNULL(i.artist, '') COLLATE NOCASE" + way + ", IFNULL(i.album, '') COLLATE NOCASE, IFNULL(i.disc_no, 1), IFNULL(i.track, 9999)"
	case "album":
		order = "IFNULL(i.album, '') COLLATE NOCASE" + way + ", IFNULL(i.disc_no, 1), IFNULL(i.track, 9999)"
	case "year":
		order = "IFNULL(i.year, 0)" + way
	case "added":
		order = "i.added_at" + way
	case "runtime":
		order = "IFNULL(i.runtime_sec, 0)" + way
	case "random":
		order = "RANDOM()"
	}
	switch q.View {
	case "liked":
		join = " JOIN item_reactions r ON r.item_id = i.id AND r.user_id = ? AND r.value = 1"
		joinArgs = append(joinArgs, q.UserID)
		if q.Sort == "" {
			order = "r.created_at DESC"
		}
	case "played":
		join = " JOIN item_stats st ON st.item_id = i.id AND st.user_id = ?"
		joinArgs = append(joinArgs, q.UserID)
		where = append(where, "st.seconds > 0")
		order = "st.plays DESC, st.seconds DESC"
	case "recent":
		join = " JOIN item_stats st ON st.item_id = i.id AND st.user_id = ?"
		joinArgs = append(joinArgs, q.UserID)
		where = append(where, "st.seconds > 0")
		order = "st.last_at DESC"
	case "added":
		order = "i.added_at DESC"
	}
	limit := q.Limit
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	return s.tracksInOrder(ctx, `SELECT i.id FROM items i`+join+` WHERE `+strings.Join(where, " AND ")+` ORDER BY `+order+`, i.id LIMIT ? OFFSET ?`,
		append(append(joinArgs, args...), limit, q.Offset), access)
}

// tracksInOrder reads the items a query of ids names, in that query's order.
func (s *Store) tracksInOrder(ctx context.Context, query string, args []any, access *Access) ([]*Item, error) {
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*Item{}, nil
	}
	got, _, err := s.Items(ctx, ItemQuery{IDs: ids, Kind: "audio", Brief: true, NoTotal: true, Access: access, Limit: len(ids)})
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*Item, len(got))
	for _, it := range got {
		byID[it.ID] = it
	}
	out := make([]*Item, 0, len(got))
	for _, id := range ids {
		if it := byID[id]; it != nil {
			out = append(out, it)
		}
	}
	return out, nil
}

// LikedTracks is the ids of the songs someone has liked in a library, or in all of
// them with libraryID 0, so a heart can be drawn without asking for each song.
func (s *Store) LikedTracks(ctx context.Context, userID, libraryID int64, access *Access) ([]int64, error) {
	clause, cargs := access.ItemClause()
	rows, err := s.DB.QueryContext(ctx, `SELECT r.item_id FROM item_reactions r JOIN items i ON i.id = r.item_id
		WHERE r.user_id = ? AND r.value = 1 AND i.kind = 'audio' AND i.missing = 0 AND (? = 0 OR i.library_id = ?) AND `+clause+` LIMIT 20000`,
		append([]any{userID, libraryID, libraryID}, cargs...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// RadioFrom makes a station from a song, out of this library alone: the song first,
// then others scored by how close they are, with enough chance in the score that two
// stations from the same song differ. Songs this person disliked are left out.
func (s *Store) RadioFrom(ctx context.Context, seed *Item, userID int64, limit int, access *Access) ([]*Item, error) {
	where, args := s.audioWhere(seed.LibraryID, access)
	where = append(where, "i.id <> ?", "i.id NOT IN (SELECT item_id FROM item_reactions WHERE user_id = ? AND value = -1)")
	args = append(args, seed.ID, userID)
	genre := ""
	if len(seed.Genres) > 0 {
		genre = "%|" + seed.Genres[0] + "|%"
	}
	year := 0
	if seed.Year != nil {
		year = *seed.Year
	}
	score := `((IFNULL(i.artist, '') = ?) * 4 + (` + albumArtist + ` = ?) * 2 + (? <> '' AND ('|' || IFNULL(i.genres,'') || '|') LIKE ?) * 3
		+ (? > 0 AND i.year IS NOT NULL AND ABS(i.year - ?) <= 5) * 2 + (ABS(RANDOM()) % 500) / 100.0)`
	all := append(append([]any{}, args...), seed.Artist, seed.Artist, genre, genre, year, year, limit-1)
	list, err := s.tracksInOrder(ctx, `SELECT i.id FROM items i WHERE `+strings.Join(where, " AND ")+` ORDER BY `+score+` DESC LIMIT ?`, all, access)
	if err != nil {
		return nil, err
	}
	return append([]*Item{seed}, list...), nil
}

// MusicCounts is the size of a library, for its page.
func (s *Store) MusicCounts(ctx context.Context, libraryID, userID int64, access *Access) (albums, artists, songs, liked int, err error) {
	where, args := s.audioWhere(libraryID, access)
	w := strings.Join(where, " AND ")
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT `+albumArtist+` || char(0) || IFNULL(i.album, '')), COUNT(DISTINCT `+albumArtist+`), COUNT(*),
		COUNT(DISTINCT (SELECT r.item_id FROM item_reactions r WHERE r.user_id = ? AND r.item_id = i.id AND r.value = 1)) FROM items i WHERE `+w,
		append([]any{userID}, args...)...).Scan(&albums, &artists, &songs, &liked)
	return
}

// InProgress is what someone has started and not finished in a library of books or
// podcasts, newest first.
func (s *Store) InProgress(ctx context.Context, libraryID, userID int64, limit int, access *Access) ([]*Item, error) {
	where, args := s.audioWhere(libraryID, access)
	return s.tracksInOrder(ctx, `SELECT i.id FROM items i JOIN progress p ON p.item_id = i.id AND p.user_id = ?
		WHERE `+strings.Join(where, " AND ")+` AND p.position_sec > 5 AND p.watched = 0 ORDER BY p.updated_at DESC LIMIT ?`,
		append(append([]any{userID}, args...), limit), access)
}
