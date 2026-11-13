// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gamemeta

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"
)

// RetroAchievements lists a game's achievements, through the owner's own Web API key.
// It isn't asked about games while the library is looked up: it's asked when someone
// opens a game, and only then (see Sidecar).
//
// Nothing about a game file leaves this server. Each console's list of games, with
// every checksum each is known by, is fetched whole and kept for a week; the file's
// checksum is looked up in it here. A game's achievements are kept for three days, and
// their pictures for good, as a picture's name changes when it does.
type RetroAchievements struct {
	Key  string
	HTTP *http.Client
	// Dir holds what's kept between requests and restarts.
	Dir string
	// Base and MediaBase are only changed by tests.
	Base, MediaBase string
	// Gap is the least time between requests: a second, unless a test says otherwise.
	Gap time.Duration

	once  sync.Once
	c     *Client
	media *Client

	mu    sync.Mutex // one console's list or one game's achievements fetched at a time
	lists map[int]*raList
	games map[int]*RAGame
}

// Sidecar is a source that isn't asked about games during lookups. It's listed with
// the others, for its key and its switch, and serves something else.
type Sidecar interface {
	Sidecar() bool
}

// How long what RetroAchievements said is used before asking again. Asking for a whole
// console's list is what its documentation asks to be cached aggressively.
const (
	raListFresh = 7 * 24 * time.Hour
	raGameFresh = 3 * 24 * time.Hour
	raBadgeMax  = 512 << 10
)

// RAAchievement is one achievement, as it's shown.
type RAAchievement struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Points      int    `json:"points"`
	Badge       string `json:"badge"` // its picture's name at RetroAchievements
	Order       int    `json:"order"`
}

// RAGame is one game and its achievements.
type RAGame struct {
	ID           int             `json:"id"`
	Title        string          `json:"title"`
	Points       int             `json:"points"`
	Achievements []RAAchievement `json:"achievements"`
	Fetched      time.Time       `json:"fetched"`
}

// URL is the game's page at RetroAchievements.
func (g *RAGame) URL() string { return "https://retroachievements.org/game/" + strconv.Itoa(g.ID) }

// HasBadge says whether this picture is one of the game's.
func (g *RAGame) HasBadge(name string) bool {
	for _, a := range g.Achievements {
		if a.Badge == name {
			return true
		}
	}
	return false
}

type raListGame struct {
	ID     int      `json:"id"`
	Title  string   `json:"title"`
	Points int      `json:"points"`
	Count  int      `json:"count"`
	Hashes []string `json:"hashes"`
}

type raList struct {
	Fetched time.Time    `json:"fetched"`
	Games   []raListGame `json:"games"`
	byHash  map[string]int
}

func (s *RetroAchievements) init() {
	s.once.Do(func() {
		// No published limit; one request a second is well within anything reasonable.
		if s.Gap <= 0 {
			s.Gap = time.Second
		}
		s.c = NewClient("RetroAchievements", s.HTTP, s.Gap)
		s.c.Secrets = []string{s.Key}
		s.media = NewClient("RetroAchievements", s.HTTP, 100*time.Millisecond)
		if s.Base == "" {
			s.Base = "https://retroachievements.org/API"
		}
		if s.MediaBase == "" {
			s.MediaBase = "https://media.retroachievements.org"
		}
		s.lists = map[int]*raList{}
		s.games = map[int]*RAGame{}
	})
}

// About implements Source.
func (s *RetroAchievements) About() About {
	return About{ID: "retroachievements", Name: "RetroAchievements", Site: "https://retroachievements.org",
		Why:    "Lists each game's achievements, found by the exact game file, to show with the game. They aren't earned or tracked yet.",
		Gives:  []string{"achievements"},
		Fields: []KeyField{{Setting: "retroAchievementsKey", Label: "Web API key"}},
		KeyURL: "https://retroachievements.org/settings", Cost: "Free",
		Steps: []string{"Make a free account at retroachievements.org, or sign in.",
			"Open Settings, and find Authentication (Web API key).",
			"Copy the Web API key below."},
		Limit:   "No published limit. This server asks at most once a second, keeps each console's list of games for a week and each game's achievements for three days.",
		Sends:   "Console and game numbers. Checksums of your games are matched on this server and never sent",
		Credit:  "Achievements from RetroAchievements.org",
		License: "RetroAchievements' terms of use",
	}
}

