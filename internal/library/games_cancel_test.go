package library

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
)

// A cancelled scan keeps what it really worked out, and nothing else: what it didn't get to
// isn't taken for a file that isn't a game, now or in any scan after it.
func TestCancellingAScanDoesNotChangeWhatTheNextOneFinds(t *testing.T) {
	modtest.Need(t, "wittyplay")
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	store := &Store{DB: d}
	ctx := context.Background()
	root := t.TempDir()
	lib, err := store.CreateLibrary(ctx, "Games", "games", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"Alpha Quest.sfc", "Bravo Racer.sfc", "Charlie Golf.sfc", "Delta Force Ten.sfc"} {
		if err := os.WriteFile(filepath.Join(root, name), bytes.Repeat([]byte{byte(i + 1)}, 512<<10), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	whole := func(id int64) int {
		ix := newArchiveIndex(store, id)
		defer ix.close()
		c, err := ix.discover(root, "snes")
		if err != nil {
			t.Fatal(err)
		}
		return len(c)
	}
	other, err := store.CreateLibrary(ctx, "Same games", "games", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	want := whole(other.ID)
	if want != 4 {
		t.Fatalf("found %d of 4 to begin with", want)
	}

	// Cancelled partway: it says so, rather than handing back a short list.
	ix := newArchiveIndex(store, lib.ID)
	asked := 0
	ix.stopped = func() bool { asked++; return asked > 2 }
	found, err := ix.discover(root, "snes")
	ix.close()
	if !errors.Is(err, context.Canceled) || found != nil {
		t.Fatalf("cancelled: %v, %v", found, err)
	}
	if n := whole(lib.ID); n != want {
		t.Fatalf("after a cancelled scan: %d, want %d", n, want)
	}
}
