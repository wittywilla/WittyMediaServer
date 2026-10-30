package library

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittydisc/isofs"
	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

var videoExts = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".mov": true, ".avi": true, ".wmv": true, ".ts": true, ".m2ts": true,
	".mts": true, ".webm": true, ".flv": true, ".mpg": true, ".mpeg": true, ".vob": true, ".ogv": true, ".3gp": true,
}

// Scanner walks library paths and keeps items up to date.
type Scanner struct {
	Store    *Store
	Tools    *ffmpeg.Tools
	CacheDir string
	// ImagesDir holds downloaded and generated artwork; "cache:<name>"
	// references are relative to it, so it can be moved.
	ImagesDir string
	TMDBKey   func() string
	// TVDBKey returns TheTVDB API key and optional subscriber PIN.
	TVDBKey func() (key, pin string)
	// TVMazeOn reports whether TVmaze may be used when there is no TheTVDB key.
	TVMazeOn func() bool
	// WikidataOn: Wikidata may fill in movies when there is no TMDB key.
	WikidataOn func() bool
	// AnimeTitles: english | romaji | native titles for anime libraries.
	AnimeTitles func() string
	// SaveMeta reports whether to write NFO/artwork beside the media.
	SaveMeta func() bool
	Log      *slog.Logger

	mu      sync.Mutex
	running map[int64]context.CancelFunc
	// Progress for the UI.
	Progress map[int64]*ScanProgress
}

type ScanProgress struct {
	LibraryID int64  `json:"libraryId"`
	Phase     string `json:"phase"`
	Current   string `json:"current"`
	Found     int    `json:"found"`
	Done      int    `json:"done"`
	StartedAt int64  `json:"startedAt"`
}

func NewScanner(store *Store, tools *ffmpeg.Tools, cacheDir string, tmdbKey func() string, log *slog.Logger) *Scanner {
	return &Scanner{Store: store, Tools: tools, CacheDir: cacheDir, ImagesDir: filepath.Join(cacheDir, "images"), TMDBKey: tmdbKey, Log: log, running: map[int64]context.CancelFunc{}, Progress: map[int64]*ScanProgress{}}
}

// Start scans a library in the background (no-op if already scanning).
func (s *Scanner) Start(libraryID int64) bool {
	s.mu.Lock()
	if _, ok := s.running[libraryID]; ok {
		s.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.running[libraryID] = cancel
	s.Progress[libraryID] = &ScanProgress{LibraryID: libraryID, Phase: "starting", StartedAt: time.Now().Unix()}
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.running, libraryID)
			delete(s.Progress, libraryID)
			s.mu.Unlock()
		}()
		if err := s.scan(ctx, libraryID); err != nil {
			s.Log.Error("scan failed", "library", libraryID, "err", err)
		}
	}()
	return true
}

// Cancel stops a running scan.
func (s *Scanner) Cancel(libraryID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.running[libraryID]; ok {
		c()
	}
}

func (s *Scanner) progress(id int64, f func(p *ScanProgress)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p := s.Progress[id]; p != nil {
		f(p)
	}
}

// Snapshot returns a copy of the progress for a library, or nil when idle.
func (s *Scanner) Snapshot(id int64) *ScanProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p := s.Progress[id]; p != nil {
		cp := *p
		return &cp
	}
	return nil
}

type candidate struct {
	path  string
	kind  string
	size  int64
	mtime int64
}

// discover lists playable things under a root: video files, disc images and
// VIDEO_TS folders (a folder containing VIDEO_TS becomes one item).
func discover(root string) ([]candidate, error) {
	var out []candidate
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || strings.EqualFold(name, "@eaDir") || strings.EqualFold(name, "extras") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(p, "VIDEO_TS", "VIDEO_TS.IFO")); err == nil {
				st, _ := d.Info()
				out = append(out, candidate{path: p, kind: "dvd-folder", mtime: st.ModTime().Unix()})
				return filepath.SkipDir
			}
			if strings.EqualFold(name, "VIDEO_TS") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(p, "BDMV", "index.bdmv")); err == nil {
				st, _ := d.Info()
				out = append(out, candidate{path: p, kind: "bluray-folder", mtime: st.ModTime().Unix()})
				return filepath.SkipDir
			}
			if strings.EqualFold(name, "BDMV") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		st, err := d.Info()
		if err != nil {
			return nil
		}
		switch {
		case ext == ".iso" || ext == ".img":
			out = append(out, candidate{path: p, kind: "dvd-iso", size: st.Size(), mtime: st.ModTime().Unix()})
		case videoExts[ext]:
			if strings.HasPrefix(strings.ToLower(name), "sample") || st.Size() < 5<<20 {
				return nil
			}
			out = append(out, candidate{path: p, kind: "video", size: st.Size(), mtime: st.ModTime().Unix()})
		}
		return nil
	})
	return out, err
}

