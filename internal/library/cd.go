package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittycd"
)

// CD images. A disc is one item (kind cd-image), which is what a shelf shows and
// what the CD player opens. In a music library each of its tracks is an audio
// item as well, so albums, songs, search, queues and every native app work with
// no special case; a track's path is the image's with #track=N on the end. Names
// come from the disc's own text, then MusicBrainz, then the file names.

// CDKind is a CD image's item kind.
const CDKind = "cd-image"

// cdTrackSep joins an image's path and a track number in a track's path.
const cdTrackSep = "#track="

// IsCDTrack says whether an item is a track of a CD image.
func (it *Item) IsCDTrack() bool { return it.CDDisc != nil }

// CDTrackNumber reads a track's number from its path.
func CDTrackNumber(path string) (image string, n int, ok bool) {
	image, v, ok := strings.Cut(path, cdTrackSep)
	if !ok {
		return "", 0, false
	}
	n, err := strconv.Atoi(v)
	return image, n, err == nil
}

// CDInfo is what the server remembers about a disc, besides the item's own
// fields. The layout isn't kept: the sheet is read again when the disc is played.
type CDInfo struct {
	DiscID string `json:"discId"`
	CDDBID string `json:"cddbId,omitempty"`
	Format string `json:"format"`
	// Lookup: "" not asked, matched, ask (several possible, a person chooses),
	// unknown (nobody knows it), failed (couldn't ask; a later scan tries again),
	// off (lookups are turned off) or sidecar (names read from a file beside it).
	Lookup  string `json:"lookup,omitempty"`
	Matches int    `json:"matches,omitempty"`
	// ReleaseID is the MusicBrainz release in use, Disc which disc of it.
	ReleaseID string `json:"releaseId,omitempty"`
	Disc      int    `json:"disc,omitempty"`
	// Source says where the names came from: musicbrainz, sheet (CD-TEXT or the
	// sheet's titles), sidecar or file (the file's own name).
	Source string        `json:"source,omitempty"`
	Tracks []CDTrackName `json:"tracks"`
}

// discoverCD says which of a folder's files are CD images, and which files those
// images use, so they aren't indexed again as songs. cue and toc sheets decide
// for themselves: a sheet beside already tagged per-track files is only a note
// about a rip, not an image.
func cdCandidates(sheets []candidate) (keep []candidate, claimed map[string]bool) {
	claimed = map[string]bool{}
	for _, c := range sheets {
		files, image := wittycd.SheetFiles(c.path)
		if !image {
			continue
		}
		keep = append(keep, c)
		for _, f := range files {
			claimed[f] = true
		}
	}
	return keep, claimed
}

// discoverDiscCDs finds CD images in a folder of discs.
func discoverDiscCDs(root string) []candidate {
	var sheets []candidate
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") || strings.EqualFold(name, "@eaDir")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !wittycd.IsImageName(name) || strings.HasPrefix(name, "._") {
			return nil
		}
		st, err := d.Info()
		if err != nil || !mediaEntry(root, p, d) {
			return nil
		}
		sheets = append(sheets, candidate{path: p, kind: CDKind, size: st.Size(), mtime: st.ModTime().Unix()})
		return nil
	})
	keep, _ := cdCandidates(sheets)
	return keep
}

// dropClaimed takes out what a CD image's sheet has claimed: the song files it
// plays, and an .img or .bin that goes with a .ccd or .cue.
func dropClaimed(c []candidate, claimed map[string]bool, images []candidate) []candidate {
	bases := map[string]bool{}
	for _, im := range images {
		bases[strings.ToLower(strings.TrimSuffix(im.path, filepath.Ext(im.path)))] = true
	}
	out := c[:0]
	for _, x := range c {
		if claimed[x.path] {
			continue
		}
		if wittycd.MayBeRaw(x.path) && bases[strings.ToLower(strings.TrimSuffix(x.path, filepath.Ext(x.path)))] {
			continue
		}
		out = append(out, x)
	}
	return out
}

// genericImageName: names ripping tools give every disc, which say nothing.
var genericImageName = regexp.MustCompile(`(?i)^(cdimage|image|disc|disk|cd|audio|album|rip|range|track ?\d*|cd ?\d+|dis[ck] ?\d+)$`)

