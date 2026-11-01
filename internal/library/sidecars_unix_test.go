//go:build !windows

package library

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A pipe at a sidecar's name would hold a scan for good, and scans run one at a time.
func TestPipesDontStopAScan(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "film.mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"film.nfo", "film.json", "tvshow.nfo", "game.cue", "x_meta.xml", "y.wittycd.json"} {
		if err := syscall.Mkfifo(filepath.Join(dir, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "Disc", "VIDEO_TS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "Disc", "VIDEO_TS", "VIDEO_TS.IFO"), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		applyNFO(&Item{}, filepath.Join(dir, "film.nfo"))
		applySidecarJSON(&Item{}, filepath.Join(dir, "film.json"))
		applyShowNFO(&Show{}, filepath.Join(dir, "tvshow.nfo"))
		applyArchiveMeta(&Item{}, filepath.Join(dir, "x_meta.xml"), true)
		readCDSidecar(filepath.Join(dir, "y.iso"))
		_, _ = readPlain(filepath.Join(dir, "film.nfo"))
		_, _ = readSidecar(filepath.Join(dir, "film.json"))
		_, _ = discoverGames(dir, "auto")
		got, _ := discover(dir)
		for _, c := range got {
			if strings.Contains(c.path, "Disc") {
				t.Errorf("a pipe passed for a disc: %s", c.path)
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a pipe stopped the scan")
	}
}

// A link to a device reports no size: it has to be refused for what it is.
func TestLinkToADeviceIsNotASidecar(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "movie.nfo")
	if err := os.Symlink("/dev/zero", p); err != nil {
		t.Fatal(err)
	}
	if got := allocated(func() { applyNFO(&Item{}, p) }); got > 32<<20 {
		t.Errorf("allocated %d MB", got>>20)
	}
}
