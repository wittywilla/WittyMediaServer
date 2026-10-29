package library

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// Show is a TV series in a shows library. Its episodes are items.
type Show struct {
	ID            int64    `json:"id"`
	LibraryID     int64    `json:"libraryId"`
	Path          string   `json:"path"`
	Title         string   `json:"title"`
	SortTitle     string   `json:"sortTitle"`
	Year          *int     `json:"year"`
	Overview      string   `json:"overview"`
	Genres        []string `json:"genres"`
	Rating        *float64 `json:"rating"`
	ContentRating string   `json:"contentRating"`
	Network       string   `json:"network"`
	Status        string   `json:"status"`
	Poster        string   `json:"poster"`
	Backdrop      string   `json:"backdrop"`
	Provider      string   `json:"provider"`
	ProviderID    string   `json:"providerId"`
	AddedAt       int64    `json:"addedAt"`
	UpdatedAt     int64    `json:"updatedAt"`
	// Show-wide intro range used by "Skip intro" when an episode has no
	// intro chapter of its own.
	IntroStart *float64 `json:"introStart,omitempty"`
	IntroEnd   *float64 `json:"introEnd,omitempty"`
	// View is this show's episode layout: cards | list | "" (server default).
	View string `json:"view"`
	// Aggregates (per user for Unwatched):
	EpisodeCount int   `json:"episodeCount"`
	SeasonCount  int   `json:"seasonCount"`
	Unwatched    int   `json:"unwatched"`
	LastAddedAt  int64 `json:"lastAddedAt"`
}

// Season is per-season artwork and counts.
type Season struct {
	Number       int    `json:"number"`
	Title        string `json:"title"`
	Overview     string `json:"overview"`
	Poster       string `json:"poster"`
	EpisodeCount int    `json:"episodeCount"`
	Unwatched    int    `json:"unwatched"`
}

const showCols = `s.id, s.library_id, s.path, s.title, s.sort_title, s.year, s.overview, s.genres, s.rating, s.content_rating,
	s.network, s.status, s.poster, s.backdrop, s.provider, s.provider_id, s.added_at, s.updated_at, s.intro_start, s.intro_end, IFNULL(s.view, ''),
	(SELECT COUNT(*) FROM items i WHERE i.show_id = s.id AND i.missing = 0),
	(SELECT COUNT(DISTINCT i.season) FROM items i WHERE i.show_id = s.id AND i.missing = 0),
	(SELECT COUNT(*) FROM items i WHERE i.show_id = s.id AND i.missing = 0 AND NOT EXISTS
		(SELECT 1 FROM progress p WHERE p.item_id = i.id AND p.user_id = ? AND p.watched = 1)),
	(SELECT IFNULL(MAX(i.added_at), 0) FROM items i WHERE i.show_id = s.id AND i.missing = 0)`

func scanShow(sc interface{ Scan(...any) error }) (*Show, error) {
	var sh Show
	var year sql.NullInt64
	var rating sql.NullFloat64
	var overview, genres, cr, network, status, poster, backdrop, provider, providerID sql.NullString
	var introStart, introEnd sql.NullFloat64
	if err := sc.Scan(&sh.ID, &sh.LibraryID, &sh.Path, &sh.Title, &sh.SortTitle, &year, &overview, &genres, &rating, &cr,
		&network, &status, &poster, &backdrop, &provider, &providerID, &sh.AddedAt, &sh.UpdatedAt, &introStart, &introEnd, &sh.View,
		&sh.EpisodeCount, &sh.SeasonCount, &sh.Unwatched, &sh.LastAddedAt); err != nil {
		return nil, err
	}
	if introStart.Valid && introEnd.Valid {
		sh.IntroStart, sh.IntroEnd = &introStart.Float64, &introEnd.Float64
	}
	sh.Year = nullInt(year)
	if rating.Valid {
		sh.Rating = &rating.Float64
	}
	sh.Overview, sh.Genres, sh.ContentRating, sh.Network, sh.Status = overview.String, splitList(genres.String), cr.String, network.String, status.String
	sh.Poster, sh.Backdrop, sh.Provider, sh.ProviderID = poster.String, backdrop.String, provider.String, providerID.String
	return &sh, nil
}

// ShowQuery filters shows.
type ShowQuery struct {
	LibraryID int64
	Search    string
	Genre     string
	Network   string // exact network name (case-insensitive)
	Sort      string // title | added | year | rating
	Desc      bool
	Limit     int
	UserID    int64
}

