package wittydvd

import (
	"strings"
	"testing"
)

// mkPGC builds a PGC of n ten-sector, five-second cells; programs lists the
// first cell of each program (nil: one program per cell).
func mkPGC(n int, programs []int) *PGC {
	p := &PGC{Programs: programs}
	for i := 0; i < n; i++ {
		p.Cells = append(p.Cells, Cell{FirstSector: uint32(i * 10), LastSector: uint32(i*10 + 9), Duration: 5})
		if programs == nil {
			p.Programs = append(p.Programs, i+1)
		}
	}
	for i := range p.AudioControl {
		p.AudioControl[i] = -1
	}
	return p
}

// mkDisc wraps title PGCs in a one-title disc whose chapter table points at
// the first PGC.
func mkDisc(pgcs ...PGCEntry) *Disc {
	for i := range pgcs {
		if i == 0 {
			pgcs[i].Entry, pgcs[i].TitleNumber = true, 1
		}
	}
	v := &VTS{Number: 1, PGCs: pgcs, PTT: [][]PTT{{{PGCN: 1, PGN: 1}}}}
	return &Disc{VMG: &VMG{Titles: []TitleSearch{{Number: 1, VTS: 1, VTSTitle: 1, Chapters: 1}}}, VTS: map[int]*VTS{1: v}}
}

// seq returns a Rand that yields the given values in turn.
func seq(vals ...float64) func() float64 {
	i := 0
	return func() float64 {
		v := vals[i%len(vals)]
		i++
		return v
	}
}

