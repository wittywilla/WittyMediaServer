package library

import (
	"context"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/gamearchive"
	"github.com/wittywilla/wittymediaserver/internal/gameid"
	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
	"github.com/wittywilla/wittyplay"
)

// Games made of several files, and games kept more than once. Three things happen here,
// after each file has been judged by itself:
//
//   - Copies of one file become one game: checksums settle it, however they're named.
//   - The discs of a game become one game with several discs.
//   - Versions of one game (regions, revisions) become one game with other versions
//     on its page. See linkGameVersions, which needs the games to have been listed first.

// gameFile is a game found by a scan.
type gameFile struct {
	cand    *candidate
	verdict gameVerdict
	base    string // its name, without its extension
	key     string // what its name comes down to
	disc    int
	total   int
	dir     string
}

// fileBaseName is a file or archive's name for working out its title: what's inside
// when its own name is no use.
func fileBaseName(p string, v gameVerdict) string {
	if v.Name != "" {
		return v.Name
	}
	return gameBaseName(filepath.Base(p))
}

// gameKey is what's left of a name once everything that isn't its title has gone.
func gameKey(stem string) (key string, disc, total int) {
	if gameid.IsSlug(stem) {
		d, t, rest := gameid.DiscIn(stem)
		disc, total = d, t
		stem = gameid.Unslug(rest)
	} else {
		disc, total, stem = gameid.DiscIn(stem)
	}
	title, _ := gamemeta.CleanName(stem)
	return gamemeta.NameKey(gamemeta.SearchTitle(title)), disc, total
}

// mergeGames applies what a scan knows about several files at once. It returns the
// games to list, each knowing its discs.
func (ix *archiveIndex) mergeGames(found []*gameFile, root string) []candidate {
	found = ix.dropDuplicates(found)
	return ix.groupDiscs(found)
}

// dropDuplicates finds files with the same contents. Only files the same size as
// another are ever read for this, and a checksum an archive already holds costs nothing.
func (ix *archiveIndex) dropDuplicates(found []*gameFile) []*gameFile {
	bySize := map[string][]*gameFile{}
	for _, g := range found {
		if g.verdict.Payload > 0 {
			bySize[g.cand.console+"|"+itoa(g.verdict.Payload)] = append(bySize[g.cand.console+"|"+itoa(g.verdict.Payload)], g)
		}
	}
	dup := map[*gameFile]*gameFile{}
	for _, group := range bySize {
		if len(group) < 2 {
			continue
		}
		byCRC := map[uint32][]*gameFile{}
		for _, g := range group {
			if crc := ix.payloadCRC(g); crc != 0 {
				byCRC[crc] = append(byCRC[crc], g)
			}
		}
		for _, same := range byCRC {
			if len(same) < 2 {
				continue
			}
			// The one that needs no unpacking stays, then the one named more plainly.
			sort.SliceStable(same, func(a, b int) bool {
				aa, ab := gamearchive.IsArchive(same[a].cand.path), gamearchive.IsArchive(same[b].cand.path)
				if aa != ab {
					return ab
				}
				return len(filepath.Base(same[a].cand.path)) < len(filepath.Base(same[b].cand.path))
			})
			for _, g := range same[1:] {
				dup[g] = same[0]
			}
		}
	}
	if len(dup) == 0 {
		return found
	}
	out := found[:0]
	for _, g := range found {
		if keep := dup[g]; keep != nil {
			ix.noteFile(g.cand.path, g.cand.size, reviewDuplicate, "This is the same game as "+filepath.Base(keep.cand.path)+": their checksums match.", g.cand.console, nil)
			continue
		}
		out = append(out, g)
	}
	return out
}

// payloadCRC is the checksum of a game's main file. It's kept in its verdict, and a loose
// file is only read once.
func (ix *archiveIndex) payloadCRC(g *gameFile) uint32 {
	if g.verdict.CRC != 0 {
		return g.verdict.CRC
	}
	if gamearchive.IsArchive(g.cand.path) {
		return 0
	}
	p := mainFileOf(g.cand.path)
	crc := crcOfFile(p)
	if crc == 0 {
		return 0
	}
	g.verdict.CRC = crc
	key := ix.lastLibrary + "|" + g.cand.path
	if c, ok := ix.seenVerdicts[key]; ok {
		c.v.CRC = crc
		ix.seenVerdicts[key] = c
		ix.freshVerdicts[key] = true
	}
	return crc
}

// mainFileOf is the file holding a game: a sheet's first data track, or the file itself.
func mainFileOf(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".cue", ".gdi", ".ccd", ".toc", ".m3u":
		if s, err := gameid.ReadSheet(p); err == nil && s.Data != "" {
			return s.Data
		}
	}
	return p
}

func sizeOf(p string) int64 {
	if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
		return st.Size()
	}
	return 0
}

func crcOfFile(p string) uint32 {
	f, err := os.Open(p)
	if err != nil {
		return 0
	}
	defer f.Close()
	h := crc32.NewIEEE()
	if _, err := io.Copy(h, f); err != nil {
		return 0
	}
	return h.Sum32()
}

