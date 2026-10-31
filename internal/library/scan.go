package library

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittycd"
	"github.com/wittywilla/wittydisc/isofs"
	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/logging"
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
	// MusicBrainzOn: audio CDs may be looked up by their table of contents.
	MusicBrainzOn func() bool
	// MusicBrainzBase and CoverArtBase replace the services' addresses in tests.
	MusicBrainzBase, CoverArtBase string
	// AnimeTitles: english | romaji | native titles for anime libraries.
	AnimeTitles func() string
	// Agents returns the custom metadata agents, in the order they run.
	Agents func() []Agent
	// GameMeta finds out about games after a scan has listed them.
	GameMeta *GameMeta
	// SaveMeta reports whether to write NFO/artwork beside the media.
	SaveMeta func() bool
	Log      *slog.Logger

	// slot lets one library scan at a time: a small machine has one disk and a
	// few cores to spare, and the periodic rescan starts every library at once.
	slot chan struct{}

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
	return &Scanner{Store: store, Tools: tools, CacheDir: cacheDir, ImagesDir: filepath.Join(cacheDir, "images"), TMDBKey: tmdbKey, Log: log.With("cat", "library"), slot: make(chan struct{}, 1), running: map[int64]context.CancelFunc{}, Progress: map[int64]*ScanProgress{}}
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
	id := logging.NewID("scan")
	ctx = logging.WithSession(ctx, id) // every record of this scan carries it
	logging.Go("library scan", func() {
		defer func() {
			s.mu.Lock()
			delete(s.running, libraryID)
			delete(s.Progress, libraryID)
			s.mu.Unlock()
		}()
		if err := s.scan(ctx, libraryID); err != nil {
			if errors.Is(err, context.Canceled) {
				s.Log.InfoContext(ctx, "scan cancelled", "library", libraryID)
				return
			}
			s.Log.ErrorContext(ctx, "scan failed", "library", libraryID, "err", err)
		}
	}, "cat", "library", "session", id)
	return true
}

// scanTally counts what a file or a lookup failed at without stopping the scan,
// for the summary at its end. With the network down, a scan of thousands would
// otherwise say the same thing thousands of times.
type scanTally struct {
	mu   sync.Mutex
	what map[string]*tallied
}

type tallied struct {
	n   int
	err error // the last one
}

type tallyKey struct{}

// minorFailure records one such failure: at Debug during a scan, where the
// summary counts it, and as a warning when it happens outside one.
func (s *Scanner) minorFailure(ctx context.Context, msg string, err error, args ...any) {
	args = append(args, "err", err)
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		s.Log.DebugContext(ctx, msg, args...)
		return
	}
	t, _ := ctx.Value(tallyKey{}).(*scanTally)
	if t == nil {
		s.Log.WarnContext(ctx, msg, args...)
		return
	}
	s.Log.DebugContext(ctx, msg, args...)
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.what[msg]
	if e == nil {
		e = &tallied{}
		t.what[msg] = e
	}
	e.n++
	e.err = err
}

// report leaves a warning for each kind of failure, and says how many in all.
func (t *scanTally) report(ctx context.Context, log *slog.Logger) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	total := 0
	for msg, e := range t.what {
		total += e.n
		log.WarnContext(ctx, "scan had failures", "what", msg, "count", e.n, "err", e.err)
	}
	return total
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
	// motion: the clip of a live photo (photos libraries).
	motion string
	// console: games, the console this file was sorted into.
	console string
}

// discover lists playable things under a root: video files, disc images and
// VIDEO_TS folders (a folder containing VIDEO_TS becomes one item).
func discover(root string) ([]candidate, error) { return discoverVideos(root, true) }

// discoverClips is for commercials: short, small files are the whole point.
func discoverClips(root string) ([]candidate, error) { return discoverVideos(root, false) }

// unreadable remembers what a walk couldn't open. The walk carries on past it,
// and the scan, which is handed it as an error along with what was found, logs it.
type unreadable struct {
	n     int
	first error
}

func (u *unreadable) note(err error) {
	if u.n++; u.first == nil {
		u.first = err
	}
}

func (u *unreadable) with(err error) error {
	if err != nil || u.n == 0 {
		return err
	}
	return fmt.Errorf("%d folders or files could not be read, the first: %w", u.n, u.first)
}