// playCells starts the disc and lets every cell end, returning the cells
// shown until the navigator stops (or a limit is reached).
func playCells(t *testing.T, n *Nav) ([]int, *Step) {
	t.Helper()
	var cells []int
	st := n.Start()
	for i := 0; i < 50 && st.Kind != StepStop; i++ {
		cells = append(cells, st.CellN)
		st = n.CellEnded()
	}
	return cells, st
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRandomPGC(t *testing.T) {
	// Random, two programs out of four (repeats allowed): draws 0.6 -> 3,
	// 0.6 -> 3 again, then the post command runs.
	p := mkPGC(4, nil)
	p.PlaybackMode = 0x02
	p.Post = []Command{cmd(0x71, 0, 0, 5, 0, 1)} // g5 = 1
	n := NewNav(mkDisc(PGCEntry{PGC: p}), NavOptions{RegisterOptions{Rand: seq(0.6)}})
	cells, st := playCells(t, n)
	if !equal(cells, []int{3, 3}) || n.Registers().GPRM[5] != 1 || !strings.Contains(st.Reason, "ended") {
		t.Fatalf("random: cells %v g5=%d stop=%q", cells, n.Registers().GPRM[5], st.Reason)
	}

	// Shuffle, all four programs, each once even when the draws repeat; a
	// program of two cells plays both.
	p = mkPGC(5, []int{1, 2, 4, 5})
	p.PlaybackMode = 0x84
	n = NewNav(mkDisc(PGCEntry{PGC: p}), NavOptions{RegisterOptions{Rand: seq(0.3)}})
	cells, _ = playCells(t, n)
	if !equal(cells, []int{2, 3, 1, 4, 5}) {
		t.Fatalf("shuffle: cells %v", cells)
	}
}

func TestRandomPGCStill(t *testing.T) {
	// The PGC still follows whichever program ends the random run, even when
	// that is not the PGC's last cell.
	p := mkPGC(3, nil)
	p.PlaybackMode, p.StillTime = 0x01, 4
	n := NewNav(mkDisc(PGCEntry{PGC: p}), NavOptions{RegisterOptions{Rand: seq(0.0)}})
	st := n.Start()
	if st.CellN != 1 || st.StillSec != 0 {
		t.Fatalf("first: %+v", st)
	}
	st = n.CellEnded()
	if !st.Hold || st.StillSec != 4 {
		t.Fatalf("PGC still after the random program: %+v", st)
	}
}

func TestParentalBlock(t *testing.T) {
	// PGCs 1 and 2 are two versions of the same scene; PTL_MAIT gives title
	// set 1 the parental ID 0x0002 at level 8 and 0x0001 at level 1.
	theatrical, unrated := mkPGC(1, nil), mkPGC(2, nil)
	d := mkDisc(
		PGCEntry{PGC: theatrical, BlockType: 1, BlockMode: 1, ParentalMask: 0x0001},
		PGCEntry{PGC: unrated, BlockType: 1, BlockMode: 3, ParentalMask: 0x0002, Entry: true, TitleNumber: 1},
	)
	var c ParentalCountry
	c.Country = "US"
	for l := range c.Masks {
		c.Masks[l] = []uint16{0xffff, 0x0001}
	}
	c.Masks[7] = []uint16{0xffff, 0x0002}
	d.VMG.Parental = []ParentalCountry{c}

	// No parental control (SPRM13 = 15) acts at level 8.
	if st := NewNav(d, NavOptions{}).Start(); st.PGCN != 2 {
		t.Fatalf("level 8: pgc %d", st.PGCN)
	}
	n := NewNav(d, NavOptions{})
	n.fresh()
	n.st.regs.SPRM[SprmParental] = 1
	st := n.settle(n.st, n.playTitle(n.st, 1, 1), false)
	if st.PGCN != 1 {
		t.Fatalf("level 1: pgc %d", st.PGCN)
	}
	// Without a table the PGC plays as named.
	d.VMG.Parental = nil
	if st := NewNav(d, NavOptions{}).Start(); st.PGCN != 1 {
		t.Fatalf("no table: pgc %d", st.PGCN)
	}
}

func TestMenuLanguageFallback(t *testing.T) {
	// The English unit has no root menu; the French one does. Pressing MENU
	// in English uses the French root menu, and its links stay in French.
	enChapter := mkPGC(1, nil)
	frRoot, frSub := mkPGC(1, nil), mkPGC(1, nil)
	frRoot.Cells[0].StillTime = 0xff
	frSub.Cells[0].StillTime = 0xff
	frSub.Cells[0].FirstSector, frSub.Cells[0].LastSector = 100, 109
	d := mkDisc(PGCEntry{PGC: mkPGC(1, nil)})
	d.VTS[1].Menus = []LangUnit{
		{Language: "en", PGCs: []PGCEntry{{Entry: true, MenuType: 7, PGC: enChapter}}},
		{Language: "fr", PGCs: []PGCEntry{{Entry: true, MenuType: 3, PGC: frRoot}, {PGC: frSub}}},
	}
	n := NewNav(d, NavOptions{RegisterOptions{MenuLanguage: "en"}})
	n.Start()
	st := n.MenuKey("root")
	if st.Kind != StepMenu || n.StepPGC(st) != frRoot {
		t.Fatalf("root menu: %+v", st)
	}
	st = n.PressButton(1, &[]Command{cmd(0x20, 0x04, 0, 0, 0, 0, 0, 2)}[0]) // LinkPGCN 2
	if n.StepPGC(st) != frSub || st.FirstSector != 100 {
		t.Fatalf("link inside the French unit: %+v", st)
	}
	// The chapter menu exists in English, which wins again.
	n.Resume()
	if st := n.MenuKey("chapter"); n.StepPGC(st) != enChapter {
		t.Fatalf("chapter menu: %+v", st)
	}
}

func TestJunkCells(t *testing.T) {
	// Cell 1 points backwards, cell 2 starts past the end of the title VOB:
	// both are skipped (their cell commands still run).
	p := mkPGC(3, nil)
	p.Cells[0].FirstSector, p.Cells[0].LastSector = 50, 10
	p.Cells[1].FirstSector, p.Cells[1].LastSector = 900, 909
	p.Cells[1].Command = 1
	p.CellCmds = []Command{cmd(0x71, 0, 0, 4, 0, 7)} // g4 = 7
	d := mkDisc(PGCEntry{PGC: p})
	d.VTS[1].TitleVOBSectors = 500
	n := NewNav(d, NavOptions{})
	if st := n.Start(); st.CellN != 3 || n.Registers().GPRM[4] != 7 {
		t.Fatalf("junk cells: %+v g4=%d", st, n.Registers().GPRM[4])
	}

	// A PGC of nothing but junk that loops on itself stops instead of
	// spinning forever.
	q := mkPGC(1, nil)
	q.Cells[0].FirstSector, q.Cells[0].LastSector = 9, 1
	q.Cells[0].Command = 1
	q.CellCmds = []Command{cmd(0x20, 0x01, 0, 0, 0, 0, 0, 0x09)} // LinkTopPGC
	st := NewNav(mkDisc(PGCEntry{PGC: q}), NavOptions{}).Start()
	if st.Kind != StepStop {
		t.Fatalf("junk loop: %+v", st)
	}
}

func TestChapterTableNamesMissingPGC(t *testing.T) {
	d := mkDisc(PGCEntry{PGC: mkPGC(2, nil)})
	d.VTS[1].PTT = [][]PTT{{{PGCN: 7, PGN: 1}}}
	if st := NewNav(d, NavOptions{}).Start(); st.Kind != StepTitle || st.PGCN != 1 {
		t.Fatalf("fallback to the entry PGC: %+v", st)
	}
}

func TestNavTimer(t *testing.T) {
	// Pre-command SetNVTMR 10 s -> PGC 2. It fires in the title domain only.
	p1, p2 := mkPGC(1, nil), mkPGC(1, nil)
	p1.Cells[0].StillTime = 0xff
	p1.Pre = []Command{cmd(0x52, 0, 0, 10, 0, 2)}
	n := NewNav(mkDisc(PGCEntry{PGC: p1}, PGCEntry{PGC: p2}), NavOptions{})
	n.Start()
	if st := n.Tick(9); st != nil {
		t.Fatalf("fired early: %+v", st)
	}
	if st := n.Tick(1); st == nil || st.PGCN != 2 {
		t.Fatalf("timer: %+v", st)
	}
}

func TestCallSSResumeCell(t *testing.T) {
	// A cell command calls the root menu with resume cell 3; RSM restarts
	// cell 3 from its beginning (not mid-cell).
	p := mkPGC(3, nil)
	p.Cells[0].Command = 1
	p.CellCmds = []Command{cmd(0x30, 0x08, 0, 0, 3, 0x83, 0, 0)} // CallSS VTSM root, resume cell 3
	root := mkPGC(1, nil)
	root.Cells[0].StillTime = 0xff
	d := mkDisc(PGCEntry{PGC: p})
	d.VTS[1].Menus = []LangUnit{{Language: "en", PGCs: []PGCEntry{{Entry: true, MenuType: 3, PGC: root}}}}
	n := NewNav(d, NavOptions{})
	n.Start()
	if st := n.CellEnded(); st.Kind != StepMenu {
		t.Fatalf("CallSS: %+v", st)
	}
	st := n.PressButton(1, &[]Command{cmd(0x20, 0x01, 0, 0, 0, 0, 0, 0x10)}[0]) // RSM
	if st.Kind != StepTitle || st.CellN != 3 || st.Resumed {
		t.Fatalf("RSM: %+v", st)
	}
	// RSM with nothing to resume leaves the menu as it is.
	n.MenuKey("root")
	n.st.resume = nil
	if st := n.PressButton(1, &[]Command{cmd(0x20, 0x01, 0, 0, 0, 0, 0, 0x10)}[0]); st.Kind != StepMenu {
		t.Fatalf("RSM without resume info: %+v", st)
	}
}

func TestLinkNextPGAtLastProgram(t *testing.T) {
	p := mkPGC(2, nil)
	p.Cells[1].Command = 1
	p.CellCmds = []Command{cmd(0x20, 0x01, 0, 0, 0, 0, 0, 0x06)} // LinkNextPG
	p.Post = []Command{cmd(0x71, 0, 0, 6, 0, 1)}                 // g6 = 1
	n := NewNav(mkDisc(PGCEntry{PGC: p}), NavOptions{})
	n.Start()
	n.CellEnded()
	if st := n.CellEnded(); st.Kind != StepStop || n.Registers().GPRM[6] != 1 {
		t.Fatalf("LinkNextPG past the last program runs the post commands: %+v", st)
	}
}

func TestRegionRegister(t *testing.T) {
	// A disc for regions 2 and 4 only (bits set = prohibited).
	d := mkDisc(PGCEntry{PGC: mkPGC(1, nil)})
	d.VMG.RegionMask = 0xff &^ (1<<1 | 1<<3)
	if r := NewNav(d, NavOptions{}).Registers().SPRM[SprmRegion]; r != 1<<1 {
		t.Fatalf("SPRM20 = %#x, want region 2", r)
	}
	if r := NewNav(d, NavOptions{RegisterOptions{Region: 4}}).Registers().SPRM[SprmRegion]; r != 1<<3 {
		t.Fatalf("SPRM20 = %#x, want region 4", r)
	}
}