var discPartRe = regexp.MustCompile(`(?i)(?:^|[\s_\-\(\[])(?:cd|dis[ck])[\s_\-]*(\d{1,2})(?:$|[\s_\-\)\]])`)

// cdNames works out album and artist from where a disc is kept, for a disc that
// says nothing about itself: Artist/Album/CDImage.cue, Artist - Album.cue.
func cdNames(path string, lib *Library) (album, artist string) {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	dir := filepath.Dir(path)
	folder := filepath.Base(dir)
	inRoot := samePath(dir, lib.Paths...)
	name := base
	if genericImageName.MatchString(strings.TrimSpace(base)) && !inRoot {
		name = folder
		if discPartRe.MatchString(" "+folder+" ") && !samePath(filepath.Dir(dir), lib.Paths...) {
			name = filepath.Base(filepath.Dir(dir)) // Album/CD1/image.cue
			dir = filepath.Dir(dir)
			folder = name
		}
	}
	if a, b, ok := strings.Cut(name, " - "); ok && strings.TrimSpace(a) != "" && strings.TrimSpace(b) != "" {
		return strings.TrimSpace(b), strings.TrimSpace(a)
	}
	album, _ = ParseName(name)
	if album == "" {
		album = name
	}
	if !inRoot && name == folder {
		if parent := filepath.Dir(dir); !samePath(dir, lib.Paths...) && !samePath(parent, lib.Paths...) {
			artist = filepath.Base(parent)
		}
	} else if !inRoot {
		artist = folder
	}
	return album, artist
}

// discNumberOf reads the disc number from a sheet or a name: "Album CD2".
func discNumberOf(d *wittycd.Disc, path string) *int {
	n := d.DiscNumber
	if n <= 0 {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		for _, s := range []string{name, filepath.Base(filepath.Dir(path))} {
			if m := discPartRe.FindStringSubmatch(" " + s + " "); m != nil {
				n, _ = strconv.Atoi(m[1])
				break
			}
		}
	}
	if n <= 0 {
		return nil
	}
	return &n
}

// cdSidecar is the file saved beside a disc's image when saving metadata is on,
// so a disc keeps its names without asking anyone again.
type cdSidecar struct {
	ReleaseID string        `json:"releaseId,omitempty"`
	Disc      int           `json:"disc,omitempty"`
	Title     string        `json:"title"`
	Artist    string        `json:"artist,omitempty"`
	Year      *int          `json:"year,omitempty"`
	Tracks    []CDTrackName `json:"tracks"`
}

func cdSidecarPath(image string) string {
	return strings.TrimSuffix(image, filepath.Ext(image)) + ".wittycd.json"
}