// Sidecar implements Sidecar.
func (s *RetroAchievements) Sidecar() bool { return true }

// Lookup implements Source: this source isn't asked about games that way.
func (s *RetroAchievements) Lookup(ctx context.Context, q Query) (*Info, error) {
	return nil, ErrNotFound
}

// Search implements Source.
func (s *RetroAchievements) Search(ctx context.Context, console, title string) ([]Candidate, error) {
	return nil, nil
}

// Fetch implements Source.
func (s *RetroAchievements) Fetch(ctx context.Context, console, id string) (*Info, error) {
	return nil, ErrNotFound
}

// Test implements Source, with the smallest thing its API answers.
func (s *RetroAchievements) Test(ctx context.Context) (string, error) {
	s.init()
	if s.Key == "" {
		return "", ErrNotSet
	}
	var out []struct {
		ID int `json:"ID"`
	}
	if err := s.get(ctx, "API_GetConsoleIDs.php", url.Values{"g": {"1"}}, &out); err != nil {
		return "", err
	}
	if len(out) == 0 {
		return "", errors.New("RetroAchievements answered with no consoles")
	}
	return "RetroAchievements accepted the key.", nil
}

func (s *RetroAchievements) get(ctx context.Context, endpoint string, q url.Values, out any) error {
	if s.Key == "" {
		return ErrNotSet
	}
	q.Set("y", s.Key)
	return s.c.Get(ctx, s.Base+"/"+endpoint+"?"+q.Encode(), out)
}

func readKept(p string, out any) bool {
	b, err := os.ReadFile(p)
	return err == nil && json.Unmarshal(b, out) == nil
}

func keep(p string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return keepBytes(p, b)
}

// keepBytes writes a file whole or not at all, so a reader never sees half of one.
func keepBytes(p string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".keep-*")
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), p)
	}
	if err != nil {
		_ = os.Remove(f.Name())
	}
	return err
}

// list is one console's games and their checksums. A list that can't be refreshed is
// used as it is.
func (s *RetroAchievements) list(ctx context.Context, console int) (*raList, error) {
	if l := s.lists[console]; l != nil && time.Since(l.Fetched) < raListFresh {
		return l, nil
	}
	p := filepath.Join(s.Dir, "lists", strconv.Itoa(console)+".json")
	var kept raList
	have := readKept(p, &kept)
	if have && time.Since(kept.Fetched) < raListFresh {
		kept.index()
		s.lists[console] = &kept
		return &kept, nil
	}
	var raw []struct {
		ID              int      `json:"ID"`
		Title           string   `json:"Title"`
		Points          int      `json:"Points"`
		NumAchievements int      `json:"NumAchievements"`
		Hashes          []string `json:"Hashes"`
	}
	// Only games with achievements: the rest have nothing to show.
	err := s.get(ctx, "API_GetGameList.php", url.Values{"i": {strconv.Itoa(console)}, "f": {"1"}, "h": {"1"}}, &raw)
	if err != nil {
		if have && !errors.Is(err, ErrKey) && !errors.Is(err, ErrNotSet) {
			kept.index()
			s.lists[console] = &kept
			return &kept, nil
		}
		return nil, err
	}
	l := &raList{Fetched: time.Now()}
	for _, g := range raw {
		if g.ID <= 0 {
			continue
		}
		e := raListGame{ID: g.ID, Title: Line(g.Title), Points: g.Points, Count: g.NumAchievements}
		for _, h := range g.Hashes {
			if raHashRe.MatchString(h) {
				e.Hashes = append(e.Hashes, h)
			}
		}
		l.Games = append(l.Games, e)
	}
	_ = keep(p, l) // if it can't be kept, it's asked for again next time
	l.index()
	s.lists[console] = l
	return l, nil
}

var raHashRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

func (l *raList) index() {
	l.byHash = make(map[string]int, len(l.Games)*2)
	for i, g := range l.Games {
		for _, h := range g.Hashes {
			l.byHash[h] = i
		}
	}
}

