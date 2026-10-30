package wittydvd

import (
	"fmt"
	"math"
)

const maxHops = 1000

// StepKind is what the navigator wants shown.
type StepKind string

const (
	StepMenu  StepKind = "menu"
	StepTitle StepKind = "title"
	StepStop  StepKind = "stop"
)

// TitleInfo describes the title being played in a title step.
type TitleInfo struct {
	TTN      int     `json:"ttn"`
	VTSTTN   int     `json:"vtsTtn"`
	PTTN     int     `json:"pttn"`
	Cells    []Cell  `json:"-"`
	CellNums []int   `json:"cellNums"`
	Duration float64 `json:"durationSec"`
}

// Step is the navigator's instruction to the presentation layer.
type Step struct {
	Kind          StepKind   `json:"kind"`
	Domain        Domain     `json:"-"`
	DomainName    string     `json:"domain"`
	VTS           int        `json:"vts"`
	PGCN          int        `json:"pgcn"`
	CellN         int        `json:"cellN"`
	FirstSector   uint32     `json:"firstSector"`
	LastSector    uint32     `json:"lastSector"`
	StillSec      float64    `json:"stillSec"` // +Inf = wait for the viewer
	Loop          bool       `json:"loop"`
	Palette       [16]uint32 `json:"palette"`
	DefaultButton int        `json:"defaultButton"`
	Serial        int        `json:"serial"`
	Reason        string     `json:"reason,omitempty"`
	Hold          bool       `json:"hold,omitempty"`
	Resumed       bool       `json:"resumed,omitempty"`
	Title         *TitleInfo `json:"title,omitempty"`
	CellDuration  float64    `json:"cellDurationSec"`
}

// NavOptions configure a navigator.
type NavOptions struct {
	RegisterOptions
}

type resumePoint struct {
	vts, pgcn, cellN int
	atPoint          bool
	sprm             [5]uint16
}

var resumeSPRMs = [5]int{SprmTTN, SprmVTSTTN, SprmTTPGCN, SprmPTTN, SprmHLBTNN}

type navState struct {
	regs     *Registers
	domain   Domain
	vts      int
	pgcn     int
	cellN    int
	held     bool
	resume   *resumePoint
	serial   int
	current  *Step
	inButton bool
	// Random and shuffle program chains: programs started so far in the
	// current PGC, and which ones (shuffle never repeats).
	pbCount  int
	pbPlayed [128]bool
	// menuLU remembers, per menu space (0 = VMGM, n = VTSM n), the language
	// unit (index+1) used because SPRM0's unit lacked a requested menu.
	menuLU [maxTitleSets + 1]uint8
	// skips counts unplayable cells passed in a row (junk structures).
	skips int
}

func (s *navState) clone() *navState {
	c := *s
	c.regs = s.regs.Clone()
	if s.resume != nil {
		r := *s.resume
		c.resume = &r
	}
	c.inButton = false
	return &c
}

// action is an internal navigator transition.
type actKind int

const (
	actPGC actKind = iota
	actCell
	actPost
	actVM
	actLink
	actJump
	actResume
	actNoop
	actStop
	actDone // carries a finished Step
)

type action struct {
	t       actKind
	domain  Domain
	vts     int
	pgcn    int
	program int
	cell    int
	noPre   bool
	n       int
	resumed bool
	r       Result
	link    *Link
	jump    *Jump
	reason  string
	step    *Step
}

func stopA(reason string) action { return action{t: actStop, reason: reason} }
func pgcA(d Domain, vts, pgcn int) action {
	return action{t: actPGC, domain: d, vts: vts, pgcn: pgcn}
}
func cellA(n int) action { return action{t: actCell, n: n} }

// Nav is a DVD navigator for one viewer.
type Nav struct {
	Disc *Disc
	opts NavOptions
	st   *navState
}

// NewNav creates a navigator over parsed IFOs.
func NewNav(d *Disc, o NavOptions) *Nav {
	n := &Nav{Disc: d, opts: o}
	n.st = n.fresh()
	return n
}

func (n *Nav) fresh() *navState {
	ro := n.opts.RegisterOptions
	if ro.Region == 0 {
		ro.Region = n.Disc.Region()
	}
	return &navState{regs: NewRegisters(ro)}
}

// Clone returns an independent navigator in the same state (for what-if
// exploration such as `wms dvd crawl`).
func (n *Nav) Clone() *Nav {
	return &Nav{Disc: n.Disc, opts: n.opts, st: n.st.clone()}
}

// Registers exposes the live register file (read-only use recommended).
func (n *Nav) Registers() *Registers { return n.st.regs }

// Current returns the last step.
func (n *Nav) Current() *Step { return n.st.current }

// ---------------------------------------------------------------------------
// Cell helpers
// ---------------------------------------------------------------------------

func stillOf(t int) float64 {
	switch {
	case t <= 0:
		return 0
	case t == 0xff:
		return math.Inf(1)
	}
	return float64(t)
}

// angleCell maps cell n to the cell for the current angle inside an angle block.
func angleCell(p *PGC, n, angle int) int {
	if n < 1 || n > len(p.Cells) {
		return n
	}
	c := p.Cells[n-1]
	if c.BlockType != 1 || c.BlockMode != 1 {
		return n
	}
	k := n
	for a := 1; a < angle; a++ {
		if k >= len(p.Cells) || p.Cells[k-1].BlockMode == 3 {
			break
		}
		k++
	}
	return k
}

