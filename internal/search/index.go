// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package search

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// The groups results come in, in the order they're shown when two match equally well.
var Groups = []string{"films", "shows", "episodes", "artists", "albums", "songs", "audiobooks", "podcasts",
	"books", "comics", "games", "karaoke", "videos", "photos", "radio", "channels", "people"}

// GroupLabels are the groups' names on screen.
var GroupLabels = map[string]string{
	"films": "Films", "shows": "Shows", "episodes": "Episodes", "artists": "Artists", "albums": "Albums",
	"songs": "Songs", "audiobooks": "Audiobooks", "podcasts": "Podcasts", "books": "Books", "comics": "Comics",
	"games": "Games", "karaoke": "Karaoke", "videos": "Videos", "photos": "Photos", "radio": "Radio",
	"channels": "Channels", "people": "People",
}

func groupOrder(g string) int {
	if i := slices.Index(Groups, g); i >= 0 {
		return i
	}
	return len(Groups)
}

// Doc is one thing a search can find.
type Doc struct {
	Group string // one of Groups
	// ID is the item, show, person or channel; for an album or an artist, the item
	// whose cover stands for it.
	ID    int64
	Key   string // a channel's key, an album's or artist's name: what ID can't hold
	Lib   int64  // 0: not in a library (people, channels)
	Title string
	Alt   string // other names it goes by: the show, the artist and album, a title in another language
	Year  int
	Count int // an album's tracks, an artist's albums, a person's titles: the better known first
}

// doc is a Doc as the index keeps it: small, for a library of hundreds of
// thousands on a computer with little memory. Its other names are kept only as
// grams and, normalised, in segment.alt.
type doc struct {
	id    int64
	key   string
	title string
	lib   int32
	count int32
	year  int16
	group uint8
}

func (d *doc) Doc() Doc {
	return Doc{Group: Groups[d.group], ID: d.id, Key: d.key, Lib: int64(d.lib), Title: d.title, Year: int(d.year), Count: int(d.count)}
}

// segment is one library's documents and the grams that find them. Every gram's
// list of documents is a slice of one array, so a library costs four bytes for each
// gram of each title and little more.
type segment struct {
	docs  []doc
	title []string         // each title, normalised
	alt   map[int32]string // other names, normalised, for those that have them
	post  map[gram][]int32
}

func build(in []Doc) *segment {
	sg := &segment{docs: make([]doc, len(in)), title: make([]string, len(in)), alt: map[int32]string{}}
	for i := range in {
		d := &in[i]
		g := slices.Index(Groups, d.Group)
		if g < 0 {
			g = slices.Index(Groups, "videos")
		}
		sg.docs[i] = doc{id: d.ID, key: d.Key, title: d.Title, lib: int32(d.Lib), count: int32(min(d.Count, 1<<30)), year: int16(d.Year), group: uint8(g)}
		sg.title[i] = Normalize(d.Title)
		if a := Normalize(d.Alt); a != "" {
			sg.alt[int32(i)] = a
		}
	}
	// Twice over the grams: once to count them, once to fill the one array. Holding
	// them all between the two would take more memory than the index itself.
	seen := map[gram]struct{}{}
	each := func(fn func(g gram, i int32)) {
		for i := range sg.docs {
			clear(seen)
			add := func(g gram) {
				if _, ok := seen[g]; !ok {
					seen[g] = struct{}{}
					fn(g, int32(i))
				}
			}
			grams(sg.title[i], false, add)
			grams(sg.alt[int32(i)], false, add)
		}
	}
	counts := map[gram]int32{}
	total := 0
	each(func(g gram, _ int32) { counts[g]++; total++ })
	arena := make([]int32, 0, total)
	sg.post = make(map[gram][]int32, len(counts))
	for g, n := range counts {
		sg.post[g] = arena[len(arena) : len(arena) : len(arena)+int(n)]
		arena = arena[:len(arena)+int(n)]
	}
	each(func(g gram, i int32) { sg.post[g] = append(sg.post[g], i) })
	return sg
}

