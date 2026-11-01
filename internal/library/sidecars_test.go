package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// allocated says how much a function allocates in all, whatever it gives back.
func allocated(f func()) uint64 {
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	f()
	runtime.ReadMemStats(&b)
	return b.TotalAlloc - a.TotalAlloc
}

// bigFile makes a file of that size without writing it.
func bigFile(t *testing.T, p string, size int64) {
	t.Helper()
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(size); err != nil {
		t.Skipf("no room for a large file: %v", err)
	}
}

// A sidecar is read as it stands by whichever scan comes across it, so one the size of the disk
// has to be refused before it is read. Each of these was read whole.
func TestLargeSidecarsAreNotRead(t *testing.T) {
	dir := t.TempDir()
	const size = 256 << 20
	for name, read := range map[string]func(p string){
		"movie.nfo":             func(p string) { applyNFO(&Item{}, p) },
		"movie.json":            func(p string) { applySidecarJSON(&Item{}, p) },
		"tvshow.nfo":            func(p string) { applyShowNFO(&Show{}, p) },
		"episode.nfo":           func(p string) { applyEpisodeNFO(&Item{}, p) },
		"video.nfo":             func(p string) { readMusicVideoNFO(p) },
		"thing_meta.xml":        func(p string) { applyArchiveMeta(&Item{}, p, true) },
		"own.wittycd.json":      func(p string) { readCDSidecar(strings.TrimSuffix(p, ".wittycd.json") + ".iso") },
		"old.nfo":               func(p string) { _, _ = readPlain(p) },
		"sidecar-big.info.json": func(p string) { readSidecar(p) },
	} {
		p := filepath.Join(dir, name)
		if name == "sidecar-big.info.json" {
			bigFile(t, p, 3*maxSidecar)
		} else {
			bigFile(t, p, size)
		}
		if got := allocated(func() { read(p) }); got > 32<<20 {
			t.Errorf("%s: allocated %d MB", name, got>>20)
		}
		_ = os.Remove(p)
	}
}