// nextCellN is the cell after n, skipping the rest of an angle block.
func nextCellN(p *PGC, n int) int {
	if n < 1 || n > len(p.Cells) {
		return n + 1
	}
	c := p.Cells[n-1]
	if c.BlockType != 1 {
		return n + 1
	}
	k := n
	for k <= len(p.Cells) && p.Cells[k-1].BlockType == 1 && p.Cells[k-1].BlockMode != 3 {
		k++
	}
	return k + 1
}

// programOf is the 1-based program that contains cell n.
func programOf(p *PGC, n int) int {
	pg := 1
	for i, entry := range p.Programs {
		if entry <= n {
			pg = i + 1
		}
	}
	return pg
}

// ---------------------------------------------------------------------------
// PGC lookup
// ---------------------------------------------------------------------------

// langUnit is the index of the menu language unit for SPRM0, or -1.
func langUnit(units []LangUnit, want uint16) int {
	for i, u := range units {
		if u.Language != "" && code2(u.Language) == want {
			return i
		}
	}
	return -1
}

// menuUnits returns the menu language units of a menu space and its key in
// navState.menuLU.
func (n *Nav) menuUnits(d Domain, vts int) ([]LangUnit, int) {
	switch d {
	case DomainVMGM:
		return n.Disc.VMG.Menus, 0
	case DomainVTSM:
		if v := n.Disc.VTS[vts]; v != nil && vts >= 1 && vts <= maxTitleSets {
			return v.Menus, vts
		}
	}
	return nil, 0
}

// menuLangUnit picks the language unit a menu space plays from: one chosen
// earlier because SPRM0's unit lacked a menu, else SPRM0's, else the first.
func (n *Nav) menuLangUnit(s *navState, units []LangUnit, key int) int {
	if k := int(s.menuLU[key]); k >= 1 && k <= len(units) {
		return k - 1
	}
	if i := langUnit(units, s.regs.SPRM[SprmMenuLang]); i >= 0 {
		return i
	}
	return 0
}

func (n *Nav) pgcList(s *navState, d Domain, vts int) []PGCEntry {
	switch d {
	case DomainVMGM, DomainVTSM:
		units, key := n.menuUnits(d, vts)
		if len(units) == 0 {
			return nil
		}
		return units[n.menuLangUnit(s, units, key)].PGCs
	case DomainVTS:
		if v := n.Disc.VTS[vts]; v != nil {
			return v.PGCs
		}
	}
	return nil
}

func (n *Nav) pgcOf(s *navState, d Domain, vts, pgcn int) *PGC {
	if d == DomainFP {
		return n.Disc.VMG.FirstPlay
	}
	list := n.pgcList(s, d, vts)
	if pgcn < 1 || pgcn > len(list) {
		return nil
	}
	return list[pgcn-1].PGC
}

func (n *Nav) curPGC(s *navState) *PGC { return n.pgcOf(s, s.domain, s.vts, s.pgcn) }

// StepPGC returns the program chain a step plays, resolved the way the
// navigator does (menu language unit, first play).
func (n *Nav) StepPGC(st *Step) *PGC {
	if st == nil {
		return nil
	}
	return n.pgcOf(n.st, st.Domain, st.VTS, st.PGCN)
}

// playable reports whether a cell can be read at all. Junk cells (structural
// copy protection, damaged IFOs) point backwards or start past the end of
// their VOB; the navigator passes over them like cells with no picture.
func (n *Nav) playable(s *navState, c Cell) bool {
	if c.LastSector < c.FirstSector {
		return false
	}
	size := n.Disc.vobSectors(s.domain, s.vts)
	return size == 0 || c.FirstSector < size
}

// randomMode decodes a PGC's program playback mode: 0 is sequential,
// otherwise bit 7 chooses shuffle (1) or random (0) and bits 6-0 the number
// of programs to play.
func randomMode(p *PGC) (shuffle bool, count int, ok bool) {
	if p == nil || p.PlaybackMode == 0 || len(p.Programs) == 0 {
		return false, 0, false
	}
	return p.PlaybackMode&0x80 != 0, int(p.PlaybackMode & 0x7f), true
}

// pickProgram chooses the next program of a random (repeats allowed) or
// shuffle (each program once) PGC; 0 when the PGC has played its count.
func (n *Nav) pickProgram(s *navState, p *PGC) int {
	shuffle, count, ok := randomMode(p)
	if !ok || s.pbCount >= count {
		return 0
	}
	np := len(p.Programs)
	if np >= len(s.pbPlayed) {
		np = len(s.pbPlayed) - 1
	}
	pick := func(k int) int {
		i := int(s.regs.Rand() * float64(k))
		return max(0, min(i, k-1))
	}
	pg := 1 + pick(np)
	if shuffle {
		var left []int
		for i := 1; i <= np; i++ {
			if !s.pbPlayed[i] {
				left = append(left, i)
			}
		}
		if len(left) == 0 {
			return 0
		}
		pg = left[pick(len(left))]
	}
	s.pbCount++
	s.pbPlayed[pg] = true
	return pg
}