func readCDSidecar(image string) *cdSidecar {
	p := cdSidecarPath(image)
	st, err := os.Stat(p)
	if err != nil || st.Size() > 1<<20 {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var sc cdSidecar
	if json.Unmarshal(b, &sc) != nil || sc.Title == "" {
		return nil
	}
	return &sc
}

// indexCD adds or refreshes a disc and, in a music library, its tracks.
func (s *Scanner) indexCD(ctx context.Context, lib *Library, c candidate) error {
	existing, err := s.Store.itemByPath(ctx, lib.ID, c.path)
	if err == nil && existing.MTime == c.mtime && existing.SizeBytes == c.size && existing.Kind == CDKind && cdInfoOf(existing) != nil && cdInfoOf(existing).DiscID != "" {
		if existing.Missing {
			if err := s.Store.restore(ctx, existing.ID); err != nil {
				return err
			}
			if err := s.Store.restoreCDTracks(ctx, existing.ID); err != nil {
				return err
			}
		}
		// A lookup that couldn't get through last time gets another go.
		if cd := cdInfoOf(existing); cd != nil && cd.Lookup == "failed" && s.cdLookupOn() {
			d, err := wittycd.Open(c.path)
			if err == nil {
				defer d.Close()
				return s.identifyCD(ctx, lib, existing, d)
			}
		}
		return nil
	}
	d, err := wittycd.Open(c.path)
	if err != nil {
		// Not a CD at all, or a data disc: a sheet for a game, a stray .bin. Passed over.
		if errors.Is(err, wittycd.ErrNotCD) || errors.Is(err, wittycd.ErrNoAudio) {
			s.Log.Info("not an audio CD", "cat", "library", "path", c.path, "why", err)
			return nil
		}
		// A disc that can't be read yet stays on the shelf, where opening it says why.
		if !IsAudioLibrary(lib.Type) && existing == nil {
			album, artist := cdNames(c.path, lib)
			broken := &Item{LibraryID: lib.ID, Kind: CDKind, Path: c.path, Title: album, Album: album, Artist: artist, AlbumArtist: artist, SortTitle: SortTitle(album),
				SizeBytes: c.size, MTime: c.mtime, Disc: &DiscInfo{Label: album, Format: "cd", Content: "music", Titles: []DiscTitle{}, CD: &CDInfo{Tracks: []CDTrackName{}}}}
			if err2 := s.Store.upsertItem(ctx, broken); err2 != nil {
				return err2
			}
		}
		return fmt.Errorf("reading CD image: %w", err)
	}
	defer d.Close()
	it := &Item{LibraryID: lib.ID, Kind: CDKind, Path: c.path, SizeBytes: c.size, MTime: c.mtime}
	if existing != nil {
		it.ID, it.AddedAt = existing.ID, existing.AddedAt
		it.PlayMode, it.InsertFX = existing.PlayMode, existing.InsertFX
	}
	toc := d.TOC()
	cd := &CDInfo{DiscID: toc.MusicBrainzID(), CDDBID: toc.CDDBID(), Format: d.Format, Tracks: []CDTrackName{}}
	it.Container, it.VideoCodec = d.Format, "pcm_s16le"
	it.RuntimeSec = d.DurationSec()
	it.DiscNo = discNumberOf(d, c.path)
	album, artist := cdNames(c.path, lib)
	cd.Source = "file"
	if d.Text.Title != "" {
		album, cd.Source = d.Text.Title, "sheet"
	}
	if d.Text.Performer != "" {
		artist = d.Text.Performer
	}
	it.Album, it.Title = album, album
	it.Artist, it.AlbumArtist = artist, artist
	if y, err := strconv.Atoi(yearText(d.Date)); err == nil && y > 1000 {
		it.Year = &y
	}
	if d.Genre != "" {
		it.Genres = []string{d.Genre}
	}
	for _, t := range d.AudioTracks() {
		name := CDTrackName{Number: t.Number, Title: t.Text.Title, Artist: t.Text.Performer}
		if name.Title != "" {
			cd.Source = "sheet"
		}
		if name.Artist == artist {
			name.Artist = ""
		}
		cd.Tracks = append(cd.Tracks, name)
	}
	it.Disc = &DiscInfo{Label: album, Format: d.Format, Content: "music", Titles: []DiscTitle{}, CD: cd}
	// A file beside it from an earlier save says what the disc is.
	if sc := readCDSidecar(c.path); sc != nil {
		it.Album, it.Title = sc.Title, sc.Title
		it.Artist, it.AlbumArtist = sc.Artist, sc.Artist
		if sc.Year != nil {
			it.Year = sc.Year
		}
		cd.ReleaseID, cd.Disc, cd.Source, cd.Lookup = sc.ReleaseID, sc.Disc, "sidecar", "sidecar"
		mergeNames(cd, sc.Tracks)
	}
	// Pictures the owner has beside it come first, then a folder's cover.
	base := strings.TrimSuffix(filepath.Base(c.path), filepath.Ext(c.path))
	dir := filepath.Dir(c.path)
	it.Poster, _ = localArtwork(dir, base)
	if it.Poster == "" {
		it.Poster = albumCover(dir)
	}
	it.SortTitle = SortTitle(it.Title)
	if err := s.Store.upsertItem(ctx, it); err != nil {
		return err
	}
	if cd.Lookup == "" {
		if err := s.identifyCD(ctx, lib, it, d); err != nil {
			s.Log.Info("cd lookup", "cat", "library", "path", c.path, "err", err)
		}
	}
	return s.Store.syncCDTracks(ctx, lib, it, d)
}

func yearText(s string) string {
	if len(s) < 4 {
		return s
	}
	return s[:4]
}

func (s *Scanner) cdLookupOn() bool { return s.MusicBrainzOn == nil || s.MusicBrainzOn() }

func cdInfoOf(it *Item) *CDInfo {
	if it.Disc == nil {
		return nil
	}
	return it.Disc.CD
}

func mergeNames(cd *CDInfo, names []CDTrackName) {
	for _, n := range names {
		found := false
		for i := range cd.Tracks {
			if cd.Tracks[i].Number == n.Number {
				if n.Title != "" {
					cd.Tracks[i].Title = n.Title
				}
				cd.Tracks[i].Artist = n.Artist
				found = true
			}
		}
		_ = found
	}
}

// identifyCD asks MusicBrainz about a disc, using nothing but its table of
// contents. One clear answer is used straight away. Several different ones are
// left for a person to choose between, with the disc's own names until then.
func (s *Scanner) identifyCD(ctx context.Context, lib *Library, it *Item, d *wittycd.Disc) error {
	cd := cdInfoOf(it)
	if cd == nil {
		return errors.New("not a disc")
	}
	if !s.cdLookupOn() {
		cd.Lookup = "off"
		return s.Store.setDiscJSON(ctx, it)
	}
	toc := d.TOC()
	matches, err := (&MusicBrainz{Base: s.MusicBrainzBase, CoverBase: s.CoverArtBase}).Lookup(ctx, cd.DiscID, toc.LookupString(), len(toc.Offsets))
	if err != nil {
		cd.Lookup = "failed"
		_ = s.Store.setDiscJSON(ctx, it)
		return err
	}
	cd.Matches = len(matches)
	switch {
	case len(matches) == 0:
		cd.Lookup = "unknown"
	case len(matches) == 1 && matches[0].Exact, sameAlbum(matches):
		return s.ApplyCDMatch(ctx, lib, it, d, &matches[0])
	default:
		cd.Lookup = "ask"
	}
	return s.Store.setDiscJSON(ctx, it)
}

// ApplyCDMatch puts a release's names and cover on a disc and its tracks.
func (s *Scanner) ApplyCDMatch(ctx context.Context, lib *Library, it *Item, d *wittycd.Disc, m *CDMatch) error {
	cd := cdInfoOf(it)
	if cd == nil {
		return errors.New("not a disc")
	}
	it.Title, it.Album = m.Title, m.Title
	it.SortTitle = SortTitle(it.Title)
	it.Artist, it.AlbumArtist = m.Artist, m.Artist
	if y, err := strconv.Atoi(yearText(m.Date)); err == nil && y > 1000 {
		it.Year = &y
	}
	it.Provider, it.ProviderID = "musicbrainz", m.ReleaseID
	if m.Discs > 1 {
		it.DiscNo = &m.Disc
	}
	cd.Lookup, cd.ReleaseID, cd.Disc, cd.Source, cd.Matches = "matched", m.ReleaseID, m.Disc, "musicbrainz", 0
	// Track numbers on a disc that starts with data are off by the data tracks.
	audio := d.AudioTracks()
	for i, t := range m.Tracks {
		if i < len(audio) {
			mergeNames(cd, []CDTrackName{{Number: audio[i].Number, Title: t.Title, Artist: t.Artist}})
		}
	}
	// A cover only when the owner hasn't got one of their own.
	if it.Poster == "" || strings.HasPrefix(it.Poster, "cache:cd-") {
		if p := s.fetchCDCover(ctx, m.ReleaseID); p != "" {
			it.Poster = p
		}
	}
	if err := s.Store.upsertItem(ctx, it); err != nil {
		return err
	}
	if err := s.Store.syncCDTracks(ctx, lib, it, d); err != nil {
		return err
	}
	s.saveCDSidecar(ctx, it)
	return nil
}

// fetchCDCover downloads a release's cover into the artwork cache.
func (s *Scanner) fetchCDCover(ctx context.Context, release string) string {
	if !validMBID(release) {
		return ""
	}
	name := "cd-" + release + ".jpg"
	out := filepath.Join(s.ImagesDir, name)
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return "cache:" + name
	}
	b, err := (&MusicBrainz{Base: s.MusicBrainzBase, CoverBase: s.CoverArtBase}).Cover(ctx, release, 500)
	if err != nil || !isPicture(b) {
		return ""
	}
	if err := os.MkdirAll(s.ImagesDir, 0o755); err != nil {
		return ""
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		return ""
	}
	return "cache:" + name
}

