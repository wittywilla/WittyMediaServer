// Package library owns libraries, media items, scanning and metadata.
package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

type Library struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Type       string   `json:"type"` // movies | shows | videos
	Paths      []string `json:"paths"`
	CreatedAt  int64    `json:"createdAt"`
	LastScanAt *int64   `json:"lastScanAt"`
	Scanning   bool     `json:"scanning"`
	ItemCount  int      `json:"itemCount"`
}

type Item struct {
	ID            int64     `json:"id"`
	LibraryID     int64     `json:"libraryId"`
	Kind          string    `json:"kind"` // video | dvd-iso | dvd-folder
	Path          string    `json:"path"`
	Title         string    `json:"title"`
	SortTitle     string    `json:"sortTitle"`
	Year          *int      `json:"year"`
	Overview      string    `json:"overview"`
	Tagline       string    `json:"tagline"`
	Genres        []string  `json:"genres"`
	Rating        *float64  `json:"rating"`
	ContentRating string    `json:"contentRating"`
	Studio        string    `json:"studio"`
	RuntimeSec    float64   `json:"runtimeSec"`
	Poster        string    `json:"poster"`   // cache-relative path or URL
	Backdrop      string    `json:"backdrop"` // cache-relative path or URL
	Container     string    `json:"container"`
	VideoCodec    string    `json:"videoCodec"`
	Width         int       `json:"width"`
	Height        int       `json:"height"`
	AudioLangs    []string  `json:"audioLangs"`
	SubLangs      []string  `json:"subLangs"`
	Disc          *DiscInfo `json:"disc,omitempty"`
	Provider      string    `json:"provider"`
	ProviderID    string    `json:"providerId"`
	SizeBytes     int64     `json:"sizeBytes"`
	MTime         int64     `json:"mtime"`
	AddedAt       int64     `json:"addedAt"`
	UpdatedAt     int64     `json:"updatedAt"`
	Missing       bool      `json:"missing"`
	// TV episodes (shows libraries):
	ShowID     *int64 `json:"showId,omitempty"`
	ShowTitle  string `json:"showTitle,omitempty"`
	Season     *int   `json:"season,omitempty"`
	Episode    *int   `json:"episode,omitempty"`
	EpisodeEnd *int   `json:"episodeEnd,omitempty"` // multi-episode files (S02E15-E16)
	Aired      string `json:"aired,omitempty"`
	NextID     *int64 `json:"nextId,omitempty"` // next episode, filled by the item endpoint
	// Per-item viewer preferences (set from the item's settings, never by scans):
	PlayMode string `json:"playMode"` // DVDs: menu | title | "" (server default)
	InsertFX string `json:"insertFx"` // DVDs: on | off | "" (server default)
	// Intro range for "Skip intro" (seconds); the item endpoint falls back to
	// the show's range.
	IntroStart *float64 `json:"introStart,omitempty"`
	IntroEnd   *float64 `json:"introEnd,omitempty"`
	// Filled per user:
	Progress *Progress `json:"progress,omitempty"`
}

// DiscInfo is the cached summary of a DVD.
type DiscInfo struct {
	Label     string      `json:"label"`
	Format    string      `json:"format"`
	HasMenus  bool        `json:"hasMenus"`
	MainTitle int         `json:"mainTitle"`
	Titles    []DiscTitle `json:"titles"`
	Scrambled bool        `json:"scrambled"`
}

type DiscTitle struct {
	Number      int       `json:"number"`
	DurationSec float64   `json:"durationSec"`
	Chapters    []float64 `json:"chapters"`
	Angles      int       `json:"angles"`
	Audio       []string  `json:"audio"`
	Subtitles   []string  `json:"subtitles"`
	Wide        bool      `json:"wide"`
	Standard    string    `json:"standard"`
}

