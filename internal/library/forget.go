// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// Forgetting what was downloaded, so a library can be matched and filled in again from its
// file names, as on a new server. Two parts, each optional:
//
//   - files: what this server saved beside the media (artwork, NFOs, CD sidecars). Only files
//     it recorded writing, and NFOs that carry its marker, are removed; a person's own NFO that
//     was kept as <name>.nfo.bak goes back in its place. Nothing else in the folders is touched,
//     and nothing outside them is reached through a link. This runs on a read-only library too:
//     read-only stops the server adding to the folders, and these are its own leftovers.
//   - details: what services said about each film, episode and show (overview, genres,
//     ratings, artwork, the match itself, cast and crew, where to watch), and the pictures of
//     them kept in the artwork cache. Titles go back to what their file names say, and every
//     item keeps its ID, so what people watched, their progress, ratings, lists and favourites
//     stay. Fields a person locked or typed stay as they are.

// ForgetOptions says which parts to forget.
type ForgetOptions struct {
	Files   bool
	Details bool
	// ImagesDir is the artwork cache, where downloaded pictures and their smaller copies are.
	ImagesDir string
}

// ForgetResult says what was done.
type ForgetResult struct {
	Libraries int `json:"libraries"`
	// Files beside the media.
	Removed    int      `json:"removed"`    // files of the server's removed
	Restored   int      `json:"restored"`   // a person's NFO put back from its .nfo.bak
	Companions int      `json:"companions"` // macOS "._" files that went with them
	Skipped    int      `json:"skipped"`    // recorded, but changed since or couldn't be removed
	Titles     int      `json:"titles"`     // films and episodes reset
	Shows      int      `json:"shows"`      // shows reset
	Cached     int      `json:"cached"`     // pictures removed from the artwork cache
	Logos      int      `json:"logos"`      // downloaded logos dropped (every library only)
	LockedKept int      `json:"lockedKept"` // fields kept because a person locked or typed them
	NotReset   []string `json:"notReset"`   // libraries whose kind of details aren't looked up this way
	Problems   []string `json:"problems,omitempty"`
}

func (r *ForgetResult) problem(s string) {
	if len(r.Problems) < 50 {
		r.Problems = append(r.Problems, s)
	}
}

// ForgetsDetails: a library whose titles are looked up by file name, so forgetting their
// details means something. Music, books, photos and games keep their own kinds of facts.
func ForgetsDetails(typ string) bool {
	return matchableLibrary(typ) || typ == "videos"
}