// parentalPGC picks, inside a parental block of title PGCs (alternative
// versions of a scene), the one PTL_MAIT names for the player's parental
// level and country. Outside a block, or when the disc has no table, the PGC
// is played as named.
func (n *Nav) parentalPGC(s *navState, vts, pgcn int) int {
	list := n.pgcList(s, DomainVTS, vts)
	i := pgcn - 1
	if i < 0 || i >= len(list) || list[i].BlockType != 1 {
		return pgcn
	}
	first, last := i, i
	for first > 0 && list[first].BlockMode != 1 && list[first-1].BlockType == 1 {
		first--
	}
	for last < len(list)-1 && list[last].BlockMode != 3 && list[last+1].BlockType == 1 {
		last++
	}
	mask := n.Disc.ParentalMask(s.regs.SPRM[SprmCountry], int(s.regs.SPRM[SprmParental]), vts)
	if mask == 0 {
		return pgcn
	}
	for k := first; k <= last; k++ {
		if uint16(list[k].ParentalMask)&mask != 0 {
			return k + 1
		}
	}
	return pgcn
}

func (n *Nav) globalTitle(vts, vtsTTN int) int {
	for _, t := range n.Disc.VMG.Titles {
		if t.VTS == vts && t.VTSTitle == vtsTTN {
			return t.Number
		}
	}
	return 0
}

func (n *Nav) titleVTS(s *navState) int {
	ttn := int(s.regs.SPRM[SprmTTN])
	if ttn >= 1 && ttn <= len(n.Disc.VMG.Titles) {
		return n.Disc.VMG.Titles[ttn-1].VTS
	}
	return 0
}

func (n *Nav) pttList(s *navState) []PTT {
	v := n.Disc.VTS[s.vts]
	if v == nil {
		return nil
	}
	t := int(s.regs.SPRM[SprmVTSTTN])
	if t < 1 || t > len(v.PTT) {
		return nil
	}
	return v.PTT[t-1]
}

// ---------------------------------------------------------------------------
// Presenting
// ---------------------------------------------------------------------------

func highlighted(s *navState) int {
	b := int(s.regs.SPRM[SprmHLBTNN] >> 10)
	if b >= 1 && b <= 36 {
		return b
	}
	return 0
}

func (n *Nav) stopStep(s *navState, reason string) *Step {
	s.serial++
	s.current = &Step{Kind: StepStop, Domain: s.domain, DomainName: s.domain.String(), VTS: s.vts, PGCN: s.pgcn, CellN: s.cellN, Serial: s.serial, Reason: reason}
	return s.current
}

func (n *Nav) titleInfo(s *navState, p *PGC) *TitleInfo {
	angle := int(s.regs.SPRM[SprmAngle])
	ti := &TitleInfo{TTN: int(s.regs.SPRM[SprmTTN]), VTSTTN: int(s.regs.SPRM[SprmVTSTTN]), PTTN: int(s.regs.SPRM[SprmPTTN])}
	for c := s.cellN; c >= 1 && c <= len(p.Cells); c = nextCellN(p, c) {
		k := angleCell(p, c, angle)
		ti.Cells = append(ti.Cells, p.Cells[k-1])
		ti.CellNums = append(ti.CellNums, k)
		ti.Duration += p.Cells[k-1].Duration
	}
	return ti
}

func (n *Nav) present(s *navState, resumed bool, sim bool) *Step {
	p := n.curPGC(s)
	cell := p.Cells[s.cellN-1]
	// In a random PGC any program may be the last one played; its PGC still
	// comes after the cell instead (see afterCell).
	_, _, random := randomMode(p)
	last := nextCellN(p, s.cellN) > len(p.Cells) && !random
	still := stillOf(cell.StillTime)
	if s.held {
		still = stillOf(p.StillTime)
	} else if last && cell.Command == 0 {
		still += stillOf(p.StillTime)
	}
	s.serial++
	st := &Step{
		Kind: StepMenu, Domain: s.domain, DomainName: s.domain.String(), VTS: s.vts, PGCN: s.pgcn, CellN: s.cellN,
		FirstSector: cell.FirstSector, LastSector: cell.LastSector, StillSec: still, Palette: p.Palette,
		DefaultButton: highlighted(s), Serial: s.serial, Hold: s.held, Resumed: resumed, CellDuration: cell.Duration,
	}
	if s.domain == DomainVTS {
		st.Kind = StepTitle
		st.Title = n.titleInfo(s, p)
	}
	s.current = st
	if !sim {
		st.Loop = n.loops(s, st)
	}
	return st
}

// loops reports whether, when this cell ends, the disc simply replays it with
// identical state (an animated menu loop). Clients then loop locally.
func (n *Nav) loops(s *navState, st *Step) bool {
	if st.Kind == StepStop || st.Hold || math.IsInf(st.StillSec, 1) {
		return false
	}
	c := s.clone()
	q := n.cellEnded(c, true)
	return q.Kind == st.Kind && q.Domain == st.Domain && q.VTS == st.VTS && q.PGCN == st.PGCN && q.CellN == st.CellN &&
		!q.Hold && c.regs.GPRM == s.regs.GPRM && c.regs.SPRM == s.regs.SPRM && c.regs.Counter == s.regs.Counter
}

