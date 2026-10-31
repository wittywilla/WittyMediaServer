package gamemeta

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// RAWG, with the owner's own free key. 20,000 requests a month, and its terms ask for
// a link back on every page showing its data, which the game page gives.
type RAWG struct {
	Key  string
	HTTP *http.Client
	// Base is only changed by tests.
	Base string

	once sync.Once
	c    *Client
}

func (s *RAWG) init() {
	s.once.Do(func() {
		s.c = NewClient("RAWG", s.HTTP, 500*time.Millisecond)
		s.c.Secrets = []string{s.Key}
		if s.Base == "" {
			s.Base = "https://api.rawg.io/api"
		}
	})
}

// About implements Source.
func (s *RAWG) About() About {
	return About{ID: "rawg", Name: "RAWG", Site: "https://rawg.io",
		Why:    "Metacritic scores, user scores and wide backdrops, for newer and better-known games.",
		Gives:  []string{"description", "dates", "developer", "publisher", "genres", "backdrop", "screenshots", "critic score", "user score", "ESRB rating"},
		Fields: []KeyField{{Setting: "rawgKey", Label: "API key"}},
		KeyURL: "https://rawg.io/login/?forward=developer", Cost: "Free",
		Steps: []string{"Make a free account at rawg.io.", "Open the API key page and fill in the short form.", "Copy the key below."},
		Limit: "20,000 requests a month. This server stops at 18,000 and carries on the next month.",
		Sends: "Game names and console names", Credit: "Data and images from RAWG", License: "RAWG's API terms: free with a link back to RAWG",
	}
}

// Allowance implements Metered.
func (s *RAWG) Allowance() (int, string) { return 18000, "month" }

// SetMeter implements Metered.
func (s *RAWG) SetMeter(spend func(int) error) { s.init(); s.c.meter = spend }

// GapsOnly implements GapFiller: two requests a game adds up quickly.
func (s *RAWG) GapsOnly() bool { return true }

type rawgName struct {
	Name string `json:"name"`
}
type rawgGame struct {
	ID          int64      `json:"id"`
	Slug        string     `json:"slug"`
	Name        string     `json:"name"`
	Released    string     `json:"released"`
	Background  string     `json:"background_image"`
	Additional  string     `json:"background_image_additional"`
	Rating      float64    `json:"rating"`
	RatingTop   float64    `json:"rating_top"`
	Ratings     int        `json:"ratings_count"`
	Metacritic  int        `json:"metacritic"`
	MetaURL     string     `json:"metacritic_url"`
	Description string     `json:"description"`
	Raw         string     `json:"description_raw"`
	Genres      []rawgName `json:"genres"`
	Developers  []rawgName `json:"developers"`
	Publishers  []rawgName `json:"publishers"`
	ESRB        *rawgName  `json:"esrb_rating"`
	Shots       []struct {
		Image string `json:"image"`
	} `json:"short_screenshots"`
}

func (s *RAWG) get(ctx context.Context, path string, q url.Values, out any) error {
	s.init()
	if s.Key == "" {
		return ErrNotSet
	}
	if q == nil {
		q = url.Values{}
	}
	q.Set("key", s.Key)
	return s.c.Get(ctx, s.Base+path+"?"+q.Encode(), out)
}

func (s *RAWG) search(ctx context.Context, console, title string, size int) ([]rawgGame, error) {
	q := url.Values{"search": {title}, "search_precise": {"true"}, "page_size": {strconv.Itoa(size)}}
	if p, ok := platforms[console]; ok && len(p.rawg) > 0 {
		q.Set("platforms", joinInts(p.rawg, ","))
	}
	var out struct {
		Results []rawgGame `json:"results"`
	}
	return out.Results, s.get(ctx, "/games", q, &out)
}

// rawgESRB turns RAWG's words into the board's own letters.
var rawgESRB = map[string]string{"Everyone": "E", "Everyone 10+": "E10+", "Teen": "T", "Mature": "M", "Adults Only": "AO"}