// Index holds every library's segment. It's safe for many searches at once while
// one library's segment is replaced.
type Index struct {
	mu   sync.RWMutex
	segs map[int64]*segment
}

func New() *Index { return &Index{segs: map[int64]*segment{}} }

// Replace swaps one library's documents (lib 0: people and the like) for these.
// The segment is built before the lock is taken, so searches wait only for the swap.
func (x *Index) Replace(lib int64, docs []Doc) {
	sg := build(docs)
	x.mu.Lock()
	x.segs[lib] = sg
	x.mu.Unlock()
}

// Drop forgets a library that's gone.
func (x *Index) Drop(lib int64) {
	x.mu.Lock()
	delete(x.segs, lib)
	x.mu.Unlock()
}

// Has says whether a library has a segment yet.
func (x *Index) Has(lib int64) bool {
	x.mu.RLock()
	defer x.mu.RUnlock()
	_, ok := x.segs[lib]
	return ok
}

// Len is how many documents the index holds.
func (x *Index) Len() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	n := 0
	for _, sg := range x.segs {
		n += len(sg.docs)
	}
	return n
}

// Query is a search.
type Query struct {
	Text string // as typed: year:1999 or year:1990-1999 narrow it (see Parse)
	// Library says whether a library's documents may be found; nil allows all. Lib 0
	// (people) is asked too.
	Library  func(lib int64) bool
	Groups   []string // only these groups; none: all
	YearFrom int
	YearTo   int
	PerGroup int // most hits kept in each group; 0: 12
	// Extra are documents searched along with the index's, scored the same way:
	// what is cheap to list when asked and changes too often to keep (channels).
	Extra []Doc
}

// Hit is a document found, with how well it matched.
type Hit struct {
	Doc
	Score int
}

// Group is what a search found of one kind, best first.
type Group struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Hits  []Hit  `json:"-"`
	More  bool   `json:"more"` // more matched than PerGroup
	Best  int    `json:"-"`
}

// Result is a search's groups, best group first.
type Result struct {
	Text   string // what was searched for, after Parse
	Groups []Group
	Took   time.Duration
}

// Group returns the group with this key, or nil.
func (r *Result) Group(key string) *Group {
	for i := range r.Groups {
		if r.Groups[i].Key == key {
			return &r.Groups[i]
		}
	}
	return nil
}

// Parse takes the filters out of what was typed: year:1999, year:1990-1999 or
// year:1990s. What's left is the text to look for.
func Parse(q *Query) string {
	var rest []string
	for _, w := range strings.Fields(q.Text) {
		lw := strings.ToLower(w)
		if v, ok := strings.CutPrefix(lw, "year:"); ok {
			if from, to, ok := years(v); ok {
				q.YearFrom, q.YearTo = from, to
				continue
			}
		}
		rest = append(rest, w)
	}
	return strings.Join(rest, " ")
}

func years(v string) (int, int, bool) {
	if d, ok := strings.CutSuffix(v, "s"); ok {
		if n, err := strconv.Atoi(d); err == nil && n >= 1000 && n <= 9999 {
			return n, n + 9, true
		}
		return 0, 0, false
	}
	a, b, rng := strings.Cut(v, "-")
	from, err := strconv.Atoi(a)
	if err != nil || from < 1000 || from > 9999 {
		return 0, 0, false
	}
	to := from
	if rng {
		if to, err = strconv.Atoi(b); err != nil || to < from || to > 9999 {
			return 0, 0, false
		}
	}
	return from, to, true
}

// cand is a document that shares enough grams with the search to be scored.
type cand struct {
	sg  *segment
	i   int32
	hit int
}

// scoreBudget is how many documents are scored letter by letter at most: those
// sharing the most grams with the search. Enough that a misspelt word's title is
// among them in a library of several hundred thousand.
const scoreBudget = 3000

var countsPool = sync.Pool{New: func() any { s := make([]uint16, 0, 1024); return &s }}

