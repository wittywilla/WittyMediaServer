package library

import (
	"archive/zip"
	"bufio"
	"context"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittyplay"
)

// Games libraries: one console, or wittyplay.Auto for a folder holding many. See
// docs/GAMING.md. ROM files are only listed here, never opened beyond a look at
// which files a .zip, .cue or .m3u names.

// gameSkipDirs hold things that belong to an emulator rather than games.
var gameSkipDirs = map[string]bool{"bios": true, "system": true, "saves": true, "savestates": true, "states": true, "screenshots": true,
	"boxart": true, "covers": true, "cover": true, "images": true, "media": true, "manuals": true, "__macosx": true, "@eadir": true}

// discoverGames lists the games under a root. Multi-file games become one: a
// .cue hides the tracks it names and an .m3u hides the discs it names.
func discoverGames(root, console string) ([]candidate, error) {
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
		c := gameConsole(p, rel, console)
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
				hidden[filepath.Clean(f)] = true
			}
		}
		found = append(found, candidate{path: p, kind: "game", size: st.Size(), mtime: st.ModTime().Unix(), console: c})
		return nil
	})
	out := found[:0]
	for _, c := range found {
		if !hidden[filepath.Clean(c.path)] {
			out = append(out, c)
		}
	}
	return out, err
}

// gameConsole says which console a file is for, or "" when it isn't a game this
// library holds.
func gameConsole(path, rel, console string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if console == wittyplay.Auto {
		if c := wittyplay.Detect(rel); c != "" {
			if ext != ".zip" || zipGame(path, wittyplay.ByID(c)) != "" {
				return c
			}
			return ""
		}
		if ext == ".zip" {
			return zipConsole(path)
		}
		return ""
	}
	c := wittyplay.ByID(console)
	if c == nil {
		return ""
	}
	if c.Accepts(ext) || (ext == ".zip" && zipGame(path, c) != "") {
		return console
	}
	return ""
}

// ZipGame names the first entry in a .zip that is a game for this console.
func ZipGame(path string, c *wittyplay.Console) string { return zipGame(path, c) }

func zipGame(path string, c *wittyplay.Console) string {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return ""
	}
	defer zr.Close()
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() && c.Accepts(strings.ToLower(filepath.Ext(f.Name))) {
			return f.Name
		}
	}
	return ""
}

// zipConsole sorts a .zip that isn't in a console's folder by what's inside.
func zipConsole(path string) string {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return ""
	}
	defer zr.Close()
	for _, f := range zr.File {
		if c := wittyplay.ByExtension(strings.ToLower(filepath.Ext(f.Name))); c != "" {
			return c
		}
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

// gameTagRe is the (USA), (Rev 1), [!] and similar that game file names carry.
var gameTagRe = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)