// savedBeside records that the server wrote path beside the media.
func (s *Store) savedBeside(ctx context.Context, path string) {
	if s == nil || s.DB == nil || path == "" {
		return
	}
	var size int64
	if st, err := os.Lstat(path); err == nil {
		size = st.Size()
	}
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO saved_files(path, written_at, size) VALUES(?,?,?)
		ON CONFLICT(path) DO UPDATE SET written_at = excluded.written_at, size = excluded.size`, filepath.Clean(path), time.Now().Unix(), size)
}

// RemoteArtPath is where the artwork cache keeps a picture from the internet. The name is
// the cache's own: nothing from the address becomes part of it.
func RemoteArtPath(imagesDir, u string) string {
	h := sha1.Sum([]byte(u))
	return filepath.Join(imagesDir, "r-"+hex.EncodeToString(h[:10]))
}

// smallCopies are where the smaller copies of a picture would be (GameMeta.SmallCopyOf names
// them by the picture's reference and when it last changed).
func smallCopies(imagesDir, src string, mod time.Time) []string {
	h := sha1.Sum([]byte(src + "|" + mod.String()))
	var out []string
	seen := map[int]bool{}
	for _, w := range ThumbWidths {
		w = SnapThumbWidth(min(w, thumbMaxWidth))
		if seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, filepath.Join(imagesDir, "gamethumb-"+hex.EncodeToString(h[:12])+"-"+strconv.Itoa(w)+".jpg"))
	}
	return out
}

// ---------------------------------------------------------------------------
// The files beside the media
// ---------------------------------------------------------------------------

// appleDoubleMagic starts every macOS "._" file, which holds another file's extra attributes.
var appleDoubleMagic = []byte{0x00, 0x05, 0x16, 0x07}

// ForgetFiles removes what the server saved into a library's folders. The library's read-only
// switch doesn't stop it: these are the server's own files.
func (s *Store) ForgetFiles(ctx context.Context, lib *Library, opt ForgetOptions, res *ForgetResult) error {
	roots := make([]string, 0, len(lib.Paths))
	for _, p := range lib.Paths {
		if !filepath.IsAbs(p) {
			continue
		}
		// A drive or share that is away reads as missing or empty: what's on record for it
		// stays on record until it's back.
		entries, err := os.ReadDir(p)
		if err != nil {
			res.problem(p + ": can't be reached, so nothing in it was removed")
			continue
		}
		if len(entries) == 0 {
			continue
		}
		roots = append(roots, filepath.Clean(p))
	}
	if len(roots) == 0 {
		return nil
	}
	type saved struct {
		path      string
		writtenAt int64
		size      int64
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT path, written_at, size FROM saved_files`)
	if err != nil {
		return err
	}
	var tracked []saved
	for rows.Next() {
		var f saved
		if err := rows.Scan(&f.path, &f.writtenAt, &f.size); err != nil {
			rows.Close()
			return err
		}
		if rootOf(f.path, roots) != "" {
			tracked = append(tracked, f)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	done := map[string]bool{}
	var removed []string
	for _, f := range tracked {
		if err := ctx.Err(); err != nil {
			return err
		}
		done[f.path] = true
		root := rootOf(f.path, roots)
		got, err := s.removeSaved(root, f.path, func(st fs.FileInfo, head []byte) bool {
			if isNFOName(f.path) {
				return bytes.Contains(head, []byte(nfoMarker))
			}
			// Someone may have put a picture of their own at the same name since.
			return st.Size() == f.size && st.ModTime().Unix() <= f.writtenAt+2
		}, opt, res)
		switch {
		case err != nil:
			res.Skipped++
			res.problem(f.path + ": " + err.Error())
			continue // kept on record, to try again
		case got == removedOurs:
			removed = append(removed, f.path)
		case got == notOurs:
			res.Skipped++ // the server wrote it once, and someone has replaced it since
		}
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM saved_files WHERE path = ?`, f.path)
	}
	// NFOs written before files were recorded carry the marker: found by walking the folders.
	// A walk never follows a link to a folder.
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			if err != nil {
				if d != nil && d.IsDir() && p != root {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() || !d.Type().IsRegular() || !isNFOName(p) || done[p] {
				return nil
			}
			done[p] = true
			// A person's own NFO (no marker) is simply not the server's: not counted.
			got, err := s.removeSaved(root, p, func(_ fs.FileInfo, head []byte) bool { return bytes.Contains(head, []byte(nfoMarker)) }, opt, res)
			if err != nil {
				res.Skipped++
				res.problem(p + ": " + err.Error())
			} else if got == removedOurs {
				removed = append(removed, p)
			}
			return nil
		})
		if err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
	}
	// Titles pointing at a picture that has gone go without one until they're scanned again.
	for _, p := range removed {
		for _, q := range []string{`UPDATE items SET poster = '' WHERE poster = ?`, `UPDATE items SET backdrop = '' WHERE backdrop = ?`,
			`UPDATE shows SET poster = '' WHERE poster = ?`, `UPDATE shows SET backdrop = '' WHERE backdrop = ?`, `UPDATE seasons SET poster = NULL WHERE poster = ?`} {
			if _, err := s.DB.ExecContext(ctx, q, p); err != nil {
				return err
			}
		}
	}
	return nil
}

func isNFOName(p string) bool { return strings.EqualFold(filepath.Ext(p), ".nfo") }

// What became of one file the server may have saved.
type removal int

const (
	absent      removal = iota // nothing there
	notOurs                    // something there that isn't the server's: left alone
	removedOurs                // the server's, removed
)

// removeSaved removes one file of the server's from inside root, if ours says it is: it is
// given the file's details and, for an NFO, its contents (pictures aren't read). Everything is
// done through root, so a folder swapped for a link on the way can't lead anywhere else.
func (s *Store) removeSaved(root, path string, ours func(st fs.FileInfo, head []byte) bool, opt ForgetOptions, res *ForgetResult) (got removal, err error) {
	if strings.HasSuffix(strings.ToLower(path), ".bak") {
		return notOurs, nil // a person's file, kept by the server: never removed as the server's
	}
	err = inRoot(root, path, func(r *os.Root, rel string) error {
		st, err := r.Lstat(rel)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !st.Mode().IsRegular() {
			got = notOurs // a link or a folder now: whatever it is, it isn't what was written
			return nil
		}
		var head []byte
		if isNFOName(rel) {
			if head, err = readRootFile(r, rel, maxNFO); err != nil {
				return err
			}
		}
		if !ours(st, head) {
			got = notOurs
			return nil
		}
		if err := r.Remove(rel); err != nil {
			return err
		}
		got = removedOurs
		res.Removed++
		if opt.ImagesDir != "" && !isNFOName(rel) {
			for _, c := range smallCopies(opt.ImagesDir, path, st.ModTime()) {
				if os.Remove(c) == nil {
					res.Cached++
				}
			}
		}
		if removeAppleDouble(r, rel) {
			res.Companions++
		}
		// A person's own NFO, kept when the server wrote over it, goes back.
		if isNFOName(rel) {
			bak := rel + ".bak"
			if bst, err := r.Lstat(bak); err == nil && bst.Mode().IsRegular() {
				if err := r.Rename(bak, rel); err != nil {
					return err
				}
				res.Restored++
				// And its macOS companion, if it has one and the name is free.
				from, to := appleDoubleOf(bak), appleDoubleOf(rel)
				if isAppleDouble(r, from) {
					if _, err := r.Lstat(to); errors.Is(err, fs.ErrNotExist) {
						_ = r.Rename(from, to)
					}
				}
			}
		}
		return nil
	}, func() error {
		return errors.New("can't be reached safely")
	})
	return got, err
}

// readRootFile reads up to limit bytes of a plain file inside r, without waiting on a pipe.
func readRootFile(r *os.Root, rel string, limit int64) ([]byte, error) {
	f, err := r.OpenFile(rel, os.O_RDONLY|openNoBlock, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		return nil, fs.ErrInvalid
	}
	return io.ReadAll(io.LimitReader(f, limit))
}

func appleDoubleOf(rel string) string {
	return filepath.Join(filepath.Dir(rel), "._"+filepath.Base(rel))
}

// isAppleDouble: rel is a plain file that starts as macOS's "._" files do.
func isAppleDouble(r *os.Root, rel string) bool {
	st, err := r.Lstat(rel)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	head, err := readRootFile(r, rel, int64(len(appleDoubleMagic)))
	return err == nil && bytes.Equal(head, appleDoubleMagic)
}

// removeAppleDouble removes the "._" file beside rel, if there is one and it really is one.
func removeAppleDouble(r *os.Root, rel string) bool {
	c := appleDoubleOf(rel)
	return isAppleDouble(r, c) && r.Remove(c) == nil
}

// ---------------------------------------------------------------------------
// The details in the database
// ---------------------------------------------------------------------------

// personal: a person locked the field or typed it, so it stays.
func personal(m *TitleMeta, f string) bool {
	return m.Locks[f] || m.Sources[f] == SourceYou
}

// cleanMeta is a title's row as a new scan starts it, with what a person owns carried over.
func cleanMeta(prev *TitleMeta) *TitleMeta {
	m := newTitleMeta()
	for f, on := range prev.Locks {
		if on {
			m.Locks[f] = true
		}
	}
	for f, src := range prev.Sources {
		if personal(prev, f) {
			m.Sources[f] = src
		}
	}
	for f, b := range prev.Baseline {
		if personal(prev, f) {
			m.Baseline[f] = b
		}
	}
	m.AudioFormat = prev.AudioFormat // read from the file, not downloaded
	return m
}

// keepPersonal copies what a person owns from prev onto clean, and counts it.
func keepPersonal(prev, clean *Title, res *ForgetResult) {
	for _, f := range prev.FieldNames() {
		if !personal(prev.Meta, f) {
			continue
		}
		if err := clean.SetField(f, prev.Field(f)); err == nil {
			res.LockedKept++
		}
	}
	if !personal(prev.Meta, "sortTitle") {
		_ = clean.SetField("sortTitle", SortTitle(clean.Field("title")))
	}
}

// dropBrands takes a title's studios, networks or film series off unless a person set them.
func (s *Store) dropBrands(ctx context.Context, t *Title) error {
	col := "item_id"
	if t.Target == TargetShow {
		col = "show_id"
	}
	for kind, field := range map[string]string{LogoStudio: "studios", LogoNetwork: "network", BrandSeries: "series"} {
		if personal(t.Meta, field) {
			continue
		}
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM title_brands WHERE kind = ? AND `+col+` = ?`, kind, t.ID); err != nil {
			return err
		}
	}
	return nil
}