// Search finds what the query names, grouped.
func (x *Index) Search(q Query) Result {
	start := time.Now()
	text := Parse(&q)
	res := Result{Text: text}
	qn := Normalize(text)
	if utf8.RuneCountInString(strings.ReplaceAll(qn, " ", "")) < 2 {
		res.Took = time.Since(start)
		return res
	}
	if len(qn) > 100 {
		qn = qn[:100]
	}
	var qg []gram
	seen := map[gram]bool{}
	grams(qn, true, func(g gram) {
		if !seen[g] {
			seen[g] = true
			qg = append(qg, g)
		}
	})
	// A document must share a third of the search's grams: "matirx" shares
	// " ma" and "mat" of five with "matrix".
	need := max(1, (len(qg)+2)/3)
	wantGroup := func(g string) bool { return len(q.Groups) == 0 || slices.Contains(q.Groups, g) }
	yearOK := func(y int) bool {
		return q.YearFrom == 0 || (y >= q.YearFrom && y <= q.YearTo)
	}

	// Count each document's shared grams, a segment at a time, and bucket them by count.
	buckets := make([][]cand, len(qg)+1)
	x.mu.RLock()
	defer x.mu.RUnlock()
	libs := make([]int64, 0, len(x.segs))
	for lib := range x.segs {
		if q.Library == nil || q.Library(lib) {
			libs = append(libs, lib)
		}
	}
	slices.Sort(libs) // the same search finds the same, whatever order the map gives
	segs := make([]*segment, 0, len(libs)+1)
	for _, lib := range libs {
		segs = append(segs, x.segs[lib])
	}
	if len(q.Extra) > 0 {
		segs = append(segs, build(q.Extra))
	}
	cp := countsPool.Get().(*[]uint16)
	defer countsPool.Put(cp)
	for _, sg := range segs {
		counts := (*cp)[:0]
		counts = slices.Grow(counts, len(sg.docs))[:len(sg.docs)]
		clear(counts)
		*cp = counts
		var touched []int32
		for _, g := range qg {
			for _, i := range sg.post[g] {
				if counts[i] == 0 {
					touched = append(touched, i)
				}
				counts[i]++
			}
		}
		for _, i := range touched {
			c := int(counts[i])
			if c < need {
				continue
			}
			d := &sg.docs[i]
			if !wantGroup(Groups[d.group]) || !yearOK(int(d.year)) {
				continue
			}
			if len(buckets[c]) < scoreBudget { // never scored past that
				buckets[c] = append(buckets[c], cand{sg, i, c})
			}
		}
	}

	// Score the best-sharing documents letter by letter.
	sc := newScorer(qn)
	byGroup := map[string]*Group{}
	scored := 0
scoring:
	for c := len(buckets) - 1; c >= need; c-- {
		for _, cd := range buckets[c] {
			if scored >= scoreBudget {
				break scoring
			}
			scored++
			s := sc.score(cd.sg.title[cd.i])
			if alt := cd.sg.alt[cd.i]; s < 900 && alt != "" {
				s = max(s, sc.score(alt)-200)
			}
			if s <= 0 {
				continue
			}
			d := cd.sg.docs[cd.i].Doc()
			g := byGroup[d.Group]
			if g == nil {
				g = &Group{Key: d.Group, Label: GroupLabels[d.Group]}
				byGroup[d.Group] = g
			}
			g.Hits = append(g.Hits, Hit{Doc: d, Score: s})
		}
	}
	per := q.PerGroup
	if per <= 0 {
		per = 12
	}
	for _, g := range byGroup {
		slices.SortFunc(g.Hits, func(a, b Hit) int {
			if a.Score != b.Score {
				return b.Score - a.Score
			}
			if a.Count != b.Count {
				return b.Count - a.Count
			}
			if la, lb := len(a.Title), len(b.Title); la != lb {
				return la - lb
			}
			if c := strings.Compare(a.Title, b.Title); c != 0 {
				return c
			}
			return cmp.Compare(a.Lib*1e9+a.ID, b.Lib*1e9+b.ID)
		})
		g.Best = g.Hits[0].Score
		if len(g.Hits) > per {
			g.Hits, g.More = g.Hits[:per], true
		}
		res.Groups = append(res.Groups, *g)
	}
	slices.SortFunc(res.Groups, func(a, b Group) int {
		if a.Best != b.Best {
			return b.Best - a.Best
		}
		return groupOrder(a.Key) - groupOrder(b.Key)
	})
	res.Took = time.Since(start)
	return res
}

