// Package library owns libraries, media items, scanning and metadata.
package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

type Library struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`              // movies | shows | anime | videos
	Console    string   `json:"console,omitempty"` // games libraries: a console, or auto
	Paths      []string `json:"paths"`
	CreatedAt  int64    `json:"createdAt"`
	LastScanAt *int64   `json:"lastScanAt"`
	Scanning   bool     `json:"scanning"`
	ItemCount  int      `json:"itemCount"`
	// ShowMain: a commercials library also appears in the menus, Home and
	// search. Off by default; channels can use it either way.
	ShowMain bool `json:"showMain"`
}

// CommercialsType is a library of the owner's own adverts and bumpers for
// channels. They are titles to schedule, not to browse, so they stay out of
// Home, search and everything else that looks across libraries.
const CommercialsType = "commercials"

type Item struct {
	ID            int64     `json:"id"`
	LibraryID     int64     `json:"libraryId"`
	Kind          string    `json:"kind"` // video | dvd-iso | dvd-folder | audio
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
	// StartMode and InsertEffect are what those settings come to with the
	// server's defaults applied: menu | title, and whether the insert effect
	// plays. Filled for discs by the item endpoints.
	StartMode    string `json:"startMode,omitempty"`
	InsertEffect *bool  `json:"insertEffect,omitempty"`
	// Intro range for "Skip intro" (seconds); the item endpoint falls back to
	// the show's range.
	IntroStart *float64 `json:"introStart,omitempty"`
	IntroEnd   *float64 `json:"introEnd,omitempty"`
	// Audio (music, audiobooks, podcasts), from the file's tags:
	Artist      string `json:"artist,omitempty"`
	Album       string `json:"album,omitempty"`       // album, or the book for audiobooks
	AlbumArtist string `json:"albumArtist,omitempty"` // or the author
	Track       *int   `json:"track,omitempty"`
	DiscNo      *int   `json:"discNo,omitempty"`
	// Books: "" / ltr, rtl (manga) or webtoon (vertical strip). For books
	// RuntimeSec is the length in pages (comics) or chapters (EPUB).
	Reading string `json:"reading,omitempty"`
	// Games: the console it runs on (wittyplay.Consoles).
	Console string `json:"console,omitempty"`
	// CD images: a track of a disc says which disc it is (kind audio, in a music
	// library). The disc itself is kind cd-image.
	CDDisc *int64 `json:"cdDisc,omitempty"`
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
	// Content: what the disc holds, "movie" or "tv". Worked out from the
	// library and the disc's name unless someone chose it (ContentManual).
	Content       string `json:"content,omitempty"`
	ContentManual bool   `json:"contentManual,omitempty"`
	// CD: an audio CD image's own facts (kind cd-image), left out of lists.
	CD *CDInfo `json:"cd,omitempty"`
}

// IsDiscKind: a DVD or Blu-ray image or folder.
func IsDiscKind(kind string) bool {
	return strings.HasPrefix(kind, "dvd-") || strings.HasPrefix(kind, "bluray-")
}

// GuessDiscContent: a disc in a TV library, or named like part of a series,
// holds TV episodes; anything else is taken to be a film.
func GuessDiscContent(libType, title, label string) string {
	if libType == "shows" || libType == "anime" || LooksLikeTVDisc(title) || LooksLikeTVDisc(label) {
		return "tv"
	}
	return "movie"
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
	// ResumeSec is where to pick up again, 0 to start over: the rule for
	// "Resume" that every client shares (see resumePoint).
	ResumeSec float64 `json:"resumeSec"`
}

// resumePoint: anything watched for less than a minute starts over, as does
// anything finished; books keep any position (they count pages or chapters), and
// so does a CD, whose place may be early in a track that is still playing.
func resumePoint(kind string, p *Progress) float64 {
	if p.Watched || p.PositionSec <= 0 || (kind != "book" && kind != CDKind && p.PositionSec <= 60) {
		return 0
	}
	return p.PositionSec
}

type Store struct {
	DB *db.DB
}

var ErrNotFound = errors.New("not found")

// ---------------------------------------------------------------------------
// Libraries
// ---------------------------------------------------------------------------

const libraryCols = `l.id, l.name, l.type, IFNULL(l.console, ''), l.created_at, l.last_scan_at, l.scanning,
	(SELECT COUNT(*) FROM items i WHERE i.library_id = l.id AND i.missing = 0), l.show_main`

func scanLibrary(sc interface{ Scan(...any) error }) (*Library, error) {
	var l Library
	var scanning, showMain int
	if err := sc.Scan(&l.ID, &l.Name, &l.Type, &l.Console, &l.CreatedAt, &l.LastScanAt, &scanning, &l.ItemCount, &showMain); err != nil {
		return nil, err
	}
	l.Scanning = scanning != 0
	l.ShowMain = showMain != 0
	l.Paths = []string{}
	return &l, nil
}

func (s *Store) Libraries(ctx context.Context) ([]Library, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+libraryCols+` FROM libraries l ORDER BY l.name`)
	if err != nil {
		return nil, err
	}
	out := []Library{}
	for rows.Next() {
		l, err := scanLibrary(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *l)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Fetch paths after the cursor is closed so we never hold two connections.
	byID := make(map[int64]*Library, len(out))
	for i := range out {
		byID[out[i].ID] = &out[i]
	}
	roots, err := s.DB.QueryContext(ctx, `SELECT library_id, path FROM library_paths ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer roots.Close()
	for roots.Next() {
		var id int64
		var p string
		if err := roots.Scan(&id, &p); err != nil {
			return nil, err
		}
		if l := byID[id]; l != nil {
			l.Paths = append(l.Paths, p)
		}
	}
	return out, roots.Err()
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

