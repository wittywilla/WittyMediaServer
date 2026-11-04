package music

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Link is a song and a music video, tied or suggested.
type Link struct {
	SongID  int64  `json:"songId"`
	VideoID int64  `json:"videoId"`
	State   string `json:"state"` // tied | suggested
	How     string `json:"how"`   // isrc | recording | name | close | hand
	// SyncMS is where the song's start is in the video, so playback can switch between
	// them at the same place. Missing when they don't line up.
	SyncMS *int64 `json:"syncMs,omitempty"`
	By     string `json:"by,omitempty"` // who tied it, when someone did
}

// ErrNotPair: the first isn't a song of a music library, or the second isn't a video of
// a music video library.
var ErrNotPair = errors.New("that isn't a song and a music video")

// Links keeps the ties between songs and videos, in the server's database.
type Links struct{ DB *sql.DB }

// SaveIDs keeps what a file's tags say identifies its recording. Called by scans.
func (l *Links) SaveIDs(ctx context.Context, itemID int64, isrc, mbid string) error {
	isrc, mbid = CleanISRC(isrc), CleanMBID(mbid)
	if isrc == "" && mbid == "" {
		_, err := l.DB.ExecContext(ctx, `DELETE FROM music_recording_ids WHERE item_id = ?`, itemID)
		return err
	}
	_, err := l.DB.ExecContext(ctx, `INSERT INTO music_recording_ids(item_id, isrc, recording) VALUES(?,?,?)
		ON CONFLICT(item_id) DO UPDATE SET isrc = excluded.isrc, recording = excluded.recording`, itemID, isrc, mbid)
	return err
}

// Result is what a round of matching came to.
type Result struct {
	Tied      int `json:"tied"`
	Suggested int `json:"suggested"`
}

