package library

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Each metadata service as a MatchSource: searching it and reading one title.

// seriesDetails turns what a TV service says into details.
func seriesDetails(source string, m *SeriesMeta) *MatchDetails {
	d := &MatchDetails{Source: source, ID: m.ID, Kind: "show", URL: SourceURL(source, m.ID), Title: m.Title, Year: m.Year, Overview: m.Overview, Genres: m.Genres, Rating: m.Rating,
		ContentRating: m.ContentRating, Network: m.Network, Status: m.Status, Poster: m.Poster, Backdrop: m.Backdrop, Posters: m.Posters, Backdrops: m.Backdrops,
		SeasonPosters: m.SeasonPosters, AirDays: m.AirDays, AirTime: m.AirTime, Banners: m.Banners, Logos: m.Logos}
	if len(d.Banners) > 0 {
		d.Banner = d.Banners[0]
	}
	if len(d.Logos) > 0 {
		d.Logo = d.Logos[0]
	}
	return d
}

// --- TVmaze ---------------------------------------------------------------

type tvmazeSource struct{ TVMaze }

func (t *tvmazeSource) Label() string { return "TVmaze" }

func (t *tvmazeSource) Search(ctx context.Context, q string, _ *int) ([]MatchCandidate, error) {
	var res []struct {
		Show mazeShow `json:"show"`
	}
	if err := t.get(ctx, "/search/shows?q="+url.QueryEscape(q), &res); err != nil {
		return nil, err
	}
	out := make([]MatchCandidate, 0, len(res))
	for _, r := range res {
		out = append(out, MatchCandidate{Source: "tvmaze", ID: strconv.Itoa(r.Show.ID), Kind: "show", Title: r.Show.Name, Year: yearOf(r.Show.Premiered),
			Rating: r.Show.Rating.Average, Overview: plain(r.Show.Summary), Poster: r.Show.Image.best()})
	}
	return out, nil
}

func (t *tvmazeSource) Details(ctx context.Context, id string) (*MatchDetails, error) {
	m, err := t.Series(ctx, id)
	if err != nil {
		return nil, err
	}
	return seriesDetails("tvmaze", m), nil
}

// --- TheTVDB --------------------------------------------------------------

type tvdbSource struct{ TVDB }

func (t *tvdbSource) Label() string { return "TheTVDB" }

func (t *tvdbSource) Search(ctx context.Context, q string, year *int) ([]MatchCandidate, error) {
	ids, err := t.TVDB.Search(ctx, q, year)
	if err != nil {
		return nil, err
	}
	out := []MatchCandidate{}
	for i, id := range ids {
		if i == 6 { // each one is a request of its own
			break
		}
		m, err := t.Series(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, MatchCandidate{Source: "tvdb", ID: m.ID, Kind: "show", Title: m.Title, Year: m.Year, Rating: m.Rating, Overview: m.Overview, Poster: m.Poster})
	}
	return out, nil
}

func (t *tvdbSource) Details(ctx context.Context, id string) (*MatchDetails, error) {
	m, err := t.Series(ctx, id)
	if err != nil {
		return nil, err
	}
	return seriesDetails("tvdb", m), nil
}

// --- AniList --------------------------------------------------------------

type anilistSource struct{ *AniList }

func (a *anilistSource) Label() string { return "AniList" }

func (a *anilistSource) Search(ctx context.Context, q string, _ *int) ([]MatchCandidate, error) {
	var out struct {
		Page struct {
			Media []alMedia `json:"media"`
		} `json:"Page"`
	}
	query := `query ($s: String) { Page(perPage: 10) { media(search: $s, type: ANIME) { ` + anilistFields + ` } } }`
	if err := a.query(ctx, query, map[string]any{"s": q}, &out); err != nil {
		return nil, err
	}
	list := make([]MatchCandidate, 0, len(out.Page.Media))
	for i := range out.Page.Media {
		m := a.meta(&out.Page.Media[i])
		list = append(list, MatchCandidate{Source: "anilist", ID: m.ID, Kind: "show", Title: m.Title, Year: m.Year, Rating: m.Rating, Overview: m.Overview, Poster: m.Poster})
	}
	return list, nil
}

