package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AniList reads anime metadata from anilist.co: free, no key. Anime
// libraries use it for show details; TitleLang picks English, rōmaji or
// native titles. AniList marks adult titles, which become an 18 rating so
// parental controls hide them.
type AniList struct {
	Client    *http.Client
	TitleLang string // english (default) | romaji | native
	BaseURL   string // tests
}

// AniList allows about 30 requests a minute at the moment; space them out.
var anilistGate = struct {
	sync.Mutex
	next time.Time
}{}

const anilistFields = `id title { romaji english native } description(asHtml: false) startDate { year }
 genres averageScore status episodes isAdult coverImage { extraLarge large } bannerImage
 studios(isMain: true) { nodes { name } } streamingEpisodes { title thumbnail }`

type alMedia struct {
	ID    int `json:"id"`
	Title struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
		Native  string `json:"native"`
	} `json:"title"`
	Description string `json:"description"`
	StartDate   struct {
		Year *int `json:"year"`
	} `json:"startDate"`
	Genres       []string `json:"genres"`
	AverageScore *int     `json:"averageScore"`
	Status       string   `json:"status"`
	Episodes     *int     `json:"episodes"`
	IsAdult      bool     `json:"isAdult"`
	CoverImage   struct {
		ExtraLarge string `json:"extraLarge"`
		Large      string `json:"large"`
	} `json:"coverImage"`
	BannerImage string `json:"bannerImage"`
	Studios     struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"studios"`
	StreamingEpisodes []struct {
		Title     string `json:"title"`
		Thumbnail string `json:"thumbnail"`
	} `json:"streamingEpisodes"`
}

func (a *AniList) query(ctx context.Context, q string, vars map[string]any, out any) error {
	anilistGate.Lock()
	wait := time.Until(anilistGate.next)
	anilistGate.next = time.Now().Add(max(wait, 0) + 2100*time.Millisecond)
	anilistGate.Unlock()
	if wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	body, _ := json.Marshal(map[string]any{"query": q, "variables": vars})
	u := "https://graphql.anilist.co"
	if a.BaseURL != "" {
		u = a.BaseURL
	}
	req, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	c := a.Client
	if c == nil {
		c = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return ErrNotFound
	}
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("anilist: %s: %s", res.Status, strings.TrimSpace(string(b)))
	}
	var wrap struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(res.Body).Decode(&wrap); err != nil {
		return err
	}
	if len(wrap.Errors) > 0 {
		if strings.Contains(strings.ToLower(wrap.Errors[0].Message), "not found") {
			return ErrNotFound
		}
		return errors.New("anilist: " + wrap.Errors[0].Message)
	}
	return json.Unmarshal(wrap.Data, out)
}

func (a *AniList) media(ctx context.Context, q string, vars map[string]any) (*alMedia, error) {
	var out struct {
		Media *alMedia `json:"Media"`
	}
	if err := a.query(ctx, q, vars, &out); err != nil {
		return nil, err
	}
	if out.Media == nil {
		return nil, ErrNotFound
	}
	return out.Media, nil
}

func (a *AniList) title(m *alMedia) string {
	switch a.TitleLang {
	case "romaji":
		if m.Title.Romaji != "" {
			return m.Title.Romaji
		}
	case "native":
		if m.Title.Native != "" {
			return m.Title.Native
		}
	}
	if m.Title.English != "" {
		return m.Title.English
	}
	return m.Title.Romaji
}

var alBreak = regexp.MustCompile(`(?i)<br\s*/?>`)

func (a *AniList) meta(m *alMedia) *SeriesMeta {
	sm := &SeriesMeta{ID: strconv.Itoa(m.ID), Title: a.title(m), Year: m.StartDate.Year, Genres: append([]string{"Anime"}, m.Genres...),
		Overview: plain(alBreak.ReplaceAllString(m.Description, "\n")), Backdrop: m.BannerImage, SeasonPosters: map[int]string{}}
	sm.Poster = m.CoverImage.ExtraLarge
	if sm.Poster == "" {
		sm.Poster = m.CoverImage.Large
	}
	if sm.Poster != "" {
		sm.Posters = []string{sm.Poster}
	}
	if sm.Backdrop != "" {
		sm.Backdrops = []string{sm.Backdrop}
	}
	if m.AverageScore != nil {
		r := float64(*m.AverageScore) / 10
		sm.Rating = &r
	}
	switch m.Status {
	case "RELEASING":
		sm.Status = "Continuing"
	case "FINISHED":
		sm.Status = "Ended"
	case "NOT_YET_RELEASED":
		sm.Status = "Upcoming"
	}
	if len(m.Studios.Nodes) > 0 {
		sm.Network = m.Studios.Nodes[0].Name
	}
	if m.IsAdult {
		sm.ContentRating = "18" // AniList's adult flag: hidden from Kids and Guest profiles
	}
	return sm
}

// FindSeries searches anime by name (any title language), preferring the year.
func (a *AniList) FindSeries(ctx context.Context, name string, year *int) (*SeriesMeta, error) {
	vars := map[string]any{"s": name}
	q := `query ($s: String) { Media(search: $s, type: ANIME) { ` + anilistFields + ` } }`
	if year != nil {
		vars["y"] = *year
		q = `query ($s: String, $y: Int) { Media(search: $s, type: ANIME, seasonYear: $y) { ` + anilistFields + ` } }`
	}
	m, err := a.media(ctx, q, vars)
	if errors.Is(err, ErrNotFound) && year != nil {
		delete(vars, "y")
		m, err = a.media(ctx, `query ($s: String) { Media(search: $s, type: ANIME) { `+anilistFields+` } }`, vars)
	}
	if err != nil {
		return nil, err
	}
	return a.meta(m), nil
}

// Series reads one anime by its AniList id.
func (a *AniList) Series(ctx context.Context, id string) (*SeriesMeta, error) {
	n, err := strconv.Atoi(id)
	if err != nil {
		return nil, ErrNotFound
	}
	m, err := a.media(ctx, `query ($id: Int) { Media(id: $id, type: ANIME) { `+anilistFields+` } }`, map[string]any{"id": n})
	if err != nil {
		return nil, err
	}
	return a.meta(m), nil
}

var alEpisodeTitle = regexp.MustCompile(`(?i)^episode\s+(\d+)\s*[-:]\s*(.*)$`)

// Episodes uses AniList's streaming-episode list for episode names and
// stills. Each AniList entry is one season, so they are season 1 (and the
// absolute numbers anime releases use).
func (a *AniList) Episodes(ctx context.Context, id string) (map[[2]int]EpisodeMeta, error) {
	n, err := strconv.Atoi(id)
	if err != nil {
		return nil, ErrNotFound
	}
	m, err := a.media(ctx, `query ($id: Int) { Media(id: $id, type: ANIME) { id streamingEpisodes { title thumbnail } } }`, map[string]any{"id": n})
	if err != nil {
		return nil, err
	}
	out := map[[2]int]EpisodeMeta{}
	for i, e := range m.StreamingEpisodes {
		num, title := i+1, e.Title
		if mm := alEpisodeTitle.FindStringSubmatch(e.Title); mm != nil {
			num, _ = strconv.Atoi(mm[1])
			title = mm[2]
		}
		out[[2]int{1, num}] = EpisodeMeta{ID: id + "-" + strconv.Itoa(num), Season: 1, Number: num, Title: title, Image: e.Thumbnail}
	}
	return out, nil
}

// Name identifies the source in saved metadata.
func (a *AniList) Name() string { return "anilist" }