// FindGame finds the game a checksum is in these consoles' lists. ErrNotFound: it
// isn't known, or has no achievements.
func (s *RetroAchievements) FindGame(ctx context.Context, consoles []int, hash string) (id int, title string, err error) {
	s.init()
	if s.Key == "" {
		return 0, "", ErrNotSet
	}
	if !raHashRe.MatchString(hash) {
		return 0, "", ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range consoles {
		l, err := s.list(ctx, c)
		if err != nil {
			return 0, "", err
		}
		if i, ok := l.byHash[hash]; ok {
			return l.Games[i].ID, l.Games[i].Title, nil
		}
	}
	return 0, "", ErrNotFound
}

var raBadgeRe = regexp.MustCompile(`^[0-9]{1,10}$`)

// ValidBadge says whether a name could be one of RetroAchievements' pictures.
func ValidBadge(name string) bool { return raBadgeRe.MatchString(name) }

// Game is one game's achievements, in the order the game lists them.
func (s *RetroAchievements) Game(ctx context.Context, id int) (*RAGame, error) {
	s.init()
	if s.Key == "" {
		return nil, ErrNotSet
	}
	if id <= 0 {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if g := s.games[id]; g != nil && time.Since(g.Fetched) < raGameFresh {
		return g, nil
	}
	p := filepath.Join(s.Dir, "games", strconv.Itoa(id)+".json")
	var kept RAGame
	have := readKept(p, &kept) && kept.ID == id
	if have && time.Since(kept.Fetched) < raGameFresh {
		s.games[id] = &kept
		return &kept, nil
	}
	var raw struct {
		ID    int    `json:"ID"`
		Title string `json:"Title"`
		// An object keyed by achievement, or an empty list when there are none.
		Achievements json.RawMessage `json:"Achievements"`
	}
	var list map[string]struct {
		ID           int    `json:"ID"`
		Title        string `json:"Title"`
		Description  string `json:"Description"`
		Points       int    `json:"Points"`
		BadgeName    string `json:"BadgeName"`
		DisplayOrder int    `json:"DisplayOrder"`
	}
	err := s.get(ctx, "API_GetGameExtended.php", url.Values{"i": {strconv.Itoa(id)}}, &raw)
	if err == nil && raw.ID != id {
		err = ErrNotFound // it answers an unknown game with an empty one
	}
	if a := bytes.TrimSpace(raw.Achievements); err == nil && len(a) > 0 && a[0] == '{' {
		if json.Unmarshal(a, &list) != nil {
			err = errors.New("RetroAchievements sent achievements that couldn't be read")
		}
	}
	if err != nil {
		if have && !errors.Is(err, ErrKey) && !errors.Is(err, ErrNotSet) && !errors.Is(err, ErrNotFound) {
			s.games[id] = &kept
			return &kept, nil
		}
		return nil, err
	}
	g := &RAGame{ID: id, Title: Line(raw.Title), Fetched: time.Now(), Achievements: []RAAchievement{}}
	for _, a := range list {
		if a.ID <= 0 {
			continue
		}
		badge := a.BadgeName
		if !ValidBadge(badge) {
			badge = ""
		}
		g.Achievements = append(g.Achievements, RAAchievement{ID: a.ID, Title: Line(a.Title), Description: Line(a.Description),
			Points: max(a.Points, 0), Badge: badge, Order: a.DisplayOrder})
		g.Points += max(a.Points, 0)
	}
	sort.Slice(g.Achievements, func(i, j int) bool {
		a, b := g.Achievements[i], g.Achievements[j]
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return a.ID < b.ID
	})
	_ = keep(p, g)
	s.games[id] = g
	return g, nil
}

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

// Badge is the greyed-out picture of an achievement not yet earned, as PNG. Pictures
// are kept on disk: one with this name never changes.
func (s *RetroAchievements) Badge(ctx context.Context, name string) ([]byte, error) {
	s.init()
	if !ValidBadge(name) {
		return nil, ErrNotFound
	}
	p := filepath.Join(s.Dir, "badges", name+"_lock.png")
	if b, err := os.ReadFile(p); err == nil && bytes.HasPrefix(b, pngMagic) {
		return b, nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", s.MediaBase+"/Badge/"+name+"_lock.png", nil)
	if err != nil {
		return nil, err
	}
	b, err := s.media.Read(req, raBadgeMax+1)
	if err != nil {
		return nil, err
	}
	if len(b) > raBadgeMax || !bytes.HasPrefix(b, pngMagic) {
		return nil, errors.New("RetroAchievements sent something that isn't a badge")
	}
	_ = keepBytes(p, b) // if it can't be kept, it's fetched again next time
	return b, nil
}