func (s *RAWG) info(g *rawgGame) *Info {
	in := &Info{ID: strconv.FormatInt(g.ID, 10), Title: Line(g.Name), Released: cleanDate(g.Released), Backdrop: WebAddress(g.Background)}
	if g.Slug != "" {
		in.URL = "https://rawg.io/games/" + url.PathEscape(g.Slug)
	}
	in.Description = Text(g.Raw)
	if in.Description == "" {
		in.Description = Text(g.Description)
	}
	names := func(l []rawgName) []string {
		var out []string
		for _, x := range l {
			out = append(out, x.Name)
		}
		return out
	}
	in.Genres = lines(names(g.Genres), 8)
	if d := lines(names(g.Developers), 1); len(d) > 0 {
		in.Developer = d[0]
	}
	if p := lines(names(g.Publishers), 1); len(p) > 0 {
		in.Publisher = p[0]
	}
	if a := WebAddress(g.Additional); a != "" {
		in.Art = append(in.Art, ArtRef{Ref: a, Kind: "artwork"})
	}
	for i, sh := range g.Shots {
		// The first of these is the background image again.
		if a := WebAddress(sh.Image); a != "" && i > 0 && len(in.Art) < 8 {
			in.Art = append(in.Art, ArtRef{Ref: a, Kind: "screenshot"})
		}
	}
	if g.Metacritic > 0 && g.Metacritic <= 100 {
		in.Scores = append(in.Scores, Score{Source: "Metacritic", Kind: "critic", Value: float64(g.Metacritic), URL: WebAddress(g.MetaURL)})
	}
	if g.Rating > 0 && g.Ratings > 0 {
		top := g.RatingTop
		if top <= 0 {
			top = 5
		}
		if v := g.Rating / top * 100; v <= 100 {
			in.Scores = append(in.Scores, Score{Source: "RAWG users", Kind: "user", Value: round1(v), Votes: g.Ratings, URL: in.URL})
		}
	}
	if g.ESRB != nil {
		if r, ok := NewAge("ESRB", rawgESRB[g.ESRB.Name]); ok {
			in.Ages = append(in.Ages, r)
		}
	}
	return in
}

// Lookup implements Source.
func (s *RAWG) Lookup(ctx context.Context, q Query) (*Info, error) {
	games, err := s.search(ctx, q.Console, q.Title, 8)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(games))
	for i := range games {
		names[i] = games[i].Name
	}
	i, score := Best(q.Title, names)
	if i < 0 || score < AcceptScore {
		return nil, ErrNotFound
	}
	// A search leaves out the description and who made it.
	in, err := s.Fetch(ctx, q.Console, strconv.FormatInt(games[i].ID, 10))
	if err != nil {
		return nil, err
	}
	in.Method, in.Confidence = MethodName, nameConfidence(score)
	if len(in.Art) == 0 {
		in.Art = s.info(&games[i]).Art
	}
	return in, nil
}

// Search implements Source.
func (s *RAWG) Search(ctx context.Context, console, title string) ([]Candidate, error) {
	games, err := s.search(ctx, console, title, 15)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for i := range games {
		g := &games[i]
		out = append(out, Candidate{Source: "rawg", ID: strconv.FormatInt(g.ID, 10), Title: Line(g.Name), Year: firstYear(g.Released), Poster: WebAddress(g.Background), Score: NameScore(title, g.Name)})
	}
	return out, nil
}

// Fetch implements Source.
func (s *RAWG) Fetch(ctx context.Context, console, id string) (*Info, error) {
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return nil, ErrNotFound
	}
	var g rawgGame
	if err := s.get(ctx, "/games/"+id, nil, &g); err != nil {
		return nil, err
	}
	if g.ID == 0 {
		return nil, ErrNotFound
	}
	in := s.info(&g)
	in.Method, in.Confidence = MethodManual, 100
	return in, nil
}

// Test implements Source.
func (s *RAWG) Test(ctx context.Context) (string, error) {
	var out struct {
		Count int `json:"count"`
	}
	if err := s.get(ctx, "/platforms", url.Values{"page_size": {"1"}}, &out); err != nil {
		return "", err
	}
	return "RAWG accepted this key.", nil
}
