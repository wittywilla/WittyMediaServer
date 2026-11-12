// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// The walk: a library where every title a limited profile must not see has one made-up word in its
// name, its genre, its studio and its file; a profile for each kind of limit; and every route a
// signed-in account can reach, called with every hidden title's number and with the word searched
// for. The word must never come back, and a route that is addressed by a title must refuse.
//
// The routes come from routeRules, which TestEveryRouteHasAStatedRule keeps complete, so a new
// route is walked without anyone remembering to add it here. A new route with an {id} in it has to
// say what the number names (idSpaces), or the walk fails.

// hiddenWord is in everything that is hidden. The searches ask for its first eight letters, so an
// answer that repeats the question doesn't look like a leak.
const (
	hiddenWord   = "ZZHIDDENZZ"
	hiddenSearch = "ZZHIDDEN"
)

// idSpaces says what the {id} in a route names, by the route up to its {id}: "item" and "show" are
// called with the hidden titles' numbers and must refuse; "other" names something else (a
// collection, a profile, a stream) and is called with every number there is, for what it shows.
var idSpaces = map[string]string{
	"/api/p2p/devices/{id}": "other", // a device's key
	"/api/items/{id}":       "item", "/api/media/{id}": "item", "/api/games/{id}": "item", "/api/cd/{id}": "item", "/api/radio/{id}": "item",
	"/api/videos/{id}": "item", "/api/photos/{id}": "item", "/api/music/extras/{id}": "item", "/api/music/loudness/{id}": "item",
	"/api/music/links/{id}": "item", "/api/music/lyrics/{id}": "item", "/api/music/radio/{id}": "item", "/api/rigs/games/{id}": "item",
	"/api/shows/{id}":         "show",
	"/api/meta/{target}/{id}": "target",
	"/api/collections/{id}":   "other", "/api/photos/albums/{id}": "other", "/api/creators/{id}": "other", "/api/hls/{id}": "other",
	"/api/logos/{id}": "other", "/api/licenses/{id}": "other", "/api/libraries/{id}": "other", "/api/users/{id}": "other",
	"/api/views/{id}": "other", "/api/channels/{id}": "other", "/api/couch/hosts/{id}": "other", "/api/game-profiles/{id}": "other",
	"/api/modules/{id}": "other", "/api/module-store/{id}": "other", "/api/rigs/{id}": "other", "/api/sessions/{id}": "other",
	"/api/shares/{id}": "other", "/api/recordings/{id}": "other", "/api/recording-rules/{id}": "other", "/api/tv/reminders/{id}": "other", "/api/tv-sets/{id}": "other",
	// A device and a watch party are named by random words, never by a title's number. What
	// they show of titles is walked with a device that is playing what is hidden and a party
	// that is watching it: see playingWhatIsHidden.
	"/api/remote-control/devices/{id}": "other", "/api/syncplay/rooms/{id}": "other",
	"/api/modules/wittyplay/docker/emulators/{id}": "other",
}

// walkSkips are routes the walk can't call: they hold the connection open until the caller goes
// away, or go out to the internet. None of them is addressed by a title.
var walkSkips = map[string]string{
	"GET /api/system/metrics/stream": "a stream of numbers, for as long as it is listened to",
	"GET /api/system/tasks/stream":   "a stream of tasks, for as long as it is listened to",
	"GET /api/play/{sid}/ws":         "a WebSocket",
	"GET /api/play/{sid}/stream":     "a WebSocket",
	"GET /api/couch/find":            "searches the network",
	"GET /api/playback/speedtest":    "megabytes of noise",
	"GET /api/dictionary":            "asks the internet",
	"GET /api/remote-control/events": "a stream of devices, for as long as it is listened to; the same views as the list of devices, which is walked",
}

type walkFixture struct {
	ts         *testServer
	admin      string
	hidden     []*library.Item // every title a limited profile must not see
	show       *library.Show   // the hidden show
	fineLibs   []int64
	hiddenLibs []int64
	collection int64 // a shared collection of everything hidden
	album      int64 // a shared album of the hidden photo
	creator    int64
	tv         string    // the owner's TV, playing what is hidden
	room       partyRoom // the owner's watch party, watching it
}

