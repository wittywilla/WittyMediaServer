package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// WittyShorts and WittyChannels: videos by creators, from files the owner
// already has. Both keep their videos as ordinary items (kind "video"), so
// playback, progress and profile limits work as everywhere else; video_meta adds
// what a creator video has on top, and creators holds the people who made them.
//
// A creator comes from the file's sidecar when there is one, otherwise from the
// first folder under the library: Library/Creator/[Playlist/]video.mp4.

// IsCreatorLibrary: shorts, channels or music videos. A music video's artist is
// its creator.
func IsCreatorLibrary(t string) bool { return t == "shorts" || t == "channels" || t == "musicvideos" }

// IsGalleryLibrary: a library whose edits stay in the database, never written
// beside the media (the owner's own photos and archives are left as they are).
func IsGalleryLibrary(t string) bool { return t == "photos" || IsCreatorLibrary(t) }

func discoverCreatorVideos(root string) ([]candidate, error) {
	var out []candidate
	var bad unreadable
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			bad.note(err)
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") || strings.EqualFold(name, "@eaDir") || strings.EqualFold(name, "#recycle")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || !IsVideoFile(name) {
			return nil
		}
		st, err := d.Info()
		if err != nil || st.Size() == 0 {
			return nil
		}
		out = append(out, candidate{path: p, kind: "video", size: st.Size(), mtime: st.ModTime().Unix()})
		return nil
	})
	return out, bad.with(err)
}

// videoSidecar reads the common sidecar formats: the JSON that video archivers
// leave beside a file (.info.json), the same in a plain .json, and
// .metadata.json with the fields of this project.
type videoSidecar struct {
	Type          string         `json:"_type"`
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	FullTitle     string         `json:"fulltitle"`
	Description   string         `json:"description"`
	Overview      string         `json:"overview"`
	Uploader      string         `json:"uploader"`
	UploaderID    string         `json:"uploader_id"`
	UploaderURL   string         `json:"uploader_url"`
	Channel       string         `json:"channel"`
	ChannelID     string         `json:"channel_id"`
	ChannelURL    string         `json:"channel_url"`
	Creator       string         `json:"creator"`
	Artist        string         `json:"artist"`
	Album         string         `json:"album"`
	Handle        string         `json:"handle"`
	Platform      string         `json:"platform"`
	ExtractorKey  string         `json:"extractor_key"`
	Extractor     string         `json:"extractor"`
	URL           string         `json:"url"`
	WebpageURL    string         `json:"webpage_url"`
	UploadDate    string         `json:"upload_date"`
	Date          string         `json:"date"`
	Timestamp     *float64       `json:"timestamp"`
	ViewCount     *int64         `json:"view_count"`
	LikeCount     *int64         `json:"like_count"`
	Tags          []string       `json:"tags"`
	Categories    []string       `json:"categories"`
	Genres        []string       `json:"genres"`
	AgeLimit      *int           `json:"age_limit"`
	ContentRating string         `json:"contentRating"`
	Playlist      string         `json:"playlist"`
	PlaylistTitle string         `json:"playlist_title"`
	Comments      []sideComment  `json:"comments"`
	Custom        map[string]any `json:"custom"`
}

type sideComment struct {
	Author    string   `json:"author"`
	Text      string   `json:"text"`
	Timestamp *float64 `json:"timestamp"`
	LikeCount *int64   `json:"like_count"`
}

// maxSidecar: sidecars with thousands of saved comments can be large, but a
// bigger file than this is not one.
const maxSidecar = 48 << 20

func readSidecar(path string) (*videoSidecar, int64) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() || st.Size() > maxSidecar {
		return nil, 0
	}
	b, err := io.ReadAll(io.LimitReader(f, maxSidecar))
	if err != nil {
		return nil, 0
	}
	var v videoSidecar
	if json.Unmarshal(b, &v) != nil {
		return nil, 0
	}
	return &v, st.ModTime().Unix()
}

// sidecarFor finds and merges the sidecars of a video; .metadata.json is the
// owner's own and wins over whatever the archive says.
func sidecarFor(path string) (*videoSidecar, int64) {
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	var side *videoSidecar
	var latest int64
	for _, p := range []string{stem + ".info.json", path + ".info.json", stem + ".json", path + ".json"} {
		if v, t := readSidecar(p); v != nil {
			side, latest = v, t
			break
		}
	}
	for _, p := range []string{path + ".metadata.json", stem + ".metadata.json"} {
		own, t := readSidecar(p)
		if own == nil {
			continue
		}
		if t > latest {
			latest = t
		}
		if side == nil {
			side = own
			break
		}
		merged := *side
		ownJSON, _ := json.Marshal(own)
		var set map[string]any
		_ = json.Unmarshal(ownJSON, &set)
		if own.Title != "" {
			merged.Title = own.Title
		}
		if own.Description != "" {
			merged.Description = own.Description
		}
		if own.Overview != "" {
			merged.Description = own.Overview
		}
		if own.Creator != "" {
			merged.Creator = own.Creator
		}
		if own.Platform != "" {
			merged.Platform = own.Platform
		}
		if own.URL != "" {
			merged.URL = own.URL
		}
		if own.Date != "" {
			merged.Date = own.Date
		}
		if len(own.Tags) > 0 {
			merged.Tags = own.Tags
		}
		if len(own.Genres) > 0 {
			merged.Categories = own.Genres
		}
		if own.ContentRating != "" {
			merged.ContentRating = own.ContentRating
		}
		if own.Custom != nil {
			merged.Custom = own.Custom
		}
		side = &merged
		break
	}
	return side, latest
}

