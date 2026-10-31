package gamemeta

import (
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
	"strings"
	"sync"
	"time"
)

// Wikidata for the facts (public domain) and Wikipedia for the summaries (CC BY-SA 4.0,
// credited on the game's page). No key is needed, and no pictures come from here.
//
// Asked one game at a time this would take three requests a game, for hours. Instead a
// console's whole list of games comes in one query and is matched here; then facts are
// fetched 50 games a request and summaries 20 games a request, one request at a time.

// WikidataSource is the source. The name Wikidata is taken by the film one in
// internal/library.
type WikidataSource struct {
	HTTP *http.Client
	Dir  string
	// Query, API and Wikipedia are only changed by tests.
	Query, API, Wikipedia string

	once sync.Once
	c    *Client

	mu       sync.Mutex
	console  string
	list     *wdList
	prepared map[string]*Info
	labels   map[string]string
}

func (s *WikidataSource) init() {
	s.once.Do(func() {
		s.c = NewClient("Wikidata", s.HTTP, time.Second)
		s.labels = map[string]string{}
		if s.Query == "" {
			s.Query = "https://query.wikidata.org/sparql"
		}
		if s.API == "" {
			s.API = "https://www.wikidata.org/w/api.php"
		}
		if s.Wikipedia == "" {
			s.Wikipedia = "https://en.wikipedia.org/w/api.php"
		}
	})
}

// About implements Source.
func (s *WikidataSource) About() About {
	return About{ID: "wikidata", Name: "Wikidata and Wikipedia", Site: "https://www.wikidata.org",
		Why:     "Plain-language summaries, who made a game, and some review scores and age ratings, with no account.",
		Gives:   []string{"description", "dates", "developer", "publisher", "genres", "critic score", "age ratings"},
		Fields:  []KeyField{},
		Cost:    "Free, no account",
		Limit:   "No fixed limit for reads. This server asks one thing at a time, once a second, in large batches.",
		Sends:   "Which console you have games for, and the names of games it matches",
		Credit:  "Facts from Wikidata; summaries from Wikipedia",
		License: "Wikidata: CC0. Wikipedia text: CC BY-SA 4.0",
	}
}

// wdGame is one row of a console's list.
type wdGame struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Wiki  string `json:"wiki,omitempty"`
}

type wdList struct {
	games []wdGame
	byKey map[string][]int32
	keys  []string
	byW   map[string][]int32
}

func buildWDList(games []wdGame) *wdList {
	l := &wdList{games: games, byKey: map[string][]int32{}, byW: map[string][]int32{}}
	add := func(name string, i int32) {
		k := NameKey(name)
		if k == "" {
			return
		}
		if _, ok := l.byKey[k]; !ok {
			ki := int32(len(l.keys))
			l.keys = append(l.keys, k)
			for _, w := range strings.Fields(k) {
				l.byW[w] = append(l.byW[w], ki)
			}
		}
		l.byKey[k] = append(l.byKey[k], i)
	}
	for i, g := range games {
		add(g.Label, int32(i))
		// An article's title can differ, as in "Name (video game)".
		if g.Wiki != "" && !strings.EqualFold(g.Wiki, g.Label) {
			add(g.Wiki, int32(i))
		}
	}
	return l
}

// find matches a name against the list.
func (l *wdList) find(title string) (*wdGame, int) {
	if ids := l.byKey[NameKey(title)]; len(ids) > 0 {
		return &l.games[ids[0]], 100
	}
	shared := map[int32]int{}
	for _, w := range words(title) {
		if list := l.byW[w]; len(list) < 500 {
			for _, k := range list {
				shared[k]++
			}
		}
	}
	best, bestScore := int32(-1), 0
	type pair struct{ k, n int32 }
	pairs := make([]pair, 0, len(shared))
	for k, n := range shared {
		pairs = append(pairs, pair{k, int32(n)})
	}
	sort.Slice(pairs, func(a, b int) bool {
		if pairs[a].n != pairs[b].n {
			return pairs[a].n > pairs[b].n
		}
		return pairs[a].k < pairs[b].k
	})
	if len(pairs) > 80 {
		pairs = pairs[:80]
	}
	for _, p := range pairs {
		if sc := NameScore(title, l.keys[p.k]); sc > bestScore {
			best, bestScore = p.k, sc
		}
	}
	if best < 0 {
		return nil, 0
	}
	return &l.games[l.byKey[l.keys[best]][0]], bestScore
}

