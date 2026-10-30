package bdj

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittybluray"
)

// bdjo builds a BD-J object with one application: the test disc's.
func bdjo() []byte {
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
	b = append(b, 1, 0)
	b = append(b, 1, 0x10)
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
	// One parameter: its length byte and text; the block is padded to even.
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

func TestParseOwnObject(t *testing.T) {
	o, err := wittybluray.ParseBDJObject("00000", bdjo())
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Apps) != 1 || o.Apps[0].InitialClass != "t.TestXlet" || o.Apps[0].BaseDir != "00000" || len(o.Apps[0].Params) != 1 || o.Apps[0].Params[0] != "hello world" {
		t.Fatalf("%+v", o.Apps)
	}
}

func javaForTest(t *testing.T) *Java {
	t.Helper()
	j, err := Find(os.Getenv("WMS_TEST_JAVA"))
	if err != nil {
		t.Skip("no Java on this machine: ", err)
	}
	return j
}

func TestProbe(t *testing.T) {
	j := javaForTest(t)
	v, err := j.Probe(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Java", v)
}

// testDisc is a folder holding a disc whose first play is the test application.
func testDisc(t *testing.T) *wittybluray.Disc {
	t.Helper()
	dir := t.TempDir()
	jar, err := os.ReadFile("testdata/00000.jar")
	if err != nil {
		t.Skip("test application not built: ", err)
	}
	index := make([]byte, 78)
	copy(index, "INDX0200")
	binary.BigEndian.PutUint32(index[8:], 78)
	entry := make([]byte, 12)
	entry[0] = 2 << 6
	entry[4] = 3 << 6
	copy(entry[6:], "00000")
	index = append(index, 0, 0, 0, 0)
	index = append(index, entry...)
	index = append(index, entry...)
	index = append(index, 0, 0)
	mpls := make([]byte, 90)
	copy(mpls, "MPLS0200")
	binary.BigEndian.PutUint32(mpls[8:], 58)
	binary.BigEndian.PutUint32(mpls[12:], 70)
	clpi := make([]byte, 90)
	copy(clpi, "HDMV0200")
	binary.BigEndian.PutUint32(clpi[8:], 60)
	binary.BigEndian.PutUint32(clpi[12:], 70)
	files := map[string][]byte{
		"BDMV/index.bdmv":          index,
		"BDMV/MovieObject.bdmv":    append([]byte("MOBJ0200"), make([]byte, 42)...),
		"BDMV/PLAYLIST/00001.mpls": mpls,
		"BDMV/CLIPINF/00001.clpi":  clpi,
		"BDMV/STREAM/00001.m2ts":   make([]byte, 192),
		"BDMV/BDJO/00000.bdjo":     bdjo(),
		"BDMV/JAR/00000.jar":       jar,
		"BDMV/AUXDATA/sound.bdmv":  []byte("BCLK"),
	}
	for name, data := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d, err := wittybluray.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// A disc's application runs end to end: it draws, gets keys, asks for a
// playlist, stores a file, and every attempt to leave the sandbox fails.
func TestApplication(t *testing.T) {
	j := javaForTest(t)
	d := testDisc(t)
	if !d.BDJ {
		t.Fatal("the test disc should be BD-J")
	}
	obj, err := d.BDJObject("00000")
	if err != nil {
		t.Fatal(err)
	}
	work, persist := t.TempDir(), t.TempDir()
	var mu sync.Mutex
	gpr := map[string]string{}
	var played []string
	var failed []string
	var host *Host
	h, err := Start(context.Background(), d, Options{Java: j, CacheDir: t.TempDir(), WorkDir: work, PersistDir: persist,
		Log: func(s string) { t.Log("java: ", s) },
		OnEvent: func(e Event) {
			mu.Lock()
			defer mu.Unlock()
			switch e.Kind {
			case "gpr":
				gpr[e.Args[0]] = e.Args[1]
			case "play":
				played = append(played, e.Args[0])
				if host != nil {
					host.Send("plinfo 1 60 1,2 0")
				}
			case "appfailed":
				failed = append(failed, e.Args...)
			}
		}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	mu.Lock()
	host = h
	mu.Unlock()
	h.StartTitle(0xffff, obj)

	wait := func(what string, ok func() bool) {
		t.Helper()
		for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
			mu.Lock()
			r := ok()
			f := append([]string(nil), failed...)
			mu.Unlock()
			if r {
				return
			}
			if len(f) > 0 {
				t.Fatalf("the application failed: %v", f)
			}
		}
		t.Fatalf("timed out waiting for %s", what)
	}
	wait("the application to finish starting", func() bool { return gpr["110"] == "1" })
	names := map[string]string{"100": "context arguments", "101": "Class.forName of a forbidden class", "102": "starting a process", "103": "reading outside the disc",
		"104": "reflection", "105": "the network", "106": "the disc's own storage", "107": "writing to the disc", "108": "resources from the JAR", "109": "the class loader's resources"}
	mu.Lock()
	for k, name := range names {
		if gpr[k] != "1" {
			t.Errorf("%s: result %q", name, gpr[k])
		}
	}
	mu.Unlock()
	if b, err := os.ReadFile(filepath.Join(persist, "score.txt")); err != nil || len(b) != 3 {
		t.Errorf("stored file: %v %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(work, "vfs", "BDMV", "evil.txt")); err == nil {
		t.Error("the application wrote to the disc's files")
	}
	// It drew a red box.
	pix := func(want [3]uint8) func() bool {
		return func() bool {
			r, g, b, a := h.Pixel(50, 50)
			return a == 255 && r == want[0] && g == want[1] && b == want[2]
		}
	}
	wait("the red box", pix([3]uint8{255, 0, 0}))
	if r, _, _, a := h.Pixel(500, 500); a != 0 || r != 0 {
		t.Errorf("the plane should be clear elsewhere")
	}
	// Keys: Enter reaches the scene's listener; a colour key reaches the event manager.
	h.Send("key 10")
	wait("the green box after Enter", pix([3]uint8{0, 255, 0}))
	h.Send("key 403")
	wait("the blue box after the red key", pix([3]uint8{0, 0, 255}))
	// It asked for a playlist; the end of playback reaches its listener.
	wait("the play request", func() bool { return len(played) > 0 })
	h.Send("time 1.5")
	h.Send("ended")
	wait("the end of the playlist", func() bool { return gpr["111"] == "1" })
	// A frame can be fetched, and waiting for a new one returns.
	if b, v := h.PNG(); len(b) == 0 || v == 0 {
		t.Errorf("PNG: %d bytes, version %d", len(b), v)
	}
	before := h.changes
	if got := h.WaitChange(context.Background(), before, 50*time.Millisecond); got != before {
		t.Errorf("nothing changed, yet the count moved: %d", got)
	}
	h.Kick()
	if got := h.WaitChange(context.Background(), before, time.Second); got == before {
		t.Error("Kick did not wake the wait")
	}
	// When the server goes, Java goes.
	h.Close()
	select {
	case <-h.Done():
	case <-time.After(10 * time.Second):
		t.Error("the process did not end")
	}
}
