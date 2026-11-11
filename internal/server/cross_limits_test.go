// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// Parental controls where they meet the features that were written beside them: server-driven
// screens, remote control between devices, and watch parties. Each of those was first checked
// against limits by library and by age; these tests hold them to every kind of limit a profile
// can have, to the switches that turn a feature off for a profile, and to screen time.

// limitsWorld is a house with one title for each kind of limit to hide, and a profile for
// each kind of limit that hides exactly that title and nothing else.
type limitsWorld struct {
	*testServer
	srv          *httptest.Server
	admin        string
	parent       string
	parentID     int64
	fine         *library.Item // a film for everyone, in the genre "Family"
	hidden       map[string]*library.Item
	games, films int64 // libraries
	game         *library.Item
	collection   int64
	noLimits     string // limits that hide nothing, without the braces
}

// The profiles' names are the kinds of limit, and the keys of hidden.
var limitKinds = []string{"a rating for films only", "only some genres", "a hidden tag", "a collection never allowed", "games turned off"}

func newLimitsWorld(t *testing.T) *limitsWorld {
	t.Helper()
	ts := newTestServer(t)
	root := t.TempDir()
	w := &limitsWorld{testServer: ts, hidden: map[string]*library.Item{}}
	w.noLimits = `"maxAge":-1,"allowUnrated":true,"liveTv":true,"libraryChannels":true,"tunerChannels":true`
	w.films = ts.library(t, "movies", filepath.Join(root, "films"))
	w.games = ts.library(t, "games", filepath.Join(root, "games"))
	film := func(title, rating, genre string) *library.Item {
		p := filepath.Join(root, "films", title+".mp4")
		put(t, p, make([]byte, 2048))
		return ts.item(t, &library.Item{LibraryID: w.films, Kind: "video", Path: p, Title: title, ContentRating: rating, Genres: []string{genre}, Container: "mp4", RuntimeSec: 6000, Poster: "cache:x.jpg"})
	}
	w.fine = film("Puddle Day", "G", "Family")
	w.hidden["a rating for films only"] = film("Night Shift Zebra", "R", "Family")
	w.hidden["only some genres"] = film("Courtroom Zebra", "G", "Drama")
	w.hidden["a hidden tag"] = film("Prank Zebra", "G", "Family")
	w.hidden["a collection never allowed"] = film("Shelved Zebra", "G", "Family")
	gp := filepath.Join(root, "games", "Kart Zebra.nes")
	put(t, gp, []byte("rom"))
	w.game = ts.item(t, &library.Item{LibraryID: w.games, Kind: "game", Path: gp, Title: "Kart Zebra", Console: "nes", ContentRating: "ESRB E", Genres: []string{"Family"}})
	w.hidden["games turned off"] = w.game
	_, w.admin = ts.user(t, "boss", "adult", true, "")
	bossID := ts.userID(t, w.admin)
	if _, err := ts.DB.Exec(`INSERT INTO video_meta(item_id, library_id, tags) VALUES(?, ?, 'pranks|loud')`, w.hidden["a hidden tag"].ID, w.films); err != nil {
		t.Fatal(err)
	}
	res, err := ts.DB.Exec(`INSERT INTO collections(owner_id, name, created_at, updated_at) VALUES(?, 'Not for them', 1, 1)`, bossID)
	if err != nil {
		t.Fatal(err)
	}
	w.collection, _ = res.LastInsertId()
	if _, err := ts.DB.Exec(`INSERT INTO collection_items(collection_id, item_id, added_at) VALUES(?, ?, 1)`, w.collection, w.hidden["a collection never allowed"].ID); err != nil {
		t.Fatal(err)
	}
	p, ptok := ts.user(t, "parent", "adult", false, "")
	w.parent, w.parentID = ptok, p.ID
	w.srv = httptest.NewServer(ts.h)
	t.Cleanup(w.srv.Close)
	return w
}

// limits is the limits of the profile for one kind of limit.
func (w *limitsWorld) limits(kind string) string {
	switch kind {
	case "a rating for films only":
		return `{` + w.noLimits + `,"kinds":{"film":{"maxAge":7,"allowUnrated":false}}}`
	case "only some genres":
		return `{` + w.noLimits + `,"onlyGenres":["Family"]}`
	case "a hidden tag":
		return `{` + w.noLimits + `,"blockTags":["pranks"]}`
	case "a collection never allowed":
		return fmt.Sprintf(`{%s,"blockCollections":[%d]}`, w.noLimits, w.collection)
	case "games turned off":
		return `{` + w.noLimits + `,"off":["games"]}`
	}
	panic(kind)
}

// profiles makes the profile for each kind of limit, and answers their tokens and ids.
func (w *limitsWorld) profiles(t *testing.T) (toks map[string]string, ids map[string]int64) {
	t.Helper()
	toks, ids = map[string]string{}, map[string]int64{}
	for i, kind := range limitKinds {
		u, tok := w.user(t, fmt.Sprintf("limited%d", i), "adult", false, w.limits(kind))
		toks[kind], ids[kind] = tok, u.ID
	}
	return toks, ids
}

// setLimits saves a profile's limits as the administrator does.
func (ts *testServer) setLimits(t *testing.T, admin string, id int64, access string) {
	t.Helper()
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/users/%d/access", id), admin, []byte(`{"access":`+access+`}`)); w.Code != 200 {
		t.Fatalf("saving limits: %d %s", w.Code, w.Body)
	}
}

