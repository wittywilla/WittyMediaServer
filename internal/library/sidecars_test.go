package library

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