type Progress struct {
	PositionSec float64 `json:"positionSec"`
	DurationSec float64 `json:"durationSec"`
	Watched     bool    `json:"watched"`
	DiscTitle   *int    `json:"discTitle,omitempty"`
	UpdatedAt   int64   `json:"updatedAt"`
}

type Store struct {
	DB *db.DB
}

var ErrNotFound = errors.New("not found")

// ---------------------------------------------------------------------------
// Libraries
// ---------------------------------------------------------------------------

func (s *Store) Libraries(ctx context.Context) ([]Library, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT l.id, l.name, l.type, l.created_at, l.last_scan_at, l.scanning,
		(SELECT COUNT(*) FROM items i WHERE i.library_id = l.id AND i.missing = 0) FROM libraries l ORDER BY l.name`)
	if err != nil {
		return nil, err
	}
	var out []Library
	for rows.Next() {
		var l Library
		var scanning int
		if err := rows.Scan(&l.ID, &l.Name, &l.Type, &l.CreatedAt, &l.LastScanAt, &scanning, &l.ItemCount); err != nil {
			rows.Close()
			return nil, err
		}
		l.Scanning = scanning != 0
		out = append(out, l)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Fetch paths after the cursor is closed so we never hold two connections.
	for i := range out {
		out[i].Paths, _ = s.libraryPaths(ctx, out[i].ID)
	}
	if out == nil {
		out = []Library{}
	}
	return out, nil
}

func (s *Store) libraryPaths(ctx context.Context, id int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT path FROM library_paths WHERE library_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) Library(ctx context.Context, id int64) (*Library, error) {
	libs, err := s.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	for i := range libs {
		if libs[i].ID == id {
			return &libs[i], nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) CreateLibrary(ctx context.Context, name, typ string, paths []string) (*Library, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO libraries(name, type, created_at) VALUES(?, ?, ?)`, name, typ, db.Now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	for _, p := range paths {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO library_paths(library_id, path) VALUES(?, ?)`, id, p); err != nil {
			return nil, err
		}
	}
	return s.Library(ctx, id)
}

func (s *Store) UpdateLibrary(ctx context.Context, id int64, name, typ string, paths []string) (*Library, error) {
	if _, err := s.DB.ExecContext(ctx, `UPDATE libraries SET name = ?, type = ? WHERE id = ?`, name, typ, id); err != nil {
		return nil, err
	}
	if paths != nil {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM library_paths WHERE library_id = ?`, id); err != nil {
			return nil, err
		}
		for _, p := range paths {
			if _, err := s.DB.ExecContext(ctx, `INSERT INTO library_paths(library_id, path) VALUES(?, ?)`, id, p); err != nil {
				return nil, err
			}
		}
	}
	return s.Library(ctx, id)
}

