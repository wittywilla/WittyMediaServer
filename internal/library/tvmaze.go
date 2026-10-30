package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TVMaze reads TV metadata from tvmaze.com: free, no API key, data under
// CC BY-SA 4.0 (credited in the app and the docs). It is used for TV shows
// whenever no TheTVDB key is set, so TV libraries get posters and episode
// names out of the box. TVmaze has no content ratings; those come from
// TheTVDB, an NFO file or a manual edit.
type TVMaze struct {
	Client  *http.Client
	BaseURL string // tests
}

const tvmazeBase = "https://api.tvmaze.com"

// TVmaze allows 20 requests per 10 seconds per address; calls are spaced so
// a big first scan never gets throttled.
var tvmazeGate = struct {
	sync.Mutex
	next time.Time
}{}

func (t *TVMaze) base() string {
	if t.BaseURL != "" {
		return strings.TrimRight(t.BaseURL, "/")
	}
	return tvmazeBase
}

func (t *TVMaze) get(ctx context.Context, path string, out any) error {
	c := t.Client
	if c == nil {
		c = catalogClient(20 * time.Second)
	}
	for attempt := 0; attempt < 3; attempt++ {
		tvmazeGate.Lock()
		wait := time.Until(tvmazeGate.next)
		tvmazeGate.next = time.Now().Add(max(wait, 0) + 520*time.Millisecond)
		tvmazeGate.Unlock()
		if wait > 0 {
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		req, err := http.NewRequestWithContext(ctx, "GET", t.base()+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "WittyMediaServer (+https://github.com/wittywilla/WittyMediaServer)")
		res, err := c.Do(req)
		if err != nil {
			return err
		}
		if res.StatusCode == 429 {
			res.Body.Close()
			d, _ := strconv.Atoi(res.Header.Get("Retry-After"))
			select {
			case <-time.After(time.Duration(max(d, 2)) * time.Second):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		defer res.Body.Close()
		if res.StatusCode == 404 {
			return ErrNotFound
		}
		if res.StatusCode != 200 {
			b, _ := io.ReadAll(io.LimitReader(res.Body, 300))
			return fmt.Errorf("tvmaze: %s: %s", res.Status, strings.TrimSpace(string(b)))
		}
		return json.NewDecoder(io.LimitReader(res.Body, maxAnswer)).Decode(out)
	}
	return errors.New("tvmaze: too many requests")
}

type mazeShow struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Genres    []string `json:"genres"`
	Status    string   `json:"status"`
	Premiered string   `json:"premiered"`
	Summary   string   `json:"summary"`
	Rating    struct {
		Average *float64 `json:"average"`
	} `json:"rating"`
	Network *struct {
		Name string `json:"name"`
	} `json:"network"`
	WebChannel *struct {
		Name string `json:"name"`
	} `json:"webChannel"`
	Image *mazeImage `json:"image"`
}

type mazeImage struct {
	Medium   string `json:"medium"`
	Original string `json:"original"`
}

func (i *mazeImage) best() string {
	if i == nil {
		return ""
	}
	if i.Original != "" {
		return i.Original
	}
	return i.Medium
}

var htmlTags = regexp.MustCompile(`<[^>]+>`)

// plain turns TVmaze's HTML summaries into text.
func plain(s string) string {
	s = strings.ReplaceAll(s, "</p><p>", "\n\n")
	return strings.TrimSpace(html.UnescapeString(htmlTags.ReplaceAllString(s, "")))
}

// FindSeries searches by name, preferring the given first-air year.
func (t *TVMaze) FindSeries(ctx context.Context, name string, year *int) (*SeriesMeta, error) {
	var res []struct {
		Score float64  `json:"score"`
		Show  mazeShow `json:"show"`
	}
	if err := t.get(ctx, "/search/shows?q="+url.QueryEscape(name), &res); err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, ErrNotFound
	}
	best := res[0].Show
	if year != nil {
		for _, r := range res {
			if y := yearOf(r.Show.Premiered); y != nil && *y == *year {
				best = r.Show
				break
			}
		}
	}
	return t.Series(ctx, strconv.Itoa(best.ID))
}

// Series reads one show with its artwork and season posters.
func (t *TVMaze) Series(ctx context.Context, id string) (*SeriesMeta, error) {
	var sh mazeShow
	if err := t.get(ctx, "/shows/"+url.PathEscape(id), &sh); err != nil {
		return nil, err
	}
	m := &SeriesMeta{ID: strconv.Itoa(sh.ID), Title: sh.Name, Year: yearOf(sh.Premiered), Overview: plain(sh.Summary),
		Genres: sh.Genres, Rating: sh.Rating.Average, Poster: sh.Image.best(), SeasonPosters: map[int]string{}}
	switch sh.Status {
	case "Running":
		m.Status = "Continuing"
	case "Ended":
		m.Status = "Ended"
	default:
		m.Status = sh.Status
	}
	if sh.Network != nil {
		m.Network = sh.Network.Name
	} else if sh.WebChannel != nil {
		m.Network = sh.WebChannel.Name
	}
	var imgs []struct {
		Type        string `json:"type"`
		Main        bool   `json:"main"`
		Resolutions struct {
			Original struct {
				URL string `json:"url"`
			} `json:"original"`
		} `json:"resolutions"`
	}
	if t.get(ctx, "/shows/"+url.PathEscape(id)+"/images", &imgs) == nil {
		for _, im := range imgs {
			u := im.Resolutions.Original.URL
			switch im.Type {
			case "poster":
				m.Posters = append(m.Posters, u)
			case "background":
				if im.Main && m.Backdrop == "" {
					m.Backdrop = u
				}
				m.Backdrops = append(m.Backdrops, u)
			}
		}
		if m.Backdrop == "" && len(m.Backdrops) > 0 {
			m.Backdrop = m.Backdrops[0]
		}
	}
	var seasons []struct {
		Number int        `json:"number"`
		Image  *mazeImage `json:"image"`
	}
	if t.get(ctx, "/shows/"+url.PathEscape(id)+"/seasons", &seasons) == nil {
		for _, s := range seasons {
			if p := s.Image.best(); p != "" {
				m.SeasonPosters[s.Number] = p
			}
		}
	}
	return m, nil
}

// Episodes lists the show's episodes (and specials) by season and number.
func (t *TVMaze) Episodes(ctx context.Context, id string) (map[[2]int]EpisodeMeta, error) {
	var eps []struct {
		ID      int        `json:"id"`
		Name    string     `json:"name"`
		Season  int        `json:"season"`
		Number  *int       `json:"number"`
		Airdate string     `json:"airdate"`
		Runtime *int       `json:"runtime"`
		Summary string     `json:"summary"`
		Image   *mazeImage `json:"image"`
	}
	if err := t.get(ctx, "/shows/"+url.PathEscape(id)+"/episodes?specials=1", &eps); err != nil {
		return nil, err
	}
	out := map[[2]int]EpisodeMeta{}
	special := 0
	for _, e := range eps {
		season, n := e.Season, 0
		if e.Number != nil {
			n = *e.Number
		} else {
			// Specials have no number on TVmaze; number them in airing order
			// as season 0, like most media servers.
			special++
			season, n = 0, special
		}
		m := EpisodeMeta{ID: strconv.Itoa(e.ID), Season: season, Number: n, Title: e.Name, Overview: plain(e.Summary), Aired: e.Airdate, Image: e.Image.best()}
		if e.Runtime != nil {
			m.RuntimeMin = *e.Runtime
		}
		out[[2]int{season, n}] = m
	}
	return out, nil
}

// Name identifies the source in saved metadata.
func (t *TVMaze) Name() string { return "tvmaze" }
