package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittyphotos"
)

// WittyPhotos: a gallery of the owner's own pictures and clips. Pictures are
// items of kind "photo"; clips stay kind "video", so they play like any other
// video. The scan reads headers only (EXIF and dimensions, no decoding, no
// FFmpeg for common formats); thumbnails are made the first time they are
// asked for (thumbs.go).

var photoExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".jpe": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true,
	".tif": true, ".tiff": true, ".heic": true, ".heif": true, ".hif": true, ".avif": true, ".jxl": true,
}

// PhotoEngine reads what photos say about themselves. The built-in copy of
// WittyPhotos does this until a newer version replaces it (internal/modules).
type PhotoEngine interface {
	ReadFile(path string) (*wittyphotos.Info, error)
	Dimensions(path string) (w, h int)
	RawPreview(path string) ([]byte, error)
	IsRaw(name string) bool
}

type builtinPhotos struct{}

func (builtinPhotos) ReadFile(path string) (*wittyphotos.Info, error) {
	return wittyphotos.ReadFile(path)
}
func (builtinPhotos) Dimensions(path string) (w, h int)      { return wittyphotos.Dimensions(path) }
func (builtinPhotos) RawPreview(path string) ([]byte, error) { return wittyphotos.RawPreview(path) }
func (builtinPhotos) IsRaw(name string) bool                 { return wittyphotos.IsRaw(name) }

var photoEngine atomic.Pointer[PhotoEngine]

// SetPhotoEngine chooses what reads photos, once at start.
func SetPhotoEngine(e PhotoEngine) { photoEngine.Store(&e) }

func photos() PhotoEngine {
	if e := photoEngine.Load(); e != nil {
		return *e
	}
	return builtinPhotos{}
}

// IsPhotoFile: a picture or camera RAW file by its name.
func IsPhotoFile(name string) bool {
	return photoExts[strings.ToLower(filepath.Ext(name))] || photos().IsRaw(name)
}

// IsVideoFile: a video file by its name.
func IsVideoFile(name string) bool { return videoExts[strings.ToLower(filepath.Ext(name))] }

// WebSafeImage: browsers and native TV image views show these as they are.
// Everything else is sent as a JPEG.
func WebSafeImage(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".jpe", ".png", ".gif", ".webp":
		return true
	}
	return false
}

// Short clips beside a picture of the same name (a phone's live photo) belong
// to the picture rather than the timeline.
const motionClipMax = 40 << 20

func discoverPhotos(root string) ([]candidate, error) {
	var out []candidate
	var bad unreadable
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			bad.note(err)
			return nil // unreadable folders are skipped, not fatal
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") || strings.EqualFold(name, "@eaDir") || strings.EqualFold(name, "#recycle")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		kind := ""
		switch {
		case IsPhotoFile(name):
			kind = "photo"
		case IsVideoFile(name):
			kind = "video"
		default:
			return nil
		}
		st, err := d.Info()
		if err != nil || st.Size() == 0 {
			return nil
		}
		out = append(out, candidate{path: p, kind: kind, size: st.Size(), mtime: st.ModTime().Unix()})
		return nil
	})
	return pairPhotos(out), bad.with(err)
}

// pairPhotos folds what belongs together: a RAW file with the JPEG the camera
// saved beside it shows as the JPEG, and a short clip with a picture of the same
// name becomes that picture's motion.
func pairPhotos(c []candidate) []candidate {
	stem := func(p string) string { return strings.ToLower(strings.TrimSuffix(p, filepath.Ext(p))) }
	still := map[string]int{}
	for i, x := range c {
		if x.kind != "photo" {
			continue
		}
		if j, ok := still[stem(x.path)]; ok && photos().IsRaw(c[j].path) {
			still[stem(x.path)] = i // the developed picture wins over its RAW
		} else if !ok {
			still[stem(x.path)] = i
		}
	}
	out := c[:0:0]
	motion := map[int]string{}
	for _, x := range c {
		if x.kind != "video" {
			continue
		}
		if i, ok := still[stem(x.path)]; ok && x.size <= motionClipMax {
			motion[i] = x.path
		}
	}
	for i, x := range c {
		switch {
		case x.kind == "photo":
			if still[stem(x.path)] != i {
				continue
			}
			x.motion = motion[i]
		case x.kind == "video":
			if i, ok := still[stem(x.path)]; ok && motion[i] == x.path {
				continue
			}
		}
		out = append(out, x)
	}
	return out
}