// scorer says how well names match one search, reusing its room from one name to the next.
type scorer struct {
	q     string   // the normalised search
	qs    string   // without spaces
	qw    []string // its words
	qr    [][]rune
	nr    []rune // the name being scored, and where its words are
	words [][2]int
	rows  [3][]int
	used  []bool
}

func newScorer(q string) *scorer {
	sc := &scorer{q: q, qs: strings.ReplaceAll(q, " ", ""), qw: strings.Fields(q)}
	for _, w := range sc.qw {
		sc.qr = append(sc.qr, []rune(w))
	}
	return sc
}

// score says how well a name matches the search: the whole name, its start, the
// starts of its words, somewhere inside it, its words with a letter or two wrong,
// or 0 for not at all.
func (sc *scorer) score(name string) int {
	q := sc.q
	if name == "" {
		return 0
	}
	switch {
	case name == q:
		return 1000
	case strings.HasPrefix(name, q):
		return 900 - min(len(name)-len(q), 50)
	}
	// The name's words, as letters.
	sc.nr, sc.words = sc.nr[:0], sc.words[:0]
	start := -1
	for _, r := range name {
		if r == ' ' {
			if start >= 0 {
				sc.words = append(sc.words, [2]int{start, len(sc.nr)})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = len(sc.nr)
		}
		sc.nr = append(sc.nr, r)
	}
	if start >= 0 {
		sc.words = append(sc.words, [2]int{start, len(sc.nr)})
	}
	if sc.wordStarts() {
		return 800 - min(len(name)-len(q), 50)
	}
	if strings.Contains(name, q) {
		return 700 - min(len(name)-len(q), 50)
	}
	// "spiderman" for "Spider-Man", "spider man" for "Spiderman".
	if len(sc.qs) >= 4 && strings.Contains(string(sc.nr), sc.qs) {
		return 650 - min(len(sc.nr)-len(sc.qs), 50)
	}
	// Each word typed near enough a word of the name: the last as the start of one.
	edits, missed, meant := 0, 0, 0
	for wi, w := range sc.qr {
		last := wi == len(sc.qr)-1
		lim := allowed(len(w))
		best, slipped := lim+1, false
		for _, at := range sc.words {
			n := sc.nr[at[0]:at[1]]
			d := distance(w, n, lim, &sc.rows)
			if last && len(n) > len(w) { // still typing: against the start of the word, or a letter more
				for _, cut := range []int{len(w), len(w) + 1} {
					if d > 0 && cut > 0 && cut <= len(n) {
						d = min(d, distance(w, n[:cut], lim, &sc.rows))
					}
				}
			}
			if d > lim && len(w) == 4 && slip(w, n, last) {
				d, slipped = 1, true
			}
			best = min(best, d)
			if best == 0 {
				break
			}
		}
		if best > lim && !(slipped && best == 1) {
			missed++
			if missed > 1 || len(sc.qr) < 3 {
				return 0
			}
			continue
		}
		if !little[sc.qw[wi]] {
			meant++
		}
		edits += best
	}
	switch {
	case missed == 0:
		return 600 - 60*edits
	case meant >= 2: // one word of a longer search not found, but two that say what it's about were
		return 300 - 30*edits
	}
	return 0
}

// wordStarts says whether every word typed starts a different word of the name.
func (sc *scorer) wordStarts() bool {
	if len(sc.qr) > len(sc.words) {
		return false
	}
	sc.used = append(sc.used[:0], make([]bool, len(sc.words))...)
	for _, w := range sc.qr {
		ok := false
		for j, at := range sc.words {
			if !sc.used[j] && at[1]-at[0] >= len(w) && slices.Equal(sc.nr[at[0]:at[0]+len(w)], w) {
				sc.used[j], ok = true, true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}
