package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TVDB is a small client for TheTVDB API v4 (https://thetvdb.github.io/v4-api/).
// Metadata provided by TheTVDB; attribution is required by their terms.
type TVDB struct {
	Key     string
	PIN     string // only for user-supported keys
	BaseURL string // default https://api4.thetvdb.com/v4
	Client  *http.Client
}

const tvdbBase = "https://api4.thetvdb.com/v4"

// Tokens are valid for a month; cache them per key so a scan of a thousand
// episodes logs in once.
var tvdbTokens = struct {
	sync.Mutex
	m map[string]tvdbToken
}{m: map[string]tvdbToken{}}

type tvdbToken struct {
	token string
	until time.Time
}

// Artwork type ids (GET /artwork/types).
const (
	tvdbArtPoster       = 2
	tvdbArtBackground   = 3
	tvdbArtSeasonPoster = 7
)

// SeriesMeta is the show-level metadata WittyMediaServer uses.
type SeriesMeta struct {
	ID            string
	Title         string
	Year          *int
	Overview      string
	Genres        []string
	Rating        *float64
	ContentRating string
	Network       string
	Status        string
	Poster        string
	Backdrop      string
	SeasonPosters map[int]string
	// All posters/backgrounds, best first (for the artwork picker).
	Posters   []string
	Backdrops []string
}

// EpisodeMeta is one episode from the default (aired) order.
type EpisodeMeta struct {
	ID         string
	Season     int
	Number     int
	Title      string
	Overview   string
	Aired      string
	RuntimeMin int
	Image      string
}

func (t *TVDB) base() string {
	if t.BaseURL != "" {
		return strings.TrimRight(t.BaseURL, "/")
	}
	return tvdbBase
}

func (t *TVDB) client() *http.Client {
	if t.Client != nil {
		return t.Client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (t *TVDB) token(ctx context.Context, fresh bool) (string, error) {
	if t.Key == "" {
		return "", errors.New("tvdb: no api key")
	}
	ck := t.base() + "|" + t.Key + "|" + t.PIN
	tvdbTokens.Lock()
	tok, ok := tvdbTokens.m[ck]
	tvdbTokens.Unlock()
	if ok && !fresh && time.Now().Before(tok.until) {
		return tok.token, nil
	}
	body := map[string]string{"apikey": t.Key}
	if t.PIN != "" {
		body["pin"] = t.PIN
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", t.base()+"/login", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := t.client().Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return "", fmt.Errorf("tvdb login: %s %s", res.Status, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Data.Token == "" {
		return "", errors.New("tvdb login: no token returned")
	}
	tvdbTokens.Lock()
	tvdbTokens.m[ck] = tvdbToken{token: out.Data.Token, until: time.Now().Add(24 * time.Hour)}
	tvdbTokens.Unlock()
	return out.Data.Token, nil
}

// get performs an authenticated GET, logging in again once on 401.
func (t *TVDB) get(ctx context.Context, path string, q url.Values, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := t.token(ctx, attempt > 0)
		if err != nil {
			return err
		}
		u := t.base() + path
		if len(q) > 0 {
			u += "?" + q.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json")
		res, err := t.client().Do(req)
		if err != nil {
			return err
		}
		if res.StatusCode == 401 && attempt == 0 {
			res.Body.Close()
			continue
		}
		if res.StatusCode != 200 {
			msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
			res.Body.Close()
			return fmt.Errorf("tvdb %s: %s %s", path, res.Status, strings.TrimSpace(string(msg)))
		}
		err = json.NewDecoder(res.Body).Decode(out)
		res.Body.Close()
		return err
	}
	return errors.New("tvdb: unauthorized")
}

// flexString accepts JSON strings or numbers (TVDB mixes them, e.g. tvdb_id
// and year are strings in search results but numbers elsewhere).
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	if string(b) == "null" {
		*f = ""
		return nil
	}
	*f = flexString(strings.TrimSpace(string(b)))
	return nil
}

func yearOf(s string) *int {
	if len(s) >= 4 {
		if y, err := strconv.Atoi(s[:4]); err == nil && y > 1800 {
			return &y
		}
	}
	return nil
}

// Search returns candidate series ids for a name, best first.
func (t *TVDB) Search(ctx context.Context, name string, year *int) ([]string, error) {
	var out struct {
		Data []struct {
			TVDBID flexString `json:"tvdb_id"`
			Name   string     `json:"name"`
			Year   flexString `json:"year"`
		} `json:"data"`
	}
	q := url.Values{"query": {name}, "type": {"series"}, "limit": {"10"}}
	if year != nil {
		q.Set("year", strconv.Itoa(*year))
	}
	if err := t.get(ctx, "/search", q, &out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 && year != nil {
		q.Del("year")
		if err := t.get(ctx, "/search", q, &out); err != nil {
			return nil, err
		}
	}
	norm := func(s string) string {
		var b strings.Builder
		for _, r := range strings.ToLower(s) {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	want := norm(name)
	type cand struct {
		id    string
		score int
	}
	var cs []cand
	for i, d := range out.Data {
		if d.TVDBID == "" {
			continue
		}
		sc := 100 - i // keep the API's relevance order as the tie-breaker
		if norm(d.Name) == want {
			sc += 1000
		}
		if year != nil && string(d.Year) == strconv.Itoa(*year) {
			sc += 500
		}
		cs = append(cs, cand{string(d.TVDBID), sc})
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].score > cs[j].score })
	ids := make([]string, len(cs))
	for i, c := range cs {
		ids[i] = c.id
	}
	return ids, nil
}

// FindSeries searches by name and returns the best match's metadata.
func (t *TVDB) FindSeries(ctx context.Context, name string, year *int) (*SeriesMeta, error) {
	ids, err := t.Search(ctx, name, year)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, errors.New("tvdb: no match for " + name)
	}
	return t.Series(ctx, ids[0])
}

// Series loads a series by id with English text where available.
func (t *TVDB) Series(ctx context.Context, id string) (*SeriesMeta, error) {
	var out struct {
		Data struct {
			ID         flexString `json:"id"`
			Name       string     `json:"name"`
			Overview   *string    `json:"overview"`
			Image      string     `json:"image"`
			Year       flexString `json:"year"`
			FirstAired string     `json:"firstAired"`
			Score      float64    `json:"score"`
			Status     *struct {
				Name string `json:"name"`
			} `json:"status"`
			OriginalNetwork *struct {
				Name string `json:"name"`
			} `json:"originalNetwork"`
			LatestNetwork *struct {
				Name string `json:"name"`
			} `json:"latestNetwork"`
			Genres []struct {
				Name string `json:"name"`
			} `json:"genres"`
			ContentRatings []struct {
				Name    string `json:"name"`
				Country string `json:"country"`
			} `json:"contentRatings"`
			Artworks []struct {
				Type     int     `json:"type"`
				Image    string  `json:"image"`
				Score    float64 `json:"score"`
				Language *string `json:"language"`
				SeasonID int     `json:"seasonId"`
			} `json:"artworks"`
			Seasons []struct {
				ID     int `json:"id"`
				Number int `json:"number"`
				Type   struct {
					Type string `json:"type"`
				} `json:"type"`
				Image string `json:"image"`
			} `json:"seasons"`
		} `json:"data"`
	}
	if err := t.get(ctx, "/series/"+url.PathEscape(id)+"/extended", url.Values{"short": {"false"}}, &out); err != nil {
		return nil, err
	}
	d := out.Data
	m := &SeriesMeta{ID: id, Title: d.Name, Poster: d.Image, SeasonPosters: map[int]string{}}
	if d.Overview != nil {
		m.Overview = *d.Overview
	}
	if m.Year = yearOf(string(d.Year)); m.Year == nil {
		m.Year = yearOf(d.FirstAired)
	}
	if d.Status != nil {
		m.Status = d.Status.Name
	}
	if d.OriginalNetwork != nil {
		m.Network = d.OriginalNetwork.Name
	} else if d.LatestNetwork != nil {
		m.Network = d.LatestNetwork.Name
	}
	for _, g := range d.Genres {
		m.Genres = append(m.Genres, g.Name)
	}
	for _, r := range d.ContentRatings {
		if strings.EqualFold(r.Country, "usa") || m.ContentRating == "" {
			m.ContentRating = r.Name
		}
	}
	// English translation (the base record is in the original language).
	var tr struct {
		Data struct {
			Name     string `json:"name"`
			Overview string `json:"overview"`
		} `json:"data"`
	}
	if err := t.get(ctx, "/series/"+url.PathEscape(id)+"/translations/eng", nil, &tr); err == nil {
		if tr.Data.Name != "" {
			m.Title = tr.Data.Name
		}
		if tr.Data.Overview != "" {
			m.Overview = tr.Data.Overview
		}
	}
	// Artwork: highest score, preferring English or language-neutral images.
	best := map[int]float64{}
	seasonByID := map[int]int{}
	for _, s := range d.Seasons {
		if s.Type.Type == "" || s.Type.Type == "official" || s.Type.Type == "default" {
			seasonByID[s.ID] = s.Number
			if s.Image != "" {
				m.SeasonPosters[s.Number] = s.Image
			}
		}
	}
	seasonBest := map[int]float64{}
	type scored struct {
		url   string
		score float64
	}
	var posters, backdrops []scored
	for _, a := range d.Artworks {
		if a.Image == "" {
			continue
		}
		score := a.Score
		if a.Language == nil || *a.Language == "" || *a.Language == "eng" {
			score += 1e9
		}
		switch a.Type {
		case tvdbArtPoster, tvdbArtBackground:
			if a.Type == tvdbArtPoster {
				posters = append(posters, scored{a.Image, score})
			} else {
				backdrops = append(backdrops, scored{a.Image, score})
			}
			if score > best[a.Type] {
				best[a.Type] = score
				if a.Type == tvdbArtPoster {
					m.Poster = a.Image
				} else {
					m.Backdrop = a.Image
				}
			}
		case tvdbArtSeasonPoster:
			n, ok := seasonByID[a.SeasonID]
			if ok && score > seasonBest[n] {
				seasonBest[n] = score
				m.SeasonPosters[n] = a.Image
			}
		}
	}
	for _, l := range []struct {
		in  []scored
		out *[]string
	}{{posters, &m.Posters}, {backdrops, &m.Backdrops}} {
		sort.SliceStable(l.in, func(i, j int) bool { return l.in[i].score > l.in[j].score })
		for _, x := range l.in {
			*l.out = append(*l.out, x.url)
		}
	}
	return m, nil
}

// Episodes lists every episode in aired order, keyed by {season, number}.
func (t *TVDB) Episodes(ctx context.Context, id string) (map[[2]int]EpisodeMeta, error) {
	out := map[[2]int]EpisodeMeta{}
	for page := 0; page < 50; page++ {
		var r struct {
			Data struct {
				Episodes []struct {
					ID           flexString `json:"id"`
					SeasonNumber int        `json:"seasonNumber"`
					Number       int        `json:"number"`
					Name         *string    `json:"name"`
					Overview     *string    `json:"overview"`
					Aired        *string    `json:"aired"`
					Runtime      *int       `json:"runtime"`
					Image        *string    `json:"image"`
				} `json:"episodes"`
			} `json:"data"`
			Links struct {
				Next *string `json:"next"`
			} `json:"links"`
		}
		if err := t.get(ctx, "/series/"+url.PathEscape(id)+"/episodes/default/eng", url.Values{"page": {strconv.Itoa(page)}}, &r); err != nil {
			if page == 0 {
				return nil, err
			}
			break
		}
		for _, e := range r.Data.Episodes {
			em := EpisodeMeta{ID: string(e.ID), Season: e.SeasonNumber, Number: e.Number}
			if e.Name != nil {
				em.Title = *e.Name
			}
			if e.Overview != nil {
				em.Overview = *e.Overview
			}
			if e.Aired != nil {
				em.Aired = *e.Aired
			}
			if e.Runtime != nil {
				em.RuntimeMin = *e.Runtime
			}
			if e.Image != nil {
				em.Image = *e.Image
			}
			out[[2]int{e.SeasonNumber, e.Number}] = em
		}
		if r.Links.Next == nil || *r.Links.Next == "" || len(r.Data.Episodes) == 0 {
			break
		}
	}
	return out, nil
}