var (
	bracketIDRe = regexp.MustCompile(`\s*\[[A-Za-z0-9_-]{6,}\]$`)
	dateDigits  = regexp.MustCompile(`^(\d{4})-?(\d{2})-?(\d{2})`)
)

// uploadDate normalises a date to YYYY-MM-DD ("" when it isn't one).
func uploadDate(v string) string {
	m := dateDigits.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil || m[2] == "00" || m[3] == "00" {
		return ""
	}
	return m[1] + "-" + m[2] + "-" + m[3]
}

// cleanTitle: a file name made readable, without the id archivers append.
func cleanTitle(base string) string {
	t := strings.TrimSpace(bracketIDRe.ReplaceAllString(base, ""))
	if !strings.Contains(t, " ") {
		t = strings.NewReplacer("_", " ", ".", " ").Replace(t)
	}
	if t == "" {
		return base
	}
	return t
}

func cleanTags(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.ReplaceAll(t, "|", " ")), "#"))
		if t == "" || len(t) > 80 || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
		if len(out) >= 40 {
			break
		}
	}
	return out
}

// creatorScan remembers the creators seen in one scan.
type creatorScan struct{ byKey map[string]int64 }

func newCreatorScan() *creatorScan { return &creatorScan{byKey: map[string]int64{}} }

