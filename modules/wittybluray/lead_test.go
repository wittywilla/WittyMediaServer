package wittybluray

import (
	"math"
	"testing"
)

// tsPES is one source packet starting a PES packet with a presentation time.
func tsPES(pid uint16, pts uint64) []byte {
	p := make([]byte, packetSize)
	p[4], p[5], p[6], p[7] = 0x47, 0x40|byte(pid>>8), byte(pid), 0x10
	h := p[8:]
	h[2], h[3] = 1, 0xe0
	h[7], h[8] = 0x80, 5
	h[9] = 0x21 | byte(pts>>30&7)<<1
	h[10], h[11] = byte(pts>>22), 1|byte(pts>>15)<<1
	h[12], h[13] = byte(pts>>7), 1|byte(pts)<<1
	return p
}

func TestScanLead(t *testing.T) {
	const video = 0x1011
	cases := []struct {
		name string
		pkts [][]byte
		want float64
	}{
		{"sound stored ahead of the picture", [][]byte{tsPES(video, 900000), tsPES(0x1100, 900000-22500)}, 0.25},
		{"the earliest of several", [][]byte{tsPES(0x1100, 900000-9000), tsPES(video, 900000), tsPES(0x1101, 900000-18000)}, 0.2},
		{"only the first packet of each stream counts", [][]byte{tsPES(0x1100, 900000-9000), tsPES(0x1100, 1), tsPES(video, 900000)}, 0.1},
		{"sound after the picture", [][]byte{tsPES(video, 900000), tsPES(0x1100, 900000+1000)}, 0},
		{"subtitles just ahead", [][]byte{tsPES(video, 900000), tsPES(0x1100, 900000), tsPES(0x1200, 900000-45000)}, 0.5},
		{"subtitles long before", [][]byte{tsPES(video, 900000), tsPES(0x1100, 900000), tsPES(0x1200, 900000-180000)}, 0},
		{"no picture in sight", [][]byte{tsPES(0x1100, 900000)}, 0},
	}
	for _, c := range cases {
		var b []byte
		for _, p := range c.pkts {
			b = append(b, p...)
		}
		if got := scanLead(b, video); math.Abs(got-c.want) > 1e-6 {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// What FFmpeg is told to read decides where its clock starts: other streams don't count.
func TestScanLeadOfSomeStreams(t *testing.T) {
	const video = 0x1011
	b := append(append(tsPES(video, 900000), tsPES(0x1100, 900000-9000)...), tsPES(0x1101, 900000-18000)...)
	if got := scanLead(b, video, video, 0x1100); math.Abs(got-0.1) > 1e-6 {
		t.Errorf("%v, want 0.1", got)
	}
	if got := scanLead(b, video, video); got != 0 {
		t.Errorf("the picture alone: %v", got)
	}
}

func TestEntryAfter(t *testing.T) {
	c := &Clip{EPMaps: []EPMap{{PID: 0x1011, Entries: []EPEntry{{PTS: 100, SPN: 10}, {PTS: 200, SPN: 20}, {PTS: 300, SPN: 30}}}}}
	if e, ok := c.EntryAfter(0x1011, -1, 150); !ok || e.SPN != 20 {
		t.Errorf("%+v %v", e, ok)
	}
	if e, ok := c.EntryAfter(0x1011, -1, 200); !ok || e.SPN != 30 {
		t.Errorf("exactly on one: %+v %v", e, ok)
	}
	if _, ok := c.EntryAfter(0x1011, -1, 300); ok {
		t.Error("nothing comes after the last one")
	}
}
