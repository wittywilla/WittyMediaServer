// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gamemeta

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const raTestKey = "secret-web-api-key"

// fakeRA stands in for RetroAchievements' Web API and its picture host.
type fakeRA struct {
	mu    sync.Mutex
	calls map[string]int
	down  bool
}

func (f *fakeRA) count(k string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[k]
}

func newFakeRA(t *testing.T) (*fakeRA, *httptest.Server) {
	f := &fakeRA{calls: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.calls[r.URL.Path+"?"+r.URL.Query().Get("i")]++
		down := f.down
		f.mu.Unlock()
		if down {
			http.Error(w, "down", 503)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/Badge/") {
			if r.URL.Path != "/Badge/12345_lock.png" {
				http.NotFound(w, r)
				return
			}
			w.Write(append([]byte("\x89PNG\r\n\x1a\n"), "fake picture"...))
			return
		}
		if r.URL.Query().Get("y") != raTestKey {
			http.Error(w, `{"message":"Unauthenticated."}`, 401)
			return
		}
		switch r.URL.Path {
		case "/API/API_GetConsoleIDs.php":
			w.Write([]byte(`[{"ID":7,"Name":"NES/Famicom"}]`))
		case "/API/API_GetGameList.php":
			if r.URL.Query().Get("h") != "1" {
				t.Errorf("list asked for without checksums: %s", r.URL.RawQuery)
			}
			switch r.URL.Query().Get("i") {
			case "7":
				w.Write([]byte(`[{"Title":"Made Up <b>Quest</b>","ID":1234,"ConsoleID":7,"NumAchievements":2,"Points":15,"Hashes":["0123456789abcdef0123456789abcdef","NOT-A-HASH"]}]`))
			case "6":
				w.Write([]byte(`[{"Title":"Colour Game","ID":55,"ConsoleID":6,"NumAchievements":1,"Points":5,"Hashes":["fedcba9876543210fedcba9876543210"]}]`))
			default:
				w.Write([]byte(`[]`))
			}
		case "/API/API_GetGameExtended.php":
			if r.URL.Query().Get("i") != "1234" {
				w.Write([]byte(`{"ID":null,"Title":null,"Achievements":[]}`))
				return
			}
			w.Write([]byte(`{"ID":1234,"Title":"Made Up Quest","ConsoleID":7,"Achievements":{
				"9":{"ID":9,"Title":"Second","Description":"Do the <i>second</i> thing","Points":10,"BadgeName":"12345","DisplayOrder":2},
				"3":{"ID":3,"Title":"First","Description":"Do the first thing","Points":5,"BadgeName":"../../etc","DisplayOrder":1}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func newTestRA(srv *httptest.Server, dir, key string) *RetroAchievements {
	return &RetroAchievements{Key: key, HTTP: srv.Client(), Dir: dir, Base: srv.URL + "/API", MediaBase: srv.URL, Gap: time.Millisecond}
}

func TestRetroAchievementsFindsAGameAndItsAchievements(t *testing.T) {
	f, srv := newFakeRA(t)
	dir := t.TempDir()
	ra := newTestRA(srv, dir, raTestKey)
	ctx := context.Background()
	if ra.About().ID != "retroachievements" || ra.About().Fields[0].Setting != "retroAchievementsKey" || !ra.Sidecar() {
		t.Fatalf("%+v", ra.About())
	}
	if _, err := ra.Lookup(ctx, Query{Console: "nes", Title: "x"}); !errors.Is(err, ErrNotFound) {
		t.Error("asked during lookups")
	}
	id, title, err := ra.FindGame(ctx, []int{7}, "0123456789abcdef0123456789abcdef")
	if err != nil || id != 1234 || title != "Made Up Quest" {
		t.Fatalf("%d %q %v", id, title, err)
	}
	// Game Boy is two consoles there: both lists are looked in.
	if id, _, err := ra.FindGame(ctx, []int{4, 6}, "fedcba9876543210fedcba9876543210"); err != nil || id != 55 {
		t.Fatalf("%d %v", id, err)
	}
	if _, _, err := ra.FindGame(ctx, []int{7}, "00000000000000000000000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown checksum: %v", err)
	}
	g, err := ra.Game(ctx, 1234)
	if err != nil {
		t.Fatal(err)
	}
	if g.Points != 15 || len(g.Achievements) != 2 || g.Achievements[0].ID != 3 || g.Achievements[1].Title != "Second" {
		t.Fatalf("%+v", g)
	}
	if g.Achievements[1].Description != "Do the second thing" || g.Achievements[1].Badge != "12345" {
		t.Errorf("not cleaned: %+v", g.Achievements[1])
	}
	if g.Achievements[0].Badge != "" || g.HasBadge("../../etc") {
		t.Error("a badge name that isn't a number was kept")
	}
	if g.URL() != "https://retroachievements.org/game/1234" {
		t.Error(g.URL())
	}
	if _, err := ra.Game(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown game: %v", err)
	}

	// Kept: asking again, or after a restart, doesn't ask the service.
	ra.FindGame(ctx, []int{7}, "0123456789abcdef0123456789abcdef")
	ra.Game(ctx, 1234)
	again := newTestRA(srv, dir, raTestKey)
	if id, _, err := again.FindGame(ctx, []int{7}, "0123456789abcdef0123456789abcdef"); err != nil || id != 1234 {
		t.Fatal(err)
	}
	if _, err := again.Game(ctx, 1234); err != nil {
		t.Fatal(err)
	}
	if n := f.count("/API/API_GetGameList.php?7"); n != 1 {
		t.Errorf("list asked for %d times", n)
	}
	if n := f.count("/API/API_GetGameExtended.php?1234"); n != 1 {
		t.Errorf("game asked for %d times", n)
	}

	// Old lists are asked for again; if the service is down, the old one does.
	old := filepath.Join(dir, "lists", "7.json")
	var l raList
	b, _ := os.ReadFile(old)
	json.Unmarshal(b, &l)
	l.Fetched = time.Now().Add(-30 * 24 * time.Hour)
	b, _ = json.Marshal(l)
	os.WriteFile(old, b, 0o644)
	f.mu.Lock()
	f.down = true
	f.mu.Unlock()
	third := newTestRA(srv, dir, raTestKey)
	if id, _, err := third.FindGame(ctx, []int{7}, "0123456789abcdef0123456789abcdef"); err != nil || id != 1234 {
		t.Fatalf("stale list not used: %v", err)
	}
	if n := f.count("/API/API_GetGameList.php?7"); n != 2 {
		t.Errorf("stale list not refreshed: %d", n)
	}
}

func TestRetroAchievementsBadges(t *testing.T) {
	f, srv := newFakeRA(t)
	dir := t.TempDir()
	ra := newTestRA(srv, dir, raTestKey)
	ctx := context.Background()
	b, err := ra.Badge(ctx, "12345")
	if err != nil || !strings.HasSuffix(string(b), "fake picture") {
		t.Fatalf("%q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "badges", "12345_lock.png")); err != nil {
		t.Error("not kept on disk")
	}
	ra.Badge(ctx, "12345")
	if n := f.count("/Badge/12345_lock.png?"); n != 1 {
		t.Errorf("fetched %d times", n)
	}
	for _, bad := range []string{"../x", "12a", "", "123456789012"} {
		if _, err := ra.Badge(ctx, bad); err == nil {
			t.Errorf("%q fetched", bad)
		}
	}
	if _, err := ra.Badge(ctx, "777"); err == nil {
		t.Error("a missing badge was served")
	}
}

func TestRetroAchievementsKeys(t *testing.T) {
	_, srv := newFakeRA(t)
	ctx := context.Background()
	none := newTestRA(srv, t.TempDir(), "")
	if _, _, err := none.FindGame(ctx, []int{7}, "0123456789abcdef0123456789abcdef"); !errors.Is(err, ErrNotSet) {
		t.Errorf("no key: %v", err)
	}
	if _, err := none.Test(ctx); !errors.Is(err, ErrNotSet) {
		t.Errorf("no key: %v", err)
	}
	wrong := newTestRA(srv, t.TempDir(), "wrong-key-value")
	_, _, err := wrong.FindGame(ctx, []int{7}, "0123456789abcdef0123456789abcdef")
	if !errors.Is(err, ErrKey) {
		t.Errorf("wrong key: %v", err)
	}
	if _, err := wrong.Test(ctx); !errors.Is(err, ErrKey) {
		t.Errorf("wrong key: %v", err)
	}
	if msg, err := newTestRA(srv, t.TempDir(), raTestKey).Test(ctx); err != nil || msg == "" {
		t.Errorf("%q %v", msg, err)
	}
	// An address with the key in it never shows up in an error.
	srv.Close()
	_, _, err = newTestRA(srv, t.TempDir(), raTestKey).FindGame(ctx, []int{7}, "0123456789abcdef0123456789abcdef")
	if err == nil || strings.Contains(err.Error(), raTestKey) {
		t.Errorf("%v", err)
	}
}
