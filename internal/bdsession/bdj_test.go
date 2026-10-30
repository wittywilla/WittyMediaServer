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

// A BD-J object naming the test application, as bdj's own test builds it.
func testBDJO() []byte {
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
	str("t.TestXlet")
	param := "hello world"
	b = append(b, byte(len(param)+1), byte(len(param)))
	b = append(b, param...)
	if (len(param)+2)&1 != 0 {
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

// The whole path: a BD-J first play starts Java, the application's picture
// reaches the poll, keys reach it, its playlist plays, and when the disc's
// application cannot run the disc still plays.
func TestBDJSession(t *testing.T) {
	jar, err := os.ReadFile("../../modules/wittybluray/bdj/testdata/00000.jar")
	if err != nil {
		t.Skip("test application not built")
	}
	java := os.Getenv("WMS_TEST_JAVA")
	if _, err := bdj.Find(java); err != nil {
		t.Skip("no Java on this machine")
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
		"BDMV/BDJO/00000.bdjo":     testBDJO(),
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
	// The registers it set are the navigator's now.
	for end := time.Now().Add(5 * time.Second); s.nav.Registers().GPR[110] != 1 && time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
	}
	if s.nav.Registers().GPR[110] != 1 || s.nav.Registers().GPR[103] != 1 {
		t.Fatalf("GPR110 %d GPR103 %d", s.nav.Registers().GPR[110], s.nav.Registers().GPR[103])
	}
	// The end of the clip goes to the application.
	s.Do(ctx, Action{Type: "ended"})
	for end := time.Now().Add(5 * time.Second); s.nav.Registers().GPR[111] != 1 && time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
	}
	if s.nav.Registers().GPR[111] != 1 {
		t.Fatal("the application did not hear the end of its playlist")
	}
	// Leaving for a plain playlist ends the applications.
	var _ = wittybluray.NoButton
	p = s.Do(ctx, Action{Type: "title", TTN: 1})
	if p.BDJ != nil || s.bd.active {
		t.Fatalf("still in BD-J: %+v", p)
	}
}
