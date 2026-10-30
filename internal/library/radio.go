package library

import (
	"bufio"
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// Internet radio: stations are items of kind "radio" whose path is their stream
// address. The server only contacts a station while someone is listening to it,
// and the owner adds every address; there is no directory to phone home to.

// ValidStreamURL keeps web addresses with a host.
func ValidStreamURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && len(raw) <= 2000
}

// RadioStation is a station to add.
type RadioStation struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Genre string `json:"genre"`
}

// AddRadioStation saves a station. Adding the same address again renames it.
func (s *Store) AddRadioStation(ctx context.Context, libraryID int64, st RadioStation) (*Item, error) {
	st.Name, st.URL, st.Genre = strings.TrimSpace(st.Name), strings.TrimSpace(st.URL), strings.TrimSpace(st.Genre)
	if !ValidStreamURL(st.URL) {
		return nil, ErrBadInput("a station needs a web address (http or https)")
	}
	if st.Name == "" {
		u, _ := url.Parse(st.URL)
		st.Name = u.Host
	}
	if len(st.Name) > 200 || len(st.Genre) > 80 {
		return nil, ErrBadInput("that name is too long")
	}
	it := &Item{LibraryID: libraryID, Kind: "radio", Path: st.URL, Title: st.Name, SortTitle: SortTitle(st.Name), Container: "radio"}
	if st.Genre != "" {
		it.Genres = []string{st.Genre}
	}
	if existing, err := s.itemByPath(ctx, libraryID, st.URL); err == nil {
		it.ID, it.AddedAt = existing.ID, existing.AddedAt
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if err := s.upsertItem(ctx, it); err != nil {
		return nil, err
	}
	return it, nil
}

// RemoveRadioStation removes a station from a library.
func (s *Store) RemoveRadioStation(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM items WHERE id = ? AND kind = 'radio'`, id)
	return err
}

var (
	extinfRe = regexp.MustCompile(`(?i)^#EXTINF:[^,]*,(.*)$`)
	genreRe  = regexp.MustCompile(`(?i)group-title="([^"]*)"`)
	plsKeyRe = regexp.MustCompile(`(?i)^(file|title)(\d+)=(.*)$`)
)

// maxPlaylistStations bounds one import.
const maxPlaylistStations = 2000

// ParsePlaylist reads stations from an M3U or PLS playlist.
func ParsePlaylist(text string) []RadioStation {
	var out []RadioStation
	pls := map[string]*RadioStation{}
	var order []string
	name, genre := "", ""
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() && len(out) < maxPlaylistStations {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\xef\xbb\xbf"))
		if line == "" {
			continue
		}
		if m := plsKeyRe.FindStringSubmatch(line); m != nil {
			st := pls[m[2]]
			if st == nil {
				st = &RadioStation{}
				pls[m[2]] = st
				order = append(order, m[2])
			}
			if strings.EqualFold(m[1], "file") {
				st.URL = strings.TrimSpace(m[3])
			} else {
				st.Name = strings.TrimSpace(m[3])
			}
			continue
		}
		if m := extinfRe.FindStringSubmatch(line); m != nil {
			name = strings.TrimSpace(m[1])
			if g := genreRe.FindStringSubmatch(line); g != nil {
				genre = strings.TrimSpace(g[1])
			}
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		if ValidStreamURL(line) {
			out = append(out, RadioStation{Name: name, URL: line, Genre: genre})
		}
		name, genre = "", ""
	}
	for _, k := range order {
		if st := pls[k]; ValidStreamURL(st.URL) && len(out) < maxPlaylistStations {
			out = append(out, *st)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Year in review
// ---------------------------------------------------------------------------

// ReviewEntry is one line of a top list.
type ReviewEntry struct {
	ID      int64   `json:"id,omitempty"`
	ShowID  int64   `json:"showId,omitempty"`
	Name    string  `json:"name"`
	Sub     string  `json:"sub,omitempty"`
	Seconds float64 `json:"seconds"`
	Plays   int     `json:"plays,omitempty"`
}

// YearReview is what one account's year looked like. It comes from the account's
// own record on this server, and goes nowhere else.
type YearReview struct {
	Year        int           `json:"year"`
	Seconds     float64       `json:"seconds"`
	Items       int           `json:"items"`
	Days        int           `json:"days"`
	LongestRun  int           `json:"longestRun"`
	BusiestDay  int           `json:"busiestDay,omitempty"`
	BusiestSecs float64       `json:"busiestSeconds,omitempty"`
	ByType      []ReviewEntry `json:"byType"`
	ByMonth     [12]float64   `json:"byMonth"`
	TopTitles   []ReviewEntry `json:"topTitles"`
	TopShows    []ReviewEntry `json:"topShows"`
	TopArtists  []ReviewEntry `json:"topArtists"`
	TopCreators []ReviewEntry `json:"topCreators"`
	TopGenres   []ReviewEntry `json:"topGenres"`
	Years       []int         `json:"years"`
}

// ReviewOfYear adds up an account's play log for a year.
func (s *Store) ReviewOfYear(ctx context.Context, userID int64, year int) (*YearReview, error) {
	r := &YearReview{Year: year, ByType: []ReviewEntry{}, TopTitles: []ReviewEntry{}, TopShows: []ReviewEntry{}, TopArtists: []ReviewEntry{}, TopCreators: []ReviewEntry{}, TopGenres: []ReviewEntry{}, Years: []int{}}
	from, to := year*10000, (year+1)*10000
	yrs, err := s.DB.QueryContext(ctx, `SELECT DISTINCT day / 10000 FROM play_log WHERE user_id = ? ORDER BY 1 DESC`, userID)
	if err != nil {
		return nil, err
	}
	for yrs.Next() {
		var y int
		if yrs.Scan(&y) == nil {
			r.Years = append(r.Years, y)
		}
	}
	yrs.Close()
	// Days and runs.
	days, err := s.DB.QueryContext(ctx, `SELECT day, SUM(seconds) FROM play_log WHERE user_id = ? AND day >= ? AND day < ? GROUP BY day ORDER BY day`, userID, from, to)
	if err != nil {
		return nil, err
	}
	run, prev := 0, 0
	for days.Next() {
		var d int
		var secs float64
		if err := days.Scan(&d, &secs); err != nil {
			days.Close()
			return nil, err
		}
		r.Days++
		r.Seconds += secs
		if m := (d / 100) % 100; m >= 1 && m <= 12 {
			r.ByMonth[m-1] += secs
		}
		if secs > r.BusiestSecs {
			r.BusiestSecs, r.BusiestDay = secs, d
		}
		if prev != 0 && nextDay(prev) == d {
			run++
		} else {
			run = 1
		}
		if run > r.LongestRun {
			r.LongestRun = run
		}
		prev = d
	}
	days.Close()
	if r.Days == 0 {
		return r, nil
	}
	type row struct {
		id, show        int64
		title, typ, sub string
		artist, studio  string
		genres, sTitle  string
		secs            float64
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id, IFNULL(i.show_id, 0), i.title, IFNULL(l.type, ''), IFNULL(i.artist, ''), IFNULL(i.album_artist, ''), IFNULL(i.studio, ''),
		IFNULL(i.genres, ''), IFNULL((SELECT title FROM shows s WHERE s.id = i.show_id), ''), SUM(p.seconds)
		FROM play_log p JOIN items i ON i.id = p.item_id LEFT JOIN libraries l ON l.id = i.library_id
		WHERE p.user_id = ? AND p.day >= ? AND p.day < ? GROUP BY i.id`, userID, from, to)
	if err != nil {
		return nil, err
	}
	byType, byShow, byArtist, byCreator, byGenre := map[string]float64{}, map[int64]*ReviewEntry{}, map[string]float64{}, map[string]float64{}, map[string]float64{}
	var titles []ReviewEntry
	for rows.Next() {
		var x row
		var albumArtist string
		if err := rows.Scan(&x.id, &x.show, &x.title, &x.typ, &x.artist, &albumArtist, &x.studio, &x.genres, &x.sTitle, &x.secs); err != nil {
			rows.Close()
			return nil, err
		}
		r.Items++
		byType[x.typ] += x.secs
		if x.show != 0 {
			e := byShow[x.show]
			if e == nil {
				e = &ReviewEntry{ShowID: x.show, Name: x.sTitle}
				byShow[x.show] = e
			}
			e.Seconds += x.secs
		} else {
			titles = append(titles, ReviewEntry{ID: x.id, Name: x.title, Sub: x.typ, Seconds: x.secs})
		}
		if x.typ == "music" || x.typ == "musicvideos" {
			a := x.artist
			if a == "" {
				a = albumArtist
			}
			if a == "" {
				a = x.studio
			}
			if a != "" {
				byArtist[a] += x.secs
			}
		}
		if (x.typ == "channels" || x.typ == "shorts") && x.studio != "" {
			byCreator[x.studio] += x.secs
		}
		for _, g := range splitList(x.genres) {
			byGenre[g] += x.secs
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	r.ByType = topSeconds(byType, 12)
	r.TopArtists = topSeconds(byArtist, 5)
	r.TopCreators = topSeconds(byCreator, 5)
	r.TopGenres = topSeconds(byGenre, 5)
	for _, e := range byShow {
		r.TopShows = append(r.TopShows, *e)
	}
	sortEntries(r.TopShows)
	if len(r.TopShows) > 5 {
		r.TopShows = r.TopShows[:5]
	}
	sortEntries(titles)
	if len(titles) > 5 {
		titles = titles[:5]
	}
	if titles != nil {
		r.TopTitles = titles
	}
	return r, nil
}

func topSeconds(m map[string]float64, limit int) []ReviewEntry {
	out := make([]ReviewEntry, 0, len(m))
	for k, v := range m {
		out = append(out, ReviewEntry{Name: k, Seconds: v})
	}
	sortEntries(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func sortEntries(e []ReviewEntry) {
	for i := 1; i < len(e); i++ {
		for j := i; j > 0 && (e[j].Seconds > e[j-1].Seconds || (e[j].Seconds == e[j-1].Seconds && e[j].Name < e[j-1].Name)); j-- {
			e[j], e[j-1] = e[j-1], e[j]
		}
	}
}

// nextDay is the YYYYMMDD after a YYYYMMDD.
func nextDay(d int) int {
	y, m, day := d/10000, (d/100)%100, d%100
	last := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	n := 0
	if m >= 1 && m <= 12 {
		n = last[m-1]
	}
	if m == 2 && (y%4 == 0 && (y%100 != 0 || y%400 == 0)) {
		n = 29
	}
	if day < n {
		return d + 1
	}
	if m < 12 {
		return y*10000 + (m+1)*100 + 1
	}
	return (y+1)*10000 + 101
}

// ---------------------------------------------------------------------------
// Recommendations for the home page
// ---------------------------------------------------------------------------

// Recommendation is a film or show for someone, and why.
type Recommendation struct {
	Item   *Item  `json:"item,omitempty"`
	Show   *Show  `json:"show,omitempty"`
	Reason string `json:"reason"`
}

// RecommendForYou picks unwatched films and shows that match the genres this
// account spends its time on. It reads only this account's record on this server.
func (s *Store) RecommendForYou(ctx context.Context, userID int64, access *Access, seed int64, limit int) ([]*Item, []*Show, string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT IFNULL(i.genres, ''), IFNULL((SELECT genres FROM shows sh WHERE sh.id = i.show_id), ''), MAX(p.position_sec, 60), p.watched
		FROM progress p JOIN items i ON i.id = p.item_id WHERE p.user_id = ? ORDER BY p.updated_at DESC LIMIT 400`, userID)
	if err != nil {
		return nil, nil, "", err
	}
	genres := map[string]float64{}
	for rows.Next() {
		var a, b string
		var secs float64
		var watched int
		if err := rows.Scan(&a, &b, &secs, &watched); err != nil {
			rows.Close()
			return nil, nil, "", err
		}
		w := 1.0
		if watched != 0 {
			w = 2
		}
		for _, g := range append(splitList(a), splitList(b)...) {
			genres[g] += w
		}
	}
	rows.Close()
	if len(genres) == 0 {
		return nil, nil, "", nil
	}
	score := func(gs []string, id int64, rating *float64) float64 {
		v := 0.0
		for _, g := range gs {
			v += genres[g]
		}
		if len(gs) > 0 {
			v /= float64(len(gs))
		}
		if rating != nil {
			v += *rating / 10
		}
		return v + jitter(seed, id)
	}
	items, _, err := s.Items(ctx, ItemQuery{Watchable: true, NoEpisodes: true, Sort: "added", Desc: true, Limit: 600, Access: access})
	if err != nil {
		return nil, nil, "", err
	}
	_ = s.AttachProgress(ctx, userID, items)
	type ranked struct {
		it   *Item
		sh   *Show
		rank float64
	}
	var pool []ranked
	for _, it := range items {
		if it.Progress != nil {
			continue // already watched or started
		}
		if v := score(it.Genres, it.ID, it.Rating); v > 1 {
			pool = append(pool, ranked{it: it, rank: v})
		}
	}
	shows, err := s.Shows(ctx, ShowQuery{UserID: userID, Access: access, Sort: "added", Desc: true, Limit: 300})
	if err != nil {
		return nil, nil, "", err
	}
	for _, sh := range shows {
		if sh.EpisodeCount > 0 && sh.Unwatched == sh.EpisodeCount {
			if v := score(sh.Genres, sh.ID+1<<40, sh.Rating); v > 1 {
				pool = append(pool, ranked{sh: sh, rank: v})
			}
		}
	}
	for i := 1; i < len(pool); i++ {
		for j := i; j > 0 && pool[j].rank > pool[j-1].rank; j-- {
			pool[j], pool[j-1] = pool[j-1], pool[j]
		}
	}
	if len(pool) > limit {
		pool = pool[:limit]
	}
	outItems, outShows := []*Item{}, []*Show{}
	for _, p := range pool {
		if p.it != nil {
			outItems = append(outItems, p.it)
		} else {
			outShows = append(outShows, p.sh)
		}
	}
	top := topKeys(func() map[string]int {
		m := map[string]int{}
		for k, v := range genres {
			m[k] = int(v * 100)
		}
		return m
	}(), 2)
	return outItems, outShows, strings.Join(top, " and "), nil
}

// recordRadio notes when a station was last used.
func (s *Store) recordRadio(ctx context.Context, id int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE items SET updated_at = ? WHERE id = ?`, db.Now(), id)
}

// TouchRadio marks a station as just listened to.
func (s *Store) TouchRadio(ctx context.Context, id int64) { s.recordRadio(ctx, id) }
