// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
	"github.com/wittywilla/wittymodule/service"
)

// wittyPlayCalls stands in for the module's answers to the server's own calls, and keeps what
// the server asked.
type wittyPlayCalls struct {
	mu       sync.Mutex
	launches []emulatorLaunch
	offers   [][2]string
	result   emulatorResult
	err      error
}

func fakeWittyPlayCalls(t *testing.T) *wittyPlayCalls {
	t.Helper()
	c := &wittyPlayCalls{}
	old, running, console := callWittyPlay, service.Running, emulatorConsole
	service.Running = func(string) bool { return true }
	emulatorConsole = func(id string) *wittyplay.Console {
		if id != "ps2" {
			return nil
		}
		return &wittyplay.Console{ID: "ps2", Name: "PlayStation 2"}
	}
	callWittyPlay = func(_ context.Context, name string, args []any, out []any) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		switch name {
		case "docker.Offers":
			c.offers = append(c.offers, [2]string{args[0].(string), args[1].(string)})
			if c.err != nil {
				return c.err
			}
			*out[0].(*[]emulatorOffer) = []emulatorOffer{{ID: "pcsx2", Name: "PCSX2", Version: "2.2.0", Stream: "pipe", Ready: true}}
		case "docker.Launch":
			c.launches = append(c.launches, args[0].(emulatorLaunch))
			if c.err != nil {
				return c.err
			}
			// A copy: the server changes what it's given.
			b, _ := json.Marshal(c.result)
			return json.Unmarshal(b, out[0])
		case "stream.Session.Wait":
			*out[0].(*bool) = true
		default:
			t.Errorf("the server called %s", name)
		}
		return nil
	}
	t.Cleanup(func() { callWittyPlay, service.Running, emulatorConsole = old, running, console })
	return c
}

func (c *wittyPlayCalls) launched() []emulatorLaunch {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]emulatorLaunch(nil), c.launches...)
}

// ps2Game puts a PlayStation 2 game in a library, its file really there.
func ps2Game(t *testing.T, ts *testServer, title, rating string) (*library.Item, int64) {
	t.Helper()
	dir := t.TempDir()
	lib := ts.library(t, "games", dir)
	path := filepath.Join(dir, title+".iso")
	if err := os.WriteFile(path, []byte("not really a game"), 0o644); err != nil {
		t.Fatal(err)
	}
	return ts.item(t, &library.Item{LibraryID: lib, Kind: "game", Path: path, Title: title, SortTitle: title, Console: "ps2", ContentRating: rating}), lib
}

