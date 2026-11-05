package hls

import (
	"strings"
	"testing"
	"time"
)

func TestLadderPlaylistStartsWithTheFirstRung(t *testing.T) {
	s := &Session{Spec: Spec{Variant: &Variant{Subtitles: []Rendition{{Stream: 0, Name: "English", Language: "en"}}}}}
	got := string(s.LadderPlaylist([]LadderEntry{
		{Media: "../v-a-q480/playlist.m3u8?st=x", Variant: Variant{BandwidthKbps: 1400, Codecs: "avc1.640028,mp4a.40.2", Width: 854, Height: 480, Range: "SDR"}},
		{Media: "../v-a-q720/playlist.m3u8?st=x", Variant: Variant{BandwidthKbps: 2800, Codecs: "avc1.640028,mp4a.40.2", Width: 1280, Height: 720, Range: "SDR"}},
	}, func(i int) string { return "subs/0.m3u8?st=x" }))
	first, second := strings.Index(got, "q480"), strings.Index(got, "q720")
	if first < 0 || second < first {
		t.Fatalf("the start rung must come first:\n%s", got)
	}
	for _, want := range []string{
		`BANDWIDTH=1400000,AVERAGE-BANDWIDTH=1400000,CODECS="avc1.640028,mp4a.40.2",RESOLUTION=854x480,VIDEO-RANGE=SDR,SUBTITLES="subs"`,
		`BANDWIDTH=2800000,AVERAGE-BANDWIDTH=2800000,CODECS="avc1.640028,mp4a.40.2",RESOLUTION=1280x720,VIDEO-RANGE=SDR,SUBTITLES="subs"`,
		`URI="subs/0.m3u8?st=x"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "#EXT-X-MEDIA:"); n != 1 {
		t.Errorf("the rungs share one subtitle group, found %d renditions:\n%s", n, got)
	}
}

// A viewer who leaves leaves every rung, and a rung nobody watches any more rests
// while the player is on another.
func TestLadderGroup(t *testing.T) {
	m := testManager(t, 4)
	was := siblingRest
	siblingRest = 30 * time.Millisecond
	t.Cleanup(func() { siblingRest = was })
	hi := m.Ensure(Spec{ID: "v-a-r1-q720", Group: "v-a-r1", Duration: 60, SegmentSec: 2})
	lo := m.Ensure(Spec{ID: "v-a-r1-q480", Group: "v-a-r1", Duration: 60, SegmentSec: 2})
	other := m.Ensure(Spec{ID: "v-b-r1-q480", Group: "v-b-r1", Duration: 60, SegmentSec: 2})
	if lo.Sibling("v-a-r1-q720") != hi || lo.Sibling("v-b-r1-q480") != nil || other.Sibling("v-a-r1-q720") != nil {
		t.Fatal("Sibling should only find rungs of the same ladder")
	}
	running(hi, time.Now())
	running(other, time.Now())
	hi.used.Store(time.Now().Add(-time.Minute).UnixNano())
	lo.restSiblings() // the player moved down
	time.Sleep(200 * time.Millisecond)
	if hi.Active() {
		t.Error("the rung the player left is still converting")
	}
	if !other.Active() {
		t.Error("another ladder's rung was stopped")
	}
	// A rung someone still asks for keeps going.
	running(hi, time.Now())
	lo.restSiblings()
	hi.touch()
	time.Sleep(200 * time.Millisecond)
	if !hi.Active() {
		t.Error("a rung asked for since was stopped")
	}

	hi.See("tv")
	lo.See("tv")
	m.Leave("v-a-r1-q480", "tv")
	if m.Get("v-a-r1-q720") != nil || m.Get("v-a-r1-q480") != nil {
		t.Error("leaving a ladder should end all its rungs")
	}
	if m.Get("v-b-r1-q480") == nil {
		t.Error("leaving one ladder ended another")
	}
}

// At the transcode limit, the rung a player left gives way to the one it moved to, rather
// than the new rung waiting (503) until the old one rests.
func TestLadderRungGivesWayAtTheLimit(t *testing.T) {
	m := testManager(t, 1)
	hi := m.Ensure(Spec{ID: "v-a-r1-q720", Group: "v-a-r1", Duration: 60, SegmentSec: 2})
	lo := m.Ensure(Spec{ID: "v-a-r1-q480", Group: "v-a-r1", Duration: 60, SegmentSec: 2})
	running(hi, time.Now())
	if err := m.makeRoom(lo); err != nil {
		t.Fatalf("the new rung had to wait: %v", err)
	}
	if hi.Active() {
		t.Error("the rung the player left is still converting")
	}
	// Someone else's stream, in use, still keeps its place.
	other := m.Ensure(Spec{ID: "v-b-n1", Duration: 60})
	running(other, time.Now())
	if err := m.makeRoom(lo); err != ErrBusy {
		t.Errorf("another viewer's stream gave way: %v", err)
	}
}