// dateNameRe finds a date and time in names cameras and phones give files:
// IMG_20230412_153045, 2023-04-12 15.30.45, Screenshot_20230412-153045.
var dateNameRe = regexp.MustCompile(`(?:^|\D)(20\d{2}|19\d{2})[-_.]?(0[1-9]|1[0-2])[-_.]?(0[1-9]|[12]\d|3[01])(?:[-_. T]?([01]\d|2[0-3])[-_.:]?([0-5]\d)[-_.:]?([0-5]\d))?(?:\D|$)`)

// DateFromName reads a date from a file name, as wall-clock seconds.
func DateFromName(name string) (int64, bool) {
	m := dateNameRe.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	n := func(s string) int { v, _ := strconv.Atoi(s); return v }
	t := time.Date(n(m[1]), time.Month(n(m[2])), n(m[3]), n(m[4]), n(m[5]), n(m[6]), 0, time.UTC)
	return t.Unix(), true
}

// wallClock turns a time into the clock on the wall where it was, as if UTC.
func wallClock(t time.Time) int64 {
	y, mo, d := t.Date()
	h, mi, s := t.Clock()
	return time.Date(y, mo, d, h, mi, s, 0, time.UTC).Unix()
}

func ymOf(wall int64) int {
	t := time.Unix(wall, 0).UTC()
	return t.Year()*100 + int(t.Month())
}

// photoMeta is one row of photo_meta.
type photoMeta struct {
	TakenAt             int64
	Folder              string
	Make, Model, Lens   string
	Exposure            string
	FNumber, Focal, Alt float64
	ISO, Orientation    int
	Lat, Lon            *float64
	Raw                 bool
	Motion              string
	AltSet              bool
}

func relFolder(lib *Library, p string) string {
	dir := filepath.Dir(p)
	for _, r := range lib.Paths {
		if rel, err := filepath.Rel(r, dir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if rel == "." {
				return ""
			}
			return filepath.ToSlash(rel)
		}
	}
	return ""
}

// indexPhoto saves a picture or clip of a photos library.
func (s *Scanner) indexPhoto(ctx context.Context, lib *Library, c candidate) error {
	existing, err := s.Store.itemByPath(ctx, lib.ID, c.path)
	if err == nil && existing.MTime == c.mtime && existing.SizeBytes == c.size && !existing.Missing {
		return nil // unchanged
	}
	it := &Item{LibraryID: lib.ID, Kind: c.kind, Path: c.path, SizeBytes: c.size, MTime: c.mtime, Poster: LazyPoster}
	if existing != nil {
		it.ID, it.AddedAt = existing.ID, existing.AddedAt
	}
	base := strings.TrimSuffix(filepath.Base(c.path), filepath.Ext(c.path))
	it.Title, it.SortTitle = base, SortTitle(base)
	it.Container = strings.TrimPrefix(strings.ToLower(filepath.Ext(c.path)), ".")
	m := photoMeta{Folder: relFolder(lib, c.path), Motion: c.motion, Raw: photos().IsRaw(c.path)}
	if c.kind == "video" {
		s.probeVideo(ctx, it)
		s.videoDate(ctx, it, &m)
	} else {
		s.readPhoto(ctx, it, &m)
	}
	if m.TakenAt == 0 {
		if t, ok := DateFromName(filepath.Base(c.path)); ok {
			m.TakenAt = t
		} else {
			m.TakenAt = wallClock(time.Unix(c.mtime, 0))
		}
	}
	y := time.Unix(m.TakenAt, 0).UTC().Year()
	it.Year = &y
	if err := s.Store.upsertItem(ctx, it); err != nil {
		return err
	}
	return s.Store.savePhotoMeta(ctx, it, &m)
}