// LibraryRoots lists every library's folders (what artwork paths are checked
// against, for each picture a page shows).
func (s *Store) LibraryRoots(ctx context.Context) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT path FROM library_paths`)
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
	l, err := scanLibrary(s.DB.QueryRowContext(ctx, `SELECT `+libraryCols+` FROM libraries l WHERE l.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if l.Paths, err = s.libraryPaths(ctx, id); err != nil {
		return nil, err
	}
	return l, nil
}

// ResetScanning clears flags left by a server that stopped during a scan.
func (s *Store) ResetScanning(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE libraries SET scanning = 0 WHERE scanning <> 0`)
	return err
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

// SetLibraryShowMain says whether a commercials library shows in the main menus.
func (s *Store) SetLibraryShowMain(ctx context.Context, id int64, on bool) error {
	v := 0
	if on {
		v = 1
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE libraries SET show_main = ? WHERE id = ? AND type = ?`, v, id, CommercialsType)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Hidden reports whether a library stays out of the main menus.
func (l *Library) Hidden() bool { return l.Type == CommercialsType && !l.ShowMain }

// SetLibraryConsole says which console a games library holds.
func (s *Store) SetLibraryConsole(ctx context.Context, id int64, console string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE libraries SET console = ? WHERE id = ?`, console, id)
	return err
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
	IFNULL(i.play_mode, ''), IFNULL(i.insert_fx, ''), i.intro_start, i.intro_end,
	IFNULL(i.artist, ''), IFNULL(i.album, ''), IFNULL(i.album_artist, ''), i.track, i.disc_no, IFNULL(i.reading, ''), IFNULL(i.console, ''), i.cd_disc`

var itemColsBrief = strings.Replace(itemCols, "i.disc_json", "CASE WHEN json_valid(i.disc_json) THEN json_remove(i.disc_json, '$.titles', '$.cd') END", 1)

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
	var track, discNo, cdDisc sql.NullInt64
	err := sc.Scan(&it.ID, &it.LibraryID, &it.Kind, &it.Path, &it.Title, &it.SortTitle, &year, &overview, &tagline, &genres, &rating,
		&contentRating, &studio, &runtime, &poster, &backdrop, &container, &vcodec, &width, &height,
		&audio, &subs, &discJSON, &provider, &providerID, &size, &mtime, &it.AddedAt, &it.UpdatedAt, &missing,
		&showID, &showTitle, &season, &episode, &episodeEnd, &aired, &it.PlayMode, &it.InsertFX, &introStart, &introEnd,
		&it.Artist, &it.Album, &it.AlbumArtist, &track, &discNo, &it.Reading, &it.Console, &cdDisc)
	if err != nil {
		return nil, err
	}
	if cdDisc.Valid {
		it.CDDisc = &cdDisc.Int64
	}
	if introStart.Valid && introEnd.Valid {
		it.IntroStart, it.IntroEnd = &introStart.Float64, &introEnd.Float64
	}
	if showID.Valid {
		it.ShowID = &showID.Int64
	}
	it.ShowTitle, it.Aired = showTitle.String, aired.String
	it.Season, it.Episode, it.EpisodeEnd = nullInt(season), nullInt(episode), nullInt(episodeEnd)
	it.Track, it.DiscNo = nullInt(track), nullInt(discNo)
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
			if d.Titles == nil { // left out of lists (ItemQuery.Brief); apps decode this as a list
				d.Titles = []DiscTitle{}
			}
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
	Kind      string // an item kind, or "disc" for every DVD and Blu-ray
	// DiscContent: "movie" or "tv", discs holding films or TV episodes.
	DiscContent string
	ShowID      int64
	// NoEpisodes leaves TV episodes out (home hero, movie views).
	NoEpisodes bool
	// Access limits the result to what a Kids/Guest profile may see.
	Access *Access
	// IDs limits the result to these items.
	IDs []int64
	// NoTotal skips counting all matches, for callers that only want the list.
	NoTotal bool
	// Brief leaves a disc's list of titles and chapters out: only its own page
	// reads them, and they can be most of what a list of discs weighs.
	Brief bool
	// Watchable: movies, videos and discs only (no music, books or podcasts).
	Watchable bool
	// NoGallery leaves out photo, vertical video and creator libraries, which
	// are not what the home page's hero or Shuffle are for.
	NoGallery bool
	// Console: games for one console.
	Console string
	// PlayedBy: only what this user has played for more than a minute (games'
	// Continue playing shelf, which must not depend on how far a list has loaded).
	PlayedBy int64
	// HasArt: only items with a backdrop or a frame from the video, for the home
	// page's hero, which would otherwise be an empty dark box.
	HasArt bool
	// Audio: one album (by album artist and album title).
	Album       *string
	AlbumArtist *string
}

