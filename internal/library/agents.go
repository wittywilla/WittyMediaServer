package library

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Custom metadata agents: web services the owner points the server at, for
// catalogues the built-in agents don't cover (a family archive, a club's
// film list, a script of their own). An agent is a URL template; the server
// fills in the item's title, year and type, and reads back JSON in the
// format below. Agents run before the built-in ones and only fill in what
// is still empty, so a sidecar file or an edit always wins.
//
//	{"id": "x1", "title": "…", "year": 2004, "overview": "…", "tagline": "…",
//	 "genres": ["…"], "rating": 7.4, "contentRating": "PG", "studio": "…",
//	 "runtimeMinutes": 93, "poster": "https://…", "backdrop": "https://…"}
//
// The answer may also be a list, or {"results": [...]}: the first entry is used.

// Agent is one custom metadata agent.
type Agent struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	URL     string   `json:"url"`    // with {title}, {year}, {type}
	Header  string   `json:"header"` // optional "Name: value", for an API key
	Kinds   []string `json:"kinds"`  // library types it serves: movies, videos, books
	Enabled bool     `json:"enabled"`
}

// AgentKinds are the library types custom agents can serve.
var AgentKinds = []string{"movies", "discs", "videos", "books"}

var agentIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// Validate checks an agent before it is saved.
func (a *Agent) Validate() error {
	a.Name, a.URL, a.Header = strings.TrimSpace(a.Name), strings.TrimSpace(a.URL), strings.TrimSpace(a.Header)
	if a.Name == "" || len(a.Name) > 60 {
		return errors.New("give the agent a name of up to 60 characters")
	}
	if !agentIDRe.MatchString(a.ID) {
		return errors.New("bad agent id")
	}
	u, err := url.Parse(strings.NewReplacer("{title}", "x", "{year}", "2000", "{type}", "movie").Replace(a.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("the address must start with http:// or https://")
	}
	if !strings.Contains(a.URL, "{title}") {
		return errors.New("the address needs {title} where the title goes")
	}
	if a.Header != "" && !strings.Contains(a.Header, ":") {
		return errors.New(`the header must look like "Name: value"`)
	}
	ok := a.Kinds[:0]
	for _, k := range a.Kinds {
		for _, v := range AgentKinds {
			if k == v {
				ok = append(ok, k)
			}
		}
	}
	a.Kinds = ok
	if len(a.Kinds) == 0 {
		return errors.New("choose at least one kind of library")
	}
	return nil
}

// Serves reports whether the agent is on and covers a library type.
func (a *Agent) Serves(libType string) bool {
	if !a.Enabled {
		return false
	}
	for _, k := range a.Kinds {
		if k == libType {
			return true
		}
	}
	return false
}

// AgentResult is what an agent answers.
type AgentResult struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Year           int      `json:"year"`
	Overview       string   `json:"overview"`
	Tagline        string   `json:"tagline"`
	Genres         []string `json:"genres"`
	Rating         float64  `json:"rating"`
	ContentRating  string   `json:"contentRating"`
	Studio         string   `json:"studio"`
	RuntimeMinutes int      `json:"runtimeMinutes"`
	Poster         string   `json:"poster"`
	Backdrop       string   `json:"backdrop"`
}

// The agents are the owner's own services, often on the local network, so
// they are reached directly (unlike feeds and catalogues); only http(s),
// short timeouts and small answers.
var agentClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 || (req.URL.Scheme != "http" && req.URL.Scheme != "https") {
			return errors.New("too many or unsafe redirects")
		}
		// The agent's API key is for the agent: another host never sees it.
		if !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
			for k := range req.Header {
				if k != "Accept" && k != "User-Agent" {
					req.Header.Del(k)
				}
			}
		}
		return nil
	},
}

func agentType(libType string) string {
	switch libType {
	case "movies":
		return "movie"
	case "videos":
		return "video"
	case "discs":
		return "disc"
	}
	return "book"
}