// remoteRefs collects addresses of pictures from the internet.
type remoteRefs map[string]bool

func (r remoteRefs) add(vs ...string) {
	for _, v := range vs {
		if isRemote(v) {
			r[v] = true
		}
	}
}

// ForgetDetails resets a library's films, episodes and shows to what a new scan of their
// files starts from, keeping every ID and whatever a person locked or typed. The next scan
// reads each file again (its size and time are forgotten too) and looks it up afresh.
// Pictures from the internet that nothing uses any more leave the artwork cache.
func (s *Store) ForgetDetails(ctx context.Context, lib *Library, opt ForgetOptions, res *ForgetResult) error {
	if !ForgetsDetails(lib.Type) {
		res.NotReset = append(res.NotReset, lib.Name)
		return nil
	}
	dropped := remoteRefs{}
	// The where-to-watch answers asked by these titles' TMDB ids.
	tmdbIDs := map[string]bool{}
	ids := func(q string) ([]int64, error) {
		rows, err := s.DB.QueryContext(ctx, q, lib.ID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		return out, rows.Err()
	}
	// CD images in a discs library are music, read from MusicBrainz and the disc: left alone.
	itemIDs, err := ids(`SELECT id FROM items WHERE library_id = ? AND kind NOT IN ('` + CDKind + `', '` + playlistKind + `') AND cd_disc IS NULL`)
	if err != nil {
		return err
	}
	showIDs, err := ids(`SELECT id FROM shows WHERE library_id = ?`)
	if err != nil {
		return err
	}
	rules := s.loadRuleSet(ctx, lib)
	now := db.Now()
	for _, id := range itemIDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		prev, err := s.LoadTitle(ctx, TargetItem, id)
		if err != nil {
			continue // gone meanwhile
		}
		p := prev.Item
		if p.Provider == "tmdb" && p.ProviderID != "" {
			tmdbIDs[p.ProviderID] = true
		}
		it := *p
		it.Overview, it.Tagline, it.Genres, it.Rating, it.ContentRating, it.Studio = "", "", nil, nil, "", ""
		it.Poster, it.Backdrop, it.Provider, it.ProviderID, it.Aired = "", "", "", "", ""
		if p.ShowID != nil {
			ref := episodeRef(lib, p.Path)
			it.Title, it.Year = ref.Title, nil
			if it.Title == "" && it.Episode != nil {
				it.Title = "Episode " + strconv.Itoa(*it.Episode)
			}
		} else {
			it.Title, it.Year, _, _ = nameFromFile(lib, p.Path, p.Kind, rules)
		}
		if strings.TrimSpace(it.Title) == "" {
			it.Title = strings.TrimSuffix(filepath.Base(p.Path), filepath.Ext(p.Path))
		}
		it.SortTitle = SortTitle(it.Title)
		clean := &Title{Target: TargetItem, ID: id, Item: &it, Meta: cleanMeta(prev.Meta)}
		keepPersonal(prev, clean, res)
		dropped.add(p.Poster, p.Backdrop, prev.Meta.Banner, prev.Meta.Logo)
		// mtime 0: the next scan takes the file for a changed one and reads it from the start.
		if _, err := s.DB.ExecContext(ctx, `UPDATE items SET title=?, sort_title=?, year=?, overview=?, tagline=?, genres=?, rating=?, content_rating=?,
			studio=?, poster=?, backdrop=?, provider=?, provider_id=?, aired=?, season=?, episode=?, episode_end=?, intro_start=?, intro_end=?, min_age=?,
			mtime=0, updated_at=? WHERE id=?`,
			it.Title, it.SortTitle, it.Year, it.Overview, it.Tagline, joinList(it.Genres), it.Rating, it.ContentRating,
			it.Studio, it.Poster, it.Backdrop, it.Provider, it.ProviderID, it.Aired, it.Season, it.Episode, it.EpisodeEnd, it.IntroStart, it.IntroEnd, AgeFor(it.ContentRating),
			now, id); err != nil {
			return err
		}
		if err := s.saveTitleMeta(ctx, TargetItem, id, clean.Meta); err != nil {
			return err
		}
		if err := s.dropBrands(ctx, clean); err != nil {
			return err
		}
		res.Titles++
	}
	for _, id := range showIDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		prev, err := s.LoadTitle(ctx, TargetShow, id)
		if err != nil {
			continue
		}
		p := prev.Show
		if p.Provider == "tmdb" && p.ProviderID != "" {
			tmdbIDs[p.ProviderID] = true
		}
		sh := *p
		sh.Overview, sh.Genres, sh.Rating, sh.ContentRating, sh.Network, sh.Status = "", nil, nil, "", "", ""
		sh.Poster, sh.Backdrop, sh.Provider, sh.ProviderID = "", "", "", ""
		sh.Title, sh.Year = s.showNameFromFiles(ctx, lib, p)
		sh.SortTitle = SortTitle(sh.Title)
		clean := &Title{Target: TargetShow, ID: id, Show: &sh, Meta: cleanMeta(prev.Meta)}
		keepPersonal(prev, clean, res)
		dropped.add(p.Poster, p.Backdrop, prev.Meta.Banner, prev.Meta.Logo)
		if _, err := s.DB.ExecContext(ctx, `UPDATE shows SET title=?, sort_title=?, year=?, overview=?, genres=?, rating=?, content_rating=?,
			network=?, status=?, poster=?, backdrop=?, provider=?, provider_id=?, intro_start=?, intro_end=?, min_age=?, air_days=NULL, air_time=NULL, updated_at=? WHERE id=?`,
			sh.Title, sh.SortTitle, sh.Year, sh.Overview, joinList(sh.Genres), sh.Rating, sh.ContentRating,
			sh.Network, sh.Status, sh.Poster, sh.Backdrop, sh.Provider, sh.ProviderID, sh.IntroStart, sh.IntroEnd, AgeFor(sh.ContentRating), now, id); err != nil {
			return err
		}
		if err := s.saveTitleMeta(ctx, TargetShow, id, clean.Meta); err != nil {
			return err
		}
		if err := s.dropBrands(ctx, clean); err != nil {
			return err
		}
		res.Shows++
	}
	// Season artwork: the next scan finds the pictures in the folders again, and asks the
	// show's service for the rest.
	if rows, err := s.DB.QueryContext(ctx, `SELECT IFNULL(poster, '') FROM seasons WHERE show_id IN (SELECT id FROM shows WHERE library_id = ?)`, lib.ID); err == nil {
		for rows.Next() {
			var p string
			if rows.Scan(&p) == nil {
				dropped.add(p)
			}
		}
		rows.Close()
	}
	people, err := s.personPhotos(ctx)
	if err != nil {
		return err
	}
	items := `(SELECT id FROM items WHERE library_id = ? AND kind NOT IN ('` + CDKind + `', '` + playlistKind + `') AND cd_disc IS NULL)`
	shows := `(SELECT id FROM shows WHERE library_id = ?)`
	for _, q := range []string{
		`UPDATE seasons SET poster = NULL, title = NULL, overview = NULL WHERE show_id IN ` + shows,
		`DELETE FROM credits WHERE target = 'item' AND target_id IN ` + items,
		`DELETE FROM credits WHERE target = 'show' AND target_id IN ` + shows,
		`DELETE FROM credits_read WHERE target = 'item' AND target_id IN ` + items,
		`DELETE FROM credits_read WHERE target = 'show' AND target_id IN ` + shows,
		`DELETE FROM meta_history WHERE target = 'item' AND target_id IN ` + items,
		`DELETE FROM meta_history WHERE target = 'show' AND target_id IN ` + shows,
		`DELETE FROM listings WHERE target = 'item' AND target_id IN ` + items,
		`DELETE FROM listings WHERE target = 'show' AND target_id IN ` + shows,
		`DELETE FROM listing_titles WHERE target = 'item' AND target_id IN ` + items,
		`DELETE FROM listing_titles WHERE target = 'show' AND target_id IN ` + shows,
	} {
		if _, err := s.DB.ExecContext(ctx, q, lib.ID); err != nil {
			return err
		}
	}
	for id := range tmdbIDs {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM watch_offers_cache WHERE tmdb_id = ?`, id); err != nil {
			return err
		}
	}
	if err := s.PrunePeople(ctx); err != nil {
		return err
	}
	left, err := s.personPhotos(ctx)
	if err != nil {
		return err
	}
	for u := range people {
		if !left[u] {
			dropped[u] = true
		}
	}
	if opt.ImagesDir != "" {
		s.dropCachedRemote(ctx, opt.ImagesDir, dropped, res)
	}
	return nil
}

// showNameFromFiles is the name a new scan gives a show: from one of its episodes' paths,
// else from its folder.
func (s *Store) showNameFromFiles(ctx context.Context, lib *Library, sh *Show) (string, *int) {
	var p string
	if s.DB.QueryRowContext(ctx, `SELECT path FROM items WHERE show_id = ? ORDER BY id LIMIT 1`, sh.ID).Scan(&p) == nil {
		if ref := episodeRef(lib, p); filepath.Clean(ref.ShowDir) == filepath.Clean(sh.Path) && ref.Show != "" {
			return ref.Show, ref.ShowYear
		}
	}
	name, year := ParseName(strings.TrimSpace(bracketTagRe.ReplaceAllString(stripIDTags(filepath.Base(sh.Path)), " ")))
	if name == "" {
		name = filepath.Base(sh.Path)
	}
	return name, year
}

// personPhotos are the addresses of people's photos from the internet.
func (s *Store) personPhotos(ctx context.Context) (map[string]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT photo FROM people WHERE photo LIKE 'http%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out[p] = true
	}
	return out, rows.Err()
}

// dropCachedRemote removes cached copies of pictures from the internet, with their smaller
// copies, unless something still shows them.
func (s *Store) dropCachedRemote(ctx context.Context, imagesDir string, urls remoteRefs, res *ForgetResult) {
	if len(urls) == 0 {
		return
	}
	inUse := map[string]bool{}
	for _, q := range []string{`SELECT IFNULL(poster, '') FROM items WHERE poster LIKE 'http%'`, `SELECT IFNULL(backdrop, '') FROM items WHERE backdrop LIKE 'http%'`,
		`SELECT IFNULL(poster, '') FROM shows WHERE poster LIKE 'http%'`, `SELECT IFNULL(backdrop, '') FROM shows WHERE backdrop LIKE 'http%'`,
		`SELECT IFNULL(poster, '') FROM seasons WHERE poster LIKE 'http%'`, `SELECT banner FROM title_meta WHERE banner LIKE 'http%'`,
		`SELECT logo FROM title_meta WHERE logo LIKE 'http%'`, `SELECT photo FROM people WHERE photo LIKE 'http%'`} {
		rows, err := s.DB.QueryContext(ctx, q)
		if err != nil {
			return // unsure what's still used: keep the cache as it is
		}
		for rows.Next() {
			var v string
			if rows.Scan(&v) == nil {
				inUse[v] = true
			}
		}
		rows.Close()
	}
	for u := range urls {
		if inUse[u] {
			continue
		}
		p := RemoteArtPath(imagesDir, u)
		st, err := os.Lstat(p)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		for _, c := range smallCopies(imagesDir, "cache:"+filepath.Base(p), st.ModTime()) {
			if os.Remove(c) == nil {
				res.Cached++
			}
		}
		if os.Remove(p) == nil {
			res.Cached++
		}
	}
}

// ForgetEverywhereExtras is what forgetting every library's details adds: where-to-watch
// answers kept by TMDB id, and the logo service's verdicts that nothing was found (looked
// for again). Downloaded logos themselves are the Logos service's to drop.
func (s *Store) ForgetEverywhereExtras(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM watch_offers_cache`); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE logos SET status = ?, tried_at = 0 WHERE status = ?`, LogoPending, LogoNone)
	return err
}