func (a *anilistSource) Details(ctx context.Context, id string) (*MatchDetails, error) {
	m, err := a.Series(ctx, id)
	if err != nil {
		return nil, err
	}
	return seriesDetails("anilist", m), nil
}

// --- TMDB -----------------------------------------------------------------

type tmdbMovies struct{ TMDB }

func (t *tmdbMovies) Name() string  { return "tmdb" }
func (t *tmdbMovies) Label() string { return "TMDB" }

func tmdbPoster(p string) string {
	if p == "" {
		return ""
	}
	return tmdbImage + "w500" + p
}

func tmdbFloat(f float64) *float64 {
	if f <= 0 {
		return nil
	}
	return &f
}

func (t *tmdbMovies) Search(ctx context.Context, q string, year *int) ([]MatchCandidate, error) {
	v := url.Values{"query": {q}, "include_adult": {"false"}}
	var s tmdbSearch
	if err := t.get(ctx, "/search/movie", v, &s); err != nil {
		return nil, err
	}
	out := make([]MatchCandidate, 0, len(s.Results))
	for _, r := range s.Results {
		out = append(out, MatchCandidate{Source: "tmdb", ID: strconv.Itoa(r.ID), Kind: "movie", Title: r.Title, Year: yearOf(r.ReleaseDate), Rating: tmdbFloat(r.VoteAverage), Overview: r.Overview, Poster: tmdbPoster(r.PosterPath)})
	}
	return out, nil
}

type tmdbImages struct {
	Posters []struct {
		FilePath string `json:"file_path"`
	} `json:"posters"`
	Backdrops []struct {
		FilePath string `json:"file_path"`
	} `json:"backdrops"`
	Logos []struct {
		FilePath string `json:"file_path"`
	} `json:"logos"`
}

func (d *MatchDetails) addTMDBImages(im tmdbImages) {
	for _, p := range im.Posters {
		d.Posters = append(d.Posters, tmdbImage+"w500"+p.FilePath)
	}
	for _, p := range im.Backdrops {
		d.Backdrops = append(d.Backdrops, tmdbImage+"w1280"+p.FilePath)
	}
	for _, p := range im.Logos {
		d.Logos = append(d.Logos, tmdbImage+"w500"+p.FilePath)
	}
	if d.Logo == "" && len(d.Logos) > 0 {
		d.Logo = d.Logos[0]
	}
}

func (t *tmdbMovies) Details(ctx context.Context, id string) (*MatchDetails, error) {
	var m struct {
		tmdbMovie
		Credits struct {
			Crew []struct {
				Name string `json:"name"`
				Job  string `json:"job"`
			} `json:"crew"`
		} `json:"credits"`
		Images tmdbImages `json:"images"`
	}
	if err := t.get(ctx, "/movie/"+url.PathEscape(id), url.Values{"append_to_response": {"release_dates,credits,images"}, "include_image_language": {"en,null"}}, &m); err != nil {
		return nil, err
	}
	d := &MatchDetails{Source: "tmdb", ID: strconv.Itoa(m.ID), Kind: "movie", URL: SourceURL("tmdb", id), Title: m.Title, Year: yearOf(m.ReleaseDate), Overview: m.Overview, Tagline: m.Tagline,
		Rating: tmdbFloat(m.VoteAverage), RuntimeMin: m.Runtime, Poster: tmdbPoster(m.PosterPath)}
	if m.BackdropPath != "" {
		d.Backdrop = tmdbImage + "w1280" + m.BackdropPath
	}
	for _, g := range m.Genres {
		d.Genres = append(d.Genres, g.Name)
	}
	for _, c := range m.ProductionCompanies {
		d.Studios = append(d.Studios, c.Name)
	}
	var directors []string
	for _, c := range m.Credits.Crew {
		if c.Job == "Director" {
			directors = append(directors, c.Name)
		}
	}
	d.Director = strings.Join(directors, ", ")
	if m.Collection != nil {
		d.Series = m.Collection.Name
	}
	for _, r := range m.ReleaseDates.Results {
		if r.ISO == "US" {
			for _, rd := range r.Dates {
				if rd.Certification != "" {
					d.ContentRating = rd.Certification
					break
				}
			}
		}
	}
	d.addTMDBImages(m.Images)
	return d, nil
}

