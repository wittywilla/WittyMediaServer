package gamemeta

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TheGamesDB, with a key from its site. It counts requests by month and says how many
// are left in every answer, so this listens rather than guessing. It knows ROM
// checksums, but mostly it's here for box art.
type TGDB struct {
	Key  string
	HTTP *http.Client
	// Base is only changed by tests.
	Base string

	once sync.Once
	c    *Client
	mu   sync.Mutex
	// Names for the ids games come with; they rarely change, so they're fetched once.
	developers, publishers, genres map[string]string
}

func (s *TGDB) init() {
	s.once.Do(func() {
		s.c = NewClient("TheGamesDB", s.HTTP, time.Second)
		s.c.Secrets = []string{s.Key}
		if s.Base == "" {
			s.Base = "https://api.thegamesdb.net"
		}
	})
}

// About implements Source.
func (s *TGDB) About() About {
	return About{ID: "tgdb", Name: "TheGamesDB", Site: "https://thegamesdb.net",
		Why:    "Box art and fan art from a community database, and it can recognise some files by checksum.",
		Gives:  []string{"description", "dates", "developer", "publisher", "genres", "players", "cover", "screenshots", "backdrop", "age rating"},
		Fields: []KeyField{{Setting: "tgdbKey", Label: "API key"}},
		KeyURL: "https://api.thegamesdb.net/key.php", Cost: "Free",
		Steps: []string{"Make a free account at forums.thegamesdb.net.", "While signed in, open the API key page.", "Copy the key below."},
		Limit: "A monthly allowance (about 3,000 requests). It's only asked about games still missing something, and stops when the allowance runs low.",
		Sends: "Game names, console names and file checksums", Credit: "Data and images from TheGamesDB", License: "No written terms. Credit given and images kept on this server",
	}
}

// GapsOnly implements GapFiller: its allowance is smaller than some collections.
func (s *TGDB) GapsOnly() bool { return true }

// tgdbAnswer is the wrapper round every answer.
type tgdbAnswer struct {
	Code   int             `json:"code"`
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
	Incl   struct {
		Boxart struct {
			BaseURL struct {
				Original string `json:"original"`
				Medium   string `json:"medium"`
				Thumb    string `json:"thumb"`
			} `json:"base_url"`
			Data map[string][]tgdbImage `json:"data"`
		} `json:"boxart"`
	} `json:"include"`
	Remaining *int `json:"remaining_monthly_allowance"`
	Extra     *int `json:"extra_allowance"`
	Refresh   *int `json:"allowance_refresh_timer"`
}

type tgdbImage struct {
	Type     string `json:"type"`
	Side     string `json:"side"`
	Filename string `json:"filename"`
}

type tgdbGame struct {
	ID         int64    `json:"id"`
	Title      string   `json:"game_title"`
	Released   string   `json:"release_date"`
	Overview   string   `json:"overview"`
	Players    int      `json:"players"`
	Rating     string   `json:"rating"`
	Developers []int    `json:"developers"`
	Publishers []int    `json:"publishers"`
	Genres     []int    `json:"genres"`
	Platform   int      `json:"platform"`
	Alternates []string `json:"alternates"`
}

// tgdbFloor is how much of the month's allowance is left alone.
const tgdbFloor = 50

func (s *TGDB) get(ctx context.Context, path string, q url.Values) (*tgdbAnswer, error) {
	s.init()
	if s.Key == "" {
		return nil, ErrNotSet
	}
	if q == nil {
		q = url.Values{}
	}
	q.Set("apikey", s.Key)
	var out tgdbAnswer
	if err := s.c.Get(ctx, s.Base+path+"?"+q.Encode(), &out); err != nil {
		return nil, err
	}
	if out.Remaining != nil && *out.Remaining+intOr(out.Extra) < tgdbFloor {
		wait := 7 * 24 * time.Hour
		if out.Refresh != nil && *out.Refresh > 0 {
			wait = time.Duration(*out.Refresh) * time.Second
		}
		s.c.gate.pause(wait, "the monthly allowance is nearly used")
	}
	return &out, nil
}

