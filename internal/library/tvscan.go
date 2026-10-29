package library

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// tvScan carries per-scan state for a shows library: shows already resolved
// and TheTVDB episode lists, so each is looked up once per scan.
type tvScan struct {
	shows    map[string]*Show
	episodes map[int64]map[[2]int]EpisodeMeta
}

func newTVScan() *tvScan {
	return &tvScan{shows: map[string]*Show{}, episodes: map[int64]map[[2]int]EpisodeMeta{}}
}

func (s *Scanner) tvdb() *TVDB {
	if s.TVDBKey == nil {
		return nil
	}
	key, pin := s.TVDBKey()
	if key == "" {
		return nil
	}
	return &TVDB{Key: key, PIN: pin}
}

// rootFor is the library folder that contains p (longest match).
func rootFor(roots []string, p string) string {
	best := ""
	for _, r := range roots {
		rel, err := filepath.Rel(r, p)
		if err == nil && !strings.HasPrefix(rel, "..") && len(r) > len(best) {
			best = r
		}
	}
	if best == "" {
		return filepath.Dir(p)
	}
	return best
}

// indexEpisode indexes one file of a shows library.
func (s *Scanner) indexEpisode(ctx context.Context, lib *Library, c candidate, tv *tvScan) error {
	root := rootFor(lib.Paths, c.path)
	ref, ok := ParseEpisode(root, c.path)
	if !ok {
		// Unnumbered file: keep it under its show folder as an extra.
		rel, _ := filepath.Rel(root, c.path)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		name := strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(c.path))
		ref = EpisodeRef{Title: cleanEpisodeTitle(name)}
		if len(parts) > 1 {
			ref.Show, ref.ShowYear = ParseName(parts[0])
			ref.ShowDir = filepath.Join(root, filepath.FromSlash(parts[0]))
		} else {
			ref.Show, ref.ShowYear = ParseName(name)
			ref.ShowDir = filepath.Join(root, ref.Show)
		}
		ref.Season, ref.Episode = -1, -1
	}
	sh, err := s.tvShow(ctx, lib, ref, tv)
	if err != nil {
		return err
	}
	existing, err := s.Store.itemByPath(ctx, lib.ID, c.path)
	if err == nil && existing.MTime == c.mtime && existing.SizeBytes == c.size && existing.Poster != "" && !existing.Missing &&
		existing.ShowID != nil && *existing.ShowID == sh.ID && (s.tvdb() == nil || existing.Provider == "tvdb" || sh.Provider != "tvdb") {
		return nil // unchanged
	}
	it := &Item{LibraryID: lib.ID, Kind: c.kind, Path: c.path, SizeBytes: c.size, MTime: c.mtime, ShowID: &sh.ID}
	if existing != nil {
		it.ID, it.AddedAt = existing.ID, existing.AddedAt
	}
	if ref.Season >= 0 {
		season, ep := ref.Season, ref.Episode
		it.Season, it.Episode = &season, &ep
		if ref.EpisodeEnd > 0 {
			end := ref.EpisodeEnd
			it.EpisodeEnd = &end
		}
	}
	it.Title = ref.Title
	switch c.kind {
	case "video":
		s.probeVideo(ctx, it)
	default:
		s.probeDisc(ctx, it)
	}
	if ref.Title != "" || it.Title == "" {
		it.Title = ref.Title
	}

	dir := filepath.Dir(c.path)
	base := strings.TrimSuffix(filepath.Base(c.path), filepath.Ext(c.path))
	it.Poster = findArt(dir, base+"-thumb", base)
	fromNFO := applyEpisodeNFO(it, filepath.Join(dir, base+".nfo"))
	for _, sc := range []string{c.path + ".metadata.json", filepath.Join(dir, base+".metadata.json")} {
		if applySidecarJSON(it, sc) {
			fromNFO = true
			break
		}
	}
	// Season artwork from the season folder or the show folder.
	if it.Season != nil {
		n := *it.Season
		local := ""
		if !strings.EqualFold(filepath.Clean(dir), filepath.Clean(sh.Path)) {
			local = findArt(dir, "poster", "folder", "cover")
		}
		if local == "" {
			name := fmt.Sprintf("season%02d", n)
			if n == 0 {
				name = "season-specials"
			}
			local = findArt(sh.Path, name+"-poster", name)
		}
		if local != "" {
			_ = s.Store.SetSeasonPoster(ctx, sh.ID, n, local, true)
		}
	}
	// TheTVDB episode data.
	if it.Season != nil && sh.Provider == "tvdb" {
		if eps := s.tvEpisodes(ctx, sh, tv); eps != nil {
			applyEpisodeMeta(it, eps, fromNFO)
		}
	}
	if it.Title == "" && it.Episode != nil {
		it.Title = "Episode " + strconv.Itoa(*it.Episode)
	}
	if it.Year == nil {
		it.Year = yearOf(it.Aired)
	}
	it.SortTitle = SortTitle(it.Title)
	if it.Poster == "" {
		if p, err := s.thumbnail(ctx, it); err == nil {
			it.Poster = p
		} else {
			s.Log.Debug("thumbnail", "path", c.path, "err", err)
		}
	}
	it.ShowTitle = sh.Title
	s.saveOnScan(ctx, it)
	return s.Store.upsertItem(ctx, it)
}

