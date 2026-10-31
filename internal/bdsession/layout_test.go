package bdsession

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittybluray"
)

// seamSession is a session over a disc folder with two clips of 100 000 source
// packets, entry points every 45 000 ticks (one second) from 45 000.
func seamSession(t *testing.T) *Session {
	t.Helper()
	dir := t.TempDir()
	index := make([]byte, 78)
	copy(index, "INDX0200")
	binary.BigEndian.PutUint32(index[8:], 78)
	entry := make([]byte, 12)
	entry[0], entry[4] = 2<<6, 3<<6
	copy(entry[6:], "00000")
	index = append(index, 0, 0, 0, 0)
	index = append(index, entry...)
	index = append(index, entry...)
	index = append(index, 0, 0)
	for name, data := range map[string][]byte{
		"BDMV/index.bdmv":          index,
		"BDMV/MovieObject.bdmv":    append([]byte("MOBJ0200"), make([]byte, 42)...),
		"BDMV/PLAYLIST/00001.mpls": testPlaylist(),
		"BDMV/CLIPINF/00001.clpi":  testClip(),
		"BDMV/STREAM/00001.m2ts":   nil,
		"BDMV/STREAM/00002.m2ts":   nil,
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if data == nil {
			if err := os.Truncate(p, 100000*192); err != nil {
				t.Fatal(err)
			}
		}
	}
	d, err := wittybluray.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	d.Clips = map[string]*wittybluray.Clip{}
	for _, name := range []string{"00001", "00002"} {
		c := &wittybluray.Clip{Name: name, Packets: 100000, STC: []wittybluray.STCSequence{{ID: 0, Start: 45000, End: 45000 * 101}}}
		m := wittybluray.EPMap{}
		for k := uint32(0); k < 100; k++ {
			m.Entries = append(m.Entries, wittybluray.EPEntry{PTS: 45000 * (k + 1), SPN: 1000 * k})
		}
		c.EPMaps = []wittybluray.EPMap{m}
		d.Clips[name] = c
	}
	return &Session{disc: &engineDisc{DiscEngine: d, builtin: d}, nav: wittybluray.NewNav(d, wittybluray.NavOptions{})}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.001 }

func video() []wittybluray.Stream { return []wittybluray.Stream{{PID: 0x1011}} }

// An item that ends part-way through its clip must read past the entry before its
// end: stopping there is what used to lose the pictures before the join. It reads
// on far enough for the sound stored after them, and the rest is taken out again.
func TestLayoutReadsPastTheEndOfAnItem(t *testing.T) {
	s := seamSession(t)
	out := uint32(45000*11 + 20000) // 10.44 s, ending between entries
	pl := &wittybluray.Playlist{Number: 1, Items: []wittybluray.PlayItem{
		{Clip: "00001", In: 45000, Out: out, Video: video()},
		{Clip: "00002", In: 45000, Out: 45000 * 101, Video: video()},
	}}
	laid := s.layout(pl, -1)
	first := laid.items[0]
	if first.spn[1] != 13000 { // the entry after 2 s past the end: at 14 s, the 14th
		t.Errorf("reads to packet %d", first.spn[1])
	}
	if first.stop != out {
		t.Errorf("stops at %d", first.stop)
	}
	if first.joins {
		t.Error("another clip doesn't carry on from this one")
	}
	if !near(laid.items[1].start, 10+20000.0/45000) {
		t.Errorf("the next item starts at %v", laid.items[1].start)
	}
	segs, holes, err := s.segments(laid)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 2 || !near(segs[0].Outpoint, float64(out+readOn)/45000) || segs[1].Outpoint != 0 {
		t.Fatalf("%+v", segs)
	}
	// The input has room for what's read past the end, and the stream doesn't.
	if !near(segs[0].Duration, first.duration+tailHole) {
		t.Errorf("the input lasts %v", segs[0].Duration)
	}
	if len(holes) != 1 || !near(holes[0].At, first.duration-holeGuard) || holes[0].Length != tailHole {
		t.Errorf("%+v", holes)
	}
}

// Without an entry that far on, it reads to the end of its clip.
func TestLayoutReadsToTheEndOfTheClip(t *testing.T) {
	s := seamSession(t)
	pl := &wittybluray.Playlist{Number: 1, Items: []wittybluray.PlayItem{{Clip: "00001", In: 45000, Out: 45000 * 100, Video: video()}}}
	first := s.layout(pl, -1).items[0]
	if first.spn[1] != 100000 || first.stop != 45000*100 {
		t.Errorf("%+v", first)
	}
}

