// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/gamearchive"
	"github.com/wittywilla/wittymediaserver/internal/gameid"
	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
)

// Games libraries: one console, or wittyplay.Auto for a folder holding many. See
// docs/GAMING.md. ROM files are only listed here, never opened beyond a look at
// which files an archive, .cue or .m3u names. What's known about them comes later,
// in the background: see games_meta.go.

// gameSkipDirs hold things that belong to an emulator rather than games.
var gameSkipDirs = map[string]bool{"bios": true, "system": true, "saves": true, "savestates": true, "states": true, "screenshots": true,
	"boxart": true, "covers": true, "cover": true, "images": true, "media": true, "manuals": true, "__macosx": true, "@eadir": true}

// dumpGame is the file that stands for a whole game kept as a folder, when dir is one: a PS3
// disc's PS3_DISC.SFB beside its PS3_GAME folder, or a Wii U game's code/*.rpx beside its meta
// folder. Names are matched in any case.
func dumpGame(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) > 64 {
		return "" // a dump's root holds a handful of things; a folder of games holds more
	}
	var sfb, ps3Game, code, meta string
	for _, e := range entries {
		switch n := strings.ToLower(e.Name()); {
		case n == "ps3_disc.sfb" && !e.IsDir():
			sfb = e.Name()
		case n == "ps3_game" && e.IsDir():
			ps3Game = e.Name()
		case n == "code" && e.IsDir():
			code = e.Name()
		case n == "meta" && e.IsDir():
			meta = e.Name()
		}
	}
	if sfb != "" && ps3Game != "" {
		return filepath.Join(dir, sfb)
	}
	if code != "" && meta != "" {
		list, err := os.ReadDir(filepath.Join(dir, code))
		if err != nil {
			return ""
		}
		for _, e := range list {
			if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".rpx") {
				return filepath.Join(dir, code, e.Name())
			}
		}
	}
	return ""
}

// discoverGames lists the games under a root. Multi-file games become one: a
// .cue hides the tracks it names and an .m3u hides the discs it names, whatever
// the case of the names in it.
func discoverGames(root, console string) ([]candidate, error) {
	ix := newArchiveIndex(nil, 0)
	defer ix.close()
	return ix.discover(root, console)
}

func (ix *archiveIndex) discover(root, console string) ([]candidate, error) {
	ix.verdictOnce.Do(ix.loadVerdicts)
	ix.lastLibrary = console
	freshConsoles()
	type seen struct {
		path, rel   string
		size, mtime int64
	}
	var files []seen
	hidden := map[string]bool{}
	var bad unreadable
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			bad.note(err)
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") || gameSkipDirs[strings.ToLower(name)]) {
				return filepath.SkipDir
			}
			// A PS3 disc or a Wii U game kept as a folder is one game of thousands of files: it's
			// listed by the one file every such dump has, and nothing else in it is looked at.
			if game := dumpGame(p); game != "" {
				if rel, err := filepath.Rel(root, game); err == nil {
					if st, err := os.Stat(game); err == nil && st.Mode().IsRegular() {
						files = append(files, seen{game, rel, st.Size(), st.ModTime().Unix()})
					}
				}
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || !mediaEntry(root, p, d) {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		st, err := d.Info()
		if err != nil {
			return nil
		}
		// What a sheet names is part of what it describes, whatever else it might be.
		switch strings.ToLower(filepath.Ext(name)) {
		case ".cue", ".m3u", ".gdi", ".ccd", ".toc":
			if sheet, err := gameid.ReadSheet(p); err == nil {
				for _, f := range sheet.Files {
					hidden[strings.ToLower(filepath.Clean(f))] = true
				}
			}
		}
		files = append(files, seen{p, rel, st.Size(), st.ModTime().Unix()})
		return nil
	})
	var found []*gameFile
	for i, f := range files {
		if ix.looking != nil {
			ix.looking(i, len(files))
		}
		// Between files, never within one: a file not looked at must not be judged, as these
		// verdicts are saved and believed by every scan after this one.
		if ix.stopped != nil && ix.stopped() {
			ix.saveVerdicts(context.Background())
			return nil, context.Canceled
		}
		if hidden[strings.ToLower(filepath.Clean(f.path))] {
			continue
		}
		d, decided := ix.decisions[f.path]
		if decided && d.Action == "ignore" {
			ix.noteFile(f.path, f.size, reviewIgnored, "You said this isn't a game.", "", nil)
			continue
		}
		var v gameVerdict
		switch {
		case decided && d.Action == "add" && consoles().byID(d.Console) != nil:
			v = accept(d.Console, howChosen, 100, "You said what this is.")
		default:
			v = ix.verdictFor(f.path, f.rel, console, f.size, f.mtime)
		}
		if !v.isGame() {
			if v.Left != "" {
				ix.noteFile(f.path, f.size, v.Left, v.Why, "", v.Suggest)
			}
			continue
		}
		c := &candidate{path: f.path, kind: "game", size: f.size, mtime: f.mtime, console: v.Console, verdict: &v}
		if v.Payload == 0 {
			v.Payload = sizeOf(mainFileOf(f.path))
		}
		g := &gameFile{cand: c, verdict: v, dir: filepath.Dir(f.path)}
		g.key, g.disc, g.total = gameKey(fileBaseName(f.path, v))
		if g.disc == 0 {
			// Discs are often kept in folders of their own: Game/Disc 2/game.cue.
			if parent := filepath.Base(g.dir); parent != "." {
				if disc, total, rest := gameid.DiscIn(parent); disc > 0 {
					g.disc, g.total = disc, total
					g.key, _, _ = gameKey(rest)
					g.dir = filepath.Dir(g.dir)
				}
			}
		}
		found = append(found, g)
		if v.Problem != "" {
			ix.noteFile(f.path, f.size, reviewBlocked, problemText(v)+" "+v.Proof, v.Console, nil)
		}
	}
	return ix.mergeGames(found), bad.with(err)
}