// applyEpisodeMeta fills an episode from TheTVDB. Titles parsed from file
// names are replaced (they are approximations); NFO/sidecar values are kept.
func applyEpisodeMeta(it *Item, eps map[[2]int]EpisodeMeta, keepText bool) {
	e, ok := eps[[2]int{*it.Season, *it.Episode}]
	if !ok {
		return
	}
	it.Provider, it.ProviderID = "tvdb", e.ID
	if !keepText || it.Title == "" {
		it.Title = e.Title
		if it.EpisodeEnd != nil {
			if e2, ok := eps[[2]int{*it.Season, *it.EpisodeEnd}]; ok && e2.Title != "" && e2.Title != e.Title {
				it.Title = e.Title + " / " + e2.Title
			}
		}
	}
	if !keepText || it.Overview == "" {
		if e.Overview != "" {
			it.Overview = e.Overview
		}
	}
	if e.Aired != "" {
		it.Aired = e.Aired
	}
	if (it.Poster == "" || isRemote(it.Poster) || strings.HasPrefix(it.Poster, "cache:thumb-")) && e.Image != "" {
		it.Poster = e.Image
	}
	if it.RuntimeSec == 0 && e.RuntimeMin > 0 {
		it.RuntimeSec = float64(e.RuntimeMin) * 60
	}
}

func isRemote(p string) bool {
	return strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://")
}