func (s *Scanner) readPhoto(ctx context.Context, it *Item, m *photoMeta) {
	in, err := photos().ReadFile(it.Path)
	if err == nil {
		m.Make, m.Model, m.Lens, m.Exposure = in.Make, in.Model, in.Lens, in.Exposure
		m.FNumber, m.Focal, m.ISO, m.Orientation = in.FNumber, in.FocalMM, in.ISO, in.Orientation
		if !in.Taken.IsZero() {
			m.TakenAt = wallClock(in.Taken)
		}
		if in.HasGPS {
			la, lo := in.Lat, in.Lon
			m.Lat, m.Lon, m.Alt, m.AltSet = &la, &lo, in.Alt, true
		}
		it.Width, it.Height = in.Width, in.Height
	} else {
		s.Log.DebugContext(ctx, "no details read from a photo", "file", it.Path, "err", err)
	}
	if w, h := photos().Dimensions(it.Path); w > 0 {
		it.Width, it.Height = w, h // the picture's own header beats EXIF, which editors leave stale
	} else if it.Width == 0 && !m.Raw {
		pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		if p, err := s.Tools.ProbeFile(pctx, it.Path); err == nil {
			if v := p.Video(); v != nil {
				it.Width, it.Height = v.Width, v.Height
			}
		}
		cancel()
	}
	if m.Orientation >= 5 {
		it.Width, it.Height = it.Height, it.Width // what the viewer sees
	}
}

var iso6709Re = regexp.MustCompile(`^([+-]\d+(?:\.\d+)?)([+-]\d+(?:\.\d+)?)`)

// videoDate reads when and where a clip was shot from its container tags.
func (s *Scanner) videoDate(ctx context.Context, it *Item, m *photoMeta) {
	pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	p, err := s.Tools.ProbeFile(pctx, it.Path)
	if err != nil {
		s.minorFailure(ctx, "could not read a file's details", err, "file", it.Path)
		return
	}
	for k, v := range p.Format.Tags {
		switch strings.ToLower(k) {
		case "creation_time", "com.apple.quicktime.creationdate":
			v, _, _ = strings.Cut(v, ";") // some muxers join two values
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				t, err = time.Parse("2006-01-02T15:04:05-0700", v) // phones
			}
			if err == nil && t.Year() > 1980 && m.TakenAt == 0 {
				m.TakenAt = wallClock(t)
			}
		case "location", "com.apple.quicktime.location.iso6709":
			if g := iso6709Re.FindStringSubmatch(v); g != nil {
				la, e1 := strconv.ParseFloat(g[1], 64)
				lo, e2 := strconv.ParseFloat(g[2], 64)
				if e1 == nil && e2 == nil && la >= -90 && la <= 90 && lo >= -180 && lo <= 180 && (la != 0 || lo != 0) {
					m.Lat, m.Lon = &la, &lo
				}
			}
		}
	}
}

