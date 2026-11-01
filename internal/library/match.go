package library

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// Matching: searching every source at once, saying how sure it is and why, showing
// what would change before anything does, and remembering what a person decided.
// Everything a source answers is treated as hostile: see cleanDetails.

// MatchCandidate is one search result.
type MatchCandidate struct {
	Source   string   `json:"source"`
	ID       string   `json:"id"`
	Kind     string   `json:"kind"` // movie | show
	Title    string   `json:"title"`
	Year     *int     `json:"year,omitempty"`
	Rating   *float64 `json:"rating,omitempty"`
	Overview string   `json:"overview,omitempty"`
	Poster   string   `json:"poster,omitempty"`
	URL      string   `json:"url,omitempty"`
	// Score is how likely this is the right one, from 0 to 100, and Reasons say why.
	Score   int      `json:"score"`
	Reasons []string `json:"reasons,omitempty"`
}

// MatchDetails is everything a source knows about one title.
type MatchDetails struct {
	Source, ID, Kind, URL string

	Title         string
	Year          *int
	Overview      string
	Tagline       string
	Genres        []string
	Rating        *float64
	ContentRating string
	Studios       []string
	Network       string
	Director      string
	Status        string
	RuntimeMin    int

	Poster, Backdrop, Banner, Logo     string
	Posters, Backdrops, Banners, Logos []string
	SeasonPosters                      map[int]string
	AirDays                            []string
	AirTime                            string
}

// MatchSource is a service titles can be searched for on.
type MatchSource interface {
	Name() string
	Label() string
	Search(ctx context.Context, query string, year *int) ([]MatchCandidate, error)
	Details(ctx context.Context, id string) (*MatchDetails, error)
}

// SeriesLister is a source that also knows a show's episodes.
type SeriesLister interface {
	Episodes(ctx context.Context, id string) (map[[2]int]EpisodeMeta, error)
}

// SourceLabel is a service's name for people.
func SourceLabel(src string) string {
	switch src {
	case "tmdb", "tmdb-tv":
		return "TMDB"
	case "tvdb":
		return "TheTVDB"
	case "tvmaze":
		return "TVmaze"
	case "anilist":
		return "AniList"
	case "wikidata":
		return "Wikidata"
	case SourceYou:
		return "You"
	case SourceFile:
		return "File name"
	case SourceNFO:
		return "NFO file"
	case SourceLocal:
		return "Beside the file"
	case SourceFrame:
		return "From the video"
	case SourceProbe:
		return "The file itself"
	}
	if strings.HasPrefix(src, "agent:") {
		return "Custom agent"
	}
	return src
}

// SourceURL is a title's page on a service. Only known addresses are built, from
// an id that's been checked, so a link can't be pointed anywhere else.
func SourceURL(src, id string) string {
	if id == "" || !providerIDRe.MatchString(id) {
		return ""
	}
	switch src {
	case "tmdb":
		return "https://www.themoviedb.org/movie/" + id
	case "tmdb-tv":
		return "https://www.themoviedb.org/tv/" + id
	case "tvdb":
		return "https://thetvdb.com/?tab=series&id=" + id
	case "tvmaze":
		return "https://www.tvmaze.com/shows/" + id
	case "anilist":
		return "https://anilist.co/anime/" + id
	case "wikidata":
		return "https://www.wikidata.org/wiki/" + id
	}
	return ""
}

var providerIDRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,80}$`)

// cleanDetails bounds and tidies what a source said, whoever it is. Only web addresses
// survive as pictures, so an answer can't point the server at a file on this machine.
func cleanDetails(d *MatchDetails) *MatchDetails {
	d.Title = cleanText(plain(d.Title), 300, false)
	d.Overview = cleanText(plain(d.Overview), 20000, true)
	d.Tagline = cleanText(plain(d.Tagline), 500, false)
	d.Genres = cleanList(d.Genres, 30, 60)
	d.Studios = cleanList(d.Studios, 12, 120)
	d.Network = cleanText(d.Network, 120, false)
	d.Director = cleanText(d.Director, 300, false)
	d.Status = cleanText(d.Status, 40, false)
	d.ContentRating = cleanText(d.ContentRating, 20, false)
	if d.Rating != nil && (*d.Rating < 0 || *d.Rating > 10 || *d.Rating != *d.Rating) {
		d.Rating = nil
	}
	if d.Year != nil && (*d.Year < 1800 || *d.Year > 3000) {
		d.Year = nil
	}
	d.Poster, d.Backdrop, d.Banner, d.Logo = webURL(d.Poster), webURL(d.Backdrop), webURL(d.Banner), webURL(d.Logo)
	webOnly := func(l []string) []string {
		out := make([]string, 0, len(l))
		for _, u := range l {
			if v := webURL(u); v != "" && len(out) < 60 {
				out = append(out, v)
			}
		}
		return out
	}
	d.Posters, d.Backdrops, d.Banners, d.Logos = webOnly(d.Posters), webOnly(d.Backdrops), webOnly(d.Banners), webOnly(d.Logos)
	return d
}

func cleanCandidate(c *MatchCandidate) {
	c.Title = cleanText(plain(c.Title), 300, false)
	c.Overview = clip(cleanText(plain(c.Overview), 600, false), 600)
	c.Poster = webURL(c.Poster)
	if c.Rating != nil && (*c.Rating <= 0 || *c.Rating > 10) {
		c.Rating = nil
	}
	if c.Year != nil && (*c.Year < 1800 || *c.Year > 3000) {
		c.Year = nil
	}
	if c.URL = SourceURL(c.Source, c.ID); c.URL == "" {
		c.URL = ""
	}
}

// ---------------------------------------------------------------------------
// Comparing names
// ---------------------------------------------------------------------------

var romanWords = map[string]string{"ii": "2", "iii": "3", "iv": "4", "v": "5", "vi": "6", "vii": "7", "viii": "8", "ix": "9", "x": "10", "two": "2", "three": "3", "four": "4", "five": "5", "second": "2", "third": "3", "fourth": "4", "fifth": "5", "part": "", "chapter": "", "vol": "", "volume": ""}

// titleWords breaks a title into comparable words: "SHREK_THE_THIRD" and
// "Shrek the Third" are the same, and the numbers in sequels are noticed.
func titleWords(s string) []string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
		case r == '\'' || r == '’':
		default:
			b.WriteRune(' ')
		}
	}
	var out []string
	for _, w := range strings.Fields(b.String()) {
		if w == "the" || w == "a" || w == "an" || w == "and" || w == "of" {
			continue
		}
		if n, ok := romanWords[w]; ok {
			if n != "" {
				out = append(out, n)
			}
			continue
		}
		out = append(out, w)
	}
	return out
}

func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			c := 1
			if a[i-1] == b[j-1] {
				c = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+c)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// numbersDiffer: "Tale 2" isn't "Tale 3" however alike the rest is.
func numbersDiffer(a, b []string) bool {
	nums := func(w []string) string {
		var n []string
		for _, x := range w {
			if _, err := strconv.Atoi(x); err == nil && len(x) < 4 {
				n = append(n, x)
			}
		}
		return strings.Join(n, " ")
	}
	return nums(a) != nums(b)
}

// TitleAlike says how alike two titles are, from 0 to 1.
func TitleAlike(a, b string) float64 {
	wa, wb := titleWords(a), titleWords(b)
	if len(wa) == 0 || len(wb) == 0 {
		return 0
	}
	ja, jb := strings.Join(wa, " "), strings.Join(wb, " ")
	if ja == jb {
		return 1
	}
	ra, rb := []rune(ja), []rune(jb)
	ratio := 1 - float64(levenshtein(ra, rb))/float64(max(len(ra), len(rb)))
	// One name inside the other ("Dr Seuss The Lorax" and "The Lorax").
	set := map[string]bool{}
	for _, w := range wb {
		set[w] = true
	}
	shared := 0
	for _, w := range wa {
		if set[w] {
			shared++
		}
	}
	contain := float64(shared) / float64(min(len(wa), len(wb)))
	overlap := contain * 0.85
	if len(wa) == 1 || len(wb) == 1 {
		overlap = contain * 0.6 // a single word isn't much to go on
	}
	s := ratio
	if overlap > s {
		s = overlap
	}
	if numbersDiffer(wa, wb) && s > 0.55 {
		s = 0.55
	}
	return s
}

// scoreMatch rates a result against what's known about a title, with reasons in
// words a person can check for themselves.
func scoreMatch(wantTitle string, wantYear *int, libType string, title string, year *int, kind string) (int, []string) {
	alike := TitleAlike(wantTitle, title)
	score := alike * 70
	var why []string
	switch {
	case alike >= 0.995:
		why = append(why, "The name is exactly the same")
	case alike >= 0.8:
		why = append(why, "The name is very close")
	case alike >= 0.5:
		why = append(why, "The name is only partly the same")
	default:
		why = append(why, "The name is quite different")
	}
	switch {
	case wantYear == nil && year == nil:
		score += 12
	case wantYear == nil:
		score += 15
		why = append(why, fmt.Sprintf("It came out in %d. The file gives no year to check against", *year))
	case year == nil:
		score += 10
		why = append(why, "This result gives no year to check against")
	case *wantYear == *year:
		score += 30
		why = append(why, fmt.Sprintf("Same year (%d)", *year))
	case abs(*wantYear-*year) == 1:
		score += 20
		why = append(why, fmt.Sprintf("One year off: the file says %d, this says %d", *wantYear, *year))
	default:
		why = append(why, fmt.Sprintf("Different year: the file says %d, this says %d", *wantYear, *year))
	}
	film := libType == "movies" || libType == "discs"
	tv := libType == "shows" || libType == "anime"
	if (film && kind == "show") || (tv && kind == "movie") {
		score -= 15
		what := "a film"
		lib := "TV"
		if kind == "show" {
			what, lib = "a show", "film"
		}
		why = append(why, fmt.Sprintf("It's %s, and this is a %s library", what, lib))
	}
	return max(0, min(100, int(score+0.5))), why
}

// fileNameOf is what a title's file or folder says it is, without what matching may have changed.
func fileNameOf(t *Title) (string, *int) {
	p := t.Path()
	if t.Item != nil && !strings.HasSuffix(t.Item.Kind, "-folder") {
		p = strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	} else {
		p = filepath.Base(p)
	}
	return ParseName(stripIDTags(p))
}

// ConfidenceFor rates a title's current match from what's stored, without asking
// anyone: it's run over a whole library to find what needs a look.
func ConfidenceFor(t *Title, libType string) (int, []string) {
	switch t.Meta.MatchState {
	case MatchManual:
		return 100, []string{"You chose this match"}
	case MatchNone:
		return 100, []string{"You said this has no match"}
	}
	provider, _ := t.Provider()
	if provider == "" {
		return 0, []string{"Nothing was found for this"}
	}
	if strings.HasPrefix(provider, "agent:") {
		return 100, []string{"Your own agent answered"}
	}
	name, year := fileNameOf(t)
	cur := t.Name()
	var curYear *int
	if t.Item != nil {
		curYear = t.Item.Year
	} else {
		curYear = t.Show.Year
	}
	kind := "movie"
	if t.Show != nil {
		kind = "show"
	}
	score, why := scoreMatch(name, year, libType, cur, curYear, kind)
	if t.Meta.Sources["title"] == SourceNFO {
		score = max(score, 85)
		why = append(why, "The name comes from an NFO file beside it")
	}
	if t.Meta.Sources["title"] == SourceYou {
		score = max(score, 85)
		why = append(why, "You set the name yourself")
	}
	return score, why
}

// ---------------------------------------------------------------------------
// Sources
// ---------------------------------------------------------------------------

// matchSources lists what to search for a title, in the order they're believed.
func (s *Scanner) matchSources(t *Title, libType string) []MatchSource {
	var out []MatchSource
	key := ""
	if s.TMDBKey != nil {
		key = s.TMDBKey()
	}
	tvdbKey, pin := "", ""
	if s.TVDBKey != nil {
		tvdbKey, pin = s.TVDBKey()
	}
	if t.Item != nil && s.Agents != nil {
		for i := range s.Agents() {
			a := s.Agents()[i]
			if a.Serves(libType) {
				out = append(out, &agentSource{a: a, libType: libType})
			}
		}
	}
	if t.Item != nil {
		if key != "" {
			out = append(out, &tmdbMovies{TMDB{Key: key, Base: s.TMDBBase}})
		} else if s.WikidataOn == nil || s.WikidataOn() {
			out = append(out, &wikidataSource{Wikidata{}})
		}
	}
	if key != "" {
		out = append(out, &tmdbShows{TMDB{Key: key, Base: s.TMDBBase}})
	}
	if tvdbKey != "" {
		out = append(out, &tvdbSource{TVDB{Key: tvdbKey, PIN: pin}})
	}
	if s.TVMazeOn == nil || s.TVMazeOn() {
		out = append(out, &tvmazeSource{TVMaze{}})
	}
	if libType == "anime" {
		out = append(out, &anilistSource{s.anilist()})
	}
	return out
}

// matchSourceNamed finds a source by its name, for applying a choice or a refresh.
func (s *Scanner) matchSourceNamed(name string) MatchSource {
	key := ""
	if s.TMDBKey != nil {
		key = s.TMDBKey()
	}
	tvdbKey, pin := "", ""
	if s.TVDBKey != nil {
		tvdbKey, pin = s.TVDBKey()
	}
	switch name {
	case "tmdb":
		if key != "" {
			return &tmdbMovies{TMDB{Key: key, Base: s.TMDBBase}}
		}
	case "tmdb-tv":
		if key != "" {
			return &tmdbShows{TMDB{Key: key, Base: s.TMDBBase}}
		}
	case "tvdb":
		if tvdbKey != "" {
			return &tvdbSource{TVDB{Key: tvdbKey, PIN: pin}}
		}
	case "tvmaze":
		if s.TVMazeOn == nil || s.TVMazeOn() {
			return &tvmazeSource{TVMaze{}}
		}
	case "wikidata":
		if s.WikidataOn == nil || s.WikidataOn() {
			return &wikidataSource{Wikidata{}}
		}
	case "anilist":
		return &anilistSource{s.anilist()}
	}
	if strings.HasPrefix(name, "agent:") && s.Agents != nil {
		for _, a := range s.Agents() {
			if "agent:"+a.ID == name {
				return &agentSource{a: a, libType: "movies"}
			}
		}
	}
	return nil
}

// MatchRef is an exact reference someone typed: an id or an address.
type MatchRef struct {
	Source, ID string
}

var (
	imdbRe    = regexp.MustCompile(`^tt\d{5,10}$`)
	wikiIDRe  = regexp.MustCompile(`^Q\d{1,12}$`)
	refPrefix = regexp.MustCompile(`(?i)^(tmdb|tmdb-tv|tmdbtv|tv|tvdb|thetvdb|tvmaze|anilist|imdb|wikidata)[ :=-]+\s*(\S+)$`)
	urlNum    = regexp.MustCompile(`^\d+`)
)

// ParseMatchRef recognises a TMDB, IMDb, TheTVDB, TVmaze, AniList or Wikidata id or address.
func ParseMatchRef(q string) (MatchRef, bool) {
	q = strings.TrimSpace(q)
	if q == "" || len(q) > 500 {
		return MatchRef{}, false
	}
	if imdbRe.MatchString(q) {
		return MatchRef{"imdb", q}, true
	}
	if wikiIDRe.MatchString(q) {
		return MatchRef{"wikidata", q}, true
	}
	if m := refPrefix.FindStringSubmatch(q); m != nil {
		src := strings.ToLower(m[1])
		switch src {
		case "thetvdb":
			src = "tvdb"
		case "tmdbtv", "tv":
			src = "tmdb-tv"
		}
		if providerIDRe.MatchString(m[2]) {
			return MatchRef{src, m[2]}, true
		}
		return MatchRef{}, false
	}
	if !strings.HasPrefix(strings.ToLower(q), "http://") && !strings.HasPrefix(strings.ToLower(q), "https://") {
		return MatchRef{}, false
	}
	u, err := url.Parse(q)
	if err != nil {
		return MatchRef{}, false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	seg := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}
	switch host {
	case "themoviedb.org":
		// Addresses can start with a language: /en/movie/123-a-name is rare, but it happens.
		if len(parts) > 2 && len(parts[0]) <= 5 {
			parts = parts[1:]
		}
		if id := urlNum.FindString(seg(1)); id != "" {
			switch seg(0) {
			case "movie":
				return MatchRef{"tmdb", id}, true
			case "tv":
				return MatchRef{"tmdb-tv", id}, true
			}
		}
	case "imdb.com", "m.imdb.com":
		for i := range parts {
			if imdbRe.MatchString(parts[i]) {
				return MatchRef{"imdb", parts[i]}, true
			}
		}
	case "tvmaze.com":
		if seg(0) == "shows" {
			if id := urlNum.FindString(seg(1)); id != "" {
				return MatchRef{"tvmaze", id}, true
			}
		}
	case "thetvdb.com":
		if id := urlNum.FindString(u.Query().Get("id")); id != "" {
			return MatchRef{"tvdb", id}, true
		}
		if seg(0) == "series" && seg(1) != "" && providerIDRe.MatchString(seg(1)) {
			return MatchRef{"tvdb", seg(1)}, true
		}
	case "anilist.co":
		if seg(0) == "anime" {
			if id := urlNum.FindString(seg(1)); id != "" {
				return MatchRef{"anilist", id}, true
			}
		}
	case "wikidata.org":
		for i := range parts {
			if wikiIDRe.MatchString(parts[i]) {
				return MatchRef{"wikidata", parts[i]}, true
			}
		}
	}
	return MatchRef{}, false
}

// SearchResult is the answer to a search.
type SearchResult struct {
	Results []MatchCandidate `json:"results"`
	// Warnings: sources that couldn't answer, so an empty list isn't taken for "nothing exists".
	Warnings []string `json:"warnings"`
	// Sources: who was asked, for saying so.
	Sources []string `json:"sources"`
	Exact   bool     `json:"exact"`
}

// libTypeOf is the type of a title's library.
func (s *Scanner) libTypeOf(ctx context.Context, t *Title) string {
	lib, err := s.Store.Library(ctx, t.LibraryID())
	if err != nil {
		return ""
	}
	return lib.Type
}

// SearchMatches searches every source for a title, or finds the exact thing if the
// query is an id or an address. An empty query uses the file's name. Sources are asked
// at the same time and each has its own time limit, so one being slow only costs its own answers.
func (s *Scanner) SearchMatches(ctx context.Context, t *Title, query string, year *int) (*SearchResult, error) {
	libType := s.libTypeOf(ctx, t)
	wantName, wantYear := fileNameOf(t)
	if t.Meta.Sources["title"] == SourceYou || t.Meta.Sources["title"] == SourceNFO {
		wantName = t.Name()
	}
	query = strings.TrimSpace(query)
	if year != nil {
		wantYear = year
	}
	res := &SearchResult{Results: []MatchCandidate{}, Warnings: []string{}, Sources: []string{}}
	if ref, ok := ParseMatchRef(query); ok {
		res.Exact = true
		c, err := s.resolveRef(ctx, ref, t)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				res.Warnings = append(res.Warnings, "Nothing has that id.")
				return res, nil
			}
			res.Warnings = append(res.Warnings, err.Error())
			return res, nil
		}
		res.Results = append(res.Results, c...)
		for i := range res.Results {
			res.Results[i].Score, res.Results[i].Reasons = 100, []string{"You asked for exactly this"}
		}
		return res, nil
	}
	if query == "" {
		query = wantName
	} else {
		// Someone typing a name knows what they want, so what they typed is what's compared.
		if q, y := ParseName(query); q != "" {
			query = q
			if y != nil {
				wantYear = y
			}
		}
		wantName = query
	}
	if query == "" {
		return res, nil
	}
	sources := s.matchSources(t, libType)
	type answer struct {
		src  MatchSource
		list []MatchCandidate
		err  error
	}
	answers := make([]answer, len(sources))
	var wg sync.WaitGroup
	for i, src := range sources {
		res.Sources = append(res.Sources, src.Label())
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if p := recover(); p != nil {
					answers[i] = answer{src: src, err: fmt.Errorf("%v", p)}
				}
			}()
			sctx, cancel := context.WithTimeout(ctx, 25*time.Second)
			defer cancel()
			list, err := src.Search(sctx, query, wantYear)
			answers[i] = answer{src, list, err}
		}()
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, a := range answers {
		if a.err != nil && !errors.Is(a.err, ErrNotFound) {
			res.Warnings = append(res.Warnings, a.src.Label()+" couldn't answer just now.")
			continue
		}
		for _, c := range a.list {
			if len(res.Results) >= 60 {
				break
			}
			cleanCandidate(&c)
			k := c.Source + "|" + c.ID
			if c.Title == "" || seen[k] {
				continue
			}
			seen[k] = true
			c.Score, c.Reasons = scoreMatch(wantName, wantYear, libType, c.Title, c.Year, c.Kind)
			res.Results = append(res.Results, c)
		}
	}
	sort.SliceStable(res.Results, func(i, j int) bool { return res.Results[i].Score > res.Results[j].Score })
	return res, nil
}

// resolveRef finds what an id or address means.
func (s *Scanner) resolveRef(ctx context.Context, ref MatchRef, t *Title) ([]MatchCandidate, error) {
	if ref.Source == "imdb" {
		return s.findIMDb(ctx, ref.ID, t)
	}
	src := s.matchSourceNamed(ref.Source)
	if ref.Source == "tmdb" && t.Show != nil {
		// A bare TMDB number on a show means the show of that number.
		src = s.matchSourceNamed("tmdb-tv")
	}
	if src == nil {
		return nil, fmt.Errorf("%s isn't available here: add its key or turn it on in Settings, Metadata agents", SourceLabel(ref.Source))
	}
	d, err := src.Details(ctx, ref.ID)
	if err != nil {
		return nil, err
	}
	return []MatchCandidate{candidateOf(d)}, nil
}

func candidateOf(d *MatchDetails) MatchCandidate {
	cleanDetails(d)
	c := MatchCandidate{Source: d.Source, ID: d.ID, Kind: d.Kind, Title: d.Title, Year: d.Year, Rating: d.Rating, Overview: d.Overview, Poster: d.Poster}
	cleanCandidate(&c)
	return c
}

func (s *Scanner) findIMDb(ctx context.Context, id string, t *Title) ([]MatchCandidate, error) {
	key := ""
	if s.TMDBKey != nil {
		key = s.TMDBKey()
	}
	var out []MatchCandidate
	if key != "" {
		tm := &TMDB{Key: key, Base: s.TMDBBase}
		if list, err := tm.findExternal(ctx, id); err == nil {
			out = append(out, list...)
		}
	}
	if len(out) == 0 && t.Show != nil && (s.TVMazeOn == nil || s.TVMazeOn()) {
		var sh mazeShow
		if err := (&TVMaze{}).get(ctx, "/lookup/shows?imdb="+url.QueryEscape(id), &sh); err == nil && sh.ID != 0 {
			out = append(out, MatchCandidate{Source: "tvmaze", ID: strconv.Itoa(sh.ID), Kind: "show", Title: sh.Name, Year: yearOf(sh.Premiered), Overview: plain(sh.Summary), Poster: sh.Image.best()})
		}
	}
	if len(out) == 0 && t.Item != nil && (s.WikidataOn == nil || s.WikidataOn()) {
		w := &wikidataSource{Wikidata{}}
		if q, err := w.Wikidata.byIMDb(ctx, id); err == nil && q != "" {
			if d, err := w.Details(ctx, q); err == nil {
				out = append(out, candidateOf(d))
			}
		}
	}
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	for i := range out {
		cleanCandidate(&out[i])
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// What would change, and changing it
// ---------------------------------------------------------------------------

// FieldChange is one row of "here's what would change".
type FieldChange struct {
	Field    string `json:"field"`
	Label    string `json:"label"`
	Current  string `json:"current"`
	Incoming string `json:"incoming"`
	Source   string `json:"source"`
	Locked   bool   `json:"locked"`
	// Change says it differs; Apply says it would be changed unless someone unticks it.
	Change bool   `json:"change"`
	Apply  bool   `json:"apply"`
	Note   string `json:"note,omitempty"`
}

var fieldLabels = map[string]string{"title": "Title", "sortTitle": "Sort name", "year": "Year", "director": "Director", "edition": "Edition", "tagline": "Tagline", "overview": "Overview",
	"genres": "Genres", "tags": "Tags", "contentRating": "Content rating", "rating": "Rating", "studios": "Studios", "network": "Network", "status": "Status",
	"poster": "Poster", "backdrop": "Backdrop", "banner": "Banner", "logo": "Logo", "introStart": "Intro starts", "introEnd": "Intro ends", "season": "Season", "episode": "Episode", "episodeEnd": "Last episode", "aired": "Aired"}

// FieldLabel is a field's name for people.
func FieldLabel(f string) string { return fieldLabels[f] }

func (d *MatchDetails) fields() map[string]string {
	m := map[string]string{
		"title": d.Title, "overview": d.Overview, "tagline": d.Tagline, "genres": joinList(d.Genres), "contentRating": d.ContentRating, "rating": floatStr(d.Rating),
		"studios": joinList(d.Studios), "network": d.Network, "director": d.Director, "status": d.Status, "year": intStr(d.Year),
		"poster": d.Poster, "backdrop": d.Backdrop, "banner": d.Banner, "logo": d.Logo,
	}
	if d.Title != "" {
		m["sortTitle"] = SortTitle(d.Title)
	}
	return m
}

func isArtField(f string) bool {
	return f == "poster" || f == "backdrop" || f == "banner" || f == "logo"
}

// DiffDetails compares what a source says with what a title has. Locked fields
// are shown so nobody wonders why they didn't change. Text someone typed, an NFO and
// pictures beside the file aren't replaced unless someone ticks them.
func DiffDetails(t *Title, d *MatchDetails) []FieldChange {
	in := d.fields()
	out := []FieldChange{}
	for _, f := range t.FieldNames() {
		incoming, known := in[f]
		if !known {
			continue
		}
		cur := t.Field(f)
		src := t.sourceOf(f)
		c := FieldChange{Field: f, Label: FieldLabel(f), Current: cur, Incoming: incoming, Source: src, Locked: t.Meta.Locks[f], Change: cur != incoming}
		if !c.Change {
			continue
		}
		switch {
		case c.Locked:
			c.Note = "Locked, so it stays"
		case incoming == "" && !isServiceSource(src):
			continue // the source has nothing, and what's here isn't from a service
		case incoming == "":
			c.Apply, c.Note = true, "This source has nothing here, so what the last match gave goes"
		case isArtField(f) && (src == SourceLocal || src == SourceYou):
			c.Note = "A picture of yours. Tick it to replace it"
		case src == SourceYou:
			c.Note = "You typed this. Tick it to replace it"
		case src == SourceNFO:
			c.Note = "From an NFO file. Tick it to replace it"
		default:
			c.Apply = true
		}
		out = append(out, c)
	}
	return out
}

// applyDetails changes a title to what a source says. only, if not nil, names the
// fields to change, ticked or unticked; nil takes the defaults DiffDetails gives.
// Locked fields never change either way.
func applyDetails(t *Title, d *MatchDetails, only map[string]bool) error {
	for _, c := range DiffDetails(t, d) {
		if c.Locked {
			continue
		}
		if only != nil && !only[c.Field] {
			continue
		}
		if only == nil && !c.Apply {
			continue
		}
		if err := t.SetField(c.Field, c.Incoming); err != nil {
			continue // a source's nonsense doesn't stop the rest
		}
		if c.Incoming == "" {
			delete(t.Meta.Sources, c.Field)
		} else {
			t.Meta.Sources[c.Field] = d.Source
		}
		delete(t.Meta.Baseline, c.Field)
	}
	t.setProvider(d.Source, d.ID)
	if t.Item != nil && t.Item.RuntimeSec == 0 && d.RuntimeMin > 0 && d.RuntimeMin < 10000 {
		t.Item.RuntimeSec = float64(d.RuntimeMin) * 60
	}
	if t.Item != nil && t.Item.Disc != nil && !t.Item.Disc.ContentManual {
		if d.Kind == "show" {
			t.Item.Disc.Content = "tv"
		} else if d.Kind == "movie" {
			t.Item.Disc.Content = "movie"
		}
	}
	if t.Show != nil {
		t.Show.airDays = d.AirDays
		at := d.AirTime
		t.Show.airTime = &at
	}
	return nil
}

// MatchPreview says what choosing a result would do.
type MatchPreview struct {
	Source    string        `json:"source"`
	ID        string        `json:"id"`
	Kind      string        `json:"kind"`
	URL       string        `json:"url,omitempty"`
	Label     string        `json:"sourceLabel"`
	Title     string        `json:"title"`
	Changes   []FieldChange `json:"changes"`
	Unchanged int           `json:"unchanged"`
	// Episodes: for a show, how many of its episodes the source knows.
	Episodes *EpisodePreview `json:"episodes,omitempty"`
}

// EpisodePreview says what a match does for a show's episodes.
type EpisodePreview struct {
	Total   int      `json:"total"`
	Known   int      `json:"known"`
	Unknown []string `json:"unknown"` // the numbers it doesn't know, for fixing them
}

func (s *Scanner) detailsOf(ctx context.Context, source, id string) (*MatchDetails, error) {
	if !providerIDRe.MatchString(id) {
		return nil, errors.New("that isn't an id")
	}
	src := s.matchSourceNamed(source)
	if src == nil {
		return nil, fmt.Errorf("%s isn't available here: add its key or turn it on in Settings, Metadata agents", SourceLabel(source))
	}
	d, err := src.Details(ctx, id)
	if err != nil {
		return nil, err
	}
	return cleanDetails(d), nil
}

// PreviewMatch says what choosing this would change, without changing anything.
func (s *Scanner) PreviewMatch(ctx context.Context, t *Title, source, id string) (*MatchPreview, error) {
	if t.Show != nil && (source == "tmdb" || source == "wikidata") {
		return nil, errors.New("a show needs a show from TMDB, TheTVDB, TVmaze or AniList")
	}
	d, err := s.detailsOf(ctx, source, id)
	if err != nil {
		return nil, err
	}
	p := &MatchPreview{Source: d.Source, ID: d.ID, Kind: d.Kind, URL: SourceURL(d.Source, d.ID), Label: SourceLabel(d.Source), Title: d.Title, Changes: DiffDetails(t, d)}
	p.Unchanged = len(d.fields()) - len(p.Changes)
	if t.Show != nil {
		p.Episodes = s.previewEpisodes(ctx, t.Show, source, d.ID)
	}
	return p, nil
}

func (s *Scanner) previewEpisodes(ctx context.Context, sh *Show, source, id string) *EpisodePreview {
	src, ok := s.matchSourceNamed(source).(SeriesLister)
	if !ok {
		return nil
	}
	eps, err := src.Episodes(ctx, id)
	if err != nil {
		return nil
	}
	items, err := s.Store.Episodes(ctx, sh.ID, nil)
	if err != nil {
		return nil
	}
	out := &EpisodePreview{Unknown: []string{}}
	for _, it := range items {
		if it.Season == nil || it.Episode == nil {
			continue
		}
		out.Total++
		if _, ok := eps[[2]int{*it.Season, *it.Episode}]; ok {
			out.Known++
		} else if len(out.Unknown) < 50 {
			out.Unknown = append(out.Unknown, fmt.Sprintf("S%02dE%02d", *it.Season, *it.Episode))
		}
	}
	return out
}

// ApplyMatch changes a title to a result, and remembers that a person chose it, so no
// scan or refresh changes it to anything else. It returns the batch that undoes it.
func (s *Scanner) ApplyMatch(ctx context.Context, t *Title, source, id string, only map[string]bool, user string) (string, error) {
	if t.Show != nil && (source == "tmdb" || source == "wikidata") {
		return "", errors.New("a show needs a show from TMDB, TheTVDB, TVmaze or AniList")
	}
	d, err := s.detailsOf(ctx, source, id)
	if err != nil {
		return "", err
	}
	batch := NewBatch()
	if err := s.Store.Remember(ctx, batch, user, "Matched", t, t.snapshot()); err != nil {
		return "", err
	}
	if err := applyDetails(t, d, only); err != nil {
		return "", err
	}
	t.Meta.MatchState = MatchManual
	t.Meta.Suggestions, t.Meta.SuggestedAt = nil, 0
	if t.Show != nil {
		for n, p := range d.SeasonPosters {
			_ = s.Store.SetSeasonPoster(ctx, t.ID, n, p, false)
		}
		if err := s.applyEpisodes(ctx, t.Show, source, d.ID, batch, user); err != nil {
			s.Log.WarnContext(ctx, "could not read a show's episodes", "show", t.Show.Title, "err", err)
		}
	}
	s.persist(ctx, t)
	if err := s.Store.SaveTitle(ctx, t); err != nil {
		return "", err
	}
	return batch, nil
}

// applyEpisodes fills a show's episodes from a source. Fields locked on an episode
// stay, and an episode's own numbers are never changed by this.
func (s *Scanner) applyEpisodes(ctx context.Context, sh *Show, source, id, batch, user string) error {
	src, ok := s.matchSourceNamed(source).(SeriesLister)
	if !ok {
		return nil
	}
	eps, err := src.Episodes(ctx, id)
	if err != nil {
		return err
	}
	items, err := s.Store.Episodes(ctx, sh.ID, nil)
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.Season == nil || it.Episode == nil {
			continue
		}
		t, err := s.Store.LoadTitle(ctx, TargetItem, it.ID)
		if err != nil {
			continue
		}
		if batch != "" {
			if err := s.Store.Remember(ctx, batch, user, "Matched", t, t.snapshot()); err != nil {
				return err
			}
		}
		prev := *t.Item
		applyEpisodeMeta(t.Item, source, eps, false)
		if t.Item.Year == nil {
			t.Item.Year = yearOf(t.Item.Aired)
		}
		t.Item.SortTitle = SortTitle(t.Item.Title)
		s.Store.keepLockedItem(ctx, t.Item, &prev, t.Meta)
		if err := s.Store.upsertItem(ctx, t.Item); err != nil {
			return err
		}
	}
	return nil
}

// persist writes a title's metadata beside its files, if the owner wants that.
func (s *Scanner) persist(ctx context.Context, t *Title) {
	if s.SaveMeta == nil || !s.SaveMeta() {
		return
	}
	var err error
	if t.Item != nil {
		if lib, e := s.Store.Library(ctx, t.Item.LibraryID); e == nil && IsGalleryLibrary(lib.Type) {
			return
		}
		err = s.SaveItemMetadata(ctx, t.Item)
	} else {
		err = s.SaveShowMetadata(ctx, t.Show)
	}
	if err != nil {
		s.Log.WarnContext(ctx, "could not save metadata beside the media", "title", t.Name(), "err", err)
	}
}

// RemoveMatch takes a title's match away. What came from a service goes, what a person
// typed and what's beside the file stay, and a title that came from a service is named
// after its file again. Nothing is looked up for it until someone chooses a match.
func (s *Scanner) RemoveMatch(ctx context.Context, t *Title, user string) (string, error) {
	batch := NewBatch()
	if err := s.Store.Remember(ctx, batch, user, "Removed the match", t, t.snapshot()); err != nil {
		return "", err
	}
	name, year := fileNameOf(t)
	for _, f := range t.FieldNames() {
		if t.Meta.Locks[f] || !isServiceSource(t.sourceOf(f)) {
			continue
		}
		v := ""
		switch f {
		case "title":
			v = name
		case "sortTitle":
			v = SortTitle(name)
		case "year":
			v = intStr(year)
		}
		if err := t.SetField(f, v); err != nil {
			continue
		}
		delete(t.Meta.Baseline, f)
		if v == "" {
			delete(t.Meta.Sources, f)
		} else {
			t.Meta.Sources[f] = SourceFile
		}
	}
	t.setProvider("", "")
	t.Meta.MatchState = MatchNone
	t.Meta.Suggestions, t.Meta.SuggestedAt = nil, 0
	if t.Item != nil && t.Item.Poster == "" {
		tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if p, err := s.thumbnail(tctx, t.Item); err == nil {
			t.Item.Poster = p
			t.Meta.Sources["poster"] = SourceFrame
		}
		cancel()
	}
	s.persist(ctx, t)
	return batch, s.Store.SaveTitle(ctx, t)
}

// PreviewRefresh says what looking a title up again would change.
func (s *Scanner) PreviewRefresh(ctx context.Context, t *Title) (*MatchPreview, error) {
	source, id, err := s.refreshTarget(ctx, t)
	if err != nil {
		return nil, err
	}
	return s.PreviewMatch(ctx, t, source, id)
}

// refreshTarget works out what a refresh asks: the match it has, or the best result
// of a search if the file name makes that nearly certain.
func (s *Scanner) refreshTarget(ctx context.Context, t *Title) (string, string, error) {
	if t.Meta.MatchState == MatchNone {
		return "", "", errors.New("you said this has no match, so nothing is looked up for it. Choose a match to change that")
	}
	source, id := t.Provider()
	if source != "" && id != "" && s.matchSourceNamed(source) != nil && !strings.HasPrefix(source, "agent:") {
		return source, id, nil
	}
	res, err := s.SearchMatches(ctx, t, "", nil)
	if err != nil {
		return "", "", err
	}
	if len(res.Results) > 0 && res.Results[0].Score >= autoMatchScore {
		return res.Results[0].Source, res.Results[0].ID, nil
	}
	if len(res.Warnings) > 0 {
		return "", "", errors.New(res.Warnings[0])
	}
	return "", "", errors.New("no source is sure enough about this. Use Search and match to choose one")
}

// autoMatchScore is how sure a search must be for anything to be chosen without asking.
const autoMatchScore = 85

// RefreshTitle looks a title up again, keeping its match and everything locked.
func (s *Scanner) RefreshTitle(ctx context.Context, t *Title, only map[string]bool, user string) (string, error) {
	source, id, err := s.refreshTarget(ctx, t)
	if err != nil {
		return "", err
	}
	d, err := s.detailsOf(ctx, source, id)
	if err != nil {
		return "", err
	}
	batch := NewBatch()
	if err := s.Store.Remember(ctx, batch, user, "Refreshed", t, t.snapshot()); err != nil {
		return "", err
	}
	if err := applyDetails(t, d, only); err != nil {
		return "", err
	}
	if t.Show != nil {
		if err := s.applyEpisodes(ctx, t.Show, source, d.ID, batch, user); err != nil {
			s.Log.WarnContext(ctx, "could not read a show's episodes", "show", t.Show.Title, "err", err)
		}
	}
	s.persist(ctx, t)
	return batch, s.Store.SaveTitle(ctx, t)
}

// ---------------------------------------------------------------------------
// The review queue
// ---------------------------------------------------------------------------

// QueueRow is one title in the review queue.
type QueueRow struct {
	Target     string           `json:"target"`
	ID         int64            `json:"id"`
	LibraryID  int64            `json:"libraryId"`
	Title      string           `json:"title"`
	Year       *int             `json:"year,omitempty"`
	File       string           `json:"file"`
	Kind       string           `json:"kind"`
	Provider   string           `json:"provider"`
	ProviderID string           `json:"providerId"`
	Label      string           `json:"sourceLabel"`
	URL        string           `json:"url,omitempty"`
	Poster     string           `json:"poster"`
	UpdatedAt  int64            `json:"updatedAt"`
	Confidence int              `json:"confidence"`
	Reasons    []string         `json:"reasons"`
	State      string           `json:"state"` // unmatched | low | ok | manual | none
	Top        []MatchCandidate `json:"suggestions,omitempty"`
}

// reviewBelow: a match scoring under this needs a look.
const reviewBelow = 70

// QueueResult is a page of the queue with the counts for its tabs.
type QueueResult struct {
	Rows   []QueueRow     `json:"rows"`
	Total  int            `json:"total"`
	Counts map[string]int `json:"counts"`
}

func stateOf(t *Title, confidence int) string {
	switch t.Meta.MatchState {
	case MatchManual:
		return "manual"
	case MatchNone:
		return "none"
	}
	if p, _ := t.Provider(); p == "" {
		return "unmatched"
	}
	if confidence < reviewBelow {
		return "low"
	}
	return "ok"
}

// matchableLibrary: a library whose titles are looked up. Home videos, commercials and
// the like aren't, so they don't belong in a queue of things to fix.
func matchableLibrary(typ string) bool {
	switch typ {
	case "movies", "discs", "shows", "anime":
		return true
	}
	return false
}

// MatchQueue lists the titles of a library (0 = all), worst first. Everything it says is
// worked out from what's stored, so opening it asks nobody anything. state is review
// (unmatched and low), unmatched, low, ok, manual, none or all.
func (s *Scanner) MatchQueue(ctx context.Context, libraryID int64, state, search string, limit, offset int) (*QueueResult, error) {
	libs, err := s.Store.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	libTypes := map[int64]string{}
	for _, l := range libs {
		if matchableLibrary(l.Type) && (libraryID == 0 || libraryID == l.ID) {
			libTypes[l.ID] = l.Type
		}
	}
	metas, err := s.Store.allTitleMeta(ctx)
	if err != nil {
		return nil, err
	}
	var rows []QueueRow
	add := func(t *Title) {
		score, why := ConfidenceFor(t, libTypes[t.LibraryID()])
		provider, id := t.Provider()
		row := QueueRow{Target: t.Target, ID: t.ID, LibraryID: t.LibraryID(), Title: t.Name(), File: filepath.Base(t.Path()), Provider: provider, ProviderID: id,
			Label: SourceLabel(provider), URL: SourceURL(provider, id), Confidence: score, Reasons: why, State: stateOf(t, score), Top: t.Meta.Suggestions}
		if t.Item != nil {
			row.Year, row.Kind, row.Poster, row.UpdatedAt = t.Item.Year, "movie", t.Item.Poster, t.Item.UpdatedAt
		} else {
			row.Year, row.Kind, row.Poster, row.UpdatedAt = t.Show.Year, "show", t.Show.Poster, t.Show.UpdatedAt
		}
		rows = append(rows, row)
	}
	for id, typ := range libTypes {
		if typ == "shows" || typ == "anime" {
			shows, err := s.Store.Shows(ctx, ShowQuery{LibraryID: id})
			if err != nil {
				return nil, err
			}
			for _, sh := range shows {
				add(&Title{Target: TargetShow, ID: sh.ID, Show: sh, Meta: metas.get(TargetShow, sh.ID)})
			}
		}
		items, _, err := s.Store.Items(ctx, ItemQuery{LibraryID: id, NoEpisodes: true, Watchable: true, NoTotal: true, Brief: true})
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			add(&Title{Target: TargetItem, ID: it.ID, Item: it, Meta: metas.get(TargetItem, it.ID)})
		}
	}
	counts := map[string]int{"review": 0, "unmatched": 0, "low": 0, "ok": 0, "manual": 0, "none": 0, "all": len(rows)}
	search = strings.ToLower(strings.TrimSpace(search))
	var shown []QueueRow
	for _, r := range rows {
		counts[r.State]++
		if r.State == "unmatched" || r.State == "low" {
			counts["review"]++
		}
		if search != "" && !strings.Contains(strings.ToLower(r.Title), search) && !strings.Contains(strings.ToLower(r.File), search) {
			continue
		}
		switch state {
		case "all":
		case "", "review":
			if r.State != "unmatched" && r.State != "low" {
				continue
			}
		default:
			if r.State != state {
				continue
			}
		}
		shown = append(shown, r)
	}
	sort.SliceStable(shown, func(i, j int) bool {
		if shown[i].Confidence != shown[j].Confidence {
			return shown[i].Confidence < shown[j].Confidence
		}
		return strings.ToLower(shown[i].Title) < strings.ToLower(shown[j].Title)
	})
	total := len(shown)
	if offset > total {
		offset = total
	}
	shown = shown[offset:]
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if len(shown) > limit {
		shown = shown[:limit]
	}
	if shown == nil {
		shown = []QueueRow{}
	}
	return &QueueResult{Rows: shown, Total: total, Counts: counts}, nil
}

// metaIndex holds title_meta rows read in one go.
type metaIndex map[string]*TitleMeta

func (m metaIndex) get(target string, id int64) *TitleMeta {
	if v := m[target+strconv.FormatInt(id, 10)]; v != nil {
		return v
	}
	return newTitleMeta()
}

func (s *Store) allTitleMeta(ctx context.Context) (metaIndex, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT target, target_id FROM title_meta`)
	if err != nil {
		return nil, err
	}
	type key struct {
		t  string
		id int64
	}
	var keys []key
	for rows.Next() {
		var k key
		if err := rows.Scan(&k.t, &k.id); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, k)
	}
	rows.Close()
	out := metaIndex{}
	for _, k := range keys {
		out[k.t+strconv.FormatInt(k.id, 10)] = s.TitleMeta(ctx, k.t, k.id)
	}
	return out, nil
}