// groupDiscs turns the discs of one game into one game. What it takes is a name that
// says which disc it is, in one folder, for one console. A disc that's missing is fine:
// its place stays empty, and the game still has the discs that are there.
func (ix *archiveIndex) groupDiscs(found []*gameFile) []candidate {
	type setKey struct{ console, dir, key string }
	sets := map[setKey][]*gameFile{}
	var out []candidate
	for _, g := range found {
		if g.disc == 0 {
			out = append(out, *g.cand)
			continue
		}
		k := setKey{g.cand.console, g.dir, g.key}
		sets[k] = append(sets[k], g)
	}
	for _, discs := range sets {
		sort.SliceStable(discs, func(a, b int) bool {
			if discs[a].disc != discs[b].disc {
				return discs[a].disc < discs[b].disc
			}
			// The same disc twice, and they aren't the same file: the unpacked one first.
			return !gamearchive.IsArchive(discs[a].cand.path) && gamearchive.IsArchive(discs[b].cand.path)
		})
		first := discs[0]
		c := *first.cand
		for i, d := range discs {
			if i > 0 && discs[i-1].disc == d.disc {
				ix.noteFile(d.cand.path, d.cand.size, reviewDuplicate, "This is another copy of disc "+itoa(int64(d.disc))+" of "+filepath.Base(first.cand.path)+".", d.cand.console, nil)
				continue
			}
			c.discs = append(c.discs, gameDisc{disc: d.disc, path: d.cand.path})
		}
		out = append(out, c)
	}
	return out
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// gameDisc is one disc of a game.
type gameDisc struct {
	disc int
	path string
}

// saveDiscs records which discs a game has. Only a game with more than one has any rows.
func (s *Store) saveDiscs(ctx context.Context, itemID int64, discs []gameDisc) error {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM game_discs WHERE item_id = ?`, itemID); err != nil {
		return err
	}
	if len(discs) < 2 {
		return nil
	}
	for _, d := range discs {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO game_discs(item_id, disc, path) VALUES(?,?,?)`, itemID, d.disc, d.path); err != nil {
			return err
		}
	}
	return nil
}

// GameDisc is one disc of a game, as far as anyone asking needs to know.
type GameDisc struct {
	Disc int    `json:"disc"`
	Path string `json:"-"`
	Name string `json:"name"`
}