func (n *Nav) keep(s *navState, reason string) *Step {
	cur := s.current
	if cur == nil {
		if reason == "" {
			reason = "the disc has not been started"
		}
		return n.stopStep(s, reason)
	}
	p := *cur
	if p.Kind != StepStop {
		p.DefaultButton = highlighted(s)
		p.Reason = ""
	}
	if reason != "" {
		p.Reason = reason
	}
	s.current = &p
	if p.Kind != StepStop {
		p.Loop = n.loops(s, &p)
	}
	return &p
}

// ---------------------------------------------------------------------------
// The state machine
// ---------------------------------------------------------------------------

func (n *Nav) settle(s *navState, a action, sim bool) *Step {
	for hop := 0; hop < maxHops; hop++ {
		if a.t == actDone {
			return a.step
		}
		a = n.step(s, a, sim)
	}
	return n.stopStep(s, fmt.Sprintf("the disc's programs kept jumping without showing anything (%d hops)", maxHops))
}

func done(st *Step) action { return action{t: actDone, step: st} }

func (n *Nav) step(s *navState, a action, sim bool) action {
	switch a.t {
	case actPGC:
		return n.enterPGC(s, a)
	case actCell:
		return done(n.showCell(s, a, sim))
	case actPost:
		return n.runPost(s)
	case actVM:
		return n.fromVM(a.r)
	case actLink:
		return n.doLink(s, a.link)
	case actJump:
		return n.doJump(s, a.jump)
	case actResume:
		return n.doResume(s)
	case actNoop:
		if s.inButton && s.current != nil && s.current.Kind != StepStop {
			return done(n.keep(s, a.reason))
		}
		return done(n.stopStep(s, a.reason))
	}
	if a.reason == "" {
		a.reason = "stopped"
	}
	return done(n.stopStep(s, a.reason))
}

func (n *Nav) fromVM(r Result) action {
	switch r.Kind {
	case ResultLink:
		return action{t: actLink, link: r.Link}
	case ResultJump:
		return action{t: actJump, jump: r.Jump}
	case ResultExit:
		return stopA("the disc ended playback (Exit)")
	}
	if r.Reason == "" {
		r.Reason = "the program stopped"
	}
	return stopA(r.Reason)
}

func (n *Nav) enterPGC(s *navState, a action) action {
	p := n.pgcOf(s, a.domain, a.vts, a.pgcn)
	if p == nil {
		where := ""
		if a.vts != 0 {
			where = fmt.Sprintf(" of title set %d", a.vts)
		}
		return stopA(fmt.Sprintf("%s program chain %d%s does not exist", a.domain, a.pgcn, where))
	}
	if a.domain == DomainVTS {
		if q := n.parentalPGC(s, a.vts, a.pgcn); q != a.pgcn {
			a.pgcn, p = q, n.pgcOf(s, a.domain, a.vts, q)
		}
	}
	s.domain, s.vts, s.pgcn, s.cellN, s.held = a.domain, a.vts, a.pgcn, 0, false
	s.pbCount, s.pbPlayed = 0, [128]bool{}
	if a.domain == DomainVTS {
		s.regs.SPRM[SprmTTPGCN] = uint16(a.pgcn)
		e := n.pgcList(s, DomainVTS, a.vts)[a.pgcn-1]
		if e.Entry && e.TitleNumber != 0 && e.TitleNumber != int(s.regs.SPRM[SprmVTSTTN]) {
			s.regs.SPRM[SprmVTSTTN] = uint16(e.TitleNumber)
			if g := n.globalTitle(a.vts, e.TitleNumber); g != 0 {
				s.regs.SPRM[SprmTTN] = uint16(g)
			}
		}
	}
	if !a.noPre {
		r := s.regs.Run(p.Pre, RunOptions{})
		if r.Kind != ResultEnd && r.Kind != ResultBreak {
			return action{t: actVM, r: r}
		}
	}
	if len(p.Cells) == 0 {
		return action{t: actPost}
	}
	cell := 1
	if a.program > 0 && a.program <= len(p.Programs) {
		cell = p.Programs[a.program-1]
		if _, _, ok := randomMode(p); ok && a.program < len(s.pbPlayed) {
			s.pbCount, s.pbPlayed[a.program] = 1, true
		}
	} else if a.cell > 0 {
		cell = a.cell
	} else if _, _, ok := randomMode(p); ok {
		pg := n.pickProgram(s, p)
		if pg == 0 {
			return action{t: actPost}
		}
		cell = p.Programs[pg-1]
	}
	if cell < 1 {
		cell = 1
	}
	return action{t: actCell, n: cell, resumed: a.resumed}
}

func (n *Nav) showCell(s *navState, a action, sim bool) *Step {
	p := n.curPGC(s)
	if p == nil {
		return n.stopStep(s, "no program chain")
	}
	if a.n > len(p.Cells) {
		// Ran off the end: run post commands.
		return n.settle(s, action{t: actPost}, sim)
	}
	if a.n < 1 {
		a.n = 1
	}
	s.cellN = angleCell(p, a.n, int(s.regs.SPRM[SprmAngle]))
	s.held = false
	if s.domain == DomainVTS {
		pgn := programOf(p, s.cellN)
		for i, x := range n.pttList(s) {
			if x.PGCN == s.pgcn && x.PGN <= pgn {
				s.regs.SPRM[SprmPTTN] = uint16(i + 1)
			}
		}
	}
	if !n.playable(s, p.Cells[s.cellN-1]) {
		s.skips++
		if s.skips > maxHops {
			s.skips = 0
			return n.stopStep(s, "the program chain has no playable cells")
		}
		return n.afterCell(s, sim)
	}
	s.skips = 0
	return n.present(s, a.resumed, sim)
}