// Lookup asks the agent about a title.
func (a *Agent) Lookup(ctx context.Context, title string, year *int, libType string) (*AgentResult, error) {
	y := ""
	if year != nil {
		y = strconv.Itoa(*year)
	}
	u := strings.NewReplacer("{title}", url.QueryEscape(title), "{year}", y, "{type}", agentType(libType)).Replace(a.URL)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "WittyMediaServer")
	if k, v, ok := strings.Cut(a.Header, ":"); ok {
		req.Header.Set(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	res, err := agentClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s did not answer", a.Name)
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return nil, ErrNotFound
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("%s answered %s", a.Name, res.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	return parseAgentAnswer(raw)
}

func parseAgentAnswer(raw []byte) (*AgentResult, error) {
	var one AgentResult
	var list []AgentResult
	var wrapped struct {
		Results []AgentResult `json:"results"`
	}
	switch {
	case json.Unmarshal(raw, &list) == nil:
	case json.Unmarshal(raw, &wrapped) == nil && wrapped.Results != nil:
		list = wrapped.Results
	case json.Unmarshal(raw, &one) == nil:
		list = []AgentResult{one}
	default:
		return nil, errors.New("the answer is not JSON in the agent format")
	}
	if len(list) == 0 || (list[0].Title == "" && list[0].Overview == "" && list[0].Poster == "") {
		return nil, ErrNotFound
	}
	r := list[0]
	r.Overview, r.Tagline = clip(r.Overview, 10000), clip(r.Tagline, 500)
	if len(r.Genres) > 12 {
		r.Genres = r.Genres[:12]
	}
	return &r, nil
}

// applyAgents fills an item's empty fields from the custom agents that serve
// its library, in order. Pictures are downloaded now (an agent on the local
// network can't be reached when the page asks for the picture later).
func (s *Scanner) applyAgents(ctx context.Context, it *Item, libType string) {
	if s.Agents == nil {
		return
	}
	for _, a := range s.Agents() {
		if !a.Serves(libType) {
			continue
		}
		if it.Overview != "" && it.Poster != "" && len(it.Genres) > 0 {
			return
		}
		r, err := a.Lookup(ctx, it.Title, it.Year, libType)
		if err != nil {
			s.Log.Info("agent lookup", "cat", "library", "agent", a.Name, "title", it.Title, "err", err)
			continue
		}
		s.applyAgentResult(ctx, &a, it, r)
	}
}

func (s *Scanner) applyAgentResult(ctx context.Context, a *Agent, it *Item, r *AgentResult) {
	if it.Overview == "" {
		it.Overview = r.Overview
	}
	if it.Tagline == "" {
		it.Tagline = r.Tagline
	}
	if len(it.Genres) == 0 {
		it.Genres = r.Genres
	}
	if it.Rating == nil && r.Rating > 0 && r.Rating <= 10 {
		v := r.Rating
		it.Rating = &v
	}
	if it.ContentRating == "" {
		it.ContentRating = clip(r.ContentRating, 20)
	}
	if it.Studio == "" {
		it.Studio = clip(r.Studio, 120)
	}
	if it.Year == nil && r.Year > 1800 && r.Year < 3000 {
		y := r.Year
		it.Year = &y
	}
	if it.RuntimeSec == 0 && r.RuntimeMinutes > 0 && r.RuntimeMinutes < 10000 {
		it.RuntimeSec = float64(r.RuntimeMinutes * 60)
	}
	if (it.Poster == "" || strings.HasPrefix(it.Poster, "cache:thumb-")) && r.Poster != "" {
		if ref, err := s.agentImage(ctx, a, r.Poster); err == nil {
			it.Poster = ref
		}
	}
	if it.Backdrop == "" && r.Backdrop != "" {
		if ref, err := s.agentImage(ctx, a, r.Backdrop); err == nil {
			it.Backdrop = ref
		}
	}
	if it.Provider == "" && r.ID != "" {
		it.Provider, it.ProviderID = "agent:"+a.ID, clip(r.ID, 200)
	}
}

// agentImage stores a picture from an agent in the cache. A picture on the
// agent's own server is fetched like the agent; any other is fetched from
// the internet only.
func (s *Scanner) agentImage(ctx context.Context, a *Agent, ref string) (string, error) {
	pu, err := url.Parse(ref)
	if err != nil || (pu.Scheme != "http" && pu.Scheme != "https") {
		return "", errors.New("not a web address")
	}
	client := SafeClient
	if au, err := url.Parse(strings.NewReplacer("{title}", "x", "{year}", "", "{type}", "movie").Replace(a.URL)); err == nil && strings.EqualFold(au.Host, pu.Host) {
		client = agentClient
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, "GET", ref, nil)
	if err != nil {
		return "", err
	}
	if k, v, ok := strings.Cut(a.Header, ":"); ok && client == agentClient {
		req.Header.Set(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("picture: %s", res.Status)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 20<<20))
	if err != nil {
		return "", err
	}
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif"}[http.DetectContentType(b)]
	if ext == "" {
		return "", errors.New("not a picture")
	}
	h := sha1.Sum([]byte(a.ID + "|" + ref))
	name := "agent-" + hex.EncodeToString(h[:10]) + ext
	if err := os.MkdirAll(s.ImagesDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(s.ImagesDir, name), b, 0o644); err != nil {
		return "", err
	}
	return "cache:" + name, nil
}

// ParseAgents reads the agents setting (a JSON list); bad entries are dropped.
func ParseAgents(raw string) []Agent {
	var list []Agent
	if raw == "" || json.Unmarshal([]byte(raw), &list) != nil {
		return nil
	}
	out := list[:0]
	for _, a := range list {
		if a.Validate() == nil {
			out = append(out, a)
		}
	}
	return out
}

// TestAgent asks an agent about a title without changing anything; for the
// settings page.
func TestAgent(ctx context.Context, a *Agent, title string, year *int, libType string) (*AgentResult, error) {
	return a.Lookup(ctx, title, year, libType)
}

// ApplyAgents runs the custom agents for an item (its "Refresh" button).
func (s *Scanner) ApplyAgents(ctx context.Context, it *Item, libType string) {
	s.applyAgents(ctx, it, libType)
}
