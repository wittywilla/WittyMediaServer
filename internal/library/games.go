package library

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/gamearchive"
	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
	"github.com/wittywilla/wittyplay"
)

// Games libraries: one console, or wittyplay.Auto for a folder holding many. See
// docs/GAMING.md. ROM files are only listed here, never opened beyond a look at
// which files an archive, .cue or .m3u names. What's known about them comes later,
// in the background: see games_meta.go.

// gameSkipDirs hold things that belong to an emulator rather than games.
var gameSkipDirs = map[string]bool{"bios": true, "system": true, "saves": true, "savestates": true, "states": true, "screenshots": true,
	"boxart": true, "covers": true, "cover": true, "images": true, "media": true, "manuals": true, "__macosx": true, "@eadir": true}

// discoverGames lists the games under a root. Multi-file games become one: a
// .cue hides the tracks it names and an .m3u hides the discs it names, whatever
// the case of the names in it.
func discoverGames(root, console string) ([]candidate, error) {
	ix := newArchiveIndex(nil, 0)
	defer ix.close()
	return ix.discover(root, console)
}

func (ix *archiveIndex) discover(root, console string) ([]candidate, error) {
	var found []candidate
	hidden := map[string]bool{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") || gameSkipDirs[strings.ToLower(name)]) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		c := ix.gameConsole(p, rel, console)
		if c == "" {
			return nil
		}
		st, err := d.Info()
		if err != nil {
			return nil
		}
		switch strings.ToLower(filepath.Ext(name)) {
		case ".cue", ".m3u":
			for _, f := range referencedFiles(p) {
				hidden[strings.ToLower(filepath.Clean(f))] = true
			}
		}
		found = append(found, candidate{path: p, kind: "game", size: st.Size(), mtime: st.ModTime().Unix(), console: c})
		return nil
	})
	out := found[:0]
	for _, c := range found {
		if !hidden[strings.ToLower(filepath.Clean(c.path))] {
			out = append(out, c)
		}
	}
	return out, err
}

// gameLikeExts are files that are probably games. One that gameConsole turns down
// is worth telling the owner about; anything else in the folder (a text file, a
// picture) isn't.
var gameLikeExts = map[string]bool{".7z": true, ".rar": true, ".zip": true, ".gz": true, ".tgz": true, ".tar": true, ".bz2": true, ".tbz": true, ".tbz2": true,
	".xz": true, ".txz": true, ".zst": true, ".tzst": true, ".bin": true, ".iso": true, ".chd": true, ".cue": true, ".img": true,
	".gcm": true, ".gcz": true, ".wbfs": true, ".rvz": true, ".nsp": true, ".xci": true, ".3ds": true, ".cia": true, ".xiso": true}

// skippedGameFiles counts, by extension, game-like files a library can't use. An
// archive that can't be read at all counts by why instead (see gamearchive.Error).
func skippedGameFiles(root, console string) map[string]int {
	ix := newArchiveIndex(nil, 0)
	defer ix.close()
	return ix.skipped(root, console)
}

func (ix *archiveIndex) skipped(root, console string) map[string]int {
	out := map[string]int{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") || gameSkipDirs[strings.ToLower(name)]) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		if !gameLikeExts[ext] || strings.HasPrefix(name, ".") {
			return nil
		}
		if gamearchive.IsLaterVolume(name) {
			return nil
		}
		if rel, err := filepath.Rel(root, p); err == nil && ix.gameConsole(p, rel, console) == "" {
			if problem := ix.problem(p); problem != "" {
				out[problem]++
			} else {
				out[ext]++
			}
		}
		return nil
	})
	return out
}

const skippedGamesKey = "gamesSkipped."

// noteSkippedGames remembers what the last scan of a games library left out.
func (s *Store) noteSkippedGames(ctx context.Context, lib *Library, ix *archiveIndex) {
	total := map[string]int{}
	for _, root := range lib.Paths {
		for ext, n := range ix.skipped(root, lib.Console) {
			total[ext] += n
		}
	}
	b, err := json.Marshal(total)
	if err == nil {
		_ = s.DB.SetSetting(ctx, skippedGamesKey+strconv.FormatInt(lib.ID, 10), string(b))
	}
}

// SkippedGames says what the last scan of a games library left out, by extension.
func (s *Store) SkippedGames(ctx context.Context, libraryID int64) map[string]int {
	var out map[string]int
	raw := s.DB.GetSetting(ctx, skippedGamesKey+strconv.FormatInt(libraryID, 10), "")
	if raw == "" || json.Unmarshal([]byte(raw), &out) != nil {
		return nil
	}
	return out
}