// cellEnded advances when the current cell (and any still) has finished.
func (n *Nav) cellEnded(s *navState, sim bool) *Step {
	cur := s.current
	if cur == nil || cur.Kind == StepStop {
		return n.keep(s, "")
	}
	if s.held {
		return n.settle(s, action{t: actPost}, sim)
	}
	return n.afterCell(s, sim)
}

// afterCell runs the current cell's command and moves on: to the next cell
// (in a random PGC, the next program drawn), a PGC still, or the post
// commands.
func (n *Nav) afterCell(s *navState, sim bool) *Step {
	p := n.curPGC(s)
	if p == nil || s.cellN < 1 || s.cellN > len(p.Cells) {
		return n.stopStep(s, "no program chain")
	}
	cell := p.Cells[s.cellN-1]
	if cell.Command != 0 {
		r := s.regs.Run(p.CellCmds, RunOptions{Start: cell.Command - 1, Once: true})
		if r.Kind != ResultEnd && r.Kind != ResultBreak {
			return n.settle(s, action{t: actVM, r: r}, sim)
		}
	}
	next := nextCellN(p, s.cellN)
	_, _, random := randomMode(p)
	if random && (next > len(p.Cells) || programOf(p, next) != programOf(p, s.cellN)) {
		next = len(p.Cells) + 1
		if pg := n.pickProgram(s, p); pg > 0 {
			next = p.Programs[pg-1]
		}
	}
	if next <= len(p.Cells) {
		return n.settle(s, cellA(next), sim)
	}
	if (cell.Command != 0 || random) && p.StillTime != 0 && n.playable(s, cell) {
		s.held = true
		return n.present(s, false, sim)
	}
	return n.settle(s, action{t: actPost}, sim)
}

func (n *Nav) runPost(s *navState) action {
	p := n.curPGC(s)
	s.held = false
	r := s.regs.Run(p.Post, RunOptions{})
	if r.Kind != ResultEnd && r.Kind != ResultBreak {
		return action{t: actVM, r: r}
	}
	switch s.domain {
	case DomainFP:
		return n.playTitle(s, 1, 1)
	case DomainVTS:
		if next := nextTitlePGC(n.Disc.VTS[s.vts], int(s.regs.SPRM[SprmVTSTTN]), s.pgcn); next != 0 {
			return pgcA(DomainVTS, s.vts, next)
		}
		return stopA(fmt.Sprintf("title %d ended", s.regs.SPRM[SprmTTN]))
	}
	return stopA("the menu ended without saying where to go")
}

func (n *Nav) doLink(s *navState, l *Link) action {
	p := n.curPGC(s)
	if p == nil {
		return stopA("no program chain")
	}
	cur := s.cellN
	if cur == 0 {
		cur = 1
	}
	rel := func(target int, what string) action {
		if target != 0 && s.domain != DomainFP {
			return pgcA(s.domain, s.vts, target)
		}
		return stopA(fmt.Sprintf("there is no %s program chain", what))
	}
	progCell := func(pg int) int {
		if pg >= 1 && pg <= len(p.Programs) {
			return p.Programs[pg-1]
		}
		return 1
	}
	switch l.Op {
	case LinkTopCell:
		return cellA(cur)
	case LinkNextCell:
		if s.cellN == 0 {
			return cellA(1)
		}
		return cellA(nextCellN(p, s.cellN))
	case LinkPrevCell:
		if cur > 1 {
			return cellA(cur - 1)
		}
		return cellA(1)
	case LinkTopPG:
		return cellA(progCell(programOf(p, cur)))
	case LinkNextPG:
		pg := 1
		if s.cellN != 0 {
			pg = programOf(p, cur) + 1
		}
		if pg <= len(p.Programs) {
			return cellA(p.Programs[pg-1])
		}
		return action{t: actPost}
	case LinkPrevPG:
		pg := programOf(p, cur) - 1
		if pg < 1 {
			pg = 1
		}
		return cellA(progCell(pg))
	case LinkTopPGC:
		return pgcA(s.domain, s.vts, s.pgcn)
	case LinkNextPGC:
		return rel(p.NextPGCN, "next")
	case LinkPrevPGC:
		return rel(p.PrevPGCN, "previous")
	case LinkGoUpPGC:
		return rel(p.GoUpPGCN, "go-up")
	case LinkTailPGC:
		return action{t: actPost}
	case LinkRSM:
		return action{t: actResume}
	case LinkPGCN:
		return rel(l.N, fmt.Sprint(l.N))
	case LinkPGN:
		if l.N >= 1 && l.N <= len(p.Programs) {
			return cellA(p.Programs[l.N-1])
		}
		return stopA(fmt.Sprintf("program %d does not exist", l.N))
	case LinkCN:
		if l.N >= 1 && l.N <= len(p.Cells) {
			return cellA(l.N)
		}
		return stopA(fmt.Sprintf("cell %d does not exist", l.N))
	case LinkPTTN:
		if s.domain != DomainVTS {
			return stopA("LinkPTTN outside a title")
		}
		list := n.pttList(s)
		if l.N < 1 || l.N > len(list) {
			return stopA(fmt.Sprintf("chapter %d does not exist", l.N))
		}
		ptt := list[l.N-1]
		if ptt.PGCN == s.pgcn {
			return cellA(progCell(ptt.PGN))
		}
		return action{t: actPGC, domain: DomainVTS, vts: s.vts, pgcn: ptt.PGCN, program: ptt.PGN}
	}
	return stopA("unknown link")
}