func (s *Store) savePhotoMeta(ctx context.Context, it *Item, m *photoMeta) error {
	var alt *float64
	if m.AltSet {
		alt = &m.Alt
	}
	raw := 0
	if m.Raw {
		raw = 1
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO photo_meta(item_id, library_id, taken_at, ym, folder, make, model, lens, fnumber, exposure, iso, focal, orientation, lat, lon, alt, raw, motion)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(item_id) DO UPDATE SET taken_at=excluded.taken_at, ym=excluded.ym, folder=excluded.folder,
		make=excluded.make, model=excluded.model, lens=excluded.lens, fnumber=excluded.fnumber, exposure=excluded.exposure, iso=excluded.iso, focal=excluded.focal,
		orientation=excluded.orientation, lat=excluded.lat, lon=excluded.lon, alt=excluded.alt, raw=excluded.raw, motion=excluded.motion`,
		it.ID, it.LibraryID, m.TakenAt, ymOf(m.TakenAt), m.Folder, m.Make, m.Model, m.Lens, m.FNumber, m.Exposure, m.ISO, m.Focal, m.Orientation, m.Lat, m.Lon, alt, raw, m.Motion)
	return err
}

// ---------------------------------------------------------------------------
// Reading the gallery
// ---------------------------------------------------------------------------

// Photo is one picture or clip in a list. It says nothing about where it is on
// disk or where it was taken; PhotoDetail does, for one picture at a time.
type Photo struct {
	ID          int64   `json:"id"`
	LibraryID   int64   `json:"libraryId"`
	Kind        string  `json:"kind"` // photo | video
	Title       string  `json:"title"`
	TakenAt     int64   `json:"takenAt"` // wall-clock seconds; show as UTC
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	DurationSec float64 `json:"durationSec,omitempty"`
	Raw         bool    `json:"raw,omitempty"`
	Motion      bool    `json:"motion,omitempty"`
	HasLocation bool    `json:"hasLocation,omitempty"`
	Favorite    bool    `json:"favorite,omitempty"`
	UpdatedAt   int64   `json:"updatedAt"`
}

// PhotoQuery picks a slice of a gallery. Results run newest first and page by
// the cursor each page returns.
type PhotoQuery struct {
	LibraryID int64
	UserID    int64
	Access    *Access
	YM        int
	Album     int64
	Folder    *string
	Favorites bool
	Kind      string // photo | video
	Search    string
	Camera    string
	BBox      *[4]float64 // south, west, north, east
	Limit     int
	Cursor    string // from the previous page
}

// FormatPhotoCursor and parsePhotoCursor: the place in a list, "taken.id".
func formatPhotoCursor(taken, id int64) string { return fmt.Sprintf("%d.%d", taken, id) }

func parsePhotoCursor(c string) (taken, id int64, ok bool) {
	a, b, found := strings.Cut(c, ".")
	if !found {
		return 0, 0, false
	}
	t, e1 := strconv.ParseInt(a, 10, 64)
	i, e2 := strconv.ParseInt(b, 10, 64)
	return t, i, e1 == nil && e2 == nil
}

func (q *PhotoQuery) where() (string, []any) {
	w := []string{"i.missing = 0", "i.library_id = p.library_id"}
	var args []any
	if q.LibraryID != 0 {
		w = append(w, "p.library_id = ?")
		args = append(args, q.LibraryID)
	}
	if q.Access != nil {
		c, a := q.Access.ItemClause()
		w = append(w, c)
		args = append(args, a...)
	}
	if q.YM != 0 {
		w = append(w, "p.ym = ?")
		args = append(args, q.YM)
	}
	if q.Album != 0 {
		w = append(w, "p.item_id IN (SELECT item_id FROM photo_album_items WHERE album_id = ?)")
		args = append(args, q.Album)
	}
	if q.Folder != nil {
		w = append(w, "p.folder = ?")
		args = append(args, *q.Folder)
	}
	if q.Favorites {
		w = append(w, "p.item_id IN (SELECT item_id FROM item_reactions WHERE user_id = ? AND value > 0)")
		args = append(args, q.UserID)
	}
	if q.Kind != "" {
		w = append(w, "i.kind = ?")
		args = append(args, q.Kind)
	}
	if q.Search != "" {
		w = append(w, "(i.title LIKE ? OR p.folder LIKE ? OR p.model LIKE ?)")
		like := "%" + q.Search + "%"
		args = append(args, like, like, like)
	}
	if q.Camera != "" {
		w = append(w, "p.model = ?")
		args = append(args, q.Camera)
	}
	if q.BBox != nil {
		w = append(w, "p.lat IS NOT NULL AND p.lat BETWEEN ? AND ? AND p.lon BETWEEN ? AND ?")
		args = append(args, q.BBox[0], q.BBox[2], q.BBox[1], q.BBox[3])
	}
	return strings.Join(w, " AND "), args
}

// Photos returns one page and the cursor of the next ("" at the end).
func (s *Store) Photos(ctx context.Context, q PhotoQuery) ([]Photo, string, error) {
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	w, args := q.where()
	if t, id, ok := parsePhotoCursor(q.Cursor); ok {
		w += " AND (p.taken_at < ? OR (p.taken_at = ? AND p.item_id < ?))"
		args = append(args, t, t, id)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT p.item_id, p.library_id, i.kind, i.title, p.taken_at, IFNULL(i.width, 0), IFNULL(i.height, 0), IFNULL(i.runtime_sec, 0),
		p.raw, IFNULL(p.motion, '') <> '', p.lat IS NOT NULL, i.updated_at,
		EXISTS(SELECT 1 FROM item_reactions r WHERE r.user_id = ? AND r.item_id = p.item_id AND r.value > 0)
		FROM photo_meta p JOIN items i ON i.id = p.item_id WHERE `+w+` ORDER BY p.taken_at DESC, p.item_id DESC LIMIT ?`,
		append(append([]any{q.UserID}, args...), limit+1)...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []Photo{}
	for rows.Next() {
		var ph Photo
		if err := rows.Scan(&ph.ID, &ph.LibraryID, &ph.Kind, &ph.Title, &ph.TakenAt, &ph.Width, &ph.Height, &ph.DurationSec, &ph.Raw, &ph.Motion, &ph.HasLocation, &ph.UpdatedAt, &ph.Favorite); err != nil {
			return nil, "", err
		}
		out = append(out, ph)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		last := out[limit-1]
		next = formatPhotoCursor(last.TakenAt, last.ID)
	}
	return out, next, nil
}

// PhotoMonth is one month of a gallery, for the timeline and its scrubber.
type PhotoMonth struct {
	YM    int `json:"ym"` // 202304
	Count int `json:"count"`
}

// PhotoMonths lists months with pictures, newest first.
func (s *Store) PhotoMonths(ctx context.Context, q PhotoQuery) ([]PhotoMonth, error) {
	w, args := q.where()
	rows, err := s.DB.QueryContext(ctx, `SELECT p.ym, COUNT(*) FROM photo_meta p JOIN items i ON i.id = p.item_id WHERE `+w+` GROUP BY p.ym ORDER BY p.ym DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PhotoMonth{}
	for rows.Next() {
		var m PhotoMonth
		if err := rows.Scan(&m.YM, &m.Count); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// PhotoDetail is everything known about one picture.
type PhotoDetail struct {
	Photo
	FileName string   `json:"fileName"`
	Folder   string   `json:"folder"`
	Size     int64    `json:"sizeBytes"`
	Make     string   `json:"make,omitempty"`
	Model    string   `json:"model,omitempty"`
	Lens     string   `json:"lens,omitempty"`
	FNumber  float64  `json:"fNumber,omitempty"`
	Exposure string   `json:"exposure,omitempty"`
	ISO      int      `json:"iso,omitempty"`
	FocalMM  float64  `json:"focalMm,omitempty"`
	Lat      *float64 `json:"lat,omitempty"`
	Lon      *float64 `json:"lon,omitempty"`
	Alt      *float64 `json:"alt,omitempty"`
	Albums   []int64  `json:"albums"`
	WebSafe  bool     `json:"webSafe"` // the original can be shown as it is
}

// PhotoDetail loads one picture for an account that may see its item.
func (s *Store) PhotoDetail(ctx context.Context, userID int64, it *Item) (*PhotoDetail, error) {
	d := PhotoDetail{Photo: Photo{ID: it.ID, LibraryID: it.LibraryID, Kind: it.Kind, Title: it.Title, Width: it.Width, Height: it.Height, DurationSec: it.RuntimeSec, UpdatedAt: it.UpdatedAt},
		FileName: filepath.Base(it.Path), Size: it.SizeBytes, WebSafe: WebSafeImage(it.Path), Albums: []int64{}}
	var mk, model, lens, exposure, motion sql.NullString
	var fnumber, focal sql.NullFloat64
	var iso sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT taken_at, folder, make, model, lens, fnumber, exposure, iso, focal, lat, lon, alt, raw, motion FROM photo_meta WHERE item_id = ?`, it.ID).Scan(
		&d.TakenAt, &d.Folder, &mk, &model, &lens, &fnumber, &exposure, &iso, &focal, &d.Lat, &d.Lon, &d.Alt, &d.Raw, &motion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.Make, d.Model, d.Lens, d.Exposure = mk.String, model.String, lens.String, exposure.String
	d.FNumber, d.FocalMM, d.ISO = fnumber.Float64, focal.Float64, int(iso.Int64)
	d.Motion = motion.String != ""
	d.HasLocation = d.Lat != nil
	var like sql.NullInt64
	_ = s.DB.QueryRowContext(ctx, `SELECT value FROM item_reactions WHERE user_id = ? AND item_id = ?`, userID, it.ID).Scan(&like)
	d.Favorite = like.Int64 > 0
	rows, err := s.DB.QueryContext(ctx, `SELECT a.album_id FROM photo_album_items a JOIN photo_albums b ON b.id = a.album_id WHERE a.item_id = ? AND (b.owner_id = ? OR b.shared = 1)`, it.ID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			d.Albums = append(d.Albums, id)
		}
	}
	return &d, rows.Err()
}

// PhotoOrientation and motion clip of a picture, for its thumbnail and player.
func (s *Store) photoExtras(ctx context.Context, id int64) (orientation int, motion string) {
	var m sql.NullString
	_ = s.DB.QueryRowContext(ctx, `SELECT orientation, motion FROM photo_meta WHERE item_id = ?`, id).Scan(&orientation, &m)
	return orientation, m.String
}

// PhotoLocated: the picture or clip has a known place.
func (s *Store) PhotoLocated(ctx context.Context, id int64) bool {
	var one int
	return s.DB.QueryRowContext(ctx, `SELECT 1 FROM photo_meta WHERE item_id = ? AND lat IS NOT NULL`, id).Scan(&one) == nil
}

// PhotoMotion is the clip that belongs to a live photo ("" when none).
func (s *Store) PhotoMotion(ctx context.Context, id int64) string {
	_, m := s.photoExtras(ctx, id)
	return m
}

// PhotoFolder is one folder of a gallery, a read-only album of its own.
type PhotoFolder struct {
	Folder string `json:"folder"`
	Count  int    `json:"count"`
	Cover  int64  `json:"coverId"`
}

// PhotoFolders lists the folders with pictures in them.
func (s *Store) PhotoFolders(ctx context.Context, q PhotoQuery) ([]PhotoFolder, error) {
	w, args := q.where()
	rows, err := s.DB.QueryContext(ctx, `SELECT p.folder, COUNT(*), MAX(p.item_id) FROM photo_meta p JOIN items i ON i.id = p.item_id WHERE `+w+` GROUP BY p.folder ORDER BY p.folder COLLATE NOCASE LIMIT 5000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PhotoFolder{}
	for rows.Next() {
		var f PhotoFolder
		if err := rows.Scan(&f.Folder, &f.Count, &f.Cover); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// PhotoCluster is a group of pictures close together on the map.
type PhotoCluster struct {
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Count int     `json:"count"`
	Cover int64   `json:"coverId"`
	South float64 `json:"south"`
	West  float64 `json:"west"`
	North float64 `json:"north"`
	East  float64 `json:"east"`
}

// PhotoClusters groups located pictures into cells that suit a map zoom level
// (0 shows the whole world). The work is one GROUP BY, whatever the library's size.
func (s *Store) PhotoClusters(ctx context.Context, q PhotoQuery, zoom int) ([]PhotoCluster, error) {
	if zoom < 0 {
		zoom = 0
	}
	if zoom > 18 {
		zoom = 18
	}
	cell := 360.0 / float64(int64(1)<<(zoom+3))
	w, args := q.where()
	rows, err := s.DB.QueryContext(ctx, `SELECT AVG(p.lat), AVG(p.lon), COUNT(*), MAX(p.item_id), MIN(p.lat), MIN(p.lon), MAX(p.lat), MAX(p.lon)
		FROM photo_meta p JOIN items i ON i.id = p.item_id WHERE `+w+` AND p.lat IS NOT NULL
		GROUP BY CAST((p.lat + 90) / ? AS INTEGER), CAST((p.lon + 180) / ? AS INTEGER) LIMIT 3000`, append(args, cell, cell)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PhotoCluster{}
	for rows.Next() {
		var c PhotoCluster
		if err := rows.Scan(&c.Lat, &c.Lon, &c.Count, &c.Cover, &c.South, &c.West, &c.North, &c.East); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Albums
// ---------------------------------------------------------------------------

// PhotoAlbum is a named set of pictures. It belongs to the account that made
// it; a shared album is also visible to every account that can open the library,
// which still sees only the pictures its own limits allow.
type PhotoAlbum struct {
	ID          int64  `json:"id"`
	LibraryID   int64  `json:"libraryId"`
	OwnerID     int64  `json:"ownerId"`
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Shared      bool   `json:"shared"`
	Count       int    `json:"count"`
	CoverID     int64  `json:"coverId"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// PhotoAlbums lists the albums an account can see in a library.
func (s *Store) PhotoAlbums(ctx context.Context, libraryID, userID int64, access *Access) ([]PhotoAlbum, error) {
	clause, cargs := access.ItemClause()
	args := append(append(append([]any{}, cargs...), cargs...), libraryID, userID)
	rows, err := s.DB.QueryContext(ctx, `SELECT a.id, a.library_id, a.owner_id, IFNULL(u.name, ''), a.name, a.description, a.shared, a.created_at, a.updated_at,
		(SELECT COUNT(*) FROM photo_album_items x JOIN items i ON i.id = x.item_id WHERE x.album_id = a.id AND i.missing = 0 AND `+clause+`),
		IFNULL((SELECT i.id FROM items i WHERE i.id = a.cover_item AND i.missing = 0 AND `+clause+`),
		  (SELECT x.item_id FROM photo_album_items x JOIN items i ON i.id = x.item_id WHERE x.album_id = a.id AND i.missing = 0 ORDER BY x.added_at DESC LIMIT 1))
		FROM photo_albums a LEFT JOIN users u ON u.id = a.owner_id
		WHERE a.library_id = ? AND (a.owner_id = ? OR a.shared = 1) ORDER BY a.name COLLATE NOCASE LIMIT 2000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PhotoAlbum{}
	for rows.Next() {
		var a PhotoAlbum
		var cover sql.NullInt64
		if err := rows.Scan(&a.ID, &a.LibraryID, &a.OwnerID, &a.Owner, &a.Name, &a.Description, &a.Shared, &a.CreatedAt, &a.UpdatedAt, &a.Count, &cover); err != nil {
			return nil, err
		}
		a.CoverID = cover.Int64
		out = append(out, a)
	}
	return out, rows.Err()
}

// PhotoAlbumByID loads one album ("not found" when the account may not see it).
func (s *Store) PhotoAlbumByID(ctx context.Context, id, userID int64, isAdmin bool) (*PhotoAlbum, error) {
	var a PhotoAlbum
	err := s.DB.QueryRowContext(ctx, `SELECT a.id, a.library_id, a.owner_id, IFNULL(u.name, ''), a.name, a.description, a.shared, a.created_at, a.updated_at
		FROM photo_albums a LEFT JOIN users u ON u.id = a.owner_id WHERE a.id = ?`, id).Scan(
		&a.ID, &a.LibraryID, &a.OwnerID, &a.Owner, &a.Name, &a.Description, &a.Shared, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !a.Shared && a.OwnerID != userID && !isAdmin {
		return nil, ErrNotFound
	}
	return &a, nil
}

// CreatePhotoAlbum makes an album.
func (s *Store) CreatePhotoAlbum(ctx context.Context, libraryID, ownerID int64, name, description string, shared bool) (*PhotoAlbum, error) {
	now := db.Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO photo_albums(library_id, owner_id, name, description, shared, created_at, updated_at) VALUES(?,?,?,?,?,?,?)`, libraryID, ownerID, name, description, shared, now, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.PhotoAlbumByID(ctx, id, ownerID, true)
}

// UpdatePhotoAlbum changes an album's name, description, sharing and cover. A
// cover of 0 goes back to the newest picture.
func (s *Store) UpdatePhotoAlbum(ctx context.Context, id int64, name, description string, shared bool, cover *int64) error {
	if cover != nil {
		var v any
		if *cover != 0 {
			v = *cover
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE photo_albums SET cover_item = ? WHERE id = ?`, v, id); err != nil {
			return err
		}
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE photo_albums SET name = ?, description = ?, shared = ?, updated_at = ? WHERE id = ?`, name, description, shared, db.Now(), id)
	return err
}

// DeletePhotoAlbum removes an album; the pictures stay.
func (s *Store) DeletePhotoAlbum(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM photo_albums WHERE id = ?`, id)
	return err
}

// AddToPhotoAlbum adds pictures of the album's library (others are skipped).
func (s *Store) AddToPhotoAlbum(ctx context.Context, album *PhotoAlbum, ids []int64) (int, error) {
	now, added := db.Now(), 0
	for _, id := range ids {
		res, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO photo_album_items(album_id, item_id, added_at)
			SELECT ?, id, ? FROM items WHERE id = ? AND library_id = ?`, album.ID, now, id, album.LibraryID)
		if err != nil {
			return added, err
		}
		n, _ := res.RowsAffected()
		added += int(n)
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE photo_albums SET updated_at = ? WHERE id = ?`, now, album.ID)
	return added, err
}

// RemoveFromPhotoAlbum takes pictures out of an album.
func (s *Store) RemoveFromPhotoAlbum(ctx context.Context, albumID int64, ids []int64) error {
	for _, id := range ids {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM photo_album_items WHERE album_id = ? AND item_id = ?`, albumID, id); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Uploads
// ---------------------------------------------------------------------------

// IndexUploaded saves one file that was just added to a library, without a scan.
func (s *Scanner) IndexUploaded(ctx context.Context, lib *Library, path string) (*Item, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	kind := "video"
	if IsPhotoFile(path) {
		kind = "photo"
	}
	c := candidate{path: path, kind: kind, size: st.Size(), mtime: st.ModTime().Unix()}
	err = guard(func() error {
		switch lib.Type {
		case "photos":
			return s.indexPhoto(ctx, lib, c)
		default:
			return s.indexCreatorVideo(ctx, lib, c, newCreatorScan())
		}
	})
	if err != nil {
		return nil, err
	}
	return s.Store.itemByPath(ctx, lib.ID, path)
}

var reservedNameRe = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[0-9]|lpt[0-9])(\..*)?$`)

// SafeFileName makes a name that came from a client safe to create on any file
// system: no folders, control characters or characters some systems forbid, no
// leading dots, no reserved Windows names, and a sensible length. It returns ""
// when nothing usable is left.
func SafeFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || strings.ContainsRune(`<>:"|?*`, r) {
			return '_'
		}
		return r
	}, strings.ToValidUTF8(name, "_"))
	name = strings.TrimRight(strings.TrimLeft(name, ". "), ". ")
	if name == "" || reservedNameRe.MatchString(name) {
		return ""
	}
	if ext := filepath.Ext(name); len(name) > 180 {
		if len(ext) > 16 {
			return ""
		}
		name = strings.ToValidUTF8(name[:180-len(ext)], "") + ext
	}
	return name
}