// wdListFresh is how long a console's list is kept before fetching it again.
const wdListFresh = 90 * 24 * time.Hour

// getList gets a console's list, from disk if it's recent. Failing to refresh an old
// one isn't a problem.
func (s *WikidataSource) getList(ctx context.Context, console string) (*wdList, error) {
	s.init()
	s.mu.Lock()
	if s.console == console && s.list != nil {
		l := s.list
		s.mu.Unlock()
		return l, nil
	}
	s.mu.Unlock()
	p, ok := platforms[console]
	if !ok || len(p.wikidata) == 0 || console == "pc" {
		return nil, ErrNotFound
	}
	var cached []wdGame
	file := filepath.Join(s.Dir, console+".json")
	st, statErr := os.Stat(file)
	if statErr == nil {
		if b, err := os.ReadFile(file); err == nil {
			_ = json.Unmarshal(b, &cached)
		}
	}
	games := cached
	if statErr != nil || time.Since(st.ModTime()) > wdListFresh || len(cached) == 0 {
		fresh, err := s.queryList(ctx, p.wikidata)
		switch {
		case err == nil:
			games = fresh
			if s.Dir != "" && os.MkdirAll(s.Dir, 0o755) == nil {
				if b, err := json.Marshal(fresh); err == nil {
					_ = os.WriteFile(file, b, 0o644)
				}
			}
		case len(cached) == 0:
			return nil, err
		}
	}
	l := buildWDList(games)
	s.mu.Lock()
	s.console, s.list = console, l
	s.prepared = map[string]*Info{}
	s.mu.Unlock()
	return l, nil
}

var qidRe = regexp.MustCompile(`^Q[0-9]+$`)

