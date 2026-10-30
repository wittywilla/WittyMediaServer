package channels

import (
	"context"
	"database/sql"
	"encoding/json"
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
	if len(r.Libraries) > 0 {
		add("i.library_id IN ("+placeholders(len(r.Libraries))+")", anyArgs(r.Libraries)...)
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
			parts = append(parts, "i.studio = ? COLLATE NOCASE")
			a = append(a, n)
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
	if len(r.Shows) > 0 || len(r.Items) > 0 {
		var parts []string
		var a []any
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
	return out, rows.Err()
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
	w, args := r.where()
	var st PoolStats
	var sec sql.NullFloat64
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(DISTINCT i.show_id), SUM(i.runtime_sec)
		FROM items i LEFT JOIN shows s ON s.id = i.show_id WHERE `+w, args...).Scan(&st.Titles, &st.Shows, &sec)
	st.Seconds = sec.Float64
	return st, err
}

// excludes reports whether rules choose any title the account may not see.
// One query per set of rules, stopping at the first such title.
func (s *Store) excludes(ctx context.Context, r Rules, a *library.Access) (bool, error) {
	w, args := r.where()
	c, cargs := a.ItemClause()
	var one int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM items i LEFT JOIN shows s ON s.id = i.show_id WHERE `+w+` AND NOT `+c+` LIMIT 1`,
		append(args, cargs...)...).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}