type tmdbShows struct{ TMDB }

func (t *tmdbShows) Name() string  { return "tmdb-tv" }
func (t *tmdbShows) Label() string { return "TMDB" }

func (t *tmdbShows) Search(ctx context.Context, q string, _ *int) ([]MatchCandidate, error) {
	var s struct {
		Results []struct {
			ID           int     `json:"id"`
			Name         string  `json:"name"`
			Overview     string  `json:"overview"`
			FirstAirDate string  `json:"first_air_date"`
			PosterPath   string  `json:"poster_path"`
			VoteAverage  float64 `json:"vote_average"`
		} `json:"results"`
	}
	if err := t.get(ctx, "/search/tv", url.Values{"query": {q}, "include_adult": {"false"}}, &s); err != nil {
		return nil, err
	}
	out := make([]MatchCandidate, 0, len(s.Results))
	for _, r := range s.Results {
		out = append(out, MatchCandidate{Source: "tmdb-tv", ID: strconv.Itoa(r.ID), Kind: "show", Title: r.Name, Year: yearOf(r.FirstAirDate), Rating: tmdbFloat(r.VoteAverage), Overview: r.Overview, Poster: tmdbPoster(r.PosterPath)})
	}
	return out, nil
}

func (t *tmdbShows) Details(ctx context.Context, id string) (*MatchDetails, error) {
	var m struct {
		ID           int     `json:"id"`
		Name         string  `json:"name"`
		Overview     string  `json:"overview"`
		Tagline      string  `json:"tagline"`
		FirstAirDate string  `json:"first_air_date"`
		PosterPath   string  `json:"poster_path"`
		BackdropPath string  `json:"backdrop_path"`
		VoteAverage  float64 `json:"vote_average"`
		Status       string  `json:"status"`
		Genres       []struct {
			Name string `json:"name"`
		} `json:"genres"`
		Networks []struct {
			Name string `json:"name"`
		} `json:"networks"`
		ProductionCompanies []struct {
			Name string `json:"name"`
		} `json:"production_companies"`
		CreatedBy []struct {
			Name string `json:"name"`
		} `json:"created_by"`
		ContentRatings struct {
			Results []struct {
				ISO    string `json:"iso_3166_1"`
				Rating string `json:"rating"`
			} `json:"results"`
		} `json:"content_ratings"`
		Seasons []struct {
			Number int    `json:"season_number"`
			Poster string `json:"poster_path"`
		} `json:"seasons"`
		Images tmdbImages `json:"images"`
	}
	if err := t.get(ctx, "/tv/"+url.PathEscape(id), url.Values{"append_to_response": {"content_ratings,images"}, "include_image_language": {"en,null"}}, &m); err != nil {
		return nil, err
	}
	d := &MatchDetails{Source: "tmdb-tv", ID: strconv.Itoa(m.ID), Kind: "show", URL: SourceURL("tmdb-tv", id), Title: m.Name, Year: yearOf(m.FirstAirDate), Overview: m.Overview, Tagline: m.Tagline,
		Rating: tmdbFloat(m.VoteAverage), Poster: tmdbPoster(m.PosterPath), SeasonPosters: map[int]string{}}
	switch m.Status {
	case "Returning Series":
		d.Status = "Continuing"
	default:
		d.Status = m.Status
	}
	if m.BackdropPath != "" {
		d.Backdrop = tmdbImage + "w1280" + m.BackdropPath
	}
	for _, g := range m.Genres {
		d.Genres = append(d.Genres, g.Name)
	}
	if len(m.Networks) > 0 {
		d.Network = m.Networks[0].Name
	}
	for _, c := range m.ProductionCompanies {
		d.Studios = append(d.Studios, c.Name)
	}
	names := make([]string, 0, len(m.CreatedBy))
	for _, c := range m.CreatedBy {
		names = append(names, c.Name)
	}
	d.Director = strings.Join(names, ", ")
	for _, r := range m.ContentRatings.Results {
		if r.ISO == "US" {
			d.ContentRating = r.Rating
		}
	}
	for _, s := range m.Seasons {
		if s.Poster != "" {
			d.SeasonPosters[s.Number] = tmdbPoster(s.Poster)
		}
	}
	d.addTMDBImages(m.Images)
	return d, nil
}

