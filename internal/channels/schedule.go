package channels

import (
	"cmp"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"slices"
	"time"
)

// Kinds of programme.
const (
	KindShow   = "show"   // something from the library
	KindFiller = "filler" // a bumper from the owner's files
	KindSlate  = "slate"  // nothing to play: the "right back" card
)

// minSlate: gaps shorter than this are not worth starting FFmpeg for.
const minSlate = 3

// maxPerDay stops a day of absurdly short files from growing without limit.
const maxPerDay = 3000

// Programme is one thing on a channel's schedule. Start and End are Unix
// seconds; the rest is a snapshot for the guide, so drawing it needs no
// further queries.
type Programme struct {
	Start   int64  `json:"start"`
	End     int64  `json:"end"`
	Kind    string `json:"kind"`
	ItemID  int64  `json:"itemId,omitempty"`
	ShowID  int64  `json:"showId,omitempty"`
	Title   string `json:"title"`
	Sub     string `json:"sub,omitempty"`
	Year    int    `json:"year,omitempty"`
	Rating  string `json:"rating,omitempty"`
	Genre   string `json:"genre,omitempty"`
	Season  *int   `json:"season,omitempty"`
	Episode *int   `json:"episode,omitempty"`
}

// Entry is a title in a pool.
type Entry struct {
	ID      int64
	Group   int64 // the show, or 0 for films and other standalone titles
	Dur     int64 // seconds
	Title   string
	Sub     string
	Year    int
	Rating  string
	Genre   string
	Aired   string
	Season  *int
	Episode *int
	ShowID  int64
}

// groupCur is where one show has got to: the last item played and, in case
// that file has gone, about where it was.
type groupCur struct {
	Last int64 `json:"l"`
	Pos  int   `json:"p"`
}

// cursor is how far a pool has been played.
type cursor struct {
	Cycle  int                `json:"c,omitempty"`
	Pos    int                `json:"p,omitempty"`
	Last   int64              `json:"l,omitempty"`
	Turn   int                `json:"t,omitempty"`
	Groups map[int64]groupCur `json:"g,omitempty"`
}

// State is what one day hands to the next.
type State struct {
	Carry   int64              `json:"carry"` // when the last programme ends
	Cursors map[string]*cursor `json:"cursors"`
	Filler  uint64             `json:"filler,omitempty"`
}

// PoolLoader loads the titles a set of rules chooses, in their stable order.
// minSec leaves shorter files out.
type PoolLoader func(r Rules, minSec int64) ([]Entry, error)

// rng is SplitMix64: small, and it will never change between Go versions,
// which a stored schedule's determinism relies on.
type rng uint64

func (r *rng) next() uint64 {
	*r += 0x9e3779b97f4a7c15
	z := uint64(*r)
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *rng) intn(n int) int { return int(r.next() % uint64(n)) }

func hashKey(parts ...any) string {
	b, _ := json.Marshal(parts)
	h := sha1.Sum(b)
	return hex.EncodeToString(h[:8])
}

func keySeed(key string, n int64) rng {
	var v uint64
	for i := 0; i < len(key); i++ {
		v = v*131 + uint64(key[i])
	}
	return rng(v ^ uint64(n)*0x100000001b3)
}

