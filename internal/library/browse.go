package library

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// Browsing films and shows by anything, as one list: a filter and a sort make a
// query over both, so a studio's page can hold its films and its shows in one order.
// Every value reaches SQL as an argument; nothing from a request becomes statement text.

// BrowseFilter is what's asked for. The zero value is everything.
type BrowseFilter struct {
	Library    int64
	Search     string
	Genre      string
	Studio     string
	Network    string
	Year       int
	Decade     int
	Content    string // a content rating
	MinRating  float64
	Resolution string // 4k | 1080p | 720p | sd
	Audio      string // an audio language
	Subtitle   string // a subtitle language
	Format     string // an audio format such as "AC3 5.1"
	Tag        string
	Edition    string
	Series     string // a film series, which is a collection made from metadata
	Kind       string // video | dvd | bluray | disc | show
	Disc       string // what a disc holds: movie | tv | music
	// AnimeOf: this anime library's shows and the anime of every TV library; HideAnime: a
	// TV library when anime goes to the anime library only.
	AnimeOf, HideAnime bool
	Watched    string // yes | no
	Platform   string // a streaming service's key (needs listings on)
	Sort       string // title | added | newest | oldest | rating | quality | shuffle
	Desc       bool
	Seed       int
	Limit      int
	Offset     int
}

// ParseBrowseFilter reads a filter from a request's query or a saved view.
func ParseBrowseFilter(q url.Values) BrowseFilter {
	n := func(k string) int {
		v, _ := strconv.Atoi(q.Get(k))
		return v
	}
	f := BrowseFilter{Library: int64(n("library")), Search: clip(q.Get("search"), 200), Genre: clip(q.Get("genre"), 100), Studio: clip(q.Get("studio"), 120), Network: clip(q.Get("network"), 120),
		Year: n("year"), Decade: n("decade"), Content: clip(q.Get("content"), 20), Resolution: q.Get("resolution"), Audio: clip(q.Get("audio"), 12), Subtitle: clip(q.Get("subtitle"), 12), Format: clip(q.Get("format"), 40),
		Tag: clip(q.Get("tag"), 60), Edition: clip(q.Get("edition"), 60), Series: clip(q.Get("series"), 120), Kind: q.Get("kind"), Disc: q.Get("disc"), Watched: q.Get("watched"), Platform: clip(q.Get("platform"), 80), Sort: q.Get("sort"), Desc: q.Get("desc") == "1",
		Seed: n("seed"), Limit: n("limit"), Offset: n("offset")}
	if v, err := strconv.ParseFloat(q.Get("minRating"), 64); err == nil && v > 0 && v <= 10 {
		f.MinRating = v
	}
	return f
}

// browseKeys are what a saved view can hold.
var browseKeys = []string{"genre", "studio", "network", "year", "decade", "content", "minRating", "resolution", "audio", "subtitle", "format", "tag", "edition", "series", "kind", "disc", "watched", "platform", "sort", "desc", "search"}

// CleanViewQuery keeps only what a saved view can hold, for storing.
func CleanViewQuery(q url.Values) url.Values {
	out := url.Values{}
	for _, k := range browseKeys {
		if v := q.Get(k); v != "" {
			out.Set(k, clip(v, 200))
		}
	}
	return out
}

// BrowseRow is one result, a film or a show.
type BrowseRow struct {
	Type string `json:"type"` // item | show
	Item *Item  `json:"item,omitempty"`
	Show *Show  `json:"show,omitempty"`
}

// BrowseResult is a page of results.
type BrowseResult struct {
	Results []BrowseRow `json:"results"`
	Total   int         `json:"total"`
}

func genreClause(col string, names []string) (string, []any) {
	if len(names) == 0 {
		return "", nil
	}
	var or []string
	var args []any
	for _, n := range names {
		or = append(or, "('|' || IFNULL("+col+",'') || '|') LIKE ?")
		args = append(args, "%|"+likeEscape(n)+"|%")
	}
	return "(" + strings.Join(or, " OR ") + ")", args
}

// likeEscape: names are compared by LIKE, where % and _ mean something.
func likeEscape(s string) string {
	return strings.NewReplacer("%", "", "_", " ").Replace(s)
}

const (
	res4K   = "(%[1]swidth >= 3000 OR %[1]sheight >= 1800)"
	res1080 = "((%[1]swidth >= 1700 OR %[1]sheight >= 900) AND %[1]swidth < 3000 AND %[1]sheight < 1800)"
	res720  = "((%[1]swidth >= 1100 OR %[1]sheight >= 600) AND %[1]swidth < 1700 AND %[1]sheight < 900)"
	resSD   = "(%[1]swidth > 0 AND %[1]swidth < 1100 AND %[1]sheight < 600)"
)

func resolutionClause(res string, alias string) string {
	var tpl string
	switch res {
	case "4k":
		tpl = res4K
	case "1080p":
		tpl = res1080
	case "720p":
		tpl = res720
	case "sd":
		tpl = resSD
	default:
		return ""
	}
	return strings.ReplaceAll(tpl, "%[1]s", alias)
}

// videoLibraries leaves out libraries whose titles aren't films or shows to browse.
const videoLibraries = "(SELECT id FROM libraries WHERE type IN ('movies', 'discs', 'videos', 'shows', 'anime') OR (type = '" + CommercialsType + "' AND show_main = 1))"