// Episodes reads a show's seasons one at a time.
func (t *tmdbShows) Episodes(ctx context.Context, id string) (map[[2]int]EpisodeMeta, error) {
	var m struct {
		Seasons []struct {
			Number int `json:"season_number"`
		} `json:"seasons"`
	}
	if err := t.get(ctx, "/tv/"+url.PathEscape(id), url.Values{}, &m); err != nil {
		return nil, err
	}
	out := map[[2]int]EpisodeMeta{}
	for i, s := range m.Seasons {
		if i == 100 {
			break
		}
		var sn struct {
			Episodes []struct {
				ID       int    `json:"id"`
				Name     string `json:"name"`
				Overview string `json:"overview"`
				AirDate  string `json:"air_date"`
				Number   int    `json:"episode_number"`
				Runtime  int    `json:"runtime"`
				Still    string `json:"still_path"`
			} `json:"episodes"`
		}
		if err := t.get(ctx, "/tv/"+url.PathEscape(id)+"/season/"+strconv.Itoa(s.Number), url.Values{}, &sn); err != nil {
			return nil, err
		}
		for _, e := range sn.Episodes {
			em := EpisodeMeta{ID: strconv.Itoa(e.ID), Season: s.Number, Number: e.Number, Title: e.Name, Overview: e.Overview, Aired: e.AirDate, RuntimeMin: e.Runtime}
			if e.Still != "" {
				em.Image = tmdbImage + "w500" + e.Still
			}
			out[[2]int{s.Number, e.Number}] = em
		}
	}
	return out, nil
}

// findExternal finds a title by its IMDb id.
func (t *TMDB) findExternal(ctx context.Context, id string) ([]MatchCandidate, error) {
	var r struct {
		Movies []struct {
			ID          int     `json:"id"`
			Title       string  `json:"title"`
			Overview    string  `json:"overview"`
			ReleaseDate string  `json:"release_date"`
			PosterPath  string  `json:"poster_path"`
			VoteAverage float64 `json:"vote_average"`
		} `json:"movie_results"`
		Shows []struct {
			ID           int     `json:"id"`
			Name         string  `json:"name"`
			Overview     string  `json:"overview"`
			FirstAirDate string  `json:"first_air_date"`
			PosterPath   string  `json:"poster_path"`
			VoteAverage  float64 `json:"vote_average"`
		} `json:"tv_results"`
	}
	if err := t.get(ctx, "/find/"+url.PathEscape(id), url.Values{"external_source": {"imdb_id"}}, &r); err != nil {
		return nil, err
	}
	out := []MatchCandidate{}
	for _, m := range r.Movies {
		out = append(out, MatchCandidate{Source: "tmdb", ID: strconv.Itoa(m.ID), Kind: "movie", Title: m.Title, Year: yearOf(m.ReleaseDate), Overview: m.Overview, Poster: tmdbPoster(m.PosterPath), Rating: tmdbFloat(m.VoteAverage)})
	}
	for _, m := range r.Shows {
		out = append(out, MatchCandidate{Source: "tmdb-tv", ID: strconv.Itoa(m.ID), Kind: "show", Title: m.Name, Year: yearOf(m.FirstAirDate), Overview: m.Overview, Poster: tmdbPoster(m.PosterPath), Rating: tmdbFloat(m.VoteAverage)})
	}
	return out, nil
}