func (n *Nav) playTitle(s *navState, ttn, pttn int) action {
	if ttn < 1 || ttn > len(n.Disc.VMG.Titles) {
		return stopA(fmt.Sprintf("title %d does not exist", ttn))
	}
	t := n.Disc.VMG.Titles[ttn-1]
	return n.playVTSTitle(s, t.VTS, t.VTSTitle, pttn, ttn)
}

func (n *Nav) playVTSTitle(s *navState, vts, vtsTTN, pttn, ttn int) action {
	v := n.Disc.VTS[vts]
	if v == nil {
		return stopA(fmt.Sprintf("title set %d is missing", vts))
	}
	var ptts []PTT
	if vtsTTN >= 1 && vtsTTN <= len(v.PTT) {
		ptts = v.PTT[vtsTTN-1]
	}
	var target PTT
	if pttn >= 1 && pttn <= len(ptts) && ptts[pttn-1].PGCN >= 1 && ptts[pttn-1].PGCN <= len(v.PGCs) {
		target = ptts[pttn-1]
	} else {
		// No chapter table entry, or one naming a PGC that does not exist:
		// chapter 1 is the title's entry PGC.
		idx := -1
		for i, e := range v.PGCs {
			if e.Entry && e.TitleNumber == vtsTTN {
				idx = i
				break
			}
		}
		if pttn > 1 || idx < 0 {
			return stopA(fmt.Sprintf("title %d of title set %d has no chapter %d", vtsTTN, vts, pttn))
		}
		target = PTT{PGCN: idx + 1, PGN: 1}
	}
	if ttn == 0 {
		ttn = n.globalTitle(vts, vtsTTN)
	}
	if ttn != 0 {
		s.regs.SPRM[SprmTTN] = uint16(ttn)
	}
	s.regs.SPRM[SprmVTSTTN] = uint16(vtsTTN)
	s.regs.SPRM[SprmPTTN] = uint16(pttn)
	program := target.PGN
	if _, _, ok := randomMode(v.PGCs[target.PGCN-1].PGC); ok && pttn <= 1 {
		program = 0 // a random title starts with a program drawn at random
	}
	return action{t: actPGC, domain: DomainVTS, vts: vts, pgcn: target.PGCN, program: program}
}

// menuAction enters the entry PGC of a menu type. The language unit for
// SPRM0 is searched first; when it lacks that menu (or the disc has no unit
// in that language), the first other unit that has it is used and
// remembered, so the menu's own links stay inside that unit.
func (n *Nav) menuAction(s *navState, d Domain, vts, typ int) action {
	units, key := n.menuUnits(d, vts)
	def := max(langUnit(units, s.regs.SPRM[SprmMenuLang]), 0)
	order := []int{def}
	if k := int(s.menuLU[key]); k >= 1 && k-1 != def {
		order = append(order, k-1)
	}
	for i := range units {
		order = append(order, i)
	}
	for _, u := range order {
		if u >= len(units) {
			continue
		}
		for i, e := range units[u].PGCs {
			if e.Entry && e.MenuType == typ {
				s.menuLU[key] = 0
				if u != def {
					s.menuLU[key] = uint8(u + 1)
				}
				return pgcA(d, vts, i+1)
			}
		}
	}
	who := "the disc"
	if d == DomainVTSM {
		who = fmt.Sprintf("title set %d", vts)
	}
	return stopA(fmt.Sprintf("%s has no menu of type %d", who, typ))
}

func (n *Nav) enterSS(s *navState, j *Jump) action {
	switch j.Domain {
	case DomainFP:
		return pgcA(DomainFP, 0, 0)
	case DomainVMGM:
		if j.HasPGCN {
			return pgcA(DomainVMGM, 0, j.PGCN)
		}
		return n.menuAction(s, DomainVMGM, 0, j.Menu)
	}
	vts := s.vts
	if s.domain != DomainVTS && s.domain != DomainVTSM {
		vts = n.titleVTS(s)
	}
	if j.VTS != 0 {
		vts = j.VTS
		if j.TTN != 0 {
			s.regs.SPRM[SprmVTSTTN] = uint16(j.TTN)
			if g := n.globalTitle(vts, j.TTN); g != 0 {
				s.regs.SPRM[SprmTTN] = uint16(g)
			}
		}
	}
	return n.menuAction(s, DomainVTSM, vts, j.Menu)
}

func (n *Nav) saveResume(s *navState, rsmCell int) {
	r := &resumePoint{vts: s.vts, pgcn: s.pgcn, cellN: rsmCell, atPoint: rsmCell == 0}
	if r.cellN == 0 {
		r.cellN = s.cellN
	}
	if r.cellN == 0 {
		r.cellN = 1
	}
	for i, idx := range resumeSPRMs {
		r.sprm[i] = s.regs.SPRM[idx]
	}
	s.resume = r
}

