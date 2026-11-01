package diagnose

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

func metadataChecks() []Check {
	return []Check{
		{ID: "library-folders", Area: AreaMetadata, Title: "Can every library folder be reached?", Run: checkLibraryRoots},
		{ID: "missing-files", Area: AreaMetadata, Title: "Are there titles whose files have gone?", Run: checkMissingFiles, Fix: fixMissingFiles},
		{ID: "artwork-refs", Area: AreaMetadata, Title: "Does every title's artwork still exist?", Run: checkArtRefs, Fix: fixArtRefs},
		{ID: "artwork-orphans", Area: AreaMetadata, Title: "Is there artwork nothing uses?", Run: checkArtOrphans, Fix: fixArtOrphans},
		{ID: "matches", Area: AreaMetadata, Title: "Are the matches to online sources well formed?", Run: checkMatches, Fix: fixMatches},
		{ID: "duplicates", Area: AreaMetadata, Title: "Is anything listed twice?", Run: checkDuplicates, Fix: fixDuplicates},
		{ID: "nfo-files", Area: AreaMetadata, Title: "Can the NFO and metadata files beside your media be read?", Run: checkSidecars},
	}
}

// metaDB is the database for a metadata check, or what the check should say instead. There
// being no database is no finding, and a damaged one is the data check's to report: reading
// through it could say anything, so nothing is concluded from it.
func (e *DoctorEnv) metaDB(what string) (*db.DB, []Issue) {
	d, err := e.readDB()
	if errors.Is(err, db.ErrNoDatabase) {
		return nil, nil
	}
	if err != nil {
		return nil, notChecked("meta:"+what, what, "The database can't be opened, so this waits until that's fixed.")
	}
	if e.damaged != "" {
		return nil, notChecked("meta:"+what, what, "The database is damaged, so this waits until that's fixed.")
	}
	return d, nil
}

// --------------------------------------------------------------- library folders

type libRoot struct {
	lib  int64
	name string
	path string
}