func (s *Store) Shows(ctx context.Context, q ShowQuery) ([]*Show, error) {
	where := []string{"s.missing = 0"}
	args := []any{q.UserID}
	if q.LibraryID != 0 {
		where = append(where, "s.library_id = ?")
		args = append(args, q.LibraryID)
	}
	if q.Search != "" {
		where = append(where, "(s.title LIKE ? OR s.overview LIKE ?)")
		like := "%" + q.Search + "%"
		args = append(args, like, like)
	}
	if q.Genre != "" {
		where = append(where, "('|' || IFNULL(s.genres,'') || '|') LIKE ?")
		args = append(args, "%|"+q.Genre+"|%")
	}
	if q.Network != "" {
		where = append(where, "s.network = ? COLLATE NOCASE")
		args = append(args, q.Network)
	}
	order := "s.sort_title COLLATE NOCASE"
	switch q.Sort {
	case "added":
		order = "(SELECT IFNULL(MAX(i.added_at), 0) FROM items i WHERE i.show_id = s.id AND i.missing = 0)"
	case "year":
		order = "IFNULL(s.year, 0)"
	case "rating":
		order = "IFNULL(s.rating, 0)"
	}
	if q.Desc {
		order += " DESC"
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 10000
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+showCols+` FROM shows s WHERE `+strings.Join(where, " AND ")+
		` ORDER BY `+order+`, s.id LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Show{}
	for rows.Next() {
		sh, err := scanShow(rows)
		if err != nil {
			return nil, err
		}
		if sh.EpisodeCount > 0 {
			out = append(out, sh)
		}
	}
	return out, rows.Err()
}

func (s *Store) Show(ctx context.Context, id, userID int64) (*Show, error) {
	sh, err := scanShow(s.DB.QueryRowContext(ctx, `SELECT `+showCols+` FROM shows s WHERE s.id = ?`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return sh, err
}

func (s *Store) showByPath(ctx context.Context, libraryID int64, path string) (*Show, error) {
	sh, err := scanShow(s.DB.QueryRowContext(ctx, `SELECT `+showCols+` FROM shows s WHERE s.library_id = ? AND s.path = ?`, 0, libraryID, path))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return sh, err
}

// UpsertShow inserts or updates a show (and clears its missing flag).
func (s *Store) UpsertShow(ctx context.Context, sh *Show) error {
	now := db.Now()
	if sh.SortTitle == "" {
		sh.SortTitle = SortTitle(sh.Title)
	}
	if sh.ID == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO shows(library_id, path, title, sort_title, year, overview, genres, rating, content_rating,
			network, status, poster, backdrop, provider, provider_id, added_at, updated_at, missing, intro_start, intro_end) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,?,?)`,
			sh.LibraryID, sh.Path, sh.Title, sh.SortTitle, sh.Year, sh.Overview, joinList(sh.Genres), sh.Rating, sh.ContentRating,
			sh.Network, sh.Status, sh.Poster, sh.Backdrop, sh.Provider, sh.ProviderID, now, now, sh.IntroStart, sh.IntroEnd)
		if err != nil {
			return err
		}
		sh.ID, _ = res.LastInsertId()
		sh.AddedAt, sh.UpdatedAt = now, now
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE shows SET title=?, sort_title=?, year=?, overview=?, genres=?, rating=?, content_rating=?,
		network=?, status=?, poster=?, backdrop=?, provider=?, provider_id=?, updated_at=?, missing=0, intro_start=?, intro_end=? WHERE id=?`,
		sh.Title, sh.SortTitle, sh.Year, sh.Overview, joinList(sh.Genres), sh.Rating, sh.ContentRating,
		sh.Network, sh.Status, sh.Poster, sh.Backdrop, sh.Provider, sh.ProviderID, now, sh.IntroStart, sh.IntroEnd, sh.ID)
	sh.UpdatedAt = now
	return err
}

// Seasons lists the seasons that have episodes, with per-user unwatched counts.
func (s *Store) Seasons(ctx context.Context, showID, userID int64) ([]Season, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT IFNULL(i.season, 0), COUNT(*),
		SUM(CASE WHEN EXISTS (SELECT 1 FROM progress p WHERE p.item_id = i.id AND p.user_id = ? AND p.watched = 1) THEN 0 ELSE 1 END)
		FROM items i WHERE i.show_id = ? AND i.missing = 0 AND i.season IS NOT NULL GROUP BY i.season ORDER BY 1`, userID, showID)
	if err != nil {
		return nil, err
	}
	var out []Season
	for rows.Next() {
		var se Season
		if err := rows.Scan(&se.Number, &se.EpisodeCount, &se.Unwatched); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, se)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	art := map[int]Season{}
	rows, err = s.DB.QueryContext(ctx, `SELECT number, IFNULL(title,''), IFNULL(overview,''), IFNULL(poster,'') FROM seasons WHERE show_id = ?`, showID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var se Season
		if err := rows.Scan(&se.Number, &se.Title, &se.Overview, &se.Poster); err != nil {
			rows.Close()
			return nil, err
		}
		art[se.Number] = se
	}
	rows.Close()
	for i := range out {
		a := art[out[i].Number]
		out[i].Title, out[i].Overview, out[i].Poster = a.Title, a.Overview, a.Poster
		if out[i].Title == "" {
			out[i].Title = SeasonName(out[i].Number)
		}
	}
	if out == nil {
		out = []Season{}
	}
	return out, nil
}