// saveCDSidecar writes what is known about a disc beside it, only when the owner
// has said metadata may be written beside their files.
func (s *Scanner) saveCDSidecar(ctx context.Context, it *Item) {
	if s.SaveMeta == nil || !s.SaveMeta() {
		return
	}
	cd := cdInfoOf(it)
	if cd == nil {
		return
	}
	sc := cdSidecar{ReleaseID: cd.ReleaseID, Disc: cd.Disc, Title: it.Title, Artist: it.Artist, Year: it.Year, Tracks: cd.Tracks}
	b, err := json.MarshalIndent(sc, "", "  ")
	if err == nil {
		err = writeBeside(cdSidecarPath(it.Path), append(b, '\n'))
	}
	if err != nil {
		s.Log.Debug("save cd metadata", "path", it.Path, "err", err)
	}
	// And the cover, for other software and for the next rescan.
	if strings.HasPrefix(it.Poster, "cache:cd-") {
		base := strings.TrimSuffix(it.Path, filepath.Ext(it.Path))
		dst := base + "-poster.jpg"
		if _, err := os.Stat(dst); err != nil {
			if err := s.writeArt(ctx, it.Poster, dst); err == nil {
				it.Poster = dst
				_ = s.Store.setPoster(ctx, it.ID, dst)
				_, _ = s.Store.DB.ExecContext(ctx, `UPDATE items SET poster = ? WHERE cd_disc = ?`, dst, it.ID)
			}
		}
	}
}