// An item ending at its clip's end has nothing to stop or take out. This is nearly
// every item on nearly every disc.
func TestLayoutLeavesAClipEndAlone(t *testing.T) {
	s := seamSession(t)
	pl := &wittybluray.Playlist{Number: 1, Items: []wittybluray.PlayItem{
		{Clip: "00001", In: 45000, Out: 45000 * 101, Video: video()},
		{Clip: "00002", In: 45000, Out: 45000 * 101, Video: video()},
	}}
	laid := s.layout(pl, -1)
	segs, holes, err := s.segments(laid)
	if err != nil {
		t.Fatal(err)
	}
	if len(holes) != 0 || len(segs) != 2 || segs[0].Outpoint != 0 || !near(segs[0].Duration, 100) || segs[0].Extents[0].Length != 100000*192 {
		t.Errorf("%+v %+v", segs, holes)
	}
}

// The next item carries on in the same clip: one read, so nothing is cut, and
// each item's time and chapter marks stay where the disc has them.
func TestLayoutJoinsItemsOfOneClip(t *testing.T) {
	s := seamSession(t)
	mid := uint32(45000*11 + 20000)
	pl := &wittybluray.Playlist{Number: 1, Items: []wittybluray.PlayItem{
		{Clip: "00001", In: 45000, Out: mid, Video: video()},
		{Clip: "00001", In: mid, Out: 45000 * 21, Video: video()},
	}, Marks: []wittybluray.Mark{{Type: 1, Item: 0, Time: 45000}, {Type: 1, Item: 1, Time: mid}, {Type: 1, Item: 1, Time: 45000 * 15}}}
	laid := s.layout(pl, -1)
	if !laid.items[0].joins || laid.items[0].stop != 0 {
		t.Fatalf("%+v", laid.items[0])
	}
	want := []float64{0, 10 + 20000.0/45000, 14}
	for i, c := range laid.chapters {
		if !near(c, want[i]) {
			t.Errorf("chapter %d at %v, want %v", i+1, c, want[i])
		}
	}
	if !near(laid.duration, 20) {
		t.Errorf("duration %v", laid.duration)
	}
	segs, holes, err := s.segments(laid)
	if err != nil {
		t.Fatal(err)
	}
	// One read, with the end of the last item the only thing to cut.
	if len(segs) != 1 || !near(segs[0].Duration, 20+tailHole) || !near(segs[0].Outpoint, float64(45000*21+readOn)/45000) {
		t.Fatalf("%+v", segs)
	}
	if len(holes) != 1 || !near(holes[0].At, 20-holeGuard) {
		t.Errorf("%+v", holes)
	}
}

// An entry's time in the table is rounded down: the first picture is the item's
// IN time when that lies within the rounding, or the marks are off by it. Further
// from the entry, the pictures before IN are read and taken out again.
func TestLayoutStartsOnTheFirstPicture(t *testing.T) {
	s := seamSession(t)
	pl := &wittybluray.Playlist{Number: 1, Items: []wittybluray.PlayItem{
		{Clip: "00001", In: 45000 + 192, Out: 45000 * 101, Video: video()},
		{Clip: "00002", In: 45000*3 + 20000, Out: 45000 * 101, Video: video()},
	}}
	laid := s.layout(pl, -1)
	if laid.items[0].startPTS != 45000+192 || laid.items[0].preroll != 0 {
		t.Errorf("%+v", laid.items[0])
	}
	second := laid.items[1]
	if second.startPTS != 45000*3+20000 || !near(second.preroll, 20000.0/45000) || !near(second.duration, 98-20000.0/45000) {
		t.Errorf("%+v", second)
	}
	if second.spn[0] != 2000 {
		t.Errorf("reads from packet %d", second.spn[0])
	}
	segs, holes, err := s.segments(laid)
	if err != nil {
		t.Fatal(err)
	}
	if !near(segs[1].Duration, 98) {
		t.Errorf("the input lasts %v", segs[1].Duration)
	}
	if len(holes) != 1 || !near(holes[0].At, second.start-holeGuard) || !near(holes[0].Length, 20000.0/45000) {
		t.Errorf("%+v", holes)
	}
}

// Stills and items with no video keep to the entry points, with no holes.
func TestLayoutKeepsStillsToEntries(t *testing.T) {
	s := seamSession(t)
	pl := &wittybluray.Playlist{Number: 1, Items: []wittybluray.PlayItem{
		{Clip: "00001", In: 45000*3 + 20000, Out: 45000*11 + 20000, StillMode: 1, Video: video()},
		{Clip: "00002", In: 45000*3 + 20000, Out: 45000*11 + 20000},
	}}
	laid := s.layout(pl, -1)
	for _, pi := range laid.items {
		if pi.stop != 0 || pi.preroll != 0 || pi.startPTS != 45000*3 || pi.spn != [2]uint32{2000, 10000} {
			t.Errorf("%+v", pi)
		}
	}
	if _, holes, err := s.segments(laid); err != nil || len(holes) != 0 {
		t.Errorf("%+v %v", holes, err)
	}
}
