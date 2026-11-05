package library

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittydisc/isofs"
	"github.com/wittywilla/wittymediaserver/internal/cd"
	"github.com/wittywilla/wittymediaserver/internal/dvd"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

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
	// TMDBBase replaces TMDB's address in tests.
	TMDBBase string
	// OnScanned runs after a scan of a library that holds films or shows.
	OnScanned func()
	// AfterScan is told when any library's scan has finished without trouble. It's called
	// with the scan's turn at the disk given up, and must not hold things up.
	AfterScan func(libraryID int64)
	// OnGamesScanned runs after a scan of a library of games.
	OnGamesScanned func()
	// SaveMeta reports whether to write NFO/artwork beside the media.
	SaveMeta func() bool
	Log      *slog.Logger
	// Tasks is where scans show up in the activity panel. May be nil.
	Tasks *tasks.Registry

	// slot lets one library scan at a time: a small machine has one disk and a
	// few cores to spare, and the periodic rescan starts every library at once.
	slot chan struct{}

	mu      sync.Mutex
	running map[int64]context.CancelFunc
	// Progress for the UI.
	Progress map[int64]*ScanProgress
	lastTrim time.Time // when the caches were last kept to their limits; guarded by mu
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
	sc := &Scanner{Store: store, Tools: tools, CacheDir: cacheDir, ImagesDir: filepath.Join(cacheDir, "images"), TMDBKey: tmdbKey, Log: log.With("cat", "library"), slot: make(chan struct{}, 1), running: map[int64]context.CancelFunc{}, Progress: map[int64]*ScanProgress{}}
	if tools != nil && tools.FFmpeg != "" {
		cd.ForeignAudio = sc.flacOfSheetAudio
	}
	return sc
}

// Start scans a library in the background (no-op if already scanning).
func (s *Scanner) Start(libraryID int64) bool { return s.StartBy(libraryID, tasks.By{}) }

// StartBy is Start, saying who asked. A scan of a library whose scan was stopped picks
// up where it left off: files it has already seen, unchanged, take almost no time.
func (s *Scanner) StartBy(libraryID int64, by tasks.By) bool {
	s.mu.Lock()
	if _, ok := s.running[libraryID]; ok {
		s.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.running[libraryID] = cancel
	s.Progress[libraryID] = &ScanProgress{LibraryID: libraryID, Phase: "starting", StartedAt: time.Now().Unix()}
	s.mu.Unlock()
	name, rig := "", false
	if lib, err := s.Store.Library(ctx, libraryID); err == nil {
		name, rig = lib.Name, lib.IsRig()
	}
	t := s.Tasks.Queue(tasks.Spec{Type: tasks.TypeScan, Title: "Scanning " + name, Phase: "Waiting for its turn", LibraryID: libraryID, LibraryName: name, Unit: "files",
		By: by, Cancel: cancel, Pausable: true, RetryKey: strconv.FormatInt(libraryID, 10), Quiet: rig})
	id := logging.NewID("scan")
	ctx = logging.WithSession(ctx, id) // every record of this scan carries it
	logging.Go("library scan", func() {
		defer func() {
			s.mu.Lock()
			delete(s.running, libraryID)
			delete(s.Progress, libraryID)
			s.mu.Unlock()
		}()
		err := scanGuarded(func() error { return s.scan(context.WithValue(ctx, taskKey{}, t), libraryID) }, "cat", "library", "session", id)
		t.Finish(err)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				s.Log.InfoContext(ctx, "scan cancelled", "library", libraryID)
				return
			}
			s.Log.ErrorContext(ctx, "scan failed", "library", libraryID, "err", err)
			return
		}
		if s.AfterScan != nil {
			s.AfterScan(libraryID)
		}
	}, "cat", "library", "session", id)
	return true
}