func (s *Store) DeleteLibrary(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM libraries WHERE id = ?`, id)
	return err
}

func (s *Store) setScanning(ctx context.Context, id int64, on bool) {
	if on {
		_, _ = s.DB.ExecContext(ctx, `UPDATE libraries SET scanning = 1 WHERE id = ?`, id)
	} else {
		_, _ = s.DB.ExecContext(ctx, `UPDATE libraries SET scanning = 0, last_scan_at = ? WHERE id = ?`, db.Now(), id)
	}
}

// ---------------------------------------------------------------------------
// Items
// ---------------------------------------------------------------------------

const itemCols = `i.id, i.library_id, i.kind, i.path, i.title, i.sort_title, i.year, i.overview, i.tagline, i.genres, i.rating,
	i.content_rating, i.studio, i.runtime_sec, i.poster, i.backdrop, i.container, i.video_codec, i.width, i.height,
	i.audio_langs, i.sub_langs, i.disc_json, i.provider, i.provider_id, i.size_bytes, i.mtime, i.added_at, i.updated_at, i.missing,
	i.show_id, (SELECT s.title FROM shows s WHERE s.id = i.show_id), i.season, i.episode, i.episode_end, i.aired,
	IFNULL(i.play_mode, ''), IFNULL(i.insert_fx, ''), i.intro_start, i.intro_end`

func scanItem(sc interface{ Scan(...any) error }) (*Item, error) {
	var it Item
	var overview, tagline, genres, contentRating, studio, poster, backdrop, container, vcodec, audio, subs, discJSON, provider, providerID sql.NullString
	var year sql.NullInt64
	var rating, runtime sql.NullFloat64
	var width, height, size, mtime sql.NullInt64
	var missing int
	var showID, season, episode, episodeEnd sql.NullInt64
	var showTitle, aired sql.NullString
	var introStart, introEnd sql.NullFloat64
	err := sc.Scan(&it.ID, &it.LibraryID, &it.Kind, &it.Path, &it.Title, &it.SortTitle, &year, &overview, &tagline, &genres, &rating,
		&contentRating, &studio, &runtime, &poster, &backdrop, &container, &vcodec, &width, &height,
		&audio, &subs, &discJSON, &provider, &providerID, &size, &mtime, &it.AddedAt, &it.UpdatedAt, &missing,
		&showID, &showTitle, &season, &episode, &episodeEnd, &aired, &it.PlayMode, &it.InsertFX, &introStart, &introEnd)
	if err != nil {
		return nil, err
	}
	if introStart.Valid && introEnd.Valid {
		it.IntroStart, it.IntroEnd = &introStart.Float64, &introEnd.Float64
	}
	if showID.Valid {
		it.ShowID = &showID.Int64
	}
	it.ShowTitle, it.Aired = showTitle.String, aired.String
	it.Season, it.Episode, it.EpisodeEnd = nullInt(season), nullInt(episode), nullInt(episodeEnd)
	if year.Valid {
		y := int(year.Int64)
		it.Year = &y
	}
	if rating.Valid {
		it.Rating = &rating.Float64
	}
	it.Overview, it.Tagline, it.ContentRating, it.Studio = overview.String, tagline.String, contentRating.String, studio.String
	it.Poster, it.Backdrop, it.Container, it.VideoCodec = poster.String, backdrop.String, container.String, vcodec.String
	it.Provider, it.ProviderID = provider.String, providerID.String
	it.RuntimeSec = runtime.Float64
	it.Width, it.Height = int(width.Int64), int(height.Int64)
	it.SizeBytes, it.MTime = size.Int64, mtime.Int64
	it.Missing = missing != 0
	it.Genres = splitList(genres.String)
	it.AudioLangs = splitList(audio.String)
	it.SubLangs = splitList(subs.String)
	if discJSON.Valid && discJSON.String != "" {
		var d DiscInfo
		if json.Unmarshal([]byte(discJSON.String), &d) == nil {
			it.Disc = &d
		}
	}
	return &it, nil
}

func nullInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

func splitList(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, "|")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func joinList(l []string) string { return strings.Join(l, "|") }

// ItemQuery filters and sorts items.
type ItemQuery struct {
	LibraryID int64
	Search    string
	Sort      string // title | added | year | rating | runtime
	Desc      bool
	Limit     int
	Offset    int
	Genre     string
	Kind      string
	ShowID    int64
	// NoEpisodes leaves TV episodes out (home hero, movie views).
	NoEpisodes bool
}

func (s *Store) Items(ctx context.Context, q ItemQuery) ([]*Item, int, error) {
	where := []string{"i.missing = 0"}
	var args []any
	if q.LibraryID != 0 {
		where = append(where, "i.library_id = ?")
		args = append(args, q.LibraryID)
	}
	if q.Search != "" {
		where = append(where, "(i.title LIKE ? OR i.overview LIKE ?)")
		like := "%" + q.Search + "%"
		args = append(args, like, like)
	}
	if q.Genre != "" {
		where = append(where, "('|' || IFNULL(i.genres,'') || '|') LIKE ?")
		args = append(args, "%|"+q.Genre+"|%")
	}
	if q.Kind != "" {
		where = append(where, "i.kind = ?")
		args = append(args, q.Kind)
	}
	if q.NoEpisodes {
		where = append(where, "i.show_id IS NULL")
	}
	if q.ShowID != 0 {
		where = append(where, "i.show_id = ?")
		args = append(args, q.ShowID)
	}
	order := "i.sort_title COLLATE NOCASE"
	switch q.Sort {
	case "added":
		order = "i.added_at"
	case "year":
		order = "IFNULL(i.year, 0)"
	case "rating":
		order = "IFNULL(i.rating, 0)"
	case "runtime":
		order = "IFNULL(i.runtime_sec, 0)"
	case "updated":
		order = "i.updated_at"
	case "episode":
		order = "IFNULL(i.season, 0), IFNULL(i.episode, 0)"
	}
	if q.Desc {
		order += " DESC"
	}
	order += ", i.id"
	w := strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM items i WHERE `+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 10000
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+itemCols+` FROM items i WHERE `+w+` ORDER BY `+order+` LIMIT ? OFFSET ?`, append(args, limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

func (s *Store) Item(ctx context.Context, id int64) (*Item, error) {
	it, err := scanItem(s.DB.QueryRowContext(ctx, `SELECT `+itemCols+` FROM items i WHERE i.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return it, err
}

func (s *Store) itemByPath(ctx context.Context, libraryID int64, path string) (*Item, error) {
	it, err := scanItem(s.DB.QueryRowContext(ctx, `SELECT `+itemCols+` FROM items i WHERE i.library_id = ? AND i.path = ?`, libraryID, path))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return it, err
}

func (s *Store) upsertItem(ctx context.Context, it *Item) error {
	var discJSON *string
	if it.Disc != nil {
		b, _ := json.Marshal(it.Disc)
		str := string(b)
		discJSON = &str
	}
	now := db.Now()
	if it.ID == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, year, overview, tagline, genres, rating,
			content_rating, studio, runtime_sec, poster, backdrop, container, video_codec, width, height, audio_langs, sub_langs, disc_json,
			provider, provider_id, size_bytes, mtime, added_at, updated_at, missing, show_id, season, episode, episode_end, aired, intro_start, intro_end)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,?,?,?,?,?,?,?)`,
			it.LibraryID, it.Kind, it.Path, it.Title, it.SortTitle, it.Year, it.Overview, it.Tagline, joinList(it.Genres), it.Rating,
			it.ContentRating, it.Studio, it.RuntimeSec, it.Poster, it.Backdrop, it.Container, it.VideoCodec, it.Width, it.Height,
			joinList(it.AudioLangs), joinList(it.SubLangs), discJSON, it.Provider, it.ProviderID, it.SizeBytes, it.MTime, now, now,
			it.ShowID, it.Season, it.Episode, it.EpisodeEnd, it.Aired, it.IntroStart, it.IntroEnd)
		if err != nil {
			return err
		}
		it.ID, _ = res.LastInsertId()
		it.AddedAt, it.UpdatedAt = now, now
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE items SET kind=?, title=?, sort_title=?, year=?, overview=?, tagline=?, genres=?, rating=?,
		content_rating=?, studio=?, runtime_sec=?, poster=?, backdrop=?, container=?, video_codec=?, width=?, height=?, audio_langs=?, sub_langs=?,
		disc_json=?, provider=?, provider_id=?, size_bytes=?, mtime=?, updated_at=?, missing=0,
		show_id=?, season=?, episode=?, episode_end=?, aired=?, intro_start=?, intro_end=? WHERE id=?`,
		it.Kind, it.Title, it.SortTitle, it.Year, it.Overview, it.Tagline, joinList(it.Genres), it.Rating,
		it.ContentRating, it.Studio, it.RuntimeSec, it.Poster, it.Backdrop, it.Container, it.VideoCodec, it.Width, it.Height,
		joinList(it.AudioLangs), joinList(it.SubLangs), discJSON, it.Provider, it.ProviderID, it.SizeBytes, it.MTime, now,
		it.ShowID, it.Season, it.Episode, it.EpisodeEnd, it.Aired, it.IntroStart, it.IntroEnd, it.ID)
	it.UpdatedAt = now
	return err
}

// SetItemPrefs stores per-item viewer preferences.
func (s *Store) SetItemPrefs(ctx context.Context, id int64, playMode, insertFX string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE items SET play_mode = ?, insert_fx = ? WHERE id = ?`, playMode, insertFX, id)
	return err
}

// UpdateItemMeta applies user edits.
func (s *Store) UpdateItem(ctx context.Context, it *Item) error { return s.upsertItem(ctx, it) }

func (s *Store) markMissing(ctx context.Context, libraryID int64, keep map[string]bool) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, path FROM items WHERE library_id = ?`, libraryID)
	if err != nil {
		return err
	}
	var gone []int64
	for rows.Next() {
		var id int64
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			rows.Close()
			return err
		}
		if !keep[p] {
			gone = append(gone, id)
		}
	}
	rows.Close()
	for _, id := range gone {
		if _, err := s.DB.ExecContext(ctx, `UPDATE items SET missing = 1 WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Genres(ctx context.Context, libraryID int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT genres FROM items WHERE missing = 0 AND (? = 0 OR library_id = ?)`, libraryID, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[string]bool{}
	for rows.Next() {
		var g sql.NullString
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		for _, x := range splitList(g.String) {
			set[x] = true
		}
	}
	out := make([]string, 0, len(set))
	for g := range set {
		out = append(out, g)
	}
	sortStrings(out)
	return out, nil
}

// ---------------------------------------------------------------------------
// Progress
// ---------------------------------------------------------------------------

func (s *Store) SetProgress(ctx context.Context, userID, itemID int64, p Progress) error {
	watched := 0
	if p.Watched || (p.DurationSec > 0 && p.PositionSec/p.DurationSec > 0.92) {
		watched = 1
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO progress(user_id, item_id, position_sec, duration_sec, watched, disc_title, updated_at)
		VALUES(?,?,?,?,?,?,?) ON CONFLICT(user_id, item_id) DO UPDATE SET position_sec = excluded.position_sec,
		duration_sec = excluded.duration_sec, watched = excluded.watched, disc_title = excluded.disc_title, updated_at = excluded.updated_at`,
		userID, itemID, p.PositionSec, p.DurationSec, watched, p.DiscTitle, db.Now())
	return err
}

func (s *Store) ClearProgress(ctx context.Context, userID, itemID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM progress WHERE user_id = ? AND item_id = ?`, userID, itemID)
	return err
}

func (s *Store) AttachProgress(ctx context.Context, userID int64, items []*Item) error {
	if len(items) == 0 || userID == 0 {
		return nil
	}
	byID := map[int64]*Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT item_id, position_sec, duration_sec, watched, disc_title, updated_at FROM progress WHERE user_id = ?`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var p Progress
		var watched int
		var dt sql.NullInt64
		if err := rows.Scan(&id, &p.PositionSec, &p.DurationSec, &watched, &dt, &p.UpdatedAt); err != nil {
			return err
		}
		p.Watched = watched != 0
		if dt.Valid {
			n := int(dt.Int64)
			p.DiscTitle = &n
		}
		if it := byID[id]; it != nil {
			pp := p
			it.Progress = &pp
		}
	}
	return rows.Err()
}

// ContinueWatching lists in-progress items for a user.
func (s *Store) ContinueWatching(ctx context.Context, userID int64, limit int) ([]*Item, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+itemCols+` FROM items i JOIN progress p ON p.item_id = i.id
		WHERE p.user_id = ? AND p.watched = 0 AND p.position_sec > 60 AND i.missing = 0 ORDER BY p.updated_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	out := []*Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, it)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, s.AttachProgress(ctx, userID, out)
}