// --- Wikidata -------------------------------------------------------------

type wikidataSource struct{ Wikidata }

func (w *wikidataSource) Name() string  { return "wikidata" }
func (w *wikidataSource) Label() string { return "Wikidata" }

const (
	wdDirector = "P57"
	wdStudio   = "P272"
	wdNetwork  = "P449"
	wdIMDb     = "P345"
)

func wdWhat(desc string) string {
	d := strings.ToLower(desc)
	if strings.Contains(d, "series") || strings.Contains(d, "show") || strings.Contains(d, "miniseries") || strings.Contains(d, "sitcom") || strings.Contains(d, "anime") {
		return "show"
	}
	if strings.Contains(d, "film") || strings.Contains(d, "movie") {
		return "movie"
	}
	return ""
}

func (w *wikidataSource) Search(ctx context.Context, q string, _ *int) ([]MatchCandidate, error) {
	var search struct {
		Search []struct {
			ID          string `json:"id"`
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"search"`
	}
	v := url.Values{"action": {"wbsearchentities"}, "search": {q}, "language": {"en"}, "type": {"item"}, "limit": {"20"}, "format": {"json"}}
	if err := w.get(ctx, w.api()+"?"+v.Encode(), &search); err != nil {
		return nil, err
	}
	var ids []string
	info := map[string]MatchCandidate{}
	for _, r := range search.Search {
		kind := wdWhat(r.Description)
		d := strings.ToLower(r.Description)
		if kind == "" || containsAny(d, "franchise", "film series", "trilogy", "soundtrack", "character", "festival", "studio", "company", "book", "episode", "season of", "album") {
			continue
		}
		ids = append(ids, r.ID)
		info[r.ID] = MatchCandidate{Source: "wikidata", ID: r.ID, Kind: kind, Title: r.Label, Overview: r.Description}
	}
	if len(ids) == 0 {
		return w.searchText(ctx, q)
	}
	ents, err := w.entities(ctx, ids, "claims")
	if err != nil {
		return nil, err
	}
	posters := w.commonsPosters(ctx, ents)
	out := make([]MatchCandidate, 0, len(ids))
	for _, id := range ids {
		c := info[id]
		c.Poster = posters[id]
		if e, ok := ents[id]; ok {
			if cl := e.Claims[wdPublication]; len(cl) > 0 {
				if y, err := strconv.Atoi(firstN(cl[0].Mainsnak.time(), 4)); err == nil {
					c.Year = &y
				}
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// Wikidata's two kinds of film and of TV series.
const (
	wdFilm   = "Q11424"
	wdSeries = "Q5398426"
)

// searchText looks through everything a film or series is called, for a name that doesn't
// start the way its entry does: "Dr Story The Treeline" for "The Treeline".
func (w *wikidataSource) searchText(ctx context.Context, q string) ([]MatchCandidate, error) {
	ids, err := w.searchTextIDs(ctx, q)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	ents, err := w.entities(ctx, ids, "claims|labels|descriptions")
	if err != nil {
		return nil, err
	}
	posters := w.commonsPosters(ctx, ents)
	out := make([]MatchCandidate, 0, len(ids))
	for _, id := range ids {
		e, ok := ents[id]
		if !ok {
			continue
		}
		kind := wdWhat(e.Descriptions["en"].Value)
		if kind == "" {
			kind = "movie"
		}
		c := MatchCandidate{Source: "wikidata", ID: id, Kind: kind, Title: e.Labels["en"].Value, Overview: e.Descriptions["en"].Value, Poster: posters[id]}
		if cl := e.Claims[wdPublication]; len(cl) > 0 {
			if y, err := strconv.Atoi(firstN(cl[0].Mainsnak.time(), 4)); err == nil {
				c.Year = &y
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// searchTextIDs: the films and series whose names, any of them, have these words, best first.
func (w *Wikidata) searchTextIDs(ctx context.Context, q string) ([]string, error) {
	var res struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	v := url.Values{"action": {"query"}, "list": {"search"}, "srlimit": {"10"}, "format": {"json"},
		"srsearch": {strings.ReplaceAll(q, `"`, " ") + " haswbstatement:" + wdInstanceOf + "=" + wdFilm + "|" + wdInstanceOf + "=" + wdSeries}}
	if err := w.get(ctx, w.api()+"?"+v.Encode(), &res); err != nil {
		return nil, err
	}
	var ids []string
	for _, r := range res.Query.Search {
		if wikiIDRe.MatchString(r.Title) {
			ids = append(ids, r.Title)
		}
	}
	return ids, nil
}

func (w *wikidataSource) Details(ctx context.Context, id string) (*MatchDetails, error) {
	if !wikiIDRe.MatchString(id) {
		return nil, ErrNotFound
	}
	ents, err := w.entities(ctx, []string{id}, "claims|labels|descriptions|sitelinks")
	if err != nil {
		return nil, err
	}
	e, ok := ents[id]
	if !ok {
		return nil, ErrNotFound
	}
	d := &MatchDetails{Source: "wikidata", ID: id, Kind: wdWhat(e.Descriptions["en"].Value), URL: SourceURL("wikidata", id), Title: e.Labels["en"].Value}
	if c := e.Claims[wdPublication]; len(c) > 0 {
		if y, err := strconv.Atoi(firstN(c[0].Mainsnak.time(), 4)); err == nil {
			d.Year = &y
		}
	}
	if c := e.Claims[wdDuration]; len(c) > 0 {
		d.RuntimeMin = int(c[0].Mainsnak.amount())
	}
	// Other items are named in a second request, in one go.
	refs := map[string][]string{}
	var all []string
	for _, p := range []string{wdGenre, wdDirector, wdStudio, wdNetwork, wdMPARating, wdPartOfSeries} {
		for i, c := range e.Claims[p] {
			if i == 12 {
				break
			}
			if ref := c.Mainsnak.itemID(); ref != "" {
				refs[p] = append(refs[p], ref)
				all = append(all, ref)
			}
		}
	}
	if len(all) > 50 {
		all = all[:50]
	}
	if len(all) > 0 {
		names, err := w.entities(ctx, all, "labels")
		if err == nil {
			label := func(ref string) string { return names[ref].Labels["en"].Value }
			for _, ref := range refs[wdGenre] {
				if l := label(ref); l != "" {
					d.Genres = append(d.Genres, titleCase(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(l, " film"), " movie"), " television series")))
				}
			}
			var dirs []string
			for _, ref := range refs[wdDirector] {
				if l := label(ref); l != "" {
					dirs = append(dirs, l)
				}
			}
			d.Director = strings.Join(dirs, ", ")
			for _, ref := range refs[wdStudio] {
				if l := label(ref); l != "" {
					d.Studios = append(d.Studios, l)
				}
			}
			if r := refs[wdNetwork]; len(r) > 0 {
				d.Network = label(r[0])
			}
			if r := refs[wdMPARating]; len(r) > 0 {
				d.ContentRating = strings.TrimSuffix(label(r[0]), " rating")
			}
			var series []string
			for _, r := range refs[wdPartOfSeries] {
				series = append(series, label(r))
			}
			d.Series = pickSeries(series, d.Title)
		}
	}
	if page := e.Sitelinks["enwiki"].Title; page != "" {
		var sum struct {
			Extract string `json:"extract"`
		}
		if w.get(ctx, "https://en.wikipedia.org/api/rest_v1/page/summary/"+url.PathEscape(strings.ReplaceAll(page, " ", "_")), &sum) == nil {
			d.Overview = sum.Extract
		}
	}
	if d.Overview == "" {
		d.Overview = e.Descriptions["en"].Value
	}
	if p := w.commonsPoster(ctx, e); p != "" {
		d.Poster, d.Posters = p, []string{p}
	}
	return d, nil
}

