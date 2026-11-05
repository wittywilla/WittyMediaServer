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

// Why a bumper is where it is. They all count as KindFiller.
const (
	RolePre     = "pre"     // leads into a programme, and starts its slot
	RolePost    = "post"    // after a programme
	RoleBreak   = "break"   // a break inside a programme
	RoleAd      = "ad"      // commercials after a programme
	RoleBetween = "between" // between commercials
	RolePad     = "pad"     // filling a gap up to the next start time
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
	// Role says why a bumper is here; Rule names the rule that put it here;
	// Mandatory: it may not give way.
	Role      string `json:"role,omitempty"`
	Rule      string `json:"rule,omitempty"`
	Mandatory bool   `json:"mandatory,omitempty"`
	// Offset is how far into the title this stretch starts: a programme with
	// breaks inside it is several stretches.
	Offset int64 `json:"offset,omitempty"`
	// Trimmed is how many seconds were cut off the end to keep to the timetable.
	Trimmed int64 `json:"trimmed,omitempty"`
	// Clip: this plays a scene from a disc, not the whole title.
	Clip *ClipRef `json:"clip,omitempty"`
}

// ClipRef says which part of a disc plays. Times are in the disc's title.
type ClipRef struct {
	Title int     `json:"title"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Label string  `json:"label,omitempty"`
}

// Entry is a title in a pool.
type Entry struct {
	ID int64
	// Item is the library item; 0 means ID. A clip's ID is its own, because a
	// disc can give a pool many clips, and its Item is the disc.
	Item    int64
	Clip    *ClipRef
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

func (e *Entry) itemID() int64 {
	if e.Item != 0 {
		return e.Item
	}
	return e.ID
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
	// Bumpers: how far each bumper rule has got through its videos.
	Bumpers map[string]int `json:"bumpers,omitempty"`
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

// blockOn says whether a block has an occurrence starting on this day.
func blockOn(b *Block, day time.Time) bool {
	if !daySet(b, int(day.Weekday())) {
		return false
	}
	if b.FromDate == "" {
		return true
	}
	d := day.Format("01-02")
	if b.FromDate <= b.ToDate {
		return d >= b.FromDate && d <= b.ToDate
	}
	return d >= b.FromDate || d <= b.ToDate
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
		for i := range def.Blocks {
			b := &def.Blocks[i]
			if blockOn(b, prev) {
				if s, e := blockSpan(b, prev); e.After(dayStart) {
					spans = append(spans, span{start: max(s.Unix(), dayStart.Unix()), end: e.Unix(), b: b})
				}
			}
			if blockOn(b, dayStart) {
				s, e := blockSpan(b, dayStart)
				sp := span{start: s.Unix(), end: e.Unix(), b: b}
				if e.After(dayEnd) {
					sp.end, sp.continues = dayEnd.Unix(), true
				}
				spans = append(spans, sp)
			}
		}
	}
	// A holiday season takes over from what plays all year, for as long as it
	// runs, however the two overlap.
	var seasonal []span
	for _, sp := range spans {
		if sp.b.FromDate != "" {
			seasonal = append(seasonal, sp)
		}
	}
	for _, h := range seasonal {
		var kept []span
		for _, sp := range spans {
			if sp.b.FromDate != "" || sp.end <= h.start || sp.start >= h.end {
				kept = append(kept, sp)
				continue
			}
			if sp.start < h.start {
				before := sp
				before.end, before.continues = h.start, false
				kept = append(kept, before)
			}
			if sp.end > h.end {
				sp.start = h.end
				kept = append(kept, sp)
			}
		}
		spans = kept
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
	// recent: the commercials picked last, newest at the end.
	recent []int64
	rules  []*ruleState
	out    []Programme
	err    error
}

// ruleState is a bumper rule ready to use.
type ruleState struct {
	rule    *BumperRule
	entries []Entry
	only    map[int64]bool // nil = every programme
}

func (r *ruleState) count() int {
	if r.rule.Count <= 0 {
		return 1
	}
	return min(r.rule.Count, len(r.entries))
}

// piece is one bumper or commercial waiting to be placed.
type piece struct {
	e    *Entry
	dur  int64
	role string
	rule string
	must bool
	cut  int64
}

// stop is a break inside a programme, this far into it.
type stop struct {
	at     int64
	pieces []piece
}

// adUnit is one commercial with any bumper leading into it, and which break it
// is in. No stop: it follows the programme.
type adUnit struct {
	pieces []piece
	stop   *stop
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
	if err := g.loadRules(); err != nil {
		return nil, prev, err
	}
	if err := g.loadFiller(); err != nil {
		return nil, prev, err
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

// loadFiller gathers everything this channel may play between programmes: its
// filler rules and each of its commercials libraries. A video one of its bumper
// rules plays is a bumper on this channel, not a commercial as well, even when
// it is in a commercials library.
func (g *generator) loadFiller() error {
	var sets []Rules
	if r := g.def.Filler.Rules; r != nil {
		sets = append(sets, *r)
	}
	for _, c := range g.def.Filler.Commercials {
		if r, ok := c.rules(); ok {
			sets = append(sets, r)
		}
	}
	bumper := map[int64]bool{}
	for _, r := range g.rules {
		for _, e := range r.entries {
			bumper[e.ID] = true
		}
	}
	seen := map[int64]bool{}
	var bumpers []Entry
	for _, r := range sets {
		entries, err := g.load(r, 1)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if m := int64(g.def.Filler.MaxSeconds); seen[e.ID] || (m > 0 && e.Dur > m) {
				continue
			}
			seen[e.ID] = true
			if bumper[e.ID] {
				bumpers = append(bumpers, e)
				continue
			}
			g.filler = append(g.filler, e)
		}
	}
	// Bumpers are all there is: then they are the commercials too.
	if len(g.filler) == 0 {
		g.filler = bumpers
	}
	return nil
}

// loadRules gets each bumper rule's videos, and the programmes it is tied to.
func (g *generator) loadRules() error {
	for i := range g.def.Bumpers.Rules {
		r := &g.def.Bumpers.Rules[i]
		if r.Off {
			continue
		}
		entries, err := g.load(r.Source, 1)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			continue
		}
		rs := &ruleState{rule: r, entries: entries}
		if r.Only != nil {
			match, err := g.load(*r.Only, 0)
			if err != nil {
				return err
			}
			rs.only = make(map[int64]bool, len(match))
			for j := range match {
				rs.only[match[j].itemID()] = true
			}
		}
		g.rules = append(g.rules, rs)
	}
	return nil
}

// applies says whether a rule is for this programme in this part of the day.
func (g *generator) applies(r *ruleState, e *Entry, marathon bool) bool {
	if marathon && !r.rule.Mandatory {
		return false
	}
	return r.only == nil || r.only[e.itemID()]
}

// take gives the rule's next videos, which then take their turn in order.
func (g *generator) take(r *ruleState, role string) []piece {
	if g.state.Bumpers == nil {
		g.state.Bumpers = map[string]int{}
	}
	key := r.rule.ID
	if key == "" {
		key = r.rule.When
	}
	at := g.state.Bumpers[key]
	out := make([]piece, 0, r.count())
	for i := 0; i < r.count(); i++ {
		e := &r.entries[(at+i)%len(r.entries)]
		out = append(out, piece{e: e, dur: e.Dur, role: role, rule: r.rule.Name, must: r.rule.Mandatory})
	}
	g.state.Bumpers[key] = (at + r.count()) % len(r.entries)
	return out
}

// breaksIn counts the breaks an "every N minutes" rule puts in a programme:
// none so close to the end that the viewer would just wait for it to finish.
func breaksIn(dur int64, everyMin int) int {
	every := int64(everyMin) * 60
	n := 0
	for off := every; off < dur-every/2; off += every {
		n++
	}
	return n
}

// mandatoryLen is how long this programme's mandatory bumpers run, without
// using up anyone's turn, so that a slot can be judged before committing to it.
func (g *generator) mandatoryLen(e *Entry, marathon bool) int64 {
	var total int64
	for _, r := range g.rules {
		if !r.rule.Mandatory || r.rule.When == WhenBetween || !g.applies(r, e, marathon) {
			continue
		}
		times := int64(1)
		if r.rule.When == WhenEvery {
			times = int64(breaksIn(e.Dur, r.rule.EveryMin))
		}
		at := g.state.Bumpers[r.rule.ID]
		for t := int64(0); t < times; t++ {
			for i := range r.count() {
				total += r.entries[(at+i)%len(r.entries)].Dur
			}
			at += r.count()
		}
	}
	return total
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

func (g *generator) place(start, end int64, kind string, e *Entry) *Programme {
	pr := Programme{Start: start, End: end, Kind: kind}
	if e != nil {
		pr.ItemID, pr.ShowID, pr.Title, pr.Sub, pr.Year, pr.Rating, pr.Genre, pr.Season, pr.Episode, pr.Clip =
			e.itemID(), e.ShowID, e.Title, e.Sub, e.Year, e.Rating, e.Genre, e.Season, e.Episode, e.Clip
	}
	g.out = append(g.out, pr)
	return &g.out[len(g.out)-1]
}

func (g *generator) placePieces(t int64, list []piece) int64 {
	for _, b := range list {
		if b.dur <= 0 {
			continue
		}
		pr := g.place(t, t+b.dur, KindFiller, b.e)
		pr.Role, pr.Rule, pr.Mandatory, pr.Trimmed = b.role, b.rule, b.must, b.cut
		t += b.dur
	}
	return t
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
	marathon := seg.block != nil && seg.block.Marathon
	trim := g.def.Bumpers.Trim
	blockLen := seg.end - seg.start
	for t < seg.end && len(g.out) < maxPerDay {
		e := p.peek()
		if e == nil {
			// Nothing to play: stay off air until the segment ends.
			g.pad(t, seg.end)
			return seg.end
		}
		dur := e.Dur
		must := g.mandatoryLen(e, marathon)
		if over := t + dur + must - seg.end; over > 0 && !seg.soft {
			switch {
			case trim.On && over <= int64(trim.TailSec) && dur > over:
				dur -= over // allowed to lose this much from its end; play() records it
			case fit == "cut":
				dur = max(seg.end-t-must, 1)
			case fit == "overrun" || e.Dur > blockLen:
				// A programme longer than the whole block has nowhere else
				// to go; refusing it would leave the block stuck on it.
			default:
				alt := p.fitting(seg.end - t - must)
				if alt == nil {
					g.pad(t, seg.end)
					return seg.end
				}
				e, dur = alt, alt.Dur
			}
		}
		p.advance(e)
		t = g.play(seg, t, e, dur, marathon)
	}
	return t
}

// play places one programme with everything around it, and returns when the
// next one may start: first what leads in, the programme with any breaks inside
// it, what follows, then commercials, and a gap filled up to the time grid.
//
// With trimming on, the next start time comes first. It is the first grid time
// that the programme and its mandatory bumpers can make; everything else gets
// what room is left, and gives way in the order the owner chose. See Trim.
func (g *generator) play(seg segment, t int64, e *Entry, dur int64, marathon bool) int64 {
	plan := g.def.Bumpers
	var pre, post []piece
	var between []*ruleState
	var stops []*stop
	stopAt := func(off int64) *stop {
		for _, st := range stops {
			if st.at == off {
				return st
			}
		}
		st := &stop{at: off}
		stops = append(stops, st)
		return st
	}
	for _, r := range g.rules {
		if !g.applies(r, e, marathon) {
			continue
		}
		switch r.rule.When {
		case WhenBefore:
			pre = append(pre, g.take(r, RolePre)...)
		case WhenAfter:
			post = append(post, g.take(r, RolePost)...)
		case WhenBetween:
			between = append(between, r)
		case WhenEvery:
			every := int64(r.rule.EveryMin) * 60
			for off := every; off < dur-every/2; off += every {
				st := stopAt(off)
				st.pieces = append(st.pieces, g.take(r, RoleBreak)...)
			}
		}
	}

	limit := seg.end
	if seg.soft {
		limit = seg.end + 3600
	}
	grid := int64(g.def.AlignMin) * 60
	ads, breakAds, breakEvery := g.def.Filler.Between, g.def.Filler.BreakAds, int64(g.def.Filler.BreakEveryMin)*60
	if marathon {
		grid, ads, breakAds = 0, 0, 0
	}
	if breakAds == 0 || breakEvery == 0 || len(g.filler) == 0 {
		breakEvery = 0
	}
	if breakEvery > 0 {
		for off := breakEvery; off < dur-breakEvery/2; off += breakEvery {
			stopAt(off)
		}
	}
	slices.SortStableFunc(stops, func(a, b *stop) int { return cmp.Compare(a.at, b.at) })
	trim := plan.Trim

	sum := func(list []piece, must bool) (n int64) {
		for _, b := range list {
			if b.must == must {
				n += b.dur
			}
		}
		return n
	}
	bumpers := func(must bool) int64 {
		n := sum(pre, must) + sum(post, must)
		for _, st := range stops {
			if st.at < dur {
				n += sum(st.pieces, must)
			}
		}
		return n
	}

	var target int64 // the start time being kept to, 0 = none
	var room int64
	if trim.On {
		base := t + dur + bumpers(true)
		target = nextStart(base, grid, seg)
		// Could the programme make the start time before that by losing no
		// more than it is allowed to? Then it does, rather than leave a long gap.
		if back := target - grid; grid > 0 && trim.TailSec > 0 && back > t && base > back && base-back <= int64(trim.TailSec) && base-back < dur {
			dur -= base - back // recorded on the programme: see Trimmed
			base = back
			target = back
		}
		if target > 0 {
			room = target - base - bumpers(false)
		}
	} else {
		room = limit - t - dur - bumpers(true) - bumpers(false)
	}
	choosing := Trim{Ads: TrimDrop}
	if trim.On && target > 0 {
		choosing = trim
	}

	// The commercials, in the order they would play. Inside the programme first, then
	// what follows it, which is what gives way first.
	var units []adUnit
	if breakEvery > 0 {
		for _, st := range stops {
			if st.at >= dur || st.at%breakEvery != 0 {
				continue
			}
			for _, u := range g.pickAds(breakAds, between, choosing, room) {
				room -= piecesLen(u)
				units = append(units, adUnit{u, st})
			}
		}
	}
	if trim.On && target > 0 {
		for _, u := range g.pickAds(ads, between, trim, room) {
			room -= piecesLen(u)
			units = append(units, adUnit{u, nil})
		}
		// room has gone below nothing by however much it doesn't fit.
		excess := -room
		units, excess = shrinkAds(units, excess, trim)
		lists := [][]piece{post}
		for i := len(stops) - 1; i >= 0; i-- {
			lists = append(lists, stops[i].pieces)
		}
		lists = append(lists, pre)
		for _, l := range lists {
			excess = shrinkBumpers(l, excess, trim)
		}
	}
	for _, u := range units {
		if u.stop != nil {
			u.stop.pieces = append(u.stop.pieces, u.pieces...)
		}
	}

	t = g.placePieces(t, pre)
	var from int64
	for _, st := range stops {
		// A break that has nothing left in it doesn't interrupt anything.
		if st.at >= dur || piecesLen(st.pieces) == 0 {
			continue
		}
		pr := g.place(t, t+st.at-from, KindShow, e)
		pr.Offset = from
		t += st.at - from
		from = st.at
		t = g.placePieces(t, st.pieces)
	}
	pr := g.place(t, t+dur-from, KindShow, e)
	pr.Offset = from
	pr.Trimmed = e.Dur - dur
	t += dur - from
	t = g.placePieces(t, post)

	if !trim.On || target == 0 {
		for _, u := range g.pickAds(ads, between, Trim{Ads: TrimDrop}, limit-t) {
			t = g.placePieces(t, u)
		}
	} else {
		for _, u := range units {
			if u.stop == nil {
				t = g.placePieces(t, u.pieces)
			}
		}
	}

	if grid > 0 {
		next := (t + grid - 1) / grid * grid
		if target >= t {
			next = target
		}
		if next > t {
			t = g.pad(t, min(next, limit))
		}
		// Keeping to the timetable means exactly: a moment too short for a card
		// goes to what is playing, rather than the next programme starting early.
		if trim.On && target > t && target-t < minSlate && len(g.out) > 0 {
			g.out[len(g.out)-1].End = target
			t = target
		}
	}
	return t
}

func piecesLen(list []piece) (n int64) {
	for _, b := range list {
		n += b.dur
	}
	return n
}

// nextStart is the first start time at or after t: a grid time, or the end of a
// slot that has a firm end. 0 means nothing is holding the next programme back.
func nextStart(t, grid int64, seg segment) int64 {
	var next int64
	if grid > 0 {
		next = (t + grid - 1) / grid * grid
	}
	if !seg.soft && t <= seg.end && (next == 0 || next > seg.end) {
		next = seg.end
	}
	return next
}

// pickAds chooses commercials for a break, each unit being one with any
// bumper that leads into it. room is how long is free; policies that can
// shorten a commercial don't need it to fit whole.
func (g *generator) pickAds(n int, between []*ruleState, trim Trim, room int64) [][]piece {
	var out [][]piece
	for i := 0; i < n; i++ {
		var u []piece
		if i > 0 {
			for _, r := range between {
				u = append(u, g.take(r, RoleBetween)...)
			}
		}
		lead := int64(0)
		for _, b := range u {
			lead += b.dur
		}
		fits := room - lead
		if trim.adsPolicy() != TrimDrop {
			fits = 1 << 40 // anything: it can be shortened, or is to play whatever happens
		}
		e := g.pickFiller(fits)
		if e == nil {
			break
		}
		u = append(u, piece{e: e, dur: e.Dur, role: RoleAd})
		room -= lead + e.Dur
		out = append(out, u)
	}
	return out
}

// shrinkAds takes excess seconds out of the commercials, from the last one back,
// and returns what could not be found.
func shrinkAds(units []adUnit, excess int64, trim Trim) ([]adUnit, int64) {
	policy := trim.adsPolicy()
	if policy == TrimKeep {
		return units, excess
	}
	for i := len(units) - 1; i >= 0 && excess > 0; i-- {
		u := units[i].pieces
		ad := &u[len(u)-1]
		if policy == TrimCut && ad.dur-excess >= trim.minKeep() {
			ad.dur -= excess
			ad.cut = excess
			return units, 0
		}
		// Too little would be left, or it may not be shortened: it goes, along
		// with the bumper that led into it.
		excess -= piecesLen(u)
		units = units[:i]
	}
	return units, excess
}

// shrinkBumpers takes excess seconds out of optional bumpers, from the last back.
func shrinkBumpers(list []piece, excess int64, trim Trim) int64 {
	policy := trim.bumperPolicy()
	if policy == TrimKeep {
		return excess
	}
	for i := len(list) - 1; i >= 0 && excess > 0; i-- {
		b := &list[i]
		if b.must {
			continue
		}
		if policy == TrimCut && b.dur-excess >= trim.minKeep() {
			b.dur -= excess
			b.cut = excess
			return 0
		}
		excess -= b.dur
		b.dur = 0
	}
	return excess
}

// maxRecent: how many commercials go by before one may play again, at most.
const maxRecent = 24

// pickFiller chooses a commercial no longer than room. One that played lately
// waits its turn, so a break of many doesn't show the same one twice; it plays
// again only when nothing else fits.
func (g *generator) pickFiller(room int64) *Entry {
	if len(g.filler) == 0 || room <= 0 {
		return nil
	}
	start := g.fillerR.intn(len(g.filler))
	var again *Entry
	for i := range g.filler {
		e := &g.filler[(start+i)%len(g.filler)]
		if e.Dur > room {
			continue
		}
		if !slices.Contains(g.recent, e.itemID()) {
			g.played(e)
			return e
		}
		if again == nil || g.playedAt(e) < g.playedAt(again) {
			again = e
		}
	}
	if again != nil {
		g.played(again)
	}
	return again
}

// played remembers a commercial, keeping as many as half the videos there are.
func (g *generator) played(e *Entry) {
	id := e.itemID()
	if i := slices.Index(g.recent, id); i >= 0 {
		g.recent = slices.Delete(g.recent, i, i+1)
	}
	g.recent = append(g.recent, id)
	if n := min(maxRecent, len(g.filler)/2); len(g.recent) > n {
		g.recent = g.recent[len(g.recent)-n:]
	}
}

// playedAt is how far back a commercial is in what played lately: lower is longer ago.
func (g *generator) playedAt(e *Entry) int { return slices.Index(g.recent, e.itemID()) }

// pad fills [t, until) with filler that fits, then a slate, and returns where
// it stopped: a gap too short for a slate is left for the next programme to
// close by starting a moment early.
func (g *generator) pad(t, until int64) int64 {
	trim := g.def.Bumpers.Trim
	for until-t >= minSlate && len(g.out) < maxPerDay {
		e := g.pickFiller(until - t)
		if e == nil {
			// When commercials may be cut short, one can fill the gap exactly.
			if trim.On && trim.adsPolicy() == TrimCut && until-t >= trim.minKeep() {
				if e = g.pickFiller(1 << 40); e != nil {
					pr := g.place(t, until, KindFiller, e)
					pr.Role, pr.Trimmed = RolePad, e.Dur-(until-t)
					return until
				}
			}
			g.place(t, until, KindSlate, nil)
			return until
		}
		g.place(t, t+e.Dur, KindFiller, e).Role = RolePad
		t += e.Dur
	}
	return t
}