// DecryptedCopyPlays is what a person with an encrypted game can do: nothing here decrypts one,
// or looks for keys, but a copy they decrypted themselves plays like any other game.
const DecryptedCopyPlays = "A copy you decrypted yourself, from a console you own, will play."

// problemText says why a game on the list can't play as it is.
func problemText(v gameVerdict) string {
	switch v.Problem {
	case "encrypted":
		return "This game file is encrypted, as it came from the console or the store. It can't be played here, and this server won't decrypt it. " + DecryptedCopyPlays
	}
	return v.Proof
}

// skippedGameFiles counts, by extension, game-like files a library can't use. An
// archive that can't be read at all counts by why instead (see gamearchive.Error).
func skippedGameFiles(root, console string) map[string]int {
	ix := newArchiveIndex(nil, 0)
	defer ix.close()
	return ix.skipped(root, console)
}

func (ix *archiveIndex) skipped(root, console string) map[string]int {
	_, _ = ix.discover(root, console)
	out := map[string]int{}
	for _, n := range ix.notes {
		if n.kind == reviewNeeds || n.kind == reviewBlocked {
			key := strings.ToLower(filepath.Ext(n.path))
			if code := problemCode(n.reason); code != "" {
				key = code
			}
			out[key]++
		}
	}
	return out
}

// problemCode reads back which archive problem a note is about.
func problemCode(reason string) string {
	for _, code := range []string{gamearchive.ErrEncrypted.Code, gamearchive.ErrDamaged.Code, gamearchive.ErrTooBig.Code, gamearchive.ErrTooMany.Code, gamearchive.ErrUnsafe.Code} {
		if reason == archiveProblem(code) {
			return code
		}
	}
	return ""
}

