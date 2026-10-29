package library

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func sortStrings(s []string) { sort.Strings(s) }

// ---------------------------------------------------------------------------
// Filename parsing
// ---------------------------------------------------------------------------

var (
	yearRe    = regexp.MustCompile(`^(.*?)[\s._(\[-]+((?:19|20)\d{2})[\s._)\]-]*(.*)$`)
	junkRe    = regexp.MustCompile(`(?i)[\s._-]*(1080p|2160p|720p|480p|4k|uhd|bluray|blu-ray|bdrip|brrip|dvdrip|dvd|web-?dl|webrip|hdtv|x264|x265|h\.?264|h\.?265|hevc|aac|ac3|dts|remux|proper|repack|extended|unrated|directors?\.?cut|multi|dual|hdr|dv)\b.*$`)
	spacersRe = regexp.MustCompile(`[._]+`)
	wsRe      = regexp.MustCompile(`\s+`)
	articles  = []string{"the ", "a ", "an "}
)

// ParseName guesses title and year from a file or folder name.
func ParseName(name string) (title string, year *int) {
	n := strings.TrimSuffix(name, filepath.Ext(name))
	if strings.EqualFold(filepath.Ext(name), ".iso") || strings.EqualFold(filepath.Ext(name), ".img") {
		n = strings.TrimSuffix(name, filepath.Ext(name))
	}
	if m := yearRe.FindStringSubmatch(n); m != nil && strings.TrimSpace(m[1]) != "" {
		y, _ := strconv.Atoi(m[2])
		year = &y
		n = m[1]
	}
	n = junkRe.ReplaceAllString(n, "")
	n = spacersRe.ReplaceAllString(n, " ")
	n = strings.Trim(n, " -()[]")
	n = wsRe.ReplaceAllString(n, " ")
	if n == "" {
		n = strings.TrimSuffix(name, filepath.Ext(name))
	}
	// DVD rips are often ALL CAPS volume labels.
	if n == strings.ToUpper(n) && len(n) > 3 {
		n = titleCase(strings.ToLower(n))
	}
	return n, year
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// SortTitle strips leading articles.
func SortTitle(t string) string {
	l := strings.ToLower(t)
	for _, a := range articles {
		if strings.HasPrefix(l, a) {
			return strings.TrimSpace(t[len(a):])
		}
	}
	return t
}

// ---------------------------------------------------------------------------
// Sidecars: Kodi NFO and <name>.metadata.json
// ---------------------------------------------------------------------------

type nfoMovie struct {
	XMLName   xml.Name `xml:"movie"`
	Title     string   `xml:"title"`
	SortTitle string   `xml:"sorttitle"`
	Year      int      `xml:"year"`
	Plot      string   `xml:"plot"`
	Outline   string   `xml:"outline"`
	Tagline   string   `xml:"tagline"`
	Genres    []string `xml:"genre"`
	Rating    float64  `xml:"rating"`
	MPAA      string   `xml:"mpaa"`
	Studio    string   `xml:"studio"`
	Runtime   int      `xml:"runtime"`
	Premiered string   `xml:"premiered"`
	Thumb     []string `xml:"thumb"`
	Fanart    struct {
		Thumb []string `xml:"thumb"`
	} `xml:"fanart"`
	UniqueIDs []struct {
		Type string `xml:"type,attr"`
		ID   string `xml:",chardata"`
	} `xml:"uniqueid"`
	TMDBID  string `xml:"tmdbid"`
	Ratings struct {
		Rating []struct {
			Name  string  `xml:"name,attr"`
			Value float64 `xml:"value"`
		} `xml:"rating"`
	} `xml:"ratings"`
}

func applyNFO(it *Item, path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var m nfoMovie
	// NFOs sometimes have trailing URLs; cut at the closing tag.
	if i := strings.LastIndex(string(b), "</movie>"); i > 0 {
		b = b[:i+len("</movie>")]
	}
	if err := xml.Unmarshal(b, &m); err != nil {
		return false
	}
	if m.Title != "" {
		it.Title = m.Title
	}
	if m.SortTitle != "" {
		it.SortTitle = m.SortTitle
	}
	if m.Year != 0 {
		y := m.Year
		it.Year = &y
	} else if len(m.Premiered) >= 4 {
		if y, err := strconv.Atoi(m.Premiered[:4]); err == nil {
			it.Year = &y
		}
	}
	if m.Plot != "" {
		it.Overview = m.Plot
	} else if m.Outline != "" {
		it.Overview = m.Outline
	}
	if m.Tagline != "" {
		it.Tagline = m.Tagline
	}
	if len(m.Genres) > 0 {
		it.Genres = m.Genres
	}
	if m.Rating > 0 {
		r := m.Rating
		it.Rating = &r
	} else {
		for _, r := range m.Ratings.Rating {
			if r.Value > 0 {
				v := r.Value
				it.Rating = &v
				break
			}
		}
	}
	if m.MPAA != "" {
		it.ContentRating = m.MPAA
	}
	if m.Studio != "" {
		it.Studio = m.Studio
	}
	if m.Runtime > 0 && it.RuntimeSec == 0 {
		it.RuntimeSec = float64(m.Runtime) * 60
	}
	for _, u := range m.UniqueIDs {
		if u.Type == "tmdb" && u.ID != "" {
			it.Provider, it.ProviderID = "tmdb", u.ID
		}
	}
	if m.TMDBID != "" {
		it.Provider, it.ProviderID = "tmdb", m.TMDBID
	}
	if it.Poster == "" {
		for _, t := range m.Thumb {
			if strings.HasPrefix(t, "http") {
				it.Poster = t
				break
			}
		}
	}
	if it.Backdrop == "" {
		for _, t := range m.Fanart.Thumb {
			if strings.HasPrefix(t, "http") {
				it.Backdrop = t
				break
			}
		}
	}
	return true
}

type sidecarJSON struct {
	Title      string   `json:"title"`
	SortTitle  string   `json:"sortTitle"`
	Year       *int     `json:"year"`
	Overview   *string  `json:"overview"`
	Tagline    string   `json:"tagline"`
	Genres     []string `json:"genres"`
	Rating     *float64 `json:"rating"`
	Provider   *string  `json:"provider"`
	ProviderID *string  `json:"providerId"`
	Poster     string   `json:"poster"`
	Backdrop   string   `json:"backdrop"`
}

func applySidecarJSON(it *Item, path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var m sidecarJSON
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	if m.Title != "" {
		it.Title = m.Title
	}
	if m.SortTitle != "" {
		it.SortTitle = m.SortTitle
	}
	if m.Year != nil {
		it.Year = m.Year
	}
	if m.Overview != nil && *m.Overview != "" {
		it.Overview = *m.Overview
	}
	if m.Tagline != "" {
		it.Tagline = m.Tagline
	}
	if len(m.Genres) > 0 {
		it.Genres = m.Genres
	}
	if m.Rating != nil {
		it.Rating = m.Rating
	}
	if m.Provider != nil && m.ProviderID != nil && *m.ProviderID != "" {
		it.Provider, it.ProviderID = *m.Provider, *m.ProviderID
	}
	if m.Poster != "" {
		it.Poster = m.Poster
	}
	if m.Backdrop != "" {
		it.Backdrop = m.Backdrop
	}
	return true
}

// localArtwork finds poster/fanart files beside the media.
func localArtwork(dir, base string) (poster, backdrop string) {
	try := func(names ...string) string {
		for _, n := range names {
			for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
				p := filepath.Join(dir, n+ext)
				if st, err := os.Stat(p); err == nil && !st.IsDir() {
					return p
				}
			}
		}
		return ""
	}
	poster = try(base+"-poster", base, "poster", "folder", "cover", "movie")
	backdrop = try(base+"-fanart", base+"-backdrop", "fanart", "backdrop", "background")
	return
}