func (w *Wikidata) api() string {
	if w.API != "" {
		return w.API
	}
	return "https://www.wikidata.org/w/api.php"
}

// byIMDb finds the entity with this IMDb id.
func (w *Wikidata) byIMDb(ctx context.Context, id string) (string, error) {
	var res struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	v := url.Values{"action": {"query"}, "list": {"search"}, "srsearch": {"haswbstatement:" + wdIMDb + "=" + id}, "format": {"json"}}
	if err := w.get(ctx, w.api()+"?"+v.Encode(), &res); err != nil {
		return "", err
	}
	if len(res.Query.Search) == 0 {
		return "", ErrNotFound
	}
	return res.Query.Search[0].Title, nil
}

// --- Custom agents --------------------------------------------------------

// agentSource asks one of the owner's agents. An agent has no idea of a list or an
// id: it's asked by name and gives its best answer, kept for a moment so reading the
// "details" of that answer doesn't ask a second time, with a name it was never asked.
type agentSource struct {
	a       Agent
	libType string
}

var agentAnswers = struct {
	sync.Mutex
	m map[string]agentAnswer
}{m: map[string]agentAnswer{}}

type agentAnswer struct {
	r  *AgentResult
	at time.Time
}

func (a *agentSource) Name() string  { return "agent:" + a.a.ID }
func (a *agentSource) Label() string { return a.a.Name }