// SniffMedia: the first bytes of a file really are a picture or video of a kind
// this library takes. Used for uploads, where the name alone proves nothing.
func SniffMedia(b []byte) bool {
	has := func(sig string) bool { return strings.HasPrefix(string(b), sig) }
	switch {
	case len(b) < 12:
		return false
	case has("\xFF\xD8\xFF"), has("\x89PNG\r\n\x1a\n"), has("GIF87a"), has("GIF89a"), has("BM"):
		return true
	case has("II*\x00"), has("MM\x00*"), has("IIRO"), has("IIRS"), has("IIU\x00"), has("MMOR"): // TIFF and RAW on TIFF
		return true
	case has("FUJIFILMCCD-RAW"), has("\x00MRM"), has("FOVb"), has("CIFF"), has("II\x1a\x00\x00\x00HEAPCCDR"):
		return true
	case has("\x00\x00\x00\x0cJXL \r\n\x87\n"), has("\xFF\x0A"):
		return true
	case has("RIFF") && (string(b[8:12]) == "WEBP" || string(b[8:12]) == "AVI "):
		return true
	case string(b[4:8]) == "ftyp": // HEIC, AVIF, MP4, MOV, CR3
		return true
	case has("\x1A\x45\xDF\xA3"): // Matroska and WebM
		return true
	case string(b[4:8]) == "moov", string(b[4:8]) == "mdat", string(b[4:8]) == "free", string(b[4:8]) == "wide": // older QuickTime files
		return true
	}
	return false
}