func (s *Scanner) scan(ctx context.Context, libraryID int64) error {
	lib, err := s.Store.Library(ctx, libraryID)
	if err != nil {
		return err
	}
	s.Store.setScanning(ctx, libraryID, true)
	defer s.Store.setScanning(context.Background(), libraryID, false)
	s.Log.Info("scan start", "library", lib.Name)

	var all []candidate
	for _, root := range lib.Paths {
		s.progress(libraryID, func(p *ScanProgress) { p.Phase = "discovering"; p.Current = root })
		find := discover
		switch {
		case IsAudioLibrary(lib.Type):
			find = discoverAudio
		case lib.Type == "books":
			find = discoverBooks
		}
		c, err := find(root)
		if err != nil {
			s.Log.Warn("discover", "root", root, "err", err)
		}
		all = append(all, c...)
	}
	s.progress(libraryID, func(p *ScanProgress) { p.Phase = "indexing"; p.Found = len(all) })
	keep := map[string]bool{}
	tv := newTVScan()
	covers := map[string]string{} // audio: one cover per folder+album
	for i, c := range all {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		keep[c.path] = true
		s.progress(libraryID, func(p *ScanProgress) { p.Current = filepath.Base(c.path); p.Done = i })
		var err error
		switch {
		case lib.Type == "shows" || lib.Type == "anime":
			err = s.indexEpisode(ctx, lib, c, tv)
		case IsAudioLibrary(lib.Type):
			err = s.indexAudio(ctx, lib, c, covers)
		case lib.Type == "books":
			err = s.indexBook(ctx, lib, c)
		default:
			err = s.index(ctx, lib, c)
		}
		if err != nil {
			s.Log.Warn("index failed", "path", c.path, "err", err)
		} else {
			s.Log.Info("indexed", "cat", "library", "library", lib.Name, "path", c.path)
		}
	}
	if err := s.Store.markMissing(ctx, libraryID, keep); err != nil {
		return err
	}
	if err := s.Store.markShowsMissing(ctx, libraryID); err != nil {
		return err
	}
	s.Log.Info("scan done", "library", lib.Name, "items", len(all))
	return nil
}