// GameDiscs lists a game's discs, in order. Empty for a game on one disc.
func (s *Store) GameDiscs(ctx context.Context, itemID int64) ([]GameDisc, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT disc, path FROM game_discs WHERE item_id = ? ORDER BY disc`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GameDisc{}
	for rows.Next() {
		var d GameDisc
		if err := rows.Scan(&d.Disc, &d.Path); err != nil {
			return nil, err
		}
		d.Name = filepath.Base(d.Path)
		out = append(out, d)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- versions of one game

// versionTagRe spots a pre-release, which is never what to show first.
var versionTagRe = regexp.MustCompile(`(?i)\b(?:beta|alpha|proto|prototype|demo|sample|pre-?release|preview|hack|pirate)\b`)

// versionRank orders versions of a game: a released version in a widely-played region
// first, then the newest revision of it. Lower is better.
func versionRank(tags string) int {
	t := strings.ToLower(tags)
	rank := 0
	if versionTagRe.MatchString(t) {
		rank += 1000
	}
	switch {
	case strings.Contains(t, "usa"), strings.Contains(t, "world"):
	case strings.Contains(t, "europe"), strings.Contains(t, "australia"):
		rank += 100
	case strings.Contains(t, "japan"):
		rank += 200
	default:
		rank += 300
	}
	return rank
}

var revisionRe = regexp.MustCompile(`(?i)\b(?:rev|v|version)\s*\.?\s*([0-9a-z.]+)`)

func revisionOf(tags string) string {
	if m := revisionRe.FindStringSubmatch(tags); m != nil {
		return strings.ToLower(m[1])
	}
	return ""
}

// linkGameVersions puts versions of one game under one tile: games with the same name,
// on one console, in one library. Which goes first stays first once chosen, as a person's
// saves belong to it. Someone who has split a game off has the last word. A game's own
// title is what its file calls it here, not what a source does, so nothing changes when
// a lookup comes back.
func (s *Store) linkGameVersions(ctx context.Context, lib *Library) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id, i.path, i.console, IFNULL(i.tagline, ''), IFNULL(l.parent_item, 0), IFNULL(l.link, '') FROM items i
		LEFT JOIN item_links l ON l.item_id = i.id WHERE i.library_id = ? AND i.kind = 'game' AND i.missing = 0`, lib.ID)
	if err != nil {
		return err
	}
	type row struct {
		id, parent int64
		path, tags string
		link       string
	}
	groups := map[string][]*row{}
	for rows.Next() {
		r := &row{}
		var console string
		if err := rows.Scan(&r.id, &r.path, &console, &r.tags, &r.parent, &r.link); err != nil {
			rows.Close()
			return err
		}
		stem := gameBaseName(filepath.Base(r.path))
		key, _, _ := gameKey(stem)
		if key == "" || console == "" || console == RigConsole {
			continue
		}
		_, r.tags = gamemeta.CleanName(stem)
		groups[console+"|"+key] = append(groups[console+"|"+key], r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	own := map[string]bool{}
	dec, err := s.DB.QueryContext(ctx, `SELECT path FROM game_decisions WHERE library_id = ? AND action = 'own'`, lib.ID)
	if err != nil {
		return err
	}
	for dec.Next() {
		var p string
		if dec.Scan(&p) == nil {
			own[p] = true
		}
	}
	dec.Close()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, group := range groups {
		var members []*row
		for _, r := range group {
			if !own[r.path] && (r.link == "" || r.link == LinkVersion) {
				members = append(members, r)
			}
		}
		if len(members) < 2 {
			continue
		}
		// Only versions that really differ belong together: two files with no region or
		// revision to tell them apart are different games that share a name.
		labels := map[string]bool{}
		for _, r := range members {
			labels[strings.ToLower(r.tags)] = true
		}
		if len(labels) < 2 {
			continue
		}
		sort.SliceStable(members, func(a, b int) bool {
			ra, rb := versionRank(members[a].tags), versionRank(members[b].tags)
			if ra != rb {
				return ra < rb
			}
			// The newest revision of the same region.
			return revisionOf(members[a].tags) > revisionOf(members[b].tags)
		})
		head := members[0]
		// One already at the head of these keeps its place.
		heads := map[int64]bool{}
		for _, r := range members {
			if r.parent != 0 {
				heads[r.parent] = true
			}
		}
		for _, r := range members {
			if heads[r.id] {
				head = r
				break
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM item_links WHERE item_id = ?`, head.id); err != nil {
			return err
		}
		for _, r := range members {
			if r == head {
				continue
			}
			label := r.tags
			if len(label) > 80 {
				label = label[:80]
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO item_links(item_id, link, extra_type, parent_item, parent_show, label, created_at) VALUES(?,?,'',?,NULL,?,strftime('%s','now'))
				ON CONFLICT(item_id) DO UPDATE SET link = excluded.link, parent_item = excluded.parent_item, parent_show = NULL, label = excluded.label`, r.id, LinkVersion, head.id, label); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ConsoleLogoName is the name a console goes by for finding its logo, which is its
// common name in English: what a Wikidata entry is called.
func ConsoleLogoName(id string) string {
	if n, ok := consoleLogoNames[id]; ok {
		return n
	}
	if c := wittyplay.ByID(id); c != nil {
		return c.Name
	}
	return ""
}

var consoleLogoNames = map[string]string{
	"nes": "Nintendo Entertainment System", "snes": "Super Nintendo Entertainment System", "gb": "Game Boy", "gba": "Game Boy Advance",
	"md": "Sega Genesis", "sms": "Master System", "gg": "Game Gear", "ps1": "PlayStation", "ps2": "PlayStation 2", "psp": "PlayStation Portable",
	"pce": "TurboGrafx-16", "a2600": "Atari 2600", "a7800": "Atari 7800", "lynx": "Atari Lynx", "ws": "WonderSwan", "ngp": "Neo Geo Pocket",
	"vb": "Virtual Boy", "n64": "Nintendo 64", "nds": "Nintendo DS", "3ds": "Nintendo 3DS", "dc": "Dreamcast",
}

// PinGameEntry says which file in a game's archive to play, or "" to choose by the rules.
// That's a different file, so what's known about it is worked out again.
func (s *Store) PinGameEntry(ctx context.Context, it *Item, entry string) error {
	if _, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO game_meta(item_id, library_id) VALUES(?, ?)`, it.ID, it.LibraryID); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM game_meta_sources WHERE item_id = ? AND method <> 'manual'`, it.ID); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE game_meta SET chosen_entry = ?, file_size = -1, status = CASE WHEN locked = 1 THEN status ELSE 'pending' END, next_try = 0, tries = 0 WHERE item_id = ?`, entry, it.ID)
	return err
}

// GameEntry says which file in a game's archive someone chose, or "".
func (s *Store) GameEntry(ctx context.Context, id int64) string {
	var e string
	_ = s.DB.QueryRowContext(ctx, `SELECT chosen_entry FROM game_meta WHERE item_id = ?`, id).Scan(&e)
	return e
}

// DecideGamePath records what a person said about a game's file. The next scan goes by it.
func (s *Store) DecideGamePath(ctx context.Context, it *Item, action, console string) error {
	return s.decideGamePath(ctx, it.LibraryID, it.Path, action, console)
}

// SplitGameVersion makes a version of a game a tile of its own, and keeps it that way.
func (s *Store) SplitGameVersion(ctx context.Context, lib *Library, it *Item) error {
	if err := s.DetachItem(ctx, it.ID); err != nil {
		return err
	}
	return s.decideGamePath(ctx, lib.ID, it.Path, "own", "")
}