// findArt returns the first existing image named name.{jpg,jpeg,png,webp}.
func findArt(dir string, names ...string) string {
	for _, n := range names {
		for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".tbn"} {
			p := filepath.Join(dir, n+ext)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return ""
}

// tvShow resolves (and on first sight creates) the show for an episode.
func (s *Scanner) tvShow(ctx context.Context, lib *Library, ref EpisodeRef, tv *tvScan) (*Show, error) {
	if sh := tv.shows[ref.ShowDir]; sh != nil {
		return sh, nil
	}
	sh, err := s.Store.showByPath(ctx, lib.ID, ref.ShowDir)
	if err != nil {
		sh = &Show{LibraryID: lib.ID, Path: ref.ShowDir, Title: ref.Show, Year: ref.ShowYear}
	}
	base := filepath.Base(ref.ShowDir)
	if p := findArt(ref.ShowDir, "poster", "folder", "cover", "show", base); p != "" {
		sh.Poster = p
	}
	if b := findArt(ref.ShowDir, "fanart", "backdrop", "background", "landscape"); b != "" {
		sh.Backdrop = b
	}
	fromNFO := applyShowNFO(sh, filepath.Join(ref.ShowDir, "tvshow.nfo"))
	if t := s.tvdb(); t != nil && (sh.Provider != "tvdb" || sh.Overview == "") {
		var meta *SeriesMeta
		var err error
		if sh.Provider == "tvdb" && sh.ProviderID != "" {
			meta, err = t.Series(ctx, sh.ProviderID)
		} else {
			meta, err = t.FindSeries(ctx, sh.Title, sh.Year)
		}
		if err != nil {
			s.Log.Info("tvdb lookup failed", "show", sh.Title, "err", err)
		} else {
			applySeriesMeta(sh, meta, fromNFO, false)
		}
		if err := s.Store.UpsertShow(ctx, sh); err != nil {
			return nil, err
		}
		if meta != nil {
			for n, p := range meta.SeasonPosters {
				_ = s.Store.SetSeasonPoster(ctx, sh.ID, n, p, false)
			}
		}
	} else if err := s.Store.UpsertShow(ctx, sh); err != nil {
		return nil, err
	}
	s.saveShowOnScan(ctx, sh)
	tv.shows[ref.ShowDir] = sh
	return sh, nil
}

func (s *Scanner) tvEpisodes(ctx context.Context, sh *Show, tv *tvScan) map[[2]int]EpisodeMeta {
	if eps, ok := tv.episodes[sh.ID]; ok {
		return eps
	}
	var eps map[[2]int]EpisodeMeta
	if t := s.tvdb(); t != nil && sh.ProviderID != "" {
		var err error
		if eps, err = t.Episodes(ctx, sh.ProviderID); err != nil {
			s.Log.Info("tvdb episodes failed", "show", sh.Title, "err", err)
		}
	}
	tv.episodes[sh.ID] = eps
	return eps
}

// applySeriesMeta merges TheTVDB data into a show. force replaces text and
// remote artwork (a manual refresh); local files are never replaced.
func applySeriesMeta(sh *Show, m *SeriesMeta, keepText, force bool) {
	sh.Provider, sh.ProviderID = "tvdb", m.ID
	if (!keepText || force) && m.Title != "" {
		sh.Title, sh.SortTitle = m.Title, SortTitle(m.Title)
	}
	if (sh.Overview == "" || force) && m.Overview != "" {
		sh.Overview = m.Overview
	}
	if (sh.Year == nil || force) && m.Year != nil {
		sh.Year = m.Year
	}
	if (len(sh.Genres) == 0 || force) && len(m.Genres) > 0 {
		sh.Genres = m.Genres
	}
	if (sh.ContentRating == "" || force) && m.ContentRating != "" {
		sh.ContentRating = m.ContentRating
	}
	if (sh.Network == "" || force) && m.Network != "" {
		sh.Network = m.Network
	}
	if m.Status != "" {
		sh.Status = m.Status
	}
	if (sh.Poster == "" || isRemote(sh.Poster)) && m.Poster != "" {
		sh.Poster = m.Poster
	}
	if (sh.Backdrop == "" || isRemote(sh.Backdrop)) && m.Backdrop != "" {
		sh.Backdrop = m.Backdrop
	}
}

// RefreshShow re-reads a show from TheTVDB (optionally forcing a series id)
// and re-applies episode data to all of its episodes.
func (s *Scanner) RefreshShow(ctx context.Context, showID int64, tvdbID string) (*Show, error) {
	t := s.tvdb()
	if t == nil {
		return nil, fmt.Errorf("add a TheTVDB API key in Settings first")
	}
	sh, err := s.Store.Show(ctx, showID, 0)
	if err != nil {
		return nil, err
	}
	var meta *SeriesMeta
	switch {
	case tvdbID != "":
		meta, err = t.Series(ctx, tvdbID)
	case sh.Provider == "tvdb" && sh.ProviderID != "":
		meta, err = t.Series(ctx, sh.ProviderID)
	default:
		meta, err = t.FindSeries(ctx, sh.Title, sh.Year)
	}
	if err != nil {
		return nil, err
	}
	applySeriesMeta(sh, meta, false, true)
	if err := s.Store.UpsertShow(ctx, sh); err != nil {
		return nil, err
	}
	for n, p := range meta.SeasonPosters {
		_ = s.Store.SetSeasonPoster(ctx, sh.ID, n, p, false)
	}
	eps, err := t.Episodes(ctx, meta.ID)
	if err != nil {
		return sh, err
	}
	items, err := s.Store.Episodes(ctx, sh.ID, nil)
	if err != nil {
		return sh, err
	}
	for _, it := range items {
		if it.Season == nil || it.Episode == nil {
			continue
		}
		applyEpisodeMeta(it, eps, false)
		if it.Year == nil {
			it.Year = yearOf(it.Aired)
		}
		it.SortTitle = SortTitle(it.Title)
		if err := s.Store.upsertItem(ctx, it); err != nil {
			return sh, err
		}
	}
	return s.Store.Show(ctx, showID, 0)
}

// Kodi tvshow.nfo / episode NFO.

type nfoShow struct {
	XMLName   xml.Name `xml:"tvshow"`
	Title     string   `xml:"title"`
	Year      int      `xml:"year"`
	Plot      string   `xml:"plot"`
	Genres    []string `xml:"genre"`
	MPAA      string   `xml:"mpaa"`
	Studio    string   `xml:"studio"`
	Premiered string   `xml:"premiered"`
	Status    string   `xml:"status"`
	Rating    float64  `xml:"rating"`
	TVDBID    string   `xml:"tvdbid"`
	UniqueIDs []struct {
		Type string `xml:"type,attr"`
		ID   string `xml:",chardata"`
	} `xml:"uniqueid"`
}

func applyShowNFO(sh *Show, path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if i := strings.LastIndex(string(b), "</tvshow>"); i > 0 {
		b = b[:i+len("</tvshow>")]
	}
	var m nfoShow
	if xml.Unmarshal(b, &m) != nil {
		return false
	}
	if m.Title != "" {
		sh.Title, sh.SortTitle = m.Title, SortTitle(m.Title)
	}
	if m.Year != 0 {
		y := m.Year
		sh.Year = &y
	} else if y := yearOf(m.Premiered); y != nil {
		sh.Year = y
	}
	if m.Plot != "" {
		sh.Overview = m.Plot
	}
	if len(m.Genres) > 0 {
		sh.Genres = m.Genres
	}
	if m.MPAA != "" {
		sh.ContentRating = m.MPAA
	}
	if m.Studio != "" {
		sh.Network = m.Studio
	}
	if m.Status != "" {
		sh.Status = m.Status
	}
	if m.Rating > 0 {
		r := m.Rating
		sh.Rating = &r
	}
	id := m.TVDBID
	for _, u := range m.UniqueIDs {
		if u.Type == "tvdb" && strings.TrimSpace(u.ID) != "" {
			id = strings.TrimSpace(u.ID)
		}
	}
	if id != "" && sh.Provider == "" {
		sh.Provider, sh.ProviderID = "tvdb", id
	}
	return m.Title != ""
}

type nfoEpisode struct {
	XMLName xml.Name `xml:"episodedetails"`
	Title   string   `xml:"title"`
	Plot    string   `xml:"plot"`
	Aired   string   `xml:"aired"`
	Season  *int     `xml:"season"`
	Episode *int     `xml:"episode"`
	Rating  float64  `xml:"rating"`
	Thumb   []string `xml:"thumb"`
}

func applyEpisodeNFO(it *Item, path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// Multi-episode NFOs hold several <episodedetails>; use the first.
	if i := strings.Index(string(b), "</episodedetails>"); i > 0 {
		b = b[:i+len("</episodedetails>")]
	}
	var m nfoEpisode
	if xml.Unmarshal(b, &m) != nil {
		return false
	}
	if m.Title != "" {
		it.Title = m.Title
	}
	if m.Plot != "" {
		it.Overview = m.Plot
	}
	if m.Aired != "" {
		it.Aired = m.Aired
	}
	if m.Season != nil && m.Episode != nil {
		it.Season, it.Episode = m.Season, m.Episode
	}
	if m.Rating > 0 {
		r := m.Rating
		it.Rating = &r
	}
	if it.Poster == "" {
		for _, t := range m.Thumb {
			if isRemote(t) {
				it.Poster = t
				break
			}
		}
	}
	return m.Title != "" || m.Plot != ""
}