func (s *Scanner) index(ctx context.Context, lib *Library, c candidate) error {
	existing, err := s.Store.itemByPath(ctx, lib.ID, c.path)
	if err == nil && existing.MTime == c.mtime && existing.SizeBytes == c.size && existing.Poster != "" && !existing.Missing {
		return nil // unchanged
	}
	it := &Item{LibraryID: lib.ID, Kind: c.kind, Path: c.path, SizeBytes: c.size, MTime: c.mtime}
	if existing != nil {
		it.ID = existing.ID
		it.AddedAt = existing.AddedAt
	}
	base := strings.TrimSuffix(filepath.Base(c.path), filepath.Ext(c.path))
	dir := filepath.Dir(c.path)
	if c.kind == "dvd-folder" || c.kind == "bluray-folder" {
		base = filepath.Base(c.path)
		dir = c.path
	}
	it.Title, it.Year = ParseName(base)
	// Folder name is often better than "VIDEO_TS"/"movie.mkv".
	if parentTitle, parentYear := ParseName(filepath.Base(filepath.Dir(c.path))); c.kind != "dvd-folder" && c.kind != "bluray-folder" && it.Year == nil && parentYear != nil && !strings.EqualFold(filepath.Dir(c.path), lib.Paths[0]) {
		it.Title, it.Year = parentTitle, parentYear
	}

	// Technical facts.
	switch c.kind {
	case "video":
		s.probeVideo(ctx, it)
	default:
		s.probeDisc(ctx, it)
	}

	// Sidecars.
	it.Poster, it.Backdrop = localArtwork(dir, base)
	if c.kind == "dvd-folder" || c.kind == "bluray-folder" {
		p2, b2 := localArtwork(filepath.Dir(c.path), filepath.Base(c.path))
		if it.Poster == "" {
			it.Poster = p2
		}
		if it.Backdrop == "" {
			it.Backdrop = b2
		}
	}
	for _, nfo := range []string{filepath.Join(dir, base+".nfo"), filepath.Join(dir, "movie.nfo"), filepath.Join(filepath.Dir(c.path), filepath.Base(c.path)+".nfo")} {
		if applyNFO(it, nfo) {
			break
		}
	}
	for _, sc := range []string{c.path + ".metadata.json", filepath.Join(dir, base+".metadata.json"), filepath.Join(dir, "metadata.json")} {
		if applySidecarJSON(it, sc) {
			break
		}
	}
	if it.SortTitle == "" {
		it.SortTitle = SortTitle(it.Title)
	}

	// Online metadata.
	if key := s.TMDBKey(); key != "" && lib.Type != "videos" && (it.Overview == "" || it.Poster == "") {
		t := &TMDB{Key: key}
		if err := t.Enrich(ctx, it); err != nil {
			s.Log.Info("tmdb lookup failed", "cat", "library", "title", it.Title, "err", err)
		}
	} else if key == "" && lib.Type == "movies" && it.Overview == "" && (s.WikidataOn == nil || s.WikidataOn()) {
		// No TMDB key: basic facts and the rating from Wikidata (no key needed).
		if err := (&Wikidata{}).Enrich(ctx, it); err != nil {
			s.Log.Info("wikidata lookup failed", "cat", "library", "title", it.Title, "err", err)
		}
	}
	// Generated thumbnail as the last resort.
	if it.Poster == "" {
		if p, err := s.thumbnail(ctx, it); err == nil {
			it.Poster = p
		} else {
			s.Log.Info("thumbnail failed", "cat", "library", "path", c.path, "err", err)
		}
	}
	s.saveOnScan(ctx, it)
	return s.Store.upsertItem(ctx, it)
}

func (s *Scanner) probeVideo(ctx context.Context, it *Item) {
	pctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	p, err := s.Tools.ProbeFile(pctx, it.Path)
	if err != nil {
		s.Log.Info("probe failed", "cat", "library", "path", it.Path, "err", err)
		return
	}
	it.RuntimeSec = p.Duration()
	it.Container = strings.Split(p.Format.FormatName, ",")[0]
	if v := p.Video(); v != nil {
		it.VideoCodec, it.Width, it.Height = v.CodecName, v.Width, v.Height
	}
	for _, a := range p.Audio() {
		it.AudioLangs = append(it.AudioLangs, orUnd(a.Tags["language"]))
	}
	for _, st := range p.Subtitles() {
		it.SubLangs = append(it.SubLangs, orUnd(st.Tags["language"]))
	}
	// "Skip intro": an Intro/Opening chapter marks the intro exactly.
	for _, ch := range p.Chapters {
		if introChapter(ch.Tags["title"]) {
			a, _ := strconv.ParseFloat(ch.Start, 64)
			b, _ := strconv.ParseFloat(ch.End, 64)
			if b > a && b-a < 300 {
				it.IntroStart, it.IntroEnd = &a, &b
				break
			}
		}
	}
	if t := p.Format.Tags["title"]; t != "" && it.Title == "" {
		it.Title = t
	}
}

var introRe = regexp.MustCompile(`(?i)\b(intro|opening|op|theme|title sequence|opening credits)\b`)

func introChapter(title string) bool {
	return introRe.MatchString(title) && !strings.Contains(strings.ToLower(title), "outro")
}

func orUnd(s string) string {
	if s == "" {
		return "und"
	}
	return s
}