func discoverVideos(root string, skipSamples bool) ([]candidate, error) {
	var out []candidate
	var bad unreadable
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			bad.note(err)
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
		if err != nil || !mediaEntry(root, p, d) {
			return nil
		}
		switch {
		case ext == ".iso" || ext == ".img":
			out = append(out, candidate{path: p, kind: "dvd-iso", size: st.Size(), mtime: st.ModTime().Unix()})
		case videoExts[ext]:
			if skipSamples && (strings.HasPrefix(strings.ToLower(name), "sample") || st.Size() < 5<<20) {
				return nil
			}
			out = append(out, candidate{path: p, kind: "video", size: st.Size(), mtime: st.ModTime().Unix()})
		}
		return nil
	})
	return out, bad.with(err)
}

func (s *Scanner) scan(ctx context.Context, libraryID int64) error {
	lib, err := s.Store.Library(ctx, libraryID)
	if err != nil {
		return err
	}
	if lib.IsRig() {
		return nil // its games come from the PC, not from folders here
	}
	s.Store.setScanning(ctx, libraryID, true)
	defer s.Store.setScanning(context.Background(), libraryID, false)
	if s.slot != nil {
		s.progress(libraryID, func(p *ScanProgress) { p.Phase = "waiting" })
		select {
		case s.slot <- struct{}{}:
			defer func() { <-s.slot }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.Log.InfoContext(ctx, "scan started", "library", lib.Name, "type", lib.Type, "folders", len(lib.Paths))
	started := time.Now()
	tally := &scanTally{what: map[string]*tallied{}}
	ctx = context.WithValue(ctx, tallyKey{}, tally)
	failed := 0

	var all []candidate
	var offline []string
	var archives *archiveIndex
	if lib.Type == "games" {
		archives = newArchiveIndex(s.Store, libraryID)
		archives.reading = func(path string) {
			s.progress(libraryID, func(p *ScanProgress) { p.Current = filepath.Base(path) })
		}
		defer archives.close()
	}
	for _, root := range lib.Paths {
		s.progress(libraryID, func(p *ScanProgress) { p.Phase = "discovering"; p.Current = root })
		// A drive or share that is away (an unmounted mount point reads as an
		// empty folder) is not the same as every file having been deleted.
		if entries, err := os.ReadDir(root); err != nil || len(entries) == 0 {
			s.Log.WarnContext(ctx, "library folder is empty or can't be reached; keeping its items", "library", lib.Name, "dir", root, "err", err)
			offline = append(offline, root)
			continue
		}
		find := discover
		switch {
		case IsAudioLibrary(lib.Type):
			find = discoverAudio
		case lib.Type == "books":
			find = discoverBooks
		case lib.Type == "photos":
			find = discoverPhotos
		case IsCreatorLibrary(lib.Type):
			find = discoverCreatorVideos
		case lib.Type == CommercialsType, lib.Type == "videos":
			find = discoverClips
		case lib.Type == "games":
			find = func(root string) ([]candidate, error) { return archives.discover(root, lib.Console) }
		}
		c, err := find(root)
		if err != nil {
			s.Log.WarnContext(ctx, "could not read all of a library folder", "library", lib.Name, "dir", root, "err", err)
		}
		if lib.Type == "discs" {
			images := discoverDiscCDs(root)
			c = append(dropClaimed(onlyDiscs(c), nil, images), images...)
		}
		all = append(all, c...)
	}
	if lib.Type == "games" {
		s.Store.noteSkippedGames(ctx, lib, archives)
		archives.save(ctx, len(offline) == 0)
	}
	s.progress(libraryID, func(p *ScanProgress) { p.Phase = "indexing"; p.Found = len(all) })
	keep := map[string]bool{}
	tv := newTVScan()
	covers := map[string]string{} // audio: one cover per folder+album
	creators := newCreatorScan()
	for i, c := range all {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		keep[c.path] = true
		s.progress(libraryID, func(p *ScanProgress) { p.Current = filepath.Base(c.path); p.Done = i })
		err := guard(func() error {
			switch {
			case (lib.Type == "shows" || lib.Type == "anime") && IsDiscKind(c.kind):
				return s.index(ctx, lib, c) // a series on DVD or Blu-ray: a disc, with its menus
			case lib.Type == "shows" || lib.Type == "anime":
				return s.indexEpisode(ctx, lib, c, tv)
			case c.kind == CDKind:
				return s.indexCD(ctx, lib, c)
			case IsAudioLibrary(lib.Type):
				return s.indexAudio(ctx, lib, c, covers)
			case lib.Type == "books":
				return s.indexBook(ctx, lib, c)
			case lib.Type == "photos":
				return s.indexPhoto(ctx, lib, c)
			case IsCreatorLibrary(lib.Type):
				return s.indexCreatorVideo(ctx, lib, c, creators)
			case lib.Type == "games":
				return s.indexGame(ctx, lib, c)
			}
			return s.index(ctx, lib, c)
		})
		if err != nil {
			if ctx.Err() == nil {
				failed++
				s.Log.WarnContext(ctx, "could not add a file to the library", "file", c.path, "err", err)
			}
		} else {
			s.Log.DebugContext(ctx, "indexed", "file", c.path)
		}
	}
	if err := s.Store.markMissing(ctx, libraryID, keep, offline); err != nil {
		return err
	}
	if err := s.Store.markShowsMissing(ctx, libraryID); err != nil {
		return err
	}
	s.Store.DB.Optimize(ctx)
	problems := tally.report(ctx, s.Log)
	s.Log.InfoContext(ctx, "scan finished", "library", lib.Name, "items", len(all), "errors", failed+problems,
		"offline", len(offline), "took", time.Since(started).Round(time.Second).String())
	if lib.Type == "games" {
		s.GameMeta.Kick()
	}
	return nil
}

func (s *Scanner) index(ctx context.Context, lib *Library, c candidate) error {
	// An .iso or .img that holds only audio is a CD, however it is named.
	if c.kind == "dvd-iso" && c.size%wittycd.SectorBytes == 0 {
		if d, err := wittycd.Open(c.path); err == nil {
			d.Close()
			c.kind = CDKind
			return s.indexCD(ctx, lib, c)
		}
	}
	existing, err := s.Store.itemByPath(ctx, lib.ID, c.path)
	if err == nil && existing.MTime == c.mtime && existing.SizeBytes == c.size {
		// The file is what it was. Its details and anyone's edits stay; only a
		// picture may still be found (it needs no second look at the file or
		// the internet, and a broken file must not cost that on every scan).
		if existing.Missing {
			if err := s.Store.restore(ctx, existing.ID); err != nil {
				return err
			}
		}
		if existing.Poster == "" {
			s.findPoster(ctx, existing, c)
		}
		s.fixParsedTitle(ctx, existing, c.kind)
		s.fillDiscContent(ctx, existing, lib.Type)
		return nil
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

	// What a disc holds: kept when someone chose it, else worked out.
	if it.Disc != nil {
		if existing != nil && existing.Disc != nil && existing.Disc.ContentManual {
			it.Disc.Content, it.Disc.ContentManual = existing.Disc.Content, true
		} else {
			it.Disc.Content = GuessDiscContent(lib.Type, it.Title, it.Disc.Label)
		}
	}

	// Online metadata: the owner's own agents first, then the built-in ones.
	// Discs of TV series are not looked up as films (they keep their name,
	// disc label and agents' answers).
	s.applyAgents(ctx, it, lib.Type)
	movieLike := lib.Type != "videos" && lib.Type != CommercialsType && (it.Disc == nil || it.Disc.Content != "tv")
	if key := s.TMDBKey(); key != "" && movieLike && (it.Overview == "" || it.Poster == "") {
		t := &TMDB{Key: key}
		if err := t.Enrich(ctx, it); err != nil {
			s.minorFailure(ctx, "metadata lookup failed", err, "source", "tmdb", "title", it.Title)
		}
	} else if key == "" && movieLike && (lib.Type == "movies" || lib.Type == "discs") && it.Overview == "" && (s.WikidataOn == nil || s.WikidataOn()) {
		// No TMDB key: basic facts and the rating from Wikidata (no key needed).
		if err := (&Wikidata{}).Enrich(ctx, it); err != nil {
			s.minorFailure(ctx, "metadata lookup failed", err, "source", "wikidata", "title", it.Title)
		}
	}
	// Generated thumbnail as the last resort.
	if it.Poster == "" && (it.Disc == nil || !it.Disc.Scrambled) {
		if p, err := s.thumbnail(ctx, it); err == nil {
			it.Poster = p
		} else {
			s.minorFailure(ctx, "could not make a thumbnail", err, "file", c.path)
		}
	}
	s.saveOnScan(ctx, it)
	return s.Store.upsertItem(ctx, it)
}

// findPoster looks once more for a picture for an unchanged item without one:
// a file put beside it since, else a frame from it.
func (s *Scanner) findPoster(ctx context.Context, it *Item, c candidate) {
	base := strings.TrimSuffix(filepath.Base(c.path), filepath.Ext(c.path))
	dir := filepath.Dir(c.path)
	if c.kind == "dvd-folder" || c.kind == "bluray-folder" {
		base, dir = filepath.Base(c.path), c.path
	}
	p, _ := localArtwork(dir, base)
	if p == "" && (it.Disc == nil || !it.Disc.Scrambled) {
		var err error
		if p, err = s.thumbnail(ctx, it); err != nil {
			s.minorFailure(ctx, "could not make a thumbnail", err, "file", c.path)
		}
	}
	if p != "" {
		if err := s.Store.setPoster(ctx, it.ID, p); err != nil {
			s.Log.WarnContext(ctx, "could not save a poster", "file", it.Path, "err", err)
		}
	}
}

func (s *Scanner) probeVideo(ctx context.Context, it *Item) {
	pctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	p, err := s.Tools.ProbeFile(pctx, it.Path)
	if err != nil {
		s.minorFailure(ctx, "could not read a file's details", err, "file", it.Path)
		return
	}
	it.RuntimeSec = p.Duration()
	it.Container = strings.Split(p.Format.FormatName, ",")[0]
	if v := p.Video(); v != nil {
		it.VideoCodec = v.CodecName
		it.Width, it.Height = v.DisplaySize()
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
		if d, e2 := isofs.Open(it.Path); e2 == nil {
			d.Close()
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
		input = ffmpeg.FileInput(it.Path)
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

// fixParsedTitle corrects the title of an unchanged item that was named from
// its file by the earlier, faulty parse ("Dr.Seuss.The.Lorax" became
// "Dr Seuss The"). Items matched online or retitled by hand keep theirs.
func (s *Scanner) fixParsedTitle(ctx context.Context, it *Item, kind string) {
	if it.Provider != "" {
		return
	}
	base := strings.TrimSuffix(filepath.Base(it.Path), filepath.Ext(it.Path))
	if kind == "dvd-folder" || kind == "bluray-folder" {
		base = filepath.Base(it.Path)
	}
	title, _ := ParseName(base)
	if title == it.Title || it.Title != legacyTitle(base) {
		return
	}
	if err := s.Store.retitle(ctx, it.ID, title); err != nil {
		s.Log.WarnContext(ctx, "could not save a corrected title", "file", it.Path, "err", err)
	}
}

// guard runs f, turning a panic into an error: files are read by parsers
// that see whatever is on disk, and one malformed disc or archive must cost
// that file, not the server.
func guard(f func() error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			logging.ReportPanic("library index", p, "cat", "library") // a bug, whatever the file was
			err = fmt.Errorf("could not read this file (%v)", p)
		}
	}()
	return f()
}

// onlyDiscs keeps the DVD and Blu-ray images and folders of a Discs library.
func onlyDiscs(c []candidate) []candidate {
	out := c[:0]
	for _, x := range c {
		if strings.HasPrefix(x.kind, "dvd-") || strings.HasPrefix(x.kind, "bluray-") {
			out = append(out, x)
		}
	}
	return out
}

var tvDiscRe = regexp.MustCompile(`(?i)\b(season|series|seizoen|staffel|saison|temporada|s\d{1,2}(d\d{1,2})?|complete|box ?set|episodes?|vol(ume)?\.? ?\d+)\b`)

// LooksLikeTVDisc reports whether a disc's name reads like part of a TV
// series ("Friends Season 1 Disc 2", "THE_OFFICE_S2D1") rather than a film.
func LooksLikeTVDisc(title string) bool {
	return tvDiscRe.MatchString(strings.NewReplacer("_", " ", ".", " ").Replace(title))
}

// fillDiscContent works out what an unchanged disc holds when it was scanned
// before discs said so.
func (s *Scanner) fillDiscContent(ctx context.Context, it *Item, libType string) {
	if it.Disc == nil || it.Disc.Content != "" {
		return
	}
	it.Disc.Content = GuessDiscContent(libType, it.Title, it.Disc.Label)
	if err := s.Store.setDisc(ctx, it.ID, it.Disc); err != nil {
		s.Log.WarnContext(ctx, "could not save what a disc holds", "file", it.Path, "err", err)
	}
}