// SetShowView stores a show's episode layout (cards | list | "").
func (s *Store) SetShowView(ctx context.Context, id int64, view string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE shows SET view = ? WHERE id = ?`, view, id)
	return err
}

// SeasonName is the display name of a season number.
func SeasonName(n int) string {
	if n == 0 {
		return "Specials"
	}
	return "Season " + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// SeasonPoster returns the stored poster for a season ("" if none).
func (s *Store) SeasonPoster(ctx context.Context, showID int64, n int) string {
	var p sql.NullString
	_ = s.DB.QueryRowContext(ctx, `SELECT poster FROM seasons WHERE show_id = ? AND number = ?`, showID, n).Scan(&p)
	return p.String
}

// SetSeasonPoster records a season poster, keeping a local file over a remote one.
func (s *Store) SetSeasonPoster(ctx context.Context, showID int64, n int, poster string, replace bool) error {
	q := `INSERT INTO seasons(show_id, number, poster) VALUES(?,?,?) ON CONFLICT(show_id, number) DO UPDATE SET poster = excluded.poster`
	if !replace {
		q += ` WHERE IFNULL(seasons.poster, '') = ''`
	}
	_, err := s.DB.ExecContext(ctx, q, showID, n, poster)
	return err
}

// Episodes lists a show's episodes in order.
func (s *Store) Episodes(ctx context.Context, showID int64) ([]*Item, error) {
	items, _, err := s.Items(ctx, ItemQuery{ShowID: showID, Sort: "episode"})
	return items, err
}

// NextUp picks the episode to offer next: an episode in progress, else the
// first unwatched one after the last watched, else the first episode.
// Episodes must be in order and carry progress.
func NextUp(eps []*Item) *Item {
	last := -1
	for i, e := range eps {
		if e.Progress != nil && !e.Progress.Watched && e.Progress.PositionSec > 30 {
			return e
		}
		if e.Progress != nil && e.Progress.Watched {
			last = i
		}
	}
	for i := last + 1; i < len(eps); i++ {
		if eps[i].Progress == nil || !eps[i].Progress.Watched {
			return eps[i]
		}
	}
	if last == -1 && len(eps) > 0 {
		return eps[0]
	}
	return nil
}

// NextEpisode returns the id of the episode after it in its show, if any.
func (s *Store) NextEpisode(ctx context.Context, it *Item) *int64 {
	if it.ShowID == nil {
		return nil
	}
	eps, err := s.Episodes(ctx, *it.ShowID)
	if err != nil {
		return nil
	}
	sort.SliceStable(eps, func(i, j int) bool { return episodeKey(eps[i]) < episodeKey(eps[j]) })
	for i, e := range eps {
		if e.ID == it.ID && i+1 < len(eps) {
			return &eps[i+1].ID
		}
	}
	return nil
}

func episodeKey(it *Item) int {
	k := 0
	if it.Season != nil {
		k = *it.Season * 10000
		if *it.Season == 0 {
			k = 1 << 30 // specials last
		}
	}
	if it.Episode != nil {
		k += *it.Episode
	}
	return k
}

// Recommend suggests what to watch after it: for an episode another show
// (genre overlap first, shows already started before new ones), for a
// movie a similar unwatched movie. Returns nil when nothing fits.
func (s *Store) Recommend(ctx context.Context, userID int64, it *Item) (*Item, *Show, error) {
	genres := it.Genres
	if it.ShowID != nil {
		sh, err := s.Show(ctx, *it.ShowID, userID)
		if err != nil {
			return nil, nil, err
		}
		genres = sh.Genres
		shows, err := s.Shows(ctx, ShowQuery{LibraryID: it.LibraryID, UserID: userID})
		if err != nil {
			return nil, nil, err
		}
		var best *Show
		bestScore := -1
		for _, c := range shows {
			if c.ID == sh.ID || c.Unwatched == 0 {
				continue
			}
			sc := overlap(genres, c.Genres) * 10
			if c.Unwatched < c.EpisodeCount {
				sc += 5 // already started
			}
			if best == nil || sc > bestScore || (sc == bestScore && c.LastAddedAt > best.LastAddedAt) {
				best, bestScore = c, sc
			}
		}
		if best == nil {
			return nil, nil, nil
		}
		eps, err := s.Episodes(ctx, best.ID)
		if err != nil {
			return nil, nil, err
		}
		_ = s.AttachProgress(ctx, userID, eps)
		return NextUp(eps), best, nil
	}
	items, _, err := s.Items(ctx, ItemQuery{LibraryID: it.LibraryID, NoEpisodes: true, Sort: "rating", Desc: true, Limit: 500})
	if err != nil {
		return nil, nil, err
	}
	_ = s.AttachProgress(ctx, userID, items)
	var best *Item
	bestScore := -1.0
	for _, c := range items {
		if c.ID == it.ID || (c.Progress != nil && c.Progress.Watched) {
			continue
		}
		sc := float64(overlap(genres, c.Genres)) * 10
		if c.Rating != nil {
			sc += *c.Rating / 2
		}
		if it.Year != nil && c.Year != nil && abs(*it.Year-*c.Year) <= 5 {
			sc += 2
		}
		if sameSeries(it.Title, c.Title) {
			sc += 30 // sequels and prequels first (Shrek -> Shrek 2)
		}
		if sc > bestScore {
			best, bestScore = c, sc
		}
	}
	return best, nil, nil
}

func overlap(a, b []string) int {
	n := 0
	for _, x := range a {
		for _, y := range b {
			if strings.EqualFold(x, y) {
				n++
			}
		}
	}
	return n
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// sameSeries matches titles that share their first word(s), e.g.
// "Shrek" / "Shrek 2" / "Shrek the Third".
func sameSeries(a, b string) bool {
	fa, fb := strings.Fields(strings.ToLower(SortTitle(a))), strings.Fields(strings.ToLower(SortTitle(b)))
	return len(fa) > 0 && len(fb) > 0 && fa[0] == fb[0] && len(fa[0]) > 3
}

// markShowsMissing hides shows whose episodes are all gone.
func (s *Store) markShowsMissing(ctx context.Context, libraryID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE shows SET missing = CASE WHEN EXISTS
		(SELECT 1 FROM items i WHERE i.show_id = shows.id AND i.missing = 0) THEN 0 ELSE 1 END WHERE library_id = ?`, libraryID)
	return err
}

