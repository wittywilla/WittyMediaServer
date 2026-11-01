package channels

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// maxPool bounds what one set of rules loads, so a very large library stays
// light on small machines. Rules narrower than this are unaffected.
const maxPool = 50000

const yearExpr = `COALESCE(NULLIF(CAST(substr(IFNULL(i.aired, ''), 1, 4) AS INTEGER), 0), i.year, s.year)`
const genreList = `('|' || IFNULL(i.genres, '') || '|' || IFNULL(s.genres, '') || '|')`
const playable = `i.missing = 0 AND i.kind IN ('video', 'dvd-iso', 'dvd-folder', 'bluray-iso', 'bluray-folder')
	AND COALESCE(json_extract(i.disc_json, '$.scrambled'), 0) = 0`

// notCommercial keeps commercials out of what a channel plays as programmes.
// A rule that names their library or the videos themselves still gets them.
const notCommercial = `i.library_id NOT IN (SELECT id FROM libraries WHERE type = '` + library.CommercialsType + `')`

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func anyArgs[T any](v []T) []any {
	out := make([]any, len(v))
	for i, x := range v {
		out[i] = x
	}
	return out
}

// where turns rules into an SQL condition over items "i" joined to shows "s".
// All values are bound arguments.
func (r *Rules) where() (string, []any) {
	cond := []string{playable}
	var args []any
	add := func(c string, a ...any) { cond = append(cond, c); args = append(args, a...) }
	or := func(parts []string, a []any) {
		if len(parts) > 0 {
			add("("+strings.Join(parts, " OR ")+")", a...)
		}
	}
	// Nothing but scenes from discs: only those discs are in question.
	if r.onlyClips() {
		ids := r.clipItems()
		add("i.id IN ("+placeholders(len(ids))+")", anyArgs(ids)...)
		return strings.Join(cond, " AND "), args
	}
	if len(r.Libraries) > 0 {
		add("i.library_id IN ("+placeholders(len(r.Libraries))+")", anyArgs(r.Libraries)...)
	} else if len(r.Items) == 0 && len(r.Collections) == 0 {
		add(notCommercial)
	}
	if len(r.Kinds) > 0 {
		var parts []string
		for _, k := range r.Kinds {
			switch k {
			case "episodes":
				parts = append(parts, "i.show_id IS NOT NULL")
			case "movies":
				parts = append(parts, "(i.show_id IS NULL AND i.kind = 'video')")
			case "discs":
				parts = append(parts, "i.kind <> 'video'")
			}
		}
		or(parts, nil)
	}
	if len(r.Genres) > 0 {
		var parts []string
		var a []any
		for _, g := range r.Genres {
			parts = append(parts, genreList+" LIKE ?")
			a = append(a, "%|"+g+"|%")
		}
		or(parts, a)
	}
	for _, g := range r.ExcludeGenres {
		add(genreList+" NOT LIKE ?", "%|"+g+"|%")
	}
	if len(r.Networks) > 0 {
		var parts []string
		var a []any
		for _, n := range r.Networks {
			parts = append(parts, "s.network = ? COLLATE NOCASE", "(i.show_id IS NULL AND i.studio = ? COLLATE NOCASE)")
			a = append(a, n, n)
		}
		or(parts, a)
	}
	if len(r.Studios) > 0 {
		var parts []string
		var a []any
		for _, n := range r.Studios {
			// A title can have several: its first is on the item, all of them are here.
			parts = append(parts, "i.studio = ? COLLATE NOCASE", "i.id IN (SELECT item_id FROM title_brands WHERE kind = 'studio' AND name = ? COLLATE NOCASE)")
			a = append(a, n, n)
		}
		or(parts, a)
	}
	if r.YearFrom != 0 {
		add(yearExpr+" >= ?", r.YearFrom)
	}
	if r.YearTo != 0 {
		add(yearExpr+" <= ?", r.YearTo)
	}
	if len(r.Decades) > 0 {
		var parts []string
		var a []any
		for _, d := range r.Decades {
			parts = append(parts, yearExpr+" BETWEEN ? AND ?")
			a = append(a, d, d+9)
		}
		or(parts, a)
	}
	if r.MaxAge != nil {
		const age = "COALESCE(i.min_age, s.min_age)"
		c := age + " <= ?"
		if r.AllowUnrated {
			c = "(" + c + " OR " + age + " IS NULL)"
		}
		add(c, *r.MaxAge)
	}
	if len(r.Ratings) > 0 {
		add("COALESCE(NULLIF(i.content_rating, ''), s.content_rating) COLLATE NOCASE IN ("+placeholders(len(r.Ratings))+")", anyArgs(r.Ratings)...)
	}
	if r.MinScore > 0 {
		add("COALESCE(i.rating, s.rating) >= ?", r.MinScore)
	}
	// Chosen shows and hand-picked titles add up.
	if len(r.Shows) > 0 || len(r.Items) > 0 || len(r.Collections) > 0 {
		var parts []string
		var a []any
		if len(r.Collections) > 0 {
			in := placeholders(len(r.Collections))
			parts = append(parts, "i.id IN (SELECT item_id FROM collection_items WHERE collection_id IN ("+in+"))",
				"i.show_id IN (SELECT show_id FROM collection_shows WHERE collection_id IN ("+in+"))")
			a = append(a, anyArgs(r.Collections)...)
			a = append(a, anyArgs(r.Collections)...)
		}
		if len(r.Shows) > 0 {
			parts = append(parts, "i.show_id IN ("+placeholders(len(r.Shows))+")")
			a = append(a, anyArgs(r.Shows)...)
		}
		if len(r.Items) > 0 {
			parts = append(parts, "i.id IN ("+placeholders(len(r.Items))+")")
			a = append(a, anyArgs(r.Items)...)
		}
		or(parts, a)
	}
	if len(r.ExcludeShows) > 0 {
		add("IFNULL(i.show_id, 0) NOT IN ("+placeholders(len(r.ExcludeShows))+")", anyArgs(r.ExcludeShows)...)
	}
	if len(r.ExcludeItems) > 0 {
		add("i.id NOT IN ("+placeholders(len(r.ExcludeItems))+")", anyArgs(r.ExcludeItems)...)
	}
	if r.SeasonFrom != 0 {
		add("(i.show_id IS NULL OR i.season >= ?)", r.SeasonFrom)
	}
	if r.SeasonTo != 0 {
		add("(i.show_id IS NULL OR i.season <= ?)", r.SeasonTo)
	}
	if !r.IncludeSpecials && r.SeasonFrom == 0 {
		add("(i.show_id IS NULL OR IFNULL(i.season, 1) <> 0)")
	}
	switch r.Watched {
	case "watched":
		add("EXISTS (SELECT 1 FROM progress p WHERE p.item_id = i.id AND p.user_id = ? AND p.watched = 1)", r.WatchedBy)
	case "unwatched":
		add("NOT EXISTS (SELECT 1 FROM progress p WHERE p.item_id = i.id AND p.user_id = ? AND p.watched = 1)", r.WatchedBy)
	}
	if r.MinMinutes > 0 {
		add("i.runtime_sec >= ?", r.MinMinutes*60)
	}
	if r.MaxMinutes > 0 {
		add("i.runtime_sec <= ?", r.MaxMinutes*60)
	}
	if s := strings.TrimSpace(r.Search); s != "" {
		add("(i.title LIKE ? OR s.title LIKE ?)", "%"+s+"%", "%"+s+"%")
	}
	return strings.Join(cond, " AND "), args
}