// ---------------------------------------------------------------------------
// TMDB (optional; attribution required by TMDB terms)
// ---------------------------------------------------------------------------

type TMDB struct {
	Key    string
	Client *http.Client
}

const tmdbImage = "https://image.tmdb.org/t/p/"

type tmdbSearch struct {
	Results []struct {
		ID           int     `json:"id"`
		Title        string  `json:"title"`
		Overview     string  `json:"overview"`
		ReleaseDate  string  `json:"release_date"`
		PosterPath   string  `json:"poster_path"`
		BackdropPath string  `json:"backdrop_path"`
		VoteAverage  float64 `json:"vote_average"`
		Popularity   float64 `json:"popularity"`
	} `json:"results"`
}

type tmdbMovie struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Overview     string  `json:"overview"`
	Tagline      string  `json:"tagline"`
	ReleaseDate  string  `json:"release_date"`
	PosterPath   string  `json:"poster_path"`
	BackdropPath string  `json:"backdrop_path"`
	VoteAverage  float64 `json:"vote_average"`
	Runtime      int     `json:"runtime"`
	Genres       []struct {
		Name string `json:"name"`
	} `json:"genres"`
	ProductionCompanies []struct {
		Name string `json:"name"`
	} `json:"production_companies"`
	ReleaseDates struct {
		Results []struct {
			ISO   string `json:"iso_3166_1"`
			Dates []struct {
				Certification string `json:"certification"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
}

func (t *TMDB) get(ctx context.Context, path string, q url.Values, out any) error {
	if t.Key == "" {
		return errors.New("tmdb: no api key")
	}
	// TMDB hands out two credentials on the same page: the short v3 "API
	// key" and the long v4 "API Read Access Token". Accept either.
	key := strings.TrimSpace(t.Key)
	bearer := strings.HasPrefix(key, "eyJ")
	if !bearer {
		q.Set("api_key", key)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.themoviedb.org/3"+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	if bearer {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	c := t.Client
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("tmdb: %s: %s", res.Status, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// Images lists poster and backdrop URLs for an item's movie (best first),
// searching by title when the item is not matched yet.
func (t *TMDB) Images(ctx context.Context, it *Item) (posters, backdrops []string, err error) {
	id := ""
	if it.Provider == "tmdb" {
		id = it.ProviderID
	}
	if id == "" {
		q := url.Values{"query": {it.Title}, "include_adult": {"false"}}
		if it.Year != nil {
			q.Set("year", strconv.Itoa(*it.Year))
		}
		var s tmdbSearch
		if err := t.get(ctx, "/search/movie", q, &s); err != nil {
			return nil, nil, err
		}
		if len(s.Results) == 0 {
			return nil, nil, errors.New("tmdb: no match")
		}
		id = strconv.Itoa(s.Results[0].ID)
	}
	var r struct {
		Posters []struct {
			FilePath string `json:"file_path"`
		} `json:"posters"`
		Backdrops []struct {
			FilePath string `json:"file_path"`
		} `json:"backdrops"`
	}
	if err := t.get(ctx, "/movie/"+id+"/images", url.Values{"include_image_language": {"en,null"}}, &r); err != nil {
		return nil, nil, err
	}
	for _, p := range r.Posters {
		posters = append(posters, tmdbImage+"w500"+p.FilePath)
	}
	for _, b := range r.Backdrops {
		backdrops = append(backdrops, tmdbImage+"w1280"+b.FilePath)
	}
	return posters, backdrops, nil
}

// Enrich fills missing fields from TMDB. It never overwrites a title that a
// sidecar supplied; it does fill overview, artwork, genres, rating.
func (t *TMDB) Enrich(ctx context.Context, it *Item) error {
	id := ""
	if it.Provider == "tmdb" && it.ProviderID != "" {
		id = it.ProviderID
	}
	if id == "" {
		q := url.Values{"query": {it.Title}, "include_adult": {"false"}}
		if it.Year != nil {
			q.Set("year", strconv.Itoa(*it.Year))
		}
		var s tmdbSearch
		if err := t.get(ctx, "/search/movie", q, &s); err != nil {
			return err
		}
		if len(s.Results) == 0 && it.Year != nil {
			q.Del("year")
			if err := t.get(ctx, "/search/movie", q, &s); err != nil {
				return err
			}
		}
		if len(s.Results) == 0 {
			return errors.New("tmdb: no match")
		}
		id = strconv.Itoa(s.Results[0].ID)
	}
	var m tmdbMovie
	if err := t.get(ctx, "/movie/"+id, url.Values{"append_to_response": {"release_dates"}}, &m); err != nil {
		return err
	}
	it.Provider, it.ProviderID = "tmdb", id
	if it.Title == "" || it.Provider == "tmdb" {
		it.Title = m.Title
	}
	if it.Overview == "" {
		it.Overview = m.Overview
	}
	if it.Tagline == "" {
		it.Tagline = m.Tagline
	}
	if it.Year == nil && len(m.ReleaseDate) >= 4 {
		if y, err := strconv.Atoi(m.ReleaseDate[:4]); err == nil {
			it.Year = &y
		}
	}
	if len(it.Genres) == 0 {
		for _, g := range m.Genres {
			it.Genres = append(it.Genres, g.Name)
		}
	}
	if it.Rating == nil && m.VoteAverage > 0 {
		r := m.VoteAverage
		it.Rating = &r
	}
	if it.Studio == "" && len(m.ProductionCompanies) > 0 {
		it.Studio = m.ProductionCompanies[0].Name
	}
	if it.RuntimeSec == 0 && m.Runtime > 0 {
		it.RuntimeSec = float64(m.Runtime) * 60
	}
	if it.ContentRating == "" {
		for _, r := range m.ReleaseDates.Results {
			if r.ISO == "US" {
				for _, d := range r.Dates {
					if d.Certification != "" {
						it.ContentRating = d.Certification
						break
					}
				}
			}
		}
	}
	if (it.Poster == "" || strings.HasPrefix(it.Poster, "generated:")) && m.PosterPath != "" {
		it.Poster = tmdbImage + "w500" + m.PosterPath
	}
	if it.Backdrop == "" && m.BackdropPath != "" {
		it.Backdrop = tmdbImage + "w1280" + m.BackdropPath
	}
	return nil
}

// Check verifies the key with a cheap request (used by the setup wizard).
func (t *TMDB) Check(ctx context.Context) error {
	var out struct{}
	return t.get(ctx, "/configuration", url.Values{}, &out)
}