func libraryRoots(ctx context.Context, d *db.DB) ([]libRoot, error) {
	rows, err := d.QueryContext(ctx, `SELECT p.library_id, l.name, p.path FROM library_paths p JOIN libraries l ON l.id = p.library_id ORDER BY p.library_id, p.path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []libRoot
	for rows.Next() {
		var r libRoot
		if err := rows.Scan(&r.lib, &r.name, &r.path); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// rootAway: a folder that can't be reached, or is empty. An unmounted drive often looks
// like an empty folder, so scans treat it the same way, and so does this.
func rootAway(p string) (away bool, why string) {
	entries, err := os.ReadDir(p)
	if err != nil {
		return true, "can't be reached"
	}
	if len(entries) == 0 {
		return true, "is empty"
	}
	return false, ""
}

func checkLibraryRoots(ctx context.Context, e *DoctorEnv) []Issue {
	d, skip := e.metaDB("library folders")
	if d == nil {
		return skip
	}
	roots, err := libraryRoots(ctx, d)
	if err != nil {
		return notChecked("library-folders:failed", "library folders", "The list couldn't be read: "+err.Error()+".")
	}
	var out []Issue
	for _, r := range roots {
		away, why := rootAway(r.path)
		if !away {
			continue
		}
		out = append(out, Issue{Key: "library-folders:" + hashOf(r.path), Severity: SevWarn, Title: "A folder of “" + r.name + "” " + why, Safety: SafetyManual, Input: "remap", Details: []string{r.path},
			What: "The folder " + r.path + " " + why + ". Its titles stay listed and nothing about them was changed, in case the drive comes back.",
			Fix:  "If a drive is unplugged, plug it back in. If the folder moved, tell this program where it went.",
			Steps: []string{
				"If it's on a drive that's away, connect the drive. Nothing else is needed.",
				"If it moved: wms doctor remap --from \"" + r.path + "\" --to \"<the new folder>\" --fix, or use Moved a drive? in Settings, Repair.",
			}})
	}
	return out
}

func hashOf(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:6])
}

// --------------------------------------------------------------- missing files

const (
	missingStatLimit = 200000
	missingTimeLimit = 2 * time.Minute
)

// itemFile is the file an item's path names, if it names one: podcast episodes live at web
// addresses, stations and game PCs have no file, and a CD's tracks share their image.
func itemFile(path, kind string) (string, bool) {
	if kind == "radio" || strings.Contains(path, "://") {
		return "", false
	}
	if i := strings.Index(path, "#track="); i >= 0 {
		path = path[:i]
	}
	if !filepath.IsAbs(path) {
		return "", false
	}
	return path, true
}

func checkMissingFiles(ctx context.Context, e *DoctorEnv) []Issue {
	d, skip := e.metaDB("files of titles")
	if d == nil {
		return skip
	}
	roots, err := libraryRoots(ctx, d)
	if err != nil {
		return notChecked("missing-files:failed", "files of titles", "The list couldn't be read: "+err.Error()+".")
	}
	here := map[int64][]string{}
	for _, r := range roots {
		if away, _ := rootAway(r.path); !away {
			here[r.lib] = append(here[r.lib], r.path)
		}
	}
	rows, err := d.QueryContext(ctx, `SELECT id, library_id, kind, path FROM items WHERE missing = 0`)
	if err != nil {
		return notChecked("missing-files:failed", "files of titles", "The list couldn't be read: "+err.Error()+".")
	}
	defer rows.Close()
	var ids []int64
	var names []string
	seen, deadline := 0, time.Now().Add(missingTimeLimit)
	stopped := false
	for rows.Next() {
		var id, lib int64
		var kind, p string
		if err := rows.Scan(&id, &lib, &kind, &p); err != nil {
			return notChecked("missing-files:failed", "files of titles", "The list couldn't be read: "+err.Error()+".")
		}
		file, ok := itemFile(p, kind)
		if !ok || !underRoot(file, here[lib]) {
			continue // a folder that's away decides nothing about what's in it
		}
		if seen++; seen > missingStatLimit || time.Now().After(deadline) || ctx.Err() != nil {
			stopped = true
			break
		}
		if _, err := os.Stat(file); errors.Is(err, fs.ErrNotExist) {
			ids = append(ids, id)
			names = append(names, filepath.Base(file))
		}
	}
	if err := rows.Err(); err != nil {
		return notChecked("missing-files:failed", "files of titles", "The list couldn't be read: "+err.Error()+".")
	}
	if len(ids) == 0 {
		if stopped {
			return notChecked("missing-files:partial", "every title's file", fmt.Sprintf("It looked at %d files and stopped there, to keep from running for too long. Nothing it looked at has gone.", seen))
		}
		return nil
	}
	return []Issue{{Key: "missing-files", Severity: SevWarn, Title: "Some titles' files have gone", Safety: SafetySafe, Count: len(ids), Names: names, data: ids,
		What: countOf(len(ids), "title has a file", "titles have files") + " that aren't in a folder that is there.",
		Fix:  "Hides them, as a scan would. Nothing is deleted: they come back with everything known about them if the files return."}}
}

func underRoot(p string, roots []string) bool {
	for _, r := range roots {
		if rel, err := filepath.Rel(r, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func fixMissingFiles(ctx context.Context, e *DoctorEnv, i *Issue, _ *FixContext) (string, error) {
	ids, ok := i.data.([]int64)
	if !ok {
		return "", errors.New("run the check again first")
	}
	err := withWrite(ctx, e, func(d *db.DB) error {
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, `UPDATE items SET missing = 1 WHERE id = ?`, id); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
	if err != nil {
		return "", err
	}
	return countOf(len(ids), "title was", "titles were") + " hidden until the files come back.", nil
}

// --------------------------------------------------------------- artwork that has gone

const cachePrefix = "cache:"

type artRef struct {
	table, column string
	where         string // the row, by its key
	args          []any
	name          string
}

func artRefs(ctx context.Context, d *db.DB) ([]artRef, error) {
	var out []artRef
	read := func(q, table, column, where string) error {
		rows, err := d.QueryContext(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a, b any
			var v string
			if err := rows.Scan(&a, &b, &v); err != nil {
				return err
			}
			r := artRef{table: table, column: column, where: where, args: []any{a}, name: strings.TrimPrefix(v, cachePrefix)}
			if b != nil {
				r.args = append(r.args, b)
			}
			out = append(out, r)
		}
		return rows.Err()
	}
	for _, s := range []struct{ table, col string }{{"items", "poster"}, {"items", "backdrop"}, {"shows", "poster"}, {"shows", "backdrop"}} {
		if err := read(`SELECT id, NULL, `+s.col+` FROM `+s.table+` WHERE `+s.col+` LIKE 'cache:%'`, s.table, s.col, `id = ?`); err != nil {
			return nil, err
		}
	}
	if err := read(`SELECT show_id, number, poster FROM seasons WHERE poster LIKE 'cache:%'`, "seasons", "poster", `show_id = ? AND number = ?`); err != nil {
		return nil, err
	}
	return out, nil
}

func checkArtRefs(ctx context.Context, e *DoctorEnv) []Issue {
	d, skip := e.metaDB("artwork of titles")
	if d == nil {
		return skip
	}
	refs, err := artRefs(ctx, d)
	if err != nil {
		return notChecked("artwork-refs:failed", "artwork of titles", "The list couldn't be read: "+err.Error()+".")
	}
	var gone []artRef
	var names []string
	for _, r := range refs {
		if r.name == "" || r.name != filepath.Base(r.name) {
			continue
		}
		if _, err := os.Stat(filepath.Join(e.Images, r.name)); errors.Is(err, fs.ErrNotExist) {
			gone = append(gone, r)
			names = append(names, r.name)
		}
	}
	if len(gone) == 0 {
		return nil
	}
	// Everything gone at once is a folder that's away, or the wrong one, not lost pictures.
	if len(refs) >= 20 && len(gone) == len(refs) {
		return []Issue{{Key: "artwork-refs:folder", Severity: SevProblem, Title: "None of your artwork can be found", Safety: SafetyManual, Count: len(gone),
			What: "All " + fmt.Sprint(len(refs)) + " pictures the database knows are missing from " + e.Images + ", so the folder is probably away or the setting is wrong. Nothing was changed.",
			Fix:  "Check the artwork folder.",
			Steps: []string{
				"Check that the drive holding " + e.Images + " is there.",
				"Run wms config path to see which folder is in use, and check metadataDir in wms.json.",
				"Run this check again. If the pictures really are gone, a refresh of each library fetches them again.",
			}}}
	}
	return []Issue{{Key: "artwork-refs", Severity: SevWarn, Title: "Some titles point at artwork that has gone", Safety: SafetySafe, Count: len(gone), Names: names, data: gone,
		What: countOf(len(gone), "picture is", "pictures are") + " missing from the artwork folder, so those titles show a blank.",
		Fix:  "Clears the broken reference, so the next refresh or scan fetches a new picture. Nothing that exists is touched."}}
}

func fixArtRefs(ctx context.Context, e *DoctorEnv, i *Issue, _ *FixContext) (string, error) {
	gone, ok := i.data.([]artRef)
	if !ok {
		return "", errors.New("run the check again first")
	}
	err := withWrite(ctx, e, func(d *db.DB) error {
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, r := range gone {
			if _, err := tx.ExecContext(ctx, `UPDATE `+db.QuoteIdent(r.table)+` SET `+db.QuoteIdent(r.column)+` = '' WHERE `+r.where, r.args...); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
	if err != nil {
		return "", err
	}
	return countOf(len(gone), "broken reference was", "broken references were") + " cleared. A refresh fetches new pictures.", nil
}

// --------------------------------------------------------------- artwork nothing uses

// Only pictures named for what they are, that nothing but a "cache:" reference can reach:
// an upload, a title's cover, and a frame from its video. These are the rest, and are left
// alone: r- files are kept by web address, and the thumbnails are small copies named by what
// they're copied from, made again as needed. logos/ and any other folder aren't looked in.
var orphanable = regexp.MustCompile(`^(up|agent|a|b|cd|thumb|game)-[0-9a-zA-Z_-]+\.(jpg|jpeg|png|webp|gif)$`)

const orphanAge = 24 * time.Hour

var cacheRefRe = regexp.MustCompile(`cache:[A-Za-z0-9._-]+`)

// usedArtwork finds every "cache:" reference in every text column of every table: edits
// awaiting undo and things a later version adds count too. On any error it answers an
// error: a list that may be short would make used pictures look unused.
func usedArtwork(ctx context.Context, d *db.DB) (map[string]bool, error) {
	used := map[string]bool{}
	tables, err := d.Tables(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range tables {
		cols, err := d.Columns(ctx, t)
		if err != nil {
			return nil, err
		}
		for _, c := range cols {
			ty := strings.ToUpper(c.Type)
			if !(ty == "" || strings.Contains(ty, "TEXT") || strings.Contains(ty, "CHAR") || strings.Contains(ty, "CLOB")) {
				continue
			}
			rows, err := d.QueryContext(ctx, `SELECT `+db.QuoteIdent(c.Name)+` FROM `+db.QuoteIdent(t)+` WHERE `+db.QuoteIdent(c.Name)+` LIKE '%cache:%'`)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var v string
				if err := rows.Scan(&v); err != nil {
					rows.Close()
					return nil, err
				}
				for _, m := range cacheRefRe.FindAllString(v, -1) {
					used[strings.TrimPrefix(m, cachePrefix)] = true
				}
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, err
			}
		}
	}
	return used, nil
}

func checkArtOrphans(ctx context.Context, e *DoctorEnv) []Issue {
	d, skip := e.metaDB("unused artwork")
	if d == nil {
		return skip
	}
	entries, err := os.ReadDir(e.Images)
	if err != nil {
		return nil
	}
	used, err := usedArtwork(ctx, d)
	if err != nil {
		return notChecked("artwork-orphans:failed", "unused artwork", "The list couldn't be read: "+err.Error()+".")
	}
	now := e.now()
	var files, names []string
	var size int64
	for _, ent := range entries {
		if ent.IsDir() || !ent.Type().IsRegular() || !orphanable.MatchString(ent.Name()) || used[ent.Name()] {
			continue
		}
		info, err := ent.Info()
		if err != nil || !olderThan(info, now, orphanAge) {
			continue // it may be new, with its row on the way
		}
		files = append(files, filepath.Join(e.Images, ent.Name()))
		names = append(names, ent.Name())
		size += info.Size()
	}
	if len(files) == 0 {
		return nil
	}
	return []Issue{{Key: "artwork-orphans", Severity: SevInfo, Title: "Artwork nothing uses", Safety: SafetySafe, Count: len(files), Names: names, data: files,
		What: fmt.Sprintf("%s in the artwork folder, %s, belonging to no title.", countOf(len(files), "picture", "pictures"), sizeWords(size)),
		Fix:  "Moves them to the safety copy, which is removed in time. You can't tell they've gone."}}
}

func sizeWords(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%d KB", (n+1023)>>10)
}

func fixArtOrphans(_ context.Context, _ *DoctorEnv, i *Issue, f *FixContext) (string, error) {
	files, ok := i.data.([]string)
	if !ok {
		return "", errors.New("run the check again first")
	}
	for _, p := range files {
		if err := f.Remove(p); err != nil {
			return "", err
		}
	}
	return countOf(len(files), "unused picture was", "unused pictures were") + " moved to the safety copy.", nil
}

// --------------------------------------------------------------- broken matches

var matchShapes = map[string]*regexp.Regexp{
	"tmdb":     regexp.MustCompile(`^\d+$`),
	"tmdb-tv":  regexp.MustCompile(`^\d+$`),
	"tvmaze":   regexp.MustCompile(`^\d+$`),
	"anilist":  regexp.MustCompile(`^\d+$`),
	"tvdb":     regexp.MustCompile(`^(?:series-|movie-)?\d+$`),
	"imdb":     regexp.MustCompile(`^tt\d+$`),
	"wikidata": regexp.MustCompile(`^Q\d+$`),
}

type badMatch struct {
	table string
	id    int64
}

func checkMatches(ctx context.Context, e *DoctorEnv) []Issue {
	d, skip := e.metaDB("matches")
	if d == nil {
		return skip
	}
	var bad []badMatch
	var names []string
	for _, table := range []string{"items", "shows"} {
		rows, err := d.QueryContext(ctx, `SELECT id, title, IFNULL(provider, ''), IFNULL(provider_id, '') FROM `+table+` WHERE IFNULL(provider, '') <> '' OR IFNULL(provider_id, '') <> ''`)
		if err != nil {
			return notChecked("matches:failed", "matches", "The list couldn't be read: "+err.Error()+".")
		}
		for rows.Next() {
			var id int64
			var title, p, pid string
			if err := rows.Scan(&id, &title, &p, &pid); err != nil {
				rows.Close()
				return notChecked("matches:failed", "matches", "The list couldn't be read: "+err.Error()+".")
			}
			shape, known := matchShapes[p]
			why := ""
			switch {
			case p == "":
				why = "an id with no source"
			case known && pid == "":
				why = p + " with no id"
			case known && !shape.MatchString(pid):
				why = p + " id “" + pid + "” isn't one"
			default:
				continue
			}
			bad = append(bad, badMatch{table, id})
			names = append(names, title+": "+why)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return notChecked("matches:failed", "matches", "The list couldn't be read: "+err.Error()+".")
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return []Issue{{Key: "matches", Severity: SevWarn, Title: "Some matches are broken", Safety: SafetyCareful, Count: len(bad), Details: names, data: bad,
		What: countOf(len(bad), "title is", "titles are") + " matched to something that can't exist, so looking them up fails.",
		Fix:  "Forgets the broken match, so the next scan or refresh looks the title up afresh. What you typed and locked stays."}}
}

func fixMatches(ctx context.Context, e *DoctorEnv, i *Issue, _ *FixContext) (string, error) {
	bad, ok := i.data.([]badMatch)
	if !ok {
		return "", errors.New("run the check again first")
	}
	err := withWrite(ctx, e, func(d *db.DB) error {
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, b := range bad {
			if _, err := tx.ExecContext(ctx, `UPDATE `+db.QuoteIdent(b.table)+` SET provider = '', provider_id = '' WHERE id = ?`, b.id); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
	if err != nil {
		return "", err
	}
	return countOf(len(bad), "broken match was", "broken matches were") + " forgotten.", nil
}

// --------------------------------------------------------------- listed twice

type dupRow struct {
	table      string
	keep, drop int64
}

// samePath: the same place, however it was written. Case counts only where the system's does.
func samePath(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(p)
	}
	return p
}

func checkDuplicates(ctx context.Context, e *DoctorEnv) []Issue {
	d, skip := e.metaDB("duplicates")
	if d == nil {
		return skip
	}
	failed := func(err error) []Issue {
		return notChecked("duplicates:failed", "duplicates", "The list couldn't be read: "+err.Error()+".")
	}
	var merge []dupRow
	var names []string
	var out []Issue
	// The same place written two ways, in one library.
	for _, table := range []string{"items", "shows"} {
		rows, err := d.QueryContext(ctx, `SELECT id, library_id, path FROM `+table+` WHERE missing = 0 ORDER BY id`)
		if err != nil {
			return failed(err)
		}
		first := map[string]int64{}
		for rows.Next() {
			var id, lib int64
			var p string
			if err := rows.Scan(&id, &lib, &p); err != nil {
				rows.Close()
				return failed(err)
			}
			if strings.Contains(p, "://") {
				continue
			}
			k := fmt.Sprint(lib) + "\x00" + samePath(p)
			if keep, ok := first[k]; ok {
				merge = append(merge, dupRow{table, keep, id})
				names = append(names, filepath.Base(p))
			} else {
				first[k] = id
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return failed(err)
		}
	}
	if len(merge) > 0 {
		out = append(out, Issue{Key: "duplicates:same", Severity: SevWarn, Title: "The same file or show is listed twice", Safety: SafetyCareful, Count: len(merge), Names: names, data: merge,
			What: countOf(len(merge), "entry names", "entries name") + " the same place as another one in the same library.",
			Fix:  "Merges them: watch progress and episodes move to the older entry, and the second one goes."})
	}
	// A file in two libraries: often overlapping folders, and sometimes on purpose.
	rows, err := d.QueryContext(ctx, `SELECT path, COUNT(DISTINCT library_id) FROM items WHERE missing = 0 AND path NOT LIKE '%://%' GROUP BY path HAVING COUNT(DISTINCT library_id) > 1 LIMIT 200`)
	if err != nil {
		return failed(err)
	}
	var overlap []string
	for rows.Next() {
		var p string
		var n int
		if err := rows.Scan(&p, &n); err != nil {
			rows.Close()
			return failed(err)
		}
		overlap = append(overlap, filepath.Base(p))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return failed(err)
	}
	if len(overlap) > 0 {
		out = append(out, Issue{Key: "duplicates:libraries", Severity: SevInfo, Title: "Some files are in two libraries", Safety: SafetyManual, Count: len(overlap), Names: overlap,
			What:  "Two libraries include the same files, which usually means their folders overlap. It can be on purpose.",
			Fix:   "If it isn't on purpose, take the overlapping folder out of one library.",
			Steps: []string{"Open Settings, Libraries, and compare the folders of each library.", "Remove the folder that's inside another, then scan."}})
	}
	// The same show in two folders: they could be parts of one show, or two different shows
	// with a name in common. Only a person can say.
	rows, err = d.QueryContext(ctx, `SELECT library_id, title, IFNULL(year, 0), COUNT(*) FROM shows WHERE missing = 0 GROUP BY library_id, lower(title), IFNULL(year, 0) HAVING COUNT(*) > 1 LIMIT 200`)
	if err != nil {
		return failed(err)
	}
	var shows []string
	for rows.Next() {
		var lib, year, n int64
		var title string
		if err := rows.Scan(&lib, &title, &year, &n); err != nil {
			rows.Close()
			return failed(err)
		}
		shows = append(shows, title)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return failed(err)
	}
	if len(shows) > 0 {
		out = append(out, Issue{Key: "duplicates:shows", Severity: SevWarn, Title: "A show is listed twice under one name", Safety: SafetyManual, Count: len(shows), Details: shows,
			What:  "Two folders in one library have the same name and year. They could be parts of one show or two different shows, so nothing is merged by itself.",
			Fix:   "Put the episodes in one folder and scan, or tell them apart in their folder names.",
			Steps: []string{"Move the episodes into one show folder, or add the year or an id to the folder name, such as My Show (2004) [tvdb-12345].", "Scan the library."}})
	}
	return out
}

func fixDuplicates(ctx context.Context, e *DoctorEnv, i *Issue, _ *FixContext) (string, error) {
	merge, ok := i.data.([]dupRow)
	if !ok {
		return "", errors.New("run the check again first")
	}
	err := withWrite(ctx, e, func(d *db.DB) error {
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, m := range merge {
			if m.table == "shows" {
				if _, err := tx.ExecContext(ctx, `UPDATE items SET show_id = ? WHERE show_id = ?`, m.keep, m.drop); err != nil {
					return err
				}
			} else if _, err := tx.ExecContext(ctx, `UPDATE OR IGNORE progress SET item_id = ? WHERE item_id = ?`, m.keep, m.drop); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+db.QuoteIdent(m.table)+` WHERE id = ?`, m.drop); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
	if err != nil {
		return "", err
	}
	return countOf(len(merge), "duplicate was", "duplicates were") + " merged.", nil
}

// --------------------------------------------------------------- NFO and sidecar files

const (
	sidecarSizeLimit = 4 << 20
	sidecarTimeLimit = 90 * time.Second
	sidecarFileLimit = 50000
)

// xmlProblem says what's wrong with an NFO, and on which line. It reads a file the way the
// scanner does: anything after the last closing tag is ignored, because NFOs often end with
// a web address. A file that isn't XML at all and holds only an address is legal too.
func xmlProblem(b []byte) string {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 {
		return "it's empty"
	}
	if trimmed[0] != '<' {
		if bytes.Contains(trimmed, []byte("://")) && !bytes.Contains(trimmed, []byte("\n")) {
			return "" // a bare link, a format of its own
		}
		return "line 1: it doesn't start with an XML tag"
	}
	if i := bytes.LastIndex(b, []byte("</")); i >= 0 {
		if j := bytes.IndexByte(b[i:], '>'); j >= 0 {
			b = b[:i+j+1]
		}
	}
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = true
	for {
		_, err := dec.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var se *xml.SyntaxError
			if errors.As(err, &se) {
				return fmt.Sprintf("line %d: %s", se.Line, se.Msg)
			}
			return err.Error()
		}
	}
	dec = xml.NewDecoder(bytes.NewReader(b))
	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return ""
		}
		if err != nil {
			var se *xml.SyntaxError
			if errors.As(err, &se) {
				return fmt.Sprintf("line %d: %s", se.Line, se.Msg)
			}
			return err.Error()
		}
	}
}

// jsonProblem says what's wrong with a metadata file, and on which line.
func jsonProblem(b []byte) string {
	var v map[string]any
	err := json.Unmarshal(b, &v)
	if err == nil {
		return ""
	}
	var se *json.SyntaxError
	var te *json.UnmarshalTypeError
	switch {
	case errors.As(err, &se):
		return fmt.Sprintf("line %d: %s", 1+bytes.Count(b[:min(int(se.Offset), len(b))], []byte("\n")), se.Error())
	case errors.As(err, &te):
		return fmt.Sprintf("line %d: the file has to be one { } object", 1+bytes.Count(b[:min(int(te.Offset), len(b))], []byte("\n")))
	}
	return err.Error()
}

func checkSidecars(ctx context.Context, e *DoctorEnv) []Issue {
	d, skip := e.metaDB("NFO files")
	if d == nil {
		return skip
	}
	roots, err := libraryRoots(ctx, d)
	if err != nil {
		return notChecked("nfo-files:failed", "NFO files", "The list couldn't be read: "+err.Error()+".")
	}
	here := map[int64][]string{}
	for _, r := range roots {
		if away, _ := rootAway(r.path); !away {
			here[r.lib] = append(here[r.lib], r.path)
		}
	}
	rows, err := d.QueryContext(ctx, `SELECT library_id, kind, path FROM items WHERE missing = 0`)
	if err != nil {
		return notChecked("nfo-files:failed", "NFO files", "The list couldn't be read: "+err.Error()+".")
	}
	defer rows.Close()
	seen := map[string]bool{}
	var bad []string
	deadline := time.Now().Add(sidecarTimeLimit)
	look := func(p string) {
		if seen[p] {
			return
		}
		seen[p] = true
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			return
		}
		if st.Size() > sidecarSizeLimit {
			bad = append(bad, p+": it's too big to be a metadata file, and would be ignored")
			return
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return
		}
		why := ""
		if strings.HasSuffix(p, ".json") {
			why = jsonProblem(b)
		} else {
			why = xmlProblem(b)
		}
		if why != "" {
			bad = append(bad, p+": "+why)
		}
	}
	for rows.Next() {
		var lib int64
		var kind, p string
		if err := rows.Scan(&lib, &kind, &p); err != nil {
			return notChecked("nfo-files:failed", "NFO files", "The list couldn't be read: "+err.Error()+".")
		}
		file, ok := itemFile(p, kind)
		if !ok || !underRoot(file, here[lib]) {
			continue
		}
		if len(seen) > sidecarFileLimit || time.Now().After(deadline) || ctx.Err() != nil {
			break
		}
		dir := filepath.Dir(file)
		stem := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		look(filepath.Join(dir, stem+".nfo"))
		look(filepath.Join(dir, stem+".metadata.json"))
		look(file + ".metadata.json")
		look(filepath.Join(dir, "movie.nfo"))
		look(filepath.Join(dir, "tvshow.nfo"))
		look(filepath.Join(filepath.Dir(dir), "tvshow.nfo"))
	}
	if len(bad) == 0 {
		return nil
	}
	return []Issue{{Key: "nfo-files", Severity: SevWarn, Title: "Some NFO or metadata files can't be read", Safety: SafetyManual, Count: len(bad), Details: bad,
		What: countOf(len(bad), "file beside your media has", "files beside your media have") + " a mistake in them, so what's in them is ignored. These are your own files, and nothing here changes them.",
		Fix:  "Open the file and fix the line named.",
		Steps: []string{
			"Open the file in a text editor and go to the line named. Most often it's a stray & (write &amp;) or a tag that was never closed.",
			"Or delete the file, and with saving beside media on, the server writes a new one at the next scan.",
			"Then scan the library. The metadata guide in docs/METADATA.md has examples.",
		}}}
}

// --------------------------------------------------------------- moving a drive

// RemapPlan says what moving a folder would do.
type RemapPlan struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Roots     int      `json:"roots"`    // library folders that change
	Rows      int      `json:"rows"`     // every row that would change
	Checked   int      `json:"checked"`  // files looked for
	Found     int      `json:"found"`    // ...and found in the new place
	Sample    []string `json:"sample"`   // a few of the new paths
	NotFound  []string `json:"notFound"` // a few that weren't there
	CanApply  bool     `json:"canApply"`
	Why       string   `json:"why,omitempty"`
	Applied   bool     `json:"applied,omitempty"`
	Copy      string   `json:"copy,omitempty"`
}

const (
	remapSample   = 2000
	remapMinShare = 60 // percent of files that must be in the new place
)

// remapped is v moved from one root to another, if it's under the first.
func remapped(v, from, to string) (string, bool) {
	from, to = strings.TrimRight(from, `/\`), strings.TrimRight(to, `/\`)
	if from == "" || len(v) < len(from) || !strings.EqualFold(v[:len(from)], from) {
		return "", false
	}
	if runtime.GOOS != "windows" && v[:len(from)] != from {
		return "", false
	}
	rest := v[len(from):]
	if rest != "" && rest[0] != '/' && rest[0] != '\\' {
		return "", false
	}
	// A drive can move between systems: the rest of the path takes the new one's separators.
	if strings.Contains(to, "/") && !strings.Contains(to, `\`) {
		rest = strings.ReplaceAll(rest, `\`, "/")
	} else if strings.Contains(to, `\`) {
		rest = strings.ReplaceAll(rest, "/", `\`)
	}
	return to + rest, true
}

// PlanRemap works out what moving from to to would change, and checks the files are
// really there. It changes nothing.
func PlanRemap(ctx context.Context, e *DoctorEnv, from, to string) (*RemapPlan, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" || to == "" {
		return nil, errors.New("say both where the folder was, and where it is now")
	}
	p := &RemapPlan{From: from, To: to, Sample: []string{}, NotFound: []string{}}
	if !filepath.IsAbs(to) {
		return nil, errors.New("the new place has to be a full path")
	}
	st, err := os.Stat(to)
	if err != nil || !st.IsDir() {
		p.Why = "The new place isn't a folder this program can see: " + to
		return p, nil
	}
	if underRoot(to, []string{e.Data}) {
		p.Why = "The new place can't be inside the server's data folder."
		return p, nil
	}
	e.closeDB()
	defer e.closeDB()
	d, err := e.readDB()
	if err != nil {
		return nil, fmt.Errorf("the database can't be opened: %w", err)
	}
	roots, err := libraryRoots(ctx, d)
	if err != nil {
		return nil, err
	}
	for _, r := range roots {
		if _, ok := remapped(r.path, from, to); ok {
			p.Roots++
		}
	}
	if p.Roots == 0 {
		p.Why = "No library folder is at " + from + " or inside it, so there's nothing to change."
		return p, nil
	}
	if underRoot(from, []string{to}) || underRoot(to, []string{from}) {
		p.Why = "The old and new places can't be inside each other."
		return p, nil
	}
	rows, err := d.QueryContext(ctx, `SELECT kind, path FROM items WHERE missing = 0`)
	if err != nil {
		return nil, err
	}
	var all []string
	for rows.Next() {
		var kind, path string
		if err := rows.Scan(&kind, &path); err != nil {
			rows.Close()
			return nil, err
		}
		if f, ok := itemFile(path, kind); ok {
			if n, ok := remapped(f, from, to); ok {
				all = append(all, n)
			}
		} else if n, ok := remapped(strings.SplitN(path, "#track=", 2)[0], from, to); ok {
			all = append(all, n)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// A spread through the list, not just the first folder.
	step := 1
	if len(all) > remapSample {
		step = len(all) / remapSample
	}
	for i := 0; i < len(all); i += step {
		p.Checked++
		if _, err := os.Stat(all[i]); err == nil {
			p.Found++
			if len(p.Sample) < 8 {
				p.Sample = append(p.Sample, all[i])
			}
		} else if len(p.NotFound) < 8 {
			p.NotFound = append(p.NotFound, all[i])
		}
	}
	p.Rows, err = countRemapRows(ctx, d, from, to)
	if err != nil {
		return nil, err
	}
	if p.Checked > 0 && p.Found*100 < p.Checked*remapMinShare {
		p.Why = fmt.Sprintf("Only %d of %d files were found in the new place. It doesn't look like the same files, so nothing would be changed.", p.Found, p.Checked)
		return p, nil
	}
	p.CanApply = true
	return p, nil
}

// remapColumns are the text columns that can hold a path. The rest can't, and aren't read.
func remapColumns(ctx context.Context, d *db.DB) (map[string][]string, error) {
	tables, err := d.Tables(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, t := range tables {
		if t == "meta_history" || t == "schema" { // what's in history stays as it was
			continue
		}
		cols, err := d.Columns(ctx, t)
		if err != nil {
			return nil, err
		}
		for _, c := range cols {
			ty := strings.ToUpper(c.Type)
			if ty == "" || strings.Contains(ty, "TEXT") || strings.Contains(ty, "CHAR") {
				out[t] = append(out[t], c.Name)
			}
		}
	}
	return out, nil
}

func countRemapRows(ctx context.Context, d *db.DB, from, to string) (int, error) {
	cols, err := remapColumns(ctx, d)
	if err != nil {
		return 0, err
	}
	n := 0
	for t, list := range cols {
		for _, c := range list {
			var k int
			like := strings.TrimRight(from, `/\`) + "%"
			if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+db.QuoteIdent(t)+` WHERE `+db.QuoteIdent(c)+` LIKE ? ESCAPE '`+"\\"+`'`, likeEscape(like)).Scan(&k); err != nil {
				return 0, err
			}
			n += k
		}
	}
	return n, nil
}

// likeEscape protects the wildcards in a path; the % it ends with stays one.
func likeEscape(s string) string {
	body := strings.TrimSuffix(s, "%")
	body = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(body)
	return body + "%"
}

// ApplyRemap moves a folder: every path under the old place, in every table, in one
// transaction, so it either all happens or none of it does. Only values that start with the
// path are touched. A safety copy comes first.
func ApplyRemap(ctx context.Context, e *DoctorEnv, from, to string) (*RemapPlan, error) {
	p, err := PlanRemap(ctx, e, from, to)
	if err != nil {
		return nil, err
	}
	if !p.CanApply {
		return p, nil
	}
	e.closeDB()
	fix := &FixContext{env: e}
	if err := fix.ensureCopy(ctx); err != nil {
		return nil, fmt.Errorf("no safety copy could be made, so nothing was changed: %w", err)
	}
	p.Copy = fix.copyDir
	err = withWrite(ctx, e, func(d *db.DB) error {
		cols, err := remapColumns(ctx, d)
		if err != nil {
			return err
		}
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		like := likeEscape(strings.TrimRight(p.From, `/\`) + "%")
		for t, list := range cols {
			for _, c := range list {
				qt, qc := db.QuoteIdent(t), db.QuoteIdent(c)
				rows, err := tx.QueryContext(ctx, `SELECT rowid, `+qc+` FROM `+qt+` WHERE `+qc+` LIKE ? ESCAPE '\'`, like)
				if err != nil {
					if strings.Contains(err.Error(), "no such column: rowid") {
						continue // settings-shaped tables with no row numbers hold no paths
					}
					return err
				}
				type change struct {
					id int64
					v  string
				}
				var todo []change
				for rows.Next() {
					var id int64
					var v string
					if err := rows.Scan(&id, &v); err != nil {
						rows.Close()
						return err
					}
					if n, ok := remapped(v, p.From, p.To); ok {
						todo = append(todo, change{id, n})
					}
				}
				err = rows.Err()
				rows.Close()
				if err != nil {
					return err
				}
				for _, ch := range todo {
					if _, err := tx.ExecContext(ctx, `UPDATE `+qt+` SET `+qc+` = ? WHERE rowid = ?`, ch.v, ch.id); err != nil {
						if strings.Contains(err.Error(), "UNIQUE") {
							return errors.New("the new place is already in this library, because it has been added and scanned. Remove that folder from the library first, so what you know about these titles can move to it")
						}
						return err
					}
				}
			}
		}
		return tx.Commit()
	})
	if err != nil {
		return nil, err
	}
	p.Applied = true
	return p, nil
}