// DayNumber is a local date as a day count, stable across DST changes.
func DayNumber(t time.Time, loc *time.Location) int {
	y, m, d := t.In(loc).Date()
	return int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

// DayStart is the moment a day begins.
func DayStart(day int, loc *time.Location) time.Time {
	return time.Date(1970, 1, 1+day, 0, 0, 0, 0, loc)
}

// pool is a set of titles with its order and cursor.
type pool struct {
	entries []Entry
	mode    string
	keep    bool
	key     string
	seed    uint64
	cur     *cursor
	// groups: titles per group in order, the group ids in order.
	groups   map[int64][]int
	groupIDs []int64
	idx      map[int64]int // item id -> position in entries
	gpos     map[int64]int // item id -> position in its group
	// The last shuffle worked out (they are asked for repeatedly).
	permN, permCycle int
	permCached       []int
}

func newPool(entries []Entry, o Order, key string, seed uint64, cur *cursor) *pool {
	p := &pool{entries: entries, mode: o.Mode, keep: o.KeepEpisodeOrder, key: key, seed: seed, cur: cur}
	if p.mode == "" {
		p.mode = "rotate"
	}
	if p.mode == "aired" {
		p.entries = slices.Clone(entries)
		slices.SortStableFunc(p.entries, func(a, b Entry) int {
			switch {
			case a.Aired == "" && b.Aired != "":
				return 1
			case a.Aired != "" && b.Aired == "":
				return -1
			}
			return cmp.Compare(a.Aired, b.Aired)
		})
	}
	p.idx = make(map[int64]int, len(p.entries))
	p.gpos = make(map[int64]int, len(p.entries))
	p.groups = map[int64][]int{}
	for i, e := range p.entries {
		p.idx[e.ID] = i
		if _, ok := p.groups[e.Group]; !ok {
			p.groupIDs = append(p.groupIDs, e.Group)
		}
		p.gpos[e.ID] = len(p.groups[e.Group])
		p.groups[e.Group] = append(p.groups[e.Group], i)
	}
	if p.cur.Groups == nil {
		p.cur.Groups = map[int64]groupCur{}
	}
	return p
}

// perm is the shuffled order of n things in one cycle of this pool.
func (p *pool) perm(n, cycle int) []int {
	if p.permCached != nil && p.permN == n && p.permCycle == cycle {
		return p.permCached
	}
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	r := keySeed(p.key, int64(p.seed)^int64(cycle)<<20)
	for i := n - 1; i > 0; i-- {
		j := r.intn(i + 1)
		out[i], out[j] = out[j], out[i]
	}
	p.permN, p.permCycle, p.permCached = n, cycle, out
	return out
}

// usesGroups: these orders pick a show first, then its next episode.
func (p *pool) usesGroups() bool { return p.mode == "rotate" || (p.mode == "shuffle" && p.keep) }

// groupAt is which show has the turn at position t.
func (p *pool) groupAt(t int) int64 {
	n := len(p.groupIDs)
	if p.mode == "rotate" {
		return p.groupIDs[t%n]
	}
	return p.groupIDs[p.perm(n, t/n)[t%n]]
}

// nextInGroup is the entry after the one this group last played.
func (p *pool) nextInGroup(g int64) (entry, pos int) {
	list := p.groups[g]
	gc := p.cur.Groups[g]
	pos = gc.Pos
	if gc.Last != 0 {
		if i, ok := p.gpos[gc.Last]; ok {
			pos = i + 1
		}
	}
	if pos >= len(list) || pos < 0 {
		pos = 0
	}
	return list[pos], pos
}

// peek is the next title, without moving on.
func (p *pool) peek() *Entry {
	if len(p.entries) == 0 {
		return nil
	}
	if p.usesGroups() {
		i, _ := p.nextInGroup(p.groupAt(p.cur.Turn))
		return &p.entries[i]
	}
	if p.mode == "shuffle" {
		c, pos := p.cur.Cycle, p.cur.Pos
		if pos >= len(p.entries) {
			c, pos = c+1, 0
		}
		return &p.entries[p.perm(len(p.entries), c)[pos]]
	}
	pos := p.cur.Pos
	if p.cur.Last != 0 {
		if i, ok := p.idx[p.cur.Last]; ok {
			pos = i + 1
		}
	}
	if pos >= len(p.entries) {
		pos = 0
	}
	return &p.entries[pos]
}

// advance records that e played.
func (p *pool) advance(e *Entry) {
	if p.usesGroups() {
		if p.groupAt(p.cur.Turn) == e.Group {
			p.cur.Turn++
			// Keep it small and stable across restarts.
			if n := len(p.groupIDs); p.cur.Turn >= n*1024 {
				p.cur.Turn %= n
			}
		}
		_, pos := p.nextInGroup(e.Group)
		p.cur.Groups[e.Group] = groupCur{Last: e.ID, Pos: pos + 1}
		return
	}
	if p.mode == "shuffle" {
		if p.cur.Pos >= len(p.entries) {
			p.cur.Cycle, p.cur.Pos = p.cur.Cycle+1, 0
		}
		p.cur.Pos++
		return
	}
	p.cur.Last = e.ID
	p.cur.Pos = p.idx[e.ID] + 1
}

// fitting looks for another title no longer than limit, for orders that can
// take things out of turn.
func (p *pool) fitting(limit int64) *Entry {
	switch {
	case p.usesGroups():
		// The next episode of each show, starting with the one after this turn.
		n := len(p.groupIDs)
		for k := 1; k < n; k++ {
			i, _ := p.nextInGroup(p.groupAt(p.cur.Turn + k))
			if e := &p.entries[i]; e.Dur <= limit {
				return e
			}
		}
	case p.mode == "shuffle":
		n := len(p.entries)
		perm := p.perm(n, p.cur.Cycle)
		for k := p.cur.Pos + 1; k < n && k < p.cur.Pos+64; k++ {
			if e := &p.entries[perm[k]]; e.Dur <= limit {
				return e
			}
		}
	}
	return nil
}

// segment is a stretch of the day played by one block, or by the channel's
// own rules when block is nil.
type segment struct {
	start, end int64
	block      *Block
	soft       bool // the end is only midnight: programmes may run past it
}

func daySet(b *Block, weekday int) bool {
	if len(b.Days) == 0 {
		return true
	}
	return slices.Contains(b.Days, weekday)
}

// blockSpan is when a block's occurrence starting on this day runs.
func blockSpan(b *Block, dayStart time.Time) (start, end time.Time) {
	y, m, d := dayStart.Date()
	loc := dayStart.Location()
	sm, em := minutes(b.Start), minutes(b.End)
	start = time.Date(y, m, d, 0, sm, 0, 0, loc)
	if em <= sm {
		em += 24 * 60
	}
	end = time.Date(y, m, d, 0, em, 0, 0, loc)
	return start, end
}

// segments cuts one day into what plays when. Blocks are placed in start
// order and an earlier block stops where a later one begins; what they leave
// free goes to the channel's own rules.
func segments(def *Definition, dayStart, dayEnd time.Time) []segment {
	type span struct {
		start, end int64
		b          *Block
		continues  bool
	}
	var spans []span
	if def.Mode != "simple" && def.Mode != "" {
		prev := dayStart.AddDate(0, 0, -1)
		prevWD, wd := int(prev.Weekday()), int(dayStart.Weekday())
		for i := range def.Blocks {
			b := &def.Blocks[i]
			if daySet(b, prevWD) {
				if s, e := blockSpan(b, prev); e.After(dayStart) {
					spans = append(spans, span{start: max(s.Unix(), dayStart.Unix()), end: e.Unix(), b: b})
				}
			}
			if daySet(b, wd) {
				s, e := blockSpan(b, dayStart)
				sp := span{start: s.Unix(), end: e.Unix(), b: b}
				if e.After(dayEnd) {
					sp.end, sp.continues = dayEnd.Unix(), true
				}
				spans = append(spans, sp)
			}
		}
	}
	slices.SortStableFunc(spans, func(a, b span) int { return cmp.Compare(a.start, b.start) })
	var out []segment
	t := dayStart.Unix()
	end := dayEnd.Unix()
	for i, s := range spans {
		if s.end <= t {
			continue
		}
		if s.start > t {
			out = append(out, segment{start: t, end: s.start})
			t = s.start
		}
		stop := s.end
		soft := s.continues
		if i+1 < len(spans) && spans[i+1].start < stop && spans[i+1].start > t {
			stop, soft = spans[i+1].start, false
		}
		out = append(out, segment{start: t, end: stop, block: s.b, soft: soft})
		t = stop
	}
	if t < end {
		out = append(out, segment{start: t, end: end, soft: true})
	}
	return out
}

// generator fills one day.
type generator struct {
	def     *Definition
	seed    uint64
	load    PoolLoader
	state   *State
	pools   map[string]*pool
	filler  []Entry
	fillerR rng
	out     []Programme
	err     error
}

// GenerateDay works out one day's programmes from the state the previous day
// left, and returns the state for the next. cache, when given, keeps pools
// between days generated together, so a pool is loaded only once.
func GenerateDay(def *Definition, seed uint64, day int, loc *time.Location, prev State, load PoolLoader, cache map[string][]Entry) ([]Programme, State, error) {
	g := &generator{def: def, seed: seed, state: &prev, pools: map[string]*pool{}, fillerR: rng(seed ^ uint64(day)<<32 ^ prev.Filler)}
	if prev.Cursors == nil {
		prev.Cursors = map[string]*cursor{}
	}
	g.load = func(r Rules, minSec int64) ([]Entry, error) {
		k := hashKey(r, minSec)
		if e, ok := cache[k]; ok {
			return e, nil
		}
		e, err := load(r, minSec)
		if err == nil && cache != nil {
			cache[k] = e
		}
		return e, err
	}
	dayStart := DayStart(day, loc)
	dayEnd := DayStart(day+1, loc)
	t := max(prev.Carry, dayStart.Unix())
	if def.Filler.Rules != nil {
		minimum := int64(1)
		g.filler, g.err = g.load(*def.Filler.Rules, minimum)
		if g.err != nil {
			return nil, prev, g.err
		}
		if m := int64(def.Filler.MaxSeconds); m > 0 {
			g.filler = slices.DeleteFunc(slices.Clone(g.filler), func(e Entry) bool { return e.Dur > m })
		}
	}
	for _, seg := range segments(def, dayStart, dayEnd) {
		if t >= seg.end {
			continue
		}
		t = g.fill(seg, max(t, seg.start))
		if g.err != nil {
			return nil, prev, g.err
		}
		if len(g.out) >= maxPerDay {
			t = max(t, dayEnd.Unix())
			break
		}
	}
	prev.Carry = t
	prev.Filler = g.fillerR.next()
	return g.out, prev, nil
}

func (g *generator) poolFor(seg segment) (*pool, error) {
	rules, order := g.def.Rules, g.def.Order
	if seg.block != nil {
		if seg.block.Rules != nil {
			rules = *seg.block.Rules
		}
		if seg.block.Order != nil {
			order = *seg.block.Order
		}
	}
	key := hashKey(rules, order)
	if p, ok := g.pools[key]; ok {
		return p, nil
	}
	entries, err := g.load(rules, 30)
	if err != nil {
		return nil, err
	}
	c := g.state.Cursors[key]
	if c == nil {
		c = &cursor{}
		g.state.Cursors[key] = c
	}
	p := newPool(entries, order, key, g.seed, c)
	g.pools[key] = p
	return p, nil
}

func (g *generator) place(start, end int64, kind string, e *Entry) {
	pr := Programme{Start: start, End: end, Kind: kind}
	if e != nil {
		pr.ItemID, pr.ShowID, pr.Title, pr.Sub, pr.Year, pr.Rating, pr.Genre, pr.Season, pr.Episode =
			e.ID, e.ShowID, e.Title, e.Sub, e.Year, e.Rating, e.Genre, e.Season, e.Episode
	}
	g.out = append(g.out, pr)
}

// fill plays a segment from t on and returns when it finished, which may be
// past the segment's end when a programme overran.
func (g *generator) fill(seg segment, t int64) int64 {
	p, err := g.poolFor(seg)
	if err != nil {
		g.err = err
		return t
	}
	fit := "fill"
	if seg.block != nil && seg.block.Fit != "" {
		fit = seg.block.Fit
	}
	blockLen := seg.end - seg.start
	for t < seg.end && len(g.out) < maxPerDay {
		e := p.peek()
		if e == nil {
			// Nothing to play: stay off air until the segment ends.
			g.pad(t, seg.end)
			return seg.end
		}
		end := t + e.Dur
		if end > seg.end && !seg.soft {
			switch {
			case fit == "cut":
				end = seg.end
			case fit == "overrun" || e.Dur > blockLen:
				// A programme longer than the whole block has nowhere else
				// to go; refusing it would leave the block stuck on it.
			default:
				alt := p.fitting(seg.end - t)
				if alt == nil {
					g.pad(t, seg.end)
					return seg.end
				}
				e, end = alt, t+alt.Dur
			}
		}
		p.advance(e)
		g.place(t, end, KindShow, e)
		t = end
		t = g.between(t, seg)
	}
	return t
}

// between adds bumpers after a programme and fills the gap to the time grid.
func (g *generator) between(t int64, seg segment) int64 {
	limit := seg.end
	if seg.soft {
		limit = seg.end + 3600
	}
	for i := 0; i < g.def.Filler.Between; i++ {
		e := g.pickFiller(limit - t)
		if e == nil {
			break
		}
		g.place(t, t+e.Dur, KindFiller, e)
		t += e.Dur
	}
	if g.def.AlignMin > 0 {
		grid := int64(g.def.AlignMin) * 60
		if next := (t + grid - 1) / grid * grid; next > t {
			t = g.pad(t, min(next, limit))
		}
	}
	return t
}

func (g *generator) pickFiller(room int64) *Entry {
	if len(g.filler) == 0 || room <= 0 {
		return nil
	}
	start := g.fillerR.intn(len(g.filler))
	for i := range g.filler {
		e := &g.filler[(start+i)%len(g.filler)]
		if e.Dur <= room {
			return e
		}
	}
	return nil
}

// pad fills [t, until) with filler that fits, then a slate, and returns where
// it stopped: a gap too short for a slate is left for the next programme to
// close by starting a moment early.
func (g *generator) pad(t, until int64) int64 {
	for until-t >= minSlate && len(g.out) < maxPerDay {
		e := g.pickFiller(until - t)
		if e == nil {
			g.place(t, until, KindSlate, nil)
			return until
		}
		g.place(t, t+e.Dur, KindFiller, e)
		t += e.Dur
	}
	return t
}
