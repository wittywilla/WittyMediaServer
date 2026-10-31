package gamearchive

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wittywilla/wittyplay"
)

// Run by hand with WMS_GAME_ARCHIVE set to a real archive and WMS_GAME_CONSOLE to its
// console, to see it unpack. It only reads the archive, unpacks to a temporary folder
// that's removed afterwards, and prints sizes and times, never a name.
func TestRealArchive(t *testing.T) {
	p, id := os.Getenv("WMS_GAME_ARCHIVE"), os.Getenv("WMS_GAME_CONSOLE")
	if p == "" {
		t.Skip("set WMS_GAME_ARCHIVE and WMS_GAME_CONSOLE to run this")
	}
	console := wittyplay.ByID(id)
	if console == nil {
		t.Fatalf("no console %q", id)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	l, err := List(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%.0f MB archive: headers read in %v, %d files", float64(st.Size())/(1<<20), time.Since(start).Round(time.Millisecond), len(l.Entries))

	c := &Cache{Dir: filepath.Join(t.TempDir(), "unpacked"), Limit: func() int64 { return 16 << 30 }}
	defer c.Close()
	start = time.Now()
	seen := map[string]int{}
	shown := 0
	for {
		s := c.Prepare(p, console)
		seen[s.State]++
		if s.State == stateFailed {
			t.Fatal(s.Error)
		}
		if s.State == stateReady {
			break
		}
		if s.State == stateUnpacking && s.Total > 0 {
			if pct := int(s.Done * 100 / s.Total); pct >= shown+25 {
				shown = pct
				t.Logf("  %d%% after %v", pct, time.Since(start).Round(100*time.Millisecond))
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("unpacked in %v (asked %d times while unpacking)", time.Since(start).Round(100*time.Millisecond), seen[stateUnpacking])
	rom, release, err := c.Acquire(context.Background(), p, console)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	t.Logf("the core opens a %s", filepath.Ext(rom))
	var files int
	var size int64
	_ = filepath.Walk(filepath.Dir(rom), func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files++
			size += info.Size()
		}
		return nil
	})
	t.Logf("%d files, %.1f MB, all checked against what the entry file names", files, float64(size)/(1<<20))
	start = time.Now()
	again, release2, err := c.Acquire(context.Background(), p, console)
	if err != nil {
		t.Fatal(err)
	}
	release2()
	t.Logf("second start: %v, same file: %v, unpacks run: %d", time.Since(start).Round(time.Millisecond), again == rom, c.builds.Load())
}