func (n *Nav) doJump(s *navState, j *Jump) action {
	switch j.Op {
	case JumpTT:
		return n.playTitle(s, j.TTN, 1)
	case JumpVTSTT, JumpVTSPTT:
		vts := s.vts
		if vts == 0 {
			vts = n.titleVTS(s)
		}
		pttn := 1
		if j.Op == JumpVTSPTT {
			pttn = j.PTTN
		}
		return n.playVTSTitle(s, vts, j.TTN, pttn, 0)
	case JumpSS:
		return n.enterSS(s, j)
	case CallSS:
		if s.domain == DomainVTS {
			n.saveResume(s, j.RsmCell)
		}
		return n.enterSS(s, j)
	}
	return stopA("unknown jump")
}

func (n *Nav) doResume(s *navState) action {
	r := s.resume
	if r == nil {
		return action{t: actNoop, reason: "there is nothing to resume"}
	}
	if n.pgcOf(s, DomainVTS, r.vts, r.pgcn) == nil {
		return stopA("the resume point no longer exists")
	}
	s.resume = nil
	for i, idx := range resumeSPRMs {
		s.regs.SPRM[idx] = r.sprm[i]
	}
	s.domain, s.vts, s.pgcn = DomainVTS, r.vts, r.pgcn
	return action{t: actCell, n: r.cellN, resumed: r.atPoint}
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Start runs first-play and returns what to show.
func (n *Nav) Start() *Step {
	n.st = n.fresh()
	if n.Disc.VMG.FirstPlay == nil {
		return n.settle(n.st, n.playTitle(n.st, 1, 1), false)
	}
	return n.settle(n.st, pgcA(DomainFP, 0, 0), false)
}

// CellEnded tells the navigator the shown cell (and still) finished.
func (n *Nav) CellEnded() *Step { return n.cellEnded(n.st, false) }

// PressButton activates button n with its 8-byte command (from the PCI).
func (n *Nav) PressButton(btn int, cmd *Command) *Step {
	s := n.st
	if s.current == nil || s.current.Kind == StepStop {
		return n.keep(s, "no menu is showing")
	}
	if btn >= 1 && btn <= 36 {
		s.regs.SPRM[SprmHLBTNN] = uint16(btn) << 10
	}
	if cmd == nil {
		return n.keep(s, fmt.Sprintf("button %d has no command", btn))
	}
	r := s.regs.Run([]Command{*cmd}, RunOptions{Once: true})
	if r.Kind == ResultStop {
		return n.keep(s, fmt.Sprintf("button %d: %s", btn, r.Reason))
	}
	if r.Kind == ResultEnd || r.Kind == ResultBreak {
		return n.keep(s, "")
	}
	s.inButton = true
	defer func() { s.inButton = false }()
	return n.settle(s, action{t: actVM, r: r}, false)
}

// SyncTitleCell tells the navigator which cell of the current title the
// viewer has reached. Clients play a whole title as one stream, so without
// this the navigator would still think it is on the first cell when the
// viewer calls a menu (and Resume would restart from there).
func (n *Nav) SyncTitleCell(pgcn, cellN int) bool {
	s := n.st
	if s.domain != DomainVTS || s.current == nil || s.current.Kind != StepTitle {
		return false
	}
	p := n.pgcOf(s, DomainVTS, s.vts, pgcn)
	if p == nil || cellN < 1 || cellN > len(p.Cells) {
		return false
	}
	if pgcn != s.pgcn {
		s.pgcn = pgcn
		s.regs.SPRM[SprmTTPGCN] = uint16(pgcn)
	}
	s.cellN = cellN
	pgn := programOf(p, cellN)
	for i, x := range n.pttList(s) {
		if x.PGCN == pgcn && x.PGN <= pgn {
			s.regs.SPRM[SprmPTTN] = uint16(i + 1)
		}
	}
	return true
}

// SelectButton highlights button n without activating it.
func (n *Nav) SelectButton(btn int) *Step {
	if btn >= 1 && btn <= 36 {
		n.st.regs.SPRM[SprmHLBTNN] = uint16(btn) << 10
	}
	return n.keep(n.st, "")
}

var menuUOP = map[string]uint{"title": 10, "root": 11, "subpicture": 12, "audio": 13, "angle": 14, "chapter": 15}

// MenuKey handles the remote's TITLE / MENU (root) / etc. keys.
func (n *Nav) MenuKey(kind string) *Step {
	s := n.st
	typ, ok := MenuTypes[kind]
	if !ok {
		return n.keep(s, "unknown menu "+kind)
	}
	if s.current != nil {
		if p := n.curPGC(s); p != nil && (p.UOPs>>menuUOP[kind])&1 == 1 {
			return n.keep(s, fmt.Sprintf("the disc does not allow the %s menu here", kind))
		}
	}
	j := &Jump{Op: CallSS, Domain: DomainVTSM, Menu: typ}
	if kind == "title" {
		j.Domain = DomainVMGM
	}
	if s.domain == DomainVTS {
		n.saveResume(s, 0)
	}
	a := n.enterSS(s, j)
	if a.t == actStop && kind != "title" {
		// No VTS root menu: fall back to the title menu like most players.
		a = n.enterSS(s, &Jump{Op: CallSS, Domain: DomainVMGM, Menu: MenuTypes["title"]})
	}
	return n.settle(s, a, false)
}

// GoUp is the remote's RETURN key: follow the PGC's go-up link, or resume
// the title when there is none.
func (n *Nav) GoUp() *Step {
	s := n.st
	if s.current != nil && s.current.Kind != StepStop {
		if p := n.curPGC(s); p != nil && p.GoUpPGCN != 0 && s.domain != DomainFP {
			return n.settle(s, pgcA(s.domain, s.vts, p.GoUpPGCN), false)
		}
	}
	if s.resume == nil {
		return n.keep(s, "")
	}
	return n.Resume()
}

// OpenMenu starts the disc directly in one of its menus ("title" for the
// disc's title menu, or root/chapter/audio/subpicture/angle of title set
// vts). Discs that insist on running first-play before their menus fall
// back to a normal start.
func (n *Nav) OpenMenu(kind string, vts int) *Step {
	n.st = n.fresh()
	typ, ok := MenuTypes[kind]
	if !ok {
		return n.Start()
	}
	j := &Jump{Op: JumpSS, Domain: DomainVTSM, Menu: typ, VTS: vts}
	if kind == "title" || vts == 0 {
		j = &Jump{Op: JumpSS, Domain: DomainVMGM, Menu: typ}
	} else {
		// The title set's menus belong to one of its titles: use the first.
		for _, t := range n.Disc.VMG.Titles {
			if t.VTS == vts {
				j.TTN = t.VTSTitle
				break
			}
		}
	}
	st := n.settle(n.st, n.enterSS(n.st, j), false)
	if st.Kind == StepStop {
		return n.Start()
	}
	return st
}

// Resume returns to the title that called the menu.
func (n *Nav) Resume() *Step { return n.settle(n.st, action{t: actResume}, false) }

// PlayTitle jumps straight to a title (and optional chapter).
func (n *Nav) PlayTitle(ttn, pttn int) *Step {
	if pttn < 1 {
		pttn = 1
	}
	snap := n.st.clone()
	st := n.settle(n.st, n.playTitle(n.st, ttn, pttn), false)
	if st.Kind == StepTitle && st.Title != nil && st.Title.TTN == ttn {
		return st
	}
	// Many discs guard their feature: its pre-commands bounce to a menu
	// unless the disc's own "Play movie" button set a flag first. The viewer
	// asked for this title explicitly, so enter it without the guard.
	n.st = snap
	a := n.playTitle(n.st, ttn, pttn)
	if a.t != actPGC {
		return n.settle(n.st, a, false)
	}
	a.noPre = true
	return n.settle(n.st, a, false)
}

// NextChapter / PrevChapter move within the current title.
func (n *Nav) NextChapter() *Step {
	s := n.st
	if s.domain != DomainVTS {
		return n.keep(s, "not in a title")
	}
	list := n.pttList(s)
	cur := int(s.regs.SPRM[SprmPTTN])
	if cur >= len(list) {
		return n.settle(s, action{t: actPost}, false)
	}
	return n.settle(s, n.doLink(s, &Link{Op: LinkPTTN, N: cur + 1}), false)
}

func (n *Nav) PrevChapter() *Step {
	s := n.st
	if s.domain != DomainVTS {
		return n.keep(s, "not in a title")
	}
	cur := int(s.regs.SPRM[SprmPTTN])
	if cur <= 1 {
		return n.settle(s, n.doLink(s, &Link{Op: LinkPTTN, N: 1}), false)
	}
	return n.settle(s, n.doLink(s, &Link{Op: LinkPTTN, N: cur - 1}), false)
}

// NavTimerFired handles SPRM9 expiry: jump to the PGC named in SPRM10.
func (n *Nav) NavTimerFired() *Step {
	s := n.st
	pgcn := int(s.regs.SPRM[SprmNavPGCN])
	if pgcn == 0 {
		return n.keep(s, "")
	}
	d := s.domain
	if d == DomainFP {
		d = DomainVMGM
	}
	return n.settle(s, pgcA(d, s.vts, pgcn), false)
}

// SetAudio / SetSubpicture / SetAngle change the viewer's streams.
func (n *Nav) SetAudio(stream int) { n.st.regs.SPRM[SprmAudio] = uint16(stream) }
func (n *Nav) SetSubpicture(stream int, on bool) {
	v := uint16(stream & 31)
	if on {
		v |= 0x40
	} else {
		v = 62
	}
	n.st.regs.SPRM[SprmSubp] = v
}
func (n *Nav) SetAngle(a int) { n.st.regs.SPRM[SprmAngle] = uint16(a) }

// Tick advances timers; returns a new step if the nav timer fired.
func (n *Nav) Tick(seconds float64) *Step {
	// The navigation timer only runs during title playback; in menus discs
	// use it as scratch state, so firing there would jump unexpectedly.
	if n.st.regs.Tick(seconds) && n.st.domain == DomainVTS && n.st.current != nil && n.st.current.Kind != StepStop {
		return n.NavTimerFired()
	}
	return nil
}
