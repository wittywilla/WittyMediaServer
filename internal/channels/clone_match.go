// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package channels

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Matching a cloned guide to the library. Each slot looks first for the programme itself (by
// metadata id, series, season and episode, episode title or air date), then for a stand-in, in
// the order the owner can switch off one by one: the same series' closest episode, any of its
// episodes, a show like it, and last whatever is closest in length. Every slot says which rule
// found it and how sure that is.

// LibEpisode is one episode a clone can play.
type LibEpisode struct {
	Item    int64
	Title   string
	Season  int // -1 when not known
	Episode int
	Aired   string
	Dur     int
}

// LibShow is a series in the library with its episodes in order.
type LibShow struct {
	ID         int64
	Title      string
	Year       int
	Network    string
	Genres     []string
	Rating     string
	Provider   string
	ProviderID string
	Episodes   []LibEpisode
	norm       string
	avgDur     int
}

// LibTitle is a film or other title that is not an episode.
type LibTitle struct {
	Item       int64
	Title      string
	Year       int
	Genres     []string
	Rating     string
	Studio     string
	Provider   string
	ProviderID string
	Dur        int
	norm       string
}

// CloneLibrary is what a clone can match against.
type CloneLibrary struct {
	Shows  []*LibShow
	Titles []*LibTitle
	byNorm map[string][]*LibShow
	titles map[string][]*LibTitle
}

// Candidate is one thing a slot could play, for the swap list.
type Candidate struct {
	Item       int64  `json:"item"`
	Title      string `json:"title"`
	Sub        string `json:"sub,omitempty"`
	Dur        int    `json:"dur"`
	Rule       string `json:"rule"`
	Confidence int    `json:"confidence"`
	Why        string `json:"why,omitempty"`
	// Show is the series it is an episode of, if any: a stand-in much shorter than its slot
	// plays that series on until the slot ends.
	Show int64 `json:"show,omitempty"`
}

// Prepare indexes the library; call it once after filling Shows and Titles.
func (l *CloneLibrary) Prepare() {
	l.byNorm = map[string][]*LibShow{}
	l.titles = map[string][]*LibTitle{}
	for _, s := range l.Shows {
		s.norm = normTitle(s.Title)
		l.byNorm[s.norm] = append(l.byNorm[s.norm], s)
		if b := stripYear(s.norm); b != s.norm {
			l.byNorm[b] = append(l.byNorm[b], s)
		}
		total := 0
		for _, e := range s.Episodes {
			total += e.Dur
		}
		if len(s.Episodes) > 0 {
			s.avgDur = total / len(s.Episodes)
		}
	}
	for _, t := range l.Titles {
		t.norm = normTitle(t.Title)
		l.titles[t.norm] = append(l.titles[t.norm], t)
	}
}

// matcher keeps what one import has used, so stand-ins don't repeat while others are free.
type matcher struct {
	lib   *CloneLibrary
	o     CloneOptions
	used  map[int64]int
	next  map[int64]int // each show's rotation
	shows map[int64]int // how often each series stood in, so filler takes turns between them
}

// Match fills in every slot that wasn't chosen by hand.
func (l *CloneLibrary) Match(slots []CloneSlot, o CloneOptions) {
	m := &matcher{lib: l, o: o, used: map[int64]int{}, next: map[int64]int{}, shows: map[int64]int{}}
	for i := range slots {
		if slots[i].Pinned && slots[i].Item > 0 {
			m.used[slots[i].Item]++
		}
	}
	for i := range slots {
		s := &slots[i]
		if s.Pinned && s.Item > 0 {
			continue
		}
		c := m.best(s)
		s.Item, s.Rule, s.Confidence, s.Why = c.Item, c.Rule, c.Confidence, c.Why
		s.ItemTitle, s.ItemSub, s.ItemDur, s.Show = c.Title, c.Sub, c.Dur, c.Show
		s.Pinned = false
		if c.Item > 0 {
			m.used[c.Item]++
		}
		if c.Show > 0 && !Exact(c.Rule) {
			m.shows[c.Show]++
		}
	}
}

// Candidates lists up to n things a slot could play instead, best first.
func (l *CloneLibrary) Candidates(slot CloneSlot, o CloneOptions, n int) []Candidate {
	s := &slot
	m := &matcher{lib: l, o: o, used: map[int64]int{}, next: map[int64]int{}, shows: map[int64]int{}}
	var out []Candidate
	seen := map[int64]bool{}
	add := func(c Candidate) {
		if c.Item > 0 && !seen[c.Item] && len(out) < n {
			seen[c.Item] = true
			out = append(out, c)
		}
	}
	if sh, conf := m.findShow(s); sh != nil {
		for _, c := range m.episodeCandidates(sh, s, conf) {
			add(c)
		}
	}
	if t, conf := m.findTitle(s); t != nil {
		add(titleCandidate(t, RuleExact, conf, "the same film"))
	}
	for _, sh := range m.similarShows(s, 6) {
		if ep := m.pickEpisode(sh.show, s.Dur); ep != nil {
			add(episodeCandidate(sh.show, ep, RuleSimilar, similarConfidence(sh.score), sh.why))
		}
	}
	for _, t := range m.similarTitles(s, 4) {
		add(titleCandidate(t.title, RuleSimilar, similarConfidence(t.score), t.why))
	}
	if c := m.filler(s); c.Item > 0 {
		add(c)
	}
	return out
}

func (m *matcher) best(s *CloneSlot) Candidate {
	if !s.Gap {
		if sh, conf := m.findShow(s); sh != nil {
			if c := m.fromShow(sh, s, conf); c.Item > 0 {
				return c
			}
		}
		if t, conf := m.findTitle(s); t != nil {
			return titleCandidate(t, RuleExact, conf, "the same film")
		}
		if !m.o.NoSimilar {
			for _, sh := range m.similarShows(s, 3) {
				if ep := m.pickEpisode(sh.show, s.Dur); ep != nil {
					return episodeCandidate(sh.show, ep, RuleSimilar, similarConfidence(sh.score), sh.why)
				}
			}
			if ts := m.similarTitles(s, 1); len(ts) > 0 {
				return titleCandidate(ts[0].title, RuleSimilar, similarConfidence(ts[0].score), ts[0].why)
			}
		}
	}
	if !m.o.NoFiller {
		if c := m.filler(s); c.Item > 0 {
			return c
		}
	}
	return Candidate{Rule: RuleNone, Why: "nothing in the library fits"}
}

// findShow finds the series a slot is an episode of, and how sure that is.
func (m *matcher) findShow(s *CloneSlot) (*LibShow, int) {
	if s.Movie || s.Title == "" {
		return nil, 0
	}
	ids := map[string]string{"tvdb": s.TVDB, "imdb": s.IMDb, "tvmaze": s.TVmaze, "tmdb": strings.TrimPrefix(s.TMDb, "tv/")}
	for _, sh := range m.lib.Shows {
		if v := ids[strings.ToLower(sh.Provider)]; v != "" && v == sh.ProviderID {
			return sh, 100
		}
	}
	n := normTitle(s.Title)
	if list := m.lib.byNorm[n]; len(list) > 0 {
		return pickByYear(list, s.Year), 98
	}
	if list := m.lib.byNorm[stripYear(n)]; len(list) > 0 {
		return pickByYear(list, s.Year), 94
	}
	var best *LibShow
	bestScore := 0.0
	for _, sh := range m.lib.Shows {
		if sc := similarity(n, sh.norm); sc > bestScore {
			best, bestScore = sh, sc
		}
	}
	if bestScore >= 0.86 {
		return best, int(bestScore * 90)
	}
	return nil, 0
}

func pickByYear(list []*LibShow, year int) *LibShow {
	best := list[0]
	for _, sh := range list[1:] {
		if year != 0 && abs(sh.Year-year) < abs(best.Year-year) {
			best = sh
		}
	}
	return best
}

// findTitle finds a film (or other single title) by name and year.
func (m *matcher) findTitle(s *CloneSlot) (*LibTitle, int) {
	if s.Title == "" {
		return nil, 0
	}
	ids := map[string]string{"imdb": s.IMDb, "tmdb": strings.TrimPrefix(s.TMDb, "movie/")}
	for _, t := range m.lib.Titles {
		if v := ids[strings.ToLower(t.Provider)]; v != "" && v == t.ProviderID {
			return t, 100
		}
	}
	n := normTitle(s.Title)
	for _, t := range m.lib.titles[n] {
		if s.Year == 0 || t.Year == 0 || abs(t.Year-s.Year) <= 1 {
			return t, 95
		}
	}
	if !s.Movie {
		return nil, 0
	}
	var best *LibTitle
	bestScore := 0.0
	for _, t := range m.lib.Titles {
		if s.Year != 0 && t.Year != 0 && abs(t.Year-s.Year) > 1 {
			continue
		}
		if sc := similarity(n, t.norm); sc > bestScore {
			best, bestScore = t, sc
		}
	}
	if bestScore >= 0.9 {
		return best, int(bestScore * 85)
	}
	return nil, 0
}

// fromShow picks the episode of a series that plays for a slot.
func (m *matcher) fromShow(sh *LibShow, s *CloneSlot, showConf int) Candidate {
	for _, c := range m.episodeCandidates(sh, s, showConf) {
		switch c.Rule {
		case RuleNear:
			if m.o.NoNear {
				continue
			}
		case RuleSeries:
			if m.o.NoSeries {
				continue
			}
		}
		if Exact(c.Rule) || m.used[c.Item] == 0 || c.Rule == RuleSeries {
			if c.Rule == RuleSeries {
				m.next[sh.ID] = indexOf(sh, c.Item) + 1
			}
			return c
		}
	}
	return Candidate{}
}

// episodeCandidates are a series' episodes for a slot, the best first.
func (m *matcher) episodeCandidates(sh *LibShow, s *CloneSlot, showConf int) []Candidate {
	if len(sh.Episodes) == 0 {
		return nil
	}
	scale := func(c int) int { return c * showConf / 100 }
	var out []Candidate
	have := func(id int64) bool {
		return slices.ContainsFunc(out, func(c Candidate) bool { return c.Item == id })
	}
	if s.Season != nil && s.Episode != nil {
		for i := range sh.Episodes {
			if e := &sh.Episodes[i]; e.Season == *s.Season && e.Episode == *s.Episode {
				out = append(out, episodeCandidate(sh, e, RuleExact, scale(100), fmt.Sprintf("season %d, episode %d", e.Season, e.Episode)))
			}
		}
	}
	if sub := normTitle(s.Sub); sub != "" {
		best, bestScore := -1, 0.0
		for i := range sh.Episodes {
			if sc := similarity(sub, normTitle(sh.Episodes[i].Title)); sc > bestScore {
				best, bestScore = i, sc
			}
		}
		if bestScore >= 0.75 && !have(sh.Episodes[best].Item) {
			out = append(out, episodeCandidate(sh, &sh.Episodes[best], RuleTitle, scale(int(70+bestScore*25)), "the episode's title"))
		}
	}
	if s.Aired != "" {
		for i := range sh.Episodes {
			if e := &sh.Episodes[i]; e.Aired == s.Aired && !have(e.Item) {
				out = append(out, episodeCandidate(sh, e, RuleAired, scale(92), "first aired "+s.Aired))
			}
		}
	}
	// The closest: by season and episode, else by air date.
	if (s.Season != nil && s.Episode != nil) || s.Aired != "" {
		type near struct {
			i    int
			dist float64
		}
		var list []near
		for i := range sh.Episodes {
			e := &sh.Episodes[i]
			if have(e.Item) {
				continue
			}
			var d float64
			switch {
			case s.Season != nil && s.Episode != nil && e.Season >= 0:
				d = float64(abs(e.Season-*s.Season))*1000 + float64(abs(e.Episode-*s.Episode))
				if e.Episode < *s.Episode {
					d += 0.5 // the next episode before the previous one
				}
			case s.Aired != "" && e.Aired != "":
				d = float64(dayDistance(s.Aired, e.Aired))
			default:
				d = 1e9
			}
			if m.used[e.Item] > 0 {
				d += 1e7
			}
			list = append(list, near{i, d})
		}
		slices.SortStableFunc(list, func(a, b near) int { return cmp.Compare(a.dist, b.dist) })
		for k, n := range list {
			if k >= 3 {
				break
			}
			e := &sh.Episodes[n.i]
			why := "the closest episode"
			if s.Season != nil && e.Season == *s.Season {
				why = "the closest episode of the same season"
			}
			out = append(out, episodeCandidate(sh, e, RuleNear, scale(72-k*4), why))
		}
	}
	// Any episode, taking turns through the series.
	if ep := m.pickEpisode(sh, 0); ep != nil && !have(ep.Item) {
		why := "another episode of the series"
		conf := 60
		if s.Season == nil && s.Aired == "" && s.Sub == "" {
			why, conf = "the guide doesn't say which episode", 75
		}
		out = append(out, episodeCandidate(sh, ep, RuleSeries, scale(conf), why))
	}
	return out
}

// pickEpisode is a series' next episode in turn that hasn't played yet in this import; with
// a slot length, one that fits it where there is one.
func (m *matcher) pickEpisode(sh *LibShow, slotDur int) *LibEpisode {
	n := len(sh.Episodes)
	if n == 0 {
		return nil
	}
	start := m.next[sh.ID] % n
	var fallback *LibEpisode
	for k := range n {
		e := &sh.Episodes[(start+k)%n]
		fits := slotDur == 0 || e.Dur <= slotDur+30
		if m.used[e.Item] == 0 && fits {
			return e
		}
		if fallback == nil && fits {
			fallback = e
		}
	}
	if fallback != nil {
		return fallback
	}
	return &sh.Episodes[start]
}

type scoredShow struct {
	show  *LibShow
	score float64
	why   string
}

type scoredTitle struct {
	title *LibTitle
	score float64
	why   string
}

// similarShows ranks the library's series by how like the slot's programme they are.
func (m *matcher) similarShows(s *CloneSlot, n int) []scoredShow {
	if s.Movie {
		return nil
	}
	var out []scoredShow
	for _, sh := range m.lib.Shows {
		if len(sh.Episodes) == 0 {
			continue
		}
		score, why := likeness(s, sh.Genres, sh.Network, sh.Year, sh.Rating, sh.norm, sh.avgDur)
		if score >= 1.5 {
			out = append(out, scoredShow{sh, score, why})
		}
	}
	slices.SortStableFunc(out, func(a, b scoredShow) int { return cmp.Compare(b.score, a.score) })
	return out[:min(n, len(out))]
}

// similarTitles ranks the films for a slot that is a film.
func (m *matcher) similarTitles(s *CloneSlot, n int) []scoredTitle {
	if !s.Movie {
		return nil
	}
	var out []scoredTitle
	for _, t := range m.lib.Titles {
		if m.used[t.Item] > 0 {
			continue
		}
		score, why := likeness(s, t.Genres, t.Studio, t.Year, t.Rating, t.norm, t.Dur)
		if score >= 1.5 {
			out = append(out, scoredTitle{t, score, why})
		}
	}
	slices.SortStableFunc(out, func(a, b scoredTitle) int { return cmp.Compare(b.score, a.score) })
	return out[:min(n, len(out))]
}

// likeness scores a show or film against a slot: shared genres count most, then the same
// network, the same era, the same rating, words in common in the title, and a length that suits
// the slot. why says what it shared.
func likeness(s *CloneSlot, genres []string, network string, year int, rating, norm string, dur int) (float64, string) {
	var score float64
	var why []string
	if g := genreOverlap(s.Genres, genres); g > 0 {
		score += 3 * g
		why = append(why, strings.Join(sharedGenres(s.Genres, genres), ", "))
	}
	if s.Network != "" && network != "" && NormalizeNetwork(s.Network) == NormalizeNetwork(network) {
		score += 2
		why = append(why, network)
	}
	if s.Year != 0 && year != 0 {
		if d := abs(s.Year - year); d < 20 {
			score += 1.5 * (1 - float64(d)/20)
			if d <= 5 {
				why = append(why, strconv.Itoa(year/10*10)+"s")
			}
		}
	}
	if s.Rating != "" && strings.EqualFold(s.Rating, rating) {
		score += 0.5
	}
	if w := wordOverlap(normTitle(s.Title), norm); w > 0 {
		score += w
	}
	if dur > 0 && s.Dur > 0 && dur <= s.Dur+30 && dur*10 >= s.Dur*6 {
		score += 1
	}
	if len(why) == 0 {
		return score, "a show like it"
	}
	return score, "like it: " + strings.Join(why, ", ")
}

func similarConfidence(score float64) int { return 25 + int(min(score*4, 25)) }

// filler is the title closest in length to the slot without running over it, preferring
// what hasn't played yet; failing that, the shortest, cut to fit.
func (m *matcher) filler(s *CloneSlot) Candidate {
	var best Candidate
	bestDiff := -1
	try := func(c Candidate) {
		if c.Dur <= 0 {
			return
		}
		diff := s.Dur - c.Dur
		if diff < 0 {
			diff = 1e6 - diff // runs over: only when nothing else will do
		}
		if m.used[c.Item] > 0 {
			diff += 1e7
		}
		// A minute's difference for each time the series already stood in: filler takes turns.
		diff += 60 * m.shows[c.Show]
		if bestDiff < 0 || diff < bestDiff {
			best, bestDiff = c, diff
		}
	}
	for _, sh := range m.lib.Shows {
		for i := range sh.Episodes {
			try(episodeCandidate(sh, &sh.Episodes[i], RuleFiller, 10, "closest in length"))
		}
	}
	for _, t := range m.lib.Titles {
		try(titleCandidate(t, RuleFiller, 10, "closest in length"))
	}
	return best
}

func episodeCandidate(sh *LibShow, e *LibEpisode, rule string, conf int, why string) Candidate {
	sub := e.Title
	if e.Season >= 0 && e.Episode > 0 {
		sub = fmt.Sprintf("S%dE%d · %s", e.Season, e.Episode, e.Title)
	}
	return Candidate{Item: e.Item, Title: sh.Title, Sub: sub, Dur: e.Dur, Rule: rule, Confidence: max(0, min(conf, 100)), Why: why, Show: sh.ID}
}

func titleCandidate(t *LibTitle, rule string, conf int, why string) Candidate {
	sub := ""
	if t.Year > 0 {
		sub = strconv.Itoa(t.Year)
	}
	return Candidate{Item: t.Item, Title: t.Title, Sub: sub, Dur: t.Dur, Rule: rule, Confidence: max(0, min(conf, 100)), Why: why}
}

func indexOf(sh *LibShow, item int64) int {
	for i := range sh.Episodes {
		if sh.Episodes[i].Item == item {
			return i
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// Comparing names
// ---------------------------------------------------------------------------

// normTitle gives a title one spelling for comparing: lower case, "&" as "and", no punctuation
// or leading "the".
func normTitle(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, "&", " and "))
	var b strings.Builder
	space := true
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			space = false
		case r == '\'' || r == '’':
		default:
			if !space {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	out := strings.TrimSpace(b.String())
	return strings.TrimPrefix(out, "the ")
}

// stripYear drops a trailing year: "doctor who 2005" is "doctor who".
func stripYear(n string) string {
	if i := strings.LastIndexByte(n, ' '); i > 0 && len(n)-i == 5 {
		if y, err := strconv.Atoi(n[i+1:]); err == nil && y > 1900 && y < 2200 {
			return n[:i]
		}
	}
	return n
}

// similarity is the Dice coefficient of two strings' letter pairs, 0 to 1.
func similarity(a, b string) float64 {
	if a == b {
		return 1
	}
	if len(a) < 2 || len(b) < 2 {
		return 0
	}
	pairs := func(s string) map[string]int {
		r := []rune(s)
		m := make(map[string]int, len(r))
		for i := 0; i+1 < len(r); i++ {
			m[string(r[i:i+2])]++
		}
		return m
	}
	pa, pb := pairs(a), pairs(b)
	var common, total int
	for k, n := range pa {
		common += min(n, pb[k])
		total += n
	}
	for _, n := range pb {
		total += n
	}
	return 2 * float64(common) / float64(total)
}

// wordOverlap scores the words two titles share, ignoring short ones: "csi miami" and "csi ny".
func wordOverlap(a, b string) float64 {
	wa := strings.Fields(a)
	if len(wa) == 0 || a == b {
		return 0
	}
	wb := strings.Fields(b)
	n := 0
	for _, w := range wa {
		if len(w) >= 3 && slices.Contains(wb, w) {
			n++
		}
	}
	return float64(n) / float64(len(wa))
}

func lowerAll(v []string) []string {
	out := make([]string, 0, len(v))
	for _, s := range v {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func genreOverlap(a, b []string) float64 {
	la, lb := lowerAll(a), lowerAll(b)
	if len(la) == 0 || len(lb) == 0 {
		return 0
	}
	n := 0
	for _, g := range la {
		if slices.Contains(lb, g) {
			n++
		}
	}
	return float64(n) / float64(len(la)+len(lb)-n)
}

func sharedGenres(a, b []string) []string {
	lb := lowerAll(b)
	var out []string
	for _, g := range a {
		if slices.Contains(lb, strings.ToLower(strings.TrimSpace(g))) {
			out = append(out, g)
		}
	}
	return out
}

func dayDistance(a, b string) int {
	ta, ea := parseDay(a)
	tb, eb := parseDay(b)
	if !ea || !eb {
		return 1 << 20
	}
	return abs(ta - tb)
}

// parseDay reads YYYY-MM-DD as a day count, roughly; close enough for nearest.
func parseDay(s string) (int, bool) {
	if len(s) < 10 {
		return 0, false
	}
	y, e1 := strconv.Atoi(s[:4])
	m, e2 := strconv.Atoi(s[5:7])
	d, e3 := strconv.Atoi(s[8:10])
	if e1 != nil || e2 != nil || e3 != nil {
		return 0, false
	}
	return y*372 + m*31 + d, true
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