// Browse runs a filter.
func (s *Store) Browse(ctx context.Context, f BrowseFilter, userID int64, access *Access) (*BrowseResult, error) {
	genres := s.expandGenre(ctx, f.Library, f.Genre)
	ic, ia := access.ItemClause()
	iw := []string{"i.missing = 0", "i.show_id IS NULL", hideLinked, "i.kind IN ('video', 'dvd-iso', 'dvd-folder', 'bluray-iso', 'bluray-folder', '" + CDKind + "')", "i.library_id IN " + videoLibraries, ic}
	iargs := append([]any{}, ia...)
	sc, sa := access.ShowClause()
	sw := []string{"s.missing = 0", "EXISTS (SELECT 1 FROM items e WHERE e.show_id = s.id AND e.missing = 0)", sc}
	sargs := append([]any{}, sa...)
	if f.Library != 0 {
		iw = append(iw, "i.library_id = ?")
		iargs = append(iargs, f.Library)
		if f.AnimeOf {
			sw = append(sw, "(s.library_id = ? OR ("+animeGenre+" AND s.library_id IN (SELECT id FROM libraries WHERE type = 'shows')))")
		} else {
			sw = append(sw, "s.library_id = ?")
		}
		sargs = append(sargs, f.Library)
	}
	if f.HideAnime {
		sw = append(sw, "NOT "+animeGenre)
	}
	both := func(item, show string, a ...any) {
		iw = append(iw, item)
		iargs = append(iargs, a...)
		sw = append(sw, show)
		sargs = append(sargs, a...)
	}
	noItems, noShows := false, false
	if f.Search != "" {
		like := "%" + f.Search + "%"
		both("i.title LIKE ?", "s.title LIKE ?", like)
	}
	if f.Genre != "" {
		c, a := genreClause("i.genres", genres)
		c2, _ := genreClause("s.genres", genres)
		both(c, c2, a...)
	}
	if f.Studio != "" {
		k := BrandKey(f.Studio)
		both("EXISTS (SELECT 1 FROM title_brands b WHERE b.kind = 'studio' AND b.key = ? AND b.item_id = i.id)", "EXISTS (SELECT 1 FROM title_brands b WHERE b.kind = 'studio' AND b.key = ? AND b.show_id = s.id)", k)
	}
	if f.Network != "" {
		k := BrandKey(f.Network)
		noItems = true
		sw = append(sw, "EXISTS (SELECT 1 FROM title_brands b WHERE b.kind = 'network' AND b.key = ? AND b.show_id = s.id)")
		sargs = append(sargs, k)
	}
	if f.Year > 0 {
		both("i.year = ?", "s.year = ?", f.Year)
	}
	if f.Decade > 0 {
		iw = append(iw, "i.year >= ? AND i.year < ?")
		iargs = append(iargs, f.Decade, f.Decade+10)
		sw = append(sw, "s.year >= ? AND s.year < ?")
		sargs = append(sargs, f.Decade, f.Decade+10)
	}
	if f.Content != "" {
		both("i.content_rating = ?", "s.content_rating = ?", f.Content)
	}
	if f.MinRating > 0 {
		both("i.rating >= ?", "s.rating >= ?", f.MinRating)
	}
	if c := resolutionClause(f.Resolution, "i."); c != "" {
		iw = append(iw, c)
		sw = append(sw, "EXISTS (SELECT 1 FROM items e WHERE e.show_id = s.id AND e.missing = 0 AND "+resolutionClause(f.Resolution, "e.")+")")
	}
	if f.Audio != "" {
		like := "%|" + likeEscape(f.Audio) + "|%"
		both("('|' || IFNULL(i.audio_langs,'') || '|') LIKE ?", "EXISTS (SELECT 1 FROM items e WHERE e.show_id = s.id AND e.missing = 0 AND ('|' || IFNULL(e.audio_langs,'') || '|') LIKE ?)", like)
	}
	if f.Subtitle != "" {
		like := "%|" + likeEscape(f.Subtitle) + "|%"
		both("('|' || IFNULL(i.sub_langs,'') || '|') LIKE ?", "EXISTS (SELECT 1 FROM items e WHERE e.show_id = s.id AND e.missing = 0 AND ('|' || IFNULL(e.sub_langs,'') || '|') LIKE ?)", like)
	}
	if f.Format != "" {
		noShows = true
		iw = append(iw, "EXISTS (SELECT 1 FROM title_meta m WHERE m.target = 'item' AND m.target_id = i.id AND m.audio_format = ?)")
		iargs = append(iargs, f.Format)
	}
	if f.Tag != "" {
		like := "%|" + likeEscape(f.Tag) + "|%"
		both("EXISTS (SELECT 1 FROM title_meta m WHERE m.target = 'item' AND m.target_id = i.id AND ('|' || m.tags || '|') LIKE ?)", "EXISTS (SELECT 1 FROM title_meta m WHERE m.target = 'show' AND m.target_id = s.id AND ('|' || m.tags || '|') LIKE ?)", like)
	}
	if f.Edition != "" {
		noShows = true
		iw = append(iw, "EXISTS (SELECT 1 FROM title_meta m WHERE m.target = 'item' AND m.target_id = i.id AND ('|' || m.edition || '|') LIKE ?)")
		iargs = append(iargs, "%|"+likeEscape(f.Edition)+"|%")
	}
	if f.Series != "" {
		noShows = true
		iw = append(iw, "EXISTS (SELECT 1 FROM title_brands b WHERE b.kind = '"+BrandSeries+"' AND b.key = ? AND b.item_id = i.id)")
		iargs = append(iargs, BrandKey(f.Series))
	}
	switch f.Kind {
	case "video":
		noShows = true
		iw = append(iw, "i.kind = 'video'")
	case "dvd":
		noShows = true
		iw = append(iw, "i.kind IN ('dvd-iso', 'dvd-folder')")
	case "bluray":
		noShows = true
		iw = append(iw, "i.kind IN ('bluray-iso', 'bluray-folder')")
	case "disc":
		noShows = true
		iw = append(iw, "i.kind IN ('dvd-iso', 'dvd-folder', 'bluray-iso', 'bluray-folder', '"+CDKind+"')")
	case "show":
		noItems = true
	}
	if f.Disc != "" {
		noShows = true
		iw = append(iw, "json_extract(i.disc_json, '$.content') = ?")
		iargs = append(iargs, f.Disc)
	}
	switch f.Watched {
	case "yes":
		iw = append(iw, "EXISTS (SELECT 1 FROM progress p WHERE p.item_id = i.id AND p.user_id = ? AND p.watched = 1)")
		iargs = append(iargs, userID)
		sw = append(sw, "NOT EXISTS (SELECT 1 FROM items e WHERE e.show_id = s.id AND e.missing = 0 AND NOT EXISTS (SELECT 1 FROM progress p WHERE p.item_id = e.id AND p.user_id = ? AND p.watched = 1))")
		sargs = append(sargs, userID)
	case "no":
		iw = append(iw, "NOT EXISTS (SELECT 1 FROM progress p WHERE p.item_id = i.id AND p.user_id = ? AND p.watched = 1)")
		iargs = append(iargs, userID)
		sw = append(sw, "EXISTS (SELECT 1 FROM items e WHERE e.show_id = s.id AND e.missing = 0 AND NOT EXISTS (SELECT 1 FROM progress p WHERE p.item_id = e.id AND p.user_id = ? AND p.watched = 1))")
		sargs = append(sargs, userID)
	}
	if f.Platform != "" {
		region := s.ListingsRegion(ctx)
		iw = append(iw, "EXISTS (SELECT 1 FROM listings l WHERE l.target = 'item' AND l.target_id = i.id AND l.region = ? AND l.platform_key = ?)")
		iargs = append(iargs, region, f.Platform)
		sw = append(sw, "EXISTS (SELECT 1 FROM listings l WHERE l.target = 'show' AND l.target_id = s.id AND l.region = ? AND l.platform_key = ?)")
		sargs = append(sargs, region, f.Platform)
	}

	key := func(item, show string) (string, string) { return item, show }
	var ik, sk, order string
	desc := f.Desc
	switch f.Sort {
	case "added":
		ik, sk = key("i.added_at", "(SELECT IFNULL(MAX(e.added_at), 0) FROM items e WHERE e.show_id = s.id AND e.missing = 0)")
		order = "k"
	case "newest", "oldest", "year":
		ik, sk = key("IFNULL(i.year, 0)", "IFNULL(s.year, 0)")
		order = "k"
		if f.Sort == "newest" {
			desc = true
		}
	case "rating":
		ik, sk = key("IFNULL(i.rating, 0)", "IFNULL(s.rating, 0)")
		order = "k"
	case "quality":
		ik, sk = key("IFNULL(i.width, 0) * IFNULL(i.height, 0)", "(SELECT IFNULL(MAX(e.width * e.height), 0) FROM items e WHERE e.show_id = s.id AND e.missing = 0)")
		order = "k"
	case "shuffle", "random":
		// The same seed gives the same order, so paging through a shuffle shows each title once.
		seed := f.Seed
		if seed <= 0 {
			seed = 1
		}
		seed = seed%100000 + 7
		ik, sk = key("((i.id * "+strconv.Itoa(seed)+" + 104729) * 2654435761) % 4294967291", "((s.id * "+strconv.Itoa(seed)+" + 15485863) * 2654435761) % 4294967291")
		order = "k"
	default:
		ik, sk = key("0", "0")
		order = "st COLLATE NOCASE"
	}
	if order == "k" {
		order = "k"
	}
	if desc {
		order += " DESC"
	}
	var parts []string
	var args []any
	if !noItems {
		parts = append(parts, "SELECT 'item' t, i.id id, i.sort_title st, "+ik+" k FROM items i WHERE "+strings.Join(iw, " AND "))
		args = append(args, iargs...)
	}
	if !noShows {
		parts = append(parts, "SELECT 'show' t, s.id id, s.sort_title st, "+sk+" k FROM shows s WHERE "+strings.Join(sw, " AND "))
		args = append(args, sargs...)
	}
	res := &BrowseResult{Results: []BrowseRow{}}
	if len(parts) == 0 {
		return res, nil
	}
	union := strings.Join(parts, " UNION ALL ")
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM ("+union+")", args...).Scan(&res.Total); err != nil {
		return nil, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 120
	}
	rows, err := s.DB.QueryContext(ctx, union+" ORDER BY "+order+", st COLLATE NOCASE, id LIMIT ? OFFSET ?", append(args, limit, max(f.Offset, 0))...)
	if err != nil {
		return nil, err
	}
	type ref struct {
		t  string
		id int64
	}
	var refs []ref
	var itemIDs, showIDs []int64
	for rows.Next() {
		var r ref
		var st, k any
		if err := rows.Scan(&r.t, &r.id, &st, &k); err != nil {
			rows.Close()
			return nil, err
		}
		refs = append(refs, r)
		if r.t == "item" {
			itemIDs = append(itemIDs, r.id)
		} else {
			showIDs = append(showIDs, r.id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	items := map[int64]*Item{}
	if len(itemIDs) > 0 {
		list, _, err := s.Items(ctx, ItemQuery{IDs: itemIDs, Limit: len(itemIDs), NoTotal: true, Brief: true, IncludeAttached: true})
		if err != nil {
			return nil, err
		}
		if err := s.AttachProgress(ctx, userID, list); err != nil {
			return nil, err
		}
		for _, it := range list {
			items[it.ID] = it
		}
	}
	shows := map[int64]*Show{}
	if len(showIDs) > 0 {
		list, err := s.Shows(ctx, ShowQuery{IDs: showIDs, UserID: userID})
		if err != nil {
			return nil, err
		}
		for _, sh := range list {
			shows[sh.ID] = sh
		}
	}
	for _, r := range refs {
		if r.t == "item" {
			if it := items[r.id]; it != nil {
				res.Results = append(res.Results, BrowseRow{Type: "item", Item: it})
			}
		} else if sh := shows[r.id]; sh != nil {
			res.Results = append(res.Results, BrowseRow{Type: "show", Show: sh})
		}
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// Facets: what there is to filter by
// ---------------------------------------------------------------------------

// Facet is one value and how many titles have it.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count int    `json:"count"`
}

// Facets lists everything a library can be filtered by.
type Facets struct {
	Genres      []Facet `json:"genres"`
	Studios     []Facet `json:"studios"`
	Networks    []Facet `json:"networks"`
	Decades     []Facet `json:"decades"`
	Years       []Facet `json:"years"`
	Content     []Facet `json:"content"`
	Resolutions []Facet `json:"resolutions"`
	Audio       []Facet `json:"audio"`
	Subtitles   []Facet `json:"subtitles"`
	Formats     []Facet `json:"formats"`
	Tags        []Facet `json:"tags"`
	Editions    []Facet `json:"editions"`
	Kinds       []Facet `json:"kinds"`
	Platforms   []Facet `json:"platforms"`
}

func bump(m map[string]int, k string) {
	if k != "" {
		m[k]++
	}
}

func facetList(m map[string]int, label func(string) string, byName bool) []Facet {
	out := make([]Facet, 0, len(m))
	for k, n := range m {
		f := Facet{Value: k, Count: n}
		if label != nil {
			f.Label = label(k)
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if !byName && out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.ToLower(out[i].Value) < strings.ToLower(out[j].Value)
	})
	return out
}

func resolutionOf(w, h int) string {
	switch {
	case w == 0 && h == 0:
		return ""
	case w >= 3000 || h >= 1800:
		return "4k"
	case w >= 1700 || h >= 900:
		return "1080p"
	case w >= 1100 || h >= 600:
		return "720p"
	}
	return "sd"
}

// LibraryFacets reads what's in a library (0 = all), counting only what this viewer may see.
func (s *Store) LibraryFacets(ctx context.Context, libraryID int64, access *Access) (*Facets, error) {
	genres, studios, networks, decades, years, content, res, audio, subs, formats, tags, editions, kinds, platforms :=
		map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	aliases := s.categoryAliases(ctx, libraryID)
	addYear := func(y sql.NullInt64) {
		if y.Valid && y.Int64 > 1800 {
			years[strconv.FormatInt(y.Int64, 10)]++
			decades[strconv.FormatInt(y.Int64/10*10, 10)]++
		}
	}
	addGenres := func(g string) {
		seen := map[string]bool{}
		for _, x := range splitList(g) {
			for _, c := range aliases.of(x) {
				if !seen[c] {
					seen[c] = true
					genres[c]++
				}
			}
		}
	}
	once := func(m map[string]int, list string) {
		seen := map[string]bool{}
		for _, x := range splitList(list) {
			if x != "und" && !seen[x] {
				seen[x] = true
				m[x]++
			}
		}
	}
	ic, ia := access.ItemClause()
	rows, err := s.DB.QueryContext(ctx, `SELECT i.kind, i.genres, i.year, i.content_rating, i.width, i.height, i.audio_langs, i.sub_langs,
		IFNULL(m.tags, ''), IFNULL(m.edition, ''), IFNULL(m.audio_format, '')
		FROM items i LEFT JOIN title_meta m ON m.target = 'item' AND m.target_id = i.id
		WHERE i.missing = 0 AND i.show_id IS NULL AND `+hideLinked+` AND i.kind IN ('video', 'dvd-iso', 'dvd-folder', 'bluray-iso', 'bluray-folder') AND i.library_id IN `+videoLibraries+` AND (? = 0 OR i.library_id = ?) AND `+ic,
		append([]any{libraryID, libraryID}, ia...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind, tag, edition, format string
		var g, cr, al, sl sql.NullString
		var year, w, h sql.NullInt64
		if err := rows.Scan(&kind, &g, &year, &cr, &w, &h, &al, &sl, &tag, &edition, &format); err != nil {
			rows.Close()
			return nil, err
		}
		addGenres(g.String)
		addYear(year)
		bump(content, cr.String)
		bump(res, resolutionOf(int(w.Int64), int(h.Int64)))
		once(audio, al.String)
		once(subs, sl.String)
		bump(formats, format)
		once(tags, tag)
		once(editions, edition)
		switch {
		case strings.HasPrefix(kind, "dvd-"):
			kinds["dvd"]++
		case strings.HasPrefix(kind, "bluray-"):
			kinds["bluray"]++
		default:
			kinds["video"]++
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	sc, sa := access.ShowClause()
	rows, err = s.DB.QueryContext(ctx, `SELECT s.genres, s.year, s.content_rating, IFNULL(m.tags, '') FROM shows s LEFT JOIN title_meta m ON m.target = 'show' AND m.target_id = s.id
		WHERE s.missing = 0 AND EXISTS (SELECT 1 FROM items e WHERE e.show_id = s.id AND e.missing = 0) AND (? = 0 OR s.library_id = ?) AND `+sc, append([]any{libraryID, libraryID}, sa...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var g, cr sql.NullString
		var year sql.NullInt64
		var tag string
		if err := rows.Scan(&g, &year, &cr, &tag); err != nil {
			rows.Close()
			return nil, err
		}
		addGenres(g.String)
		addYear(year)
		bump(content, cr.String)
		once(tags, tag)
		kinds["show"]++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// A show counts once for each thing any of its episodes has.
	rows, err = s.DB.QueryContext(ctx, `SELECT e.show_id, IFNULL(e.width, 0), IFNULL(e.height, 0), IFNULL(e.audio_langs, ''), IFNULL(e.sub_langs, '') FROM items e
		JOIN shows s ON s.id = e.show_id WHERE e.missing = 0 AND s.missing = 0 AND (? = 0 OR s.library_id = ?) AND `+sc+`
		GROUP BY e.show_id, e.width, e.height, e.audio_langs, e.sub_langs`, append([]any{libraryID, libraryID}, sa...)...)
	if err != nil {
		return nil, err
	}
	type facts struct{ res, audio, subs map[string]bool }
	byShow := map[int64]*facts{}
	for rows.Next() {
		var id int64
		var w, h int
		var al, sl string
		if err := rows.Scan(&id, &w, &h, &al, &sl); err != nil {
			rows.Close()
			return nil, err
		}
		f := byShow[id]
		if f == nil {
			f = &facts{map[string]bool{}, map[string]bool{}, map[string]bool{}}
			byShow[id] = f
		}
		if r := resolutionOf(w, h); r != "" {
			f.res[r] = true
		}
		for _, x := range splitList(al) {
			if x != "und" {
				f.audio[x] = true
			}
		}
		for _, x := range splitList(sl) {
			if x != "und" {
				f.subs[x] = true
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, f := range byShow {
		for k := range f.res {
			res[k]++
		}
		for k := range f.audio {
			audio[k]++
		}
		for k := range f.subs {
			subs[k]++
		}
	}
	for _, kb := range []struct {
		kind string
		m    map[string]int
	}{{LogoStudio, studios}, {LogoNetwork, networks}} {
		list, err := s.Brands(ctx, libraryID, kb.kind, access)
		if err != nil {
			return nil, err
		}
		for _, b := range list {
			kb.m[b.Name] = b.Count
		}
	}
	if s.ListingsEnabled(ctx) {
		list, err := s.ListingPlatforms(ctx, libraryID, access)
		if err != nil {
			return nil, err
		}
		for _, p := range list {
			platforms[p.Key] = p.Count
		}
	}
	platNames := s.platformNames(ctx)
	resNames := map[string]string{"4k": "4K", "1080p": "1080p", "720p": "720p", "sd": "SD"}
	kindNames := map[string]string{"video": "Video files", "dvd": "DVDs", "bluray": "Blu-rays", "show": "TV shows"}
	out := &Facets{
		Genres: facetList(genres, nil, false), Studios: facetList(studios, nil, false), Networks: facetList(networks, nil, false),
		Decades: facetList(decades, func(k string) string { return k + "s" }, true), Years: facetList(years, nil, true), Content: facetList(content, nil, true),
		Resolutions: facetList(res, func(k string) string { return resNames[k] }, false), Audio: facetList(audio, nil, false), Subtitles: facetList(subs, nil, false),
		Formats: facetList(formats, nil, false), Tags: facetList(tags, nil, false), Editions: facetList(editions, nil, false),
		Kinds: facetList(kinds, func(k string) string { return kindNames[k] }, false), Platforms: facetList(platforms, func(k string) string { return platNames[k] }, false),
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Generated categories
// ---------------------------------------------------------------------------

// Different services write the same genre differently, and some put two in one.
var genreAliases = map[string][]string{
	"sci-fi": {"Science Fiction"}, "sci fi": {"Science Fiction"}, "scifi": {"Science Fiction"}, "science fiction film": {"Science Fiction"}, "science-fiction": {"Science Fiction"},
	"sci-fi & fantasy": {"Science Fiction", "Fantasy"}, "science fiction & fantasy": {"Science Fiction", "Fantasy"},
	"action & adventure": {"Action", "Adventure"}, "action/adventure": {"Action", "Adventure"},
	"war & politics": {"War", "Politics"}, "kids": {"Kids"}, "children": {"Kids"}, "children's": {"Kids"}, "childrens": {"Kids"},
	"animated": {"Animation"}, "computer-animated": {"Animation"}, "computer animated": {"Animation"}, "cartoon": {"Animation"},
	"documentaries": {"Documentary"}, "docu": {"Documentary"}, "rom-com": {"Romantic Comedy"}, "romcom": {"Romantic Comedy"},
	"musical film": {"Musical"}, "music": {"Music"}, "suspense": {"Thriller"}, "thriller film": {"Thriller"},
}

func genreCanon(raw string) []string {
	k := strings.ToLower(strings.TrimSpace(raw))
	if v, ok := genreAliases[k]; ok {
		return v
	}
	if strings.HasSuffix(k, " film") {
		return []string{titleCase(strings.TrimSuffix(k, " film"))}
	}
	return []string{titleCaseWords(raw)}
}

// titleCaseWords capitalises a lower-case genre without touching one already written with care.
func titleCaseWords(s string) string {
	s = strings.TrimSpace(s)
	if s == strings.ToLower(s) {
		return titleCase(s)
	}
	return s
}

func categoryKey(name string) string { return BrandKey(name) }

// Category is one generated category.
type Category struct {
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Genre    string   `json:"genre"`
	Count    int      `json:"count"`
	Hidden   bool     `json:"hidden"`
	Position int      `json:"position"`
	Merged   []string `json:"merged,omitempty"` // the other categories folded into this one
	// What to draw it with: its members with the most artwork-worthy pictures.
	Art []CategoryArt `json:"art"`
}

// CategoryArt is one picture for a category's card.
type CategoryArt struct {
	Type    string `json:"type"` // item | show
	ID      int64  `json:"id"`
	Version int64  `json:"v"`
}

type categoryOverride struct {
	Name, Merged string
	Hidden       bool
	Position     int
	ArtItem      int64
}

type genreIndex struct {
	over map[string]categoryOverride
}

func (s *Store) categoryOverrides(ctx context.Context, libraryID int64) map[string]categoryOverride {
	out := map[string]categoryOverride{}
	rows, err := s.DB.QueryContext(ctx, `SELECT key, name, hidden, position, merged_into, art_item FROM categories WHERE library_id = ?`, libraryID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var o categoryOverride
		var hidden int
		if rows.Scan(&k, &o.Name, &hidden, &o.Position, &o.Merged, &o.ArtItem) == nil {
			o.Hidden = hidden != 0
			out[k] = o
		}
	}
	return out
}

func (s *Store) categoryAliases(ctx context.Context, libraryID int64) *genreIndex {
	return &genreIndex{over: s.categoryOverrides(ctx, libraryID)}
}

// of says which categories a raw genre belongs to, as they're named now.
func (g *genreIndex) of(raw string) []string {
	var out []string
	for _, name := range genreCanon(raw) {
		k := categoryKey(name)
		// A merge can be followed a few steps; a loop can't be followed forever.
		for i := 0; i < 8; i++ {
			o, ok := g.over[k]
			if !ok || o.Merged == "" || o.Merged == k {
				break
			}
			k = o.Merged
			name = k
		}
		if o, ok := g.over[k]; ok && o.Name != "" {
			name = o.Name
		} else if name == k {
			name = titleCase(k)
		}
		out = append(out, name)
	}
	return out
}

// expandGenre turns the name of a category into every spelling in the library it
// stands for, so Sci-Fi and Science Fiction come up together.
func (s *Store) expandGenre(ctx context.Context, libraryID int64, name string) []string {
	if name == "" {
		return nil
	}
	aliases := s.categoryAliases(ctx, libraryID)
	want := strings.ToLower(name)
	seen := map[string]bool{}
	out := []string{}
	add := func(list []string) {
		for _, raw := range list {
			for _, c := range aliases.of(raw) {
				if strings.ToLower(c) == want && !seen[raw] {
					seen[raw] = true
					out = append(out, raw)
				}
			}
		}
	}
	if g, err := s.Genres(ctx, libraryID, nil); err == nil {
		add(g)
	}
	if g, err := s.ShowGenres(ctx, libraryID, nil); err == nil {
		add(g)
	}
	if len(out) == 0 {
		out = append(out, name) // nothing has it: an empty result, not everything
	}
	return out
}

// Categories works out a library's categories from its genres. includeHidden is for
// someone arranging them.
func (s *Store) Categories(ctx context.Context, libraryID int64, access *Access, includeHidden bool) ([]Category, error) {
	over := s.categoryOverrides(ctx, libraryID)
	g := &genreIndex{over: over}
	type cat struct {
		c     Category
		rated []scored
	}
	byKey := map[string]*cat{}
	addTitle := func(genres, typ string, id, v int64, rating float64, backdrop bool) {
		seen := map[string]bool{}
		for _, raw := range splitList(genres) {
			for _, name := range g.of(raw) {
				k := categoryKey(name)
				if seen[k] {
					continue
				}
				seen[k] = true
				c := byKey[k]
				if c == nil {
					c = &cat{c: Category{Key: k, Name: name, Genre: name}}
					byKey[k] = c
				}
				c.c.Count++
				if backdrop && len(c.rated) < 200 {
					c.rated = append(c.rated, scored{CategoryArt{typ, id, v}, rating})
				}
			}
		}
	}
	ic, ia := access.ItemClause()
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id, i.genres, i.updated_at, IFNULL(i.rating, 0), (IFNULL(i.backdrop, '') <> '' OR IFNULL(i.poster, '') <> '') FROM items i
		WHERE i.missing = 0 AND i.show_id IS NULL AND IFNULL(i.genres, '') <> '' AND `+hideLinked+` AND i.kind IN ('video', 'dvd-iso', 'dvd-folder', 'bluray-iso', 'bluray-folder') AND i.library_id IN `+videoLibraries+` AND (? = 0 OR i.library_id = ?) AND `+ic,
		append([]any{libraryID, libraryID}, ia...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, v int64
		var genres string
		var rating float64
		var art bool
		if err := rows.Scan(&id, &genres, &v, &rating, &art); err != nil {
			rows.Close()
			return nil, err
		}
		addTitle(genres, "item", id, v, rating, art)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	sc, sa := access.ShowClause()
	rows, err = s.DB.QueryContext(ctx, `SELECT s.id, s.genres, s.updated_at, IFNULL(s.rating, 0), (IFNULL(s.backdrop, '') <> '' OR IFNULL(s.poster, '') <> '') FROM shows s
		WHERE s.missing = 0 AND IFNULL(s.genres, '') <> '' AND EXISTS (SELECT 1 FROM items e WHERE e.show_id = s.id AND e.missing = 0) AND (? = 0 OR s.library_id = ?) AND `+sc,
		append([]any{libraryID, libraryID}, sa...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, v int64
		var genres string
		var rating float64
		var art bool
		if err := rows.Scan(&id, &genres, &v, &rating, &art); err != nil {
			rows.Close()
			return nil, err
		}
		addTitle(genres, "show", id, v, rating, art)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	merged := map[string][]string{}
	for k, o := range over {
		if o.Merged != "" {
			merged[o.Merged] = append(merged[o.Merged], k)
		}
	}
	out := make([]Category, 0, len(byKey))
	for k, c := range byKey {
		o := over[k]
		c.c.Hidden, c.c.Position = o.Hidden, o.Position
		c.c.Merged = merged[k]
		if c.c.Hidden && !includeHidden {
			continue
		}
		sort.SliceStable(c.rated, func(i, j int) bool { return c.rated[i].rating > c.rated[j].rating })
		art := []CategoryArt{}
		if o.ArtItem != 0 {
			art = append(art, CategoryArt{"item", o.ArtItem, 0})
		}
		// More than a card shows, so there are others to choose from when one is taken.
		for _, r := range c.rated {
			if len(art) >= 12 {
				break
			}
			art = append(art, r.art)
		}
		c.c.Art = art
		out = append(out, c.c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := out[i].Position, out[j].Position
		if pi != pj {
			if pi == 0 {
				return false // one that has been placed comes before one that hasn't
			}
			if pj == 0 {
				return true
			}
			return pi < pj
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	// A row of categories shouldn't show the same picture on every card: each takes the
	// best one nothing before it has used, if it has one.
	used := map[int64]bool{}
	for i := range out {
		art := out[i].Art
		for j, a := range art {
			if !used[a.ID] {
				art[0], art[j] = art[j], art[0]
				break
			}
		}
		if len(art) > 0 {
			used[art[0].ID] = true
		}
	}
	return out, nil
}

type scored struct {
	art    CategoryArt
	rating float64
}

// CategoryChange is an edit to a category.
type CategoryChange struct {
	Key     string  `json:"key"`
	Name    *string `json:"name"`
	Hidden  *bool   `json:"hidden"`
	Merge   *string `json:"mergeInto"` // "" splits it back out
	ArtItem *int64  `json:"artItem"`
}

// ChangeCategories applies edits and an order. A new order lists keys, first to last.
func (s *Store) ChangeCategories(ctx context.Context, libraryID int64, changes []CategoryChange, order []string) error {
	if len(changes) > 500 || len(order) > 1000 {
		return errors.New("too many categories at once")
	}
	ensure := func(key string) error {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO categories(library_id, key) VALUES(?, ?) ON CONFLICT(library_id, key) DO NOTHING`, libraryID, key)
		return err
	}
	for _, c := range changes {
		k := categoryKey(c.Key)
		if k == "" {
			return errors.New("which category?")
		}
		if err := ensure(k); err != nil {
			return err
		}
		if c.Name != nil {
			n := cleanText(*c.Name, 60, false)
			if _, err := s.DB.ExecContext(ctx, `UPDATE categories SET name = ? WHERE library_id = ? AND key = ?`, n, libraryID, k); err != nil {
				return err
			}
		}
		if c.Hidden != nil {
			v := 0
			if *c.Hidden {
				v = 1
			}
			if _, err := s.DB.ExecContext(ctx, `UPDATE categories SET hidden = ? WHERE library_id = ? AND key = ?`, v, libraryID, k); err != nil {
				return err
			}
		}
		if c.ArtItem != nil {
			if _, err := s.DB.ExecContext(ctx, `UPDATE categories SET art_item = ? WHERE library_id = ? AND key = ?`, *c.ArtItem, libraryID, k); err != nil {
				return err
			}
		}
		if c.Merge != nil {
			into := categoryKey(*c.Merge)
			if into == k {
				return errors.New("a category can't be merged into itself")
			}
			// Folding B into A, where A already goes into B, would lose both.
			over := s.categoryOverrides(ctx, libraryID)
			for at, i := into, 0; at != "" && i < 16; i++ {
				if at == k {
					return errors.New("those two are already merged the other way round")
				}
				at = over[at].Merged
			}
			if into != "" {
				if err := ensure(into); err != nil {
					return err
				}
			}
			if _, err := s.DB.ExecContext(ctx, `UPDATE categories SET merged_into = ? WHERE library_id = ? AND key = ?`, into, libraryID, k); err != nil {
				return err
			}
		}
	}
	for i, key := range order {
		k := categoryKey(key)
		if k == "" {
			continue
		}
		if err := ensure(k); err != nil {
			return err
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE categories SET position = ? WHERE library_id = ? AND key = ?`, i+1, libraryID, k); err != nil {
			return err
		}
	}
	return nil
}

// ResetCategories forgets every change to a library's categories.
func (s *Store) ResetCategories(ctx context.Context, libraryID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM categories WHERE library_id = ?`, libraryID)
	return err
}

// collectionName names a series the way a collection reads.
func collectionName(series string) string {
	l := strings.ToLower(series)
	if containsAny(l, "collection", "series", "trilogy", "saga", "franchise") {
		return series
	}
	return series + " Collection"
}

// pickSeries chooses which of the series a source names to collect a film under. Some films
// have several, and some of those are only lists of a studio's films, so the one that shares
// a word with the film's title comes first, and a list is never a series.
func pickSeries(names []string, title string) string {
	inTitle := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(title)) {
		if len(w) > 3 {
			inTitle[w] = true
		}
	}
	first := ""
	for _, name := range names {
		l := strings.ToLower(name)
		if name == "" || containsAny(l, "list of", "filmography", "feature film") || strings.HasSuffix(l, " films") {
			continue
		}
		for _, w := range strings.Fields(l) {
			if inTitle[w] {
				return collectionName(name)
			}
		}
		if first == "" {
			first = name
		}
	}
	if first == "" {
		return ""
	}
	return collectionName(first)
}

// SeriesInLibrary lists the film series a library has at least two of, each with the art
// of its best-rated member: collections nobody had to make.
func (s *Store) SeriesInLibrary(ctx context.Context, libraryID int64, access *Access) ([]LibraryCollection, error) {
	ic, ia := access.ItemClause()
	rows, err := s.DB.QueryContext(ctx, `SELECT b.key, MIN(b.name), COUNT(*), i.id, i.updated_at, MAX(IFNULL(i.rating, 0)) FROM title_brands b JOIN items i ON i.id = b.item_id
		WHERE b.kind = ? AND (? = 0 OR b.library_id = ?) AND i.missing = 0 AND `+hideLinked+` AND `+ic+`
		GROUP BY b.key HAVING COUNT(*) >= 2 ORDER BY MIN(b.name) COLLATE NOCASE`, append([]any{BrandSeries, libraryID, libraryID}, ia...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LibraryCollection{}
	for rows.Next() {
		var c LibraryCollection
		var id, v int64
		var best float64
		if err := rows.Scan(&c.Key, &c.Name, &c.Count, &id, &v, &best); err != nil {
			return nil, err
		}
		c.Art = []CategoryArt{{"item", id, v}}
		out = append(out, c)
	}
	return out, rows.Err()
}

// LibraryCollections lists collections holding anything from a library, for a shelf.
type LibraryCollection struct {
	ID int64 `json:"id"`
	// Key: a series, made from metadata, rather than a collection someone made.
	Key   string        `json:"key,omitempty"`
	Name  string        `json:"name"`
	Count int           `json:"count"`
	Art   []CategoryArt `json:"art"`
}

// CollectionsInLibrary lists a viewer's collections with members in this library.
func (s *Store) CollectionsInLibrary(ctx context.Context, libraryID, userID int64, access *Access) ([]LibraryCollection, error) {
	ic, ia := access.ItemClause()
	args := append([]any{libraryID, userID}, ia...)
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id, c.name, COUNT(*), MAX(i.id), MAX(i.updated_at) FROM collections c
		JOIN collection_items ci ON ci.collection_id = c.id JOIN items i ON i.id = ci.item_id
		WHERE c.kind = 'manual' AND c.system = '' AND i.missing = 0 AND i.library_id = ? AND (c.owner_id = ? OR c.shared = 1) AND `+ic+`
		GROUP BY c.id ORDER BY c.name COLLATE NOCASE LIMIT 100`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LibraryCollection{}
	for rows.Next() {
		var c LibraryCollection
		var id, v int64
		if err := rows.Scan(&c.ID, &c.Name, &c.Count, &id, &v); err != nil {
			return nil, err
		}
		c.Art = []CategoryArt{{"item", id, v}}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Saved views
// ---------------------------------------------------------------------------

// SavedView is a browse someone kept.
type SavedView struct {
	ID        int64  `json:"id"`
	LibraryID int64  `json:"libraryId"`
	Name      string `json:"name"`
	Query     string `json:"query"` // a URL query
	Pinned    bool   `json:"pinned"`
	Position  int    `json:"position"`
}

// maxViews bounds what one account keeps.
const maxViews = 100

// SavedViews lists an account's views.
func (s *Store) SavedViews(ctx context.Context, userID int64) ([]SavedView, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, library_id, name, query, pinned, position FROM saved_views WHERE user_id = ? ORDER BY position, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SavedView{}
	for rows.Next() {
		var v SavedView
		var pinned int
		if err := rows.Scan(&v.ID, &v.LibraryID, &v.Name, &v.Query, &pinned, &v.Position); err != nil {
			return nil, err
		}
		v.Pinned = pinned != 0
		out = append(out, v)
	}
	return out, rows.Err()
}

// SaveView adds or changes a view of this account's.
func (s *Store) SaveView(ctx context.Context, userID int64, v *SavedView) error {
	v.Name = cleanText(v.Name, 60, false)
	if v.Name == "" {
		return errors.New("give the view a name")
	}
	q, err := url.ParseQuery(v.Query)
	if err != nil {
		return errors.New("that isn't a view")
	}
	v.Query = CleanViewQuery(q).Encode()
	pinned := 0
	if v.Pinned {
		pinned = 1
	}
	if v.ID == 0 {
		var n int
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM saved_views WHERE user_id = ?`, userID).Scan(&n); err != nil {
			return err
		}
		if n >= maxViews {
			return errors.New("that's as many saved views as an account can have: remove one first")
		}
		res, err := s.DB.ExecContext(ctx, `INSERT INTO saved_views(user_id, library_id, name, query, pinned, position, created_at) VALUES(?,?,?,?,?,?,?)`, userID, v.LibraryID, v.Name, v.Query, pinned, n, db.Now())
		if err != nil {
			return err
		}
		v.ID, _ = res.LastInsertId()
		return nil
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE saved_views SET name = ?, query = ?, pinned = ?, library_id = ? WHERE id = ? AND user_id = ?`, v.Name, v.Query, pinned, v.LibraryID, v.ID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteView removes a view of this account's.
func (s *Store) DeleteView(ctx context.Context, userID, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM saved_views WHERE id = ? AND user_id = ?`, id, userID)
	return err
}