// BackfillAges fills min_age for items and shows saved before parental
// controls existed. It runs once; later saves keep the column current.
func (s *Store) BackfillAges(ctx context.Context) error {
	if s.DB.GetSetting(ctx, "agesFilled", "") == "1" {
		return nil
	}
	for _, table := range []string{"items", "shows"} {
		rows, err := s.DB.QueryContext(ctx, `SELECT id, content_rating FROM `+table+` WHERE IFNULL(content_rating, '') <> ''`)
		if err != nil {
			return err
		}
		type row struct {
			id  int64
			age *int
		}
		var todo []row
		for rows.Next() {
			var r row
			var rating string
			if rows.Scan(&r.id, &rating) == nil {
				if r.age = AgeFor(rating); r.age != nil {
					todo = append(todo, r)
				}
			}
		}
		rows.Close()
		for _, r := range todo {
			if _, err := s.DB.ExecContext(ctx, `UPDATE `+table+` SET min_age = ? WHERE id = ?`, *r.age, r.id); err != nil {
				return err
			}
		}
	}
	return s.DB.SetSetting(ctx, "agesFilled", "1")
}

func (s *Store) Items(ctx context.Context, q ItemQuery) ([]*Item, int, error) {
	where := []string{"i.missing = 0"}
	var args []any
	if q.Access != nil {
		c, a := q.Access.ItemClause()
		where = append(where, c)
		args = append(args, a...)
	}
	if q.IDs != nil {
		where = append(where, "i.id IN ("+ids(q.IDs)+")")
	}
	if q.LibraryID != 0 {
		where = append(where, "i.library_id = ?")
		args = append(args, q.LibraryID)
	} else {
		where = append(where, "i.library_id NOT IN (SELECT id FROM libraries WHERE type = '"+CommercialsType+"' AND show_main = 0)")
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
	if q.Kind == "disc" {
		where = append(where, "i.kind IN ('dvd-iso', 'dvd-folder', 'bluray-iso', 'bluray-folder', '"+CDKind+"')")
	} else if q.Kind != "" {
		where = append(where, "i.kind = ?")
		args = append(args, q.Kind)
	}
	if q.DiscContent != "" {
		where = append(where, "json_extract(i.disc_json, '$.content') = ?")
		args = append(args, q.DiscContent)
	}
	if q.NoEpisodes {
		where = append(where, "i.show_id IS NULL")
	}
	if q.Watchable {
		where = append(where, "i.kind IN ('video', 'dvd-iso', 'dvd-folder', 'bluray-iso', 'bluray-folder')")
	}
	if q.Watchable || q.NoGallery {
		where = append(where, "i.library_id NOT IN (SELECT id FROM libraries WHERE type IN ('photos', 'shorts', 'channels', 'musicvideos', 'radio'))")
	}
	if q.Console != "" {
		where = append(where, "i.console = ?")
		args = append(args, q.Console)
	}
	if q.PlayedBy != 0 {
		where = append(where, "i.id IN (SELECT item_id FROM progress WHERE user_id = ? AND position_sec > 60)")
		args = append(args, q.PlayedBy)
	}
	if q.HasArt {
		where = append(where, "(IFNULL(i.backdrop, '') <> '' OR i.poster LIKE 'cache:thumb-%')")
	}
	if q.ShowID != 0 {
		where = append(where, "i.show_id = ?")
		args = append(args, q.ShowID)
	}
	if q.Album != nil {
		where = append(where, "IFNULL(i.album, '') = ?")
		args = append(args, *q.Album)
	}
	if q.AlbumArtist != nil {
		where = append(where, albumArtist+" = ?")
		args = append(args, *q.AlbumArtist)
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
	case "aired":
		order = "IFNULL(i.aired, '')"
	case "track":
		order = "IFNULL(i.disc_no, 1), IFNULL(i.track, 9999), i.sort_title COLLATE NOCASE"
	case "random":
		order = "RANDOM()" // the Shuffle widget; fine for home-sized limits
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
	if !q.NoTotal {
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM items i WHERE `+w, args...).Scan(&total); err != nil {
			return nil, 0, err
		}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 10000
	}
	cols := itemCols
	if q.Brief {
		cols = itemColsBrief
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+cols+` FROM items i WHERE `+w+` ORDER BY `+order+` LIMIT ? OFFSET ?`, append(args, limit, q.Offset)...)
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
			provider, provider_id, size_bytes, mtime, added_at, updated_at, missing, show_id, season, episode, episode_end, aired, intro_start, intro_end, min_age,
			artist, album, album_artist, track, disc_no, reading, console, cd_disc)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			it.LibraryID, it.Kind, it.Path, it.Title, it.SortTitle, it.Year, it.Overview, it.Tagline, joinList(it.Genres), it.Rating,
			it.ContentRating, it.Studio, it.RuntimeSec, it.Poster, it.Backdrop, it.Container, it.VideoCodec, it.Width, it.Height,
			joinList(it.AudioLangs), joinList(it.SubLangs), discJSON, it.Provider, it.ProviderID, it.SizeBytes, it.MTime, now, now,
			it.ShowID, it.Season, it.Episode, it.EpisodeEnd, it.Aired, it.IntroStart, it.IntroEnd, AgeFor(it.ContentRating),
			it.Artist, it.Album, it.AlbumArtist, it.Track, it.DiscNo, it.Reading, it.Console, it.CDDisc)
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
		show_id=?, season=?, episode=?, episode_end=?, aired=?, intro_start=?, intro_end=?, min_age=?,
		artist=?, album=?, album_artist=?, track=?, disc_no=?, reading=?, console=?, cd_disc=? WHERE id=?`,
		it.Kind, it.Title, it.SortTitle, it.Year, it.Overview, it.Tagline, joinList(it.Genres), it.Rating,
		it.ContentRating, it.Studio, it.RuntimeSec, it.Poster, it.Backdrop, it.Container, it.VideoCodec, it.Width, it.Height,
		joinList(it.AudioLangs), joinList(it.SubLangs), discJSON, it.Provider, it.ProviderID, it.SizeBytes, it.MTime, now,
		it.ShowID, it.Season, it.Episode, it.EpisodeEnd, it.Aired, it.IntroStart, it.IntroEnd, AgeFor(it.ContentRating),
		it.Artist, it.Album, it.AlbumArtist, it.Track, it.DiscNo, it.Reading, it.Console, it.CDDisc, it.ID)
	it.UpdatedAt = now
	return err
}

// DiscLabels are names a user gave to a disc's titles and chapters, keyed by
// title number and chapter number (as strings, for JSON).
type DiscLabels struct {
	Titles   map[string]string            `json:"titles"`
	Chapters map[string]map[string]string `json:"chapters"`
}

// Clean drops empty names and trims the rest.
func (d DiscLabels) Clean() DiscLabels {
	out := DiscLabels{Titles: map[string]string{}, Chapters: map[string]map[string]string{}}
	for k, v := range d.Titles {
		if v = strings.TrimSpace(v); v != "" {
			out.Titles[k] = v
		}
	}
	for t, m := range d.Chapters {
		for k, v := range m {
			if v = strings.TrimSpace(v); v != "" {
				if out.Chapters[t] == nil {
					out.Chapters[t] = map[string]string{}
				}
				out.Chapters[t][k] = v
			}
		}
	}
	return out
}

// DiscLabels returns the stored names for a disc (never nil maps).
func (s *Store) DiscLabels(ctx context.Context, id int64) DiscLabels {
	var raw sql.NullString
	_ = s.DB.QueryRowContext(ctx, `SELECT disc_labels FROM items WHERE id = ?`, id).Scan(&raw)
	var d DiscLabels
	_ = json.Unmarshal([]byte(raw.String), &d)
	return d.Clean()
}

// SetDiscLabels replaces the names for a disc.
func (s *Store) SetDiscLabels(ctx context.Context, id int64, d DiscLabels) error {
	b, err := json.Marshal(d.Clean())
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE items SET disc_labels = ? WHERE id = ?`, string(b), id)
	return err
}

// SetItemPrefs stores per-item viewer preferences.
func (s *Store) SetItemPrefs(ctx context.Context, id int64, playMode, insertFX string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE items SET play_mode = ?, insert_fx = ? WHERE id = ?`, playMode, insertFX, id)
	return err
}

// UpdateItemMeta applies user edits.
func (s *Store) UpdateItem(ctx context.Context, it *Item) error { return s.upsertItem(ctx, it) }

func (s *Store) setDisc(ctx context.Context, id int64, d *DiscInfo) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE items SET disc_json = ? WHERE id = ?`, string(b), id)
	return err
}

func (s *Store) retitle(ctx context.Context, id int64, title string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE items SET title = ?, sort_title = ?, updated_at = ? WHERE id = ?`, title, SortTitle(title), db.Now(), id)
	return err
}

