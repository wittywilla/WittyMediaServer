package dvdsession

import (
	"os"
	"strings"
	"testing"

	"github.com/wittywilla/wittydvd"
)

// A sub-picture unit that shows a 4x2 picture, recolours it at delay 100
// (1.14 s) and stops at delay 200 becomes two subtitle events.
func TestRenderSubFrames(t *testing.T) {
	unit := []byte{
		0, 0, 0, 6, // size (patched below), control table at 6
		0x11, 0x11, // RLE: one row per field, run 4 of code 1
		0, 0, 0, 30, // DCSQ 1 at 0, next at 30
		0x03, 0x00, 0x10, 0x04, 0x00, 0xf0,
		0x05, 0x00, 0xa0, 0x0d, 0x01, 0x40, 0x15,
		0x06, 0x00, 0x04, 0x00, 0x05,
		0x01, 0xff,
		0, 100, 0, 38, 0x03, 0x00, 0x20, 0xff, // DCSQ 2: recolour, next at 38
		0, 200, 0, 38, 0x02, 0xff, // DCSQ 3: stop (last)
	}
	unit[0], unit[1] = 0, byte(len(unit))
	unit[2], unit[3] = 0, 6
	var pgc wittydvd.PGC
	pgc.Palette[1], pgc.Palette[2] = 0x108080, 0xeb8080
	pics := (&Session{}).renderSub(wittydvd.SPUUnit{PTS: 90000, Bytes: unit}, 10, 0, 100, -1, &pgc)
	if len(pics) != 2 {
		t.Fatalf("%d pictures", len(pics))
	}
	a, b := pics[0].ev, pics[1].ev
	if a.Start != 11 || b.Start != a.End || b.End < 13.27 || b.End > 13.28 || a.X != 10 || a.W != 4 || pics[1].png == nil {
		t.Fatalf("events %+v %+v", a, b)
	}
}

// TestRealDiscSubtitles lists the main feature's first subtitle events from
// a decrypted DVD image when WMS_TEST_ISO points at one (skipped otherwise)
// and fetches each picture by the name in its URL.
func TestRealDiscSubtitles(t *testing.T) {
	p := os.Getenv("WMS_TEST_ISO")
	if p == "" {
		t.Skip("set WMS_TEST_ISO to a decrypted DVD image")
	}
	m := &Manager{BaseURL: "/api", sessions: map[string]*Session{}}
	s, err := m.Open(1, p, Prefs{Wide: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	main := s.opened.Disc.MainTitle()
	evs, err := s.Subtitles(main.Number, 0, 0, 600)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) == 0 {
		t.Skip("no subtitles in the first ten minutes")
	}
	for _, ev := range evs {
		name := ev.URL[strings.LastIndexByte(ev.URL, '/')+1:]
		if b, ok := s.SubPNG(name); !ok || len(b) == 0 || ev.End <= ev.Start {
			t.Fatalf("event %+v: picture %q missing", ev, name)
		}
	}
	t.Logf("%d subtitle events in the first ten minutes of title %d", len(evs), main.Number)
}
