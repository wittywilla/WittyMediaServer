package library

import (
	"context"
	"database/sql"
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
	AddedAt     int64   `json:"addedAt"`
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

// Albums lists a library's albums (or books). sort: title | artist | year | added.
func (s *Store) Albums(ctx context.Context, libraryID int64, artist, search, sort string, access *Access) ([]Album, error) {
	where, args := s.audioWhere(libraryID, access)
	if artist != "" {
		where = append(where, albumArtist+" = ?")
		args = append(args, artist)
	}
	if search != "" {
		where = append(where, "(i.album LIKE ? OR "+albumArtist+" LIKE ? OR i.title LIKE ?)")
		like := "%" + search + "%"
		args = append(args, like, like, like)
	}
	order := "i2 COLLATE NOCASE"
	switch sort {
	case "artist":
		order = "a COLLATE NOCASE, y"
	case "year":
		order = "y DESC"
	case "added":
		order = "added DESC"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+albumArtist+` AS a, IFNULL(i.album, '') AS i2, MIN(i.year) AS y, COUNT(*), SUM(IFNULL(i.runtime_sec, 0)),
		MIN(CASE WHEN IFNULL(i.poster, '') <> '' THEN i.id END), MAX(i.added_at) AS added
		FROM items i WHERE `+strings.Join(where, " AND ")+` GROUP BY a, i2 ORDER BY `+order+` LIMIT 5000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Album{}
	for rows.Next() {
		var al Album
		var year, cover sql.NullInt64
		if err := rows.Scan(&al.Artist, &al.Title, &year, &al.Tracks, &al.DurationSec, &cover, &al.AddedAt); err != nil {
			return nil, err
		}
		al.Year, al.CoverItem = nullInt(year), cover.Int64
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

// DirectAudio lists the codecs every current browser plays without help;
// everything else is converted to AAC while streaming. Keep in step with
// web/src/audio/player.tsx.
var DirectAudio = map[string]bool{"mp3": true, "aac": true, "flac": true, "opus": true, "vorbis": true,
	"pcm_s16le": true, "pcm_s24le": true, "pcm_u8": true, "pcm_f32le": true}
