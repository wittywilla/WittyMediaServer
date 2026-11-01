package wittybluray

import (
	"strings"
	"testing"
)

// menuDisc is a disc folder whose one clip holds a menu stream saying it is w by h.
func menuDisc(t *testing.T, w, h int) *Disc {
	t.Helper()
	const pid = 0x1400
	pkt := make([]byte, packetSize)
	ts := pkt[4:]
	ts[0], ts[1], ts[2], ts[3] = 0x47, 0x40|byte(pid>>8), byte(pid&0xff), 0x10
	pes := append([]byte{0, 0, 1, 0xbd, 0, 0, 0x80, 0, 0}, segICS, 0, 9)
	pes = append(pes, byte(w>>8), byte(w), byte(h>>8), byte(h), 0, 0, 0, 0, 0xc0)
	copy(ts[4:], pes)

	d, err := Open(writeDisc(t, map[string][]byte{
		"BDMV/index.bdmv":          buildIndex("0200", hdmv(0), hdmv(1), hdmv(2)),
		"BDMV/MovieObject.bdmv":    buildMobj(MovieObject{}, MovieObject{}, MovieObject{}),
		"BDMV/PLAYLIST/00001.mpls": buildMpls(1, 0, oneItem),
		"BDMV/CLIPINF/00001.clpi":  buildClip(1, STCSequence{ID: 0, Start: 45000, End: 45000 * 100}),
		"BDMV/STREAM/00001.m2ts":   pkt,
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// Drawing a page or a transition makes a picture this size: 65535 by 65535 is 17 GB,
// each time, for a few bytes on a disc.
func TestCraftedMenuSizeIsRefused(t *testing.T) {
	if _, err := menuDisc(t, 65535, 65535).LoadMenu("00001", 0x1400, 0); err == nil || !strings.Contains(err.Error(), "can't be drawn") {
		t.Errorf("%v", err)
	}
	m, err := menuDisc(t, 3840, 2160).LoadMenu("00001", 0x1400, 0)
	if err != nil && strings.Contains(err.Error(), "can't be drawn") {
		t.Errorf("the largest frame a Blu-ray has: %v", err)
	}
	if err == nil && (m.Width != 3840 || m.Height != 2160) {
		t.Errorf("%+v", m)
	}
}

func TestCraftedPictureSizesAreRefused(t *testing.T) {
	if _, err := decodeRLE(nil, 65535, 65535); err == nil {
		t.Error("a picture bigger than any frame was made room for")
	}
	if _, err := decodeRLE(nil, 1920, 1080); err == nil || strings.Contains(err.Error(), "can't be drawn") {
		t.Logf("a real size: %v", err)
	}
	pics := map[int]*Object{1: {ID: 1, Width: 2, Height: 2, Pixels: []byte{1, 1, 1, 1}}}
	var pal Palette
	// A crop as big as the format can say, with no window or frame to cut it.
	if sub := compose([]compObject{{object: 1, cw: 65535, ch: 65535}}, nil, pal, pics, 0, 0); sub != nil {
		t.Error("it made a picture")
	}
	if sub := compose([]compObject{{object: 1}}, nil, pal, pics, 0, 0); sub == nil {
		t.Error("an ordinary picture was refused")
	}
}