// Rematch matches every song with every music video again. What someone tied or untied
// stays as it is; the rest is replaced with this round's answer.
func (l *Links) Rematch(ctx context.Context) (Result, error) {
	songs, err := l.recordings(ctx, `SELECT i.id, i.title, IFNULL(i.artist, ''), IFNULL(i.album_artist, ''), IFNULL(r.isrc, ''), IFNULL(r.recording, ''), IFNULL(i.runtime_sec, 0)
		FROM items i JOIN libraries b ON b.id = i.library_id LEFT JOIN music_recording_ids r ON r.item_id = i.id
		WHERE b.type = 'music' AND i.kind = 'audio' AND i.missing = 0`)
	if err != nil {
		return Result{}, err
	}
	videos, err := l.recordings(ctx, `SELECT i.id, i.title, IFNULL(i.studio, ''), '', IFNULL(r.isrc, ''), IFNULL(r.recording, ''), IFNULL(i.runtime_sec, 0)
		FROM items i JOIN libraries b ON b.id = i.library_id LEFT JOIN music_recording_ids r ON r.item_id = i.id
		WHERE b.type = 'musicvideos' AND i.kind = 'video' AND i.missing = 0`)
	if err != nil {
		return Result{}, err
	}
	type key struct{ song, video int64 }
	untied := map[key]bool{}
	handled := map[int64]bool{} // videos someone tied: nothing else is looked for
	rows, err := l.DB.QueryContext(ctx, `SELECT song_id, video_id, state FROM music_video_links WHERE by <> ''`)
	if err != nil {
		return Result{}, err
	}
	for rows.Next() {
		var k key
		var state string
		if err := rows.Scan(&k.song, &k.video, &state); err != nil {
			rows.Close()
			return Result{}, err
		}
		if state == "tied" {
			handled[k.video] = true
		} else {
			untied[k] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	open := videos[:0]
	for _, v := range videos {
		if !handled[v.ID] {
			open = append(open, v)
		}
	}
	pairs := Match(songs, open, func(s, v int64) bool { return untied[key{s, v}] })

	tx, err := l.DB.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM music_video_links WHERE by = ''`); err != nil {
		return Result{}, err
	}
	ins, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO music_video_links(song_id, video_id, state, how, sync_ms, by, created_at, updated_at) VALUES(?,?,?,?,?,'',?,?)`)
	if err != nil {
		return Result{}, err
	}
	defer ins.Close()
	now := time.Now().Unix()
	var res Result
	for _, p := range pairs {
		state := "suggested"
		if p.Sure {
			state = "tied"
			res.Tied++
		} else {
			res.Suggested++
		}
		if _, err := ins.ExecContext(ctx, p.SongID, p.VideoID, state, p.How, p.SyncMS, now, now); err != nil {
			return Result{}, err
		}
	}
	return res, tx.Commit()
}

func (l *Links) recordings(ctx context.Context, q string) ([]Recording, error) {
	rows, err := l.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Recording
	for rows.Next() {
		var r Recording
		var a, aa string
		if err := rows.Scan(&r.ID, &r.Title, &a, &aa, &r.ISRC, &r.MBID, &r.Seconds); err != nil {
			return nil, err
		}
		for _, n := range []string{a, aa} {
			if n != "" {
				r.Artists = append(r.Artists, n)
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const linkCols = `song_id, video_id, state, how, sync_ms, by`

func scanLinks(rows *sql.Rows, err error) ([]Link, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Link{}
	for rows.Next() {
		var k Link
		var sync sql.NullInt64
		if err := rows.Scan(&k.SongID, &k.VideoID, &k.State, &k.How, &sync, &k.By); err != nil {
			return nil, err
		}
		if sync.Valid {
			v := sync.Int64
			k.SyncMS = &v
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// Of is every tie and suggestion an item is in, as the song or the video: ties first.
func (l *Links) Of(ctx context.Context, itemID int64) ([]Link, error) {
	return scanLinks(l.DB.QueryContext(ctx, `SELECT `+linkCols+` FROM music_video_links
		WHERE (song_id = ? OR video_id = ?) AND state IN ('tied', 'suggested')
		ORDER BY state = 'suggested', by = '', created_at, song_id, video_id`, itemID, itemID))
}

// Tied is every tie among these items, in one query, for marking lists.
func (l *Links) Tied(ctx context.Context, ids []int64) ([]Link, error) {
	if len(ids) == 0 {
		return []Link{}, nil
	}
	args := make([]any, 0, 2*len(ids))
	in := make([]byte, 0, 2*len(ids))
	for i, id := range ids {
		if i > 0 {
			in = append(in, ',')
		}
		in = append(in, '?')
		args = append(args, id)
	}
	for _, id := range ids {
		args = append(args, id)
	}
	return scanLinks(l.DB.QueryContext(ctx, `SELECT `+linkCols+` FROM music_video_links WHERE state = 'tied'
		AND (song_id IN (`+string(in)+`) OR video_id IN (`+string(in)+`)) ORDER BY by = '', created_at`, args...))
}

// Suggested is what an admin might want to look at, newest first.
func (l *Links) Suggested(ctx context.Context, limit int) ([]Link, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return scanLinks(l.DB.QueryContext(ctx, `SELECT `+linkCols+` FROM music_video_links WHERE state = 'suggested'
		ORDER BY created_at DESC, video_id, song_id LIMIT ?`, limit))
}

// Get is one pair's link, or nil.
func (l *Links) Get(ctx context.Context, songID, videoID int64) (*Link, error) {
	list, err := scanLinks(l.DB.QueryContext(ctx, `SELECT `+linkCols+` FROM music_video_links WHERE song_id = ? AND video_id = ?`, songID, videoID))
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

// pairOK checks the two are a song of a music library and a video of a music video library.
func (l *Links) pairOK(ctx context.Context, tx *sql.Tx, songID, videoID int64) error {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM items i JOIN libraries b ON b.id = i.library_id WHERE i.id = ? AND i.kind = 'audio' AND b.type = 'music') +
		(SELECT COUNT(*) FROM items i JOIN libraries b ON b.id = i.library_id WHERE i.id = ? AND i.kind = 'video' AND b.type = 'musicvideos')`, songID, videoID).Scan(&n)
	if err != nil {
		return err
	}
	if n != 2 {
		return ErrNotPair
	}
	return nil
}

// Tie ties a song to a video for good: a match never changes it. A video has one song,
// so whatever it was tied to before is untied, and stays so. syncMS sets where the song
// starts in the video; keepSync leaves it as it was.
func (l *Links) Tie(ctx context.Context, songID, videoID int64, syncMS *int64, keepSync bool, by string) error {
	if by == "" {
		by = "admin"
	}
	tx, err := l.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := l.pairOK(ctx, tx, songID, videoID); err != nil {
		return err
	}
	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx, `UPDATE music_video_links SET state = 'untied', by = ?, updated_at = ?
		WHERE video_id = ? AND song_id <> ? AND state = 'tied'`, by, now, videoID, songID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM music_video_links WHERE video_id = ? AND song_id <> ? AND state = 'suggested'`, videoID, songID); err != nil {
		return err
	}
	set := `state = 'tied', how = CASE WHEN how IN ('isrc', 'recording', 'name') THEN how ELSE 'hand' END, by = excluded.by, updated_at = excluded.updated_at`
	if !keepSync {
		set += `, sync_ms = excluded.sync_ms`
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO music_video_links(song_id, video_id, state, how, sync_ms, by, created_at, updated_at) VALUES(?,?,'tied','hand',?,?,?,?)
		ON CONFLICT(song_id, video_id) DO UPDATE SET `+set, songID, videoID, syncMS, by, now, now); err != nil {
		return err
	}
	return tx.Commit()
}

// Untie parts a song and a video, and remembers it, so no match ties them again.
func (l *Links) Untie(ctx context.Context, songID, videoID int64, by string) error {
	if by == "" {
		by = "admin"
	}
	tx, err := l.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := l.pairOK(ctx, tx, songID, videoID); err != nil {
		return err
	}
	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx, `INSERT INTO music_video_links(song_id, video_id, state, how, sync_ms, by, created_at, updated_at) VALUES(?,?,'untied','hand',NULL,?,?,?)
		ON CONFLICT(song_id, video_id) DO UPDATE SET state = 'untied', by = excluded.by, updated_at = excluded.updated_at`, songID, videoID, by, now, now); err != nil {
		return err
	}
	return tx.Commit()
}

// Candidates are items on the other side of a link to choose from by hand: videos for a
// song, songs for a video, whose title or artist has q in it. Nearest names first.
func (l *Links) Candidates(ctx context.Context, itemID int64, q string, limit int) ([]int64, error) {
	if limit <= 0 || limit > 50 {
		limit = 30
	}
	var libType string
	if err := l.DB.QueryRowContext(ctx, `SELECT b.type FROM items i JOIN libraries b ON b.id = i.library_id WHERE i.id = ?`, itemID).Scan(&libType); err != nil {
		return nil, err
	}
	other, kind, artist := "musicvideos", "video", "i.studio"
	switch libType {
	case "music":
	case "musicvideos":
		other, kind, artist = "music", "audio", "IFNULL(i.artist, '') || ' ' || IFNULL(i.album_artist, '')"
	default:
		return nil, ErrNotPair
	}
	like := "%" + strings.NewReplacer(`\`, `\`, "%", `\%`, "_", `\_`).Replace(strings.TrimSpace(q)) + "%"
	rows, err := l.DB.QueryContext(ctx, `SELECT i.id FROM items i JOIN libraries b ON b.id = i.library_id
		WHERE b.type = ? AND i.kind = ? AND i.missing = 0 AND (i.title LIKE ? ESCAPE '\' OR `+artist+` LIKE ? ESCAPE '\')
		ORDER BY i.sort_title, i.id LIMIT ?`, other, kind, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