func (s *Scanner) indexCreatorVideo(ctx context.Context, lib *Library, c candidate, cs *creatorScan) error {
	side, sideTime := sidecarFor(c.path)
	mtime := c.mtime
	if sideTime > mtime {
		mtime = sideTime
	}
	existing, err := s.Store.itemByPath(ctx, lib.ID, c.path)
	if err == nil && existing.MTime == mtime && existing.SizeBytes == c.size && !existing.Missing {
		return nil // unchanged
	}
	it := &Item{LibraryID: lib.ID, Kind: "video", Path: c.path, SizeBytes: c.size, MTime: mtime}
	if existing != nil {
		it.ID, it.AddedAt = existing.ID, existing.AddedAt
	}
	base := strings.TrimSuffix(filepath.Base(c.path), filepath.Ext(c.path))
	s.probeVideo(ctx, it)
	vm := videoMeta{}
	folderCreator, folderPlaylist, creatorDir := creatorFromPath(lib, c.path)
	cr := creatorRow{Name: folderCreator, Folder: creatorDir}
	it.Title = cleanTitle(base)
	if side != nil {
		for _, t := range []string{side.Title, side.FullTitle} {
			if t != "" {
				it.Title = t
				break
			}
		}
		it.Overview = side.Description
		if it.Overview == "" {
			it.Overview = side.Overview
		}
		if len(it.Overview) > 20000 {
			it.Overview = it.Overview[:20000]
		}
		it.Genres = side.Categories
		if len(it.Genres) == 0 {
			it.Genres = side.Genres
		}
		it.ContentRating = side.ContentRating
		if it.ContentRating == "" && side.AgeLimit != nil && *side.AgeLimit > 0 {
			it.ContentRating = fmt.Sprintf("%d+", *side.AgeLimit)
		}
		for _, n := range []string{side.Artist, side.Creator, side.Channel, side.Uploader} {
			if n != "" {
				cr.Name = n
				break
			}
		}
		for _, k := range []string{side.ChannelID, side.UploaderID} {
			if k != "" {
				cr.Key = strings.ToLower(k)
				break
			}
		}
		cr.Handle = side.Handle
		if cr.Handle == "" && strings.HasPrefix(side.UploaderID, "@") {
			cr.Handle = side.UploaderID
		}
		for _, u := range []string{side.ChannelURL, side.UploaderURL} {
			if u != "" {
				cr.URL = safeWebURL(u)
				break
			}
		}
		vm.Platform = side.Platform
		if vm.Platform == "" {
			vm.Platform = platformName(side.ExtractorKey, side.Extractor)
		}
		cr.Platform = vm.Platform
		vm.SourceURL = safeWebURL(side.WebpageURL)
		if vm.SourceURL == "" {
			vm.SourceURL = safeWebURL(side.URL)
		}
		vm.UploadDate = uploadDate(side.UploadDate)
		if vm.UploadDate == "" {
			vm.UploadDate = uploadDate(side.Date)
		}
		if vm.UploadDate == "" && side.Timestamp != nil && *side.Timestamp > 0 {
			vm.UploadDate = time.Unix(int64(*side.Timestamp), 0).UTC().Format("2006-01-02")
		}
		vm.Tags = cleanTags(side.Tags)
		if side.ViewCount != nil {
			vm.OrigViews = *side.ViewCount
		}
		if side.LikeCount != nil {
			vm.OrigLikes = *side.LikeCount
		}
		vm.Playlist = side.PlaylistTitle
		if vm.Playlist == "" {
			vm.Playlist = side.Playlist
		}
		if len(side.Custom) > 0 {
			if b, err := json.Marshal(side.Custom); err == nil && len(b) <= 16<<10 {
				vm.Custom = string(b)
			}
		}
	}
	if vm.Playlist == "" && side != nil {
		vm.Playlist = side.Album
	}
	if vm.Playlist == "" {
		vm.Playlist = folderPlaylist
	}
	if lib.Type == "musicvideos" {
		musicVideoNames(it, &cr, &vm, base, side, c.path)
	}
	if vm.UploadDate == "" {
		if t, ok := DateFromName(base); ok {
			vm.UploadDate = time.Unix(t, 0).UTC().Format("2006-01-02")
		}
	}
	if vm.UploadDate == "" {
		vm.UploadDate = time.Unix(c.mtime, 0).UTC().Format("2006-01-02")
	}
	if len(vm.UploadDate) >= 4 {
		y := 0
		if _, err := fmt.Sscanf(vm.UploadDate[:4], "%d", &y); err == nil && y > 0 {
			it.Year = &y
		}
	}
	it.Aired = vm.UploadDate
	if p := localThumb(c.path); p != "" {
		it.Poster = p
	} else {
		it.Poster = LazyPoster
	}
	vm.Vertical = it.Height > it.Width && it.Width > 0

	// What someone typed in stays what they typed.
	var prior *videoMeta
	if existing != nil {
		prior, _ = s.Store.videoMetaOf(ctx, existing.ID)
	}
	if prior != nil && prior.Edited {
		it.Title, it.Overview, it.Genres, it.ContentRating = existing.Title, existing.Overview, existing.Genres, existing.ContentRating
		edited := *prior
		edited.Vertical = vm.Vertical
		vm = edited
		it.Aired = vm.UploadDate
	}
	it.SortTitle = SortTitle(it.Title)
	cid, err := s.Store.ensureCreator(ctx, lib.ID, cr, cs)
	if err != nil {
		return err
	}
	vm.CreatorID = cid
	it.Studio = cr.Name
	if prior != nil && prior.Edited {
		it.Studio = existing.Studio
		if prior.CreatorID != 0 {
			vm.CreatorID = prior.CreatorID
		}
	}
	if err := s.Store.upsertItem(ctx, it); err != nil {
		return err
	}
	if err := s.Store.saveVideoMeta(ctx, it, &vm); err != nil {
		return err
	}
	if side != nil && len(side.Comments) > 0 {
		return s.Store.importComments(ctx, it.ID, side.Comments)
	}
	return nil
}

var artistTitleRe = regexp.MustCompile(`^(.+?)\s+[-–—]\s+(.+)$`)

// musicVideoNames: "Artist - Title.mp4", an .nfo beside the file, or the folder.
func musicVideoNames(it *Item, cr *creatorRow, vm *videoMeta, base string, side *videoSidecar, path string) {
	if side == nil || side.Title == "" {
		if m := artistTitleRe.FindStringSubmatch(cleanTitle(base)); m != nil {
			it.Title = strings.TrimSpace(m[2])
			if side == nil || (side.Artist == "" && side.Creator == "") {
				cr.Name, cr.Key = strings.TrimSpace(m[1]), ""
			}
		}
	}
	if n := readMusicVideoNFO(strings.TrimSuffix(path, filepath.Ext(path)) + ".nfo"); n != nil {
		if n.Title != "" {
			it.Title = n.Title
		}
		if n.Artist != "" {
			cr.Name, cr.Key = n.Artist, ""
		}
		if n.Album != "" {
			vm.Playlist = n.Album
		}
		if n.Plot != "" {
			it.Overview = n.Plot
		}
		if len(n.Genres) > 0 {
			it.Genres = n.Genres
		}
		if len(n.Tags) > 0 {
			vm.Tags = cleanTags(n.Tags)
		}
		if n.Year > 0 {
			y := n.Year
			it.Year = &y
			if vm.UploadDate == "" || side == nil {
				vm.UploadDate = fmt.Sprintf("%04d-01-01", n.Year)
			}
		}
	}
}

type musicVideoNFO struct {
	Title  string   `xml:"title"`
	Artist string   `xml:"artist"`
	Album  string   `xml:"album"`
	Plot   string   `xml:"plot"`
	Year   int      `xml:"year"`
	Genres []string `xml:"genre"`
	Tags   []string `xml:"tag"`
}

