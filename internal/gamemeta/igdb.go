package gamemeta

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// IGDB, through the owner's own Twitch application. Each owner's id has its own
// allowance of 4 requests a second; we use 3. Caching is what its FAQ asks for.
type IGDB struct {
	ClientID, Secret string
	HTTP             *http.Client
	// TokenURL and Base are only changed by tests.
	TokenURL, Base, ImageBase string

	once   sync.Once
	c      *Client
	mu     sync.Mutex
	token  string
	expiry time.Time
}

func (s *IGDB) init() {
	s.once.Do(func() {
		s.c = NewClient("IGDB", s.HTTP, 340*time.Millisecond)
		s.c.Secrets = []string{s.ClientID, s.Secret}
		if s.TokenURL == "" {
			s.TokenURL = "https://id.twitch.tv/oauth2/token"
		}
		if s.Base == "" {
			s.Base = "https://api.igdb.com/v4"
		}
		if s.ImageBase == "" {
			s.ImageBase = "https://images.igdb.com/igdb/image/upload"
		}
	})
}

// About implements Source.
func (s *IGDB) About() About {
	return About{ID: "igdb", Name: "IGDB", Site: "https://www.igdb.com",
		Why:    "The fullest source: covers, art, critic and user scores, and age ratings from every major board.",
		Gives:  []string{"description", "dates", "developer", "publisher", "genres", "players", "cover", "screenshots", "backdrop", "critic score", "user score", "age ratings"},
		Fields: []KeyField{{Setting: "igdbClientId", Label: "Client ID"}, {Setting: "igdbClientSecret", Label: "Client secret"}},
		KeyURL: "https://dev.twitch.tv/console/apps", Cost: "Free for non-commercial use",
		Steps: []string{"Sign in at dev.twitch.tv (a free Twitch account with two-factor sign-in turned on).",
			"Register an application: any name, with http://localhost as the OAuth redirect address, category Application Integration.",
			"Open the application, press New Secret, and copy its Client ID and the secret below."},
		Limit:   "4 requests a second, no monthly limit. This server uses 3.",
		Sends:   "Game names and console names",
		Credit:  "Game data and images from IGDB",
		License: "IGDB's terms, through Twitch's developer agreement. Free for non-commercial use",
	}
}

// oauth gets a token, or reuses the one we have. It lasts about two months.
func (s *IGDB) oauth(ctx context.Context) (string, error) {
	s.init()
	if s.ClientID == "" || s.Secret == "" {
		return "", ErrNotSet
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Now().Before(s.expiry) {
		return s.token, nil
	}
	form := url.Values{"client_id": {s.ClientID}, "client_secret": {s.Secret}, "grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, "POST", s.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := s.c.JSON(req, &out); err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", ErrKey
		}
		return "", err
	}
	if out.AccessToken == "" {
		return "", ErrKey
	}
	s.token = out.AccessToken
	s.expiry = time.Now().Add(time.Duration(max(out.ExpiresIn-3600, 60)) * time.Second)
	return s.token, nil
}

// post asks IGDB something; a token it no longer takes is replaced, once.
func (s *IGDB) post(ctx context.Context, endpoint, body string, out any) error {
	for attempt := 0; ; attempt++ {
		tok, err := s.oauth(ctx)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, "POST", s.Base+"/"+endpoint, strings.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Client-ID", s.ClientID)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "text/plain")
		err = s.c.JSON(req, out)
		if errors.Is(err, ErrKey) && attempt == 0 {
			s.mu.Lock()
			s.token = ""
			s.mu.Unlock()
			continue
		}
		return err
	}
}

const igdbFields = `fields id,name,slug,url,summary,storyline,first_release_date,aggregated_rating,aggregated_rating_count,rating,rating_count,` +
	`cover.image_id,screenshots.image_id,artworks.image_id,genres.name,franchises.name,involved_companies.company.name,involved_companies.developer,involved_companies.publisher,` +
	`multiplayer_modes.offlinemax,multiplayer_modes.offlinecoopmax,game_modes.slug,age_ratings.organization.name,age_ratings.rating_category.rating,platforms;`

type igdbImage struct {
	ImageID string `json:"image_id"`
}
type igdbName struct {
	Name string `json:"name"`
}
type igdbGame struct {
	ID                int64       `json:"id"`
	Name              string      `json:"name"`
	URL               string      `json:"url"`
	Summary           string      `json:"summary"`
	Storyline         string      `json:"storyline"`
	FirstRelease      int64       `json:"first_release_date"`
	AggregatedRating  float64     `json:"aggregated_rating"`
	AggregatedCount   int         `json:"aggregated_rating_count"`
	Rating            float64     `json:"rating"`
	RatingCount       int         `json:"rating_count"`
	Cover             *igdbImage  `json:"cover"`
	Screenshots       []igdbImage `json:"screenshots"`
	Artworks          []igdbImage `json:"artworks"`
	Genres            []igdbName  `json:"genres"`
	Franchises        []igdbName  `json:"franchises"`
	InvolvedCompanies []struct {
		Company   igdbName `json:"company"`
		Developer bool     `json:"developer"`
		Publisher bool     `json:"publisher"`
	} `json:"involved_companies"`
	MultiplayerModes []struct {
		OfflineMax     int `json:"offlinemax"`
		OfflineCoopMax int `json:"offlinecoopmax"`
	} `json:"multiplayer_modes"`
	GameModes []struct {
		Slug string `json:"slug"`
	} `json:"game_modes"`
	AgeRatings []struct {
		Organization   igdbName `json:"organization"`
		RatingCategory struct {
			Rating string `json:"rating"`
		} `json:"rating_category"`
	} `json:"age_ratings"`
}