func newWalkFixture(t *testing.T) *walkFixture {
	t.Helper()
	ts := newTestServer(t)
	ctx := context.Background()
	f := &walkFixture{ts: ts}
	// The owner's TV in the walk never opens its channel. It mustn't be forgotten for that
	// while the walk, which can be slow on a busy machine, is still going.
	ts.remoteHub().Limits.AnnounceGrace = time.Hour
	_, f.admin = ts.user(t, "boss", "adult", true, "")
	root := t.TempDir()
	secret := []byte("file of " + hiddenWord)
	picture := append(logoPicture(t), secret...)
	write := func(path string, data []byte) string {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	one := 1
	for _, k := range []struct{ typ, kind, ext, fineRating, hiddenRating string }{
		{"movies", "video", ".mp4", "G", "R"},
		{"shows", "video", ".mkv", "TV-Y", "TV-MA"},
		{"games", "game", ".nes", "ESRB E", "ESRB M"},
		{"music", "audio", ".mp3", "Clean", "Explicit"},
		{"books", "book", ".epub", "0", "18"},
		{"photos", "photo", ".png", "G", "R"},
		{"radio", "radio", "", "G", "R"},
		{"videos", "video", ".mp4", "G", "R"},
	} {
		fineDir, hiddenDir := filepath.Join(root, "fine-"+k.typ), filepath.Join(root, "hidden-"+k.typ)
		fine, hid := ts.library(t, k.typ, fineDir), ts.library(t, k.typ, hiddenDir)
		f.fineLibs, f.hiddenLibs = append(f.fineLibs, fine), append(f.hiddenLibs, hid)
		for _, side := range []struct {
			lib           int64
			dir, word, rt string
			hidden        bool
		}{{fine, fineDir, "Fine", k.fineRating, false}, {hid, hiddenDir, hiddenWord, k.hiddenRating, true}} {
			path := filepath.Join(side.dir, side.word+" "+k.typ+k.ext)
			data := secret
			if k.kind == "photo" {
				data = picture
			}
			if k.ext == "" {
				path = "https://radio.example.net/" + side.word
			} else {
				write(path, data)
			}
			it := &library.Item{LibraryID: side.lib, Kind: k.kind, Path: path, Title: side.word + " " + k.typ, ContentRating: side.rt,
				Overview: "About " + side.word, Tagline: side.word, Studio: side.word + " studio", Genres: []string{side.word + "genre"}}
			if side.hidden {
				it.Poster = write(filepath.Join(side.dir, "poster.png"), picture)
				it.Backdrop = it.Poster
			}
			if k.kind == "audio" {
				it.Artist, it.Album, it.AlbumArtist = side.word+" artist", side.word+" album", side.word+" artist"
			}
			if k.typ == "shows" {
				sh := &library.Show{LibraryID: side.lib, Path: side.dir, Title: side.word + " show", SortTitle: side.word + " show", ContentRating: side.rt,
					Genres: []string{side.word + "genre"}, Network: side.word + " network", Overview: "About " + side.word}
				if side.hidden {
					sh.Poster = it.Poster
				}
				if err := ts.Store.UpsertShow(ctx, sh); err != nil {
					t.Fatal(err)
				}
				it.ShowID, it.Season, it.Episode, it.ContentRating = &sh.ID, &one, &one, ""
				if side.hidden {
					f.show = sh
				}
			}
			ts.item(t, it)
			if k.typ == "videos" {
				res, err := ts.DB.ExecContext(ctx, `INSERT INTO creators(library_id, key, name, added_at, updated_at) VALUES(?, ?, ?, 1, 1)`, side.lib, side.word, side.word+" creator")
				if err != nil {
					t.Fatal(err)
				}
				cid, _ := res.LastInsertId()
				if _, err := ts.DB.ExecContext(ctx, `INSERT INTO video_meta(item_id, library_id, creator_id, tags) VALUES(?, ?, ?, ?)`, it.ID, side.lib, cid, side.word+"tag"); err != nil {
					t.Fatal(err)
				}
				if side.hidden {
					f.creator = cid
				}
			}
			if side.hidden {
				f.hidden = append(f.hidden, it)
			}
		}
	}
	if err := ts.Store.SyncBrands(ctx); err != nil {
		t.Fatal(err)
	}
	// A shared collection and a shared album of what is hidden, made by someone who may see it.
	bossID := ts.userID(t, f.admin)
	res, err := ts.DB.ExecContext(ctx, `INSERT INTO collections(owner_id, name, description, shared, created_at, updated_at) VALUES(?, 'Shared by the owner', '', 1, 1, 1)`, bossID)
	if err != nil {
		t.Fatal(err)
	}
	f.collection, _ = res.LastInsertId()
	for _, it := range f.hidden {
		if _, err := ts.DB.ExecContext(ctx, `INSERT INTO collection_items(collection_id, item_id, added_at) VALUES(?, ?, 1)`, f.collection, it.ID); err != nil {
			t.Fatal(err)
		}
		if w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/comments", it.ID), f.admin, map[string]any{"body": "a comment about " + hiddenWord}); w.Code >= 500 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	if _, err := ts.DB.ExecContext(ctx, `INSERT INTO collection_shows(collection_id, show_id, added_at) VALUES(?, ?, 1)`, f.collection, f.show.ID); err != nil {
		t.Fatal(err)
	}
	for _, it := range f.hidden {
		if it.Kind != "photo" {
			continue
		}
		w := ts.do(t, "POST", "/api/photos/albums", f.admin, map[string]any{"libraryId": it.LibraryID, "name": "Shared album", "shared": true})
		var alb struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &alb); err != nil || alb.ID == 0 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		f.album = alb.ID
		if w := ts.do(t, "POST", fmt.Sprintf("/api/photos/albums/%d/items", alb.ID), f.admin, map[string]any{"ids": []int64{it.ID}}); w.Code >= 300 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	return f
}

// watchedBefore gives a profile a past with everything hidden: half-watched, played, liked and
// saved for later, as if the limits were set afterwards.
func (f *walkFixture) watchedBefore(t *testing.T, userID int64) {
	t.Helper()
	ctx := context.Background()
	day := time.Now().Year()*10000 + 101
	res, err := f.ts.DB.ExecContext(ctx, `INSERT INTO collections(owner_id, name, system, created_at, updated_at) VALUES(?, 'Watch later', 'watch-later', 1, 1)`, userID)
	if err != nil {
		t.Fatal(err)
	}
	later, _ := res.LastInsertId()
	for _, it := range f.hidden {
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO progress(user_id, item_id, position_sec, duration_sec, watched, updated_at) VALUES(?, ?, 300, 3000, 0, ?)`, []any{userID, it.ID, time.Now().Unix()}},
			{`INSERT INTO item_stats(user_id, item_id, plays, seconds, completes, last_at) VALUES(?, ?, 5, 5000, 2, ?)`, []any{userID, it.ID, time.Now().Unix()}},
			{`INSERT INTO play_log(user_id, item_id, day, seconds) VALUES(?, ?, ?, 5000)`, []any{userID, it.ID, day}},
			{`INSERT INTO item_reactions(user_id, item_id, value, created_at) VALUES(?, ?, 1, 1)`, []any{userID, it.ID}},
			{`INSERT INTO collection_items(collection_id, item_id, added_at) VALUES(?, ?, 1)`, []any{later, it.ID}},
		} {
			if _, err := f.ts.DB.ExecContext(ctx, q.sql, q.args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := f.ts.DB.ExecContext(ctx, `INSERT INTO creator_subs(user_id, creator_id, created_at) VALUES(?, ?, 1)`, userID, f.creator); err != nil {
		t.Fatal(err)
	}
}

// playingWhatIsHidden sets up, for one account, the two places where what someone else is
// watching is shown: a TV of the owner's that is playing the hidden titles, which the account
// has been allowed to control, and a watch party of the owner's watching them, to which the
// account is invited. It answers the party's id.
func (f *walkFixture) playingWhatIsHidden(t *testing.T, userID int64) (room string) {
	t.Helper()
	ts := f.ts
	var ids []int64
	for _, it := range f.hidden {
		ids = append(ids, it.ID)
	}
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/remote-control/accounts/%d", userID), f.admin, map[string]any{"controlOthers": true}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	// The TV is announced again only once it has been forgotten: a device that never opens
	// its channel is kept for half a minute, and its reports are paced.
	if d, here := ts.remoteHub().Device(f.tv); !here || d.State.ItemID == 0 {
		w := ts.do(t, "POST", "/api/remote-control/devices", f.admin, map[string]any{"v": 1, "deviceId": f.tv, "name": "Den", "kind": "tv",
			"capabilities": map[string]any{"mediaKinds": []string{"video", "audio", "game", "book", "photo", "radio"}, "features": []string{"queue"}}})
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		f.tv, _ = body(w)["deviceId"].(string)
		if w := ts.do(t, "POST", "/api/remote-control/devices/"+f.tv+"/state", f.admin, map[string]any{"v": 1, "status": "playing", "itemId": ids[0], "queue": ids, "durationSec": 100}); w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	if f.room.ID != "" {
		ts.do(t, "DELETE", "/api/syncplay/rooms/"+f.room.ID+"?member="+f.room.You, f.admin, nil)
	}
	f.room = party(t, ts.do(t, "POST", "/api/syncplay/rooms", f.admin, map[string]any{"items": ids, "name": "Movie night"}))
	if w := ts.do(t, "POST", "/api/syncplay/rooms/"+f.room.ID+"/invite", f.admin, map[string]any{"member": f.room.You, "userIds": []int64{userID}}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	return f.room.ID
}

// walkTargets turns a route into the addresses to call.
func (f *walkFixture) walkTargets(t *testing.T, route string) (targets []string, space string) {
	t.Helper()
	path := strings.SplitN(route, " ", 2)[1]
	var items, shows, others []string
	for _, it := range f.hidden {
		items = append(items, fmt.Sprint(it.ID))
	}
	shows = []string{fmt.Sprint(f.show.ID)}
	others = append(append([]string{"1", "2", "3", fmt.Sprint(f.collection), fmt.Sprint(f.album), fmt.Sprint(f.creator)}, items...), shows...)
	for _, lib := range f.hiddenLibs {
		others = append(others, fmt.Sprint(lib))
	}
	fill := strings.NewReplacer("{n}", "1", "{kind}", "poster", "{file}", "x.vtt", "{file...}", "x", "{key}", "x", "{sid}", "x", "{slot}", "1",
		"{serial}", "1", "{name}", "x", "{ttn}", "1", "{stem}", "vocals", "{console}", "nes", "{bezel}", "x", "{page}", "home", "{section}", "recent-1", "{cid}", "1",
		"{aid}", "1", "{app}", "1", "{patch}", "x", "{session}", "x", "{rest...}", "x", "{version}", "1", "{path...}", "x", "{user}", "1", "{token}", "x")
	queries := []string{"", "?kind=studio", "?kind=network", "?search=" + hiddenSearch + "&q=" + hiddenSearch + "&query=" + hiddenSearch + "&name=" + hiddenSearch + "&artist=" + hiddenSearch + "+artist&album=" + hiddenSearch + "+album&limit=200&year=" + fmt.Sprint(time.Now().Year())}
	i := strings.Index(path, "{id}")
	if i < 0 {
		for _, lib := range f.hiddenLibs {
			queries = append(queries, fmt.Sprintf("?library=%d&limit=200&genre=%sgenre&tag=%stag", lib, hiddenWord, hiddenWord))
		}
		for _, q := range queries {
			targets = append(targets, fill.Replace(path)+q)
		}
		return targets, ""
	}
	space, ok := idSpaces[path[:i+len("{id}")]]
	if !ok {
		t.Errorf("%s: say in idSpaces (authz_parental_walk_test.go) what its {id} names: \"item\", \"show\" or \"other\"", route)
		return nil, ""
	}
	type id struct{ target, id string }
	var ids []id
	switch space {
	case "item":
		for _, x := range items {
			ids = append(ids, id{"item", x})
		}
	case "show":
		ids = []id{{"show", shows[0]}}
	case "target":
		for _, x := range items {
			ids = append(ids, id{"item", x})
		}
		ids = append(ids, id{"show", shows[0]})
	default:
		for _, x := range others {
			ids = append(ids, id{"", x})
		}
	}
	for _, x := range ids {
		p := strings.Replace(strings.Replace(path, "{id}", x.id, 1), "{target}", x.target, 1)
		for _, q := range []string{queries[0], queries[3]} {
			targets = append(targets, fill.Replace(p)+q)
		}
	}
	return targets, space
}

func (f *walkFixture) call(method, target, token string, body any) *httptest.ResponseRecorder {
	var rd *strings.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = strings.NewReader(string(raw))
	} else {
		rd = strings.NewReader("")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r := httptest.NewRequest(method, target, rd).WithContext(ctx)
	r.Host = "127.0.0.1:9797"
	r.RemoteAddr = "127.0.0.1:50000"
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	f.ts.h.ServeHTTP(w, r)
	return w
}

func leaks(body string) bool { return strings.Contains(strings.ToUpper(body), hiddenWord) }

func TestLimitedProfilesNeverSeeHiddenTitles(t *testing.T) {
	f := newWalkFixture(t)
	ts := f.ts
	var blockItems, kinds []string
	for _, it := range f.hidden {
		blockItems = append(blockItems, fmt.Sprint(it.ID))
	}
	for _, k := range []string{"film", "tv", "game", "music", "book"} {
		kinds = append(kinds, fmt.Sprintf(`%q:{"maxAge":7,"allowUnrated":false}`, k))
	}
	var fine []string
	for _, id := range f.fineLibs {
		fine = append(fine, fmt.Sprint(id))
	}
	open := `"maxAge":-1,"allowUnrated":true,"liveTv":true,"libraryChannels":true,"tunerChannels":true`
	profiles := []struct{ name, kind, limits string }{
		{"a Kids profile, by age", "kids", ""},
		{"a Guest, by age", "guest", ""},
		{"titles never allowed", "adult", fmt.Sprintf(`{%s,"blockItems":[%s],"blockShows":[%d]}`, open, strings.Join(blockItems, ","), f.show.ID)},
		{"a collection never allowed", "adult", fmt.Sprintf(`{%s,"blockCollections":[%d]}`, open, f.collection)},
		{"an age for each kind of media", "adult", fmt.Sprintf(`{"maxAge":7,"allowUnrated":false,"liveTv":true,"libraryChannels":true,"tunerChannels":true,"kinds":{%s}}`, strings.Join(kinds, ","))},
		{"only some libraries", "adult", fmt.Sprintf(`{%s,"libraries":[%s]}`, open, strings.Join(fine, ","))},
		{"a hidden genre", "adult", fmt.Sprintf(`{%s,"blockGenres":["%sgenre"]}`, open, hiddenWord)},
		{"only some genres", "adult", fmt.Sprintf(`{%s,"onlyGenres":["Finegenre"]}`, open)},
	}
	var routes []string
	for r := range routeRules {
		routes = append(routes, r)
	}
	sort.Strings(routes)

	// First as an adult nobody limited, with the same past the limited profiles are given: the
	// walk has to be able to find what is hidden, or it proves nothing.
	everyone, free := ts.user(t, "unlimited", "adult", false, "")
	f.watchedBefore(t, everyone.ID)
	freeRoom := f.playingWhatIsHidden(t, everyone.ID)
	if w := f.call("POST", "/api/syncplay/join", free, map[string]any{"roomId": freeRoom}); !leaks(w.Body.String()) {
		t.Errorf("the walk can't find the hidden titles in a watch party even for an adult with no limits: %d %.200s", w.Code, w.Body.String())
	}
	found := map[string]bool{}
	for _, route := range routes {
		if !strings.HasPrefix(route, "GET ") || walkSkips[route] != "" {
			continue
		}
		targets, _ := f.walkTargets(t, route)
		for _, target := range targets {
			if leaks(f.call("GET", target, free, nil).Body.String()) {
				found[route] = true
				break
			}
		}
	}
	for _, must := range []string{"GET /api/items", "GET /api/items/{id}", "GET /api/items/{id}/file", "GET /api/home", "GET /api/shows", "GET /api/shows/{id}", "GET /api/genres",
		"GET /api/collections/{id}", "GET /api/browse", "GET /api/music/artists", "GET /api/music/albums", "GET /api/radio", "GET /api/creators",
		"GET /api/videos", "GET /api/items/{id}/comments", "GET /api/brands", "GET /api/me/review", "GET /api/me/recommended", "GET /api/games/home", "GET /api/music/home",
		"GET /api/sdui/pages/{page}", "GET /api/sdui/topshelf", "GET /api/remote-control/devices"} {
		if !found[must] {
			t.Errorf("the walk can't find the hidden titles through %s even for an adult with no limits, so it would miss a leak there", must)
		}
	}
	if len(found) < 30 {
		t.Errorf("an adult with no limits found the hidden titles through only %d routes", len(found))
	}
	var through []string
	for r := range found {
		through = append(through, strings.TrimPrefix(r, "GET /api"))
	}
	sort.Strings(through)
	t.Logf("the hidden titles show through %d routes for an adult with no limits: %s", len(found), strings.Join(through, " "))

	for i, p := range profiles {
		u, tok := ts.user(t, fmt.Sprintf("limited%d", i), p.kind, false, p.limits)
		f.watchedBefore(t, u.ID)
		room := f.playingWhatIsHidden(t, u.ID)
		calls := 0
		for _, route := range routes {
			method := strings.SplitN(route, " ", 2)[0]
			if walkSkips[route] != "" {
				continue
			}
			targets, space := f.walkTargets(t, route)
			for _, target := range targets {
				var body any
				if method != "GET" {
					// Changing things isn't the walk's business, except where a title is asked for by its number.
					if method == "DELETE" || (space != "item" && space != "show" && space != "target") {
						continue
					}
					body = map[string]any{}
				}
				w := f.call(method, target, tok, body)
				calls++
				if leaks(w.Body.String()) || leaks(fmt.Sprint(w.Header())) {
					t.Errorf("%s: %s %s names what is hidden (%d): %.300s", p.name, method, target, w.Code, w.Body.String())
				}
				if method == "GET" && (space == "item" || space == "show" || space == "target") && w.Code >= 200 && w.Code < 300 {
					t.Errorf("%s: GET %s answered %d for a hidden title: %.200s", p.name, target, w.Code, w.Body.String())
				}
			}
		}
		// What other people are watching: the owner's TV is listed, and says only that it is
		// playing; the party is refused, by its id and by its code, and told nothing.
		devices := f.call("GET", "/api/remote-control/devices", tok, nil)
		if !strings.Contains(devices.Body.String(), `"hidden":true`) || strings.Contains(devices.Body.String(), `"itemId"`) {
			t.Errorf("%s: the owner's TV, playing what is hidden: %d %.300s", p.name, devices.Code, devices.Body.String())
		}
		for _, join := range []map[string]any{{"roomId": room}, {"code": f.room.Code}} {
			if w := f.call("POST", "/api/syncplay/join", tok, join); (w.Code >= 200 && w.Code < 300) || leaks(w.Body.String()) {
				t.Errorf("%s: joined a watch party of what is hidden: %d %.200s", p.name, w.Code, w.Body.String())
			}
		}
		for _, target := range []string{"/api/syncplay", "/api/syncplay/rooms/" + room, "/api/syncplay/rooms/" + room + "?member=" + f.room.You} {
			if w := f.call("GET", target, tok, nil); leaks(w.Body.String()) {
				t.Errorf("%s: GET %s names what is hidden: %.300s", p.name, target, w.Body.String())
			}
		}
		if w := f.call("POST", "/api/remote-control/devices/"+f.tv+"/commands", tok, map[string]any{"v": 1, "type": "play", "itemId": f.hidden[0].ID}); (w.Code >= 200 && w.Code < 300) || leaks(w.Body.String()) {
			t.Errorf("%s: asked the owner's TV to play what is hidden: %d %.200s", p.name, w.Code, w.Body.String())
		}
		if w := f.call("POST", "/api/syncplay/rooms", tok, map[string]any{"itemId": f.hidden[0].ID}); (w.Code >= 200 && w.Code < 300) || leaks(w.Body.String()) {
			t.Errorf("%s: started a watch party of what is hidden: %d %.200s", p.name, w.Code, w.Body.String())
		}
		// Asking in other ways.
		for _, c := range []struct {
			method, target string
			body           any
		}{
			{"POST", "/api/collections/preview", map[string]any{"rule": map[string]any{"libraries": f.hiddenLibs}}},
			{"POST", "/api/collections/preview", map[string]any{"rule": map[string]any{"search": hiddenSearch}}},
			{"POST", "/api/collections/preview", map[string]any{"rule": map[string]any{"genres": []string{hiddenWord + "genre"}}}},
			{"POST", "/api/logos/lookup", map[string]any{"names": []string{hiddenWord + " studio"}}},
			// Server-driven pages, each kind of them, and what a page can be made to name.
			{"GET", "/api/sdui/pages/search?q=" + hiddenSearch, nil}, {"GET", "/api/sdui/pages/libraries", nil}, {"GET", "/api/sdui/pages/collections", nil},
			{"GET", "/api/sdui/pages/music", nil}, {"GET", "/api/sdui/pages/books", nil}, {"GET", "/api/sdui/pages/games", nil}, {"GET", "/api/sdui/pages/photos", nil},
			{"GET", "/api/sdui/pages/livetv", nil}, {"GET", "/api/sdui/pages/home?platform=tvos&deviceClass=4k", nil}, {"GET", "/api/sdui/pages/home?components=none", nil},
			{"GET", "/api/sdui/navigation", nil}, {"GET", fmt.Sprintf("/api/sdui/pages/detail?show=%d", f.show.ID), nil},
			{"GET", fmt.Sprintf("/api/sdui/pages/detail-show/sections/episodes/items?show=%d&season=1", f.show.ID), nil},
			{"GET", fmt.Sprintf("/api/sdui/pages/detail?collection=%d", f.collection), nil},
			{"GET", fmt.Sprintf("/api/sdui/pages/detail-collection/sections/all/items?collection=%d", f.collection), nil},
		} {
			if w := f.call(c.method, c.target, tok, c.body); leaks(w.Body.String()) {
				t.Errorf("%s: %s %s names what is hidden: %.300s", p.name, c.method, c.target, w.Body.String())
			}
		}
		for _, lib := range f.hiddenLibs {
			for _, target := range []string{"/api/sdui/pages/library?library=%d", "/api/sdui/pages/library/sections/all/items?library=%d", "/api/sdui/pages/home/sections/recent-%d/items"} {
				if w := f.call("GET", fmt.Sprintf(target, lib), tok, nil); leaks(w.Body.String()) {
					t.Errorf("%s: GET %s names what is hidden: %.300s", p.name, fmt.Sprintf(target, lib), w.Body.String())
				}
			}
		}
		for _, it := range f.hidden {
			for _, target := range []string{"/api/sdui/pages/detail?item=%d", "/api/sdui/pages/detail/sections/similar/items?item=%d", "/api/sdui/pages/detail/diff?since=x&item=%d"} {
				if w := f.call("GET", fmt.Sprintf(target, it.ID), tok, nil); leaks(w.Body.String()) || (w.Code >= 200 && w.Code < 300) {
					t.Errorf("%s: GET %s for a hidden title: %d %.300s", p.name, fmt.Sprintf(target, it.ID), w.Code, w.Body.String())
				}
			}
		}
		for _, it := range f.hidden {
			for _, c := range []struct{ target string }{
				{"/api/items/%d/progress"}, {"/api/items/%d/playback"}, {"/api/items/%d/stream-token"}, {"/api/items/%d/dvd"}, {"/api/items/%d/view"},
				{"/api/items/%d/comments"}, {"/api/games/%d/play"}, {"/api/games/%d/prepare"},
			} {
				target := c.target
				body := map[string]any{"positionSec": 10, "body": "hello", "items": []int64{it.ID}, "ids": []int64{it.ID}, "itemId": it.ID}
				if strings.Contains(target, "%d") {
					target = fmt.Sprintf(target, it.ID)
				}
				if w := f.call("POST", target, tok, body); (w.Code >= 200 && w.Code < 300) || leaks(w.Body.String()) {
					t.Errorf("%s: POST %s for a hidden title: %d %.200s", p.name, target, w.Code, w.Body.String())
				}
			}
			// Saving for later answers politely, and adds nothing.
			if w := f.call("POST", "/api/collections/watch-later/items", tok, map[string]any{"items": []int64{it.ID}, "ids": []int64{it.ID}}); leaks(w.Body.String()) || strings.Contains(w.Body.String(), `"added":1`) {
				t.Errorf("%s: saved a hidden title for later: %d %s", p.name, w.Code, w.Body.String())
			}
			if w := f.call("POST", "/api/shares", tok, map[string]any{"kind": "item", "id": it.ID, "targetId": it.ID}); w.Code >= 200 && w.Code < 300 {
				t.Errorf("%s: made a share link to a hidden title: %s", p.name, w.Body.String())
			}
		}
		if t.Failed() {
			t.Fatalf("stopping at the first profile with leaks (%s)", p.name)
		}
		t.Logf("%s: %d calls, nothing hidden came back", p.name, calls)
	}
}
