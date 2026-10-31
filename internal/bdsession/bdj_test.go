package bdsession

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittybluray/bdj"
)

// A BD-J object naming one application, as bdj's own test builds it.
func testBDJO(class string, params ...string) []byte {
	var b []byte
	u32 := func(v uint32) { b = binary.BigEndian.AppendUint32(b, v) }
	u16 := func(v int) { b = binary.BigEndian.AppendUint16(b, uint16(v)) }
	str := func(s string) {
		b = append(b, byte(len(s)))
		b = append(b, s...)
		if (len(s)+1)&1 != 0 {
			b = append(b, 0)
		}
	}
	b = append(b, "BDJO0200"...)
	b = append(b, make([]byte, 40)...)
	u32(10)
	b = append(b, "*****"...)
	b = append(b, make([]byte, 5)...)
	u32(2)
	b = append(b, 0, 0)
	u32(4)
	u32(0)
	u32(0)
	b = append(b, 1, 0, 1, 0x10)
	u32(0x12345678)
	u16(0x4001)
	b = append(b, make([]byte, 10)...)
	u16(0)
	b = append(b, 200, 0x70)
	u16(0)
	str("")
	u16(0)
	str("00000")
	str("")
	str(class)
	var block []byte
	for _, p := range params {
		block = append(block, byte(len(p)))
		block = append(block, p...)
	}
	b = append(b, byte(len(block)))
	b = append(b, block...)
	if (len(block)+1)&1 != 0 {
		b = append(b, 0)
	}
	u32(0)
	u16(0)
	return b
}

func testPlaylist() []byte {
	head := make([]byte, 58)
	copy(head, "MPLS0200")
	head[45] = 1
	binary.BigEndian.PutUint32(head[8:], 58)
	pi := []byte("00001M2TS")
	pi = append(pi, 0, 1, 0)
	pi = binary.BigEndian.AppendUint32(pi, 0)
	pi = binary.BigEndian.AppendUint32(pi, 45000*10)
	pi = append(pi, make([]byte, 12)...)
	stn := make([]byte, 16)
	stn[4] = 1
	stn = append(stn, 9, 1, 0x10, 0x11, 0, 0, 0, 0, 0, 0, 5, 0x1b, 0x61, 0, 0, 0)
	binary.BigEndian.PutUint16(stn, uint16(len(stn)-2))
	pi = append(pi, stn...)
	list := append(make([]byte, 6), 0, 1, 0, 0)
	list = binary.BigEndian.AppendUint16(list, uint16(len(pi)))
	list = append(list, pi...)
	binary.BigEndian.PutUint32(head[12:], uint32(58+len(list)))
	out := append(head, list...)
	return append(out, 0, 0, 0, 0, 0, 0)
}