// ParseGameName splits a game's file name into its title and a short note made
// of its tags, e.g. "Mega Game, The (USA) (Rev 1)" -> "The Mega Game", "USA, Rev 1".
func ParseGameName(base string) (title, note string) {
	var tags []string
	for _, m := range gameTagRe.FindAllString(base, -1) {
		t := strings.Trim(strings.TrimSpace(m), "()[]")
		if t != "" && t != "!" {
			tags = append(tags, t)
		}
	}
	title = strings.TrimSpace(gameTagRe.ReplaceAllString(base, ""))
	title = strings.TrimSpace(strings.ReplaceAll(title, "_", " "))
	for _, art := range []string{"The", "A", "An"} {
		if s, ok := strings.CutSuffix(title, ", "+art); ok {
			title = art + " " + s
			break
		}
		if a, b, ok := strings.Cut(title, ", "+art+" - "); ok {
			title = art + " " + a + " - " + b
			break
		}
	}
	if title == "" {
		title = base
	}
	return title, strings.Join(tags, ", ")
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

func (s *Scanner) indexGame(ctx context.Context, lib *Library, c candidate) error {
	existing, err := s.Store.itemByPath(ctx, lib.ID, c.path)
	if err == nil && existing.MTime == c.mtime && existing.SizeBytes == c.size && !existing.Missing && existing.Console == c.console {
		return nil // unchanged
	}
	it := &Item{LibraryID: lib.ID, Kind: "game", Path: c.path, SizeBytes: c.size, MTime: c.mtime, Console: c.console}
	if existing != nil {
		it.ID, it.AddedAt = existing.ID, existing.AddedAt
	}
	base := strings.TrimSuffix(filepath.Base(c.path), filepath.Ext(c.path))
	dir := filepath.Dir(c.path)
	it.Title, it.Tagline = ParseGameName(base)
	it.SortTitle = SortTitle(it.Title)
	it.Container = strings.TrimPrefix(strings.ToLower(filepath.Ext(c.path)), ".")
	it.Poster = gameArt(dir, base)
	for _, nfo := range []string{filepath.Join(dir, base+".nfo")} {
		if applyNFO(it, nfo) {
			break
		}
	}
	if existing != nil && existing.Provider != "" {
		it.Provider, it.ProviderID = existing.Provider, existing.ProviderID
	}
	if existing != nil {
		// Scans never take away what someone chose or a source already found.
		if it.Poster == "" {
			it.Poster = existing.Poster
		}
		if it.Overview == "" {
			it.Overview = existing.Overview
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
	}
	if it.Overview == "" && it.Provider == "" && c.console != wittyplay.TestConsole && (s.WikidataOn == nil || s.WikidataOn()) {
		wctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		if err := (&Wikidata{Client: SafeClient}).EnrichGame(wctx, it); err != nil && !errors.Is(err, ErrNotFound) {
			s.Log.Debug("wikidata", "game", it.Title, "err", err)
		}
		cancel()
	}
	return s.Store.upsertItem(ctx, it)
}

const wdDeveloper = "P178"

// EnrichGame fills empty fields of a game from Wikidata, which is public domain
// (CC0): the year, developer, genres and a short summary. Box art is not taken
// from there, because it is copyrighted. It changes nothing when no video game of
// that name is found.
func (w *Wikidata) EnrichGame(ctx context.Context, it *Item) error {
	var search struct {
		Search []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
		} `json:"search"`
	}
	q := url.Values{"action": {"wbsearchentities"}, "search": {it.Title}, "language": {"en"}, "type": {"item"}, "limit": {"10"}, "format": {"json"}}
	if err := w.get(ctx, "https://www.wikidata.org/w/api.php?"+q.Encode(), &search); err != nil {
		return err
	}
	id := ""
	for _, r := range search.Search {
		d := strings.ToLower(r.Description)
		if strings.Contains(d, "video game") && !containsAny(d, "series", "franchise", "character", "company", "soundtrack", "console") {
			id = r.ID
			break
		}
	}
	if id == "" {
		return ErrNotFound
	}
	ents, err := w.entities(ctx, []string{id}, "claims|descriptions|sitelinks")
	if err != nil {
		return err
	}
	e, ok := ents[id]
	if !ok {
		return ErrNotFound
	}
	if c := e.Claims[wdPublication]; len(c) > 0 && it.Year == nil {
		if y, err := strconv.Atoi(firstN(c[0].Mainsnak.time(), 4)); err == nil {
			it.Year = &y
		}
	}
	var refs []string
	dev := ""
	if c := e.Claims[wdDeveloper]; len(c) > 0 {
		dev = c[0].Mainsnak.itemID()
		refs = append(refs, dev)
	}
	for _, c := range e.Claims[wdGenre] {
		if g := c.Mainsnak.itemID(); g != "" && len(refs) < 6 {
			refs = append(refs, g)
		}
	}
	if len(refs) > 0 {
		if names, err := w.entities(ctx, refs, "labels"); err == nil {
			var genres []string
			for _, r := range refs {
				l := names[r].Labels["en"].Value
				switch {
				case l == "":
				case r == dev:
					if it.Studio == "" {
						it.Studio = l
					}
				default:
					genres = append(genres, titleCase(strings.TrimSuffix(l, " video game")))
				}
			}
			if len(it.Genres) == 0 {
				it.Genres = genres
			}
		}
	}
	if it.Overview == "" {
		if page := e.Sitelinks["enwiki"].Title; page != "" {
			var sum struct {
				Extract string `json:"extract"`
			}
			if w.get(ctx, "https://en.wikipedia.org/api/rest_v1/page/summary/"+url.PathEscape(strings.ReplaceAll(page, " ", "_")), &sum) == nil {
				it.Overview = sum.Extract
			}
		}
		if it.Overview == "" {
			it.Overview = e.Descriptions["en"].Value
		}
	}
	it.Provider, it.ProviderID = "wikidata", id
	return nil
}