func (a *agentSource) Search(ctx context.Context, q string, year *int) ([]MatchCandidate, error) {
	r, err := a.a.Lookup(ctx, q, year, a.libType)
	if err != nil {
		return nil, err
	}
	id := r.ID
	if id == "" {
		id = "x" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	id = clip(id, 80)
	agentAnswers.Lock()
	for k, v := range agentAnswers.m {
		if time.Since(v.at) > 10*time.Minute {
			delete(agentAnswers.m, k)
		}
	}
	agentAnswers.m[a.a.ID+"|"+id] = agentAnswer{r, time.Now()}
	agentAnswers.Unlock()
	c := MatchCandidate{Source: a.Name(), ID: id, Kind: "movie", Title: r.Title, Overview: r.Overview, Poster: r.Poster}
	if r.Year > 0 {
		y := r.Year
		c.Year = &y
	}
	if r.Rating > 0 {
		v := r.Rating
		c.Rating = &v
	}
	return []MatchCandidate{c}, nil
}

func (a *agentSource) Details(_ context.Context, id string) (*MatchDetails, error) {
	agentAnswers.Lock()
	ans, ok := agentAnswers.m[a.a.ID+"|"+id]
	agentAnswers.Unlock()
	if !ok {
		return nil, errors.New("search again: that answer has gone")
	}
	r := ans.r
	d := &MatchDetails{Source: a.Name(), ID: id, Kind: "movie", Title: r.Title, Overview: r.Overview, Tagline: r.Tagline, Genres: r.Genres, ContentRating: r.ContentRating, RuntimeMin: r.RuntimeMinutes, Poster: r.Poster, Backdrop: r.Backdrop}
	if r.Year > 0 {
		y := r.Year
		d.Year = &y
	}
	if r.Rating > 0 {
		v := r.Rating
		d.Rating = &v
	}
	if r.Studio != "" {
		d.Studios = []string{r.Studio}
	}
	return d, nil
}