func testClip() []byte {
	b := make([]byte, 60)
	copy(b, "HDMV0200")
	binary.BigEndian.PutUint32(b[56:], 1)
	binary.BigEndian.PutUint32(b[8:], 60)
	binary.BigEndian.PutUint32(b[12:], 60+26)
	seq := append(make([]byte, 5), 1, 0, 0, 0, 0, 1, 0)
	seq = append(seq, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	seq = binary.BigEndian.AppendUint32(seq, 45000*10)
	seq = seq[:26]
	return append(append(b, seq...), make([]byte, 6)...)
}

// bdjDisc is a folder holding a disc whose first play is the given object and test JAR.
func bdjDisc(t *testing.T, jarName string, object []byte) string {
	t.Helper()
	jar, err := os.ReadFile("../../modules/wittybluray/bdj/testdata/" + jarName)
	if err != nil {
		t.Skip("test application not built")
	}
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
		"BDMV/STREAM/00001.m2ts":   make([]byte, 192),
		"BDMV/BDJO/00000.bdjo":     object,
		"BDMV/JAR/00000.jar":       jar,
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// javaForTest is the Java to test with: the path bdj.Find takes, or the test skips.
func javaForTest(t *testing.T) string {
	t.Helper()
	path := os.Getenv("WMS_TEST_JAVA")
	j, err := bdj.Find(path)
	if err == nil {
		_, err = j.Probe(context.Background(), t.TempDir())
	}
	if err != nil {
		if os.Getenv("WMS_REQUIRE_JAVA") != "" {
			t.Fatal("this test must run here: ", err)
		}
		t.Skip("no usable Java on this machine: ", err)
	}
	return path
}

// The whole path: a BD-J first play starts Java, the application's picture
// reaches the poll, keys reach it, its playlist plays, and when the disc's
// application cannot run the disc still plays.
func TestBDJSession(t *testing.T) {
	java := javaForTest(t)
	dir := bdjDisc(t, "00000.jar", testBDJO("t.TestXlet", "hello world"))
	ctx := context.Background()

	// With BD-J off, the disc plays from its playlists.
	m := testManager(t)
	s, err := m.Open(1, dir, Prefs{})
	if err != nil {
		t.Fatal(err)
	}
	if p := s.Start(ctx); p.BDJ != nil || p.Kind == "stop" {
		t.Fatalf("BD-J off: %+v", p)
	}
	s.Close()

	m.BDJ = func() (bool, string) { return true, java }
	s, err = m.Open(1, dir, Prefs{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := s.Start(ctx)
	if p.BDJ == nil {
		t.Fatalf("no BD-J view: %+v", p)
	}
	waitFor := func(what string, ok func(*BDJPoll) bool) *BDJPoll {
		t.Helper()
		since := -1
		for end := time.Now().Add(20 * time.Second); time.Now().Before(end); {
			r := s.PollBDJ(ctx, since)
			since = r.Change
			if ok(r) {
				return r
			}
		}
		t.Fatalf("timed out waiting for %s", what)
		return nil
	}
	// The application asks for playlist 1, a short one: a clip under its graphics.
	r := waitFor("the playlist", func(r *BDJPoll) bool {
		return r.Presentation != nil && r.Presentation.Media != nil && r.Presentation.Media.Type == "hls" && r.Presentation.BDJ != nil
	})
	if r.Presentation.Media.Loop {
		t.Error("nothing plays this again, so it should not loop")
	}
	waitFor("the first picture", func(r *BDJPoll) bool { return r.FrameURL != "" && s.bd.host.Version() > 0 })
	red := func() bool { x, _, _, a := s.bd.host.Pixel(50, 50); return x == 255 && a == 255 }
	for end := time.Now().Add(10 * time.Second); !red() && time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
	}
	if !red() {
		t.Fatal("no red box")
	}
	if b, ok := s.BDJFrame(); !ok || len(b) == 0 {
		t.Fatal("no frame")
	}
	// Keys go to the application, not the navigator.
	s.Do(ctx, Action{Type: "key", Key: "enter"})
	green := func() bool { _, g, _, a := s.bd.host.Pixel(50, 50); return g == 255 && a == 255 }
	for end := time.Now().Add(10 * time.Second); !green() && time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
	}
	if !green() {
		t.Fatal("Enter did not reach the application")
	}
	// The registers it set are the navigator's now. The application's events
	// change them on their own goroutine, under the session's lock.
	gpr := func(n int) uint32 {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.nav.Registers().GPR[n]
	}
	for end := time.Now().Add(5 * time.Second); gpr(110) != 1 && time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
	}
	if gpr(110) != 1 || gpr(103) != 1 {
		t.Fatalf("GPR110 %d GPR103 %d", gpr(110), gpr(103))
	}
	// The end of the clip goes to the application.
	s.Do(ctx, Action{Type: "ended"})
	for end := time.Now().Add(5 * time.Second); gpr(111) != 1 && time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
	}
	if gpr(111) != 1 {
		t.Fatal("the application did not hear the end of its playlist")
	}
	// Leaving for a plain playlist ends the applications.
	var _ = wittybluray.NoButton
	p = s.Do(ctx, Action{Type: "title", TTN: 1})
	s.mu.Lock()
	active := s.bd.active
	s.mu.Unlock()
	if p.BDJ != nil || active {
		t.Fatalf("still in BD-J: %+v", p)
	}
}

// A disc asking for playlists and titles as fast as it can holds up nothing: the
// session keeps answering, and the requests that would start work are paced.
func TestBDJFloodsDoNotHoldUpTheSession(t *testing.T) {
	java := javaForTest(t)
	for _, mode := range []string{"flood-play", "flood-title", "flood-gpr"} {
		t.Run(mode, func(t *testing.T) {
			dir := bdjDisc(t, "attack.jar", testBDJO("a.AttackXlet", mode))
			m := testManager(t)
			m.BDJ = func() (bool, string) { return true, java }
			s, err := m.Open(1, dir, Prefs{})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ctx := context.Background()
			if p := s.Start(ctx); p.BDJ == nil {
				t.Fatalf("no BD-J view: %+v", p)
			}
			// Let the flood build up, then ask the session for what a player asks for.
			time.Sleep(2 * time.Second)
			var worst time.Duration
			for i := 0; i < 30; i++ {
				start := time.Now()
				s.Do(ctx, Action{Type: "tick", PositionSec: float64(i) + 1})
				s.Current()
				if d := time.Since(start); d > worst {
					worst = d
				}
				time.Sleep(50 * time.Millisecond)
			}
			if worst > 3*time.Second {
				t.Errorf("the session took %v to answer", worst)
			}
			start := time.Now()
			s.Close()
			if d := time.Since(start); d > 5*time.Second {
				t.Errorf("closing took %v", d)
			}
		})
	}
}

// When a disc's code passes a limit, Java ends and the disc plays from its playlists.
func TestBDJRunawayFallsBack(t *testing.T) {
	java := javaForTest(t)
	dir := bdjDisc(t, "attack.jar", testBDJO("a.AttackXlet", "heap"))
	m := testManager(t)
	m.BDJ = func() (bool, string) { return true, java }
	s, err := m.Open(1, dir, Prefs{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if p := s.Start(ctx); p.BDJ == nil {
		t.Fatalf("no BD-J view: %+v", p)
	}
	for end := time.Now().Add(60 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if p := s.Current(); p != nil && p.BDJ == nil {
			return
		}
		if time.Now().After(end) {
			t.Fatal("the session stayed on the disc's menu after Java ended")
		}
	}
}