// HoldScans keeps any library from being scanned until release is called, once it has
// waited for one that's going to reach a stopping point. For work that needs the files
// and the database to hold still, such as a backup. Scans asked for meanwhile wait their
// turn, shown as queued.
func (s *Scanner) HoldScans(ctx context.Context) (release func(), err error) {
	select {
	case s.slot <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-s.slot }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// findUnlessCancelled walks a folder, and doesn't keep whoever cancelled waiting. A slow
// network share can take minutes to walk. The walk only reads, and goes on by itself until it
// has finished, which costs nothing but its own time.
func findUnlessCancelled(ctx context.Context, find func(string) ([]candidate, error), root string) ([]candidate, error) {
	type found struct {
		list []candidate
		err  error
	}
	ch := make(chan found, 1)
	logging.Go("library walk", func() {
		list, err := walkGuarded(find, root)
		ch <- found{list, err}
	}, "cat", "library")
	select {
	case f := <-ch:
		return f.list, f.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// errWalkStopped: a walk ended on a fault of the server's own, so what it found can't be
// trusted to be everything there is. The scan keeps that folder's items, as for a drive
// that's away.
var errWalkStopped = errors.New("the server hit a fault while reading this folder; it's in the log")

// walkGuarded runs a walk, turning a panic into errWalkStopped: a waiter that's never
// answered would hold the scan, and every scan and backup behind it, for ever.
func walkGuarded(find func(string) ([]candidate, error), root string) (list []candidate, err error) {
	defer func() {
		if p := recover(); p != nil {
			logging.ReportPanic("library walk", p, "cat", "library")
			list, err = nil, errWalkStopped
		}
	}()
	return find(root)
}

// scanGuarded runs a whole scan, turning a panic into an error, so its activity entry
// ends as failed instead of showing it running until the server restarts.
func scanGuarded(scan func() error, attrs ...any) (err error) {
	defer func() {
		if p := recover(); p != nil {
			logging.ReportPanic("library scan", p, attrs...)
			err = fmt.Errorf("the scan stopped on a fault in the server (%v)", p)
		}
	}()
	return scan()
}

// scanResult says what came of a scan, for the activity history.
func scanResult(files, problems int, took time.Duration) string {
	plural := func(n int, one, many string) string {
		if n == 1 {
			return "1 " + one
		}
		return strconv.Itoa(n) + " " + many
	}
	s := fmt.Sprintf("Looked at %s in %s", plural(files, "file", "files"), took.Round(time.Second))
	if problems == 0 {
		return s + ", with no problems."
	}
	return s + ", with " + plural(problems, "problem", "problems") + "."
}

// gameScanResult says what came of a scan of games: what someone comes to the activity
// panel to find out is how many there are, and whether any of them want their attention.
func (s *Store) gameScanResult(ctx context.Context, lib *Library, problems int, took time.Duration) string {
	games := 0
	if counts, err := s.GameCounts(ctx, lib.ID, nil); err == nil {
		for _, n := range counts {
			games += n
		}
	}
	plural := func(n int, one, many string) string {
		if n == 1 {
			return "1 " + one
		}
		return strconv.Itoa(n) + " " + many
	}
	out := fmt.Sprintf("Found %s in %s.", plural(games, "game", "games"), took.Round(time.Second))
	if review, err := s.GameReviewList(ctx, lib, "none", 1, 0); err == nil {
		if n := review.Counts["needs"]; n > 0 {
			out += " " + plural(n, "file needs", "files need") + " a look."
		}
		if n := review.Counts["blocked"]; n > 0 {
			out += " " + plural(n, "game", "games") + " can't play."
		}
	}
	if problems > 0 {
		out += " " + plural(problems, "problem", "problems") + " along the way."
	}
	return out
}

type taskKey struct{}

// scanTask is the activity entry of the scan this context belongs to. May be nil, and
// a nil one does nothing.
func scanTask(ctx context.Context) *tasks.Handle {
	t, _ := ctx.Value(taskKey{}).(*tasks.Handle)
	return t
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

// report leaves a warning for each kind of failure, and says how many in all. The
// activity panel gets it in a person's words.
func (t *scanTally) report(ctx context.Context, log *slog.Logger) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	total := 0
	task := scanTask(ctx)
	for msg, e := range t.what {
		total += e.n
		log.WarnContext(ctx, "scan had failures", "what", msg, "count", e.n, "err", e.err)
		words, fix := tasks.Explain(e.err)
		task.ProblemN("", strings.TrimSpace(sentenceCase(msg)+": "+words+" "+fix), e.n)
	}
	return total
}

// sentenceCase gives a log message a capital letter, for showing it to someone.
func sentenceCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
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

// Running says a scan of this library is queued or under way.
func (s *Scanner) Running(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.running[id]
	return ok
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
	// verdict is how the console was worked out, and discs the game's discs when it has
	// several. Games only.
	verdict *gameVerdict
	discs   []gameDisc
	// format is the registry's id, when the content said it and the name didn't.
	format string
	// known is what the library already says about this path, if anything.
	known *knownFile
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

// entryMTime is when an entry last changed, or 0 if it's gone since its folder was listed.
func entryMTime(d os.DirEntry) int64 {
	if st, err := d.Info(); err == nil {
		return st.ModTime().Unix()
	}
	return 0
}

func discoverVideos(root string, skipSamples bool) ([]candidate, error) {
	var out []candidate
	var bad unreadable
	var unnamed sniffBudget
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
			if plainFile(filepath.Join(p, "VIDEO_TS", "VIDEO_TS.IFO")) {
				out = append(out, candidate{path: p, kind: "dvd-folder", mtime: entryMTime(d)})
				return filepath.SkipDir
			}
			if strings.EqualFold(name, "VIDEO_TS") {
				return filepath.SkipDir
			}
			if plainFile(filepath.Join(p, "BDMV", "index.bdmv")) {
				out = append(out, candidate{path: p, kind: "bluray-folder", mtime: entryMTime(d)})
				return filepath.SkipDir
			}
			if strings.EqualFold(name, "BDMV") {
				return filepath.SkipDir
			}
			return nil
		}
		st, err := d.Info()
		if err != nil || !mediaEntry(root, p, d) || IsConvertedCopy(name) {
			return nil
		}
		switch {
		case isVideoDiscImage(p, name):
			out = append(out, candidate{path: p, kind: "dvd-iso", size: st.Size(), mtime: st.ModTime().Unix()})
		case mediatypes.IsVideo(name) && nameHolds(p, name, mediatypes.Video):
			if skipSamples && (strings.HasPrefix(strings.ToLower(name), "sample") || st.Size() < 5<<20) {
				return nil
			}
			out = append(out, candidate{path: p, kind: "video", size: st.Size(), mtime: st.ModTime().Unix()})
		default:
			if skipSamples && st.Size() < 5<<20 {
				return nil
			}
			if f := unnamed.sniff(p, name, st.Size(), mediatypes.Video); f != nil {
				out = append(out, candidate{path: p, kind: "video", size: st.Size(), mtime: st.ModTime().Unix(), format: f.ID})
			}
		}
		return nil
	})
	return stackParts(out), bad.with(err)
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
	task := scanTask(ctx)
	holding := false
	takeSlot := func() error {
		if s.slot == nil {
			return nil
		}
		s.progress(libraryID, func(p *ScanProgress) { p.Phase = "waiting" })
		select {
		case s.slot <- struct{}{}:
			holding = true
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	defer func() {
		if holding {
			<-s.slot
		}
	}()
	if err := takeSlot(); err != nil {
		return err
	}
	task.Begin()
	// A paused scan lets another library have its turn at the disk meanwhile.
	pausePoint := func() error {
		if !task.PauseWanted() {
			return ctx.Err()
		}
		if holding {
			<-s.slot
			holding = false
		}
		if err := task.Wait(ctx); err != nil {
			return err
		}
		return takeSlot()
	}
	s.Log.InfoContext(ctx, "scan started", "library", lib.Name, "type", lib.Type, "folders", len(lib.Paths))
	started := time.Now()
	tally := &scanTally{what: map[string]*tallied{}}
	ctx = context.WithValue(ctx, tallyKey{}, tally)
	failed := 0

	var all []candidate
	var offline []string
	var archives *archiveIndex
	if lib.Type == "games" && len(wittyplay.Consoles()) == 0 {
		// Without WittyPlay nothing can be told to be a game, which isn't the games being gone.
		s.Log.WarnContext(ctx, "WittyPlay isn't running; keeping the library's games as they are", "library", lib.Name)
		return nil
	}
	if lib.Type == "games" {
		archives = newArchiveIndex(s.Store, libraryID)
		archives.reading = func(path string) {
			s.progress(libraryID, func(p *ScanProgress) { p.Current = filepath.Base(path) })
			task.Item(filepath.Base(path), path)
			// Reading archives can be the longest part of a scan, so pausing can't wait for its end.
			_ = pausePoint()
		}
		archives.stopped = func() bool { return ctx.Err() != nil }
		archives.looking = func(done, total int) {
			s.progress(libraryID, func(p *ScanProgress) { p.Done, p.Found = done, total })
			task.Progress(int64(done), int64(total))
			_ = pausePoint()
		}
		defer archives.close()
	}
	if lib.Type == "games" {
		task.Phase("Working out what each file is")
	} else {
		task.Phase("Finding files")
	}
	for _, root := range lib.Paths {
		if err := pausePoint(); err != nil {
			return err
		}
		s.progress(libraryID, func(p *ScanProgress) { p.Phase = "discovering"; p.Current = root })
		task.Item(filepath.Base(root), root)
		// A drive or share that is away (an unmounted mount point reads as an
		// empty folder) is not the same as every file having been deleted.
		if entries, err := os.ReadDir(root); err != nil || len(entries) == 0 {
			s.Log.WarnContext(ctx, "library folder is empty or can't be reached; keeping its items", "library", lib.Name, "dir", root, "err", err)
			task.Problem(root, "This folder is empty or can't be reached, so what it held is being kept for now. If it's on a drive or a share, check that it's connected.")
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
		var c []candidate
		var err error
		if lib.Type == "games" {
			c, err = walkGuarded(find, root) // watches for a cancel as it goes, and shares what it finds with this scan
		} else {
			c, err = findUnlessCancelled(ctx, find, root)
		}
		if ctx.Err() != nil {
			return ctx.Err() // before anything half-found is kept
		}
		if errors.Is(err, errWalkStopped) {
			s.Log.ErrorContext(ctx, "could not read a library folder; keeping its items", "library", lib.Name, "dir", root, "err", err)
			task.Problem(root, "This folder couldn't be read because of a fault in the server, so what it held is being kept for now. The log has the details.")
			offline = append(offline, root)
			continue
		}
		if err != nil {
			s.Log.WarnContext(ctx, "could not read all of a library folder", "library", lib.Name, "dir", root, "err", err)
			words, fix := tasks.Explain(err)
			task.Problem(root, "Some of this folder couldn't be read. "+words+" "+fix)
		}
		if lib.Type == "discs" {
			images := discoverDiscCDs(root)
			c = append(dropClaimed(onlyDiscs(c), nil, images), images...)
		}
		all = append(all, c...)
	}
	if lib.Type == "games" {
		archives.save(ctx, len(offline) == 0)
		if err := s.Store.saveNotes(ctx, libraryID, archives, len(offline) > 0); err != nil {
			s.Log.WarnContext(ctx, "could not save what a games scan left out", "library", lib.Name, "err", err)
		}
	}
	// Files someone moved to another library or removed, and files moved here.
	if leave, adopt, err := s.Store.libraryNotes(ctx, libraryID); err == nil {
		if len(leave) > 0 {
			kept := all[:0]
			for _, c := range all {
				if !underAny(c.path, leave) {
					kept = append(kept, c)
				}
			}
			all = kept
		}
		for _, p := range adopt {
			c, err := s.adopted(p, lib)
			if err != nil {
				s.Log.WarnContext(ctx, "could not reach a title moved to this library; keeping it", "file", p, "err", err)
				offline = append(offline, p)
				continue
			}
			all = append(all, c...)
		}
	}
	rules := s.Store.loadRuleSet(ctx, lib)
	ctx = context.WithValue(ctx, rulesKey{}, rules)
	if lib.Type != "games" {
		if err := s.Store.loadKnown(ctx, libraryID, all, !IsAudioLibrary(lib.Type) && lib.Type != "photos" && lib.Type != "books"); err != nil && ctx.Err() == nil {
			s.Log.WarnContext(ctx, "could not read what the library holds in one go; looking file by file", "library", lib.Name, "err", err)
		}
	}
	s.progress(libraryID, func(p *ScanProgress) { p.Phase = "indexing"; p.Found = len(all) })
	if lib.Type == "games" {
		task.Phase("Listing the games")
		task.Unit("games")
	} else {
		task.Phase("Looking at each file")
	}
	task.Progress(0, int64(len(all)))
	keep := map[string]bool{}
	tv := newTVScan()
	covers := map[string]string{} // audio: one cover per folder+album
	creators := newCreatorScan()
	var playlists []candidate
	for i, c := range all {
		if err := pausePoint(); err != nil {
			return err
		}
		s.progress(libraryID, func(p *ScanProgress) { p.Current = filepath.Base(c.path); p.Done = i })
		task.Item(filepath.Base(c.path), c.path)
		task.Progress(int64(i), int64(len(all)))
		if c.kind == playlistKind {
			playlists = append(playlists, c)
			continue // read once the songs it names are in
		}
		if s.leftOut(ctx, lib, c) {
			continue // its name says media, its content says otherwise
		}
		keep[c.path] = true
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
				words, fix := tasks.Explain(err)
				task.Problem(c.path, "This file couldn't be added to the library. "+words+" "+fix)
			}
		} else {
			s.Log.DebugContext(ctx, "indexed", "file", c.path)
		}
	}
	task.Progress(int64(len(all)), int64(len(all)))
	task.Item("", "")
	task.Phase("Tidying up")
	if err := s.Store.markMissing(ctx, libraryID, keep, offline); err != nil {
		return err
	}
	s.importPlaylists(ctx, lib, playlists)
	if err := s.Store.markShowsMissing(ctx, libraryID); err != nil {
		return err
	}
	if matchableLibrary(lib.Type) || lib.Type == "videos" {
		if err := s.Store.SyncBrands(ctx); err != nil {
			s.Log.WarnContext(ctx, "could not update studios and networks", "library", lib.Name, "err", err)
		}
		s.fillAudioFormats(ctx, libraryID)
		if s.OnScanned != nil {
			s.OnScanned()
		}
	}
	s.Store.DB.Optimize(ctx)
	s.trimCaches()
	problems := tally.report(ctx, s.Log)
	s.Log.InfoContext(ctx, "scan finished", "library", lib.Name, "items", len(all), "errors", failed+problems,
		"offline", len(offline), "took", time.Since(started).Round(time.Second).String())
	task.Result(scanResult(len(all), failed+problems, time.Since(started)))
	if lib.Type == "games" {
		task.Phase("Putting versions of a game together")
		if err := s.Store.linkGameVersions(ctx, lib); err != nil {
			s.Log.WarnContext(ctx, "could not put versions of a game together", "library", lib.Name, "err", err)
		}
		task.Result(s.Store.gameScanResult(ctx, lib, failed+problems, time.Since(started)))
		s.Store.ensureGameLogos(ctx, lib)
		if s.OnGamesScanned != nil {
			s.OnGamesScanned()
		}
		s.GameMeta.Kick()
	}
	return nil
}

func (s *Scanner) index(ctx context.Context, lib *Library, c candidate) error {
	// An .iso or .img that holds only audio is a CD, however it is named.
	if c.kind == "dvd-iso" && c.size%cd.SectorBytes == 0 {
		if d, err := cd.Open(c.path); err == nil {
			d.Close()
			c.kind = CDKind
			return s.indexCD(ctx, lib, c)
		}
	}
	if k := c.unchanged(); k != nil && k.hasPoster && !k.hasDisc && !k.missing {
		s.fixParsedTitle(ctx, &Item{ID: k.id, Path: c.path, Title: k.title, Provider: k.provider}, c.kind, lib.Type)
		return nil
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
		s.fixParsedTitle(ctx, existing, c.kind, lib.Type)
		s.fillDiscContent(ctx, existing, lib.Type)
		return nil
	}
	it := &Item{LibraryID: lib.ID, Kind: c.kind, Path: c.path, SizeBytes: c.size, MTime: c.mtime}
	meta := newTitleMeta()
	if existing != nil {
		it.ID = existing.ID
		it.AddedAt = existing.AddedAt
		meta = s.Store.TitleMeta(ctx, TargetItem, existing.ID)
	}
	prov := newProvenance()
	base := strings.TrimSuffix(filepath.Base(c.path), filepath.Ext(c.path))
	dir := filepath.Dir(c.path)
	if c.kind == "dvd-folder" || c.kind == "bluray-folder" {
		base = filepath.Base(c.path)
		dir = c.path
	}
	// An id in the name says exactly what this is, and rules say how to read a folder's naming.
	hint, hasHint := IDTag(c.path)
	it.Title, it.Year = itemName(lib.Type, stripIDTags(base))
	// Folder name is often better than "VIDEO_TS"/"movie.mkv".
	if parentTitle, parentYear := ParseName(stripIDTags(filepath.Base(filepath.Dir(c.path)))); lib.Type != CommercialsType && c.kind != "dvd-folder" && c.kind != "bluray-folder" && it.Year == nil && parentYear != nil && !strings.EqualFold(filepath.Dir(c.path), lib.Paths[0]) {
		it.Title, it.Year = parentTitle, parentYear
	}
	rules, _ := ctx.Value(rulesKey{}).(*ruleSet)
	rule, hasRule := rules.test(c.path)
	if hasRule {
		if rule.Title != "" {
			it.Title = rule.Title
		}
		if rule.Year != nil {
			it.Year = rule.Year
		}
		if rule.ID != "" && !hasHint {
			hint, hasHint = MatchRef{rule.Source, rule.ID}, true
		}
	}
	prov.note(it, SourceFile)

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
	nfoSource := SourceNFO
	for _, nfo := range []string{filepath.Join(dir, base+".nfo"), filepath.Join(dir, "movie.nfo"), filepath.Join(filepath.Dir(c.path), filepath.Base(c.path)+".nfo")} {
		if applyNFO(it, nfo) {
			if nfoIsOurs(nfo) {
				nfoSource = SourceSaved
			}
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
	prov.note(it, nfoSource)

	// What a disc holds: kept when someone chose it, else worked out.
	if it.Disc != nil {
		if existing != nil && existing.Disc != nil && existing.Disc.ContentManual {
			it.Disc.Content, it.Disc.ContentManual = existing.Disc.Content, true
		} else {
			it.Disc.Content = GuessDiscContent(lib.Type, it.Title, it.Disc.Label)
		}
	}

	// A person's decision about the match stands: nothing is looked up against it.
	lookup := meta.MatchState == MatchAuto && !(hasRule && rule.Lookup == "none")
	if lookup && hasHint {
		if s.applyHint(ctx, it, hint, meta) {
			lookup = false
		}
	}
	if lookup {
		s.lookUpItem(ctx, it, lib, rule.Lookup, prov)
	}
	if existing != nil {
		s.Store.keepLockedItem(ctx, it, existing, meta)
	}
	prov.note(it, it.Provider)
	// Generated thumbnail as the last resort.
	if it.Poster == "" && (it.Disc == nil || !it.Disc.Scrambled) {
		if p, err := s.thumbnail(ctx, it); err == nil {
			it.Poster = p
			prov.note(it, SourceFrame)
		} else {
			s.minorFailure(ctx, "could not make a thumbnail", err, "file", c.path)
		}
	}
	if !meta.Locks["director"] && it.director != "" {
		meta.Director = it.director
		prov.sources["director"] = it.Provider
		meta.Sources["director"] = it.Provider
	}
	meta.AudioFormat = audioFormatOf(it)
	if hasRule {
		meta.Tags = cleanList(append(meta.Tags, rule.Tags...), 50, 60)
		if len(rule.Edition) > 0 && !meta.Locks["edition"] {
			meta.Edition = rule.Edition
		}
	}
	s.saveOnScan(ctx, it)
	if err := s.Store.upsertItem(ctx, it); err != nil {
		return err
	}
	s.Store.recordScan(ctx, it.ID, meta, prov)
	if hasRule && rule.Extra != "" {
		s.Store.markRuleExtra(ctx, it, rule.Extra)
	}
	return nil
}

// lookUpItem asks the online sources about a new or changed file: the owner's own agents
// first, then the built-in ones. Discs of TV series are not looked up as films (they
// keep their name, disc label and agents' answers). only limits it to one source, for a
// rule that says where to look.
func (s *Scanner) lookUpItem(ctx context.Context, it *Item, lib *Library, only string, prov *provenance) {
	s.applyAgents(ctx, it, lib.Type)
	if it.Provider != "" {
		prov.note(it, it.Provider)
	}
	movieLike := lib.Type != "videos" && lib.Type != CommercialsType && (it.Disc == nil || it.Disc.Content != "tv")
	if key := s.TMDBKey(); key != "" && movieLike && (it.Overview == "" || it.Poster == "") && (only == "" || only == "auto" || only == "tmdb") {
		t := &TMDB{Key: key, Base: s.TMDBBase}
		if err := t.Enrich(ctx, it); err != nil {
			s.minorFailure(ctx, "metadata lookup failed", err, "source", "tmdb", "title", it.Title)
		}
	} else if key == "" && movieLike && (lib.Type == "movies" || lib.Type == "discs") && it.Overview == "" && (s.WikidataOn == nil || s.WikidataOn()) && (only == "" || only == "auto" || only == "wikidata") {
		// No TMDB key: basic facts and the rating from Wikidata (no key needed).
		if err := (&Wikidata{}).Enrich(ctx, it); err != nil {
			s.minorFailure(ctx, "metadata lookup failed", err, "source", "wikidata", "title", it.Title)
		}
	}
}

// applyHint fills an item from an id its name gave. It's a person's choice as much as
// picking from a list, so it counts as one.
func (s *Scanner) applyHint(ctx context.Context, it *Item, hint MatchRef, meta *TitleMeta) bool {
	t := &Title{Target: TargetItem, ID: it.ID, Item: it, Meta: meta}
	source := hint.Source
	if source == "imdb" {
		c, err := s.findIMDb(ctx, hint.ID, t)
		if err != nil || len(c) == 0 {
			s.minorFailure(ctx, "metadata lookup failed", errors.New("no title has that IMDb id"), "id", hint.ID)
			return false
		}
		source, hint.ID = c[0].Source, c[0].ID
	}
	d, err := s.detailsOf(ctx, source, hint.ID)
	if err != nil {
		s.minorFailure(ctx, "metadata lookup failed", err, "source", source, "id", hint.ID)
		return false
	}
	if err := applyDetails(t, d, nil); err != nil {
		return false
	}
	meta.MatchState = MatchManual
	return true
}

// adopted lists what a scan looks after for a path moved to this library. A show's
// folder is under no folder of the library, so its parent counts as one for this scan.
func (s *Scanner) adopted(path string, lib *Library) ([]candidate, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		if _, err := os.Stat(filepath.Join(path, "VIDEO_TS", "VIDEO_TS.IFO")); err == nil {
			return []candidate{{path: path, kind: "dvd-folder", mtime: st.ModTime().Unix()}}, nil
		}
		if _, err := os.Stat(filepath.Join(path, "BDMV", "index.bdmv")); err == nil {
			return []candidate{{path: path, kind: "bluray-folder", mtime: st.ModTime().Unix()}}, nil
		}
		lib.Paths = append(lib.Paths, filepath.Dir(path))
		return discover(path)
	}
	kind := "video"
	if isVideoDiscImage(path, filepath.Base(path)) {
		kind = "dvd-iso"
	}
	return []candidate{{path: path, kind: kind, size: st.Size(), mtime: st.ModTime().Unix()}}, nil
}

type rulesKey struct{}

// audioFormatOf says what an item's audio is, for filtering: "AC3 5.1".
func audioFormatOf(it *Item) string {
	if it.audioFormat != "" {
		return it.audioFormat
	}
	if it.Disc != nil {
		for _, t := range it.Disc.Titles {
			if t.Number == it.Disc.MainTitle && len(t.Audio) > 0 {
				// "eng AC3 6ch"
				if f := strings.Fields(t.Audio[0]); len(f) == 3 {
					n, _ := strconv.Atoi(strings.TrimSuffix(f[2], "ch"))
					return audioFormatName(f[1], n)
				}
			}
		}
	}
	return ""
}

func audioFormatName(codec string, channels int) string {
	codec = strings.ToUpper(codec)
	switch codec {
	case "EAC3":
		codec = "E-AC3"
	case "TRUEHD":
		codec = "TrueHD"
	case "PCM_S16LE", "PCM_S24LE", "PCM_BLURAY", "PCM_DVD":
		codec = "PCM"
	case "":
		return ""
	}
	switch channels {
	case 0:
		return codec
	case 1:
		return codec + " mono"
	case 2:
		return codec + " stereo"
	case 6:
		return codec + " 5.1"
	case 8:
		return codec + " 7.1"
	}
	return codec + " " + strconv.Itoa(channels) + "ch"
}

// maxAudioFills bounds the look at files scanned before audio formats were kept, per scan.
const maxAudioFills = 200

// fillAudioFormats reads the audio format of titles scanned before it was kept.
func (s *Scanner) fillAudioFormats(ctx context.Context, libraryID int64) {
	// A disc says what its sound is in what's stored already.
	if discs, _, err := s.Store.Items(ctx, ItemQuery{LibraryID: libraryID, Kind: "disc", NoTotal: true}); err == nil {
		for _, it := range discs {
			m := s.Store.TitleMeta(ctx, TargetItem, it.ID)
			if f := audioFormatOf(it); f != "" && m.AudioFormat == "" {
				m.AudioFormat = f
				_ = s.Store.saveTitleMeta(ctx, TargetItem, it.ID, m)
			}
		}
	}
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT i.id, i.path FROM items i LEFT JOIN title_meta m ON m.target = 'item' AND m.target_id = i.id
		WHERE i.library_id = ? AND i.missing = 0 AND i.show_id IS NULL AND i.kind = 'video' AND IFNULL(m.audio_format, '') = '' LIMIT ?`, libraryID, maxAudioFills)
	if err != nil {
		return
	}
	type row struct {
		id   int64
		path string
	}
	var todo []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.path) == nil {
			todo = append(todo, r)
		}
	}
	rows.Close()
	task := scanTask(ctx)
	if len(todo) > 0 {
		task.Phase("Reading sound formats")
	}
	for _, r := range todo {
		if task.Wait(ctx) != nil {
			return
		}
		task.Item(filepath.Base(r.path), r.path)
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		p, err := s.Tools.ProbeFile(pctx, r.path)
		cancel()
		if err != nil {
			continue
		}
		format := "none"
		if a := p.Audio(); len(a) > 0 {
			format = audioFormatName(a[0].CodecName, a[0].Channels)
		}
		m := s.Store.TitleMeta(ctx, TargetItem, r.id)
		m.AudioFormat = format
		_ = s.Store.saveTitleMeta(ctx, TargetItem, r.id, m)
	}
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
	if it.RuntimeSec <= 0 {
		// A bare stream has no length written anywhere: read through it to find out.
		mctx, done := context.WithTimeout(ctx, 2*time.Minute)
		it.RuntimeSec = s.Tools.MeasureDuration(mctx, it.Path)
		done()
	}
	it.Container = strings.Split(p.Format.FormatName, ",")[0]
	if parts := StackedParts(it.Path); len(parts) > 1 {
		// A film split across files: its length is all of them.
		pctx, done := context.WithTimeout(ctx, 60*time.Second)
		for _, part := range parts[1:] {
			if q, err := s.Tools.ProbeFile(pctx, part); err == nil {
				it.RuntimeSec += q.Duration()
			}
		}
		done()
	}
	if v := p.Video(); v != nil {
		it.VideoCodec = v.CodecName
		it.Width, it.Height = v.DisplaySize()
	}
	for i, a := range p.Audio() {
		it.AudioLangs = append(it.AudioLangs, orUnd(a.Tags["language"]))
		if i == 0 {
			it.audioFormat = audioFormatName(a.CodecName, a.Channels)
		}
	}
	for _, st := range p.Subtitles() {
		it.SubLangs = append(it.SubLangs, orUnd(st.Tags["language"]))
	}
	for _, st := range FindSubtitles(it.Path) {
		it.SubLangs = append(it.SubLangs, orUnd(st.Language))
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
	if kind := discKindOf(it.Path); kind == "dvd" && !dvdEngineThere() {
		// What's on the disc can't be read without WittyDVD, and the file itself is still
		// here. It stays a disc, and the next scan after WittyDVD arrives fills it in.
		s.Log.InfoContext(ctx, "a DVD waits for WittyDVD", "file", it.Path)
		it.Container = "dvd"
		return
	} else if kind == "bluray" && !blurayEngineThere() {
		s.Log.InfoContext(ctx, "a Blu-ray waits for WittyBluray", "file", it.Path)
		if strings.HasSuffix(it.Kind, "folder") {
			it.Kind = "bluray-folder"
		} else {
			it.Kind = "bluray-iso"
		}
		it.Container = "bluray"
		return
	}
	o, err := dvdsession.Open(it.Path)
	if errors.Is(err, dvdsession.ErrScrambled) {
		s.Log.InfoContext(ctx, "this DVD is encrypted, so it won't play", "file", it.Path)
		it.Container = "dvd"
		it.Disc = &DiscInfo{Format: "dvd", Scrambled: true}
		return
	}
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
		// Quick CSS check on the first sectors of the feature, where the engine says they are.
		if len(m.Cells) > 0 {
			c := m.Cells[0].Cell
			if ext, err := o.RangeExtents(dvd.Space{Domain: dvd.DomainVTS, VTS: m.VTS}, c.FirstSector, min(c.LastSector, c.FirstSector+63)); err != nil {
				info.Scrambled = errors.Is(err, dvdsession.ErrScrambled)
			} else if len(ext) > 0 {
				info.Scrambled = extentScrambled(ext[0])
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
		o, err := dvdsession.Open(it.Path)
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
		ext, err := o.RangeExtents(dvd.Space{Domain: dvd.DomainVTS, VTS: m.VTS}, cell.Cell.FirstSector, cell.Cell.LastSector)
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
	if err := s.Tools.Poster(tctx, input, at, 600, out); err != nil {
		return "", err
	}
	return "cache:" + name, nil
}

// fixParsedTitle corrects the title of an unchanged item that was named from
// its file by an earlier, faulty parse ("Dr.Story.The.Treeline" became
// "Dr Story The", and a commercial lost all its name after a second year).
// Items matched online or retitled by hand keep theirs.
func (s *Scanner) fixParsedTitle(ctx context.Context, it *Item, kind, libType string) {
	if it.Provider != "" {
		return
	}
	base := strings.TrimSuffix(filepath.Base(it.Path), filepath.Ext(it.Path))
	if kind == "dvd-folder" || kind == "bluray-folder" {
		base = filepath.Base(it.Path)
	}
	title, _ := ParseName(base)
	old := legacyTitle(base)
	if libType == CommercialsType {
		title, _ = ClipName(stripIDTags(base))
		old, _ = ParseName(stripIDTags(base))
	}
	if title == it.Title || it.Title != old || s.Store.TitleMeta(ctx, TargetItem, it.ID).Locks["title"] {
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

// discKindOf says what kind of disc a folder or image looks like from its names alone: "dvd",
// "bluray" or "".
func discKindOf(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return ""
	}
	has := func(names map[string]bool) string {
		for n := range names {
			switch strings.ToUpper(n) {
			case "VIDEO_TS":
				return "dvd"
			case "BDMV", "BDAV":
				return "bluray"
			}
		}
		return ""
	}
	names := map[string]bool{}
	if st.IsDir() {
		names[filepath.Base(path)] = true
		if entries, err := os.ReadDir(path); err == nil {
			for _, e := range entries {
				names[e.Name()] = true
			}
		}
		return has(names)
	}
	d, err := isofs.Open(path)
	if err != nil {
		return ""
	}
	defer d.Close()
	for _, f := range d.Files() {
		top, _, _ := strings.Cut(strings.TrimPrefix(f.Path, "/"), "/")
		names[top] = true
	}
	return has(names)
}

// extentScrambled looks at the start of an extent for encrypted packs.
func extentScrambled(e dvd.FileExtent) bool {
	f, err := os.Open(e.File)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, min(e.Length, 64*dvd.Sector))
	n, err := f.ReadAt(buf, e.Offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	for off := 0; off+dvd.Sector <= n; off += dvd.Sector {
		if dvd.PackScrambled(buf[off : off+dvd.Sector]) {
			return true
		}
	}
	return false
}

// EngineThere says whether a disc engine is on this server, so a scan can tell a disc it can't
// read yet from one that can't be read. The server sets it as it starts.
var EngineThere = func(string) bool { return false }

func dvdEngineThere() bool { return EngineThere("wittydvd") }

func blurayEngineThere() bool { return EngineThere("wittybluray") }