// readMusicVideoNFO reads the common sidecar for music videos.
func readMusicVideoNFO(p string) *musicVideoNFO {
	b, err := os.ReadFile(p)
	if err != nil || len(b) > 1<<20 {
		return nil
	}
	var n musicVideoNFO
	if xml.Unmarshal(b, &n) != nil {
		return nil
	}
	return &n
}

// creatorFromPath: Library/Creator/[Playlist/]video.mp4.
func creatorFromPath(lib *Library, p string) (creator, playlist, dir string) {
	for _, root := range lib.Paths {
		rel, err := filepath.Rel(root, filepath.Dir(p))
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		creator, dir = parts[0], filepath.Join(root, parts[0])
		if len(parts) > 1 {
			playlist = parts[len(parts)-1]
		}
		return creator, playlist, dir
	}
	return "", "", ""
}

// localThumb is a picture beside the video with its name.
func localThumb(p string) string {
	stem := strings.TrimSuffix(p, filepath.Ext(p))
	for _, ext := range []string{".jpg", ".jpeg", ".webp", ".png"} {
		for _, name := range []string{stem + ext, stem + "-thumb" + ext, p + ext} {
			if st, err := os.Stat(name); err == nil && !st.IsDir() {
				return name
			}
		}
	}
	return ""
}

// platformName is the site of origin by the name people know it by.
func platformName(keys ...string) string {
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" || strings.EqualFold(k, "generic") {
			continue
		}
		if i := strings.IndexByte(k, ':'); i > 0 {
			k = k[:i]
		}
		k = strings.TrimSuffix(strings.TrimSuffix(k, "Tab"), "Video")
		return k
	}
	return ""
}

// safeWebURL keeps web addresses only: these are shown as links, and a sidecar
// must not be able to plant a "javascript:" one.
func safeWebURL(u string) string {
	u = strings.TrimSpace(u)
	l := strings.ToLower(u)
	if strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://") {
		if len(u) > 2000 {
			return ""
		}
		return u
	}
	return ""
}

// ---------------------------------------------------------------------------
// Storage
// ---------------------------------------------------------------------------

type creatorRow struct {
	Key, Name, Handle, Platform, URL, Folder string
}

// channelSidecar is an optional file in a creator's folder.
type channelSidecar struct {
	Name        string `json:"name"`
	Channel     string `json:"channel"`
	Handle      string `json:"handle"`
	Description string `json:"description"`
	Platform    string `json:"platform"`
	URL         string `json:"url"`
}

func (s *Store) ensureCreator(ctx context.Context, libraryID int64, c creatorRow, cs *creatorScan) (int64, error) {
	if c.Name == "" {
		c.Name = "Unknown creator"
	}
	if c.Key == "" {
		c.Key = strings.ToLower(c.Name)
	}
	if id, ok := cs.byKey[c.Key]; ok {
		return id, nil
	}
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM creators WHERE library_id = ? AND key = ?`, libraryID, c.Key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		// The same person under another key: a video without a sidecar is named
		// after its folder, one with a sidecar after the channel's id.
		err = s.DB.QueryRowContext(ctx, `SELECT id FROM creators WHERE library_id = ? AND (name = ? COLLATE NOCASE OR (folder <> '' AND folder = ?)) ORDER BY id LIMIT 1`, libraryID, c.Name, c.Folder).Scan(&id)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	avatar, banner, description := "", "", ""
	if c.Folder != "" {
		avatar = artIn(c.Folder, "avatar", "channel", "profile", "folder", "poster", "cover")
		banner = artIn(c.Folder, "banner", "fanart", "backdrop", "header")
		for _, n := range []string{"channel.json", "creator.json", "channel.info.json"} {
			b, err := os.ReadFile(filepath.Join(c.Folder, n))
			if err != nil || len(b) > 1<<20 {
				continue
			}
			var m channelSidecar
			if json.Unmarshal(b, &m) == nil {
				description = m.Description
				if m.Handle != "" {
					c.Handle = m.Handle
				}
				if m.Platform != "" {
					c.Platform = m.Platform
				}
				if u := safeWebURL(m.URL); u != "" {
					c.URL = u
				}
				if n := m.Name; n != "" {
					c.Name = n
				} else if m.Channel != "" {
					c.Name = m.Channel
				}
				break
			}
		}
	}
	now := db.Now()
	if id == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO creators(library_id, key, name, handle, description, avatar, banner, platform, url, folder, added_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			libraryID, c.Key, c.Name, c.Handle, description, avatar, banner, c.Platform, c.URL, c.Folder, now, now)
		if err != nil {
			return 0, err
		}
		id, _ = res.LastInsertId()
	} else {
		// Edits stay: only what is empty is filled in.
		if _, err := s.DB.ExecContext(ctx, `UPDATE creators SET
			handle = CASE WHEN handle = '' THEN ? ELSE handle END, description = CASE WHEN description = '' THEN ? ELSE description END,
			avatar = CASE WHEN avatar = '' THEN ? ELSE avatar END, banner = CASE WHEN banner = '' THEN ? ELSE banner END,
			platform = CASE WHEN platform = '' THEN ? ELSE platform END, url = CASE WHEN url = '' THEN ? ELSE url END,
			folder = CASE WHEN folder = '' THEN ? ELSE folder END WHERE id = ?`,
			c.Handle, description, avatar, banner, c.Platform, c.URL, c.Folder, id); err != nil {
			return 0, err
		}
	}
	cs.byKey[c.Key] = id
	return id, nil
}

func artIn(dir string, names ...string) string {
	for _, n := range names {
		for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
			p := filepath.Join(dir, n+ext)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return ""
}

// videoMeta is one row of video_meta.
type videoMeta struct {
	CreatorID  int64
	Platform   string
	SourceURL  string
	UploadDate string
	Tags       []string
	OrigViews  int64
	OrigLikes  int64
	Vertical   bool
	Playlist   string
	Custom     string
	Edited     bool
}

func (s *Store) videoMetaOf(ctx context.Context, id int64) (*videoMeta, error) {
	var v videoMeta
	var tags string
	var creator sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT creator_id, platform, source_url, upload_date, tags, orig_views, orig_likes, vertical, playlist, custom, edited FROM video_meta WHERE item_id = ?`, id).Scan(
		&creator, &v.Platform, &v.SourceURL, &v.UploadDate, &tags, &v.OrigViews, &v.OrigLikes, &v.Vertical, &v.Playlist, &v.Custom, &v.Edited)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v.CreatorID = creator.Int64
	v.Tags = splitList(tags)
	return &v, nil
}