// LoadPool reads the titles rules choose, in a stable order: shows by title,
// season and episode, with films by title after them.
func (s *Store) LoadPool(ctx context.Context, r Rules, minSec int64) ([]Entry, error) {
	clips, err := s.loadClips(ctx, r.Clips)
	if err != nil {
		return nil, err
	}
	if r.onlyClips() {
		return clips, nil
	}
	w, args := r.where()
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id, IFNULL(i.show_id, 0), i.runtime_sec, i.title, i.kind, i.disc_json, IFNULL(s.title, ''),
		i.season, i.episode, i.episode_end, IFNULL(i.aired, ''), `+yearExpr+`, COALESCE(NULLIF(i.content_rating, ''), s.content_rating, ''),
		COALESCE(NULLIF(i.genres, ''), s.genres, '')
		FROM items i LEFT JOIN shows s ON s.id = i.show_id WHERE `+w+`
		ORDER BY i.show_id IS NULL, s.sort_title COLLATE NOCASE, s.id, IFNULL(i.season, 0), IFNULL(i.episode, 0), i.sort_title COLLATE NOCASE, i.id
		LIMIT `+strconv.Itoa(maxPool), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		var runtime sql.NullFloat64
		var kind, title, showTitle, genres string
		var disc sql.NullString
		var season, episode, episodeEnd, year sql.NullInt64
		if err := rows.Scan(&e.ID, &e.Group, &runtime, &title, &kind, &disc, &showTitle, &season, &episode, &episodeEnd, &e.Aired, &year, &e.Rating, &genres); err != nil {
			return nil, err
		}
		dur := runtime.Float64
		if kind != "video" && disc.Valid {
			dur = discLength(disc.String, dur)
		}
		e.Dur = int64(dur + 0.999)
		if e.Dur < minSec || e.Dur <= 0 {
			continue
		}
		e.ShowID = e.Group
		e.Year = int(year.Int64)
		if g, _, _ := strings.Cut(genres, "|"); g != "" {
			e.Genre = g
		}
		e.Item = e.ID
		e.Title = title
		if showTitle != "" {
			e.Title = showTitle
			e.Sub = episodeSub(title, season, episode, episodeEnd)
		}
		if season.Valid {
			v := int(season.Int64)
			e.Season = &v
		}
		if episode.Valid {
			v := int(episode.Int64)
			e.Episode = &v
		}
		out = append(out, e)
	}
	return append(out, clips...), rows.Err()
}

// clipID gives a clip an id in the pool of its own, which real items (above
// zero) can never have, and which stays the same while the clip does.
func clipID(c DiscClip) int64 {
	h := fnv.New32a()
	fmt.Fprintf(h, "%d|%d|%.3f|%.3f", c.Item, c.Title, c.Start, c.End)
	return -int64(h.Sum32()) - 1
}

// loadClips turns scenes into titles of their own. A disc that has gone, has
// become encrypted or no longer has that title simply plays nothing.
func (s *Store) loadClips(ctx context.Context, clips []DiscClip) ([]Entry, error) {
	if len(clips) == 0 {
		return nil, nil
	}
	discs, err := s.clipDiscs(ctx, clips)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(clips))
	seen := map[int64]bool{}
	for _, c := range clips {
		d, ok := discs[c.Item]
		if !ok {
			continue
		}
		start, end, ok := d.span(c)
		if !ok {
			continue
		}
		id := clipID(c)
		if seen[id] {
			continue
		}
		seen[id] = true
		clip := &ClipRef{Title: d.titleNumber(c.Title), Start: start, End: end, Label: c.Label}
		e := Entry{ID: id, Item: c.Item, Dur: int64(end - start + 0.999), Title: d.title, Sub: clipSub(c), Year: d.year, Rating: d.rating, Genre: d.genre, Clip: clip}
		out = append(out, e)
	}
	return out, nil
}

// clipSub is how a clip is named under its disc's title.
func clipSub(c DiscClip) string {
	if c.Label != "" {
		return c.Label
	}
	switch {
	case c.FromChapter > 0 && c.ToChapter > c.FromChapter:
		return "Chapters " + strconv.Itoa(c.FromChapter) + " to " + strconv.Itoa(c.ToChapter)
	case c.FromChapter > 0:
		return "Chapter " + strconv.Itoa(c.FromChapter)
	case c.Title == 0 && c.Start == 0 && c.End == 0:
		return ""
	}
	return "A scene"
}

// clipDisc is what the schedule needs to know about a disc.
type clipDisc struct {
	info   library.DiscInfo
	title  string
	year   int
	rating string
	genre  string
}

func (d *clipDisc) titleNumber(n int) int {
	if n == 0 {
		return d.info.MainTitle
	}
	return n
}

// span is where a clip starts and ends in its title, held to what the disc has.
func (d *clipDisc) span(c DiscClip) (start, end float64, ok bool) {
	n := d.titleNumber(c.Title)
	for _, t := range d.info.Titles {
		if t.Number != n || t.DurationSec <= 0 {
			continue
		}
		start, end = c.Start, c.End
		if end == 0 || end > t.DurationSec {
			end = t.DurationSec
		}
		return start, end, end-start >= minClipSec
	}
	return 0, 0, false
}

// clipDiscs reads the discs of these clips. Only what may be played comes back,
// so a scrambled or missing disc is simply not there.
func (s *Store) clipDiscs(ctx context.Context, clips []DiscClip) (map[int64]*clipDisc, error) {
	seen := map[int64]bool{}
	ids := []int64{}
	for _, c := range clips {
		if !seen[c.Item] {
			seen[c.Item] = true
			ids = append(ids, c.Item)
		}
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id, i.title, i.disc_json, IFNULL(i.year, 0), IFNULL(i.content_rating, ''), IFNULL(i.genres, '')
		FROM items i LEFT JOIN shows s ON s.id = i.show_id WHERE `+playable+` AND i.kind <> 'video' AND i.id IN (`+placeholders(len(ids))+`)`, anyArgs(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]*clipDisc{}
	for rows.Next() {
		var id int64
		var raw sql.NullString
		var genres string
		d := &clipDisc{}
		if err := rows.Scan(&id, &d.title, &raw, &d.year, &d.rating, &genres); err != nil {
			return nil, err
		}
		if !raw.Valid || json.Unmarshal([]byte(raw.String), &d.info) != nil {
			continue
		}
		d.genre, _, _ = strings.Cut(genres, "|")
		out[id] = d
	}
	return out, rows.Err()
}