// SkippedGames says how many files in a games library need a look, by extension or by
// what's wrong with them.
func (s *Store) SkippedGames(ctx context.Context, libraryID int64) map[string]int {
	rows, err := s.DB.QueryContext(ctx, `SELECT path, reason FROM game_review WHERE library_id = ? AND kind = ?`, libraryID, reviewNeeds)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var p, reason string
		if rows.Scan(&p, &reason) != nil {
			continue
		}
		key := strings.ToLower(filepath.Ext(p))
		if code := problemCode(reason); code != "" {
			key = code
		}
		out[key]++
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// GameCounts says how many games a library holds for each console.
func (s *Store) GameCounts(ctx context.Context, libraryID int64, access *Access) (map[string]int, error) {
	q := `SELECT IFNULL(i.console, ''), COUNT(*) FROM items i WHERE i.library_id = ? AND i.kind = 'game' AND i.missing = 0 AND ` + hideLinked
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
	if x := strings.ToLower(filepath.Ext(base)); x != "" && consoles().known(x) {
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
			for _, ext := range mediatypes.ArtworkExts {
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
	for _, ext := range mediatypes.ArtworkExts {
		p := filepath.Join(dir, base+"-fanart"+ext)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func (s *Scanner) indexGame(ctx context.Context, lib *Library, c candidate) error {
	v := gameVerdict{}
	if c.verdict != nil {
		v = *c.verdict
	}
	existing, err := s.Store.itemByPath(ctx, lib.ID, c.path)
	unchanged := err == nil && existing.MTime == c.mtime && existing.SizeBytes == c.size && existing.Console == c.console
	// A game listed before files were judged needs going over once.
	judged := existing != nil && s.Store.gameJudged(ctx, existing.ID)
	if unchanged && !existing.Missing && judged {
		if err := s.Store.saveDiscs(ctx, existing.ID, c.discs); err != nil {
			return err
		}
		if err := s.Store.noteSidecar(ctx, existing); err != nil {
			return err
		}
		return s.Store.noteDetection(ctx, existing, v)
	}
	it := &Item{LibraryID: lib.ID, Kind: "game", Path: c.path, SizeBytes: c.size, MTime: c.mtime, Console: c.console}
	if existing != nil {
		it.ID, it.AddedAt = existing.ID, existing.AddedAt
	}
	base := gameBaseName(filepath.Base(c.path))
	dir := filepath.Dir(c.path)
	it.Title, it.Tagline = gameFileTitle(fileBaseName(c.path, v), v)
	it.SortTitle = SortTitle(it.Title)
	it.Container = strings.TrimPrefix(strings.ToLower(filepath.Ext(c.path)), ".")
	it.Poster = gameArt(dir, base)
	it.Backdrop = gameBackdrop(dir, base)
	parsed, tags := it.Title, it.Tagline
	nfo := filepath.Join(dir, base+".nfo")
	applyNFO(it, nfo)
	// What the owner's own files say stays: sources only fill in the rest. An NFO this
	// server wrote is only what it thought then, and a better answer can replace it. The
	// pictures saved beside it are the same.
	local := localGameFields(it, parsed, tags)
	var earlier []string
	if nfoWrittenHere(nfo) {
		local, earlier = nil, local
	}
	if existing != nil && existing.Provider != "" {
		it.Provider, it.ProviderID = existing.Provider, existing.ProviderID
	}
	if existing != nil {
		// Scans never take away what someone typed, whatever else this scan believes, or what
		// a source already found.
		kept := existing.Provider != "" || s.Store.gameFieldEdited(ctx, existing.ID, "title")
		if s.Store.gameFieldTyped(ctx, existing.ID, "title") || (kept && !slices.Contains(local, "title") && judged) {
			it.Title, it.SortTitle = existing.Title, existing.SortTitle
		}
		if s.Store.gameFieldTyped(ctx, existing.ID, "tagline") {
			it.Tagline = existing.Tagline
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
	if err := s.Store.saveDiscs(ctx, it.ID, c.discs); err != nil {
		return err
	}
	// A .wittyplay file beside it says how WittyPlay pictures it (read only, never written).
	if err := s.Store.noteSidecar(ctx, it); err != nil {
		return err
	}
	// Finding out about it happens in the background, so a scan never waits on the internet.
	// A file that's back where it was is the same game: what's known about it still holds.
	if !unchanged || !judged {
		if err := s.Store.enqueueGame(ctx, it, local, earlier); err != nil {
			return err
		}
	}
	return s.Store.noteDetection(ctx, it, v)
}

// nfoWrittenHere says an NFO beside a game is one this server saved earlier, and not one
// a person wrote.
func nfoWrittenHere(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 256)
	n, _ := f.Read(b)
	return strings.Contains(string(b[:n]), nfoMarker)
}

// gameFileTitle reads a title from a file's name. A disc's number isn't part of its
// title, and neither is the way a web address writes it. What's inside says more than the
// archive's name does about its region, as sets name archives after the game alone.
func gameFileTitle(stem string, v gameVerdict) (title, tags string) {
	disc, _, rest := gameid.DiscIn(stem)
	if gameid.IsSlug(stem) {
		rest = titleCase(gameid.Unslug(rest))
	}
	title, tags = gamemeta.CleanName(rest)
	if tags == "" && v.Entry != "" {
		_, tags = gamemeta.CleanName(strings.TrimSuffix(pathBase(v.Entry), pathExt(v.Entry)))
	}
	if tags == "" {
		tags = v.Region
	}
	if disc > 0 && tags == "" {
		tags = "Disc " + itoa(int64(disc))
	}
	return title, tags
}

func pathBase(s string) string { return filepath.Base(filepath.FromSlash(s)) }
func pathExt(s string) string  { return filepath.Ext(filepath.FromSlash(s)) }

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