// GameCounts says how many games a library holds for each console.
func (s *Store) GameCounts(ctx context.Context, libraryID int64, access *Access) (map[string]int, error) {
	q := `SELECT IFNULL(i.console, ''), COUNT(*) FROM items i WHERE i.library_id = ? AND i.kind = 'game' AND i.missing = 0`
	args := []any{libraryID}
	if access != nil {
		c, a := access.ItemClause()
		q += " AND " + c
		args = append(args, a...)
	}
	rows, err := s.DB.QueryContext(ctx, q+` GROUP BY 1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var c string
		var n int
		if rows.Scan(&c, &n) == nil {
			out[c] = n
		}
	}
	return out, rows.Err()
}

// gameConsole says which console a file is for, or "" when it isn't a game this
// library holds.
func (ix *archiveIndex) gameConsole(path, rel, console string) string {
	name := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))
	archive := gamearchive.IsArchive(name)
	if archive && gamearchive.IsLaterVolume(name) {
		return ""
	}
	if console == wittyplay.Auto {
		if c := wittyplay.Detect(rel); c != "" {
			con := wittyplay.ByID(c)
			if (!archive && con.Accepts(ext)) || (archive && gamearchive.HoldsGame(ix.exts(path), con)) {
				return c
			}
			return "" // .001 only means an archive after .7z, for one
		}
		if archive {
			return gamearchive.ConsoleOf(ix.exts(path))
		}
		return ""
	}
	c := wittyplay.ByID(console)
	if c == nil {
		return ""
	}
	if c.Accepts(ext) || (archive && gamearchive.HoldsGame(ix.exts(path), c)) {
		return console
	}
	return ""
}

var cueFileRe = regexp.MustCompile(`(?i)^\s*FILE\s+"?(.+?)"?\s+[A-Z0-9]+\s*$`)

// referencedFiles lists the files beside a .cue or .m3u that it names, without
// following anything outside its own folder.
func referencedFiles(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	cue := strings.EqualFold(filepath.Ext(path), ".cue")
	dir := filepath.Dir(path)
	var out []string
	sc := bufio.NewScanner(f)
	for lines := 0; sc.Scan() && lines < 2000; lines++ {
		line := strings.TrimSpace(sc.Text())
		name := ""
		if cue {
			if m := cueFileRe.FindStringSubmatch(line); m != nil {
				name = m[1]
			}
		} else if line != "" && !strings.HasPrefix(line, "#") {
			name = line
		}
		if name == "" {
			continue
		}
		full := filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(name, `\`, "/")))
		if rel, err := filepath.Rel(dir, full); err == nil && !strings.HasPrefix(rel, "..") {
			out = append(out, full)
		}
	}
	return out
}

// ParseGameName splits a game's file name into its title and a short note made
// of its tags, e.g. "Mega Game, The (USA) (Rev 1)" -> "The Mega Game", "USA, Rev 1".
func ParseGameName(base string) (title, note string) { return gamemeta.CleanName(base) }

// gameBaseName is a game's file name without its extension. For an archive that's
// everything that makes it one, and the game's own extension if the name carries it
// too: "Game (USA).sfc.7z" and "Game (USA).tar.gz" are both "Game (USA)".
func gameBaseName(name string) string {
	if !gamearchive.IsArchive(name) {
		return strings.TrimSuffix(name, filepath.Ext(name))
	}
	base := gamearchive.BaseName(name)
	if x := strings.ToLower(filepath.Ext(base)); x != "" && wittyplay.KnownExtension(x) {
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
	return base
}

// gameArt finds a picture for a game: its own name beside it, or in the folders
// other software keeps box art in. A shared cover.jpg is ignored, since one
// folder holds many games.
func gameArt(dir, base string) string {
	for _, d := range []string{dir, filepath.Join(dir, "boxart"), filepath.Join(dir, "covers"), filepath.Join(dir, "images"), filepath.Join(dir, "media", "box2d")} {
		for _, n := range []string{base + "-cover", base + "-poster", base} {
			for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
				p := filepath.Join(d, n+ext)
				if st, err := os.Stat(p); err == nil && !st.IsDir() {
					return p
				}
			}
		}
	}
	return ""
}

// gameBackdrop finds a wide picture for a game beside it, under the name saving
// metadata uses.
func gameBackdrop(dir, base string) string {
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		p := filepath.Join(dir, base+"-fanart"+ext)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func (s *Scanner) indexGame(ctx context.Context, lib *Library, c candidate) error {
	existing, err := s.Store.itemByPath(ctx, lib.ID, c.path)
	if err == nil && existing.MTime == c.mtime && existing.SizeBytes == c.size && !existing.Missing && existing.Console == c.console {
		return nil // unchanged
	}
	it := &Item{LibraryID: lib.ID, Kind: "game", Path: c.path, SizeBytes: c.size, MTime: c.mtime, Console: c.console}
	if existing != nil {
		it.ID, it.AddedAt = existing.ID, existing.AddedAt
	}
	base := gameBaseName(filepath.Base(c.path))
	dir := filepath.Dir(c.path)
	it.Title, it.Tagline = ParseGameName(base)
	it.SortTitle = SortTitle(it.Title)
	it.Container = strings.TrimPrefix(strings.ToLower(filepath.Ext(c.path)), ".")
	it.Poster = gameArt(dir, base)
	it.Backdrop = gameBackdrop(dir, base)
	parsed, tags := it.Title, it.Tagline
	for _, nfo := range []string{filepath.Join(dir, base+".nfo")} {
		if applyNFO(it, nfo) {
			break
		}
	}
	// What the owner's own files say stays: sources only fill in the rest.
	local := localGameFields(it, parsed, tags)
	if existing != nil && existing.Provider != "" {
		it.Provider, it.ProviderID = existing.Provider, existing.ProviderID
	}
	if existing != nil {
		// Scans never take away what someone chose or a source already found.
		if existing.Provider != "" && !slices.Contains(local, "title") {
			it.Title, it.SortTitle = existing.Title, existing.SortTitle
		}
		if it.Poster == "" {
			it.Poster = existing.Poster
		}
		if it.Backdrop == "" {
			it.Backdrop = existing.Backdrop
		}
		if it.Overview == "" {
			it.Overview = existing.Overview
		}
		if it.Tagline == "" {
			it.Tagline = existing.Tagline
		}
		if it.Year == nil {
			it.Year = existing.Year
		}
		if len(it.Genres) == 0 {
			it.Genres = existing.Genres
		}
		if it.Studio == "" {
			it.Studio = existing.Studio
		}
		if it.Rating == nil {
			it.Rating = existing.Rating
		}
		if it.ContentRating == "" {
			it.ContentRating = existing.ContentRating
		}
	}
	if err := s.Store.upsertItem(ctx, it); err != nil {
		return err
	}
	// Finding out about it happens in the background, so a scan never waits on the internet.
	// A file that's back where it was is the same game: what's known about it still holds.
	if existing != nil && existing.SizeBytes == c.size && existing.MTime == c.mtime && existing.Console == c.console {
		return nil
	}
	return s.Store.enqueueGame(ctx, it, local)
}

// localGameFields says what came from the owner's own files beside a game, rather
// than from its file name.
func localGameFields(it *Item, parsedTitle, parsedTags string) []string {
	var out []string
	if it.Title != parsedTitle {
		out = append(out, "title")
	}
	if it.Tagline != parsedTags {
		out = append(out, "tagline")
	}
	if it.Overview != "" {
		out = append(out, "overview")
	}
	if it.Year != nil {
		out = append(out, "year")
	}
	if len(it.Genres) > 0 {
		out = append(out, "genres")
	}
	if it.Studio != "" {
		out = append(out, "studio")
	}
	if it.Rating != nil {
		out = append(out, "rating")
	}
	if it.ContentRating != "" {
		out = append(out, "contentRating")
	}
	if it.Poster != "" {
		out = append(out, "poster")
	}
	if it.Backdrop != "" {
		out = append(out, "backdrop")
	}
	return out
}

// A person's saved controller layouts, one row per kind and key.

// GameControls returns a person's saved layouts for a console.
func (s *Store) GameControls(ctx context.Context, userID int64, console string) (map[string]map[string]json.RawMessage, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT kind, key, data FROM wittyplay_controls WHERE user_id = ? AND console = ?`, userID, console)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]json.RawMessage{}
	for rows.Next() {
		var kind, key, data string
		if err := rows.Scan(&kind, &key, &data); err != nil {
			return nil, err
		}
		if out[kind] == nil {
			out[kind] = map[string]json.RawMessage{}
		}
		out[kind][key] = json.RawMessage(data)
	}
	return out, rows.Err()
}

// maxGameControls bounds what one account can store per console.
const maxGameControls = 64

// ErrTooManyControls: this account has saved as many layouts as it may.
var ErrTooManyControls = errors.New("too many saved layouts for this console; reset one first")

// SetGameControl saves a layout, or removes it when data is nil.
func (s *Store) SetGameControl(ctx context.Context, userID int64, console, kind, key string, data json.RawMessage) error {
	if data == nil {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM wittyplay_controls WHERE user_id = ? AND console = ? AND kind = ? AND key = ?`, userID, console, kind, key)
		return err
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM wittyplay_controls WHERE user_id = ? AND console = ? AND NOT (kind = ? AND key = ?)`, userID, console, kind, key).Scan(&n); err != nil {
		return err
	}
	if n >= maxGameControls {
		return ErrTooManyControls
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO wittyplay_controls(user_id, console, kind, key, data, updated_at) VALUES(?,?,?,?,?,?)
		ON CONFLICT(user_id, console, kind, key) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`,
		userID, console, kind, key, string(data), db.Now())
	return err
}