// spend has a profile watch something for d (at most a minute and a half at a time).
func spend(t *testing.T, ts *testServer, clock *testClock, tok string, item int64, d time.Duration) {
	t.Helper()
	progress := fmt.Sprintf("/api/items/%d/progress", item)
	if w := ts.do(t, "POST", progress, tok, map[string]any{"positionSec": 1, "durationSec": 3000}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	clock.add(d)
	if w := ts.do(t, "POST", progress, tok, map[string]any{"positionSec": 60, "durationSec": 3000}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

// oneMinuteADay are limits by age with a minute of screen time on school days.
const oneMinuteADay = `{"maxAge":7,"liveTv":true,"libraryChannels":true,"time":{"schoolDayMinutes":1,"weekendMinutes":120}}`

// ---------------------------------------------------------------------------
// Server-driven screens
// ---------------------------------------------------------------------------

// Every stock screen, Top Shelf and a diff, for a profile of each kind of limit: the one
// title that limit hides is never in it, the title everyone may see is, and a hidden title
// or library asked for by its number isn't found.
func TestSDUIScreensFollowEveryKindOfLimit(t *testing.T) {
	w := newLimitsWorld(t)
	toks, _ := w.profiles(t)
	screens := []string{
		"/api/sdui/screens/home", "/api/sdui/screens/home?formFactor=tv", "/api/sdui/screens/home?formFactor=phone", "/api/sdui/screens/libraries",
		"/api/sdui/screens/search?q=Zebra", fmt.Sprintf("/api/sdui/screens/library?library=%d", w.films), fmt.Sprintf("/api/sdui/screens/library?library=%d", w.games),
		"/api/sdui/screens/livetv", "/api/sdui/screens/music", "/api/sdui/screens/settings", "/api/sdui/screens/player", "/api/sdui/topshelf",
	}
	// The owner sees them all, or the test would prove nothing.
	seen := ""
	for _, p := range screens {
		res := w.do(t, "GET", p, w.admin, nil)
		if res.Code != 200 {
			t.Fatalf("%s as the owner: %d %s", p, res.Code, res.Body)
		}
		seen += res.Body.String()
	}
	for kind, it := range w.hidden {
		if !strings.Contains(seen, it.Title) {
			t.Fatalf("the owner's screens never show %q, so its absence from a profile's would prove nothing", it.Title)
		}
		if res := w.do(t, "GET", fmt.Sprintf("/api/sdui/screens/detail?item=%d", it.ID), w.admin, nil); res.Code != 200 {
			t.Fatalf("%s: the owner's page for it: %d", kind, res.Code)
		}
	}
	for _, kind := range limitKinds {
		tok, hidden := toks[kind], w.hidden[kind]
		for _, p := range screens {
			res := w.do(t, "GET", p, tok, nil)
			gamesLibrary := strings.HasSuffix(p, fmt.Sprintf("library=%d", w.games))
			if kind == "games turned off" && gamesLibrary {
				if res.Code != 404 || strings.Contains(res.Body.String(), "Zebra") {
					t.Errorf("%s: the games library's screen: %d %s", kind, res.Code, res.Body)
				}
				continue
			}
			if res.Code != 200 {
				t.Errorf("%s: %s: %d %s", kind, p, res.Code, res.Body)
				continue
			}
			if strings.Contains(res.Body.String(), hidden.Title) {
				t.Errorf("%s: %s shows %q", kind, p, hidden.Title)
			}
			if (strings.Contains(p, "home") || strings.Contains(p, "topshelf")) && !strings.Contains(res.Body.String(), w.fine.Title) {
				t.Errorf("%s: %s is missing the title everyone may see", kind, p)
			}
			// What changed since an earlier render is read through the same limits.
			if etag := res.Header().Get("ETag"); etag != "" && strings.Contains(p, "/screens/") {
				path, query, _ := strings.Cut(p, "?")
				d := w.do(t, "GET", path+"/diff?since=W%2F%22none%22&"+query, tok, nil)
				if d.Code != 200 || strings.Contains(d.Body.String(), hidden.Title) {
					t.Errorf("%s: the diff of %s: %d, or it shows what is hidden", kind, p, d.Code)
				}
			}
		}
		if res := w.do(t, "GET", fmt.Sprintf("/api/sdui/screens/detail?item=%d", hidden.ID), tok, nil); res.Code != 404 || strings.Contains(res.Body.String(), "Zebra") {
			t.Errorf("%s: the page of the hidden title: %d %s", kind, res.Code, res.Body)
		}
		if res := w.do(t, "GET", fmt.Sprintf("/api/sdui/screens/detail?item=%d", w.fine.ID), tok, nil); res.Code != 200 {
			t.Errorf("%s: the page of a title it may see: %d", kind, res.Code)
		}
		// Another profile's hidden title is this one's to see: each limit hides only its own.
		for other, it := range w.hidden {
			if other == kind {
				continue
			}
			if res := w.do(t, "GET", fmt.Sprintf("/api/sdui/screens/detail?item=%d", it.ID), tok, nil); res.Code != 200 {
				t.Errorf("%s hides %q as well: %d", kind, it.Title, res.Code)
			}
		}
	}
	// With games off, the games library isn't among the libraries.
	if res := w.do(t, "GET", "/api/sdui/screens/libraries", toks["games turned off"], nil); strings.Contains(res.Body.String(), fmt.Sprintf(`"lib%d"`, w.games)) {
		t.Errorf("the games library is listed for a profile with games off: %s", res.Body)
	}
}

// What a profile has been kept from isn't in a screen's conditions: a layout that shows a
// tab or a button "when the server has this" doesn't show it to them, whatever the client
// says of itself.
func TestSDUIConditionsFollowTheProfile(t *testing.T) {
	ts := newTestServer(t)
	film := playableFilm(t, ts, "G")
	ts.user(t, "boss", "adult", true, "")
	all := `"maxAge":-1,"allowUnrated":true,"libraryChannels":true,"tunerChannels":true`
	conditions := func(kind, limits, screen string) (sdui.RenderContext, error) {
		t.Helper()
		u, _ := ts.user(t, "p"+fmt.Sprint(time.Now().UnixNano()), kind, false, limits)
		r := httptest.NewRequest("GET", "/api/sdui/screens/"+screen, nil)
		r = r.WithContext(context.WithValue(r.Context(), userKey, u))
		ctx := sdui.RenderContext{Flags: map[string]bool{"screenTimeUp": true}, Data: map[string]any{},
			Features: append(append([]string(nil), serverFeatures...), "history", "playback"),
			Modules:  []string{"wittyplay", "wittytv", "wittycouch", "wittydvd"}}
		_, err := ts.sduiLimits(r, screen, &ctx)
		return ctx, err
	}
	for _, c := range []struct {
		name, limits      string
		noFeatures, noMod []string
		stillFeatures     []string
		stillMod          []string
	}{
		{"no limits at all", "", nil, nil, []string{"syncplay", "remote-protocol", "recordings", "live-tv", "game-settings", "playback"}, []string{"wittyplay", "wittytv"}},
		{"games off", `{` + all + `,"liveTv":true,"off":["games"]}`, []string{"game-settings", "game-rigs", "standalone-emulators", "game-metadata"}, []string{"wittyplay", "wittycouch"}, []string{"live-tv", "syncplay", "playback"}, []string{"wittytv", "wittydvd"}},
		{"Live TV off", `{` + all + `,"liveTv":false}`, []string{"live-tv", "tv-favourites", "tv-reminders", "recordings"}, []string{"wittytv"}, []string{"game-settings", "playback"}, []string{"wittyplay"}},
		{"recordings off", `{` + all + `,"liveTv":true,"off":["recordings"]}`, []string{"recordings"}, nil, []string{"live-tv"}, []string{"wittytv"}},
		{"watch parties off", `{` + all + `,"liveTv":true,"off":["syncplay"]}`, []string{"syncplay"}, nil, []string{"remote-protocol", "playback"}, nil},
		{"playing on other devices off", `{` + all + `,"liveTv":true,"off":["remoteControl"]}`, []string{"remote-protocol"}, nil, []string{"syncplay", "playback"}, nil},
	} {
		ctx, err := conditions("adult", c.limits, "home")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for _, f := range c.noFeatures {
			if slices.Contains(ctx.Features, f) {
				t.Errorf("%s: the feature %q is still in a screen's conditions", c.name, f)
			}
		}
		for _, f := range c.stillFeatures {
			if !slices.Contains(ctx.Features, f) {
				t.Errorf("%s: the feature %q was taken out", c.name, f)
			}
		}
		for _, m := range c.noMod {
			if slices.Contains(ctx.Modules, m) {
				t.Errorf("%s: the module %q still counts as installed", c.name, m)
			}
		}
		for _, m := range c.stillMod {
			if !slices.Contains(ctx.Modules, m) {
				t.Errorf("%s: the module %q was taken out", c.name, m)
			}
		}
		// A client that says "screenTimeUp" of itself isn't believed either way.
		if ctx.Flags["screenTimeUp"] {
			t.Errorf("%s: screenTimeUp is set", c.name)
		}
	}
	// Every name taken out is one the server really has: a renamed feature would otherwise
	// quietly stop being hidden.
	for f, h := range profileHides {
		for _, name := range h.features {
			if !slices.Contains(serverFeatures, name) {
				t.Errorf("%s hides the feature %q, which the server doesn't have", f, name)
			}
		}
	}
	for _, name := range liveTVHides.features {
		if !slices.Contains(serverFeatures, name) {
			t.Errorf("Live TV off hides the feature %q, which the server doesn't have", name)
		}
	}

	// Screen time, through the routes themselves.
	_, kid := ts.user(t, "kid", "kids", false, oneMinuteADay)
	clock := parentalClock(ts, 2, "16:00") // a Monday
	detail := fmt.Sprintf("/api/sdui/screens/detail?item=%d", film.ID)
	before := ts.do(t, "GET", detail, kid, nil)
	if before.Code != 200 || !strings.Contains(before.Body.String(), `"detail-play"`) {
		t.Fatalf("with time left, the title's page has no Play button: %d %s", before.Code, before.Body)
	}
	if res := ts.do(t, "GET", "/api/sdui/screens/player", kid, nil); res.Code != 200 {
		t.Fatalf("with time left, the player's screen: %d %s", res.Code, res.Body)
	}
	// A screen for a profile with time rules doesn't change by the second while there is time.
	clock.add(5 * time.Second)
	if again := ts.do(t, "GET", detail, kid, nil); again.Header().Get("ETag") != before.Header().Get("ETag") {
		t.Error("the same screen has another ETag five seconds on")
	}
	spend(t, ts, clock, kid, film.ID, 61*time.Second)
	after := ts.do(t, "GET", detail+"&capabilities=playback,screenTimeLeft", kid, nil)
	if after.Code != 200 || strings.Contains(after.Body.String(), `"detail-play"`) || strings.Contains(after.Body.String(), `"detail-resume"`) {
		t.Errorf("with time up, the title's page still offers to play: %d %s", after.Code, after.Body)
	}
	if !strings.Contains(after.Body.String(), film.Title) {
		t.Error("with time up, the title's page no longer shows the title")
	}
	if res := ts.do(t, "GET", "/api/sdui/screens/home", kid, nil); res.Code != 200 {
		t.Errorf("with time up, home: %d", res.Code)
	}
	res := ts.do(t, "GET", "/api/sdui/screens/player", kid, nil)
	var refusal struct {
		Code       string `json:"code"`
		ScreenTime struct {
			Allowed bool  `json:"allowed"`
			NextAt  int64 `json:"nextAt"`
		} `json:"screenTime"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &refusal)
	if res.Code != 403 || refusal.Code != "screen-time" || refusal.ScreenTime.Allowed || refusal.ScreenTime.NextAt == 0 {
		t.Errorf("with time up, the player's screen: %d %s", res.Code, res.Body)
	}
	if res := ts.do(t, "GET", "/api/sdui/screens/player/diff?since=W%2F%22x%22", kid, nil); res.Code != 403 {
		t.Errorf("with time up, the player's diff: %d", res.Code)
	}
	ctx, err := conditions("kids", oneMinuteADay, "home")
	if err != nil || ctx.Flags["screenTimeUp"] || !slices.Contains(ctx.Features, "playback") {
		t.Errorf("a profile with time left: %v %v", err, ctx.Flags)
	}
}

// ---------------------------------------------------------------------------
// Remote control
// ---------------------------------------------------------------------------

// tv announces a device that plays everything, and opens its channel.
func (w *limitsWorld) tv(t *testing.T, tok, name string) (string, *sse) {
	t.Helper()
	res := w.do(t, "POST", "/api/remote-control/devices", tok, map[string]any{"v": 1, "name": name, "kind": "tv",
		"capabilities": map[string]any{"mediaKinds": []string{"video", "audio", "game"}, "features": []string{"seek", "volume", "queue"}}})
	if res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	id, _ := body(res)["deviceId"].(string)
	ch := openSSE(t, w.srv, "/api/remote-control/devices/"+id+"/channel", tok)
	ch.next("hello")
	return id, ch
}

func (w *limitsWorld) allowOthers(t *testing.T, user int64) {
	t.Helper()
	if res := w.do(t, "PUT", fmt.Sprintf("/api/remote-control/accounts/%d", user), w.admin, map[string]any{"controlOthers": true}); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
}

// command sends a command and has the device say yes to it, if it gets that far.
func (w *limitsWorld) command(t *testing.T, tok, deviceTok, id string, ch *sse, cmd map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	cmd["v"] = 1
	out := make(chan *httptest.ResponseRecorder, 1)
	go func() { out <- w.do(t, "POST", "/api/remote-control/devices/"+id+"/commands", tok, cmd) }()
	for {
		select {
		case res := <-out:
			return res
		case e, open := <-ch.events:
			if !open {
				return <-out
			}
			if e[0] != "command" {
				continue
			}
			var got map[string]any
			_ = json.Unmarshal([]byte(e[1]), &got)
			if res := w.do(t, "POST", "/api/remote-control/devices/"+id+"/ack", deviceTok, map[string]any{"id": got["id"], "ok": true}); res.Code != 200 {
				t.Fatalf("ack: %d %s", res.Code, res.Body)
			}
		case <-time.After(20 * time.Second):
			t.Fatal("a command was neither answered nor refused")
		}
	}
}

// The limits of the account a device is signed in as decide what plays on it, whoever asks,
// and the caller's own decide what they may ask for: for every kind of limit. What a device
// plays is shown to a controller only if that controller may see it.
func TestRemoteControlFollowsEveryKindOfLimit(t *testing.T) {
	w := newLimitsWorld(t)
	toks, ids := w.profiles(t)
	bossID := w.userID(t, w.admin)
	w.allowOthers(t, bossID)
	bossTV, bossCh := w.tv(t, w.admin, "Den")
	for _, kind := range limitKinds {
		tok, hidden := toks[kind], w.hidden[kind]
		w.allowOthers(t, ids[kind])
		tv, ch := w.tv(t, tok, "Their room")

		// On the profile's own device: the owner can't play what the profile may not see...
		res := w.command(t, w.admin, tok, tv, ch, map[string]any{"type": "play", "itemId": hidden.ID})
		if res.Code != 403 || body(res)["code"] != "not-allowed-there" || strings.Contains(res.Body.String(), "Zebra") {
			t.Errorf("%s: the owner played what is hidden on the profile's device: %d %s", kind, res.Code, res.Body)
		}
		// ...not even tucked into a queue behind something it may...
		res = w.command(t, w.admin, tok, tv, ch, map[string]any{"type": "play", "itemId": w.fine.ID, "queue": []int64{w.fine.ID, hidden.ID}})
		if res.Code != 403 {
			t.Errorf("%s: a queue with what is hidden in it: %d %s", kind, res.Code, res.Body)
		}
		// ...and can play what it may.
		if res := w.command(t, w.admin, tok, tv, ch, map[string]any{"type": "play", "itemId": w.fine.ID}); res.Code != 200 {
			t.Errorf("%s: the owner played an allowed title there: %d %s", kind, res.Code, res.Body)
		}
		// The profile itself can't ask for it anywhere: on its own device, or on the owner's.
		for name, target := range map[string]struct {
			id, tok string
			ch      *sse
		}{"its own device": {tv, tok, ch}, "the owner's device": {bossTV, w.admin, bossCh}} {
			res := w.command(t, tok, target.tok, target.id, target.ch, map[string]any{"type": "play", "itemId": hidden.ID})
			if res.Code != 404 || body(res)["code"] != "no-item" {
				t.Errorf("%s: asked for what is hidden on %s: %d %s", kind, name, res.Code, res.Body)
			}
		}
		// The owner's device is playing it: the profile is told that it is playing, and no more.
		if res := w.do(t, "POST", "/api/remote-control/devices/"+bossTV+"/state", w.admin, map[string]any{"v": 1, "status": "playing", "itemId": hidden.ID, "positionSec": 30, "durationSec": 900, "queue": []int64{hidden.ID, w.fine.ID}}); res.Code != 200 {
			t.Fatalf("%d %s", res.Code, res.Body)
		}
		list := w.do(t, "GET", "/api/remote-control/devices", tok, nil)
		if !strings.Contains(list.Body.String(), `"hidden":true`) || strings.Contains(list.Body.String(), "Zebra") || strings.Contains(list.Body.String(), fmt.Sprintf(`"itemId":%d`, hidden.ID)) {
			t.Errorf("%s: what the owner's device is playing: %s", kind, list.Body)
		}
		// And taking it over is refused the same way as asking for it.
		if res := w.command(t, tok, tok, tv, ch, map[string]any{"type": "cast", "from": bossTV}); res.Code != 404 {
			t.Errorf("%s: took over what is hidden: %d %s", kind, res.Code, res.Body)
		}
		// Something it may see is shown, with only its own part of the queue.
		time.Sleep(600 * time.Millisecond) // a device's reports are paced
		if res := w.do(t, "POST", "/api/remote-control/devices/"+bossTV+"/state", w.admin, map[string]any{"v": 1, "status": "playing", "itemId": w.fine.ID, "queue": []int64{w.fine.ID, hidden.ID}}); res.Code != 200 {
			t.Fatalf("%d %s", res.Code, res.Body)
		}
		list = w.do(t, "GET", "/api/remote-control/devices", tok, nil)
		if !strings.Contains(list.Body.String(), w.fine.Title) || strings.Contains(list.Body.String(), fmt.Sprintf("%d]", hidden.ID)) {
			t.Errorf("%s: a title it may see on the owner's device: %s", kind, list.Body)
		}
		w.do(t, "DELETE", "/api/remote-control/devices/"+tv, tok, nil)
		time.Sleep(600 * time.Millisecond)
	}
}

// "remoteControl" off: the profile can't list, watch or steer devices, its own included.
// Its devices can still announce themselves, and be played on by whoever may.
func TestRemoteControlTurnedOffForAProfile(t *testing.T) {
	w := newLimitsWorld(t)
	kidU, kid := w.user(t, "kid", "kids", false, `{"maxAge":7,"off":["remoteControl"]}`)
	w.allowOthers(t, w.parentID)
	tv, ch := w.tv(t, kid, "Kid's TV")
	for _, r := range [][2]string{{"GET", "/api/remote-control/devices"}, {"GET", "/api/remote-control/events"}, {"POST", "/api/remote-control/devices/" + tv + "/commands"}} {
		res := w.do(t, r[0], r[1], kid, map[string]any{"v": 1, "type": "pause"})
		if res.Code != 403 || body(res)["code"] != "feature-off" || body(res)["feature"] != "remoteControl" {
			t.Errorf("%s %s: %d %s", r[0], r[1], res.Code, res.Body)
		}
	}
	info := body(w.do(t, "GET", "/api/remote-control", kid, nil))
	if info["enabled"] != true || info["canControl"] != false {
		t.Errorf("what the profile is told: %v", info)
	}
	if body(w.do(t, "GET", "/api/remote-control", w.parent, nil))["canControl"] != true {
		t.Error("a profile without the switch is told it can't control")
	}
	// The device's own side works: it reports, and a parent plays on it and pauses it.
	if res := w.do(t, "POST", "/api/remote-control/devices/"+tv+"/state", kid, map[string]any{"v": 1, "status": "idle"}); res.Code != 200 {
		t.Errorf("the device's own report: %d %s", res.Code, res.Body)
	}
	if res := w.command(t, w.parent, kid, tv, ch, map[string]any{"type": "play", "itemId": w.fine.ID}); res.Code != 200 {
		t.Errorf("a parent played on the profile's device: %d %s", res.Code, res.Body)
	}
	if res := w.command(t, w.parent, kid, tv, ch, map[string]any{"type": "pause"}); res.Code != 200 {
		t.Errorf("a parent paused the profile's device: %d %s", res.Code, res.Body)
	}
	if limits := body(w.do(t, "GET", "/api/me/limits", kid, nil)); limits["features"].(map[string]any)["remoteControl"] != false || limits["features"].(map[string]any)["syncplay"] != true {
		t.Errorf("what the profile's own limits say: %v", limits["features"])
	}

	// Turned off while a controller's stream is open: the stream ends at once, and says why.
	w.setLimits(t, w.admin, kidU.ID, `{"maxAge":7,"off":[]}`)
	events := openSSE(t, w.srv, "/api/remote-control/events", kid)
	events.next("snapshot")
	w.setLimits(t, w.admin, kidU.ID, `{"maxAge":7,"off":["remoteControl"]}`)
	if bye := events.until("bye", func(string) bool { return true }); !strings.Contains(bye, "feature-off") {
		t.Errorf("the stream's last word: %s", bye)
	}
}

// Screen time and remote control: nothing is started on a device whose profile's time is
// up, by anyone; a profile whose time is up starts nothing anywhere, but can still pause
// and stop; and what is played on a profile's device counts as that profile's time, whoever
// started it.
func TestRemoteControlAndScreenTime(t *testing.T) {
	w := newLimitsWorld(t)
	_, kid := w.user(t, "kid", "kids", false, oneMinuteADay)
	w.allowOthers(t, w.parentID)
	w.allowOthers(t, w.userID(t, kid))
	w.allowOthers(t, w.userID(t, w.admin))
	clock := parentalClock(w.testServer, 2, "16:00") // a Monday
	kidTV, kidCh := w.tv(t, kid, "Kid's TV")
	parentTV, parentCh := w.tv(t, w.parent, "Living room")

	// A parent starts a film on the child's TV. The TV plays it through the ordinary routes,
	// as the child, and that is the child's screen time.
	if res := w.command(t, w.parent, kid, kidTV, kidCh, map[string]any{"type": "play", "itemId": w.fine.ID}); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	file := fmt.Sprintf("/api/items/%d/file", w.fine.ID)
	if res := w.do(t, "GET", file, kid, nil); res.Code != 200 || res.Header().Get("X-WMS-Screen-Time-Left") != "60" {
		t.Fatalf("the TV fetching what it was told to play: %d %q", res.Code, res.Header().Get("X-WMS-Screen-Time-Left"))
	}
	clock.add(40 * time.Second)
	if res := w.do(t, "POST", fmt.Sprintf("/api/items/%d/progress", w.fine.ID), kid, map[string]any{"positionSec": 40, "durationSec": 3000}); res.Code != 200 {
		t.Fatal(res.Code)
	}
	var st struct {
		UsedSec int  `json:"usedSec"`
		Allowed bool `json:"allowed"`
	}
	readBody(t, w.do(t, "GET", "/api/me/screen-time", kid, nil).Body.Bytes(), &st)
	if st.UsedSec != 40 || !st.Allowed {
		t.Fatalf("after 40 seconds of a film a parent started: %+v", st)
	}
	clock.add(25 * time.Second)
	if res := w.do(t, "POST", fmt.Sprintf("/api/items/%d/progress", w.fine.ID), kid, map[string]any{"positionSec": 65, "durationSec": 3000}); res.Code != 200 {
		t.Fatal(res.Code)
	}
	// Time is up. What the TV asks for next is refused, so the film stops there.
	if res := w.do(t, "GET", file, kid, nil); res.Code != 403 || body(res)["code"] != "screen-time" {
		t.Fatalf("the TV's next request with time up: %d %s", res.Code, res.Body)
	}
	// Nobody starts anything on that TV now: not the parent, not the owner.
	for _, cmd := range []map[string]any{{"type": "play", "itemId": w.fine.ID}, {"type": "resume"}, {"type": "next"}, {"type": "previous"}} {
		for who, tok := range map[string]string{"a parent": w.parent, "the owner": w.admin} {
			res := w.command(t, tok, kid, kidTV, kidCh, cmd)
			if res.Code != 403 || body(res)["code"] != "screen-time-there" {
				t.Errorf("%s sent %v to a TV whose profile's time is up: %d %s", who, cmd["type"], res.Code, res.Body)
			}
		}
	}
	// Taking over from the parent's TV is starting something too.
	if res := w.do(t, "POST", "/api/remote-control/devices/"+parentTV+"/state", w.parent, map[string]any{"v": 1, "status": "playing", "itemId": w.fine.ID, "durationSec": 900}); res.Code != 200 {
		t.Fatal(res.Code)
	}
	if res := w.command(t, w.parent, kid, kidTV, kidCh, map[string]any{"type": "cast", "from": parentTV}); res.Code != 403 || body(res)["code"] != "screen-time-there" {
		t.Errorf("handed a film over to a TV whose profile's time is up: %d %s", res.Code, res.Body)
	}
	// They can still stop it and turn it down.
	for _, cmd := range []map[string]any{{"type": "pause"}, {"type": "stop"}, {"type": "volume", "level": 0.2}} {
		if res := w.command(t, w.parent, kid, kidTV, kidCh, cmd); res.Code != 200 {
			t.Errorf("a parent sent %v: %d %s", cmd["type"], res.Code, res.Body)
		}
	}
	// The child, time up, can't start anything on the parent's TV either; but can pause it.
	res := w.command(t, kid, w.parent, parentTV, parentCh, map[string]any{"type": "play", "itemId": w.fine.ID})
	if res.Code != 403 || body(res)["code"] != "screen-time" {
		t.Errorf("a profile whose time is up played on someone else's TV: %d %s", res.Code, res.Body)
	}
	if res := w.command(t, kid, w.parent, parentTV, parentCh, map[string]any{"type": "resume"}); res.Code != 403 {
		t.Errorf("a profile whose time is up resumed someone else's TV: %d %s", res.Code, res.Body)
	}
	if res := w.command(t, kid, w.parent, parentTV, parentCh, map[string]any{"type": "pause"}); res.Code != 200 {
		t.Errorf("a profile whose time is up paused a TV: %d %s", res.Code, res.Body)
	}
	// A parent gives ten minutes: the TV can be played on again.
	if res := w.do(t, "POST", fmt.Sprintf("/api/users/%d/screen-time/extra", w.userID(t, kid)), w.admin, map[string]any{"minutes": 10}); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	if res := w.command(t, w.parent, kid, kidTV, kidCh, map[string]any{"type": "play", "itemId": w.fine.ID}); res.Code != 200 {
		t.Errorf("after more time was given: %d %s", res.Code, res.Body)
	}
}

// ---------------------------------------------------------------------------
// Watch parties
// ---------------------------------------------------------------------------

func (w *limitsWorld) room(t *testing.T, tok string, room partyRoom) partyRoom {
	t.Helper()
	return party(t, w.do(t, "GET", "/api/syncplay/rooms/"+room.ID+"?member="+room.You, tok, nil))
}

func (w *limitsWorld) seat(t *testing.T, tok string, room partyRoom, what string, b map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	if b == nil {
		b = map[string]any{}
	}
	b["member"] = room.You
	return w.do(t, "POST", "/api/syncplay/rooms/"+room.ID+"/"+what, tok, b)
}

func (w *limitsWorld) ready(t *testing.T, tok string, room partyRoom) {
	t.Helper()
	if res := w.seat(t, tok, room, "report", map[string]any{"state": "ready", "rev": w.room(t, tok, room).State.Rev}); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
}

func memberState(room partyRoom, id string) string {
	for _, m := range room.Members {
		if m.ID == id {
			return m.State
		}
	}
	return ""
}

// A watch party never shows a member more than their account may see, for every kind of
// limit: starting one, joining one, a queue that moves on to something hidden, and limits
// that change while the party is going.
func TestSyncplayFollowsEveryKindOfLimit(t *testing.T) {
	w := newLimitsWorld(t)
	toks, ids := w.profiles(t)
	for _, kind := range limitKinds {
		tok, hidden := toks[kind], w.hidden[kind]
		// Starting one with what is hidden, alone or in a queue: as if it didn't exist.
		for _, b := range []map[string]any{{"itemId": hidden.ID}, {"items": []int64{w.fine.ID, hidden.ID}}} {
			if res := w.do(t, "POST", "/api/syncplay/rooms", tok, b); res.Code != 404 || body(res)["code"] != "no-item" {
				t.Errorf("%s: started a party with what is hidden: %d %s", kind, res.Code, res.Body)
			}
		}
		// Joining one that is watching it: refused, and told nothing.
		theirs := party(t, w.do(t, "POST", "/api/syncplay/rooms", w.parent, map[string]any{"itemId": hidden.ID, "name": "Grown-ups"}))
		if res := w.do(t, "POST", "/api/syncplay/join", tok, map[string]any{"code": theirs.Code}); res.Code != 403 || body(res)["code"] != "cant-watch" || strings.Contains(res.Body.String(), "Zebra") {
			t.Errorf("%s: joined a party watching what is hidden: %d %s", kind, res.Code, res.Body)
		}
		w.do(t, "DELETE", "/api/syncplay/rooms/"+theirs.ID+"?member="+theirs.You, w.parent, nil)

		// A party that starts with something for everyone and moves on to what is hidden.
		host := party(t, w.do(t, "POST", "/api/syncplay/rooms", w.parent, map[string]any{"items": []int64{w.fine.ID, hidden.ID}, "policy": "everyone"}))
		mine := party(t, w.do(t, "POST", "/api/syncplay/join", tok, map[string]any{"code": host.Code}))
		if len(mine.Items) != 2 || mine.Items[0].Title != w.fine.Title || mine.Items[1].ID != 0 || mine.Items[1].Title != "" || mine.Queue[1] != 0 {
			t.Errorf("%s: the queue as the member sees it: %+v %v", kind, mine.Items, mine.Queue)
		}
		// The member can't put what is hidden in the queue, though they may steer.
		if res := w.seat(t, tok, mine, "command", map[string]any{"type": "queue", "items": []int64{hidden.ID}}); res.Code != 403 || body(res)["code"] != "cant-watch" {
			t.Errorf("%s: queued what is hidden: %d %s", kind, res.Code, res.Body)
		}
		w.ready(t, w.parent, host)
		w.ready(t, tok, mine)
		if res := w.seat(t, w.parent, host, "command", map[string]any{"type": "play"}); res.Code != 200 {
			t.Fatal(res.Code, res.Body)
		}
		if res := w.seat(t, w.parent, host, "command", map[string]any{"type": "next"}); res.Code != 200 {
			t.Fatal(res.Code, res.Body)
		}
		now := w.room(t, tok, mine)
		if !now.Blocked || now.State.ItemID != 0 || now.CanControl || strings.Contains(fmt.Sprint(now.Items), "Zebra") {
			t.Errorf("%s: the member, when the party moved on to what is hidden: %+v", kind, now)
		}
		if res := w.seat(t, tok, mine, "command", map[string]any{"type": "pause"}); res.Code != 403 {
			t.Errorf("%s: a member who may not see what is playing steered the room: %d", kind, res.Code)
		}
		// The room doesn't wait for them: the host alone being ready is enough.
		w.ready(t, w.parent, host)
		if hostNow := w.room(t, w.parent, host); hostNow.State.Waiting || hostNow.State.Paused || memberState(hostNow, mine.You) != "blocked" {
			t.Errorf("%s: the room is held up by a member who may not watch: %+v", kind, hostNow.State)
		}
		w.do(t, "DELETE", "/api/syncplay/rooms/"+host.ID+"?member="+host.You, w.parent, nil)
		_ = ids
	}

	// Limits set while a party is going count at once, not from the next item on.
	u, late := w.user(t, "late", "adult", false, "")
	film := w.hidden["a collection never allowed"]
	host := party(t, w.do(t, "POST", "/api/syncplay/rooms", w.parent, map[string]any{"itemId": film.ID}))
	mine := party(t, w.do(t, "POST", "/api/syncplay/join", late, map[string]any{"code": host.Code}))
	events := openSSE(t, w.srv, "/api/syncplay/rooms/"+mine.ID+"/events?member="+mine.You, late)
	if first := events.next("room"); !strings.Contains(first, film.Title) {
		t.Fatalf("before the limits: %s", first)
	}
	w.setLimits(t, w.admin, u.ID, w.limits("a collection never allowed"))
	told := events.until("room", func(d string) bool { return strings.Contains(d, `"blocked":true`) })
	if strings.Contains(told, "Zebra") || !strings.Contains(told, `"itemId":0`) {
		t.Errorf("after the limits were set: %s", told)
	}
	// And when the title leaves the collection, they are back.
	if _, err := w.DB.Exec(`DELETE FROM collection_items WHERE collection_id = ?`, w.collection); err != nil {
		t.Fatal(err)
	}
	w.forgetRules()
	events.until("room", func(d string) bool { return strings.Contains(d, `"blocked":false`) && strings.Contains(d, film.Title) })
}

// "syncplay" off for a profile: no starting, no joining, whatever the server's own setting
// says; and a seat it had goes the moment it is turned off. The "sharing" and "comments"
// switches say nothing about watch parties.
func TestSyncplayTurnedOffForAProfile(t *testing.T) {
	w := newLimitsWorld(t)
	kidU, kid := w.user(t, "kid", "kids", false, `{"maxAge":7,"off":["syncplay"]}`)
	host := party(t, w.do(t, "POST", "/api/syncplay/rooms", w.parent, map[string]any{"itemId": w.fine.ID}))
	for _, c := range []struct {
		method, path string
		b            map[string]any
	}{
		{"POST", "/api/syncplay/rooms", map[string]any{"itemId": w.fine.ID}},
		{"POST", "/api/syncplay/join", map[string]any{"code": host.Code}},
		{"GET", "/api/syncplay/rooms/" + host.ID, nil},
		{"GET", "/api/syncplay/rooms/" + host.ID + "/events", nil},
		{"POST", "/api/syncplay/rooms/" + host.ID + "/command", map[string]any{"type": "play"}},
	} {
		res := w.do(t, c.method, c.path, kid, c.b)
		if res.Code != 403 || body(res)["code"] != "feature-off" || body(res)["feature"] != "syncplay" {
			t.Errorf("%s %s: %d %s", c.method, c.path, res.Code, res.Body)
		}
	}
	// Invited, and still not told of it.
	if res := w.seat(t, w.parent, host, "invite", map[string]any{"userIds": []int64{kidU.ID}}); res.Code != 200 {
		t.Fatal(res.Code, res.Body)
	}
	info := body(w.do(t, "GET", "/api/syncplay", kid, nil))
	if info["enabled"] != true || info["canCreate"] != false || info["canJoin"] != false || len(info["rooms"].([]any)) != 0 {
		t.Errorf("what the profile is told: %v", info)
	}
	// Leaving is never refused, so an app that was in one can always tidy up.
	if res := w.do(t, "POST", "/api/syncplay/rooms/"+host.ID+"/leave", kid, map[string]any{"member": "x"}); res.Code == 403 {
		t.Errorf("leaving: %d %s", res.Code, res.Body)
	}

	// Sharing and comments off, watch parties on: a Kids profile starts one, is invited to
	// one and joins one. A party's invitation is not a share link.
	w.setLimits(t, w.admin, kidU.ID, `{"maxAge":7,"off":["sharing","comments","downloads"]}`)
	own := party(t, w.do(t, "POST", "/api/syncplay/rooms", kid, map[string]any{"itemId": w.fine.ID}))
	if res := w.seat(t, kid, own, "invite", map[string]any{"userIds": []int64{w.parentID}}); res.Code != 200 {
		t.Errorf("a profile with sharing off invited someone to its party: %d %s", res.Code, res.Body)
	}
	mine := party(t, w.do(t, "POST", "/api/syncplay/join", kid, map[string]any{"roomId": host.ID}))
	// The server's setting and the profile's switch: both must allow starting one.
	if res := w.do(t, "PUT", "/api/settings", w.admin, map[string]string{"syncplayCreate": "adults"}); res.Code != 200 {
		t.Fatal(res.Code)
	}
	if res := w.do(t, "POST", "/api/syncplay/rooms", kid, map[string]any{"itemId": w.fine.ID}); res.Code != 403 || body(res)["code"] != "not-allowed" {
		t.Errorf("a Kids profile started one where only adults may: %d %s", res.Code, res.Body)
	}
	if info := body(w.do(t, "GET", "/api/syncplay", kid, nil)); info["canCreate"] != false || info["canJoin"] != true {
		t.Errorf("%v", info)
	}

	// Turned off while seated, with the stream open: the seat goes and the stream says why.
	events := openSSE(t, w.srv, "/api/syncplay/rooms/"+mine.ID+"/events?member="+mine.You, kid)
	events.next("room")
	w.setLimits(t, w.admin, kidU.ID, `{"maxAge":7,"off":["syncplay"]}`)
	if last := events.until("closed", func(string) bool { return true }); !strings.Contains(last, "feature-off") {
		t.Errorf("the stream's last word: %s", last)
	}
	if _, err := w.syncEngine().View(mine.ID, mine.You, kidU.ID); err == nil {
		t.Error("a profile with watch parties off still has its seat")
	}
	if now := w.room(t, w.parent, host); len(now.Members) != 1 {
		t.Errorf("the host still sees %d members", len(now.Members))
	}
}

// Screen time and watch parties: a profile whose time is up neither starts nor joins one;
// one whose time runs out during a party sits out, so the others aren't left waiting for a
// player that can't play; and it is back when a parent gives more time.
func TestSyncplayAndScreenTime(t *testing.T) {
	w := newLimitsWorld(t)
	kidU, kid := w.user(t, "kid", "kids", false, oneMinuteADay)
	clock := parentalClock(w.testServer, 2, "16:00") // a Monday
	host := party(t, w.do(t, "POST", "/api/syncplay/rooms", w.parent, map[string]any{"itemId": w.fine.ID, "policy": "everyone"}))
	mine := party(t, w.do(t, "POST", "/api/syncplay/join", kid, map[string]any{"code": host.Code}))
	w.ready(t, w.parent, host)
	w.ready(t, kid, mine)
	if res := w.seat(t, w.parent, host, "command", map[string]any{"type": "play"}); res.Code != 200 {
		t.Fatal(res.Code, res.Body)
	}
	// The child watches for a minute, through the ordinary routes, and the time runs out.
	spend(t, w.testServer, clock, kid, w.fine.ID, 61*time.Second)
	// A seek: everyone has to load again, and the child's player can't (its requests are refused).
	if res := w.seat(t, w.parent, host, "command", map[string]any{"type": "seek", "positionSec": 300}); res.Code != 200 {
		t.Fatal(res.Code, res.Body)
	}
	w.ready(t, w.parent, host)
	if now := w.room(t, w.parent, host); !now.State.Waiting {
		t.Fatalf("before the server has looked, the room waits for the child: %+v", now.State)
	}
	w.watchPartyMembers(context.Background()) // what the server does every few seconds while there are rooms
	now := w.room(t, w.parent, host)
	if now.State.Waiting || now.State.Paused || memberState(now, mine.You) != "time-up" {
		t.Errorf("the room is held up by a member whose time is up: %+v %s", now.State, memberState(now, mine.You))
	}
	var kidView struct {
		TimeUp     bool `json:"timeUp"`
		CanControl bool `json:"canControl"`
		State      struct {
			ItemID int64 `json:"itemId"`
		} `json:"state"`
	}
	readBody(t, w.do(t, "GET", "/api/syncplay/rooms/"+mine.ID+"?member="+mine.You, kid, nil).Body.Bytes(), &kidView)
	if !kidView.TimeUp || kidView.CanControl || kidView.State.ItemID != 0 {
		t.Errorf("the room as the child sees it with time up: %+v", kidView)
	}
	if res := w.seat(t, kid, mine, "command", map[string]any{"type": "pause"}); res.Code != 403 || body(res)["code"] != "screen-time" {
		t.Errorf("a member whose time is up steered the room: %d %s", res.Code, res.Body)
	}
	// Neither starting nor joining another.
	for _, c := range []struct {
		path string
		b    map[string]any
	}{{"/api/syncplay/rooms", map[string]any{"itemId": w.fine.ID}}, {"/api/syncplay/join", map[string]any{"code": host.Code, "device": "tablet"}}} {
		if res := w.do(t, "POST", c.path, kid, c.b); res.Code != 403 || body(res)["code"] != "screen-time" {
			t.Errorf("POST %s with time up: %d %s", c.path, res.Code, res.Body)
		}
	}
	// More time from a parent: back in at once, without waiting for the server's next look.
	if res := w.do(t, "POST", fmt.Sprintf("/api/users/%d/screen-time/extra", kidU.ID), w.admin, map[string]any{"minutes": 10}); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	kidView.TimeUp = false
	readBody(t, w.do(t, "GET", "/api/syncplay/rooms/"+mine.ID+"?member="+mine.You, kid, nil).Body.Bytes(), &kidView)
	if kidView.TimeUp || !kidView.CanControl || kidView.State.ItemID != w.fine.ID {
		t.Errorf("the room as the child sees it with more time: %+v", kidView)
	}
	if res := w.seat(t, kid, mine, "leave", nil); res.Code != 200 {
		t.Errorf("leaving: %d %s", res.Code, res.Body)
	}
}
