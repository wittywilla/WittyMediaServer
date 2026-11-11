// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/parental"
)

// testClock is a clock a test turns.
type testClock struct{ t time.Time }

func (c *testClock) now() time.Time         { return c.t }
func (c *testClock) add(d time.Duration)    { c.t = c.t.Add(d) }
func (c *testClock) set(day int, at string) { c.t = mustTime(day, at) }
func mustTime(day int, at string) time.Time { // day 2 is Monday 2 November 2026
	t, err := time.ParseInLocation("2006-01-02 15:04", fmt.Sprintf("2026-11-%02d %s", day, at), time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func parentalClock(ts *testServer, day int, at string) *testClock {
	c := &testClock{}
	c.set(day, at)
	ts.parental.now = c.now
	return c
}

func readBody(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
}

// A film that can really be sent, so playing it is more than a refusal.
func playableFilm(t *testing.T, ts *testServer, rating string) *library.Item {
	t.Helper()
	root := t.TempDir()
	lib := ts.library(t, "movies", root)
	path := filepath.Join(root, "film.mp4")
	if err := os.WriteFile(path, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	return ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: path, Title: "Film", ContentRating: rating, Container: "mp4"})
}

func TestScreenTimeAllowanceStopsPlaying(t *testing.T) {
	ts := newTestServer(t)
	film := playableFilm(t, ts, "G")
	ts.user(t, "boss", "adult", true, "")
	kidUser, kid := ts.user(t, "kid", "kids", false, `{"maxAge":7,"time":{"schoolDayMinutes":1,"weekendMinutes":120}}`)
	_, free := ts.user(t, "sis", "kids", false, "")
	clock := parentalClock(ts, 2, "16:00") // a Monday
	file := fmt.Sprintf("/api/items/%d/file", film.ID)
	progress := fmt.Sprintf("/api/items/%d/progress", film.ID)

	if w := ts.do(t, "GET", file, kid, nil); w.Code != 200 || w.Header().Get("X-WMS-Screen-Time-Left") != "60" {
		t.Fatalf("with time left: %d %q %s", w.Code, w.Header().Get("X-WMS-Screen-Time-Left"), w.Body)
	}
	// Watching: a report every ten seconds for a minute.
	for i := 0; i < 6; i++ {
		clock.add(10 * time.Second)
		if w := ts.do(t, "POST", progress, kid, map[string]any{"positionSec": 10 * (i + 1), "durationSec": 3000}); w.Code != 200 {
			t.Fatalf("report %d: %d %s", i, w.Code, w.Body)
		}
	}
	var st struct {
		Allowed      bool   `json:"allowed"`
		Reason       string `json:"reason"`
		UsedSec      int    `json:"usedSec"`
		RemainingSec int    `json:"remainingSec"`
		Message      string `json:"message"`
		NextAt       int64  `json:"nextAt"`
	}
	readBody(t, ts.do(t, "GET", "/api/me/screen-time", kid, nil).Body.Bytes(), &st)
	if st.Allowed || st.Reason != parental.ReasonAllowance || st.UsedSec != 60 || st.Message == "" || st.NextAt == 0 {
		t.Fatalf("after a minute: %+v", st)
	}
	// Everything that plays is refused, politely and in a way an app can tell apart.
	for _, target := range []string{file, fmt.Sprintf("/api/items/%d/play", film.ID), fmt.Sprintf("/api/items/%d/stream", film.ID), "/api/hls/x/master.m3u8"} {
		w := ts.do(t, "GET", target, kid, nil)
		var refusal struct {
			Error, Code string
			ScreenTime  struct{ Reason string } `json:"screenTime"`
		}
		readBody(t, w.Body.Bytes(), &refusal)
		if w.Code != 403 || refusal.Code != "screen-time" || refusal.Error == "" || refusal.ScreenTime.Reason != parental.ReasonAllowance {
			t.Errorf("%s when time is up: %d %s", target, w.Code, w.Body)
		}
	}
	// Where they got to is still saved, and stopping still works.
	if w := ts.do(t, "POST", progress, kid, map[string]any{"positionSec": 70, "durationSec": 3000}); w.Code != 200 {
		t.Errorf("a last report: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "DELETE", "/api/hls/x", kid, nil); w.Code == 403 {
		t.Errorf("stopping a stream: %d %s", w.Code, w.Body)
	}
	// Looking around isn't playing.
	if w := ts.do(t, "GET", "/api/items", kid, nil); w.Code != 200 {
		t.Errorf("browsing: %d", w.Code)
	}
	// Clearing their own history doesn't hand the time back.
	if w := ts.do(t, "DELETE", "/api/me/history", kid, nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", file, kid, nil); w.Code != 403 {
		t.Errorf("after clearing their history: %d", w.Code)
	}
	// Another profile's time is its own.
	if w := ts.do(t, "GET", file, free, nil); w.Code != 200 {
		t.Errorf("a profile with no time rules: %d %s", w.Code, w.Body)
	}
	// Tomorrow is a new day. (Tuesday, still a school day: one minute.)
	clock.set(3, "16:00")
	if w := ts.do(t, "GET", file, kid, nil); w.Code != 200 {
		t.Errorf("the next day: %d %s", w.Code, w.Body)
	}
	// And the weekend has its own allowance.
	clock.set(7, "10:00")
	readBody(t, ts.do(t, "GET", "/api/me/screen-time", kid, nil).Body.Bytes(), &st)
	if !st.Allowed || st.RemainingSec != 7200 {
		t.Errorf("Saturday: %+v", st)
	}
	var written int
	if err := ts.DB.QueryRow(`SELECT seconds FROM screen_time WHERE user_id = ? AND day = 20261102`, kidUser.ID).Scan(&written); err != nil || written != 60 {
		t.Errorf("Monday as written down: %d %v", written, err)
	}
}

func TestBedtimeAndAskingAParent(t *testing.T) {
	ts := newTestServer(t)
	film := playableFilm(t, ts, "G")
	_, admin := ts.user(t, "boss", "adult", true, "")
	ts.user(t, "mum", "adult", false, "")
	ts.user(t, "teen", "adult", false, `{"maxAge":13}`)
	kidUser, kid := ts.user(t, "kid", "kids", false, `{"maxAge":7,"time":{"from":"07:00","schoolNightUntil":"20:00","weekendUntil":"21:30"}}`)
	clock := parentalClock(ts, 2, "19:59")
	file := fmt.Sprintf("/api/items/%d/file", film.ID)
	if w := ts.do(t, "GET", file, kid, nil); w.Code != 200 || w.Header().Get("X-WMS-Screen-Time-Left") != "60" {
		t.Fatalf("a minute before bedtime: %d %q", w.Code, w.Header().Get("X-WMS-Screen-Time-Left"))
	}
	clock.set(2, "20:00")
	if w := ts.do(t, "GET", file, kid, nil); w.Code != 403 || !strings.Contains(w.Body.String(), `"reason":"bedtime"`) {
		t.Fatalf("at bedtime: %d %s", w.Code, w.Body)
	}
	clock.set(3, "06:30")
	if w := ts.do(t, "GET", file, kid, nil); w.Code != 403 || !strings.Contains(w.Body.String(), `"reason":"early"`) {
		t.Errorf("before the day starts: %d %s", w.Code, w.Body)
	}
	clock.set(6, "21:00") // Friday isn't a school night
	if w := ts.do(t, "GET", file, kid, nil); w.Code != 200 {
		t.Errorf("Friday at nine: %d %s", w.Code, w.Body)
	}

	// Asking a parent, on the child's device.
	clock.set(2, "20:05")
	extra := "/api/me/screen-time/extra"
	if w := ts.do(t, "POST", extra, kid, map[string]any{"minutes": 30, "pin": "1234"}); w.Code != 403 {
		t.Errorf("a PIN when none is set: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", "/api/parental/pin", admin, map[string]any{"pin": "4821", "current": "wrong password"}); w.Code != 403 {
		t.Errorf("setting the PIN without the administrator's password: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/parental/pin", admin, map[string]any{"pin": "12", "current": "correct-horse-1"}); w.Code != 400 {
		t.Errorf("a two-digit PIN: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/parental/pin", kid, map[string]any{"pin": "4821"}); w.Code != 403 {
		t.Errorf("a Kids profile set the PIN: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/parental/pin", admin, map[string]any{"pin": "4821", "current": "correct-horse-1"}); w.Code != 200 {
		t.Fatalf("setting the PIN: %d %s", w.Code, w.Body)
	}
	if stored := ts.DB.GetSetting(context.Background(), parentPINKey, ""); stored == "" || strings.Contains(stored, "4821") {
		t.Errorf("the PIN is kept as %q", stored)
	}
	if w := ts.do(t, "GET", "/api/settings", admin, nil); strings.Contains(w.Body.String(), "parentalPin") {
		t.Errorf("the settings page holds the PIN: %s", w.Body)
	}
	if w := ts.do(t, "POST", extra, kid, map[string]any{"minutes": 30, "pin": "0000"}); w.Code != 403 {
		t.Errorf("a wrong PIN: %d %s", w.Code, w.Body)
	}
	for _, bad := range []map[string]any{{"minutes": 0, "pin": "4821"}, {"minutes": 241, "pin": "4821"}, {"minutes": 30}, {"minutes": 30, "name": "mum"}} {
		if w := ts.do(t, "POST", extra, kid, bad); w.Code != 400 {
			t.Errorf("%v: %d %s", bad, w.Code, w.Body)
		}
	}
	// Someone who doesn't manage this profile can't give it time, however right their password.
	for _, name := range []string{"teen", "kid", "nobody"} {
		if w := ts.do(t, "POST", extra, kid, map[string]any{"minutes": 30, "name": name, "password": "correct-horse-1"}); w.Code != 403 {
			t.Errorf("%s gave extra time: %d %s", name, w.Code, w.Body)
		}
	}
	if w := ts.do(t, "GET", file, kid, nil); w.Code != 403 {
		t.Fatalf("nothing was given yet: %d", w.Code)
	}
	w := ts.do(t, "POST", extra, kid, map[string]any{"minutes": 30, "pin": "4821"})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"allowed":true`) || !strings.Contains(w.Body.String(), `"remainingSec":1800`) {
		t.Fatalf("the right PIN: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", file, kid, nil); w.Code != 200 {
		t.Errorf("after the parent said yes: %d %s", w.Code, w.Body)
	}
	clock.add(31 * time.Minute)
	if w := ts.do(t, "GET", file, kid, nil); w.Code != 403 {
		t.Errorf("when the extra time ran out: %d", w.Code)
	}
	// A parent's own name and password work too.
	if w := ts.do(t, "POST", extra, kid, map[string]any{"minutes": 10, "name": "Mum", "password": "correct-horse-1"}); w.Code != 200 {
		t.Errorf("a parent's password: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", file, kid, nil); w.Code != 200 {
		t.Errorf("after that: %d", w.Code)
	}
	// And from the parent's own device.
	give := fmt.Sprintf("/api/users/%d/screen-time/extra", kidUser.ID)
	if w := ts.do(t, "POST", give, kid, map[string]any{"minutes": 60}); w.Code != 403 {
		t.Errorf("a Kids profile gave itself time: %d", w.Code)
	}
	if w := ts.do(t, "POST", give, admin, map[string]any{"minutes": 60}); w.Code != 200 {
		t.Errorf("the administrator: %d %s", w.Code, w.Body)
	}
	var events int
	if err := ts.DB.QueryRow(`SELECT COUNT(*) FROM profile_events WHERE user_id = ? AND kind = 'extra'`, kidUser.ID).Scan(&events); err != nil || events != 3 {
		t.Errorf("extra time written down %d times, %v", events, err)
	}
}

// Guessing the parents' PIN locks, as guessing a password does.
func TestParentPINLocksAfterWrongTries(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, kid := ts.user(t, "kid", "kids", false, `{"maxAge":7,"time":{"schoolDayMinutes":1,"weekendMinutes":1}}`)
	if w := ts.do(t, "PUT", "/api/parental/pin", admin, map[string]any{"pin": "4821", "current": "correct-horse-1"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	locked := false
	for i := 0; i < 12 && !locked; i++ {
		w := ts.do(t, "POST", "/api/me/screen-time/extra", kid, map[string]any{"minutes": 30, "pin": fmt.Sprintf("%04d", i)})
		switch w.Code {
		case 403:
		case 429:
			locked = true
			if w.Header().Get("Retry-After") == "" {
				t.Error("no Retry-After")
			}
		default:
			t.Fatalf("try %d: %d %s", i, w.Code, w.Body)
		}
		if i >= pairFails+1 && !locked {
			t.Fatalf("still guessing after %d wrong PINs", i+1)
		}
	}
	// The right PIN waits too, and so does another address for the same profile.
	if w := ts.do(t, "POST", "/api/me/screen-time/extra", kid, map[string]any{"minutes": 30, "pin": "4821"}); w.Code != 429 {
		t.Errorf("the right PIN while locked: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/me/screen-time/extra", kid, map[string]any{"minutes": 30, "pin": "4821"}, "RemoteAddr", "192.168.1.77:4000", "Host", "192.168.1.2:9797"); w.Code != 429 {
		t.Errorf("the right PIN from another device while locked: %d %s", w.Code, w.Body)
	}
}

func TestFeaturesTurnedOff(t *testing.T) {
	ts := newTestServer(t)
	film := playableFilm(t, ts, "G")
	games := ts.library(t, "games", t.TempDir())
	game := ts.item(t, &library.Item{LibraryID: games, Kind: "game", Path: "/g/game.nes", Title: "A game", ContentRating: "ESRB E"})
	_, admin := ts.user(t, "boss", "adult", true, "")
	open := `"maxAge":-1,"allowUnrated":true,"liveTv":true,"libraryChannels":true,"tunerChannels":true`
	limited, off := ts.user(t, "limited", "adult", false, `{`+open+`,"off":["games","recordings","downloads","sharing","comments","editProfile"]}`)
	_, on := ts.user(t, "free", "adult", false, "")
	if w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/comments", film.ID), admin, map[string]any{"body": "first"}); w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, c := range []struct {
		feature, method, target string
		body                    any
	}{
		{"games", "GET", "/api/games/consoles", nil},
		{"games", "GET", fmt.Sprintf("/api/games/%d/metadata", game.ID), nil},
		{"games", "POST", fmt.Sprintf("/api/games/%d/play", game.ID), map[string]any{}},
		{"games", "GET", "/api/couch", nil},
		{"games", "GET", "/api/rigs", nil},
		{"games", "GET", "/api/game-profiles", nil},
		{"recordings", "GET", "/api/recordings", nil},
		{"recordings", "POST", "/api/recording-rules", map[string]any{}},
		{"downloads", "GET", fmt.Sprintf("/api/items/%d/original", film.ID), nil},
		{"sharing", "POST", "/api/shares", map[string]any{"kind": "item", "id": film.ID}},
		{"comments", "GET", fmt.Sprintf("/api/items/%d/comments", film.ID), nil},
		{"comments", "POST", fmt.Sprintf("/api/items/%d/comments", film.ID), map[string]any{"body": "hello"}},
	} {
		w := ts.do(t, c.method, c.target, off, c.body)
		var refusal struct{ Error, Code, Feature string }
		_ = json.Unmarshal(w.Body.Bytes(), &refusal)
		if w.Code != 403 || refusal.Code != "feature-off" || refusal.Feature != c.feature || refusal.Error == "" {
			t.Errorf("%s %s with %s off: %d %s", c.method, c.target, c.feature, w.Code, w.Body)
		}
		if w := ts.do(t, c.method, c.target, on, c.body); w.Code == 403 && strings.Contains(w.Body.String(), "feature-off") {
			t.Errorf("%s %s for an account with no limits: %d %s", c.method, c.target, w.Code, w.Body)
		}
	}
	// With games off, games aren't listed or opened either.
	if w := ts.do(t, "GET", fmt.Sprintf("/api/items?library=%d", games), off, nil); strings.Contains(w.Body.String(), "A game") {
		t.Errorf("games listed with games off: %s", w.Body)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d", game.ID), off, nil); w.Code != 404 {
		t.Errorf("a game opened with games off: %d", w.Code)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d", film.ID), off, nil); w.Code != 200 {
		t.Errorf("a film, with games off: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/libraries", off, nil); strings.Contains(w.Body.String(), `"type":"games"`) || !strings.Contains(w.Body.String(), `"type":"movies"`) {
		t.Errorf("libraries, with games off: %s", w.Body)
	}
	if w := ts.do(t, "GET", "/api/libraries", on, nil); !strings.Contains(w.Body.String(), `"type":"games"`) {
		t.Errorf("libraries for an account with no limits: %s", w.Body)
	}
	// Their own name and picture.
	profile := fmt.Sprintf("/api/users/%d/profile", limited.ID)
	for _, body := range []map[string]any{{"displayName": "New name"}, {"avatar": "rocket:blue"}, {"username": "other"}} {
		if w := ts.do(t, "PUT", profile, off, body); w.Code != 403 {
			t.Errorf("changed their own profile (%v) with that turned off: %d %s", body, w.Code, w.Body)
		}
	}
	if w := ts.do(t, "PUT", profile, off, map[string]any{"showUsername": true}); w.Code != 200 {
		t.Errorf("which name shows is anyone's choice: %d %s", w.Code, w.Body)
	}
	// What the profile is told about itself, so an app can hide what is off.
	var mine struct {
		Limited  bool            `json:"limited"`
		Features map[string]bool `json:"features"`
	}
	readBody(t, ts.do(t, "GET", "/api/me/limits", off, nil).Body.Bytes(), &mine)
	if !mine.Limited || mine.Features["games"] || mine.Features["comments"] || mine.Features["writeComments"] || mine.Features["downloads"] || !mine.Features["remote"] || !mine.Features["liveTv"] {
		t.Errorf("%+v", mine)
	}
	readBody(t, ts.do(t, "GET", "/api/me/limits", on, nil).Body.Bytes(), &mine)
	if mine.Limited || !mine.Features["games"] || !mine.Features["writeComments"] {
		t.Errorf("an account with no limits: %+v", mine)
	}
	// The refusals are there for whoever manages the profile.
	var n int
	if err := ts.DB.QueryRow(`SELECT COUNT(*) FROM profile_events WHERE user_id = ? AND kind = 'feature'`, limited.ID).Scan(&n); err != nil || n != 5 {
		t.Errorf("%d kinds of refusal written down, want one for each feature: %v", n, err)
	}
}

// What Kids and Guest profiles could do before these were choices, they still can; and what
// they couldn't, turning a switch on doesn't give them.
func TestKidsKeepWhatTheyHadAndGainNothing(t *testing.T) {
	ts := newTestServer(t)
	film := playableFilm(t, ts, "G")
	_, admin := ts.user(t, "boss", "adult", true, "")
	kidUser, kid := ts.user(t, "kid", "kids", false, "")
	if w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/comments", film.ID), admin, map[string]any{"body": "first"}); w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/comments", film.ID), kid, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "first") {
		t.Errorf("a Kids profile reads comments: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/games/consoles", kid, nil); w.Code != 200 {
		t.Errorf("a Kids profile plays games: %d %s", w.Code, w.Body)
	}
	// An administrator turns everything on for it.
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/users/%d/access", kidUser.ID), admin, map[string]any{"access": map[string]any{"off": []string{}}}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/comments", film.ID), kid, map[string]any{"body": "hello"}); w.Code < 400 {
		t.Errorf("a Kids profile wrote a comment: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/shares", kid, map[string]any{"kind": "item", "id": film.ID}); w.Code < 400 {
		t.Errorf("a Kids profile made a share link: %d %s", w.Code, w.Body)
	}
	// Its own name and picture is the one thing that does open up.
	profile := fmt.Sprintf("/api/users/%d/profile", kidUser.ID)
	if w := ts.do(t, "PUT", profile, kid, map[string]any{"displayName": "Captain", "avatar": "rocket:blue"}); w.Code != 200 || !strings.Contains(w.Body.String(), `"avatar":"rocket:blue"`) {
		t.Errorf("its own name and picture, when allowed: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", profile, kid, map[string]any{"username": "captain"}); w.Code != 403 {
		t.Errorf("a Kids profile changed what it signs in with: %d", w.Code)
	}
	if w := ts.do(t, "PUT", profile, kid, map[string]any{"avatar": "<script>:red"}); w.Code != 400 {
		t.Errorf("a picture that isn't one of the list: %d %s", w.Code, w.Body)
	}
}

func TestLimitsAreCheckedAndMergedWhenSaved(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	kidUser, _ := ts.user(t, "kid", "kids", false, "")
	_, adult := ts.user(t, "mum", "adult", false, "")
	_, teen := ts.user(t, "teen", "adult", false, `{"maxAge":13}`)
	access := fmt.Sprintf("/api/users/%d/access", kidUser.ID)
	put := func(tok string, a map[string]any) (int, userView) {
		w := ts.do(t, "PUT", access, tok, map[string]any{"access": a})
		var v userView
		_ = json.Unmarshal(w.Body.Bytes(), &v)
		return w.Code, v
	}
	for name, bad := range map[string]map[string]any{
		"an age of 99":              {"maxAge": 99},
		"a kind that isn't one":     {"kinds": map[string]any{"photo": map[string]any{"maxAge": 7}}},
		"a kind's age of -5":        {"kinds": map[string]any{"game": map[string]any{"maxAge": -5}}},
		"a feature that isn't one":  {"off": []string{"breathing"}},
		"a bedtime that isn't one":  {"time": map[string]any{"schoolNightUntil": "25:00"}},
		"more minutes than a day":   {"time": map[string]any{"weekendMinutes": 5000}},
		"a title with no number":    {"blockItems": []int64{0}},
		"limits that aren't limits": nil,
	} {
		var body any = map[string]any{"access": bad}
		if bad == nil {
			body = map[string]any{"access": "everything"}
		}
		if w := ts.do(t, "PUT", access, admin, body); w.Code != 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	code, v := put(admin, map[string]any{"maxAge": 10, "kinds": map[string]any{"game": map[string]any{"maxAge": 12, "allowUnrated": true}},
		"off": []string{"downloads"}, "time": map[string]any{"schoolDayMinutes": 60, "schoolNightUntil": "20:00"}, "blockGenres": []string{"Horror", " ", "a|b"}})
	if code != 200 || v.Access == nil || v.Access.MaxAge != 10 || v.Access.Kinds["game"].MaxAge != 12 || v.Access.Can(library.FeatureDownloads) ||
		v.Access.Time == nil || v.Access.Time.SchoolDayMinutes != 60 || len(v.Access.BlockGenres) != 1 || !v.Access.LiveTV {
		t.Fatalf("%d %+v", code, v.Access)
	}
	// An app written before these existed sends the limits it knows. The rest stay.
	code, v = put(admin, map[string]any{"maxAge": 7, "allowUnrated": false, "libraries": []int64{}, "blockGenres": []string{}, "liveTv": true, "libraryChannels": true, "tunerChannels": false})
	if code != 200 || v.Access.MaxAge != 7 || v.Access.Kinds["game"].MaxAge != 12 || v.Access.Can(library.FeatureDownloads) || v.Access.Time == nil || len(v.Access.BlockGenres) != 0 {
		t.Errorf("after an older app saved: %d %+v", code, v.Access)
	}
	// What is sent replaces what was there, whole.
	code, v = put(admin, map[string]any{"kinds": map[string]any{"film": map[string]any{"maxAge": 13}}, "time": nil, "off": []string{}})
	if _, still := v.Access.Kinds["game"]; code != 200 || still || v.Access.Kinds["film"].MaxAge != 13 || v.Access.Time != nil || !v.Access.Can(library.FeatureDownloads) {
		t.Errorf("after replacing: %d %+v", code, v.Access)
	}
	// A collection made from a rule can't be a limit, and one that isn't there can't either.
	bossID := ts.userID(t, admin)
	if _, err := ts.DB.Exec(`INSERT INTO collections(id, owner_id, name, kind, rule, created_at, updated_at) VALUES(7, ?, 'By hand', 'manual', '', 1, 1), (8, ?, 'By rule', 'smart', '{}', 1, 1)`, bossID, bossID); err != nil {
		t.Fatal(err)
	}
	code, v = put(admin, map[string]any{"blockCollections": []int64{7, 8, 99}, "allowCollections": []int64{8}})
	if code != 200 || len(v.Access.BlockCollections) != 1 || v.Access.BlockCollections[0] != 7 || len(v.Access.AllowCollections) != 0 {
		t.Errorf("collections: %d %+v", code, v.Access)
	}
	// Who may: an adult manages Kids profiles; one with limits of their own doesn't.
	if code, _ := put(adult, map[string]any{"maxAge": 7}); code != 200 {
		t.Errorf("an adult: %d", code)
	}
	if code, _ := put(teen, map[string]any{"maxAge": -1}); code != 403 {
		t.Errorf("an adult with limits: %d", code)
	}
	// Any of the newer limits makes an adult one with limits.
	for _, limits := range []string{`{"maxAge":-1,"allowUnrated":true,"liveTv":true,"libraryChannels":true,"tunerChannels":true,"off":["downloads"]}`,
		`{"maxAge":-1,"allowUnrated":true,"liveTv":true,"libraryChannels":true,"tunerChannels":true,"time":{"weekendMinutes":60}}`,
		`{"maxAge":-1,"allowUnrated":true,"liveTv":true,"libraryChannels":true,"tunerChannels":true,"kinds":{"game":{"maxAge":12}}}`} {
		_, tok := ts.user(t, fmt.Sprintf("held%d", len(limits)), "adult", false, limits)
		if code, _ := put(tok, map[string]any{"maxAge": -1}); code != 403 {
			t.Errorf("an adult with %s changed a Kids profile's limits: %d", limits, code)
		}
	}
}

// Limits by collection and by tag reach a title asked for by its number, and follow the
// collection as it changes.
func TestCollectionAndTagLimitsOnOneTitle(t *testing.T) {
	ts := newTestServer(t)
	film := playableFilm(t, ts, "R")
	other := ts.item(t, &library.Item{LibraryID: film.LibraryID, Kind: "video", Path: "/m/other.mp4", Title: "Other", ContentRating: "R"})
	_, admin := ts.user(t, "boss", "adult", true, "")
	bossID := ts.userID(t, admin)
	if _, err := ts.DB.Exec(`INSERT INTO collections(id, owner_id, name, created_at, updated_at) VALUES(7, ?, 'Not for them', 1, 1), (8, ?, 'Approved', 1, 1)`, bossID, bossID); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.DB.Exec(`INSERT INTO collection_items(collection_id, item_id, added_at) VALUES(7, ?, 1), (8, ?, 1)`, film.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.DB.Exec(`INSERT INTO video_meta(item_id, library_id, tags) VALUES(?, ?, 'pranks|loud')`, other.ID, other.LibraryID); err != nil {
		t.Fatal(err)
	}
	open := `"maxAge":-1,"allowUnrated":true,"liveTv":true,"libraryChannels":true,"tunerChannels":true`
	_, blocked := ts.user(t, "blocked", "adult", false, `{`+open+`,"blockCollections":[7]}`)
	_, allowed := ts.user(t, "allowed", "kids", false, `{"maxAge":7,"allowCollections":[8]}`)
	_, tagged := ts.user(t, "tagged", "adult", false, `{`+open+`,"blockTags":["pranks"]}`)
	get := func(tok string, it *library.Item) int {
		return ts.do(t, "GET", fmt.Sprintf("/api/items/%d", it.ID), tok, nil).Code
	}
	file := func(tok string, it *library.Item) int {
		return ts.do(t, "GET", fmt.Sprintf("/api/items/%d/file", it.ID), tok, nil).Code
	}
	if get(blocked, film) != 404 || file(blocked, film) != 404 || get(blocked, other) != 200 {
		t.Errorf("a collection never allowed: %d %d %d", get(blocked, film), file(blocked, film), get(blocked, other))
	}
	if get(allowed, other) != 200 || get(allowed, film) != 404 {
		t.Errorf("a collection always allowed: %d %d", get(allowed, other), get(allowed, film))
	}
	if get(tagged, other) != 404 || get(tagged, film) != 200 {
		t.Errorf("a hidden tag: %d %d", get(tagged, other), get(tagged, film))
	}
	// The collection changes; what was looked up is forgotten when limits are saved, and soon anyway.
	if _, err := ts.DB.Exec(`DELETE FROM collection_items WHERE collection_id = 7`); err != nil {
		t.Fatal(err)
	}
	ts.forgetRules()
	if get(blocked, film) != 200 {
		t.Errorf("after the title left the collection: %d", get(blocked, film))
	}
}

func TestSwitchingProfiles(t *testing.T) {
	ts := newTestServer(t)
	ts.user(t, "boss", "adult", true, "")
	_, mum := ts.user(t, "mum", "adult", false, "")
	kidUser, _ := ts.user(t, "kid", "kids", false, "")
	if _, err := ts.Auth.CreateProfile(context.Background(), "tot", "", "kids", false, ""); err != nil { // no PIN: a tap at home
		t.Fatal(err)
	}
	var house struct {
		Profiles []struct {
			Name        string
			Kind        string
			HasPassword bool
			Current     bool
			Avatar      string
		}
	}
	readBody(t, ts.do(t, "GET", "/api/profiles/household", mum, nil).Body.Bytes(), &house)
	if len(house.Profiles) != 4 {
		t.Fatalf("%+v", house)
	}
	for _, p := range house.Profiles {
		if p.Current != (p.Name == "mum") || (p.Name == "tot") == p.HasPassword {
			t.Errorf("%+v", p)
		}
	}
	// Wrong PIN: refused, and the device is still mum's.
	w := ts.do(t, "POST", "/api/profiles/switch", mum, map[string]any{"name": "kid", "password": "wrong"})
	if w.Code != 403 {
		t.Fatalf("a wrong PIN: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/me", mum, nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"mum"`) {
		t.Fatalf("after a wrong PIN: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/profiles/switch", mum, map[string]any{"name": "mum", "password": "correct-horse-1"}); w.Code != 400 {
		t.Errorf("switching to the profile it is on: %d", w.Code)
	}
	// Right PIN: a new sign-in as the child, and the old one is over.
	w = ts.do(t, "POST", "/api/profiles/switch", mum, map[string]any{"name": "kid", "password": "correct-horse-1", "device": "Living room TV"})
	var in struct {
		Token string
		User  auth.User
	}
	readBody(t, w.Body.Bytes(), &in)
	if w.Code != 200 || in.Token == "" || in.User.ID != kidUser.ID || !strings.Contains(w.Header().Get("Set-Cookie"), auth.CookieName) {
		t.Fatalf("switching: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/me", mum, nil); w.Code != 401 {
		t.Errorf("the session it came from still works: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/me", in.Token, nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"kid"`) {
		t.Errorf("the new session: %d %s", w.Code, w.Body)
	}
	// The child can't go back up without the adult's password, and can tap over to a profile with none.
	if w := ts.do(t, "POST", "/api/profiles/switch", in.Token, map[string]any{"name": "mum", "password": ""}); w.Code != 403 {
		t.Errorf("back to an adult with no password: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/profiles/switch", in.Token, map[string]any{"name": "boss", "password": "guess"}); w.Code != 403 {
		t.Errorf("to the administrator with a guess: %d", w.Code)
	}
	w = ts.do(t, "POST", "/api/profiles/switch", in.Token, map[string]any{"name": "tot", "password": ""})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"tot"`) {
		t.Errorf("a tap to a profile with no PIN, at home: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/profiles/switch", "", map[string]any{"name": "tot", "password": ""}); w.Code != 401 {
		t.Errorf("switching without being signed in: %d", w.Code)
	}
	var sessions int
	if err := ts.DB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id IN (SELECT id FROM users WHERE name IN ('mum', 'kid'))`).Scan(&sessions); err != nil || sessions != 1 {
		// kid's own first sign-in from the test's set-up is the one left
		t.Errorf("%d sessions left for the profiles this device left, want only the one it never had: %v", sessions, err)
	}
}

// Guessing a profile's PIN through switching locks like guessing it at the sign-in screen.
func TestSwitchingCountsWrongTries(t *testing.T) {
	ts := newTestServer(t)
	_, kid := ts.user(t, "kid", "kids", false, "")
	ts.user(t, "mum", "adult", false, "")
	locked := false
	for i := 0; i < 10 && !locked; i++ {
		w := ts.do(t, "POST", "/api/profiles/switch", kid, map[string]any{"name": "mum", "password": fmt.Sprintf("guess-%d", i)})
		if w.Code == 429 {
			locked = true
		} else if w.Code != 403 {
			t.Fatalf("try %d: %d %s", i, w.Code, w.Body)
		}
	}
	if !locked {
		t.Fatal("ten wrong passwords and still guessing")
	}
	if w := ts.do(t, "POST", "/api/profiles/switch", kid, map[string]any{"name": "mum", "password": "correct-horse-1"}); w.Code != 429 {
		t.Errorf("the right password while locked: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/me", kid, nil); w.Code != 200 {
		t.Errorf("the device lost its own sign-in: %d", w.Code)
	}
}

// A profile kept to the home network isn't signed in anywhere else, and can't sign in there.
func TestProfileKeptAtHome(t *testing.T) {
	s, h := remoteServer(t, nil)
	admin := adminToken(t, s, h)
	putRemote(t, h, admin, map[string]any{"enabled": true})
	ctx := context.Background()
	open := `"maxAge":-1,"allowUnrated":true,"liveTv":true,"libraryChannels":true,"tunerChannels":true`
	if _, err := s.Auth.CreateProfile(ctx, "homebody", pass, "adult", false, `{`+open+`,"off":["remote"]}`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Auth.CreateProfile(ctx, "roamer", pass, "adult", false, `{`+open+`,"off":["downloads"]}`); err != nil {
		t.Fatal(err)
	}
	login := func(from, name string) reply {
		return call(h, "POST", "/api/login", from, map[string]string{"name": name, "password": pass})
	}
	var in struct{ Token string }
	r := login(lan, "homebody")
	r.json(t, &in)
	if r.code != 200 || in.Token == "" {
		t.Fatalf("at home: %d %s", r.code, r.body)
	}
	if r := call(h, "GET", "/api/me", lan, nil, "Authorization: Bearer "+in.Token); r.code != 200 {
		t.Errorf("at home: %d", r.code)
	}
	if r := call(h, "GET", "/api/me", tail, nil, "Authorization: Bearer "+in.Token); r.code != 401 {
		t.Errorf("the same sign-in away from home: %d %s", r.code, r.body)
	}
	if r := login(tail, "homebody"); r.code != 403 || !strings.Contains(r.body, "only be used at home") {
		t.Errorf("signing in away from home: %d %s", r.code, r.body)
	}
	var left int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = (SELECT id FROM users WHERE name = 'homebody')`).Scan(&left); err != nil || left != 1 {
		t.Errorf("%d sessions, want only the one made at home: %v", left, err)
	}
	r = login(tail, "roamer")
	r.json(t, &in)
	if r.code != 200 {
		t.Fatalf("a profile that may roam: %d %s", r.code, r.body)
	}
	// Switching to the homebody from away is refused, and the roamer stays signed in.
	if r := call(h, "POST", "/api/profiles/switch", tail, map[string]string{"name": "homebody", "password": pass}, "Authorization: Bearer "+in.Token); r.code != 403 {
		t.Errorf("switching to it away from home: %d %s", r.code, r.body)
	}
	if r := call(h, "GET", "/api/me", tail, nil, "Authorization: Bearer "+in.Token); r.code != 200 {
		t.Errorf("the roamer after that: %d", r.code)
	}
}

func TestActivityReport(t *testing.T) {
	ts := newTestServer(t)
	film := playableFilm(t, ts, "G")
	hidden := ts.item(t, &library.Item{LibraryID: film.LibraryID, Kind: "video", Path: "/m/hidden.mp4", Title: "Grown-up film", ContentRating: "R"})
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, mum := ts.user(t, "mum", "adult", false, "")
	mumUser, _ := ts.Auth.User(context.Background(), ts.userID(t, mum))
	kidUser, kid := ts.user(t, "kid", "kids", false, `{"maxAge":7,"time":{"schoolDayMinutes":120,"weekendMinutes":120}}`)
	otherKid, sis := ts.user(t, "sis", "kids", false, "")
	clock := parentalClock(ts, 2, "16:00")
	progress := fmt.Sprintf("/api/items/%d/progress", film.ID)
	for i := 1; i <= 6; i++ {
		clock.add(10 * time.Second)
		if w := ts.do(t, "POST", progress, kid, map[string]any{"positionSec": 10 * i, "durationSec": 3000}); w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	// Asking for what is hidden, more than once.
	for i := 0; i < 3; i++ {
		if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d", hidden.ID), kid, nil); w.Code != 404 {
			t.Fatalf("%d", w.Code)
		}
	}
	var logged float64
	if err := ts.DB.QueryRow(`SELECT seconds FROM play_log WHERE user_id = ? AND item_id = ?`, kidUser.ID, film.ID).Scan(&logged); err != nil || logged != 60 {
		t.Fatalf("the profile's own history has %v seconds, want 60: %v", logged, err)
	}
	// The child clears its own history. What the parents see stays.
	if w := ts.do(t, "DELETE", "/api/me/history", kid, nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	report := fmt.Sprintf("/api/users/%d/activity", kidUser.ID)
	var out struct {
		Days    []struct{ Day, ScreenSec, ExtraSec int }
		Watched []struct {
			Day     int
			ItemID  int64
			Title   string
			Seconds float64
		}
		Events []struct {
			Kind, Title, Detail string
			ItemID              int64
		}
	}
	w := ts.do(t, "GET", report, mum, nil)
	readBody(t, w.Body.Bytes(), &out)
	if w.Code != 200 || len(out.Days) != 1 || out.Days[0].Day != 20261102 || out.Days[0].ScreenSec != 50 { // the first report starts the count
		t.Errorf("screen time by day: %d %+v", w.Code, out.Days)
	}
	if len(out.Watched) != 1 || out.Watched[0].Title != "Film" || out.Watched[0].ItemID != film.ID || out.Watched[0].Seconds != 60 {
		t.Errorf("what was watched: %+v", out.Watched)
	}
	if len(out.Events) != 1 || out.Events[0].Kind != "blocked" || out.Events[0].Title != "Grown-up film" || out.Events[0].Detail != "R" || out.Events[0].ItemID != hidden.ID {
		t.Errorf("what was refused, written once: %+v", out.Events)
	}
	// Who may read it: whoever manages the profile, and nobody else. Not the child, not a sibling,
	// and nobody may read an adult's or an administrator's.
	for name, c := range map[string]struct {
		tok, target string
		want        int
	}{
		"the administrator":            {admin, report, 200},
		"the child itself":             {kid, report, 403},
		"a sibling":                    {sis, report, 403},
		"an adult about another adult": {admin, fmt.Sprintf("/api/users/%d/activity", ts.userID(t, admin)), 403},
		"an adult about themselves":    {mum, fmt.Sprintf("/api/users/%d/activity", mumUser.ID), 403},
		"nobody at all":                {"", report, 401},
		"a profile that isn't there":   {admin, "/api/users/9999/activity", 404},
	} {
		if w := ts.do(t, "GET", c.target, c.tok, nil); w.Code != c.want {
			t.Errorf("%s: %d, want %d", name, w.Code, c.want)
		}
		if c.want != 200 {
			if w := ts.do(t, "DELETE", c.target, c.tok, nil); w.Code != c.want {
				t.Errorf("%s, clearing: %d, want %d", name, w.Code, c.want)
			}
		}
	}
	// An account nobody limited leaves no such record: its history is its own.
	if w := ts.do(t, "POST", progress, mum, map[string]any{"positionSec": 30, "durationSec": 3000}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var rows int
	if err := ts.DB.QueryRow(`SELECT (SELECT COUNT(*) FROM profile_watch WHERE user_id = ?) + (SELECT COUNT(*) FROM screen_time WHERE user_id = ?)`, mumUser.ID, mumUser.ID).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("%d rows kept about an adult with no limits: %v", rows, err)
	}
	// Clearing it keeps today's screen time, so it isn't a way to hand time back.
	if w := ts.do(t, "DELETE", report, mum, nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	readBody(t, ts.do(t, "GET", report, mum, nil).Body.Bytes(), &out)
	if len(out.Watched) != 0 || len(out.Events) != 0 || len(out.Days) != 1 {
		t.Errorf("after clearing: %+v", out)
	}
	_ = otherKid
}

// Share links a profile made stop working when it may no longer share.
func TestShareLinksStopWhenSharingIsTurnedOff(t *testing.T) {
	ts := newTestServer(t)
	root := t.TempDir()
	lib := ts.library(t, "photos", root)
	path := filepath.Join(root, "p.png")
	if err := os.WriteFile(path, logoPicture(t), 0o644); err != nil {
		t.Fatal(err)
	}
	photo := ts.item(t, &library.Item{LibraryID: lib, Kind: "photo", Path: path, Title: "p", ContentRating: "G"})
	_, admin := ts.user(t, "boss", "adult", true, "")
	sharer, tok := ts.user(t, "sharer", "adult", false, "")
	w := ts.do(t, "POST", "/api/shares", tok, map[string]any{"kind": "item", "id": photo.ID})
	var link struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &link)
	if w.Code >= 300 || link.Token == "" {
		t.Fatalf("making a share link: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/share/"+link.Token, "", nil); w.Code != 200 {
		t.Fatalf("the link, while sharing is on: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/users/%d/access", sharer.ID), admin, map[string]any{"access": map[string]any{"off": []string{"sharing"}}}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/share/"+link.Token, "", nil); w.Code == 200 {
		t.Errorf("the link still opens after sharing was turned off: %s", w.Body)
	}
}

func TestRatingSystemsRoute(t *testing.T) {
	ts := newTestServer(t)
	_, kid := ts.user(t, "kid", "kids", false, "")
	w := ts.do(t, "GET", "/api/v1/parental/ratings", kid, nil)
	var out struct {
		Kinds   []string
		Systems []struct {
			ID      string
			Ratings []struct {
				Label string
				Age   int
			}
		}
		Features, Avatars []string
	}
	readBody(t, w.Body.Bytes(), &out)
	if w.Code != 200 || len(out.Kinds) != 5 || len(out.Systems) < 8 || len(out.Features) != len(library.Features) || len(out.Avatars) == 0 {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/parental/ratings", "", nil); w.Code != 401 {
		t.Errorf("without signing in: %d", w.Code)
	}
	for _, f := range []string{"profiles", "profile-switching", "parental-controls", "screen-time"} {
		found := false
		for _, have := range serverFeatures {
			found = found || have == f
		}
		if !found {
			t.Errorf("%s isn't among the server's features", f)
		}
	}
}

// A long answer is cut when the profile's time runs out while it is being sent.
func TestLongAnswerStopsWhenTimeRunsOut(t *testing.T) {
	ts := newTestServer(t)
	kidUser, _ := ts.user(t, "kid", "kids", false, "")
	clock := parentalClock(ts, 2, "16:00")
	rules := &parental.TimeRules{SchoolDayMinutes: 1}
	pw := &playWatch{s: ts.Server, userID: kidUser.ID, rules: rules, cancel: func() {}}
	pw.until.Store(clock.now().Add(time.Minute).UnixNano())
	rec := &discardWriter{}
	w := &timedWriter{ResponseWriter: rec, watch: pw}
	if _, err := w.Write([]byte("abc")); err != nil {
		t.Fatalf("with time left: %v", err)
	}
	// A minute of sending counts as a minute of watching, and then it stops.
	for i := 0; i < 12; i++ {
		clock.add(6 * time.Second)
		if _, err := w.Write([]byte("abc")); err != nil {
			if i < 8 {
				t.Fatalf("stopped after %d seconds: %v", 6*(i+1), err)
			}
			break
		}
		if i == 11 {
			t.Fatal("still sending after the minute was up")
		}
	}
	if _, err := w.Write([]byte("abc")); err == nil {
		t.Error("it started again by itself")
	}
	// Unless a parent gave more in the meantime.
	if err := ts.grantExtra(context.Background(), kidUser.ID, 10, "test"); err != nil {
		t.Fatal(err)
	}
	pw2 := &playWatch{s: ts.Server, userID: kidUser.ID, rules: rules, cancel: func() {}}
	pw2.until.Store(clock.now().Add(-time.Second).UnixNano())
	w2 := &timedWriter{ResponseWriter: rec, watch: pw2}
	if _, err := w2.Write([]byte("abc")); err != nil {
		t.Errorf("after extra time was given: %v", err)
	}
}

type discardWriter struct {
	n int
	h http.Header
}

func (c *discardWriter) Header() http.Header         { return c.h }
func (c *discardWriter) Write(b []byte) (int, error) { c.n += len(b); return len(b), nil }
func (c *discardWriter) WriteHeader(int)             {}