func intOr(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

const tgdbGameFields = "players,publishers,genres,overview,rating,platform,alternates"

func (s *TGDB) games(a *tgdbAnswer) []tgdbGame {
	var d struct {
		Games []tgdbGame `json:"games"`
	}
	_ = json.Unmarshal(a.Data, &d)
	return d.Games
}

// names resolves a list of ids against one of TheGamesDB's lookup lists. Failing is
// fine: an id isn't worth showing, so the field just stays empty.
func (s *TGDB) names(ctx context.Context, which string, have *map[string]string, ids []int) []string {
	if len(ids) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if *have == nil {
		a, err := s.get(ctx, "/v1/"+strings.Title(which), nil) //nolint:staticcheck
		if err != nil {
			return nil
		}
		// The list comes beside other things, such as a count, so only it is read.
		var d map[string]json.RawMessage
		if json.Unmarshal(a.Data, &d) != nil {
			return nil
		}
		var list map[string]struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(d[which], &list) != nil {
			return nil
		}
		m := map[string]string{}
		for id, x := range list {
			m[id] = x.Name
		}
		*have = m
	}
	var out []string
	for _, id := range ids {
		if n := (*have)[strconv.Itoa(id)]; n != "" {
			out = append(out, n)
		}
	}
	return out
}

func tgdbAge(rating string) (AgeRating, bool) {
	// Ratings look like "E - Everyone" or "T - Teen".
	first, _, _ := strings.Cut(strings.TrimSpace(rating), " ")
	return NewAge("ESRB", first)
}

func (s *TGDB) pictures(in *Info, a *tgdbAnswer, id int64) {
	base := WebAddress(a.Incl.Boxart.BaseURL.Original)
	if base == "" {
		return
	}
	for _, img := range a.Incl.Boxart.Data[strconv.FormatInt(id, 10)] {
		u := WebAddress(base + img.Filename)
		if u == "" {
			continue
		}
		switch {
		case img.Type == "boxart" && img.Side == "front" && in.Poster == "":
			in.Poster = u
		case img.Type == "fanart" && in.Backdrop == "":
			in.Backdrop = u
		case img.Type == "fanart" && len(in.Art) < 8:
			in.Art = append(in.Art, ArtRef{Ref: u, Kind: "artwork"})
		case img.Type == "screenshot" && len(in.Art) < 8:
			in.Art = append(in.Art, ArtRef{Ref: u, Kind: "screenshot"})
		case img.Type == "titlescreen" && len(in.Art) < 8:
			in.Art = append(in.Art, ArtRef{Ref: u, Kind: "title"})
		}
	}
}

func (s *TGDB) info(ctx context.Context, g *tgdbGame, a *tgdbAnswer) *Info {
	in := &Info{ID: strconv.FormatInt(g.ID, 10), Title: Line(g.Title), Released: cleanDate(g.Released), Description: Text(g.Overview),
		URL: "https://thegamesdb.net/game.php?id=" + strconv.FormatInt(g.ID, 10)}
	if g.Players > 0 && g.Players <= 64 {
		in.Players = g.Players
	}
	if d := lines(s.names(ctx, "developers", &s.developers, g.Developers), 1); len(d) > 0 {
		in.Developer = d[0]
	}
	if d := lines(s.names(ctx, "publishers", &s.publishers, g.Publishers), 1); len(d) > 0 {
		in.Publisher = d[0]
	}
	in.Genres = lines(s.names(ctx, "genres", &s.genres, g.Genres), 8)
	if r, ok := tgdbAge(g.Rating); ok {
		in.Ages = append(in.Ages, r)
	}
	s.pictures(in, a, g.ID)
	return in
}

func (s *TGDB) platform(console string) string {
	if p, ok := platforms[console]; ok && len(p.tgdb) > 0 {
		return joinInts(p.tgdb, ",")
	}
	return ""
}

func (s *TGDB) search(ctx context.Context, console, title string) (*tgdbAnswer, []tgdbGame, error) {
	q := url.Values{"name": {title}, "fields": {tgdbGameFields}, "include": {"boxart"}}
	if p := s.platform(console); p != "" {
		q.Set("filter[platform]", p)
	}
	a, err := s.get(ctx, "/v1.1/Games/ByGameName", q)
	if err != nil {
		return nil, nil, err
	}
	return a, s.games(a), nil
}

// Lookup implements Source.
func (s *TGDB) Lookup(ctx context.Context, q Query) (*Info, error) {
	// A checksum costs one request; so does a name. Files are the better question.
	if len(q.CRCs) > 0 {
		cq := url.Values{"hash": {fmt.Sprintf("%08x", q.CRCs[0])}, "filter[type]": {"crc"}, "fields": {tgdbGameFields}, "include": {"boxart"}}
		if p := s.platform(q.Console); p != "" {
			cq.Set("filter[platform]", p)
		}
		a, err := s.get(ctx, "/v1/Games/ByGameHash", cq)
		if err != nil && !isMissing(err) {
			return nil, err
		}
		if err == nil {
			if games := s.games(a); len(games) > 0 {
				in := s.info(ctx, &games[0], a)
				in.Method, in.Confidence = MethodChecksum, 100
				return in, nil
			}
		}
	}
	a, games, err := s.search(ctx, q.Console, q.Title)
	if err != nil {
		return nil, err
	}
	bestI, best := -1, 0
	for i := range games {
		names := append([]string{games[i].Title}, games[i].Alternates...)
		if _, score := Best(q.Title, names); score > best {
			bestI, best = i, score
		}
	}
	if bestI < 0 || best < AcceptScore {
		return nil, ErrNotFound
	}
	in := s.info(ctx, &games[bestI], a)
	in.Method, in.Confidence = MethodName, nameConfidence(best)
	return in, nil
}

func isMissing(err error) bool { return err == ErrNotFound }

// Search implements Source.
func (s *TGDB) Search(ctx context.Context, console, title string) ([]Candidate, error) {
	a, games, err := s.search(ctx, console, title)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for i := range games {
		g := &games[i]
		c := Candidate{Source: "tgdb", ID: strconv.FormatInt(g.ID, 10), Title: Line(g.Title), Year: firstYear(g.Released), Score: NameScore(title, g.Title)}
		var in Info
		s.pictures(&in, a, g.ID)
		c.Poster = in.Poster
		out = append(out, c)
		if len(out) >= 15 {
			break
		}
	}
	return out, nil
}

// Fetch implements Source.
func (s *TGDB) Fetch(ctx context.Context, console, id string) (*Info, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, ErrNotFound
	}
	a, err := s.get(ctx, "/v1/Games/ByGameID", url.Values{"id": {strconv.FormatInt(n, 10)}, "fields": {tgdbGameFields}, "include": {"boxart"}})
	if err != nil {
		return nil, err
	}
	games := s.games(a)
	if len(games) == 0 {
		return nil, ErrNotFound
	}
	in := s.info(ctx, &games[0], a)
	in.Method, in.Confidence = MethodManual, 100
	return in, nil
}

// Test implements Source.
func (s *TGDB) Test(ctx context.Context) (string, error) {
	s.init()
	if s.Key == "" {
		return "", ErrNotSet
	}
	var out struct {
		Remaining *int `json:"remaining_monthly_allowance"`
		Extra     *int `json:"extra_allowance"`
	}
	if err := s.c.Get(ctx, s.Base+"/v1/API/Limit?apikey="+url.QueryEscape(s.Key), &out); err != nil {
		return "", err
	}
	if out.Remaining != nil {
		return fmt.Sprintf("TheGamesDB accepted this key. %d requests left this month.", *out.Remaining+intOr(out.Extra)), nil
	}
	return "TheGamesDB accepted this key.", nil
}