func (s *Scanner) probeDisc(ctx context.Context, it *Item) {
	o, err := wittydvd.Open(it.Path)
	if err != nil {
		if s.probeBluray(ctx, it) {
			return
		}
		// Not a video disc: fall back to probing it as a plain file.
		if _, e2 := isofs.Open(it.Path); e2 == nil {
			it.Kind = "unsupported-disc"
		} else {
			it.Kind = "video"
			s.probeVideo(ctx, it)
		}
		return
	}
	defer o.Close()
	info := &DiscInfo{Label: o.Label, Format: o.Format}
	if o.Label != "" && it.Title == "" {
		it.Title, _ = ParseName(o.Label)
	}
	for _, lu := range o.Disc.VMG.Menus {
		if len(lu.PGCs) > 0 {
			info.HasMenus = true
		}
	}
	for _, v := range o.Disc.VTS {
		for _, lu := range v.Menus {
			if len(lu.PGCs) > 0 {
				info.HasMenus = true
			}
		}
	}
	for _, t := range o.Disc.Titles() {
		dt := DiscTitle{Number: t.Number, DurationSec: t.Duration, Chapters: t.Chapters, Angles: t.Angles, Wide: t.Video.Wide, Standard: t.Video.Standard, Audio: []string{}, Subtitles: []string{}}
		if dt.Chapters == nil {
			dt.Chapters = []float64{}
		}
		for _, a := range t.Audio {
			dt.Audio = append(dt.Audio, fmt.Sprintf("%s %s %dch", orUnd(a.Language), strings.ToUpper(a.Codec), a.Channels))
		}
		for _, sp := range t.Subp {
			dt.Subtitles = append(dt.Subtitles, orUnd(sp.Language))
		}
		info.Titles = append(info.Titles, dt)
	}
	if m := o.Disc.MainTitle(); m != nil {
		info.MainTitle = m.Number
		it.RuntimeSec = m.Duration
		it.Width, it.Height = m.Video.Size()
		it.VideoCodec = "mpeg2video"
		for _, a := range m.Audio {
			it.AudioLangs = append(it.AudioLangs, orUnd(a.Language))
		}
		for _, sp := range m.Subp {
			it.SubLangs = append(it.SubLangs, orUnd(sp.Language))
		}
		// Quick CSS check on the first sectors of the feature.
		if len(m.Cells) > 0 {
			if rd, err := o.Reader(wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: m.VTS}); err == nil {
				if buf, err := rd.ReadSectors(m.Cells[0].Cell.FirstSector, 64); err == nil {
					for off := 0; off+wittydvd.Sector <= len(buf); off += wittydvd.Sector {
						if wittydvd.PackScrambled(buf[off : off+wittydvd.Sector]) {
							info.Scrambled = true
							break
						}
					}
				}
			}
		}
	}
	it.Container = "dvd"
	it.Disc = info
}

// thumbnail grabs a frame ~20% into the feature and stores it in the cache.
func (s *Scanner) thumbnail(ctx context.Context, it *Item) (string, error) {
	h := sha1.Sum([]byte(it.Path))
	name := "thumb-" + hex.EncodeToString(h[:8]) + ".jpg"
	out := filepath.Join(s.ImagesDir, name)
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return "cache:" + name, nil
	}
	tctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var input []string
	at := it.RuntimeSec * 0.2
	switch it.Kind {
	case "video":
		input = []string{"-i", it.Path}
	case "bluray-iso", "bluray-folder":
		var err error
		if input, at, err = s.blurayThumbInput(it, h); err != nil {
			return "", err
		}
	default:
		o, err := wittydvd.Open(it.Path)
		if err != nil {
			return "", err
		}
		defer o.Close()
		m := o.Disc.MainTitle()
		if m == nil || len(m.Cells) == 0 {
			return "", fmt.Errorf("no main title")
		}
		// Pick the cell containing the 20% mark and seek within it.
		target := m.Duration * 0.2
		cell := m.Cells[0]
		for _, c := range m.Cells {
			if c.Start <= target {
				cell = c
			}
		}
		ext, err := o.RangeExtents(wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: m.VTS}, cell.Cell.FirstSector, cell.Cell.LastSector)
		if err != nil {
			return "", err
		}
		list, err := ffmpeg.WriteConcat(filepath.Join(s.CacheDir, "lists"), "thumb-"+hex.EncodeToString(h[:8]), []ffmpeg.Segment{{Extents: ext, Duration: cell.Cell.Duration}})
		if err != nil {
			return "", err
		}
		input = ffmpeg.ConcatInputArgs(list)
		at = target - cell.Start
		if at > cell.Cell.Duration-2 {
			at = cell.Cell.Duration / 2
		}
	}
	if err := s.Tools.Still(tctx, input, at, 600, out); err != nil {
		return "", err
	}
	return "cache:" + name, nil
}