// ErrEncryptedClip: a channel can't be given a scene from an encrypted disc.
// The server words the refusal the same way as everywhere else discs are refused.
var ErrEncryptedClip = errors.New("encrypted disc")

// checkClips refuses scenes from encrypted discs when a channel is saved, so an
// owner hears about it at once rather than getting a channel that plays nothing.
func (s *Store) checkClips(ctx context.Context, def *Definition) error {
	for _, r := range def.allRules() {
		ids := r.clipItems()
		if len(ids) == 0 {
			continue
		}
		var n int
		err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM items WHERE id IN (`+placeholders(len(ids))+`)
			AND COALESCE(json_extract(disc_json, '$.scrambled'), 0) <> 0`, anyArgs(ids)...).Scan(&n)
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrEncryptedClip
		}
	}
	return nil
}

// discLength is how long a disc's main title runs.
func discLength(raw string, fallback float64) float64 {
	var d library.DiscInfo
	if json.Unmarshal([]byte(raw), &d) != nil {
		return fallback
	}
	for _, t := range d.Titles {
		if t.Number == d.MainTitle && t.DurationSec > 0 {
			return t.DurationSec
		}
	}
	return fallback
}

func episodeSub(title string, season, episode, end sql.NullInt64) string {
	var b strings.Builder
	if season.Valid && episode.Valid && episode.Int64 >= 0 {
		if season.Int64 == 0 {
			b.WriteString("Special " + strconv.FormatInt(episode.Int64, 10))
		} else {
			b.WriteString("S" + strconv.FormatInt(season.Int64, 10) + " E" + strconv.FormatInt(episode.Int64, 10))
			if end.Valid && end.Int64 > episode.Int64 {
				b.WriteString("–" + strconv.FormatInt(end.Int64, 10))
			}
		}
	}
	if title != "" {
		if b.Len() > 0 {
			b.WriteString(" · ")
		}
		b.WriteString(title)
	}
	return b.String()
}

// PoolStats says how much rules choose.
type PoolStats struct {
	Titles  int     `json:"titles"`
	Shows   int     `json:"shows"`
	Seconds float64 `json:"seconds"`
}

// Stats counts a pool without loading it.
func (s *Store) Stats(ctx context.Context, r Rules) (PoolStats, error) {
	var st PoolStats
	clips, err := s.loadClips(ctx, r.Clips)
	if err != nil {
		return st, err
	}
	for _, c := range clips {
		st.Titles++
		st.Seconds += float64(c.Dur)
	}
	if r.onlyClips() {
		return st, nil
	}
	w, args := r.where()
	var titles, shows int
	var sec sql.NullFloat64
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(DISTINCT i.show_id), SUM(i.runtime_sec)
		FROM items i LEFT JOIN shows s ON s.id = i.show_id WHERE `+w, args...).Scan(&titles, &shows, &sec)
	st.Titles += titles
	st.Shows = shows
	st.Seconds += sec.Float64
	return st, err
}

// excludes reports whether rules choose any title the account may not see.
// One query per set of rules, stopping at the first such title.
func (s *Store) excludes(ctx context.Context, r Rules, a *library.Access) (bool, error) {
	w, args := r.where()
	c, cargs := a.ItemClause()
	var one int
	// The clause is NULL for an unrated title, which NOT alone would let through.
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM items i LEFT JOIN shows s ON s.id = i.show_id WHERE `+w+` AND NOT IFNULL(`+c+`, 0) LIMIT 1`,
		append(args, cargs...)...).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}