// Network groups the shows of one TV network (the building block for
// channel-style playback).
type Network struct {
	Name    string   `json:"name"`
	Shows   int      `json:"shows"`
	Genres  []string `json:"genres"`
	ShowIDs []int64  `json:"showIds"`
}

// Networks lists the networks of the shows in a library (0 = all), with the
// genres that appear on each.
func (s *Store) Networks(ctx context.Context, libraryID int64) ([]Network, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, IFNULL(network, ''), IFNULL(genres, '') FROM shows
		WHERE missing = 0 AND (? = 0 OR library_id = ?) ORDER BY network COLLATE NOCASE, sort_title COLLATE NOCASE`, libraryID, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byName := map[string]*Network{}
	var order []string
	for rows.Next() {
		var id int64
		var name, genres string
		if err := rows.Scan(&id, &name, &genres); err != nil {
			return nil, err
		}
		if name == "" {
			name = "Unknown network"
		}
		n := byName[name]
		if n == nil {
			n = &Network{Name: name, Genres: []string{}, ShowIDs: []int64{}}
			byName[name] = n
			order = append(order, name)
		}
		n.Shows++
		n.ShowIDs = append(n.ShowIDs, id)
		for _, g := range splitList(genres) {
			if !containsFold(n.Genres, g) {
				n.Genres = append(n.Genres, g)
			}
		}
	}
	out := make([]Network, 0, len(order))
	for _, name := range order {
		sortStrings(byName[name].Genres)
		out = append(out, *byName[name])
	}
	return out, rows.Err()
}

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

// ShowGenres lists the genres used by shows in a library.
func (s *Store) ShowGenres(ctx context.Context, libraryID int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT genres FROM shows WHERE missing = 0 AND library_id = ?`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[string]bool{}
	for rows.Next() {
		var g sql.NullString
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		for _, x := range splitList(g.String) {
			set[x] = true
		}
	}
	out := make([]string, 0, len(set))
	for g := range set {
		out = append(out, g)
	}
	sortStrings(out)
	return out, nil
}