// markMissing hides the items of a library that a scan did not find. Items
// under a folder that could not be reached at all (offline: a drive or a
// network share that is away) are left alone: they are not gone.
func (s *Store) markMissing(ctx context.Context, libraryID int64, keep map[string]bool, offline []string) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, path FROM items WHERE library_id = ? AND missing = 0`, libraryID)
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
		// Podcast episodes that were not downloaded live at their web address.
		if image, _, isTrack := CDTrackNumber(p); isTrack {
			p = image // a CD's tracks stay or go with its image
		}
		if !keep[p] && !strings.HasPrefix(p, "http://") && !strings.HasPrefix(p, "https://") && !underAny(p, offline) {
			gone = append(gone, id)
		}
	}
	rows.Close()
	if len(gone) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range gone {
		if _, err := tx.ExecContext(ctx, `UPDATE items SET missing = 1 WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func underAny(p string, roots []string) bool {
	for _, r := range roots {
		if rel, err := filepath.Rel(r, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// restore shows an item again whose file is back, unchanged.
func (s *Store) restore(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE items SET missing = 0 WHERE id = ?`, id)
	return err
}

func (s *Store) setPoster(ctx context.Context, id int64, poster string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE items SET poster = ?, updated_at = ? WHERE id = ?`, poster, db.Now(), id)
	return err
}

func (s *Store) Genres(ctx context.Context, libraryID int64, access *Access) ([]string, error) {
	c, a := access.ItemClause()
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT i.genres FROM items i WHERE i.missing = 0 AND (? = 0 OR i.library_id = ?) AND `+c, append([]any{libraryID, libraryID}, a...)...)
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
	byID := make(map[int64]*Item, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	q := `SELECT item_id, position_sec, duration_sec, watched, disc_title, updated_at FROM progress WHERE user_id = ?`
	if len(items) <= 500 { // a page of items: only their rows, not everything this account ever played
		list := make([]int64, 0, len(items))
		for id := range byID {
			list = append(list, id)
		}
		q += ` AND item_id IN (` + ids(list) + `)`
	}
	rows, err := s.DB.QueryContext(ctx, q, userID)
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
			pp.ResumeSec = resumePoint(it.Kind, &pp)
			it.Progress = &pp
		}
	}
	return rows.Err()
}

// ContinueWatching lists in-progress items for a user.
func (s *Store) ContinueWatching(ctx context.Context, userID int64, limit int) ([]*Item, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+itemCols+` FROM items i JOIN progress p ON p.item_id = i.id
		WHERE p.user_id = ? AND p.watched = 0 AND i.missing = 0
		  AND (p.position_sec > 60 OR (i.kind = 'book' AND p.position_sec > 0)) -- books count pages/chapters, not seconds
		ORDER BY p.updated_at DESC LIMIT ?`, userID, limit)
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