func (s *WikidataSource) queryList(ctx context.Context, qids []string) ([]wdGame, error) {
	vals := make([]string, 0, len(qids))
	for _, q := range qids {
		vals = append(vals, "wd:"+q)
	}
	// This form works on the query service's next version too, which has no label service.
	query := `PREFIX wd: <http://www.wikidata.org/entity/> PREFIX wdt: <http://www.wikidata.org/prop/direct/> ` +
		`PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#> PREFIX schema: <http://schema.org/> ` +
		`SELECT ?game ?gameLabel ?wiki WHERE { VALUES ?p { ` + strings.Join(vals, " ") + ` } ?game wdt:P31 wd:Q7889 ; wdt:P400 ?p . ` +
		`OPTIONAL { ?game rdfs:label ?en . FILTER(LANG(?en) = "en") } OPTIONAL { ?game rdfs:label ?mul . FILTER(LANG(?mul) = "mul") } ` +
		`BIND(COALESCE(?en, ?mul) AS ?gameLabel) ` +
		`OPTIONAL { ?art schema:about ?game ; schema:isPartOf <https://en.wikipedia.org/> ; schema:name ?wiki . } }`
	req, err := http.NewRequestWithContext(ctx, "POST", s.Query, strings.NewReader(url.Values{"query": {query}, "format": {"json"}}.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out struct {
		Results struct {
			Bindings []struct {
				Game  struct{ Value string } `json:"game"`
				Label struct{ Value string } `json:"gameLabel"`
				Wiki  struct{ Value string } `json:"wiki"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := s.c.JSON(req, &out); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	games := make([]wdGame, 0, len(out.Results.Bindings))
	for _, b := range out.Results.Bindings {
		id := b.Game.Value[strings.LastIndex(b.Game.Value, "/")+1:]
		if !qidRe.MatchString(id) || seen[id] || b.Label.Value == id {
			continue
		}
		seen[id] = true
		games = append(games, wdGame{ID: id, Label: Line(b.Label.Value), Wiki: Line(b.Wiki.Value)})
	}
	return games, nil
}

// ---------------------------------------------------------------- entities

type wdValue struct {
	ID     string `json:"id"`
	Time   string `json:"time"`
	Amount string `json:"amount"`
}

type wdSnak struct {
	Datavalue struct {
		Value json.RawMessage `json:"value"`
	} `json:"datavalue"`
}

func (s wdSnak) value() wdValue {
	var v wdValue
	if json.Unmarshal(s.Datavalue.Value, &v) != nil {
		var text string
		if json.Unmarshal(s.Datavalue.Value, &text) == nil {
			v.Amount = text
		}
	}
	return v
}

type wdClaim struct {
	Mainsnak   wdSnak              `json:"mainsnak"`
	Rank       string              `json:"rank"`
	Qualifiers map[string][]wdSnak `json:"qualifiers"`
}

type wdEntityData struct {
	ID        string                            `json:"id"`
	Labels    map[string]struct{ Value string } `json:"labels"`
	Sitelinks map[string]struct{ Title string } `json:"sitelinks"`
	Claims    map[string][]wdClaim              `json:"claims"`
}

const (
	wdPublished  = "P577"
	wdDeveloper  = "P178"
	wdPublisher  = "P123"
	wdGenreProp  = "P136"
	wdMode       = "P404"
	wdSeries     = "P179"
	wdReview     = "P444"
	wdReviewBy   = "P447"
	wdESRB       = "P852"
	wdPEGI       = "P908"
	wdCERO       = "P853"
	wdUSK        = "P914"
	wdSinglePlay = "Q208189"
)

// api calls the Wikimedia API, leaving it alone when it says it's busy.
func (s *WikidataSource) api(ctx context.Context, base string, q url.Values, out any) error {
	q.Set("format", "json")
	q.Set("maxlag", "5")
	var raw json.RawMessage
	began := time.Now()
	if err := s.c.Get(ctx, base+"?"+q.Encode(), &raw); err != nil {
		return err
	}
	// Wikimedia's rule for robots: after a slow answer, give it a rest.
	if time.Since(began) > time.Second {
		s.c.gate.delay(5 * time.Second)
	}
	var e struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != nil {
		if e.Error.Code == "maxlag" {
			return s.c.gate.pause(time.Minute, "Wikidata is busy")
		}
		return errors.New("Wikidata said: " + Line(e.Error.Code))
	}
	return json.Unmarshal(raw, out)
}

func (s *WikidataSource) entities(ctx context.Context, ids []string, props string) (map[string]*wdEntityData, error) {
	out := map[string]*wdEntityData{}
	for len(ids) > 0 {
		n := min(50, len(ids))
		var res struct {
			Entities map[string]*wdEntityData `json:"entities"`
		}
		q := url.Values{"action": {"wbgetentities"}, "ids": {strings.Join(ids[:n], "|")}, "props": {props}, "languages": {"en|mul"}, "sitefilter": {"enwiki"}}
		if err := s.api(ctx, s.API, q, &res); err != nil {
			return out, err
		}
		for k, v := range res.Entities {
			out[k] = v
		}
		ids = ids[n:]
	}
	return out, nil
}

// labelsFor gets names for items facts point at. Publishers and genres repeat a lot, so
// they're remembered.
func (s *WikidataSource) labelsFor(ctx context.Context, ids []string) {
	seen := map[string]bool{}
	var need []string
	s.mu.Lock()
	for _, id := range ids {
		if _, ok := s.labels[id]; !ok && !seen[id] && qidRe.MatchString(id) {
			seen[id] = true
			need = append(need, id)
		}
	}
	s.mu.Unlock()
	if len(need) == 0 {
		return
	}
	got, err := s.entities(ctx, need, "labels")
	if err != nil && len(got) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, e := range got {
		if l := e.Labels["en"].Value; l != "" {
			s.labels[id] = l
		} else {
			s.labels[id] = e.Labels["mul"].Value
		}
	}
}

func (s *WikidataSource) label(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.labels[id]
}

// referenced lists the items an entity's facts point at.
func referenced(e *wdEntityData) []string {
	var out []string
	for _, p := range []string{wdDeveloper, wdPublisher, wdGenreProp, wdSeries, wdESRB, wdPEGI, wdCERO, wdUSK} {
		for i, c := range e.Claims[p] {
			if i >= 6 {
				break
			}
			if id := c.Mainsnak.value().ID; id != "" {
				out = append(out, id)
			}
		}
	}
	for i, c := range e.Claims[wdReview] {
		if i >= 12 {
			break
		}
		for _, q := range c.Qualifiers[wdReviewBy] {
			if id := q.value().ID; id != "" {
				out = append(out, id)
			}
		}
	}
	return out
}

var (
	reviewRe  = regexp.MustCompile(`^\s*([0-9]+(?:\.[0-9]+)?)\s*/\s*([0-9]+(?:\.[0-9]+)?)\s*$`)
	percentRe = regexp.MustCompile(`^\s*([0-9]+(?:\.[0-9]+)?)\s*%\s*$`)
	digitsRe  = regexp.MustCompile(`[0-9]+\+?`)
)

// parseReview reads "8/10", "92/100" or "85%" as a score out of 100. Other marks, such
// as a letter grade, aren't worth guessing at.
func parseReview(s string) float64 {
	if m := reviewRe.FindStringSubmatch(s); m != nil {
		a, _ := strconv.ParseFloat(m[1], 64)
		b, _ := strconv.ParseFloat(m[2], 64)
		if b > 0 && a <= b {
			return round1(a / b * 100)
		}
	}
	if m := percentRe.FindStringSubmatch(s); m != nil {
		a, _ := strconv.ParseFloat(m[1], 64)
		if a <= 100 {
			return round1(a)
		}
	}
	return 0
}

// wdAge reads an age rating from its item's name, which spells it out: "PEGI 12", "CERO
// A", "Teen", "Everyone 10+".
func wdAge(system, label string) (AgeRating, bool) {
	l := strings.TrimSpace(label)
	switch system {
	case "ESRB":
		l = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(l, "ESRB"), ":"))
		switch strings.ToLower(l) {
		case "early childhood":
			return NewAge("ESRB", "EC")
		case "everyone":
			return NewAge("ESRB", "E")
		case "everyone 10+":
			return NewAge("ESRB", "E10+")
		case "teen":
			return NewAge("ESRB", "T")
		case "mature", "mature 17+":
			return NewAge("ESRB", "M")
		case "adults only", "adults only 18+":
			return NewAge("ESRB", "AO")
		case "kids to adults":
			return NewAge("ESRB", "KA")
		}
		return NewAge("ESRB", l)
	case "CERO":
		f := strings.Fields(l)
		for _, w := range f {
			if len(w) == 1 {
				return NewAge("CERO", w)
			}
		}
		return AgeRating{}, false
	}
	return NewAge(system, digitsRe.FindString(l))
}

func wdDate(t string) string {
	t = strings.TrimPrefix(t, "+")
	if len(t) < 10 {
		return ""
	}
	d := t[:10]
	d = strings.TrimSuffix(strings.TrimSuffix(d, "-00"), "-00")
	return cleanDate(d)
}

// bestClaims leaves out what's been marked deprecated, and prefers the preferred.
func bestClaims(list []wdClaim) []wdClaim {
	var out []wdClaim
	for _, c := range list {
		if c.Rank == "preferred" {
			out = append(out, c)
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, c := range list {
		if c.Rank != "deprecated" {
			out = append(out, c)
		}
	}
	return out
}

func (s *WikidataSource) info(e *wdEntityData) *Info {
	in := &Info{ID: e.ID, Title: Line(e.Labels["en"].Value), URL: "https://www.wikidata.org/wiki/" + e.ID}
	if in.Title == "" {
		in.Title = Line(e.Labels["mul"].Value)
	}
	if t := e.Sitelinks["enwiki"].Title; t != "" {
		in.URL = "https://en.wikipedia.org/wiki/" + url.PathEscape(strings.ReplaceAll(t, " ", "_"))
	}
	earliest := ""
	for _, c := range bestClaims(e.Claims[wdPublished]) {
		if d := wdDate(c.Mainsnak.value().Time); d != "" && (earliest == "" || d < earliest) {
			earliest = d
		}
	}
	in.Released = earliest
	first := func(p string) string {
		for _, c := range bestClaims(e.Claims[p]) {
			if l := s.label(c.Mainsnak.value().ID); l != "" {
				return Line(l)
			}
		}
		return ""
	}
	in.Developer, in.Publisher, in.Franchise = first(wdDeveloper), first(wdPublisher), first(wdSeries)
	var genres []string
	for _, c := range bestClaims(e.Claims[wdGenreProp]) {
		if l := s.label(c.Mainsnak.value().ID); l != "" {
			g := titleWords(strings.TrimSuffix(strings.TrimSuffix(l, " video game"), " game"))
			genres = append(genres, g)
		}
	}
	in.Genres = lines(genres, 8)
	modes := bestClaims(e.Claims[wdMode])
	if len(modes) == 1 && modes[0].Mainsnak.value().ID == wdSinglePlay {
		in.Players = 1
	}
	for _, sys := range []struct{ p, name string }{{wdESRB, "ESRB"}, {wdPEGI, "PEGI"}, {wdCERO, "CERO"}, {wdUSK, "USK"}} {
		for _, c := range bestClaims(e.Claims[sys.p]) {
			if r, ok := wdAge(sys.name, s.label(c.Mainsnak.value().ID)); ok {
				in.Ages = append(in.Ages, r)
				break
			}
		}
	}
	seen := map[string]bool{}
	for _, c := range bestClaims(e.Claims[wdReview]) {
		v := parseReview(c.Mainsnak.value().Amount)
		if v <= 0 {
			continue
		}
		for _, q := range c.Qualifiers[wdReviewBy] {
			if by := Line(s.label(q.value().ID)); by != "" && !seen[by] && len(seen) < 6 {
				seen[by] = true
				in.Scores = append(in.Scores, Score{Source: by, Kind: "critic", Value: v})
				break
			}
		}
	}
	return in
}

// titleWords capitalises a genre's name, as Wikidata keeps them in lower case.
func titleWords(s string) string {
	f := strings.Fields(s)
	for i, w := range f {
		if w != "" && w[0] >= 'a' && w[0] <= 'z' {
			f[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(f, " ")
}

// summaries gets articles' first paragraphs, twenty a request.
func (s *WikidataSource) summaries(ctx context.Context, titles []string) map[string]string {
	out := map[string]string{}
	for len(titles) > 0 {
		n := min(20, len(titles))
		var res struct {
			Query struct {
				Normalized []struct{ From, To string } `json:"normalized"`
				Redirects  []struct{ From, To string } `json:"redirects"`
				Pages      []struct {
					Title   string `json:"title"`
					Extract string `json:"extract"`
				} `json:"pages"`
			} `json:"query"`
		}
		q := url.Values{"action": {"query"}, "prop": {"extracts"}, "exintro": {"1"}, "explaintext": {"1"}, "exlimit": {"max"}, "redirects": {"1"}, "formatversion": {"2"}, "titles": {strings.Join(titles[:n], "|")}}
		if err := s.api(ctx, s.Wikipedia, q, &res); err != nil {
			return out
		}
		back := map[string]string{}
		for _, m := range res.Query.Normalized {
			back[m.To] = m.From
		}
		for _, m := range res.Query.Redirects {
			if from, ok := back[m.From]; ok {
				back[m.To] = from
			} else {
				back[m.To] = m.From
			}
		}
		for _, p := range res.Query.Pages {
			name := p.Title
			if from, ok := back[name]; ok {
				name = from
			}
			out[name] = Text(p.Extract)
		}
		titles = titles[n:]
	}
	return out
}

// fetchMany gets facts and summaries for several games.
func (s *WikidataSource) fetchMany(ctx context.Context, ids []string) (map[string]*Info, error) {
	got, err := s.entities(ctx, ids, "labels|sitelinks|claims")
	if len(got) == 0 {
		return nil, err
	}
	var refs, titles []string
	for _, e := range got {
		refs = append(refs, referenced(e)...)
		if t := e.Sitelinks["enwiki"].Title; t != "" {
			titles = append(titles, t)
		}
	}
	s.labelsFor(ctx, refs)
	texts := s.summaries(ctx, titles)
	out := map[string]*Info{}
	for id, e := range got {
		in := s.info(e)
		in.Description = texts[e.Sitelinks["enwiki"].Title]
		out[id] = in
	}
	return out, err
}

// Prepare implements Preparer: everything the batch's games will need, in as few
// requests as it takes.
func (s *WikidataSource) Prepare(ctx context.Context, console string, qs []Query) error {
	l, err := s.getList(ctx, console)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	var want []string
	seen := map[string]bool{}
	s.mu.Lock()
	for _, q := range qs {
		if g, score := l.find(q.Title); g != nil && score >= AcceptScore && !seen[g.ID] {
			if _, have := s.prepared[g.ID]; !have {
				seen[g.ID] = true
				want = append(want, g.ID)
			}
		}
	}
	s.mu.Unlock()
	if len(want) == 0 {
		return nil
	}
	got, err := s.fetchMany(ctx, want)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.prepared) > 2000 {
		s.prepared = map[string]*Info{}
	}
	for id, in := range got {
		s.prepared[id] = in
	}
	return err
}

// Lookup implements Source.
func (s *WikidataSource) Lookup(ctx context.Context, q Query) (*Info, error) {
	l, err := s.getList(ctx, q.Console)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if l == nil {
		return s.lookupBySearch(ctx, q)
	}
	g, score := l.find(q.Title)
	if g == nil || score < AcceptScore {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	in, ok := s.prepared[g.ID]
	s.mu.Unlock()
	if !ok {
		got, err := s.fetchMany(ctx, []string{g.ID})
		if err != nil && len(got) == 0 {
			return nil, err
		}
		if in, ok = got[g.ID]; !ok {
			return nil, ErrNotFound
		}
	}
	cp := *in
	cp.Method, cp.Confidence = MethodName, nameConfidence(score)
	return &cp, nil
}

type wdSearchHit struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

func (s *WikidataSource) searchNames(ctx context.Context, title string) ([]wdSearchHit, error) {
	var res struct {
		Search []wdSearchHit `json:"search"`
	}
	s.init()
	q := url.Values{"action": {"wbsearchentities"}, "search": {title}, "language": {"en"}, "type": {"item"}, "limit": {"15"}}
	if err := s.api(ctx, s.API, q, &res); err != nil {
		return nil, err
	}
	out := res.Search[:0]
	for _, r := range res.Search {
		d := strings.ToLower(r.Description)
		if strings.Contains(d, "video game") && !strings.Contains(d, "series") && !strings.Contains(d, "franchise") && !strings.Contains(d, "character") && !strings.Contains(d, "company") {
			out = append(out, r)
		}
	}
	return out, nil
}

// lookupBySearch is for games on something with too many games for one list: a PC.
func (s *WikidataSource) lookupBySearch(ctx context.Context, q Query) (*Info, error) {
	hits, err := s.searchNames(ctx, q.Title)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(hits))
	for i, h := range hits {
		names[i] = h.Label
	}
	i, score := Best(q.Title, names)
	if i < 0 || score < AcceptScore {
		return nil, ErrNotFound
	}
	got, err := s.fetchMany(ctx, []string{hits[i].ID})
	if err != nil && len(got) == 0 {
		return nil, err
	}
	in, ok := got[hits[i].ID]
	if !ok {
		return nil, ErrNotFound
	}
	in.Method, in.Confidence = MethodName, nameConfidence(score)
	return in, nil
}

// Search implements Source.
func (s *WikidataSource) Search(ctx context.Context, console, title string) ([]Candidate, error) {
	hits, err := s.searchNames(ctx, title)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, h := range hits {
		out = append(out, Candidate{Source: "wikidata", ID: h.ID, Title: Line(h.Label), Score: NameScore(title, h.Label)})
	}
	return out, nil
}

// Fetch implements Source.
func (s *WikidataSource) Fetch(ctx context.Context, console, id string) (*Info, error) {
	s.init()
	if !qidRe.MatchString(id) {
		return nil, ErrNotFound
	}
	got, err := s.fetchMany(ctx, []string{id})
	if err != nil && len(got) == 0 {
		return nil, err
	}
	in, ok := got[id]
	if !ok {
		return nil, ErrNotFound
	}
	in.Method, in.Confidence = MethodManual, 100
	return in, nil
}

// Test implements Source.
func (s *WikidataSource) Test(ctx context.Context) (string, error) {
	if _, err := s.searchNames(ctx, "Wikipedia"); err != nil {
		return "", err
	}
	return "Wikidata can be reached.", nil
}
