// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Made-up ROM bytes only: no real game is in these tests, and nothing leaves the machine.

const achTestKey = "achievements-test-key"

func achFakeRA(t *testing.T, knownHash string) (*httptest.Server, *atomic.Int32) {
	var lists atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Badge/4321_lock.png" {
			w.Write(append([]byte("\x89PNG\r\n\x1a\n"), "badge"...))
			return
		}
		if r.URL.Query().Get("y") != achTestKey {
			http.Error(w, "{}", 401)
			return
		}
		switch r.URL.Path {
		case "/API/API_GetGameList.php":
			lists.Add(1)
			fmt.Fprintf(w, `[{"Title":"Test Cart","ID":777,"ConsoleID":7,"NumAchievements":1,"Points":25,"Hashes":[%q]}]`, knownHash)
		case "/API/API_GetGameExtended.php":
			w.Write([]byte(`{"ID":777,"Title":"Test Cart","Achievements":{"1":{"ID":1,"Title":"Begin","Description":"Start the game","Points":25,"BadgeName":"4321","DisplayOrder":0}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &lists
}

func TestItemAchievements(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "player", "adult", false, "")

	// A cartridge with an iNES header, on a folder nothing may be written to.
	romDir := t.TempDir()
	cart := []byte(strings.Repeat("made-up cartridge bytes ", 100))
	sum := md5.Sum(cart)
	hash := hex.EncodeToString(sum[:])
	rom := filepath.Join(romDir, "Test Cart.nes")
	if err := os.WriteFile(rom, append([]byte("NES\x1a\x01\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"), cart...), 0o444); err != nil {
		t.Fatal(err)
	}
	os.Chmod(romDir, 0o555)
	t.Cleanup(func() { os.Chmod(romDir, 0o755) })
	other := filepath.Join(t.TempDir(), "Unknown.nes")
	os.WriteFile(other, []byte("nobody knows this one"), 0o644)

	srv, lists := achFakeRA(t, hash)
	cache := filepath.Join(t.TempDir(), "ra")
	ra := &gamemeta.RetroAchievements{Key: achTestKey, HTTP: srv.Client(), Dir: cache, Base: srv.URL + "/API", MediaBase: srv.URL, Gap: time.Millisecond}
	ts.GameMeta.Built = func() []gamemeta.Source { return []gamemeta.Source{ra} }

	lib := ts.library(t, "games", romDir)
	game := ts.item(t, &library.Item{LibraryID: lib, Kind: "game", Path: rom, Title: "Test Cart", Console: "nes"})
	unknown := ts.item(t, &library.Item{LibraryID: lib, Kind: "game", Path: other, Title: "Unknown", Console: "nes"})
	disc := ts.item(t, &library.Item{LibraryID: lib, Kind: "game", Path: filepath.Join(romDir, "Disc.cue"), Title: "Disc", Console: "ps1"})
	film := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(romDir, "a.mp4"), Title: "Film"})

	get := func(id int64, token string) (int, AchievementsAnswer, string) {
		t.Helper()
		w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/achievements", id), token, nil)
		var a AchievementsAnswer
		if w.Code == 200 {
			if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
				t.Fatalf("%s", w.Body)
			}
		}
		return w.Code, a, w.Body.String()
	}

	// No key yet.
	if _, a, body := get(game.ID, adult); a.State != "nokey" || a.Message == "" || a.Game != nil || a.Achievements == nil || !strings.Contains(body, `"game":null`) {
		t.Errorf("no key: %s", body)
	}
	// Saving the key doesn't send every game to be looked up again.
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"retroAchievementsKey": achTestKey}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w := ts.do(t, "GET", "/api/settings", admin, nil)
	if strings.Contains(w.Body.String(), achTestKey) || !strings.Contains(w.Body.String(), `"retroAchievementsKeySet":"1"`) {
		t.Errorf("the key isn't masked: %s", w.Body)
	}

	code, a, body := get(game.ID, adult)
	if code != 200 || a.State != "ok" || a.Game == nil || a.Game.ID != 777 || a.Game.Title != "Test Cart" || a.Game.Points != 25 ||
		a.Game.URL != "https://retroachievements.org/game/777" || len(a.Achievements) != 1 {
		t.Fatalf("%d %s", code, body)
	}
	badge := a.Achievements[0].Badge
	if a.Achievements[0].Title != "Begin" || badge != fmt.Sprintf("/api/items/%d/achievements/badge/4321", game.ID) {
		t.Errorf("%+v", a.Achievements[0])
	}
	if strings.Contains(body, achTestKey) || strings.Contains(body, srv.URL) || strings.Contains(body, romDir) {
		t.Errorf("gives away a key, the service or a path: %s", body)
	}
	// The checksum is kept in the cache, and nothing is written beside the game.
	if _, err := os.Stat(filepath.Join(cache, "hashes", fmt.Sprint(game.ID)+".json")); err != nil {
		t.Errorf("checksum not kept: %v", err)
	}
	if entries, _ := os.ReadDir(romDir); len(entries) != 1 {
		t.Errorf("something was written beside the game: %v", entries)
	}
	get(game.ID, adult)
	if n := lists.Load(); n != 1 {
		t.Errorf("the console's list was asked for %d times", n)
	}

	// The badge comes through the server.
	bw := ts.do(t, "GET", badge, adult, nil)
	if bw.Code != 200 || bw.Header().Get("Content-Type") != "image/png" || !strings.HasSuffix(bw.Body.String(), "badge") {
		t.Errorf("badge: %d %q", bw.Code, bw.Body)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/achievements/badge/9999", game.ID), adult, nil); w.Code != 404 {
		t.Errorf("another game's badge: %d", w.Code)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/achievements/badge/..%%2Fx", game.ID), adult, nil); w.Code != 400 && w.Code != 404 {
		t.Errorf("a made-up badge name: %d", w.Code)
	}
	if w := ts.do(t, "GET", badge, "", nil); w.Code != 401 {
		t.Errorf("badge without signing in: %d", w.Code)
	}

	if _, a, body := get(unknown.ID, adult); a.State != "notfound" || a.Message == "" || a.Game != nil {
		t.Errorf("unknown file: %s", body)
	}
	if _, a, body := get(disc.ID, adult); a.State != "unsupported" || a.Message == "" {
		t.Errorf("a disc: %s", body)
	}
	if code, _, _ := get(film.ID, adult); code != 404 {
		t.Errorf("not a game: %d", code)
	}
	if code, _, _ := get(game.ID, ""); code != 401 {
		t.Errorf("without signing in: %d", code)
	}

	// Switched off in the sources list: as if there were no key.
	ts.DB.SetSetting(ctx, "gameSources", `[{"id":"retroachievements","on":false}]`)
	if _, a, body := get(game.ID, adult); a.State != "nokey" {
		t.Errorf("switched off: %s", body)
	}
}

// A game the account can't see has no achievements to show it either.
func TestItemAchievementsHonourLimits(t *testing.T) {
	ts := newTestServer(t)
	_, kid := ts.user(t, "kid", "kids", false, "")
	lib := ts.library(t, "games", t.TempDir())
	mature := ts.item(t, &library.Item{LibraryID: lib, Kind: "game", Path: filepath.Join(t.TempDir(), "m.nes"), Title: "Mature", Console: "nes", ContentRating: "ESRB M"})
	for _, p := range []string{"/api/items/%d/achievements", "/api/items/%d/achievements/badge/1"} {
		if w := ts.do(t, "GET", fmt.Sprintf(p, mature.ID), kid, nil); w.Code != 404 {
			t.Errorf("%s as a kid: %d", p, w.Code)
		}
	}
}