// TestPlayWithAnEmulatorHonoursLimits: the server decides who may play what before WittyPlay
// hears anything. An account whose limits hide a game gets "not found" from both routes, and
// nothing reaches the module.
func TestPlayWithAnEmulatorHonoursLimits(t *testing.T) {
	t.Setenv("WMS_WITTYPLAY_RUNNER", "/nonexistent/runner")
	calls := fakeWittyPlayCalls(t)
	calls.result = emulatorResult{Session: &emulatorSession{ID: "abc123", Backend: "docker+webrtc", Emulator: "pcsx2", Name: "PCSX2",
		Connect: "/docker/sessions/abc123/connect", Stop: "/docker/sessions/abc123", Pause: "/docker/sessions/abc123/pause", Resume: "/docker/sessions/abc123/resume",
		Width: 1280, Height: 720, FPS: 60, Players: 4}}
	ts := newTestServer(t)
	game, _ := ps2Game(t, ts, "Open Game", "")
	hidden, _ := ps2Game(t, ts, "Hidden Game", "")
	film := ts.item(t, &library.Item{LibraryID: game.LibraryID, Kind: "movie", Path: filepath.Join(t.TempDir(), "film.mkv"), Title: "Film"})
	adultU, adult := ts.user(t, "grown", auth.KindAdult, false, "")
	_, limited := ts.user(t, "limited", auth.KindAdult, false, fmt.Sprintf(`{"maxAge":-1,"allowUnrated":true,"libraries":[%d]}`, game.LibraryID))
	play := func(id int64, tok string, body any) (int, map[string]any) {
		t.Helper()
		w := ts.do(t, "POST", fmt.Sprintf("/api/v1/games/%d/play/emulator", id), tok, body)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	pcsx2 := map[string]string{"emulator": "pcsx2"}

	// Signed out, a game the account's limits hide, something that isn't a game, a game that isn't there.
	for what, got := range map[string]int{
		"signed out":         first(play(game.ID, "", pcsx2)),
		"a hidden game":      first(play(hidden.ID, limited, pcsx2)),
		"a film":             first(play(film.ID, adult, pcsx2)),
		"no such game":       first(play(999999, adult, pcsx2)),
		"no emulator":        first(play(game.ID, adult, map[string]string{})),
		"a path, not a name": first(play(game.ID, adult, map[string]string{"emulator": "../../etc/passwd"})),
	} {
		want := 404
		switch what {
		case "signed out":
			want = 401
		case "no emulator", "a path, not a name":
			want = 400
		}
		if got != want {
			t.Errorf("%s: %d, want %d", what, got, want)
		}
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/v1/games/%d/emulators", hidden.ID), limited, nil); w.Code != 404 {
		t.Errorf("the list for a hidden game: %d", w.Code)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/v1/games/%d/emulators", game.ID), "", nil); w.Code != 401 {
		t.Errorf("the list signed out: %d", w.Code)
	}
	if n := len(calls.launched()); n != 0 || len(calls.offers) != 0 {
		t.Fatalf("a refused request reached WittyPlay: %d starts, %d lists", n, len(calls.offers))
	}

	// The list, for a game the account can see.
	w := ts.do(t, "GET", fmt.Sprintf("/api/v1/games/%d/emulators", game.ID), limited, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"pcsx2"`) || calls.offers[0][0] != "ps2" || !filepath.IsAbs(calls.offers[0][1]) {
		t.Fatalf("the list: %d %s %v", w.Code, w.Body, calls.offers)
	}

	// A start: WittyPlay is told this account, this game, the library's own file and the BIOS
	// folder, and nothing the client sent but the emulator's name.
	code, out := play(game.ID, adult, map[string]any{"emulator": "pcsx2", "rom": "/etc/passwd", "userId": 1, "UserID": 1, "ROM": "/etc/shadow"})
	if code != 201 {
		t.Fatalf("a start: %d %v", code, out)
	}
	l := calls.launched()
	if len(l) != 1 || l[0].UserID != adultU.ID || l[0].ItemID != game.ID || l[0].ROM != game.Path || l[0].Console != "ps2" || l[0].Emulator != "pcsx2" || !filepath.IsAbs(l[0].SystemDir) {
		t.Fatalf("what WittyPlay was asked: %+v", l)
	}
	// The answer's paths are this server's, version 1.
	for key, want := range map[string]string{
		"connect": "/api/v1/modules/wittyplay/docker/sessions/abc123/connect",
		"stop":    "/api/v1/modules/wittyplay/docker/sessions/abc123",
		"pause":   "/api/v1/modules/wittyplay/docker/sessions/abc123/pause",
		"resume":  "/api/v1/modules/wittyplay/docker/sessions/abc123/resume",
	} {
		if out[key] != want {
			t.Errorf("%s: %v, want %s", key, out[key], want)
		}
	}
	if _, ok := out["kasmUrl"]; ok {
		t.Errorf("a streamed game has a desktop address: %v", out["kasmUrl"])
	}
	if out["notices"] == nil || out["players"] != float64(4) {
		t.Errorf("the answer: %v", out)
	}

	// WittyPlay's refusals come back as its own words: 409, or 503 when there's no room.
	calls.result = emulatorResult{Refusal: &emulatorRefusal{Code: "too-small", Text: "PCSX2 needs about 4 GB of memory, and this server has 1 GB. It can't run here."}}
	if code, out := play(game.ID, adult, pcsx2); code != 409 || out["code"] != "too-small" || !strings.Contains(out["error"].(string), "memory") {
		t.Errorf("a refusal: %d %v", code, out)
	}
	calls.result = emulatorResult{Refusal: &emulatorRefusal{Code: "busy", Text: "full", Busy: true}}
	if code, _ := play(game.ID, adult, pcsx2); code != 503 {
		t.Errorf("no room: %d", code)
	}
	// A desktop session's address is moved under this server's too, ticket and all.
	calls.result = emulatorResult{Session: &emulatorSession{ID: "k1", Backend: "docker+kasmvnc", Emulator: "pcsx2", Name: "PCSX2", KasmURL: "/kasm/k1/?ticket=T", Stop: "/docker/sessions/k1"}}
	if code, out := play(game.ID, adult, pcsx2); code != 201 || out["kasmUrl"] != "/api/v1/modules/wittyplay/kasm/k1/?ticket=T" || out["connect"] != nil {
		t.Errorf("a desktop session: %d %v", code, out)
	}
	// The module failing isn't the client's fault, and isn't a start.
	calls.err = errors.New("the helper fell over")
	if code, _ := play(game.ID, adult, pcsx2); code != 500 {
		t.Errorf("the module failing: %d", code)
	}
	// The list never fails for a game the account can see: it's empty.
	if w := ts.do(t, "GET", fmt.Sprintf("/api/v1/games/%d/emulators", game.ID), adult, nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"emulators":[]`) {
		t.Errorf("the list with the module failing: %d %s", w.Code, w.Body)
	}
	// A game whose file has gone.
	calls.err = nil
	if err := os.Remove(game.Path); err != nil {
		t.Fatal(err)
	}
	n := len(calls.launched())
	if code, _ := play(game.ID, adult, pcsx2); code != 404 || len(calls.launched()) != n {
		t.Errorf("a game with no file: %d", code)
	}
}

func first(code int, _ map[string]any) int { return code }

// TestGameEmulatorsWithoutWittyPlay: a server without the module still answers the list, empty.
func TestGameEmulatorsWithoutWittyPlay(t *testing.T) {
	fakeWittyPlayCalls(t)
	service.Running = func(string) bool { return false }
	ts := newTestServer(t)
	game, _ := ps2Game(t, ts, "Game", "")
	_, adult := ts.user(t, "grown", auth.KindAdult, false, "")
	w := ts.do(t, "GET", fmt.Sprintf("/api/v1/games/%d/emulators", game.ID), adult, nil)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"emulators":[]}` {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}
