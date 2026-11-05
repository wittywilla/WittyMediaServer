package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Home reads the progress of every "Recently added" row at once: each row still shows
// what this account has watched, in every library.
func TestHomeRecentRowsCarryProgress(t *testing.T) {
	ts := newTestServer(t)
	u, tok := ts.user(t, "viewer", "adult", true, "")
	ctx := context.Background()
	var watched, started int64
	for i, name := range []string{"films", "home"} {
		dir := t.TempDir()
		lib := ts.library(t, "movies", dir)
		a := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(dir, "a.mkv"), Title: name + " a"})
		ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(dir, "b.mkv"), Title: name + " b"})
		if i == 0 {
			watched = a.ID
		} else {
			started = a.ID
		}
	}
	if err := ts.Store.SetProgress(ctx, u.ID, watched, library.Progress{Watched: true, DurationSec: 100}); err != nil {
		t.Fatal(err)
	}
	if err := ts.Store.SetProgress(ctx, u.ID, started, library.Progress{PositionSec: 30, DurationSec: 100}); err != nil {
		t.Fatal(err)
	}
	w := ts.do(t, "GET", "/api/home", tok, nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var home struct {
		Hubs []struct {
			Key   string
			Items []library.Item
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &home); err != nil {
		t.Fatal(err)
	}
	seen := map[int64]*library.Progress{}
	rows := 0
	for _, h := range home.Hubs {
		if !strings.HasPrefix(h.Key, "recent-") {
			continue
		}
		rows++
		for _, it := range h.Items {
			seen[it.ID] = it.Progress
		}
	}
	if rows != 2 {
		t.Fatalf("%d recently added rows", rows)
	}
	if p := seen[watched]; p == nil || !p.Watched {
		t.Errorf("watched film: %+v", p)
	}
	if p := seen[started]; p == nil || p.PositionSec != 30 {
		t.Errorf("started film in the second library: %+v", p)
	}
	for id, p := range seen {
		if id != watched && id != started && p != nil {
			t.Errorf("item %d has progress it never had: %+v", id, p)
		}
	}
}