// syncCDTracks makes the track items of a disc in a music library match it.
func (st *Store) syncCDTracks(ctx context.Context, lib *Library, it *Item, d *wittycd.Disc) error {
	if !IsAudioLibrary(lib.Type) {
		return nil
	}
	cd := cdInfoOf(it)
	want := map[string]bool{}
	names := map[int]CDTrackName{}
	if cd != nil {
		for _, n := range cd.Tracks {
			names[n.Number] = n
		}
	}
	for _, t := range d.AudioTracks() {
		path := fmt.Sprintf("%s%s%d", it.Path, cdTrackSep, t.Number)
		want[path] = true
		n := t.Number
		name := names[n]
		title := name.Title
		if title == "" {
			title = fmt.Sprintf("Track %02d", n)
		}
		artist := name.Artist
		if artist == "" {
			artist = it.Artist
		}
		parent := it.ID
		ti := &Item{LibraryID: lib.ID, Kind: "audio", Path: path, Title: title, SortTitle: SortTitle(title), Year: it.Year, Genres: it.Genres,
			RuntimeSec: float64(t.End-t.Start) / wittycd.FramesPerSecond, Poster: it.Poster, Container: "cd", VideoCodec: "pcm_s16le", MTime: it.MTime,
			Artist: artist, Album: it.Album, AlbumArtist: it.AlbumArtist, Track: &n, DiscNo: it.DiscNo, CDDisc: &parent}
		if old, err := st.itemByPath(ctx, lib.ID, path); err == nil {
			ti.ID, ti.AddedAt = old.ID, old.AddedAt
		}
		if err := st.upsertItem(ctx, ti); err != nil {
			return err
		}
	}
	rows, err := st.DB.QueryContext(ctx, `SELECT id, path FROM items WHERE cd_disc = ?`, it.ID)
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
		if !want[p] {
			gone = append(gone, id)
		}
	}
	rows.Close()
	for _, id := range gone {
		if _, err := st.DB.ExecContext(ctx, `UPDATE items SET missing = 1 WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

func (st *Store) restoreCDTracks(ctx context.Context, disc int64) error {
	_, err := st.DB.ExecContext(ctx, `UPDATE items SET missing = 0 WHERE cd_disc = ?`, disc)
	return err
}

// setDiscJSON saves just what the server remembers about a disc.
func (st *Store) setDiscJSON(ctx context.Context, it *Item) error {
	b, err := json.Marshal(it.Disc)
	if err != nil {
		return err
	}
	_, err = st.DB.ExecContext(ctx, `UPDATE items SET disc_json = ? WHERE id = ?`, string(b), it.ID)
	return err
}

// CDTrackItems maps a disc's track numbers to their items.
func (st *Store) CDTrackItems(ctx context.Context, disc int64) map[int]int64 {
	out := map[int]int64{}
	rows, err := st.DB.QueryContext(ctx, `SELECT id, track FROM items WHERE cd_disc = ? AND missing = 0 AND track IS NOT NULL`, disc)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n int
		if rows.Scan(&id, &n) == nil {
			out[n] = id
		}
	}
	return out
}

// ForgetCDMatch goes back to what the disc says about itself.
func (s *Scanner) ForgetCDMatch(ctx context.Context, lib *Library, it *Item, d *wittycd.Disc) error {
	cd := cdInfoOf(it)
	if cd == nil {
		return errors.New("not a disc")
	}
	album, artist := cdNames(it.Path, lib)
	if d.Text.Title != "" {
		album = d.Text.Title
	}
	if d.Text.Performer != "" {
		artist = d.Text.Performer
	}
	it.Title, it.Album, it.SortTitle = album, album, SortTitle(album)
	it.Artist, it.AlbumArtist = artist, artist
	it.Provider, it.ProviderID = "", ""
	cd.Lookup, cd.ReleaseID, cd.Disc, cd.Source, cd.Matches = "", "", 0, "file", 0
	cd.Tracks = cd.Tracks[:0]
	for _, t := range d.AudioTracks() {
		name := CDTrackName{Number: t.Number, Title: t.Text.Title, Artist: t.Text.Performer}
		if name.Title != "" {
			cd.Source = "sheet"
		}
		cd.Tracks = append(cd.Tracks, name)
	}
	if strings.HasPrefix(it.Poster, "cache:cd-") {
		it.Poster = ""
	}
	if err := s.Store.upsertItem(ctx, it); err != nil {
		return err
	}
	_ = os.Remove(cdSidecarPathIfSaving(s, it.Path))
	return s.Store.syncCDTracks(ctx, lib, it, d)
}

// cdSidecarPathIfSaving names the sidecar to remove only when writing beside the
// owner's files is allowed; otherwise nothing there is touched.
func cdSidecarPathIfSaving(s *Scanner, image string) string {
	if s.SaveMeta == nil || !s.SaveMeta() {
		return ""
	}
	return cdSidecarPath(image)
}

func sortStable[T any](s []T, less func(a, b *T) bool) {
	sort.SliceStable(s, func(i, j int) bool { return less(&s[i], &s[j]) })
}

func limitBody(res *http.Response, n int64) io.Reader { return io.LimitReader(res.Body, n) }

// cdIdentifyTimeout bounds a person's "find this album".
const cdIdentifyTimeout = 45 * time.Second

// CDLookup is what a person's "find this album" asks.
func (s *Scanner) CDLookup(ctx context.Context, d *wittycd.Disc) ([]CDMatch, error) {
	cctx, cancel := context.WithTimeout(ctx, cdIdentifyTimeout)
	defer cancel()
	toc := d.TOC()
	return (&MusicBrainz{Base: s.MusicBrainzBase, CoverBase: s.CoverArtBase}).Lookup(cctx, toc.MusicBrainzID(), toc.LookupString(), len(toc.Offsets))
}

// CDRelease finds the release a person picked.
func (s *Scanner) CDRelease(ctx context.Context, d *wittycd.Disc, release string) (*CDMatch, error) {
	cctx, cancel := context.WithTimeout(ctx, cdIdentifyTimeout)
	defer cancel()
	toc := d.TOC()
	return (&MusicBrainz{Base: s.MusicBrainzBase, CoverBase: s.CoverArtBase}).Release(cctx, release, toc.MusicBrainzID(), len(toc.Offsets))
}

// CDCoverPreview fetches a release's cover small, for the choosing dialog, so
// the browser doesn't have to ask another site for it.
func (s *Scanner) CDCoverPreview(ctx context.Context, release string) ([]byte, error) {
	return (&MusicBrainz{Base: s.MusicBrainzBase, CoverBase: s.CoverArtBase}).Cover(ctx, release, 250)
}