// Suggest searches for a title and keeps the best results, so its row in the queue
// can offer them without asking again.
func (s *Scanner) Suggest(ctx context.Context, t *Title) ([]MatchCandidate, error) {
	res, err := s.SearchMatches(ctx, t, "", nil)
	if err != nil {
		return nil, err
	}
	if len(res.Results) == 0 && len(res.Warnings) > 0 {
		return nil, errors.New(res.Warnings[0])
	}
	top := res.Results
	if len(top) > 6 {
		top = top[:6]
	}
	t.Meta.Suggestions, t.Meta.SuggestedAt = top, db.Now()
	if err := s.Store.saveTitleMeta(ctx, t.Target, t.ID, t.Meta); err != nil {
		return nil, err
	}
	return top, nil
}

// SuggestJob is the progress of finding suggestions for a whole queue.
type SuggestJob struct {
	Running bool   `json:"running"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Current string `json:"current,omitempty"`
}

var suggestState = struct {
	sync.Mutex
	job    SuggestJob
	cancel context.CancelFunc
}{}

// SuggestProgress says how the job is doing.
func SuggestProgress() SuggestJob {
	suggestState.Lock()
	defer suggestState.Unlock()
	return suggestState.job
}

// StopSuggesting stops the job.
func StopSuggesting() {
	suggestState.Lock()
	defer suggestState.Unlock()
	if suggestState.cancel != nil {
		suggestState.cancel()
	}
}

// StartSuggesting finds suggestions for everything in a library that needs a look and
// doesn't have any yet, one title at a time. It returns false if it's already running.
func (s *Scanner) StartSuggesting(libraryID int64) bool {
	suggestState.Lock()
	defer suggestState.Unlock()
	if suggestState.job.Running {
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	suggestState.cancel = cancel
	suggestState.job = SuggestJob{Running: true}
	logging.Go("match suggestions", func() {
		defer func() {
			cancel()
			suggestState.Lock()
			suggestState.job.Running = false
			suggestState.job.Current = ""
			suggestState.Unlock()
		}()
		q, err := s.MatchQueue(ctx, libraryID, "review", "", 500, 0)
		if err != nil {
			s.Log.WarnContext(ctx, "could not list what needs a look", "err", err)
			return
		}
		todo := make([]QueueRow, 0, len(q.Rows))
		for _, r := range q.Rows {
			if len(r.Top) == 0 {
				todo = append(todo, r)
			}
		}
		suggestState.Lock()
		suggestState.job.Total = len(todo)
		suggestState.Unlock()
		for i, r := range todo {
			if ctx.Err() != nil {
				return
			}
			suggestState.Lock()
			suggestState.job.Done, suggestState.job.Current = i, r.Title
			suggestState.Unlock()
			t, err := s.Store.LoadTitle(ctx, r.Target, r.ID)
			if err != nil {
				continue
			}
			if _, err := s.Suggest(ctx, t); err != nil {
				s.Log.DebugContext(ctx, "no suggestions", "title", r.Title, "err", err)
			}
			select {
			case <-time.After(700 * time.Millisecond):
			case <-ctx.Done():
				return
			}
		}
		suggestState.Lock()
		suggestState.job.Done = len(todo)
		suggestState.Unlock()
	}, "cat", "library")
	return true
}