func (s *IGDB) image(id, size string) string {
	if id == "" {
		return ""
	}
	return s.ImageBase + "/t_" + size + "/" + url.PathEscape(id) + ".jpg"
}

func (s *IGDB) info(g *igdbGame) *Info {
	in := &Info{ID: strconv.FormatInt(g.ID, 10), Title: Line(g.Name), Description: Text(g.Summary), Released: unixDate(g.FirstRelease), URL: WebAddress(g.URL)}
	if in.Description == "" {
		in.Description = Text(g.Storyline)
	}
	for _, c := range g.InvolvedCompanies {
		if c.Developer && in.Developer == "" {
			in.Developer = Line(c.Company.Name)
		}
		if c.Publisher && in.Publisher == "" {
			in.Publisher = Line(c.Company.Name)
		}
	}
	var genres []string
	for _, x := range g.Genres {
		genres = append(genres, x.Name)
	}
	in.Genres = lines(genres, 8)
	if len(g.Franchises) > 0 {
		in.Franchise = Line(g.Franchises[0].Name)
	}
	for _, m := range g.MultiplayerModes {
		in.Players = max(in.Players, m.OfflineMax, m.OfflineCoopMax)
	}
	if in.Players == 0 {
		for _, m := range g.GameModes {
			if m.Slug == "single-player" {
				in.Players = 1
			}
		}
	}
	in.Players = min(in.Players, 64)
	if g.Cover != nil {
		in.Poster = s.image(g.Cover.ImageID, "cover_big_2x")
	}
	for i, a := range g.Artworks {
		if i == 0 {
			in.Backdrop = s.image(a.ImageID, "1080p")
		}
		if i < 4 {
			in.Art = append(in.Art, ArtRef{Ref: s.image(a.ImageID, "1080p"), Kind: "artwork"})
		}
	}
	for i, a := range g.Screenshots {
		if i >= 8 {
			break
		}
		in.Art = append(in.Art, ArtRef{Ref: s.image(a.ImageID, "screenshot_huge"), Kind: "screenshot"})
		if in.Backdrop == "" {
			in.Backdrop = s.image(a.ImageID, "1080p")
		}
	}
	if g.AggregatedRating > 0 && g.AggregatedRating <= 100 {
		in.Scores = append(in.Scores, Score{Source: "IGDB critics", Kind: "critic", Value: round1(g.AggregatedRating), Votes: g.AggregatedCount, URL: in.URL})
	}
	if g.Rating > 0 && g.Rating <= 100 {
		in.Scores = append(in.Scores, Score{Source: "IGDB users", Kind: "user", Value: round1(g.Rating), Votes: g.RatingCount, URL: in.URL})
	}
	for _, a := range g.AgeRatings {
		if r, ok := NewAge(a.Organization.Name, a.RatingCategory.Rating); ok {
			in.Ages = append(in.Ages, r)
		}
	}
	return in
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

func escapeApicalypse(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", " ", "\r", " ").Replace(s)
}

func (s *IGDB) search(ctx context.Context, console, title string, limit int) ([]igdbGame, error) {
	q := `search "` + escapeApicalypse(title) + `"; ` + igdbFields + ` limit ` + strconv.Itoa(limit) + `;`
	if p, ok := platforms[console]; ok && len(p.igdb) > 0 {
		q = `search "` + escapeApicalypse(title) + `"; ` + igdbFields + ` where platforms = (` + joinInts(p.igdb, ",") + `); limit ` + strconv.Itoa(limit) + `;`
	}
	var out []igdbGame
	return out, s.post(ctx, "games", q, &out)
}

// Lookup implements Source.
func (s *IGDB) Lookup(ctx context.Context, q Query) (*Info, error) {
	games, err := s.search(ctx, q.Console, q.Title, 10)
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
	in := s.info(&games[i])
	in.Method, in.Confidence = MethodName, nameConfidence(score)
	return in, nil
}

// Search implements Source.
func (s *IGDB) Search(ctx context.Context, console, title string) ([]Candidate, error) {
	games, err := s.search(ctx, console, title, 15)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for i := range games {
		g := &games[i]
		c := Candidate{Source: "igdb", ID: strconv.FormatInt(g.ID, 10), Title: Line(g.Name), Year: firstYear(unixDate(g.FirstRelease)), Score: NameScore(title, g.Name)}
		if g.Cover != nil {
			c.Poster = s.image(g.Cover.ImageID, "cover_small")
		}
		out = append(out, c)
	}
	return out, nil
}

// Fetch implements Source.
func (s *IGDB) Fetch(ctx context.Context, console, id string) (*Info, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, ErrNotFound
	}
	var games []igdbGame
	if err := s.post(ctx, "games", igdbFields+` where id = `+strconv.FormatInt(n, 10)+`; limit 1;`, &games); err != nil {
		return nil, err
	}
	if len(games) == 0 {
		return nil, ErrNotFound
	}
	in := s.info(&games[0])
	in.Method, in.Confidence = MethodManual, 100
	return in, nil
}

// Test implements Source.
func (s *IGDB) Test(ctx context.Context) (string, error) {
	var games []struct {
		ID int64 `json:"id"`
	}
	if err := s.post(ctx, "games", `fields id; limit 1;`, &games); err != nil {
		return "", err
	}
	return "IGDB accepted these details.", nil
}