func (s *Store) saveVideoMeta(ctx context.Context, it *Item, v *videoMeta) error {
	var creator any
	if v.CreatorID != 0 {
		creator = v.CreatorID
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO video_meta(item_id, library_id, creator_id, platform, source_url, upload_date, tags, orig_views, orig_likes, vertical, playlist, custom, edited)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(item_id) DO UPDATE SET creator_id=excluded.creator_id, platform=excluded.platform, source_url=excluded.source_url,
		upload_date=excluded.upload_date, tags=excluded.tags, orig_views=excluded.orig_views, orig_likes=excluded.orig_likes, vertical=excluded.vertical,
		playlist=excluded.playlist, custom=excluded.custom, edited=excluded.edited`,
		it.ID, it.LibraryID, creator, v.Platform, v.SourceURL, v.UploadDate, joinList(v.Tags), v.OrigViews, v.OrigLikes, v.Vertical, v.Playlist, v.Custom, v.Edited)
	return err
}

// ---------------------------------------------------------------------------
// Reading
// ---------------------------------------------------------------------------

// Creator is one person or channel in a library.
type Creator struct {
	ID          int64  `json:"id"`
	LibraryID   int64  `json:"libraryId"`
	Name        string `json:"name"`
	Handle      string `json:"handle,omitempty"`
	Description string `json:"description,omitempty"`
	Platform    string `json:"platform,omitempty"`
	URL         string `json:"url,omitempty"`
	HasAvatar   bool   `json:"hasAvatar"`
	HasBanner   bool   `json:"hasBanner"`
	Videos      int    `json:"videos"`
	Subscribed  bool   `json:"subscribed"`
	Subscribers int    `json:"subscribers"`
	UpdatedAt   int64  `json:"updatedAt"`
	LatestAt    string `json:"latestAt,omitempty"`
}

// CreatorQuery lists creators.
type CreatorQuery struct {
	LibraryID  int64
	UserID     int64
	Access     *Access
	Search     string
	Subscribed bool
	ID         int64
}

// Creators lists creators with at least one video the account may see.
func (s *Store) Creators(ctx context.Context, q CreatorQuery) ([]Creator, error) {
	clause, cargs := q.Access.ItemClause()
	w := []string{"1=1"}
	var args []any
	if q.LibraryID != 0 {
		w = append(w, "c.library_id = ?")
		args = append(args, q.LibraryID)
	}
	if q.ID != 0 {
		w = append(w, "c.id = ?")
		args = append(args, q.ID)
	}
	if q.Search != "" {
		w = append(w, "c.name LIKE ?")
		args = append(args, "%"+q.Search+"%")
	}
	if q.Subscribed {
		w = append(w, "c.id IN (SELECT creator_id FROM creator_subs WHERE user_id = ?)")
		args = append(args, q.UserID)
	}
	all := append(append(append([]any{q.UserID}, cargs...), args...), cargs...) // in the order the statement names them
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id, c.library_id, c.name, c.handle, c.description, c.platform, c.url, c.avatar <> '', c.banner <> '', c.updated_at,
		EXISTS(SELECT 1 FROM creator_subs x WHERE x.creator_id = c.id AND x.user_id = ?),
		(SELECT COUNT(*) FROM creator_subs x WHERE x.creator_id = c.id),
		(SELECT COUNT(*) FROM video_meta v JOIN items i ON i.id = v.item_id WHERE v.creator_id = c.id AND i.missing = 0 AND `+clause+`),
		IFNULL((SELECT MAX(v.upload_date) FROM video_meta v WHERE v.creator_id = c.id), '')
		FROM creators c WHERE `+strings.Join(w, " AND ")+` AND EXISTS(SELECT 1 FROM video_meta v JOIN items i ON i.id = v.item_id WHERE v.creator_id = c.id AND i.missing = 0 AND `+clause+`)
		ORDER BY c.name COLLATE NOCASE LIMIT 5000`, all...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Creator{}
	for rows.Next() {
		var c Creator
		if err := rows.Scan(&c.ID, &c.LibraryID, &c.Name, &c.Handle, &c.Description, &c.Platform, &c.URL, &c.HasAvatar, &c.HasBanner, &c.UpdatedAt, &c.Subscribed, &c.Subscribers, &c.Videos, &c.LatestAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreatorArt is the path of a creator's avatar or banner.
func (s *Store) CreatorArt(ctx context.Context, id int64, kind string) (string, error) {
	col := "avatar"
	if kind == "banner" {
		col = "banner"
	}
	var p string
	err := s.DB.QueryRowContext(ctx, `SELECT `+col+` FROM creators WHERE id = ?`, id).Scan(&p)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return p, err
}

// UpdateCreator changes what a creator page says.
func (s *Store) UpdateCreator(ctx context.Context, id int64, name, handle, description, platform, url string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE creators SET name = ?, handle = ?, description = ?, platform = ?, url = ?, updated_at = ? WHERE id = ?`,
		name, handle, description, platform, safeWebURL(url), db.Now(), id)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE items SET studio = ? WHERE id IN (SELECT item_id FROM video_meta WHERE creator_id = ?)`, name, id)
	return err
}

// Subscribe follows or stops following a creator, for one account.
func (s *Store) Subscribe(ctx context.Context, userID, creatorID int64, on bool) error {
	if !on {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM creator_subs WHERE user_id = ? AND creator_id = ?`, userID, creatorID)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO creator_subs(user_id, creator_id, created_at) VALUES(?,?,?)`, userID, creatorID, db.Now())
	return err
}

// VideoCard is one creator video in a list: no file paths, one query's worth of
// counts, and everything a card or a feed page needs.
type VideoCard struct {
	ID          int64    `json:"id"`
	LibraryID   int64    `json:"libraryId"`
	Title       string   `json:"title"`
	DurationSec float64  `json:"durationSec"`
	Width       int      `json:"width"`
	Height      int      `json:"height"`
	Vertical    bool     `json:"vertical"`
	CreatorID   int64    `json:"creatorId"`
	Creator     string   `json:"creator"`
	HasAvatar   bool     `json:"hasAvatar"`
	Platform    string   `json:"platform,omitempty"`
	UploadDate  string   `json:"uploadDate"`
	Tags        []string `json:"tags"`
	Playlist    string   `json:"playlist,omitempty"`
	Views       int64    `json:"views"`     // plays on this server
	OrigViews   int64    `json:"origViews"` // what the source said when archived
	Likes       int64    `json:"likes"`     // likes on this server
	OrigLikes   int64    `json:"origLikes"`
	Comments    int64    `json:"comments"`
	Reaction    int      `json:"reaction"` // this account: 1 liked, -1 disliked, 0 neither
	PositionSec float64  `json:"positionSec"`
	Watched     bool     `json:"watched"`
	UpdatedAt   int64    `json:"updatedAt"`
	Reason      string   `json:"reason,omitempty"` // For You: why this one
}

// VideoQuery picks creator videos.
type VideoQuery struct {
	LibraryID  int64
	UserID     int64
	Access     *Access
	CreatorID  int64
	Playlist   *string
	Tag        string
	Platform   string
	Search     string
	Vertical   *bool
	Long       *bool // true: 20 minutes or more (concerts), false: shorter
	Subscribed bool
	Liked      bool
	History    bool
	IDs        []int64
	Sort       string // latest | oldest | popular | title
	Limit      int
	Offset     int
}

const videoCardCols = `i.id, i.library_id, i.title, IFNULL(i.runtime_sec, 0), IFNULL(i.width, 0), IFNULL(i.height, 0), v.vertical, IFNULL(c.id, 0), IFNULL(c.name, ''), IFNULL(c.avatar, '') <> '',
	v.platform, v.upload_date, v.tags, v.playlist, v.orig_views, v.orig_likes, i.updated_at,
	(SELECT IFNULL(SUM(st.plays), 0) FROM item_stats st WHERE st.item_id = i.id),
	(SELECT COUNT(*) FROM item_reactions r WHERE r.item_id = i.id AND r.value > 0),
	(SELECT COUNT(*) FROM item_comments m WHERE m.item_id = i.id),
	IFNULL((SELECT r.value FROM item_reactions r WHERE r.user_id = ? AND r.item_id = i.id), 0),
	IFNULL(pr.position_sec, 0), IFNULL(pr.watched, 0)`

const videoCardFrom = ` FROM video_meta v JOIN items i ON i.id = v.item_id LEFT JOIN creators c ON c.id = v.creator_id
	LEFT JOIN progress pr ON pr.item_id = i.id AND pr.user_id = ? `

func scanVideoCard(sc interface{ Scan(...any) error }) (VideoCard, error) {
	var x VideoCard
	var tags string
	err := sc.Scan(&x.ID, &x.LibraryID, &x.Title, &x.DurationSec, &x.Width, &x.Height, &x.Vertical, &x.CreatorID, &x.Creator, &x.HasAvatar,
		&x.Platform, &x.UploadDate, &tags, &x.Playlist, &x.OrigViews, &x.OrigLikes, &x.UpdatedAt, &x.Views, &x.Likes, &x.Comments, &x.Reaction, &x.PositionSec, &x.Watched)
	x.Tags = splitList(tags)
	return x, err
}

// Videos returns one page of creator videos, and whether there is more.
func (s *Store) Videos(ctx context.Context, q VideoQuery) ([]VideoCard, bool, error) {
	w := []string{"i.missing = 0"}
	args := []any{q.UserID, q.UserID}
	if q.Access != nil {
		c, a := q.Access.ItemClause()
		w = append(w, c)
		args = append(args, a...)
	}
	if q.LibraryID != 0 {
		w = append(w, "v.library_id = ?")
		args = append(args, q.LibraryID)
	}
	if q.CreatorID != 0 {
		w = append(w, "v.creator_id = ?")
		args = append(args, q.CreatorID)
	}
	if q.Playlist != nil {
		w = append(w, "v.playlist = ?")
		args = append(args, *q.Playlist)
	}
	if q.Tag != "" {
		w = append(w, "('|' || v.tags || '|') LIKE ?")
		args = append(args, "%|"+strings.ReplaceAll(q.Tag, "|", "")+"|%")
	}
	if q.Platform != "" {
		w = append(w, "v.platform = ?")
		args = append(args, q.Platform)
	}
	if q.Search != "" {
		like := "%" + q.Search + "%"
		w = append(w, "(i.title LIKE ? OR i.overview LIKE ? OR c.name LIKE ? OR v.tags LIKE ?)")
		args = append(args, like, like, like, like)
	}
	if q.Vertical != nil {
		w = append(w, "v.vertical = ?")
		args = append(args, *q.Vertical)
	}
	if q.Long != nil {
		if *q.Long {
			w = append(w, "i.runtime_sec >= 1200")
		} else {
			w = append(w, "IFNULL(i.runtime_sec, 0) < 1200")
		}
	}
	if q.Subscribed {
		w = append(w, "v.creator_id IN (SELECT creator_id FROM creator_subs WHERE user_id = ?)")
		args = append(args, q.UserID)
	}
	if q.Liked {
		w = append(w, "i.id IN (SELECT item_id FROM item_reactions WHERE user_id = ? AND value > 0)")
		args = append(args, q.UserID)
	}
	if len(q.IDs) > 0 {
		w = append(w, "i.id IN ("+ids(q.IDs)+")")
	}
	order := "v.upload_date DESC, i.id DESC"
	from := videoCardFrom
	switch q.Sort {
	case "oldest":
		order = "v.upload_date, i.id"
	case "popular":
		order = "(v.orig_views + (SELECT IFNULL(SUM(st.plays), 0) FROM item_stats st WHERE st.item_id = i.id)) DESC, i.id DESC"
	case "title":
		order = "i.sort_title COLLATE NOCASE, i.id"
	}
	if q.History {
		from += ` JOIN item_stats h ON h.item_id = i.id AND h.user_id = ? `
		// The history join comes before the WHERE arguments.
		args = append([]any{q.UserID, q.UserID, q.UserID}, args[2:]...)
		order = "h.last_at DESC"
	}
	limit := q.Limit
	if limit <= 0 || limit > 200 {
		limit = 40
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+videoCardCols+from+` WHERE `+strings.Join(w, " AND ")+` ORDER BY `+order+` LIMIT ? OFFSET ?`, append(args, limit+1, q.Offset)...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []VideoCard{}
	for rows.Next() {
		x, err := scanVideoCard(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

// VideoDetail is one video's page.
type VideoDetail struct {
	VideoCard
	Description   string         `json:"description"`
	SourceURL     string         `json:"sourceUrl,omitempty"`
	Genres        []string       `json:"genres"`
	ContentRating string         `json:"contentRating,omitempty"`
	Custom        map[string]any `json:"custom,omitempty"`
}

// VideoByID loads a video's page for an account that may see its item.
func (s *Store) VideoByID(ctx context.Context, userID int64, it *Item) (*VideoDetail, error) {
	rows, more, err := s.Videos(ctx, VideoQuery{UserID: userID, IDs: []int64{it.ID}, Limit: 1})
	if err != nil {
		return nil, err
	}
	_ = more
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	v, err := s.videoMetaOf(ctx, it.ID)
	if err != nil {
		return nil, err
	}
	d := &VideoDetail{VideoCard: rows[0], Description: it.Overview, SourceURL: v.SourceURL, Genres: it.Genres, ContentRating: it.ContentRating}
	if v.Custom != "" {
		_ = json.Unmarshal([]byte(v.Custom), &d.Custom)
	}
	return d, nil
}

// VideoEdit is custom metadata for one video; nil fields stay as they are.
type VideoEdit struct {
	Platform  *string        `json:"platform"`
	SourceURL *string        `json:"sourceUrl"`
	Date      *string        `json:"uploadDate"`
	Tags      *[]string      `json:"tags"`
	Playlist  *string        `json:"playlist"`
	Creator   *string        `json:"creator"`
	Custom    map[string]any `json:"custom"`
}

// ErrBadInput is a refused value; its text is safe to show.
type ErrBadInput string

func (e ErrBadInput) Error() string { return string(e) }

// EditVideo saves custom metadata: from then on a scan keeps it.
func (s *Store) EditVideo(ctx context.Context, it *Item, e VideoEdit) error {
	v, err := s.videoMetaOf(ctx, it.ID)
	if err != nil {
		return err
	}
	v.Edited = true
	if e.Platform != nil {
		v.Platform = strings.TrimSpace(*e.Platform)
		if len(v.Platform) > 80 {
			return ErrBadInput("that platform name is too long")
		}
	}
	if e.SourceURL != nil {
		u := strings.TrimSpace(*e.SourceURL)
		if u != "" && safeWebURL(u) == "" {
			return ErrBadInput("the source must be a web address (http or https)")
		}
		v.SourceURL = u
	}
	if e.Date != nil {
		if d := strings.TrimSpace(*e.Date); d == "" {
			v.UploadDate = ""
		} else if v.UploadDate = uploadDate(d); v.UploadDate == "" {
			return ErrBadInput("the date must look like 2024-05-31")
		}
	}
	if e.Tags != nil {
		v.Tags = cleanTags(*e.Tags)
	}
	if e.Playlist != nil {
		v.Playlist = strings.TrimSpace(*e.Playlist)
	}
	if e.Custom != nil {
		b, err := json.Marshal(e.Custom)
		if err != nil || len(b) > 16<<10 {
			return ErrBadInput("custom fields are limited to 16 KB")
		}
		v.Custom = string(b)
	}
	if e.Creator != nil {
		name := strings.TrimSpace(*e.Creator)
		if name == "" || len(name) > 200 {
			return ErrBadInput("a creator needs a name of up to 200 characters")
		}
		id, err := s.ensureCreator(ctx, it.LibraryID, creatorRow{Name: name}, newCreatorScan())
		if err != nil {
			return err
		}
		v.CreatorID = id
		it.Studio = name
	}
	it.Aired = v.UploadDate
	if err := s.upsertItem(ctx, it); err != nil {
		return err
	}
	return s.saveVideoMeta(ctx, it, v)
}

// CreatorPlaylists lists a creator's playlists (from folders and sidecars).
func (s *Store) CreatorPlaylists(ctx context.Context, creatorID int64, access *Access) ([]PlaylistSummary, error) {
	clause, cargs := access.ItemClause()
	rows, err := s.DB.QueryContext(ctx, `SELECT v.playlist, COUNT(*), MAX(v.item_id) FROM video_meta v JOIN items i ON i.id = v.item_id
		WHERE v.creator_id = ? AND v.playlist <> '' AND i.missing = 0 AND `+clause+` GROUP BY v.playlist ORDER BY v.playlist COLLATE NOCASE`, append([]any{creatorID}, cargs...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlaylistSummary{}
	for rows.Next() {
		var p PlaylistSummary
		if err := rows.Scan(&p.Name, &p.Count, &p.CoverID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PlaylistSummary is one playlist on a creator's page.
type PlaylistSummary struct {
	Name    string `json:"name"`
	Count   int    `json:"count"`
	CoverID int64  `json:"coverId"`
}

// VideoTags are the most used tags of a library, for the chips above its feed.
func (s *Store) VideoTags(ctx context.Context, libraryID int64, access *Access, limit int) ([]string, error) {
	clause, cargs := access.ItemClause()
	rows, err := s.DB.QueryContext(ctx, `SELECT v.tags FROM video_meta v JOIN items i ON i.id = v.item_id WHERE v.library_id = ? AND i.missing = 0 AND v.tags <> '' AND `+clause+` ORDER BY v.item_id DESC LIMIT 5000`, append([]any{libraryID}, cargs...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	count := map[string]int{}
	for rows.Next() {
		var t string
		if rows.Scan(&t) == nil {
			for _, x := range splitList(t) {
				count[x]++
			}
		}
	}
	return topKeys(count, limit), rows.Err()
}
