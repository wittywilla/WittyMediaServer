package wittydvd

import (
	"math"
	"testing"
)

// A run of test cells, each with its place on the "disc" and its times.
type runCell struct {
	c          Cell
	start, end uint32
}

func buildRuns(t *testing.T, in []runCell) []TitleRun {
	t.Helper()
	var cells []TitleCell
	times := map[uint32]runCell{}
	for _, x := range in {
		cells = append(cells, TitleCell{Cell: x.c})
		times[x.c.FirstSector] = x
	}
	runs, err := titleRuns(cells, func(c Cell) ([]FileExtent, error) {
		return []FileExtent{{File: "disc", Offset: int64(c.FirstSector) * Sector, Length: int64(c.LastSector-c.FirstSector+1) * Sector}}, nil
	}, func(c Cell) (uint32, uint32) { return times[c.FirstSector].start, times[c.FirstSector].end }, func(Cell, uint32) float64 { return 0.25 })
	if err != nil {
		t.Fatal(err)
	}
	return runs
}

// Cells cut from one recording are read as one: there is nothing to put
// together at their chapters, and the whole of the disc between them is read.
func TestTitleRunsJoinCellsOfOneRecording(t *testing.T) {
	runs := buildRuns(t, []runCell{
		{Cell{VOBID: 1, CellID: 1, FirstSector: 0, LastSector: 99, Duration: 10}, 90000, 990000},
		{Cell{VOBID: 1, CellID: 2, FirstSector: 100, LastSector: 149, Duration: 5}, 990000, 1440000},
		{Cell{VOBID: 1, CellID: 3, FirstSector: 150, LastSector: 299, Duration: 20}, 1440000, 3240000},
	})
	if len(runs) != 1 || len(runs[0].Extents) != 1 || runs[0].Cells != 3 {
		t.Fatalf("%+v", runs)
	}
	if e := runs[0].Extents[0]; e.Offset != 0 || e.Length != 300*Sector {
		t.Errorf("reads %+v", e)
	}
	if runs[0].Duration != 35 || runs[0].StartPTS != 90000 {
		t.Errorf("duration %v, start %v", runs[0].Duration, runs[0].StartPTS)
	}
}

// What can't be read straight through stays apart, each part with its own first picture.
func TestTitleRunsKeepRealJoinsApart(t *testing.T) {
	cases := map[string][]runCell{
		"another VOB": {
			{Cell{VOBID: 1, FirstSector: 0, LastSector: 99, Duration: 10}, 90000, 990000},
			{Cell{VOBID: 2, FirstSector: 100, LastSector: 199, Duration: 10}, 25000, 925000},
		},
		"a new time base": {
			{Cell{VOBID: 1, FirstSector: 0, LastSector: 99, Duration: 10}, 90000, 990000},
			{Cell{VOBID: 1, FirstSector: 100, LastSector: 199, Duration: 10, STCDiscontinuity: true}, 990000, 1890000},
		},
		"a jump on the disc": {
			{Cell{VOBID: 1, FirstSector: 0, LastSector: 99, Duration: 10}, 90000, 990000},
			{Cell{VOBID: 1, FirstSector: 500, LastSector: 599, Duration: 10}, 990000, 1890000},
		},
		"times that don't carry on": {
			{Cell{VOBID: 1, FirstSector: 0, LastSector: 99, Duration: 10}, 90000, 990000},
			{Cell{VOBID: 1, FirstSector: 100, LastSector: 199, Duration: 10}, 3000000, 3900000},
		},
		"an interleaved cell": {
			{Cell{VOBID: 1, FirstSector: 0, LastSector: 99, Duration: 10}, 90000, 990000},
			{Cell{VOBID: 1, FirstSector: 100, LastSector: 199, Duration: 10, Interleaved: true}, 990000, 0},
		},
		"an angle block": {
			{Cell{VOBID: 1, FirstSector: 0, LastSector: 99, Duration: 10, BlockType: 1}, 90000, 0},
			{Cell{VOBID: 1, FirstSector: 100, LastSector: 199, Duration: 10, BlockType: 1}, 990000, 0},
		},
	}
	for name, in := range cases {
		runs := buildRuns(t, in)
		if len(runs) != 2 {
			t.Errorf("%s: %d runs", name, len(runs))
			continue
		}
		if runs[1].StartPTS != in[1].start || runs[1].FirstCell != 1 {
			t.Errorf("%s: second run %+v", name, runs[1])
		}
	}
}

// Timestamps win over a cell's stated length when they are close; a stated
// length that is close but rounded is what shifts every later picture otherwise.
func TestTitleRunsLengthComesFromTimestamps(t *testing.T) {
	r := buildRuns(t, []runCell{{Cell{VOBID: 1, FirstSector: 0, LastSector: 99, Duration: 100.1}, 0 + 90000, 90000 + 9000000}})
	if r[0].Duration != 100 {
		t.Errorf("close: %v", r[0].Duration)
	}
	// Ending in VOBUs with no picture says nothing about the length.
	r = buildRuns(t, []runCell{{Cell{VOBID: 1, FirstSector: 0, LastSector: 99, Duration: 100}, 90000, 90000 + 90000}})
	if r[0].Duration != 100 {
		t.Errorf("far off: %v", r[0].Duration)
	}
	// No NAV pack at all: the stated length, and no anchor.
	r = buildRuns(t, []runCell{{Cell{VOBID: 1, FirstSector: 0, LastSector: 99, Duration: 7}, 0, 0}})
	if r[0].Duration != 7 || r[0].StartPTS != 0 {
		t.Errorf("no times: %+v", r[0])
	}
}

// pesSector is one pack holding one PES packet with a presentation time.
func pesSector(id, sub byte, pts int64) []byte {
	b := make([]byte, Sector)
	copy(b, []byte{0, 0, 1, 0xba, 0x44})
	p := 14
	copy(b[p:], []byte{0, 0, 1, id})
	b[p+4], b[p+5] = 0, 20
	b[p+6], b[p+7], b[p+8] = 0x80, 0x80, 5
	b[p+9] = 0x21 | byte(pts>>30&7)<<1
	b[p+10], b[p+11] = byte(pts>>22), 1|byte(pts>>15)<<1
	b[p+12], b[p+13] = byte(pts>>7), 1|byte(pts)<<1
	b[p+14] = sub
	return b
}

func TestScanLead(t *testing.T) {
	const first = 900000
	video := func(pts int64) []byte { return pesSector(0xe0, 0, pts) }
	audio := func(sub byte, pts int64) []byte { return pesSector(0xbd, sub, pts) }
	cases := []struct {
		name string
		s    [][]byte
		want float64
	}{
		{"sound stored ahead of the picture", [][]byte{video(first), audio(0x80, first-21600)}, 0.24},
		{"the earliest of several sound streams", [][]byte{audio(0x80, first-9000), video(first), audio(0x81, first-18000)}, 0.2},
		{"each stream's first packet counts", [][]byte{audio(0x80, first-9000), audio(0x80, 5), video(first)}, 0.1},
		{"nothing ahead of the picture", [][]byte{video(first), audio(0x80, first)}, 0},
		{"a sub-picture just ahead", [][]byte{video(first), audio(0x80, first), audio(0x20, first-45000)}, 0.5},
		{"a sub-picture long before", [][]byte{video(first), audio(0x80, first), audio(0x20, first-180000)}, 0},
	}
	for _, c := range cases {
		var buf []byte
		for _, s := range c.s {
			buf = append(buf, s...)
		}
		if got := scanLead(buf, first); math.Abs(got-c.want) > 1e-6 {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