// Ordinary sidecars still read.
func TestSmallSidecarsStillRead(t *testing.T) {
	dir := t.TempDir()
	nfo := filepath.Join(dir, "movie.nfo")
	if err := os.WriteFile(nfo, []byte("<movie><title>A Made-Up Film</title><year>2001</year></movie>"), 0o644); err != nil {
		t.Fatal(err)
	}
	it := &Item{}
	if !applyNFO(it, nfo) || it.Title != "A Made-Up Film" {
		t.Errorf("%+v", it)
	}
	js := filepath.Join(dir, "movie.json")
	if err := os.WriteFile(js, []byte(`{"title":"Another Made-Up Film"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	it = &Item{}
	if !applySidecarJSON(it, js) || it.Title != "Another Made-Up Film" {
		t.Errorf("%+v", it)
	}
	if b, err := readSmall(nfo, 1<<20); err != nil || !strings.Contains(string(b), "Made-Up") {
		t.Errorf("%q %v", b, err)
	}
	if _, err := readSmall(nfo, 4); err == nil {
		t.Error("over the limit, it read anyway")
	}
	if _, err := readSmall(dir, 1<<20); err == nil {
		t.Error("a folder read as a file")
	}
}

// A number from a file or a feed is whatever it says, and Go reads "Inf" as a number.
func TestFilesCantSayInfinity(t *testing.T) {
	for _, v := range []string{"Inf", "+Inf", "NaN", "-Inf", "-5", "1e308:1e308:1e308", "1e999"} {
		if got := parseDuration(v); got != 0 {
			t.Errorf("%q: %v", v, got)
		}
	}
	for v, want := range map[string]float64{"1:02:03": 3723, "62:03": 3723, "3723": 3723, "": 0, "x": 0} {
		if got := parseDuration(v); got != want {
			t.Errorf("%q: %v", v, got)
		}
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "m.nfo")
	if err := os.WriteFile(p, []byte("<movie><title>Bad</title><rating>Inf</rating></movie>"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, derr := db.Open(t.TempDir())
	if derr != nil {
		t.Fatal(derr)
	}
	defer d.Close()
	s := &Store{DB: d}
	lib, err := s.CreateLibrary(context.Background(), "M", "movies", []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	it := &Item{LibraryID: lib.ID, Kind: "video", Path: filepath.Join(dir, "m.mkv"), Title: "x", SortTitle: "x"}
	applyNFO(it, p)
	if err := s.UpdateItem(context.Background(), it); err != nil {
		t.Fatal(err)
	}
	got, err := s.Item(context.Background(), it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Rating != nil {
		t.Errorf("rating %v", *got.Rating)
	}
	if _, err := json.Marshal(got); err != nil {
		t.Errorf("the item can't be sent: %v", err)
	}
	inf := math.Inf(1)
	sh := &Show{LibraryID: lib.ID, Path: filepath.Join(dir, "show"), Title: "Show", Rating: &inf}
	if err := s.UpsertShow(context.Background(), sh); err != nil {
		t.Fatal(err)
	}
	gs, err := s.ShowFacts(context.Background(), sh.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(gs); err != nil {
		t.Errorf("the show can't be sent: %v", err)
	}
}

// JSON made of {},{},... costs far more once decoded than it does on disk. Nothing here reads
// the custom field, and no real sidecar has millions of comments or tags.
func TestALargeInfoJSONCostsLittle(t *testing.T) {
	dir := t.TempDir()
	const size = 47 << 20
	for name, body := range map[string][]string{
		"custom":   {`{"custom":{"a":[`, `{},`, `{}]}}`},
		"comments": {`{"comments":[`, `{},`, `{}]}`},
		"tags":     {`{"tags":[`, `"",`, `""]}`},
		"genres":   {`{"genres":[`, `"",`, `""]}`},
	} {
		p := filepath.Join(dir, name+".info.json")
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.WriteString(body[0])
		block := strings.Repeat(body[1], (1<<20)/len(body[1]))
		for n := len(body[0]); n+len(block) < size; n += len(block) {
			_, _ = f.WriteString(block)
		}
		_, _ = f.WriteString(body[2])
		f.Close()
		var got *videoSidecar
		used := allocated(func() { got, _ = readSidecar(p) })
		if got == nil {
			t.Errorf("%s: not read", name)
		}
		if used > 200<<20 {
			t.Errorf("%s: allocated %d MB", name, used>>20)
		}
		_ = os.Remove(p)
	}
	p := filepath.Join(dir, "real.info.json")
	if err := os.WriteFile(p, []byte(`{"title":"Made Up","tags":["a","b"],"comments":[{"author":"x","text":"hi"}],"custom":{"k":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	v, _ := readSidecar(p)
	if v == nil || v.Title != "Made Up" || len(v.Tags) != 2 || len(v.Comments) != 1 || v.Comments[0].Text != "hi" || string(v.Custom) != `{"k":1}` {
		t.Errorf("%+v", v)
	}
}

type zeros struct{}

func (zeros) Read(b []byte) (int, error) { clear(b); return len(b), nil }

// A feed chooses how big an episode is, and a full disk stops the database too.
func TestDownloadLeavesRoomOnTheDisk(t *testing.T) {
	var buf bytes.Buffer
	src := bytes.NewReader(make([]byte, 3<<20))
	free := int64(1 << 40)
	n, err := copyLeavingRoom(&buf, src, func() int64 { return free })
	if err != nil || n != 3<<20 {
		t.Fatalf("plenty of room: %d %v", n, err)
	}
	buf.Reset()
	calls := 0
	n, err = copyLeavingRoom(io.Discard, io.LimitReader(zeros{}, 4<<30), func() int64 {
		calls++
		if calls > 1 {
			return keepFree - 1
		}
		return keepFree + 1
	})
	if !errors.Is(err, ErrNoRoom) {
		t.Errorf("it filled up: %v", err)
	}
	if n > roomCheckEvery {
		t.Errorf("wrote %d after running out", n)
	}
	src = bytes.NewReader([]byte("x"))
	if _, err := copyLeavingRoom(&buf, src, func() int64 { return 1 }); !errors.Is(err, ErrNoRoom) {
		t.Errorf("no room at the start: %v", err)
	}
	// Unknown room isn't a reason to stop.
	buf.Reset()
	src = bytes.NewReader([]byte("x"))
	if n, err := copyLeavingRoom(&buf, src, func() int64 { return -1 }); err != nil || n != 1 {
		t.Errorf("unknown: %d %v", n, err)
	}
}
